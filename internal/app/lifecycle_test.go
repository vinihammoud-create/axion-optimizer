package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"axion/internal/collect"
	"axion/internal/facts"
	"axion/internal/netdiag"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

func TestBootCode(t *testing.T) {
	_, c := setupServer(t)
	origin := map[string]string{"X-Axion-Token": "", "Origin": "http://" + c.s.Host}
	code, err := c.s.BootCode()
	if err != nil {
		t.Fatal(err)
	}
	// Without the page's origin the code is not accepted (and not consumed).
	if st, _ := c.do("POST", "/api/boot", map[string]string{"code": code}, map[string]string{"X-Axion-Token": ""}); st != 403 {
		t.Errorf("sem Origin: %d", st)
	}
	if st, _ := c.do("POST", "/api/boot", map[string]string{"code": code}, map[string]string{"X-Axion-Token": "", "Origin": "http://127.0.0.1:1"}); st != 403 {
		t.Errorf("outra origem: %d", st)
	}
	wrong := "0" + code[1:]
	if code[0] == '0' {
		wrong = "1" + code[1:] // never the real code
	}
	if st, _ := c.do("POST", "/api/boot", map[string]string{"code": wrong}, origin); st != 401 {
		t.Errorf("código errado: %d", st)
	}
	st, b := c.do("POST", "/api/boot", map[string]string{"code": code}, origin)
	var out struct {
		Token string `json:"token"`
	}
	if st != 200 || json.Unmarshal(b, &out) != nil || out.Token != c.s.Token {
		t.Fatalf("troca do código: %d %s", st, b)
	}
	// One use only: the address left in the browser history is worthless.
	if st, _ := c.do("POST", "/api/boot", map[string]string{"code": code}, origin); st != 401 {
		t.Errorf("código reutilizado: %d", st)
	}
	// Expired codes are refused.
	code2, _ := c.s.BootCode()
	c.s.bootMu.Lock()
	c.s.boot[code2] = time.Now().Add(-time.Second)
	c.s.bootMu.Unlock()
	if st, _ := c.do("POST", "/api/boot", map[string]string{"code": code2}, origin); st != 401 {
		t.Errorf("código vencido: %d", st)
	}
	// The route never opens the rest of the API.
	if st, _ := c.do("GET", "/api/boot", nil, origin); st != 405 {
		t.Errorf("GET /api/boot: %d", st)
	}
}

func TestRunExclusive(t *testing.T) {
	m := NewJobs()
	release := make(chan struct{})
	j, err := m.Start("apply", "x", true, func(ctx context.Context, c *JobCtl) (any, error) { <-release; return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RunExclusive(func() error { return nil }); !errors.Is(err, ErrBusy) {
		t.Errorf("mudança rápida durante uma aplicação: %v", err)
	}
	close(release)
	for i := 0; i < 200; i++ {
		if _, running := m.Running(); !running {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = j
	// While a quick change runs, jobs that change the machine wait their turn.
	inside := make(chan struct{})
	done := make(chan error)
	go func() {
		done <- m.RunExclusive(func() error {
			close(inside)
			time.Sleep(50 * time.Millisecond)
			return nil
		})
	}()
	<-inside
	if _, err := m.Start("apply", "y", true, func(ctx context.Context, c *JobCtl) (any, error) { return nil, nil }); !errors.Is(err, ErrBusy) {
		t.Errorf("aplicação durante uma mudança rápida: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start("apply", "z", true, func(ctx context.Context, c *JobCtl) (any, error) { return nil, nil }); err != nil {
		t.Errorf("depois da mudança rápida: %v", err)
	}
}

func TestShutdownPolicy(t *testing.T) {
	a := &App{Jobs: NewJobs()}
	// A scan is cancelled at once.
	var cancelled atomic.Bool
	a.Jobs.Start("scan", "Análise", true, func(ctx context.Context, c *JobCtl) (any, error) {
		<-ctx.Done()
		cancelled.Store(true)
		return nil, ctx.Err()
	})
	if !a.Shutdown(3*time.Second, nil) || !cancelled.Load() {
		t.Fatal("a análise deveria ser cancelada ao fechar")
	}
	// An apply is never cancelled: Shutdown waits for it (and reports it).
	var slowTitle string
	var applyCancelled atomic.Bool
	a.Jobs.Start("apply", "Aplicando", true, func(ctx context.Context, c *JobCtl) (any, error) {
		select {
		case <-ctx.Done():
			applyCancelled.Store(true)
		case <-time.After(1800 * time.Millisecond):
		}
		return nil, nil
	})
	if !a.Shutdown(5*time.Second, func(title string) { slowTitle = title }) {
		t.Fatal("deveria esperar a aplicação terminar")
	}
	if applyCancelled.Load() {
		t.Error("uma aplicação não pode ser cancelada ao fechar a janela")
	}
	if slowTitle != "Aplicando" {
		t.Errorf("aviso de espera: %q", slowTitle)
	}
	// A step that cannot be interrupted is waited for, up to the limit.
	a.Jobs.Start("drivers-download", "Baixando", true, func(ctx context.Context, c *JobCtl) (any, error) {
		c.NoCancel("Windows Update")
		time.Sleep(400 * time.Millisecond)
		return nil, nil
	})
	time.Sleep(20 * time.Millisecond)
	if a.Shutdown(150*time.Millisecond, nil) {
		t.Error("com o limite curto, a tarefa ainda estaria rodando")
	}
	if !a.Shutdown(3*time.Second, nil) {
		t.Error("depois ela termina")
	}
}

func TestStaleUninstallTasksRemoved(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m.T.Add(`\Axion\Desinstalar 101112`, true, []int{sys.TriggerRegistration}, `C:\x\unins000.exe`)
	m.T.Add(`\Axion\Outra tarefa`, true, []int{sys.TriggerLogon}, `C:\x\y.exe`)
	a, err := New(m.Platform, "test")
	if err != nil {
		t.Fatal(err)
	}
	a.cleanupUninstallTasks() // also started in the background by New
	if ti, _ := m.T.Get(`\Axion\Desinstalar 101112`); ti.Exists {
		t.Error("tarefa de desinstalação antiga deveria ser removida")
	}
	if ti, _ := m.T.Get(`\Axion\Outra tarefa`); !ti.Exists {
		t.Error("outras tarefas do Axion não podem ser tocadas")
	}
}

func TestNetworkTest(t *testing.T) {
	m, c := setupServer(t)
	c.s.App.netTimingOverride = &netTiming{Idle: 400 * time.Millisecond, Load: 600 * time.Millisecond, Interval: 10 * time.Millisecond, LoadPing: 20 * time.Millisecond, Streams: [2]int{4, 3}}
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	c.wait(j.ID)
	c.json("POST", "/api/net/test", map[string]any{"label": "antes", "load": true, "dns": true}, &j)
	done := c.wait(j.ID)
	if done.Status != JobDone {
		t.Fatalf("teste de rede: %+v", done)
	}
	var v NetView
	c.json("GET", "/api/net", nil, &v)
	if len(v.Runs) != 1 || v.Last == nil || v.Adapter == nil || v.Adapter.Kind != "Ethernet" || v.Adapter.Gateway != "192.168.0.1" {
		t.Fatalf("página Rede: %+v", v)
	}
	r := v.Last
	if r.Label != "antes" || !r.Path.CGNAT || r.Path.Gateway != "192.168.0.1" || r.Path.FirstPublic != "187.16.216.10" {
		t.Errorf("caminho: %+v", r.Path)
	}
	if len(r.Relays) != 12 || r.Relays[0].Key != "gru" || r.BestValve() == nil || r.BestValve().POP != "gru" {
		t.Errorf("relays: %+v", r.Relays)
	}
	lan, inet := r.Target("lan"), r.Target("internet")
	if lan == nil || inet == nil || lan.Idle.Median > 1 || inet.Idle.Median < 7 || inet.Idle.Median > 10 {
		t.Errorf("latência em repouso: %+v %+v", lan, inet)
	}
	if r.Bloat == nil || len(r.Bloat.Phases) != 2 || r.Bloat.Grade == "" || r.Bloat.Phase("download").Mbps < 300 || r.Bloat.Phase("upload").AddedMs <= r.Bloat.Phase("download").AddedMs {
		t.Errorf("bufferbloat: %+v", r.Bloat)
	}
	if len(r.DNS) != 4 || !r.DNS[0].System || r.DNS[0].Queries != 12 {
		t.Errorf("DNS: %+v", r.DNS)
	}
	if !r.CS2.Found || r.CS2.BufferTicks != 1 {
		t.Errorf("CS2: %+v", r.CS2)
	}
	found := map[string]bool{}
	for _, f := range r.Findings {
		found[f.ID] = true
	}
	for _, id := range []string{"lan.ok", "net.ok", "path.cgnat", "cs2.buffer"} {
		if !found[id] {
			t.Errorf("faltou %s: %+v", id, r.Findings)
		}
	}
	if m.N.Net.Loads != 2 {
		t.Errorf("duas fases de carga: %d", m.N.Net.Loads)
	}
	// A second run without the load test, then the comparison.
	c.json("POST", "/api/net/test", map[string]any{"label": "depois", "load": false, "dns": false}, &j)
	c.wait(j.ID)
	c.json("GET", "/api/net", nil, &v)
	if len(v.Runs) != 2 || v.Runs[0].Label != "depois" || v.Runs[0].HasLoadRun {
		t.Fatalf("segundo teste: %+v", v.Runs)
	}
	var cmp NetCompare
	c.json("POST", "/api/net/compare", map[string]string{"a": v.Runs[1].ID, "b": v.Runs[0].ID}, &cmp)
	if len(cmp.Rows) == 0 {
		t.Fatal("comparação vazia")
	}
	for _, row := range cmp.Rows {
		if row.Label == "Download" && row.Verdict != "sem dados" {
			t.Errorf("sem teste de carga no B: %+v", row)
		}
	}
	// Offline: the internet part fails, the local part still reports.
	m.N.Net.Offline = true
	c.json("POST", "/api/net/test", map[string]any{"load": true, "dns": true}, &j)
	done = c.wait(j.ID)
	if done.Status != JobDone {
		t.Fatalf("offline: %+v", done)
	}
	c.json("GET", "/api/net", nil, &v)
	if len(v.Last.Errors) == 0 || v.Last.Bloat == nil || v.Last.Bloat.Grade != "" {
		t.Errorf("falhas precisam aparecer: %+v %+v", v.Last.Errors, v.Last.Bloat)
	}
	if st, _ := c.do("POST", "/api/net/delete", map[string]string{"id": "../../x"}, nil); st != 400 {
		t.Errorf("id inválido: %d", st)
	}
}

// A load test where one direction fails gets no grade (0.6.0 graded it from
// the other direction alone): the history says "incomplete" with the reason,
// and the CS2 page keeps the grade of the last complete test.
func TestNetworkTestPartialLoad(t *testing.T) {
	m, c := setupServer(t)
	c.s.App.netTimingOverride = &netTiming{Idle: 300 * time.Millisecond, Load: 500 * time.Millisecond, Interval: 10 * time.Millisecond, LoadPing: 20 * time.Millisecond, Streams: [2]int{2, 2}}
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	c.wait(j.ID)
	c.json("POST", "/api/net/test", map[string]any{"label": "antes", "load": true}, &j)
	if done := c.wait(j.ID); done.Status != JobDone {
		t.Fatalf("teste completo: %+v", done)
	}
	var v NetView
	c.json("GET", "/api/net", nil, &v)
	full := v.Runs[0]
	if full.Grade == "" || full.DownAddedMs < 0 || full.UpAddedMs < 0 || full.LoadError != "" {
		t.Fatalf("teste completo: %+v", full)
	}

	m.N.Net.FailDown = "o servidor de teste (Cloudflare) recusou o download com HTTP 429 Too Many Requests"
	c.json("POST", "/api/net/test", map[string]any{"label": "depois", "load": true}, &j)
	if done := c.wait(j.ID); done.Status != JobDone {
		t.Fatalf("teste parcial: %+v", done)
	}
	v = NetView{} // decode afresh: omitted fields must not keep the old values
	c.json("GET", "/api/net", nil, &v)
	b := v.Last.Bloat
	if b == nil || b.Grade != "" || !b.Partial || v.Last.AIM != nil || b.Phase("upload").Error != "" || !strings.Contains(b.Phase("download").Error, "HTTP 429") {
		t.Fatalf("sem nota quando uma direção falha: %+v", b)
	}
	partial := false
	for _, f := range v.Last.Findings {
		if f.ID == "bloat.ok" {
			t.Errorf("não pode dizer 'sem bufferbloat' sem o download: %+v", f)
		}
		if f.ID == "bloat.partial" {
			partial = true
		}
	}
	if !partial {
		t.Errorf("achado do teste incompleto: %+v", v.Last.Findings)
	}
	s := v.Runs[0]
	if s.Grade != "" || s.AddedMs != -1 || s.DownMbps != -1 || s.UpMbps <= 0 || s.UpAddedMs < 0 || !strings.Contains(s.LoadError, "download: ") {
		t.Errorf("histórico: %+v", s)
	}
	var g CS2Guide
	c.json("GET", "/api/cs2", nil, &g)
	for _, tip := range g.Tips {
		if tip.ID == "tip.network" && !strings.HasPrefix(tip.Current, "nota "+full.Grade+" ") {
			t.Errorf("a página do CS2 usa o último teste completo: %+v", tip)
		}
	}
}

// The adapter of the test is the one Windows routes the internet through,
// not simply the first one with a gateway (VPNs, virtual switches, Wi-Fi and
// cable at once).
func TestNetAdapterFollowsRoute(t *testing.T) {
	m, c := setupServer(t)
	vpn := sys.NetAdapter{Name: "VPN", Description: "WireGuard Tunnel", GUID: "{00000000-0000-0000-0000-00000000ABCD}", IfIndex: 40, IfType: 53, Up: true,
		MTU: 1420, Gateways: []string{"10.8.0.1"}, IPv4: []string{"10.8.0.2"}}
	m.N.NetAdapters = append([]sys.NetAdapter{vpn}, m.N.NetAdapters...)
	// Without a route answer, the physical adapter wins over the tunnel.
	m.N.BestIfIndex = 999
	ad, gw, err := c.s.App.netAdapter()
	if err != nil || ad.Kind != "Ethernet" || gw != "192.168.0.1" {
		t.Fatalf("sem rota: %+v %s %v", ad, gw, err)
	}
	// The route goes through the tunnel: the home network is still measured
	// through the physical adapter, and the tunnel is named.
	m.N.BestIfIndex = 40
	ad, gw, err = c.s.App.netAdapter()
	if err != nil || ad.Kind != "Ethernet" || gw != "192.168.0.1" || ad.Via != "WireGuard Tunnel" {
		t.Fatalf("rota pela VPN: %+v %s %v", ad, gw, err)
	}
	// Only a virtual adapter carries the route (the physical one is bound to
	// a virtual switch): that one is used.
	for i := range m.N.NetAdapters {
		if m.N.NetAdapters[i].Physical {
			m.N.NetAdapters[i].Gateways = nil
		}
	}
	m.N.NetAdapters[0] = sys.NetAdapter{Name: "vEthernet (Externa)", Description: "Hyper-V Virtual Ethernet Adapter", IfIndex: 40, IfType: 6, Up: true, MTU: 1500, Gateways: []string{"192.168.0.1"}}
	ad, gw, err = c.s.App.netAdapter()
	if err != nil || ad.Name != "vEthernet (Externa)" || gw != "192.168.0.1" || ad.Via != "" {
		t.Fatalf("só adaptador virtual: %+v %s %v", ad, gw, err)
	}
}

// Markers (lost probes) are stored exactly; measurements get 2 decimals.
func TestRound2KeepsMarkers(t *testing.T) {
	if round2(netdiag.Lost) != netdiag.Lost || round2(12.3456) != 12.35 || round2(0.004) != 0 {
		t.Errorf("round2: %v %v %v", round2(netdiag.Lost), round2(12.3456), round2(0.004))
	}
}

// Closing the interface stops the API before waiting for the work in
// progress; a quick change inside a request counts as work in progress.
func TestServerShutdownWaitsForQuickChange(t *testing.T) {
	_, c := setupServer(t)
	a := c.s.App
	release := make(chan struct{})
	entered := make(chan struct{})
	go func() {
		_ = a.Jobs.RunExclusive(func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	if !a.Jobs.Held() {
		t.Fatal("alteração rápida em andamento")
	}
	done := make(chan bool, 1)
	go func() { done <- a.Shutdown(5*time.Second, nil) }()
	select {
	case <-done:
		t.Fatal("encerrou no meio de uma alteração")
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if ok := <-done; !ok {
		t.Error("depois da alteração, nada mais em andamento")
	}
	if err := c.s.Shutdown(2 * time.Second); err != nil {
		t.Errorf("servidor: %v", err)
	}
	if res, err := http.Get("http://" + c.s.Host + "/api/state"); err == nil {
		res.Body.Close()
		t.Errorf("a API não pode aceitar pedidos depois de encerrada: %d", res.StatusCode)
	}
}

// Under extreme bufferbloat no ping comes back within the timeout: that is
// the worst grade, not a phase without data.
func TestNetworkTestNoReplyUnderLoad(t *testing.T) {
	m, c := setupServer(t)
	c.s.App.netTimingOverride = &netTiming{Idle: 300 * time.Millisecond, Load: 500 * time.Millisecond, Interval: 10 * time.Millisecond, LoadPing: 20 * time.Millisecond, Streams: [2]int{2, 2}}
	m.N.Net.BloatUp = 5000 // every ping during the upload takes longer than 2 s
	var j Job
	c.json("POST", "/api/net/test", map[string]any{"load": true, "dns": false}, &j)
	if done := c.wait(j.ID); done.Status != JobDone {
		t.Fatalf("teste: %+v", done)
	}
	var v NetView
	c.json("GET", "/api/net", nil, &v)
	b := v.Last.Bloat
	up := b.Phase("upload")
	if up == nil || !up.NoReply || up.LoadedMs != 2000 || b.Grade != "F" || v.Last.AIM == nil {
		t.Fatalf("sem resposta no upload: %+v %+v", up, b)
	}
	found := false
	for _, f := range v.Last.Findings {
		if f.ID == "bloat.bad" && strings.Contains(f.Title, "não voltou em 2 s") {
			found = true
		}
	}
	if !found {
		t.Errorf("achado do bufferbloat extremo: %+v", v.Last.Findings)
	}
	if got := censoredMedian([]float64{10, netdiag.Lost, netdiag.Lost, 12, netdiag.Lost}, 2000); got != 2000 {
		t.Errorf("mediana com perdas contadas como o limite: %v", got)
	}
}

// A part that ends the collection process is set aside and the analysis
// runs again without it; the choice holds for this version only.
func TestCollectorCrashIsSkipped(t *testing.T) {
	m, c := setupServer(t)
	a := c.s.App
	var calls [][]string
	a.Collector = func(ctx context.Context, skip []string, progress collect.Progress, mark func(string, bool)) (*facts.Facts, error) {
		calls = append(calls, append([]string(nil), skip...))
		for _, s := range skip {
			if s == collect.PartHAGS {
				set := map[string]bool{}
				for _, x := range skip {
					set[x] = true
				}
				return collect.CollectWith(ctx, m.Platform, progress, collect.Options{Skip: set, Mark: mark}), nil
			}
		}
		// The first run dies inside the driver query.
		if mark != nil {
			mark(collect.PartHAGS, false)
		}
		return nil, &CollectCrash{Part: collect.PartHAGS, Detail: "código de saída 0xC0000409"}
	}
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if done := c.wait(j.ID); done.Status != JobDone {
		t.Fatalf("a análise deveria seguir sem a parte que falhou: %+v", done)
	}
	if len(calls) != 2 || len(calls[1]) != 1 || calls[1][0] != collect.PartHAGS {
		t.Fatalf("tentativas: %+v", calls)
	}
	f := a.Facts()
	found := false
	for _, e := range f.Errors {
		if strings.Contains(e, collect.PartHAGS) && strings.Contains(e, "pulado") {
			found = true
		}
	}
	if !found {
		t.Errorf("a parte pulada precisa aparecer nos avisos da coleta: %v", f.Errors)
	}
	// The next analysis skips it from the start.
	calls = nil
	c.json("POST", "/api/scan", map[string]any{}, &j)
	c.wait(j.ID)
	if len(calls) != 1 || len(calls[0]) != 1 {
		t.Errorf("a parte fica de fora nesta versão: %+v", calls)
	}
	// A new version tries it again.
	a.Version = "9.9.9"
	if got := a.skippedParts(); len(got) != 0 {
		t.Errorf("nova versão tenta de novo: %v", got)
	}
	// A crash that repeats in a part already set aside stops the analysis
	// with the reason instead of looping.
	a.Version = "test"
	a.Collector = func(ctx context.Context, skip []string, progress collect.Progress, mark func(string, bool)) (*facts.Facts, error) {
		return nil, &CollectCrash{Part: collect.PartHAGS, Detail: "código de saída 1"}
	}
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if done := c.wait(j.ID); done.Status != JobFailed || !strings.Contains(done.Error, "código de saída 1") {
		t.Errorf("falha repetida: %+v", done)
	}
}

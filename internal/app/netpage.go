package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"axion/internal/facts"
	"axion/internal/games"
	"axion/internal/netdiag"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// NetView is the Rede page.
type NetView struct {
	Runs     []netdiag.Summary    `json:"runs"`
	Last     *netdiag.Run         `json:"last,omitempty"`
	Adapter  *netdiag.AdapterInfo `json:"adapter,omitempty"`
	Labels   []string             `json:"labels"`
	Honest   string               `json:"honest"`
	Privacy  string               `json:"privacy"`
	Sources  []tweaks.Source      `json:"sources"`
	Duration map[string]int       `json:"duration"` // seconds per phase, for the interface
	// RetryAt: the speed-test server refused the last load test and asked to
	// wait until then (Retry-After); a load test before it is refused here.
	RetryAt *time.Time `json:"retryAt,omitempty"`
}

// netWait remembers the wait the speed-test server asked for.
type netWait struct {
	mu sync.Mutex
	at time.Time
}

// netRetryAt returns when a load test may run again (zero: now).
func (a *App) netRetryAt() time.Time {
	a.netWait.mu.Lock()
	defer a.netWait.mu.Unlock()
	if time.Now().After(a.netWait.at) {
		return time.Time{}
	}
	return a.netWait.at
}

// noteRefusal keeps the server's Retry-After from a refused load phase.
func (a *App) noteRefusal(err error) {
	var w interface{ Wait() time.Duration }
	if !errors.As(err, &w) {
		return
	}
	if d := w.Wait(); d > 0 {
		a.netWait.mu.Lock()
		if at := time.Now().Add(d); at.After(a.netWait.at) {
			a.netWait.at = at
		}
		a.netWait.mu.Unlock()
	}
}

// NetTestRequest starts a network test.
type NetTestRequest struct {
	Label string `json:"label"`
	Load  bool   `json:"load"` // bufferbloat and speed
	DNS   bool   `json:"dns"`
}

// NetIDRequest names a stored test.
type NetIDRequest struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// NetCompareRequest compares two tests.
type NetCompareRequest struct {
	A string `json:"a"`
	B string `json:"b"`
}

// NetCompare is the before x after table.
type NetCompare struct {
	A    netdiag.Summary      `json:"a"`
	B    netdiag.Summary      `json:"b"`
	Rows []netdiag.CompareRow `json:"rows"`
	Note string               `json:"note"`
}

// netTiming holds the phase lengths (tests shorten them).
type netTiming struct {
	Idle, Load time.Duration
	Interval   time.Duration // idle ping interval
	LoadPing   time.Duration // ping interval under load
	Streams    [2]int        // download, upload connections
}

var defaultNetTiming = netTiming{Idle: 10 * time.Second, Load: 8 * time.Second, Interval: 40 * time.Millisecond, LoadPing: 100 * time.Millisecond, Streams: [2]int{8, 6}}

func (a *App) netDir() string { return filepath.Join(a.P.DataDir, "rede") }

func (a *App) netTiming() netTiming {
	if a.netTimingOverride != nil {
		return *a.netTimingOverride
	}
	return defaultNetTiming
}

var (
	srcNetBufferbloat = tweaks.Source{Title: netdiag.SrcBufferbloatTest.Title, URL: netdiag.SrcBufferbloatTest.URL}
	srcNetSQM         = tweaks.Source{Title: netdiag.SrcBufferbloatFix.Title, URL: netdiag.SrcBufferbloatFix.URL}
	srcNetAIM         = tweaks.Source{Title: netdiag.SrcAIM.Title, URL: netdiag.SrcAIM.URL}
	srcNetAIMCode     = tweaks.Source{Title: netdiag.SrcAIMCode.Title, URL: netdiag.SrcAIMCode.URL}
	srcNetSDR         = tweaks.Source{Title: netdiag.SrcSDR.Title, URL: netdiag.SrcSDR.URL}
	srcNetCS2         = tweaks.Source{Title: netdiag.SrcCS2Buffer.Title, URL: netdiag.SrcCS2Buffer.URL}
	srcNetRPM         = tweaks.Source{Title: "IETF — Responsiveness under Working Conditions (latência com a conexão em uso)", URL: "https://datatracker.ietf.org/doc/draft-ietf-ippm-responsiveness/"}
	srcNetTPLink      = tweaks.Source{Title: netdiag.SrcTPLinkQoS.Title, URL: netdiag.SrcTPLinkQoS.URL}
)

// netDNSOffered tells whether the latest network test offered this DNS item
// (an action of dns.fail or dns.slow). The change is applied only from there.
func (a *App) netDNSOffered(id string) bool {
	runs := netdiag.List(a.netDir())
	if len(runs) == 0 {
		return false
	}
	r, err := netdiag.Load(a.netDir(), runs[0].ID) // the newest, as Net() does
	if err != nil {
		return false
	}
	for _, f := range r.Findings {
		if f.ID != "dns.fail" && f.ID != "dns.slow" {
			continue
		}
		for _, ac := range f.Actions {
			if ac.Kind != "apply" {
				continue
			}
			for _, t := range ac.Tweaks {
				if t == id {
					return true
				}
			}
		}
	}
	return false
}

// Net builds the Rede page.
func (a *App) Net() NetView {
	v := NetView{Runs: netdiag.List(a.netDir()),
		Honest: "O teste mede a latência (ping), a oscilação (jitter) e a perda de pacotes em cada trecho: do PC ao roteador, até a internet (Cloudflare, 1.1.1.1) e até os relays da Valve que o CS2 usa. " +
			"Depois ele enche a conexão por alguns segundos em cada sentido para ver quanto o ping sobe com a internet em uso (bufferbloat), que é o que acontece quando a Steam baixa algo ou alguém da casa vê vídeo. " +
			"Ping, jitter e perda são medidos com ICMP; o CS2 usa UDP e o placar dele pode diferir alguns milissegundos. Um teste só retrata aquele horário: compare vários antes e depois de mudar algo.",
		Privacy:  "O teste fala com três serviços, só quando você clica: a Steam (lista pública de relays da Valve), a Cloudflare (speed.cloudflare.com, para encher a conexão; ela vê o seu IP e o volume transferido, como no site dela) e os servidores de DNS testados. Nenhum resultado é enviado a ninguém: fica só neste PC.",
		Sources:  []tweaks.Source{srcNetBufferbloat, srcNetSQM, srcNetRPM, srcNetAIM, srcNetAIMCode, srcNetSDR, srcNetCS2, srcNetTPLink},
		Duration: map[string]int{"idle": int(a.netTiming().Idle.Seconds()), "load": int(a.netTiming().Load.Seconds())},
	}
	seen := map[string]bool{"antes": true, "depois": true}
	v.Labels = []string{"antes", "depois"}
	for _, r := range v.Runs {
		if r.Label != "" && !seen[r.Label] {
			seen[r.Label] = true
			v.Labels = append(v.Labels, r.Label)
		}
	}
	if len(v.Runs) > 0 {
		if r, err := netdiag.Load(a.netDir(), v.Runs[0].ID); err == nil {
			v.Last = r
		}
	}
	if ad, _, err := a.netAdapter(); err == nil {
		v.Adapter = &ad
	}
	if at := a.netRetryAt(); !at.IsZero() {
		v.RetryAt = &at
	}
	return v
}

// NetRun returns one stored test.
func (a *App) NetRun(id string) (*netdiag.Run, error) {
	r, err := netdiag.Load(a.netDir(), id)
	if err != nil {
		return nil, fmt.Errorf("%w: teste não encontrado", errNotFound)
	}
	return r, nil
}

// DeleteNetRun removes a stored test.
func (a *App) DeleteNetRun(req NetIDRequest) error {
	if err := netdiag.Delete(a.netDir(), req.ID); err != nil {
		return fmt.Errorf("%w: teste não encontrado", errBadRequest)
	}
	a.measurementsChanged()
	return nil
}

// LabelNetRun renames a stored test.
func (a *App) LabelNetRun(req NetIDRequest) (*netdiag.Run, error) {
	r, err := netdiag.Relabel(a.netDir(), req.ID, req.Label)
	if err != nil {
		return nil, fmt.Errorf("%w: teste não encontrado", errBadRequest)
	}
	a.measurementsChanged()
	return r, nil
}

// CompareNet compares two stored tests (A = before, B = after).
func (a *App) CompareNet(req NetCompareRequest) (NetCompare, error) {
	ra, err := netdiag.Load(a.netDir(), req.A)
	if err != nil {
		return NetCompare{}, fmt.Errorf("%w: teste A não encontrado", errBadRequest)
	}
	rb, err := netdiag.Load(a.netDir(), req.B)
	if err != nil {
		return NetCompare{}, fmt.Errorf("%w: teste B não encontrado", errBadRequest)
	}
	sa, sb := netdiag.Summarize(ra), netdiag.Summarize(rb)
	return NetCompare{A: sa, B: sb, Rows: netdiag.Compare(sa, sb),
		Note: "Diferenças menores que um mínimo prático (1 ms na latência, 0,5 ponto na perda, 5% na velocidade) contam como iguais: a internet varia de um minuto para o outro. Repita os testes em horários parecidos."}, nil
}

// bestInterfacer is implemented by platforms that can ask the OS which
// interface routes an address.
type bestInterfacer interface {
	BestInterface(ip string) (int, error)
}

// netAdapter picks the adapter the traffic to the internet really takes (the
// route Windows chooses for 1.1.1.1); when that is unknown, the first one up
// with an IPv4 gateway, physical first. It also returns the IPv4 gateway.
func (a *App) netAdapter() (netdiag.AdapterInfo, string, error) {
	list, err := a.P.Native.Adapters()
	if err != nil {
		return netdiag.AdapterInfo{}, "", err
	}
	usable := func(ad sys.NetAdapter) bool { return ad.Up && ipv4Gateway(ad.Gateways) != "" }
	routed := -1
	if bi, ok := a.P.Native.(bestInterfacer); ok {
		if idx, err := bi.BestInterface("1.1.1.1"); err == nil {
			for i, ad := range list {
				if ad.IfIndex == idx && usable(ad) {
					routed = i
					break
				}
			}
		}
	}
	// The home network is measured through a physical adapter: the routed
	// one when it is physical, else the first physical one connected. Only
	// virtual adapters (a Hyper-V switch, say) leave the routed one.
	best := -1
	if routed >= 0 && list[routed].Physical {
		best = routed
	}
	for i, ad := range list {
		if best >= 0 {
			break
		}
		if usable(ad) && ad.Physical {
			best = i
		}
	}
	if best < 0 {
		best = routed
	}
	for i, ad := range list {
		if best >= 0 {
			break
		}
		if usable(ad) {
			best = i
		}
	}
	if best < 0 {
		return netdiag.AdapterInfo{}, "", errors.New("nenhuma conexão de rede ativa com roteador")
	}
	ad := list[best]
	info := netdiag.AdapterInfo{GUID: ad.GUID, Name: ad.Name, Description: ad.Description, Kind: "Outro", LinkMbps: int64(ad.SpeedBps / 1_000_000), MTU: ad.MTU, Gateway: ipv4Gateway(ad.Gateways)}
	if routed >= 0 && routed != best {
		// The internet goes through a VPN or virtual adapter.
		info.Via = list[routed].Description
		if info.Via == "" {
			info.Via = list[routed].Name
		}
	}
	switch ad.IfType {
	case 6:
		info.Kind = "Ethernet"
	case 71:
		info.Kind = "Wi-Fi"
	}
	for _, d := range ad.DNS {
		// IPv6 resolvers count too: providers such as Claro hand them out
		// first, and Windows asks them before the IPv4 ones.
		if ip := net.ParseIP(d); ip != nil && !ip.IsLinkLocalUnicast() {
			info.DNS = append(info.DNS, d)
		}
	}
	// The DNS item (a public resolver, offered when the test shows the DNS
	// failing or slow) needs to read the adapter's DNS: on a Windows without
	// the API nothing is offered.
	if cfg, err := a.P.Native.DNSConfig(ad.GUID); err == nil {
		info.DNSRead, info.DoH = true, cfg.DoH
		info.DNSAuto = len(cfg.V4) == 0 && len(cfg.V6) == 0
		// IPv6 DNS only counts when it is a global address (not link-local,
		// unique-local or the old site-local one): a router that hands out only
		// its own fe80:: address gives no IPv6 path to a public resolver.
		for _, d := range ad.DNS {
			if ip := net.ParseIP(d); ip != nil && ip.To4() == nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !(ip[0] == 0xfe && ip[1]&0xc0 == 0xc0) {
				info.IPv6DNS = true
			}
		}
	}
	if f := a.Facts(); f != nil {
		for _, fa := range f.Adapters {
			if strings.EqualFold(fa.InterfaceGUID, ad.GUID) {
				info.EEE = keywordValue(fa, "*EEE")
				info.IntMod = keywordValue(fa, "*InterruptModeration")
				info.SpeedDuplex = keywordValue(fa, "*SpeedDuplex")
				info.Gigabit = fa.GigabitCapable()
			}
		}
	}
	return info, info.Gateway, nil
}

func keywordValue(a facts.Adapter, name string) string {
	if k, ok := a.Keywords[name]; ok {
		return k.Current
	}
	return ""
}

func ipv4Gateway(gws []string) string {
	for _, g := range gws {
		if ip := net.ParseIP(g); ip != nil && ip.To4() != nil && !ip.IsUnspecified() {
			return ip.To4().String()
		}
	}
	return ""
}

// StartNetTest runs the network test.
func (a *App) StartNetTest(req NetTestRequest) (Job, error) {
	body, err := a.netTestJob(req)
	if err != nil {
		return Job{}, err
	}
	return a.Jobs.Start("net", "Testando a rede", true, body)
}

// wlanReading asks Windows for the signal and band of the Wi-Fi adapter with
// this GUID. Any failure (no permission, no Wi-Fi service, a timeout) gives
// 0 and "": the test then simply has no Wi-Fi remark.
func (a *App) wlanReading(ctx context.Context, guid string) (int, string) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	res, err := a.P.Run.Run(ctx, "netsh.exe", []string{"wlan", "show", "interfaces"}, sys.RunOpts{Timeout: 5 * time.Second})
	if err != nil || res.ExitCode != 0 {
		return 0, ""
	}
	return netdiag.ParseWlanInterfaces(res.Stdout, guid)
}

// netTestJob checks the request and returns the body of the network test,
// so the test runs alone (StartNetTest) or as a stage of the full
// evaluation.
func (a *App) netTestJob(req NetTestRequest) (func(ctx context.Context, c *JobCtl) (any, error), error) {
	if a.P.Net == nil {
		return nil, errors.New("teste de rede indisponível nesta plataforma")
	}
	ad, gw, err := a.netAdapter()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errBadRequest, err)
	}
	label := netdiag.CleanLabel(req.Label)
	if at := a.netRetryAt(); req.Load && !at.IsZero() {
		return nil, fmt.Errorf("%w: o servidor do teste de carga (Cloudflare) pediu para esperar até %s; faça o teste sem a carga agora ou tente depois desse horário", errBadRequest, at.Local().Format("15:04"))
	}
	return func(ctx context.Context, c *JobCtl) (any, error) {
		t := a.netTiming()
		start := time.Now()
		// The Wi-Fi signal is read here, once, because it is saved with the test.
		ad := ad
		if ad.Kind == "Wi-Fi" {
			ad.WiFiSignal, ad.WiFiBand = a.wlanReading(ctx, ad.GUID)
		}
		r := &netdiag.Run{ID: tweaks.NewID(), At: start, Label: label, Adapter: ad}
		errf := func(format string, args ...any) {
			msg := fmt.Sprintf(format, args...)
			r.Errors = append(r.Errors, msg)
			c.Logf("aviso: %s", msg)
		}
		c.Logf("adaptador: %s (%s, %d Mb/s), roteador %s", ad.Description, ad.Kind, ad.LinkMbps, gw)

		// 1. Route to the internet (TTL-limited probes, in parallel).
		c.Progress(0.03, "Descobrindo o caminho até a internet")
		r.Path = netdiag.AnalyzePath(a.tracePath(ctx, "1.1.1.1", 10))
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		for _, h := range r.Path.Hops {
			c.Logf("salto %d: %s %s", h.TTL, orDash(h.Addr), h.Kind)
		}

		// 2. Valve relays: quick look at every site, then the nearest ones.
		c.Progress(0.1, "Baixando a lista de relays da Valve (Steam)")
		var nearest []netdiag.RelayProbe
		if b, err := a.P.Net.SDRConfig(ctx, 730); err != nil {
			errf("lista de relays da Valve: %v", err)
		} else if pops, err := netdiag.ParseSDR(b); err != nil {
			errf("%v", err)
		} else {
			c.Progress(0.14, fmt.Sprintf("Medindo %d locais da Valve", len(pops)))
			r.Relays = a.probePOPs(ctx, pops)
			nearest = netdiag.Nearest(r.Relays, 3)
			if len(nearest) == 0 {
				errf("nenhum relay da Valve respondeu ao ping")
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// 3. Idle latency: router, internet and the nearest relays at once.
		c.Progress(0.22, fmt.Sprintf("Medindo latência, jitter e perda com a conexão parada (%d s)", int(t.Idle.Seconds())))
		targets := []netdiag.Target{
			{Name: "Roteador (" + gw + ")", Role: netdiag.RoleLAN, Addr: gw},
			{Name: "Internet (Cloudflare 1.1.1.1)", Role: netdiag.RoleInternet, Addr: "1.1.1.1"},
		}
		for _, n := range nearest {
			targets = append(targets, netdiag.Target{Name: n.City, Role: netdiag.RoleValve, Addr: n.Addr, POP: n.Key})
		}
		count := int(t.Idle / t.Interval)
		series := make([][]float64, len(targets))
		var wg sync.WaitGroup
		for i := range targets {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				series[i] = a.pingSeries(ctx, targets[i].Addr, count, t.Interval, time.Second)
			}(i)
		}
		wg.Wait()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		for i := range targets {
			targets[i].Idle = netdiag.Compute(series[i]).Round()
			targets[i].Series = netdiag.Downsample(series[i], 120)
			s := targets[i].Idle
			c.Logf("%s: mediana %.2f ms, P95 %.2f, jitter %.2f, perda %.1f%%", targets[i].Name, s.Median, s.P95, s.Jitter, s.LossPct)
		}
		r.Targets = targets

		// 4. Bufferbloat: fill the link each way while pinging.
		if req.Load {
			r.Bloat = &netdiag.Bloat{Server: "speed.cloudflare.com"}
			idle := targets[1].Idle
			var bytes int64
			for i, dir := range []sys.LoadDir{sys.LoadDownload, sys.LoadUpload} {
				name := [2]string{"download", "upload"}[i]
				c.Progress(0.5+0.18*float64(i), fmt.Sprintf("Enchendo a conexão: %s por %d s (medindo o ping ao mesmo tempo)", dirLabel(name), int(t.Load.Seconds())))
				ph := a.loadPhase(ctx, dir, t.Streams[i], t, gw, idle)
				ph.Dir = name
				bytes += ph.Bytes
				if ph.Error != "" {
					c.Logf("%s: %s", name, ph.Error)
				} else {
					c.Logf("%s: %.1f Mb/s, ping sob carga %.1f ms (+%.1f ms)", name, ph.Mbps, ph.Internet.Median, ph.AddedMs)
				}
				r.Bloat.Phases = append(r.Bloat.Phases, ph)
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
			}
			r.Bloat.DataMB = float64(bytes) / 1e6
			// The grade needs pings at idle and pings under load in BOTH
			// directions: the one that worked says nothing about the other,
			// and the worse of the two is the grade.
			worst, measured := 0.0, 0
			for i := range r.Bloat.Phases {
				if p := &r.Bloat.Phases[i]; p.Measured() {
					if measured == 0 || p.AddedMs > worst {
						worst = p.AddedMs
					}
					measured++
				}
			}
			switch {
			case measured > 0 && measured == len(r.Bloat.Phases):
				r.Bloat.AddedMs = worst
				r.Bloat.Grade = netdiag.BloatGrade(worst)
			case measured > 0:
				r.Bloat.Partial = true
			case !idle.OK():
				errf("sem resposta da internet com a conexão parada: não dá para calcular o bufferbloat")
			}
		}

		// 5. DNS.
		if req.DNS {
			c.Progress(0.88, "Medindo os servidores de DNS")
			r.DNS = a.dnsBench(ctx, ad.DNS)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
		}

		// 6. CS2 settings and verdicts.
		_, cs2 := a.Games()
		r.CS2 = cs2Net(cs2)
		netdiag.Analyze(r)
		r.Seconds = time.Since(start).Seconds()
		c.Progress(0.98, "Salvando o resultado")
		if err := netdiag.Save(a.netDir(), r); err != nil {
			return nil, fmt.Errorf("salvar o teste: %w", err)
		}
		// The diagnostic speaks from the newest test (the network grade).
		a.measurementsChanged()
		log.Printf("teste de rede %s: %d achados, %d avisos", r.ID, len(r.Findings), len(r.Errors))
		// Failures go to the log too, so a problem report carries the reason.
		for _, e := range r.Errors {
			log.Printf("teste de rede %s: %s", r.ID, e)
		}
		if r.Bloat != nil {
			for _, p := range r.Bloat.Phases {
				if p.Error != "" {
					log.Printf("teste de rede %s: %s falhou: %s", r.ID, p.Dir, p.Error)
				}
			}
		}
		return r, nil
	}, nil
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func dirLabel(d string) string {
	if d == "upload" {
		return "upload (envio)"
	}
	return "download (recebimento)"
}

func cs2Net(info games.CS2Info) netdiag.CS2Net {
	if !info.Installed && info.Video == nil {
		return netdiag.CS2Net{}
	}
	c := netdiag.CS2Net{Found: true, BufferTicks: info.ConvarInt("cl_net_buffer_ticks", -1), Rate: info.ConvarInt("rate", -1)}
	if len(info.ConvarFiles) > 0 {
		c.File = info.ConvarFiles[0]
	}
	return c
}

// tracePath sends TTL-limited probes (1..max) in parallel and returns the
// hops up to the destination.
func (a *App) tracePath(ctx context.Context, dst string, max int) []netdiag.Hop {
	hops := make([]netdiag.Hop, max)
	var wg sync.WaitGroup
	for ttl := 1; ttl <= max; ttl++ {
		wg.Add(1)
		go func(ttl int) {
			defer wg.Done()
			h := netdiag.Hop{TTL: ttl}
			for try := 0; try < 2 && ctx.Err() == nil && h.Addr == ""; try++ {
				rep, err := a.P.Native.Ping(dst, ttl, 32, false, 900*time.Millisecond)
				if err != nil || rep.From == "" || (!rep.OK && !rep.TTLHop) {
					continue
				}
				h.Addr, h.RTT, h.Dest = rep.From, round2(float64(rep.RTT)/float64(time.Millisecond)), rep.OK
			}
			hops[ttl-1] = h
		}(ttl)
	}
	wg.Wait()
	// Keep the hops up to the first one that reached the destination.
	for i, h := range hops {
		if h.Dest {
			return hops[:i+1]
		}
	}
	return hops
}

// round2 rounds a measurement to 2 decimals; negative values are markers
// (netdiag.Lost) and stay exact.
func round2(v float64) float64 {
	if v < 0 {
		return v
	}
	return math.Round(v*100) / 100
}

// probePOPs pings one relay of every Valve site three times (up to 16 sites
// at once) and keeps the best reply of each.
func (a *App) probePOPs(ctx context.Context, pops []netdiag.POP) []netdiag.RelayProbe {
	out := make([]netdiag.RelayProbe, len(pops))
	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	for i, p := range pops {
		wg.Add(1)
		go func(i int, p netdiag.POP) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			best := netdiag.Lost
			addr := p.Relays[0]
			for try := 0; try < 3 && ctx.Err() == nil; try++ {
				ip := p.Relays[try%len(p.Relays)]
				rep, err := a.P.Native.Ping(ip, 0, 32, false, time.Second)
				if err != nil || !rep.OK {
					continue
				}
				ms := float64(rep.RTT) / float64(time.Millisecond)
				if best < 0 || ms < best {
					best, addr = ms, ip
				}
			}
			out[i] = netdiag.RelayProbe{Key: p.Key, City: p.City(), Addr: addr, BestMs: round2(best)}
		}(i, p)
	}
	wg.Wait()
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].BestMs < 0) != (out[j].BestMs < 0) {
			return out[j].BestMs < 0
		}
		return out[i].BestMs < out[j].BestMs
	})
	return out
}

// pingSeries sends count echo requests every interval (each in flight on
// its own, so a lost reply does not delay the schedule) and returns the
// round-trip times in ms, netdiag.Lost for no reply.
func (a *App) pingSeries(ctx context.Context, ip string, count int, interval, timeout time.Duration) []float64 {
	out := make([]float64, count)
	for i := range out {
		out[i] = netdiag.Lost
	}
	var wg sync.WaitGroup
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for i := 0; i < count; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				wg.Wait()
				return out[:i]
			case <-tick.C:
			}
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rep, err := a.P.Native.Ping(ip, 0, 32, false, timeout)
			if err == nil && rep.OK {
				out[i] = float64(rep.RTT) / float64(time.Millisecond)
			}
		}(i)
	}
	wg.Wait()
	return out
}

// loadPhase saturates one direction while pinging the internet and the
// router, and measures the steady throughput.
func (a *App) loadPhase(ctx context.Context, dir sys.LoadDir, streams int, t netTiming, gw string, idle netdiag.Stats) netdiag.LoadPhase {
	ph := netdiag.LoadPhase{Streams: streams}
	var moved atomic.Int64
	pctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Byte counter sampled every 250 ms for the throughput.
	var samples []netdiag.ByteSample
	var smu sync.Mutex
	t0 := time.Now()
	go func() {
		tk := time.NewTicker(250 * time.Millisecond)
		defer tk.Stop()
		for {
			select {
			case <-pctx.Done():
				return
			case <-tk.C:
				smu.Lock()
				samples = append(samples, netdiag.ByteSample{At: time.Since(t0), Bytes: moved.Load()})
				smu.Unlock()
			}
		}
	}()
	// Pings during the load (after a short ramp-up, so they see a full queue).
	n := int((t.Load - t.Load/5) / t.LoadPing)
	var inet, lan []float64
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		select {
		case <-pctx.Done():
			return
		case <-time.After(t.Load / 5):
		}
		inet = a.pingSeries(pctx, "1.1.1.1", n, t.LoadPing, loadPingTimeout)
	}()
	go func() {
		defer wg.Done()
		select {
		case <-pctx.Done():
			return
		case <-time.After(t.Load / 5):
		}
		lan = a.pingSeries(pctx, gw, n, t.LoadPing, loadPingTimeout)
	}()
	st, err := a.P.Net.Load(ctx, dir, streams, t.Load, func(k int64) { moved.Add(k) })
	elapsed := time.Since(t0)
	smu.Lock()
	samples = append(samples, netdiag.ByteSample{At: elapsed, Bytes: moved.Load()})
	smp := append([]netdiag.ByteSample(nil), samples...)
	smu.Unlock()
	cancel() // stop the pings that are still scheduled
	wg.Wait()
	ph.Bytes, ph.Seconds = st.Bytes, round2(elapsed.Seconds())
	if err != nil {
		a.noteRefusal(err)
		ph.Error = "não foi possível encher a conexão: " + err.Error()
		return ph
	}
	ph.Mbps = netdiag.Throughput(smp, t.Load/4)
	ph.Internet = netdiag.Compute(inet).Round()
	ph.LAN = netdiag.Compute(lan).Round()
	ph.Series = netdiag.Downsample(inet, 80)
	if ph.Internet.Sent > 0 && idle.OK() {
		// A ping that did not come back in time under load waited at least
		// that long in a queue, or was dropped by a full one: it counts as the
		// timeout, so heavy bufferbloat is not hidden by the pings that made
		// it. Below the idle median is noise, not a gain: stored as 0.
		ph.LoadedMs = round2(censoredMedian(inet, float64(loadPingTimeout)/float64(time.Millisecond)))
		ph.AddedMs = round2(maxf(ph.LoadedMs-idle.Median, 0))
		ph.NoReply = !ph.Internet.OK()
	}
	return ph
}

// loadPingTimeout is how long a ping under load may take.
const loadPingTimeout = 2 * time.Second

// censoredMedian is the median of samples with the lost ones counted as
// timeoutMs.
func censoredMedian(samples []float64, timeoutMs float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	v := make([]float64, len(samples))
	for i, x := range samples {
		if x < 0 {
			x = timeoutMs
		}
		v[i] = x
	}
	sort.Float64s(v)
	return netdiag.Percentile(v, 0.5)
}

// dnsNames are looked up on every server (games, voice and common sites).
var dnsNames = []string{"steamcommunity.com", "api.steampowered.com", "store.steampowered.com", "www.faceit.com", "discord.com", "www.twitch.tv",
	"www.youtube.com", "www.google.com", "www.microsoft.com", "www.wikipedia.org", "www.instagram.com", "www.cloudflare.com"}

// dnsBench times every name on the configured servers and on the public
// ones, all servers at once.
func (a *App) dnsBench(ctx context.Context, system []string) []netdiag.DNSServer {
	var list []netdiag.DNSServer
	seen := map[string]bool{}
	for i, s := range system {
		if i >= 2 || seen[s] {
			continue
		}
		seen[s] = true
		list = append(list, netdiag.DNSServer{Addr: s, Name: "Configurado no Windows", System: true})
	}
	for _, r := range netdiag.PublicResolvers {
		// The benchmark queries the first IPv4 address of each resolver.
		p := struct{ addr, name string }{r.V4[0], r.Name}
		if seen[p.addr] {
			// The configured server is a public one: keep one line, marked.
			for i := range list {
				if list[i].Addr == p.addr {
					list[i].Name = p.name + " (configurado)"
				}
			}
			continue
		}
		seen[p.addr] = true
		list = append(list, netdiag.DNSServer{Addr: p.addr, Name: p.name})
	}
	var wg sync.WaitGroup
	for i := range list {
		wg.Add(1)
		go func(d *netdiag.DNSServer) {
			defer wg.Done()
			var times []float64
			for _, name := range dnsNames {
				if ctx.Err() != nil {
					return
				}
				d.Queries++
				rtt, rcode, err := a.P.Net.DNSQuery(ctx, d.Addr, name, 2*time.Second)
				if err != nil || (rcode != 0 && rcode != 3) {
					d.Failures++
					continue
				}
				times = append(times, float64(rtt)/float64(time.Millisecond))
			}
			if len(times) > 0 {
				sort.Float64s(times)
				d.Median = round2(netdiag.Percentile(times, 0.5))
				d.P90 = round2(netdiag.Percentile(times, 0.9))
			}
		}(&list[i])
	}
	wg.Wait()
	return list
}

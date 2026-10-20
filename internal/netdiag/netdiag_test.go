package netdiag

import (
	"math"
	"strings"
	"testing"
	"time"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestCompute(t *testing.T) {
	s := Compute([]float64{10, 12, Lost, 11, 30, 10})
	if s.Sent != 6 || s.Recv != 5 || !near(s.LossPct, 100.0/6) {
		t.Fatalf("contagem: %+v", s)
	}
	if s.Min != 10 || s.Max != 30 || s.Median != 11 {
		t.Errorf("min/max/mediana: %+v", s)
	}
	// Jitter skips the lost probe: |12-10| + |11-12| + |30-11| + |10-30| = 42 over 4.
	if !near(s.Jitter, 10.5) {
		t.Errorf("jitter: %v", s.Jitter)
	}
	if z := Compute([]float64{Lost, Lost}); z.OK() || z.LossPct != 100 {
		t.Errorf("tudo perdido: %+v", z)
	}
	if p := Percentile([]float64{1, 2, 3, 4}, 0.5); !near(p, 2.5) {
		t.Errorf("percentil interpolado: %v", p)
	}
	d := Downsample([]float64{1, 9, 2, 2, Lost, Lost, 3, 1}, 4)
	if len(d) != 4 || d[0] != 9 || d[2] != Lost || d[3] != 3 {
		t.Errorf("redução com o pior de cada balde: %v", d)
	}
}

func TestPath(t *testing.T) {
	for ip, want := range map[string]string{"192.168.0.1": KindPrivate, "10.20.0.1": KindPrivate, "172.20.1.1": KindPrivate, "100.64.12.1": KindCGNAT,
		"100.127.255.1": KindCGNAT, "100.128.0.1": KindPublic, "187.16.216.10": KindPublic, "169.254.1.1": KindLinkLocal, "x": KindUnknown} {
		if got := ClassifyIP(ip); got != want {
			t.Errorf("%s: %s, queria %s", ip, got, want)
		}
	}
	// Provider modem in router mode + own router: double NAT.
	p := AnalyzePath([]Hop{{TTL: 1, Addr: "192.168.1.1"}, {TTL: 2, Addr: "192.168.0.1"}, {TTL: 3, Addr: "10.12.0.1"}, {TTL: 4}, {TTL: 5, Addr: "187.16.216.10"}})
	if !p.DoubleNAT || p.CGNAT || p.Gateway != "192.168.1.1" || p.FirstPublic != "187.16.216.10" || p.PublicTTL != 5 {
		t.Errorf("NAT duplo: %+v", p)
	}
	if p.Hops[3].Kind != KindUnknown {
		t.Errorf("salto sem resposta: %+v", p.Hops[3])
	}
	// One router, provider CGNAT and 10/8 inside the provider: not double NAT.
	p = AnalyzePath([]Hop{{TTL: 1, Addr: "192.168.0.1"}, {TTL: 2, Addr: "100.64.12.1"}, {TTL: 3, Addr: "10.1.1.1"}, {TTL: 4, Addr: "200.160.0.10"}})
	if p.DoubleNAT || !p.CGNAT {
		t.Errorf("CGNAT: %+v", p)
	}
}

func TestParseSDR(t *testing.T) {
	b := []byte(`{"success":true,"pops":{
		"gru":{"desc":"Sao Paulo (Brazil)","geo":[-46.6,-23.5],"relays":[{"ipv4":"155.133.227.10","port_range":[27015,27060]},{"ipv4":"10.0.0.1"}]},
		"iad":{"desc":"Sterling (Virginia)","geo":[-77.4,39.0],"relays":[{"ipv4":"162.254.192.71"}]},
		"xyz":{"desc":"Partner only","geo":[0,0]}}}`)
	pops, err := ParseSDR(b)
	if err != nil || len(pops) != 2 {
		t.Fatalf("%v %+v", err, pops)
	}
	if pops[0].Key != "gru" || len(pops[0].Relays) != 1 || pops[0].City() != "São Paulo" || pops[0].Lat != -23.5 {
		t.Errorf("gru: %+v %s", pops[0], pops[0].City())
	}
	if pops[1].City() != "Sterling (EUA)" {
		t.Errorf("cidade: %s", pops[1].City())
	}
	if _, err := ParseSDR([]byte(`{"pops":{}}`)); err == nil {
		t.Error("lista vazia")
	}
	if _, err := ParseSDR([]byte(`<html>`)); err == nil {
		t.Error("formato inesperado")
	}
	near := Nearest([]RelayProbe{{Key: "a", BestMs: 40}, {Key: "b", BestMs: Lost}, {Key: "c", BestMs: 12}, {Key: "d", BestMs: 90}}, 2)
	if len(near) != 2 || near[0].Key != "c" || near[1].Key != "a" {
		t.Errorf("mais próximos: %+v", near)
	}
}

func TestGrades(t *testing.T) {
	for v, want := range map[float64]string{0: "A+", 4.9: "A+", 5: "A", 29: "A", 30: "B", 59: "B", 60: "C", 199: "C", 200: "D", 400: "F"} {
		if g := BloatGrade(v); g != want {
			t.Errorf("%v ms: %s, queria %s", v, g, want)
		}
	}
	// Cloudflare's scale: 9 ms idle (20) + no loss (10) + 3 ms added (20) = 50 -> great.
	if a := AIMGaming(9, 0, 3); a.Points != 50 || a.Class != "ótima" {
		t.Errorf("ótima: %+v", a)
	}
	// 35 ms idle (5) + 2% loss (5) + 70 ms added (0) = 10 -> poor.
	if a := AIMGaming(35, 0.02, 70); a.Points != 10 || a.Class != "fraca" {
		t.Errorf("fraca: %+v", a)
	}
	// Heavy loss drives the sum below zero: clamped.
	if a := AIMGaming(600, 0.6, 600); a.Points != 0 || a.Class != "ruim" {
		t.Errorf("ruim: %+v", a)
	}
	// 100 Mbit/s steady after a slow start that is left out.
	var s []ByteSample
	for i := 0; i <= 40; i++ {
		at := time.Duration(i) * 250 * time.Millisecond
		b := int64(i) * 3_125_000 // 100 Mbit/s
		if i < 8 {
			b = int64(i) * 1_000_000
		} else {
			b = 8_000_000 + int64(i-8)*3_125_000
		}
		s = append(s, ByteSample{At: at, Bytes: b})
	}
	if mb := Throughput(s, 2*time.Second); mb != 100 {
		t.Errorf("vazão: %v", mb)
	}
}

func baseRun() *Run {
	r := &Run{ID: "a1b2c3d4", At: time.Date(2026, 9, 26, 21, 0, 0, 0, time.UTC), Adapter: AdapterInfo{Kind: "Ethernet", LinkMbps: 1000}}
	r.Targets = []Target{
		{Name: "Roteador", Role: RoleLAN, Idle: Stats{Sent: 400, Recv: 400, Median: 0.4, P95: 0.7, Jitter: 0.1}},
		{Name: "Internet", Role: RoleInternet, Idle: Stats{Sent: 400, Recv: 400, Median: 8.6, P95: 9.8, Jitter: 0.6}},
		{Name: "São Paulo", Role: RoleValve, POP: "gru", Idle: Stats{Sent: 400, Recv: 400, Median: 12, P95: 13.5, Jitter: 0.9}},
		{Name: "Santiago", Role: RoleValve, POP: "scl", Idle: Stats{Sent: 400, Recv: 400, Median: 46, P95: 48, Jitter: 1.4}},
	}
	return r
}

func ids(r *Run) string {
	var out []string
	for _, f := range r.Findings {
		out = append(out, f.ID+":"+f.Severity)
	}
	return strings.Join(out, " ")
}

func TestAnalyzeClean(t *testing.T) {
	r := baseRun()
	r.Bloat = &Bloat{Grade: "A", AddedMs: 7, Phases: []LoadPhase{{Dir: "download", Mbps: 480, AddedMs: 7}, {Dir: "upload", Mbps: 240, AddedMs: 4}}}
	r.CS2 = CS2Net{Found: true, BufferTicks: 2, Rate: -1}
	r.DNS = []DNSServer{{Addr: "192.168.0.1", System: true, Median: 21, Queries: 12}, {Addr: "1.1.1.1", Name: "Cloudflare", Median: 9, Queries: 12}}
	r.Adapter.EEE = "1"
	Analyze(r)
	got := ids(r)
	for _, want := range []string{"lan.ok:good", "net.ok:good", "valve.route:good", "bloat.ok:good", "dns.ok:good", "cs2.buffer:info", "nic.eee:info"} {
		if !strings.Contains(got, want) {
			t.Errorf("faltou %s em %s", want, got)
		}
	}
	for _, f := range r.Findings {
		if f.ID == "cs2.buffer" && !strings.Contains(f.Title, "31,2") {
			t.Errorf("2 pacotes a mais = 2 ticks: %s", f.Title)
		}
		if f.ID == "valve.route" && !strings.Contains(f.Text, "GPN") {
			t.Errorf("rota curta explica o GPN: %s", f.Text)
		}
	}
	if r.Findings[0].Severity != "info" {
		t.Errorf("avisos antes dos pontos bons: %s", got)
	}
	if r.AIM == nil || r.AIM.Class != "ótima" {
		t.Errorf("AIM: %+v", r.AIM)
	}
}

func TestAnalyzeProblems(t *testing.T) {
	r := baseRun()
	r.Targets[0].Idle.LossPct = 1 // cable
	r.Targets[1].Idle.LossPct = 1 // the same loss shows on the way to the internet
	r.Targets[2].Idle.LossPct = 2.5
	r.Adapter.LinkMbps, r.Adapter.Gigabit = 100, true
	r.Bloat = &Bloat{Grade: "C", AddedMs: 85, Phases: []LoadPhase{{Dir: "download", Mbps: 94, AddedMs: 30}, {Dir: "upload", Mbps: 40, AddedMs: 85}}}
	r.Path = AnalyzePath([]Hop{{TTL: 1, Addr: "192.168.1.1"}, {TTL: 2, Addr: "192.168.0.1"}, {TTL: 3, Addr: "100.64.0.1"}, {TTL: 4, Addr: "187.1.1.1"}})
	r.CS2 = CS2Net{Found: true, BufferTicks: -1, Rate: 196608}
	r.DNS = []DNSServer{{Addr: "192.168.0.1", System: true, Median: 90, Queries: 10, Failures: 3}, {Addr: "1.1.1.1", Name: "Cloudflare", Median: 9, Queries: 10}}
	Analyze(r)
	got := ids(r)
	for _, want := range []string{"lan.loss:warning", "nic.link:warning", "valve.loss:warning", "bloat.bad:warning", "path.doublenat:info", "path.cgnat:info", "dns.fail:warning", "cs2.buffer:info", "cs2.rate:info"} {
		if !strings.Contains(got, want) {
			t.Errorf("faltou %s em %s", want, got)
		}
	}
	for _, f := range r.Findings {
		switch f.ID {
		case "bloat.bad":
			if !strings.Contains(f.Text, "upload (envio)") || !strings.Contains(f.Text, "download de 94 Mb/s e upload de 40 Mb/s") {
				t.Errorf("QoS com as velocidades medidas: %s", f.Text)
			}
		case "nic.link":
			if !strings.Contains(f.Text, "bateu no limite") {
				t.Errorf("download no limite do link: %s", f.Text)
			}
		case "cs2.buffer":
			if !strings.Contains(f.Title, "1 pacote") {
				t.Errorf("perda até a Valve pede buffer 1: %s", f.Title)
			}
		case "path.doublenat":
			if !strings.Contains(f.Text, "192.168.1.1 → 192.168.0.1") {
				t.Errorf("roteadores em cascata: %s", f.Text)
			}
		}
	}
	// The loss to the internet is not blamed on the provider when the LAN
	// already loses packets.
	if f := findingByID(r, "net.loss"); f == nil || !strings.Contains(f.Text, "dentro de casa") || strings.Contains(f.Title, "depois do seu roteador") {
		t.Errorf("perda local não é da operadora: %+v", f)
	}
	if f := findingByID(r, "valve.loss"); f == nil || !strings.Contains(f.Text, "só na rota até a Valve") {
		t.Errorf("perda até a Valve maior que a de casa: %+v", f)
	}
}

// A router that ignores pings addressed to itself is not a lossy cable: the
// pings through it to the internet arrive.
func TestAnalyzeRouterIgnoresPing(t *testing.T) {
	r := baseRun()
	r.Targets[0].Idle.LossPct = 3
	Analyze(r)
	got := ids(r)
	if !strings.Contains(got, "lan.icmp:info") || strings.Contains(got, "lan.loss") || strings.Contains(got, "net.loss") {
		t.Errorf("roteador limitando ping: %s", got)
	}
	// Wi-Fi: 0.8% to the router is below its limit, but it is most of the
	// 1.2% to the internet.
	r = baseRun()
	r.Adapter.Kind = "Wi-Fi"
	r.Targets[0].Idle.LossPct = 0.8
	r.Targets[1].Idle.LossPct = 1.2
	Analyze(r)
	if f := findingByID(r, "net.loss"); f == nil || !strings.Contains(f.Text, "maior parte já acontece dentro de casa") || !strings.Contains(f.Text, "Wi-Fi") {
		t.Errorf("perda do Wi-Fi: %+v", f)
	}
	if f := findingByID(r, "lan.ok"); f == nil || strings.Contains(f.Text, "Cabo") {
		t.Errorf("Wi-Fi não fala de cabo: %+v", f)
	}
	// Both parts matter: the home and the provider are named with their share.
	r = baseRun()
	r.Targets[0].Idle.LossPct = 0.8
	r.Targets[1].Idle.LossPct = 6
	Analyze(r)
	if f := findingByID(r, "net.loss"); f == nil || !strings.Contains(f.Text, "dois trechos") || !strings.Contains(f.Text, "5,2%") {
		t.Errorf("perda em casa e na operadora: %+v", f)
	}
	// Silent router and silent Cloudflare: loss to Valve cannot be compared.
	r = baseRun()
	r.Targets[0].Idle = Stats{Sent: 250, LossPct: 100}
	r.Targets[1].Idle = Stats{Sent: 250, LossPct: 100}
	r.Targets[2].Idle.LossPct = 2
	Analyze(r)
	if f := findingByID(r, "valve.loss"); f == nil || !strings.Contains(f.Text, "não dá para comparar") {
		t.Errorf("sem referência da internet: %+v", f)
	}
	// Traffic through a VPN is named.
	r = baseRun()
	r.Adapter.Via = "WireGuard Tunnel"
	Analyze(r)
	if f := findingByID(r, "path.via"); f == nil || !strings.Contains(f.Title, "WireGuard") {
		t.Errorf("VPN: %s", ids(r))
	}
	// No reply at all under load is the worst bufferbloat, not a missing test.
	r = baseRun()
	r.Bloat = &Bloat{Grade: "F", AddedMs: 1991, Phases: []LoadPhase{{Dir: "download", Mbps: 300, AddedMs: 30}, {Dir: "upload", Mbps: 20, AddedMs: 1991, NoReply: true}}}
	Analyze(r)
	if f := findingByID(r, "bloat.bad"); f == nil || !strings.Contains(f.Title, "não voltou em 2 s") || !strings.Contains(f.Text, "upload") {
		t.Errorf("sem resposta sob carga: %+v", f)
	}
}

func TestAnalyzeProviderLoss(t *testing.T) {
	r := baseRun()
	r.Targets = r.Targets[:3] // only one relay, far away
	r.Targets[1].Idle.LossPct = 3
	r.Targets[2].Idle.LossPct = 3
	r.Targets[2].Idle.Median = 60
	Analyze(r)
	got := ids(r)
	if !strings.Contains(got, "net.loss:warning") || !strings.Contains(got, "valve.loss:warning") || !strings.Contains(got, "valve.route:info") {
		t.Fatalf("perda na operadora: %s", got)
	}
	for _, f := range r.Findings {
		if f.ID == "valve.loss" && !strings.Contains(f.Text, "da sua conexão") {
			t.Errorf("a mesma perda até a Cloudflare aponta para a conexão: %s", f.Text)
		}
	}
}

func findingByID(r *Run, id string) *Finding {
	for i := range r.Findings {
		if r.Findings[i].ID == id {
			return &r.Findings[i]
		}
	}
	return nil
}

// Where a loss starts decides who is blamed; a small loss is never called
// "no loss"; the AIM class needs the load test.
func TestAnalyzeLossAttribution(t *testing.T) {
	// Loss at home and to the internet: net.loss is shown, blamed on the home.
	r := baseRun()
	r.Targets[0].Idle.LossPct = 2
	r.Targets[1].Idle.LossPct = 2
	r.Targets[2].Idle.LossPct = 2
	Analyze(r)
	if f := findingByID(r, "net.loss"); f == nil || !strings.Contains(f.Text, "dentro de casa") {
		t.Errorf("perda em casa e até a internet: %s", ids(r))
	}
	if f := findingByID(r, "valve.loss"); f == nil || !strings.Contains(f.Text, "dentro de casa") || !strings.Contains(f.Text, "Telemetria") {
		t.Errorf("perda até a Valve começando em casa: %+v", f)
	}
	if f := findingByID(r, "lan.loss"); f == nil || !strings.Contains(f.Text, "mesh") {
		t.Errorf("cabo até um nó de mesh: %+v", f)
	}
	if r.AIM != nil {
		t.Errorf("sem teste de carga não há classe AIM: %+v", r.AIM)
	}

	// The router does not answer ping: home and provider cannot be told apart.
	r = baseRun()
	r.Targets[0].Idle = Stats{Sent: 400, Recv: 0, LossPct: 100}
	r.Targets[1].Idle.LossPct = 2
	Analyze(r)
	if f := findingByID(r, "net.loss"); f == nil || !strings.Contains(f.Text, "não responde ao ping") || strings.Contains(f.Title, "depois do seu roteador") {
		t.Errorf("roteador sem resposta: %+v", f)
	}
	if findingByID(r, "lan.noreply") == nil {
		t.Errorf("roteador sem resposta: %s", ids(r))
	}

	// Loss after the router only.
	r = baseRun()
	r.Targets[1].Idle.LossPct = 2
	Analyze(r)
	if f := findingByID(r, "net.loss"); f == nil || !strings.Contains(f.Title, "depois do seu roteador") || !strings.Contains(f.Text, "não houve perda") {
		t.Errorf("perda na operadora: %+v", f)
	}

	// A small loss is tolerable but still reported.
	r = baseRun()
	r.Targets[0].Idle.LossPct = 0.25
	r.Targets[1].Idle.LossPct = 0.25
	r.Bloat = &Bloat{Phases: []LoadPhase{{Dir: "download", Error: "falhou"}}}
	Analyze(r)
	if f := findingByID(r, "lan.ok"); f == nil || !strings.Contains(f.Title, "perda de 0,25%") {
		t.Errorf("perda pequena no roteador: %+v", f)
	}
	if f := findingByID(r, "net.ok"); f == nil || !strings.Contains(f.Text, "Perda de 0,25%") {
		t.Errorf("perda pequena até a internet: %+v", f)
	}
	if r.AIM != nil {
		t.Errorf("teste de carga sem nota não dá classe AIM: %+v", r.AIM)
	}
	if s := Summarize(r); s.AddedMs != -1 || s.Grade != "" {
		t.Errorf("sem nota, o atraso sob carga não foi medido: %+v", s)
	}

	// Loaded latency below the idle one is noise, never a negative delay.
	r = baseRun()
	r.Bloat = &Bloat{Grade: "A+", AddedMs: -1.5, Phases: []LoadPhase{{Dir: "download", Mbps: 300, AddedMs: -1.5}}}
	Analyze(r)
	if s := Summarize(r); s.AddedMs != 0 {
		t.Errorf("atraso negativo: %+v", s)
	}
	if r.AIM == nil {
		t.Error("com nota de carga há classe AIM")
	}
}

func TestCompareAndHistory(t *testing.T) {
	dir := t.TempDir()
	a := baseRun()
	a.Label = "antes"
	a.Bloat = &Bloat{Grade: "C", AddedMs: 80, Phases: []LoadPhase{{Dir: "download", Mbps: 470}, {Dir: "upload", Mbps: 230}}}
	Analyze(a)
	b := baseRun()
	b.ID, b.At, b.Label = "b2c3d4e5", a.At.Add(time.Hour), "depois"
	b.Bloat = &Bloat{Grade: "A", AddedMs: 9, Phases: []LoadPhase{{Dir: "download", Mbps: 440}, {Dir: "upload", Mbps: 215}}}
	b.Targets[1].Idle.Median = 8.9 // within the practical minimum
	Analyze(b)
	if err := Save(dir, a); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, b); err != nil {
		t.Fatal(err)
	}
	list := List(dir)
	if len(list) != 2 || list[0].ID != b.ID || list[1].Grade != "C" || list[0].ValvePOP != "São Paulo" {
		t.Fatalf("histórico: %+v", list)
	}
	rows := Compare(list[1], list[0])
	v := map[string]string{}
	for _, r := range rows {
		v[r.Label] = r.Verdict
	}
	if v["Atraso a mais sob carga"] != "melhor" || v["Internet (mediana)"] != "igual" || v["Download"] != "pior" {
		t.Errorf("comparação: %+v", rows)
	}
	if _, err := Relabel(dir, a.ID, "  antes do QoS\x07 "); err != nil {
		t.Fatal(err)
	}
	if r, _ := Load(dir, a.ID); r.Label != "antes do QoS" {
		t.Errorf("rótulo: %q", r.Label)
	}
	if err := Delete(dir, "../x"); err == nil {
		t.Error("id inválido")
	}
	if err := Delete(dir, a.ID); err != nil || len(List(dir)) != 1 {
		t.Errorf("apagar: %v", err)
	}
}

func TestNum(t *testing.T) {
	for v, want := range map[float64]string{100: "100", 12: "12", 12.5: "12,5", 0.38: "0,38", 31.25: "31,2", 1234: "1234", 0: "0", 7.0: "7"} {
		if got := num(v); got != want {
			t.Errorf("num(%v) = %s, queria %s", v, got, want)
		}
	}
}

// A 10/100 card at 100 Mb/s is at its maximum: no cable warning.
func TestFastEthernetIsNotACableProblem(t *testing.T) {
	r := &Run{ID: "a1b2c3d5", Adapter: AdapterInfo{Kind: "Ethernet", LinkMbps: 100, Description: "Realtek PCIe FE Family Controller"}}
	Analyze(r)
	if strings.Contains(ids(r), "nic.link") {
		t.Errorf("Fast Ethernet card flagged: %s", ids(r))
	}
}

// A load test where one direction failed has no grade: the direction that
// worked says nothing about the other one (0.6.0 showed "A+" from the upload
// alone when the download was refused).
func TestPartialLoadTest(t *testing.T) {
	up := func(added float64) LoadPhase {
		return LoadPhase{Dir: "upload", Mbps: 83, Internet: Stats{Sent: 60, Recv: 60, Median: 12}, LoadedMs: 12 + added, AddedMs: added}
	}
	down := LoadPhase{Dir: "download", Error: "não foi possível encher a conexão: o servidor de teste (Cloudflare) recusou o download com HTTP 429 Too Many Requests"}

	r := baseRun()
	r.Bloat = &Bloat{Partial: true, Phases: []LoadPhase{down, up(0)}}
	Analyze(r)
	f := findingByID(r, "bloat.partial")
	if f == nil || f.Severity != "info" || !strings.Contains(f.Title, "o download não foi medido") || !strings.Contains(f.Text, "HTTP 429") {
		t.Fatalf("incompleto: %+v (%s)", f, ids(r))
	}
	if strings.Contains(ids(r), "bloat.ok") || strings.Contains(ids(r), "bloat.err") || r.AIM != nil {
		t.Errorf("sem nota, sem 'sem bufferbloat', sem AIM e sem achado duplicado: %s %+v", ids(r), r.AIM)
	}
	s := Summarize(r)
	if s.Grade != "" || s.AddedMs != -1 || s.UpAddedMs != 0 || s.DownAddedMs != -1 || s.DownMbps != -1 || s.UpMbps != 83 ||
		!strings.Contains(s.LoadError, "download: ") || s.Problems != 0 {
		t.Errorf("resumo: %+v", s)
	}

	// The direction measured already shows bufferbloat: that is reported.
	r = baseRun()
	r.Bloat = &Bloat{Partial: true, Phases: []LoadPhase{down, up(120)}}
	Analyze(r)
	f = findingByID(r, "bloat.partial")
	if f == nil || f.Severity != "warning" || !strings.Contains(f.Title, "Bufferbloat no upload: o ping sobe +120 ms") || !strings.Contains(f.Text, "C ou pior") {
		t.Fatalf("bufferbloat no que foi medido: %+v", f)
	}
	if s := Summarize(r); s.Problems != 1 || s.UpAddedMs != 120 {
		t.Errorf("resumo: %+v", s)
	}

	// Tests saved by 0.6.0 with an "A+" from the upload alone are fixed when read.
	dir := t.TempDir()
	old := baseRun()
	old.Bloat = &Bloat{Grade: "A+", AddedMs: 0, Phases: []LoadPhase{down, up(0)}}
	Analyze(old)
	if findingByID(old, "bloat.ok") == nil {
		t.Fatal("pré-condição: a 0.6.0 dizia 'sem bufferbloat'")
	}
	if err := Save(dir, old); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir, old.ID)
	if err != nil || got.Bloat.Grade != "" || !got.Bloat.Partial || findingByID(got, "bloat.ok") != nil || findingByID(got, "bloat.partial") == nil {
		t.Fatalf("teste antigo corrigido ao ler: %v %+v %s", err, got.Bloat, ids(got))
	}
	list := List(dir)
	if len(list) != 1 || list[0].Grade != "" || list[0].LoadError == "" {
		t.Errorf("histórico: %+v", list)
	}

	// Before x after: the overall delay needs both tests complete; each
	// direction compares on its own.
	a := Summary{AddedMs: 111, DownAddedMs: 111, UpAddedMs: 20, LANMs: -1, LANP95: -1, NetMs: -1, NetJitter: -1, NetLoss: -1, ValveMs: -1, ValveJit: -1, ValveLoss: -1, DownMbps: 827, UpMbps: 116}
	b := list[0]
	v := map[string]string{}
	for _, row := range Compare(a, b) {
		v[row.Label] = row.Verdict
	}
	if v["Atraso a mais sob carga"] != "sem dados" || v["Atraso a mais no download"] != "sem dados" || v["Atraso a mais no upload"] != "melhor" {
		t.Errorf("comparação: %+v", v)
	}
}

const wlanEN = `
There is 1 interface on the system:

    Name                   : Wi-Fi
    Description            : Intel(R) Wi-Fi 6E AX210 160MHz
    GUID                   : b1c2d3e4-f5a6-4b7c-8d9e-0f1a2b3c4d5e
    Physical address       : 74:56:3c:11:22:33
    State                  : connected
    SSID                   : Casa
    Radio type             : 802.11ax
    Authentication         : WPA2-Personal
    Band                   : 5 GHz
    Channel                : 36
    Receive rate (Mbps)    : 1201
    Signal                 : 87%
    Profile                : Casa
`

const wlanPT = "\r\nHá 2 interfaces no sistema:\r\n\r\n" +
	"    Nome                   : Wi-Fi\r\n" +
	"    GUID                   : aaaaaaaa-1111-2222-3333-444444444444\r\n" +
	"    Estado                 : conectado\r\n" +
	"    Banda                  : 2,4 GHz\r\n" +
	"    Sinal                  : 99 %\r\n" +
	"\r\n" +
	"    Nome                   : Wi-Fi 2\r\n" +
	"    GUID                   : bbbbbbbb-5555-6666-7777-888888888888\r\n" +
	"    Estado                 : conectado\r\n" +
	"    Banda                  : 5 GHz\r\n" +
	"    Sinal                  : 42%\r\n"

func TestParseWlanInterfaces(t *testing.T) {
	cases := []struct {
		name, out, guid string
		sig             int
		band            string
	}{
		{"en-US, GUID com chaves e maiúsculas", wlanEN, "{B1C2D3E4-F5A6-4B7C-8D9E-0F1A2B3C4D5E}", 87, "5 GHz"},
		{"pt-BR, 2,4 GHz e '99 %' com espaço (primeiro adaptador)", wlanPT, "{AAAAAAAA-1111-2222-3333-444444444444}", 99, "2.4 GHz"},
		{"pt-BR, segundo adaptador", wlanPT, "{bbbbbbbb-5555-6666-7777-888888888888}", 42, "5 GHz"},
		{"GUID que não está na saída", wlanPT, "{cccccccc-0000-0000-0000-000000000000}", 0, ""},
		{"GUID vazio", wlanEN, "", 0, ""},
		{"saída de erro sem blocos", "The Wireless AutoConfig Service (wlansvc) is not running.", "{b1c2d3e4-f5a6-4b7c-8d9e-0f1a2b3c4d5e}", 0, ""},
		{"sinal ilegível não vira número", "    GUID : x1\n    Signal : ???\n    Band : 5 GHz\n", "x1", 0, "5 GHz"},
	}
	for _, c := range cases {
		sig, band := ParseWlanInterfaces(c.out, c.guid)
		if sig != c.sig || band != c.band {
			t.Errorf("%s: sinal=%d banda=%q, esperava %d %q", c.name, sig, band, c.sig, c.band)
		}
	}
}

// The Wi-Fi remarks say the signal is from the start of the test, call the
// 70% line Axion's own, write 2,4 GHz with a comma and claim only what
// Microsoft's page says about that band.
func TestWiFiHints(t *testing.T) {
	for _, id := range []string{"lan.loss", "lan.spikes"} {
		r := baseRun()
		r.Adapter = AdapterInfo{Kind: "Wi-Fi", WiFiSignal: 55, WiFiBand: "2.4 GHz"}
		if id == "lan.loss" {
			r.Targets[0].Idle.LossPct = 3
			r.Targets[1].Idle.LossPct = 3
		} else {
			r.Targets[0].Idle.P95 = 30
		}
		Analyze(r)
		f := findingByID(r, id)
		if f == nil {
			t.Fatalf("%s: faltou o achado: %s", id, ids(r))
		}
		for _, want := range []string{"No começo do teste o sinal estava em 55%", "critério do Axion", "abaixo de 70%", "2,4 GHz", "micro-ondas, telefones sem fio e aparelhos Bluetooth"} {
			if !strings.Contains(f.Text, want) {
				t.Errorf("%s: o texto deveria conter %q: %s", id, want, f.Text)
			}
		}
		for _, bad := range []string{"atual", "2.4", "menos estável", "muito sensível"} {
			if strings.Contains(f.Text, bad) {
				t.Errorf("%s: o texto não deveria conter %q: %s", id, bad, f.Text)
			}
		}
		if len(f.Actions) == 0 {
			t.Errorf("%s: faltou o link da Microsoft", id)
		}
	}
	// Strong signal on 5 GHz: no extra remark at all.
	r := baseRun()
	r.Adapter = AdapterInfo{Kind: "Wi-Fi", WiFiSignal: 90, WiFiBand: "5 GHz"}
	r.Targets[0].Idle.LossPct = 3
	r.Targets[1].Idle.LossPct = 3
	Analyze(r)
	f := findingByID(r, "lan.loss")
	if f == nil || strings.Contains(f.Text, "sinal estava") || strings.Contains(f.Text, "GHz, que") {
		t.Errorf("sinal bom em 5 GHz não leva observação: %+v", f)
	}
	// A reading of exactly the line is not weak; nothing read (0) says nothing.
	for _, sig := range []int{WiFiWeakSignalPct, 0} {
		h := wifiHint(AdapterInfo{WiFiSignal: sig})
		if h != "" {
			t.Errorf("sinal %d: não deveria haver observação: %q", sig, h)
		}
	}
}

const testGUID = "{6A3F0C11-2B1D-4F5E-9C0A-7D1E2F3A4B5C}"

func dnsFailRun() *Run {
	r := baseRun()
	r.Adapter.GUID, r.Adapter.Name, r.Adapter.DNSRead = testGUID, "Ethernet", true
	r.DNS = []DNSServer{{Addr: "192.168.0.1", System: true, Median: 90, Queries: 10, Failures: 3}, {Addr: "1.1.1.1", Name: "Cloudflare", Median: 9, Queries: 10}}
	return r
}

// The DNS change is offered only in dns.fail and dns.slow, with the text that
// says it does not change the ping, and never on an adapter Axion must not touch.
func TestDNSFindingAction(t *testing.T) {
	r := dnsFailRun()
	Analyze(r)
	f := findingByID(r, "dns.fail")
	if f == nil || len(f.Actions) != 1 || f.Actions[0].Kind != "apply" {
		t.Fatalf("dns.fail deveria ter uma ação apply: %+v", f)
	}
	// No IPv6 DNS on the adapter: IPv4 only.
	if got := f.Actions[0].Tweaks; len(got) != 1 || got[0] != "dns:"+testGUID+":cloudflare:v4" {
		t.Errorf("ID sem IPv6: %v", got)
	}
	if !strings.Contains(f.Actions[0].Confirm, "não muda o ping") || !strings.Contains(f.Actions[0].Confirm, "automático") || !strings.Contains(f.Actions[0].Label, "da Cloudflare") {
		t.Errorf("texto da confirmação: %+v", f.Actions[0])
	}
	if strings.Contains(f.Actions[0].Confirm, "2606:") {
		t.Errorf("sem IPv6 no adaptador, o texto não pode prometer IPv6: %s", f.Actions[0].Confirm)
	}
	// With IPv6 DNS on the adapter: both protocols.
	r = dnsFailRun()
	r.Adapter.IPv6DNS = true
	Analyze(r)
	f = findingByID(r, "dns.fail")
	if got := f.Actions[0].Tweaks[0]; got != "dns:"+testGUID+":cloudflare" || !strings.Contains(f.Actions[0].Confirm, "2606:4700:4700::1111") {
		t.Errorf("com IPv6: %v %s", got, f.Actions[0].Confirm)
	}
	// DNS over HTTPS set by the user: no action, and the text says why.
	r = dnsFailRun()
	r.Adapter.DoH = true
	Analyze(r)
	if f = findingByID(r, "dns.fail"); f == nil || len(f.Actions) != 0 || !strings.Contains(f.Text, "DNS over HTTPS") {
		t.Errorf("DoH: %+v", f)
	}
	// Internet through a VPN: the adapter's DNS would not be the one used.
	r = dnsFailRun()
	r.Adapter.Via = "WireGuard"
	Analyze(r)
	if f = findingByID(r, "dns.fail"); f == nil || len(f.Actions) != 0 || !strings.Contains(f.Text, "WireGuard") {
		t.Errorf("VPN: %+v", f)
	}
	// Without the adapter GUID, or when its DNS could not be read: nothing to offer.
	for _, mod := range []func(*Run){func(r *Run) { r.Adapter.GUID = "" }, func(r *Run) { r.Adapter.DNSRead = false }} {
		r = dnsFailRun()
		mod(r)
		Analyze(r)
		if f = findingByID(r, "dns.fail"); f == nil || len(f.Actions) != 0 {
			t.Errorf("sem GUID ou sem leitura: %+v", f)
		}
	}
	// Slow DNS: the Google resolver is the best one.
	r = baseRun()
	r.Adapter.GUID, r.Adapter.Name, r.Adapter.IPv6DNS, r.Adapter.DNSRead = testGUID, "Ethernet", true, true
	r.DNS = []DNSServer{{Addr: "192.168.0.1", System: true, Median: 90, Queries: 10}, {Addr: "8.8.8.8", Name: "Google", Median: 20, Queries: 10}}
	Analyze(r)
	f = findingByID(r, "dns.slow")
	if f == nil || len(f.Actions) != 1 || !strings.HasSuffix(f.Actions[0].Tweaks[0], ":google") || !strings.Contains(f.Actions[0].Label, "do Google") {
		t.Fatalf("dns.slow: %+v", f)
	}
	// A DNS that is fine offers nothing.
	r = baseRun()
	r.Adapter.GUID = testGUID
	r.DNS = []DNSServer{{Addr: "192.168.0.1", System: true, Median: 21, Queries: 12}, {Addr: "1.1.1.1", Name: "Cloudflare", Median: 9, Queries: 12}}
	Analyze(r)
	if f = findingByID(r, "dns.ok"); f == nil || len(f.Actions) != 0 {
		t.Errorf("dns.ok: %+v", f)
	}
}

func TestParseDNSTweakID(t *testing.T) {
	g, k, v4, ok := ParseDNSTweakID("dns:" + testGUID + ":quad9")
	if !ok || g != testGUID || k != "quad9" || v4 {
		t.Errorf("válido: %q %q %v %v", g, k, v4, ok)
	}
	if _, k, v4, ok = ParseDNSTweakID(DNSTweakID(testGUID, "google", true)); !ok || k != "google" || !v4 {
		t.Errorf("com :v4: %q %v %v", k, v4, ok)
	}
	for _, bad := range []string{"dns:", "dns:" + testGUID, "dns:6A3F0C11-2B1D-4F5E-9C0A-7D1E2F3A4B5C:google", "dns:" + testGUID + ":opendns",
		"dns:" + testGUID + ":google:v6", "dns:" + testGUID + ":google:v4:x", "activehours:1-2", ""} {
		if _, _, _, ok := ParseDNSTweakID(bad); ok {
			t.Errorf("%q deveria ser recusado", bad)
		}
	}
	// One table for the benchmark and the item; every address parses.
	for _, r := range PublicResolvers {
		if len(r.V4) < 1 || len(r.V6) < 1 || r.Of == "" || r.Source.URL == "" {
			t.Errorf("resolvedor incompleto: %+v", r)
		}
		for _, a := range append(append([]string(nil), r.V4...), r.V6...) {
			if got, ok := ResolverByAddr(a); !ok || got.Key != r.Key {
				t.Errorf("%s deveria ser do %s", a, r.Key)
			}
		}
	}
	if r, ok := ResolverByAddr("2606:4700:4700:0:0:0:0:1111"); !ok || r.Key != "cloudflare" {
		t.Error("o endereço IPv6 por extenso deveria casar")
	}
	if _, ok := ResolverByAddr("192.168.0.1"); ok {
		t.Error("o roteador não é um resolvedor público")
	}
}

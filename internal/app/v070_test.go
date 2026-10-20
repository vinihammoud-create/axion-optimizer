package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"axion/internal/catalog"
	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/games"
	"axion/internal/netdiag"
	"axion/internal/perf"
	"axion/internal/profile"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// 0.7: the use profile. Choosing it through the API reads the report again
// (gaming findings set apart on a work PC, score up), filters the one-click
// plan (gaming items out of the recommended set, offered unchecked by the
// complete tier with the reason) and changes the pending counter; the CS2
// tier keeps the gaming reading; the live profile makes the HAGS optional.
func TestUseProfile(t *testing.T) {
	_, c := setupServer(t)
	a := c.s.App
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	var pl struct {
		Profiles []profile.Info `json:"profiles"`
		Current  string         `json:"current"`
		Chosen   bool           `json:"chosen"`
	}
	c.json("GET", "/api/profiles", nil, &pl)
	if len(pl.Profiles) != 5 || pl.Current != profile.Competitive || pl.Chosen {
		t.Fatalf("perfis: %+v", pl)
	}
	base := a.Report()
	if base.Profile != profile.Competitive {
		t.Fatalf("relatório sem perfil escolhido lê como competitivo: %q", base.Profile)
	}
	gaming := map[string]bool{}
	for _, x := range base.Findings {
		// What the OBS logs say is for who streams or records: the competitive
		// reading sets it aside, and only the live profile counts it.
		if x.OffProfile && profile.FindingScope(x.ID) != profile.ScopeLive {
			t.Fatalf("competitivo com achado fora do perfil: %s", x.ID)
		}
		if x.OffProfile != (profile.FindingScope(x.ID) == profile.ScopeLive) {
			t.Fatalf("o achado %s do OBS só conta no perfil live (fora do perfil: %v)", x.ID, x.OffProfile)
		}
		if profile.FindingScope(x.ID) != profile.ScopeGeneral && x.Severity != diag.SevGood {
			gaming[x.ID] = true
		}
	}
	if len(gaming) == 0 {
		t.Fatal("o mock deveria produzir achados de jogo")
	}
	var recBefore Plan
	c.json("GET", "/api/plan?tier=recomendado", nil, &recBefore)
	gameID := ""
	for _, it := range recBefore.Items {
		if !a.profile().TweakCounts(it.ID, it.Category) {
			t.Fatalf("no competitivo nada é filtrado: %s", it.ID)
		}
		if gameID == "" && profile.TweakScope(it.ID, it.Category) == profile.ScopeGaming {
			gameID = it.ID
		}
	}
	if gameID == "" || recBefore.Profile != profile.Competitive {
		t.Fatalf("plano recomendado do mock sem item de jogo: %+v", recBefore)
	}
	pendingBefore := a.PendingRecommended()

	// Work profile through the API.
	var st Settings
	c.json("POST", "/api/settings", map[string]any{"profile": profile.Work}, &st)
	if st.Profile != profile.Work {
		t.Fatalf("perfil salvo: %+v", st)
	}
	rep := a.Report()
	if rep.Profile != profile.Work {
		t.Fatalf("relatório relido para o perfil: %q", rep.Profile)
	}
	off := map[string]bool{}
	for _, x := range rep.Findings {
		if x.OffProfile {
			off[x.ID] = true
			if profile.FindingScope(x.ID) == profile.ScopeGeneral {
				t.Errorf("achado geral fora do perfil: %s", x.ID)
			}
		}
	}
	for id := range gaming {
		if !off[id] {
			t.Errorf("achado de jogo %s deveria estar fora do perfil trabalho", id)
		}
	}
	if rep.Overall < base.Overall {
		t.Errorf("a nota não pode cair ao tirar achados: %d → %d", base.Overall, rep.Overall)
	}
	var rec Plan
	c.json("GET", "/api/plan?tier=recomendado", nil, &rec)
	for _, it := range rec.Items {
		if profile.TweakScope(it.ID, it.Category) == profile.ScopeGaming {
			t.Errorf("item de jogo no recomendado do trabalho: %s", it.ID)
		}
	}
	if !strings.Contains(rec.Note, "Trabalho") || rec.Profile != profile.Work {
		t.Errorf("nota do plano: %q", rec.Note)
	}
	if n := a.PendingRecommended(); n >= pendingBefore || n != len(rec.Items) {
		t.Errorf("pendentes: %d antes, %d no trabalho, %d no plano", pendingBefore, n, len(rec.Items))
	}
	var comp Plan
	c.json("GET", "/api/plan?tier=completo", nil, &comp)
	found := false
	for _, it := range comp.Items {
		if it.ID == gameID {
			found = true
			if !it.Optional || it.Checked || !strings.Contains(it.Reason, "fora do 1 clique") {
				t.Errorf("%s no completo do trabalho: %+v", gameID, it)
			}
		}
	}
	if !found {
		t.Errorf("o completo oferece %s desmarcado", gameID)
	}
	var cs2 Plan
	c.json("GET", "/api/plan?tier=cs2", nil, &cs2)
	found = false
	for _, it := range cs2.Items {
		if it.ID == gameID && !it.Optional {
			found = true
		}
	}
	if !found {
		t.Error("o perfil CS2 mantém a leitura de jogo mesmo no trabalho")
	}

	// Live: a HAGS the machine recommends (an RTX with Frame Generation)
	// becomes the user's call, with the OBS reason; the mock's GPU does not
	// recommend it, so the rule is checked on the decision itself.
	hags := &tweaks.Tweak{ID: "game.hags", Category: "jogo", OneClick: true, Risk: tweaks.RiskSafe, Evidence: tweaks.EvMedium}
	yes := tweaks.State{Applicable: true, Status: "off", Rec: tweaks.Rec(tweaks.RecYes, "FG")}
	if ok, why := recommendedFor(hags, yes, profile.Get(profile.Live)); ok || !strings.Contains(why, "OBS") {
		t.Errorf("HAGS na live: %v %q", ok, why)
	}
	if ok, why := recommendedFor(hags, yes, profile.Get(profile.Competitive)); !ok || why != "" {
		t.Errorf("HAGS no competitivo: %v %q", ok, why)
	}
	if ok, why := recommendedFor(hags, yes, profile.Get(profile.Work)); ok || !strings.Contains(why, "Trabalho") {
		t.Errorf("HAGS no trabalho: %v %q", ok, why)
	}
	c.json("POST", "/api/settings", map[string]any{"profile": profile.Live}, &st)
	if r := a.Report(); r.Profile != profile.Live {
		t.Errorf("perfil live no relatório: %q", r.Profile)
	}

	// Unknown profile is refused; the simple mode is a plain setting.
	if code, _ := c.do("POST", "/api/settings", map[string]any{"profile": "x"}, nil); code != 400 {
		t.Errorf("perfil inválido: %d", code)
	}
	c.json("POST", "/api/settings", map[string]any{"simple": true}, &st)
	if !st.Simple || st.Profile != profile.Live {
		t.Errorf("modo simples: %+v", st)
	}
	c.json("GET", "/api/profiles", nil, &pl)
	if pl.Current != profile.Live || !pl.Chosen {
		t.Errorf("perfil atual: %+v", pl)
	}
	// Back to the default: the gaming findings count again.
	c.json("POST", "/api/settings", map[string]any{"profile": profile.Competitive}, &st)
	if r := a.Report(); r.Overall != base.Overall {
		t.Errorf("nota de volta ao competitivo: %d, era %d", r.Overall, base.Overall)
	}
}

// 0.7: the full evaluation runs the analysis, the network test, the FPS
// capture (the game is open) and the polling test in one job, and the
// three scores come out for the use profile; the PCIe link of the GPU is
// read while the game draws and reaches the diagnostic.
func TestFullEvaluation(t *testing.T) {
	m, c := setupServer(t)
	a := c.s.App
	a.netTimingOverride = &netTiming{Idle: 300 * time.Millisecond, Load: 500 * time.Millisecond, Interval: 10 * time.Millisecond, LoadPing: 20 * time.Millisecond, Streams: [2]int{2, 2}}
	if code, _ := c.do("GET", "/api/evaluation", nil, nil); code != 409 {
		t.Fatalf("avaliação sem análise: %d", code)
	}
	// The game is open.
	m.N.Procs = append(m.N.Procs, sys.ProcessInfo{PID: 7777, Name: "cs2.exe", WorkingSet: 2 << 30, Session: 1, Path: `C:\Program Files (x86)\Steam\steamapps\common\Counter-Strike Global Offensive\game\bin\win64\cs2.exe`})
	var j Job
	c.json("POST", "/api/evaluate", EvalRequest{Net: true, Perf: true, PerfSeconds: 30, Periph: true}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("avaliação: %+v", j)
	}
	b, _ := json.Marshal(j.Result)
	var res EvalResult
	if err := json.Unmarshal(b, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Steps) != 4 {
		t.Fatalf("etapas: %+v", res.Steps)
	}
	for _, st := range res.Steps {
		if st.Status != "done" {
			t.Errorf("etapa %s: %s %s", st.ID, st.Status, st.Note)
		}
	}
	if res.Scores == nil || res.Scores.Config == nil || res.Scores.Capacity == nil || res.Scores.Measurement == nil {
		t.Fatalf("notas: %+v", res.Scores)
	}
	var ev Evaluation
	c.json("GET", "/api/evaluation", nil, &ev)
	if ev.Profile.ID != profile.Competitive || ev.LastRun == nil || len(ev.Missing) != 0 || !ev.CanPerf || !ev.CS2Found {
		t.Fatalf("avaliação: perfil %q faltando %v canPerf %v cs2 %v lastRun %v", ev.Profile.ID, ev.Missing, ev.CanPerf, ev.CS2Found, ev.LastRun != nil)
	}
	ids := func(r *diag.Report) map[string]string {
		out := map[string]string{}
		for _, x := range r.Findings {
			out[x.ID] = x.Severity
		}
		return out
	}
	capIDs, measIDs := ids(ev.Capacity), ids(ev.Measurement)
	for _, id := range []string{"cap.cpu", "cap.ram", "cap.gpu", "cap.os", "cap.display", "cap.disk"} {
		if capIDs[id] != diag.SevGood {
			t.Errorf("capacidade %s: %q (mock: 7800X3D, 32 GB, RTX 3060, 240 Hz, SSD, Windows 11)", id, capIDs[id])
		}
	}
	if ev.Capacity.Overall != 100 {
		t.Errorf("nota de capacidade: %d", ev.Capacity.Overall)
	}
	for _, id := range []string{"net.bloat", "meas.fps", "meas.mouse"} {
		if _, ok := measIDs[id]; !ok {
			t.Errorf("medição sem %s: %v", id, measIDs)
		}
	}
	if measIDs["meas.mouse"] != diag.SevGood {
		t.Errorf("mouse sintético de 1000 Hz: %q", measIDs["meas.mouse"])
	}
	// The link was read during the capture and the newest capture carries it.
	runs := perf.List(a.perfDir())
	if len(runs) != 1 || runs[0].GPULink == nil || runs[0].GPULink.Width != 16 || runs[0].GPULink.Gen != 4 || !runs[0].GPULink.UnderLoad() {
		t.Fatalf("link PCIe na medição: %+v", runs)
	}
	if mm := a.measured(); mm.Link == nil || mm.LinkGame != "Counter-Strike 2" {
		t.Errorf("link nas medições do diagnóstico: %+v", mm)
	}
	// Work profile: the measurement reads the calls, not the game.
	var st Settings
	c.json("POST", "/api/settings", map[string]any{"profile": profile.Work}, &st)
	c.json("GET", "/api/evaluation", nil, &ev)
	measIDs, capIDs = ids(ev.Measurement), ids(ev.Capacity)
	if _, ok := measIDs["net.calls"]; !ok {
		t.Errorf("trabalho sem a leitura de chamadas: %v", measIDs)
	}
	if _, ok := measIDs["meas.fps"]; ok {
		t.Errorf("trabalho com FPS: %v", measIDs)
	}
	if capIDs["cap.ram"] != diag.SevGood || capIDs["cap.cpu"] != diag.SevGood || capIDs["cap.display"] != "" {
		t.Errorf("capacidade no trabalho: %v", capIDs)
	}
	if len(ev.Missing) != 0 {
		t.Errorf("faltando no trabalho: %v", ev.Missing)
	}
	// Nothing measured yet: the measurement pillar lists what is missing.
	_, c2 := setupServer(t)
	c2.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c2.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	c2.json("GET", "/api/evaluation", nil, &ev)
	// 0.8.1: with nothing measured the pillar has no score (a 100 would read as a result).
	if len(ev.Missing) != 3 || ev.Measurement.Overall != -1 || !ev.Measurement.Unscored || ev.CanPerf || ev.PerfWhy == "" {
		t.Errorf("sem medições: faltando %v, nota %d, canPerf %v (%s)", ev.Missing, ev.Measurement.Overall, ev.CanPerf, ev.PerfWhy)
	}
}

// 0.7: the exported report is anonymous unless the full one is asked for:
// the PC name, the Windows account (also inside paths), the Steam account,
// MAC and IP addresses and serial numbers are replaced; the test targets
// and the technical content stay.
func TestAnonymousExport(t *testing.T) {
	an := newAnonymizer("DESKTOP-GAMER", `DESKTOP-GAMER\Joao`, "", "Joao Silva")
	tree := map[string]any{
		"hostname": "DESKTOP-GAMER",
		"os":       map[string]any{"user": `DESKTOP-GAMER\Joao`, "elevatedAs": "", "caption": "Windows 11 Pro"},
		"adapters": []any{map[string]any{"name": "Ethernet", "driverVersion": "12.19.1.37", "mac": "A8-A1-59-12-34-56", "ipv4": []any{"192.168.0.15"}, "gateways": []any{"192.168.0.1"}, "dns": []any{"1.1.1.1"}}},
		"network":  []any{map[string]any{"dns": []any{map[string]any{"name": "Provedor", "addr": "189.40.1.2", "ms": 12.5}}, "label": "casa do Joao"}},
		"programs": []any{map[string]any{"name": "Discord", "path": `C:\Users\Joao\AppData\Local\Discord\Update.exe`}, map[string]any{"path": `C:\Users\Public\Desktop\jogo.lnk`}},
		"volumes":  []any{map[string]any{"letter": "D:", "label": "Jogos do Joao"}},
		"cs2":      map[string]any{"accountId": "123456789", "convars": map[string]any{"name": "JoaoSilva", "fps_max": "400"}, "video": map[string]any{"path": `C:\Program Files (x86)\Steam\userdata\123456789\730\local\cfg\cs2_video.txt`}},
		"evidence": "Roteador (192.168.0.1) a 1,2 ms; internet (1.1.1.1) a 9 ms; BSSID 3C:84:6A:AB:CD:EF; perfil em C:\\Users\\joao\\Documents; usuário Joao Silva no PC DESKTOP-GAMER; analisador ok; driver 2.1.4.3 de 2024; versão 23.90.0.5",
		"devices":  []any{map[string]any{"key": `USB\VID_0951&PID_1666\0012345ABCDE`, "pnp": `HID\VID_046D&PID_C539&MI_00\7&2b3c4d&0&0000`, "bt": `BTHENUM\{00001124-0000-1000-8000-00805f9b34fb}_VID&0002046d_PID&b035\7&1a2b3c&0&001A7DDA7113_C00000000`, "stor": `USBSTOR\DISK&VEN_KINGSTON&PROD_DATATRAVELER&REV_PMAP\0012345ABCDE&0`}},
		"ipv6":     "fe80::1c2b:3d4e:5f60:7a8b",
		"steam":    "https://steamcommunity.com/profiles/76561198012345678",
		"disks":    []any{map[string]any{"model": "Samsung 980", "serial": "S5GXNX0R123456", "wearPct": 3.0}},
		"time":     "15:04:05",
	}
	out := an.Apply(tree).(map[string]any)
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(out)
	s := buf.String()
	for _, bad := range []string{"DESKTOP-GAMER", "Joao", "joao", "A8-A1-59", "192.168.0.15", "192.168.0.1\"", "189.40.1.2", "123456789", "JoaoSilva", "3C:84:6A", "fe80::", "76561198012345678", "S5GXNX0R123456", "0012345ABCDE", "001A7DDA7113"} {
		if strings.Contains(s, bad) {
			t.Errorf("identificador %q sobrou: %s", bad, s)
		}
	}
	for _, keep := range []string{"Windows 11 Pro", "Ethernet", "1.1.1.1", `C:\\Users\\Public\\Desktop`, "Discord", "fps_max", "400", "Samsung 980", "15:04:05", "analisador ok", "12.19.1.37", "driver 2.1.4.3", "versão 23.90.0.5", "Provedor", "Jogos do <usuário>", `7&2b3c4d&0&0000`, `<série>&0`,
		"<usuário>", "<pc>", "<ip>", "<mac>", "<conta>", "<série>", "<steamid>", "<removido>", "<dns>"} {
		if !strings.Contains(s, keep) {
			t.Errorf("faltou %q em %s", keep, s)
		}
	}
	if out["os"].(map[string]any)["elevatedAs"] != "" {
		t.Error("campo vazio deve ficar vazio")
	}
	// Generic names are not scrubbed from free text (a PC called "PC").
	if got := newAnonymizer("PC", `PC\Dell`).text("o PC da Dell Inc. com o driver 1.2.3.4", false); got != "o PC da Dell Inc. com o driver 1.2.3.4" {
		t.Errorf("nomes genéricos: %q", got)
	}
	// Non-ASCII letters keep the byte offsets aligned.
	if got := newAnonymizer("İlker-PC", `İlker-PC\İlker`).text(`C:\Users\İlker\x e İLKER no İlker-PC`, false); !strings.Contains(got, `C:\Users\<usuário>\x`) || !strings.Contains(got, "<pc>") || strings.Contains(got, "İlker-PC") {
		t.Errorf("letras fora do ASCII: %q", got)
	}
	// Through the API: anonymous by default, full on request.
	_, c := setupServer(t)
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	f := c.s.App.Facts()
	_, raw := c.do("GET", "/api/export", nil, nil)
	_, full := c.do("GET", "/api/export?full=1", nil, nil)
	if f.Hostname == "" || !strings.Contains(string(full), `"hostname":"`+f.Hostname+`"`) {
		t.Errorf("nome do PC no relatório completo: %q", f.Hostname)
	}
	var anon struct {
		Anonymous bool   `json:"anonymous"`
		Profile   string `json:"profile"`
		Facts     struct {
			Hostname string `json:"hostname"`
		} `json:"facts"`
		Evaluation map[string]any `json:"evaluation"`
	}
	if err := json.Unmarshal(raw, &anon); err != nil || !anon.Anonymous || anon.Facts.Hostname != "<pc>" || anon.Profile == "" || anon.Evaluation == nil {
		t.Errorf("exportação anônima: %v %+v", err, anon)
	}
	if err := json.Unmarshal(full, &anon); err != nil || anon.Anonymous || anon.Facts.Hostname != f.Hostname {
		t.Errorf("exportação completa: %v %+v", err, anon)
	}
}

// 0.7: the HTML report is a self-contained page with the diagnostic, the
// three scores and the applied items; anonymous unless full.
func TestHTMLReport(t *testing.T) {
	_, c := setupServer(t)
	if code, _ := c.do("GET", "/api/export/html", nil, nil); code != 409 {
		t.Fatalf("sem análise: %d", code)
	}
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	c.json("POST", "/api/apply", ApplyRequest{IDs: []string{"privacy.ads"}}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("aplicar: %+v", j)
	}
	code, b := c.do("GET", "/api/export/html", nil, nil)
	s := string(b)
	if code != 200 || !strings.HasPrefix(s, "<!doctype html>") || strings.Contains(s, "<script") {
		t.Fatalf("página: %d %.80s", code, s)
	}
	f := c.s.App.Facts()
	for _, want := range []string{"relatório anônimo", "As três notas", "Diagnóstico (configuração)", "Capacidade", "Medição", "Otimizações aplicadas pelo Axion", "Anúncios, sugestões", f.CPU.Name, "Ainda não medido", "Fontes", "https://"} {
		if want == "Fontes" {
			continue
		}
		if !strings.Contains(s, want) {
			t.Errorf("faltou %q na página", want)
		}
	}
	if strings.Contains(s, "Nome do PC") {
		t.Error("nome do PC no relatório anônimo")
	}
	_, b = c.do("GET", "/api/export/html?full=1", nil, nil)
	if !strings.Contains(string(b), "Nome do PC") || !strings.Contains(string(b), f.Hostname) || strings.Contains(string(b), "relatório anônimo") {
		t.Error("relatório completo sem o nome do PC")
	}
}

// 0.7: the CS2 page is guided by the measurement. GPU-bound and short of
// the monitor → the video options above the floor are suggested, heaviest
// first, as a hypothesis to measure; CPU-bound → no video advice; the
// Steam client's downloads during the game are flagged with the measured
// bufferbloat; the Valve requirements are summarised.
func TestCS2MeasuredAdvice(t *testing.T) {
	f := &facts.Facts{Displays: []sys.DisplayMode{{Monitor: "Monitor 240", MaxHzAtRes: 240, MaxHzAny: 240, Primary: true}}, CPU: facts.CPUInfo{Name: "Ryzen", Cores: 8, Threads: 16}, RAM: facts.RAMInfo{TotalBytes: 32 << 30},
		GPUs: []facts.GPUInfo{{Name: "RTX 3060", Vendor: "NVIDIA", VRAMMiB: 12288}}, OS: facts.OSInfo{IsWin11: true, Build: 26200, DisplayVersion: "25H2", Caption: "Windows 11 Pro"}}
	info := games.CS2Info{Installed: true, Video: &games.CS2Video{Raw: map[string]string{"setting.msaa_samples": "2", "setting.videocfg_shadow_quality": "2", "setting.videocfg_dynamic_shadows": "1", "setting.videocfg_ao_detail": "0", "setting.videocfg_particle_detail": "0", "setting.shaderquality": "0", "setting.r_texturefilteringquality": "0"}}}
	// No capture yet.
	if it := videoAdvice(f, info, &diag.Measured{}); it.Status != "info" || !strings.Contains(it.Advice, "Grave 60 s") {
		t.Errorf("sem medição: %+v", it)
	}
	// GPU-bound, 1% low under 240 Hz: MSAA, dynamic shadows and shadow quality suggested; AO (off) and filtering (bilinear) not.
	m := &diag.Measured{Perf: &perf.Summary{AvgFPS: 370, Low1FPS: 174, GPUBoundPct: 65, CPUBoundPct: 28, Limiter: "gpu"}, PerfAt: time.Now().Add(-2 * 24 * time.Hour), PerfGame: "Counter-Strike 2"}
	it := videoAdvice(f, info, m)
	if it.Status != "action" {
		t.Fatalf("GPU limitando: %+v", it)
	}
	for _, want := range []string{"Anti-aliasing (MSAA): 2x MSAA → nenhum + CMAA2", "Sombras dinâmicas: todas → só o sol", "Qualidade das sombras: alta → média", "Hipótese a medir", "240 Hz"} {
		if !strings.Contains(it.Advice, want) {
			t.Errorf("faltou %q em %s", want, it.Advice)
		}
	}
	for _, no := range []string{"Oclusão", "Filtragem", "Partículas", "shaders"} {
		if strings.Contains(it.Advice, no) {
			t.Errorf("sugestão indevida %q em %s", no, it.Advice)
		}
	}
	if i := strings.Index(it.Advice, "Anti-aliasing"); i < 0 || i > strings.Index(it.Advice, "Sombras dinâmicas") {
		t.Error("o MSAA vem primeiro")
	}
	// Everything already at the floor: nothing to lower.
	info2 := games.CS2Info{Video: &games.CS2Video{Raw: map[string]string{"setting.msaa_samples": "0", "setting.videocfg_shadow_quality": "1", "setting.videocfg_dynamic_shadows": "0"}}}
	if it := videoAdvice(f, info2, m); it.Status != "action" || !strings.Contains(it.Advice, "já estão no mínimo") {
		t.Errorf("no mínimo: %+v", it)
	}
	// GPU-bound but above the monitor: ok.
	m2 := &diag.Measured{Perf: &perf.Summary{AvgFPS: 420, Low1FPS: 260, GPUBoundPct: 90, Limiter: "gpu"}, PerfAt: time.Now()}
	if it := videoAdvice(f, info, m2); it.Status != "ok" || !strings.Contains(it.Advice, "nada a baixar") {
		t.Errorf("acima do monitor: %+v", it)
	}
	// CPU-bound: no video advice.
	m3 := &diag.Measured{Perf: &perf.Summary{AvgFPS: 200, Low1FPS: 120, GPUBoundPct: 10, CPUBoundPct: 85, Limiter: "cpu"}, PerfAt: time.Now()}
	if it := videoAdvice(f, info, m3); it.Status != "ok" || !strings.Contains(it.Advice, "lado da CPU") {
		t.Errorf("CPU limitando: %+v", it)
	}
	// In-game cap.
	m4 := &diag.Measured{Perf: &perf.Summary{AvgFPS: 300, Low1FPS: 290, GPUBoundPct: 5, CPUBoundPct: 20, Limiter: "limite", CapLikely: true}, PerfAt: time.Now()}
	if it := videoAdvice(f, info, m4); it.Status != "ok" || !strings.Contains(it.Advice, "fps_max") {
		t.Errorf("limite do jogo: %+v", it)
	}

	// Steam client.
	sc, ok := games.ParseSteamClient(`"InstallConfigStore"
{
	"Software"
	{
		"Valve"
		{
			"Steam"
			{
				"AllowDownloadsDuringGameplay"		"1"
				"DownloadThrottleKbps"		"0"
				"AutoUpdateWindowEnabled"		"1"
				"AutoUpdateWindowStart"		"3"
				"AutoUpdateWindowEnd"		"7"
				"Accounts" { "x" { "SteamID" "76561198000000000" } }
			}
		}
	}
}`)
	if !ok || sc.DownloadsDuringGameplay != 1 || sc.ThrottleKbps != 0 || sc.AutoUpdateWindow != 1 || sc.WindowStart != 3 || sc.WindowEnd != 7 {
		t.Fatalf("config.vdf: %+v", sc)
	}
	bloat := &diag.Measured{Net: &netdiag.Summary{Grade: "C", AddedMs: 125}}
	tips := steamTips(games.CS2Info{Steam: sc}, bloat)
	by := map[string]CheckItem{}
	for _, x := range tips {
		by[x.ID] = x
	}
	if by["tip.steam.downloads"].Status != "action" || !strings.Contains(by["tip.steam.downloads"].Advice, "nota C") {
		t.Errorf("downloads durante o jogo: %+v", by["tip.steam.downloads"])
	}
	if by["tip.steam.throttle"].Status != "info" || by["tip.steam.window"].Status != "ok" || by["tip.steam.window"].Current != "das 03:00 às 07:00" {
		t.Errorf("limite e janela: %+v %+v", by["tip.steam.throttle"], by["tip.steam.window"])
	}
	// Keys absent: the client's defaults (paused during the game).
	sc2, _ := games.ParseSteamClient(`"InstallConfigStore" { "Software" { "Valve" { "Steam" { "Other" "1" } } } }`)
	tips = steamTips(games.CS2Info{Steam: sc2}, &diag.Measured{})
	if tips[0].Status != "ok" || tips[1].Status != "ok" || tips[2].Status != "info" {
		t.Errorf("padrões: %+v", tips)
	}
	if tips := steamTips(games.CS2Info{}, nil); len(tips) != 1 || tips[0].Status != "info" {
		t.Errorf("sem arquivo: %+v", tips)
	}

	// Requirements.
	if it := requirementsTip(f); it.Status != "ok" {
		t.Errorf("requisitos ok: %+v", it)
	}
	weak := *f
	weak.CPU = facts.CPUInfo{Name: "Pentium", Cores: 2, Threads: 2}
	weak.RAM = facts.RAMInfo{TotalBytes: 4 << 30}
	if it := requirementsTip(&weak); it.Status != "action" || !strings.Contains(it.Advice, "threads") || !strings.Contains(it.Advice, "Memória") {
		t.Errorf("abaixo do mínimo: %+v", it)
	}
	// The page carries the new items (mock: the client allows downloads).
	_, c := setupServer(t)
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	c.wait(j.ID)
	g, err := c.s.App.CS2Guide()
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, x := range append(g.Settings, g.Tips...) {
		ids[x.ID] = x.Status
	}
	if ids["cs2.video.measured"] != "info" || ids["tip.requirements"] != "ok" || ids["tip.steam.downloads"] != "action" || ids["tip.steam.window"] != "info" {
		t.Errorf("itens da página: %v", ids)
	}
}

// 0.7: the active hours of Windows Update are an item built from the
// window chosen ("activehours:20-2"), applied and undone through the
// engine; with the automatic adjustment on, the item says it is not in
// effect; the diagnostic reads the window.
func TestActiveHours(t *testing.T) {
	if _, _, ok := catalog.ParseActiveHoursID("activehours:20-2"); !ok {
		t.Fatal("ID válido recusado")
	}
	for _, bad := range []string{"activehours:8-8", "activehours:0-19", "activehours:24-3", "activehours:x-2", "profile:cs2.exe:gpu"} {
		if _, _, ok := catalog.ParseActiveHoursID(bad); ok {
			t.Errorf("ID inválido aceito: %s", bad)
		}
	}
	if !catalog.ValidActiveHours(6, 0) || catalog.ActiveHoursSpan(20, 2) != 6 || catalog.ValidActiveHours(0, 19) {
		t.Error("janela: até 18 h, passando da meia-noite")
	}
	m, c := setupServer(t)
	a := c.s.App
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	f := a.Facts()
	if f.OS.ActiveHoursStart != -1 || f.OS.ActiveHoursAuto != -1 {
		t.Fatalf("mock sem horário ativo: %+v", f.OS)
	}
	var found *diag.Finding
	for _, x := range a.Report().Findings {
		if x.ID == "os.activehours" {
			y := x
			found = &y
		}
	}
	if found == nil || found.Severity != diag.SevInfo || !strings.Contains(found.Title, "não definido por você") {
		t.Fatalf("achado do horário ativo: %+v", found)
	}
	// Apply 20:00-02:00.
	c.json("POST", "/api/apply", ApplyRequest{IDs: []string{"activehours:20-2"}, Label: "horário"}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("aplicar: %+v", j)
	}
	v, _ := m.R.GetValue(catalog.KeyActiveHours, "ActiveHoursStart")
	w, _ := m.R.GetValue(catalog.KeyActiveHours, "ActiveHoursEnd")
	if !v.Exists || v.Int != 20 || !w.Exists || w.Int != 2 {
		t.Fatalf("registro: %+v %+v", v, w)
	}
	f = a.Facts()
	if f.OS.ActiveHoursStart != 20 || f.OS.ActiveHoursEnd != 2 {
		t.Fatalf("releitura: %+v", f.OS)
	}
	for _, x := range a.Report().Findings {
		if x.ID == "os.activehours" && (x.Severity != diag.SevGood || !strings.Contains(x.Title, "20:00 às 02:00")) {
			t.Errorf("achado depois de aplicar: %+v", x)
		}
	}
	tw := a.Engine.Resolve("activehours:20-2", f)
	st := a.Engine.StateOf(context.Background(), f, tw)
	if st.Status != "on" || !st.ByAxion || st.Effect == "ignored" {
		t.Fatalf("estado: %+v", st)
	}
	// Automatic adjustment on: not in effect.
	_ = m.R.SetValue(catalog.KeyActiveHours, "SmartActiveHoursState", sys.DWord(1))
	a.refresh(context.Background())
	f = a.Facts()
	st = a.Engine.StateOf(context.Background(), f, tw)
	if st.Status != "on" || st.Effect != "ignored" || !strings.Contains(st.EffectNote, "Manualmente") {
		t.Fatalf("com ajuste automático: %+v", st)
	}
	for _, x := range a.Report().Findings {
		if x.ID == "os.activehours" && !strings.Contains(x.Title, "automaticamente") {
			t.Errorf("achado com ajuste automático: %+v", x)
		}
	}
	// Undo restores the absence of the values.
	c.json("POST", "/api/revert", RevertRequest{IDs: []string{"activehours:20-2"}}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("desfazer: %+v", j)
	}
	if v, _ := m.R.GetValue(catalog.KeyActiveHours, "ActiveHoursStart"); v.Exists {
		t.Errorf("valor não removido ao desfazer: %+v", v)
	}
	// The CS2 page carries the do-not-disturb tip with its settings page.
	g, _ := a.CS2Guide()
	ok := false
	for _, x := range g.Tips {
		if x.ID == "tip.dnd" && x.Open == "ms-settings:notifications" {
			ok = true
		}
	}
	if !ok {
		t.Error("dica do não perturbe")
	}
}

// 0.7: a stage of a composite job fills its share of the progress bar.
func TestJobCtlSpan(t *testing.T) {
	root := &JobCtl{}
	if got := root.span(0.5, 0.25, "x").frac(0.5); got != 0.625 {
		t.Errorf("subfaixa: %v", got)
	}
	if got := root.span(0.5, 0.25, "x").span(0.5, 0.5, "y").frac(1); got != 0.75 {
		t.Errorf("subfaixa aninhada: %v", got)
	}
	if got := root.frac(0.3); got != 0.3 {
		t.Errorf("raiz: %v", got)
	}
	if got := root.span(0, 0.4, "Análise").text("Jogos"); got != "Análise · Jogos" {
		t.Errorf("texto: %q", got)
	}
}

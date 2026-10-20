package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/games"
	"axion/internal/perf"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

// scanWith seeds the simulated cs2_video.txt and runs the scan, as the app
// does when the player opens it.
func scanWith(t *testing.T, m *mocksys.Mock, c *client, content string) string {
	t.Helper()
	p, err := m.SeedCS2Video(content)
	if err != nil {
		t.Fatal(err)
	}
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	return p
}

func presetItem(g CS2Guide, id string) *CS2PresetItem {
	if g.Preset == nil {
		return nil
	}
	for i := range g.Preset.Items {
		if g.Preset.Items[i].ID == id {
			return &g.Preset.Items[i]
		}
	}
	return nil
}

func TestMockCS2VideoPathMatches(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if mockCS2VideoPath(m.DataDir) != mocksys.CS2VideoPath(m.DataDir) {
		t.Fatalf("%s != %s", mockCS2VideoPath(m.DataDir), mocksys.CS2VideoPath(m.DataDir))
	}
	// The sample reads as the demo profile.
	v, ok := games.ParseCS2Video(mocksys.CS2VideoSample)
	if !ok || v.Width != 1280 || v.RefreshHz != 60 || v.LowLatency != 0 || v.VSync != 0 || v.Raw["setting.msaa_samples"] != "2" {
		t.Fatalf("exemplo não lido como o demo: %+v", v)
	}
}

// Without a seeded file the demo is as before: no card, and the preset
// never points at the fake Program Files path.
func TestCS2PresetAbsentInDemo(t *testing.T) {
	_, c := setupServer(t)
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	c.wait(j.ID)
	var g CS2Guide
	c.json("GET", "/api/cs2", nil, &g)
	if g.Preset != nil {
		t.Fatalf("o demo sem arquivo semeado não pode ter o cartão: %+v", g.Preset)
	}
	if p := c.s.App.cs2VideoPath(); p != "" {
		t.Fatalf("o demo apontou para um caminho real: %s", p)
	}
	if code, b := c.do("POST", "/api/cs2/preset", map[string]any{"ids": []string{"cs2video:aa"}}, nil); code != 400 || !strings.Contains(string(b), "não encontrado") {
		t.Errorf("sem arquivo: %d %s", code, b)
	}
}

func TestCS2PresetView(t *testing.T) {
	m, c := setupServer(t)
	scanWith(t, m, c, mocksys.CS2VideoSample)
	var g CS2Guide
	c.json("GET", "/api/cs2", nil, &g)
	if g.Preset == nil {
		t.Fatal("sem o cartão do preset")
	}
	p := g.Preset
	if p.Running || p.Cloud != "não" || !strings.Contains(p.CloudNote, "não deve sobrescrever") {
		t.Errorf("estado: running=%v cloud=%q %q", p.Running, p.Cloud, p.CloudNote)
	}
	aa := presetItem(g, "cs2video:aa")
	if aa == nil || len(aa.Changes) != 2 || !aa.Selected || !aa.Applicable {
		t.Fatalf("aa: %+v", aa)
	}
	if ch := aa.Changes[0]; ch.Key != "setting.msaa_samples" || ch.From != "2" || ch.To != "0" || ch.FromLabel != "2x MSAA" {
		t.Errorf("aa msaa: %+v", ch)
	}
	if ch := aa.Changes[1]; ch.Key != "setting.r_csgo_cmaa_enable" || ch.From != "0" || ch.To != "1" || ch.ToLabel != "CMAA2" {
		t.Errorf("aa cmaa: %+v", ch)
	}
	rf := presetItem(g, "cs2video:reflex")
	if rf == nil || len(rf.Changes) != 1 || rf.Changes[0].From != "0" || rf.Changes[0].To != "1" || rf.Changes[0].ToLabel != "ativado" || !rf.Selected {
		t.Fatalf("reflex: %+v", rf)
	}
	if presetItem(g, "cs2video:vsync") != nil {
		t.Error("o V-Sync saiu do preset (fonte não abriu)")
	}
	// Hypotheses are never pre-marked without a measurement; those with
	// something to change say why, the others say they are already done.
	for _, it := range p.Items {
		if it.Kind != "hipotese" {
			continue
		}
		if it.Selected {
			t.Errorf("%s marcado sem medição", it.ID)
		}
		if it.Why == "" {
			t.Errorf("%s sem motivo", it.ID)
		}
	}
	if sh := presetItem(g, "cs2video:shadows"); sh == nil || len(sh.Changes) != 1 || !strings.Contains(sh.Why, "sem medição") {
		t.Errorf("shadows: %+v", sh)
	}
	if ao := presetItem(g, "cs2video:ao"); ao == nil || len(ao.Changes) != 0 || ao.Why != "já no valor do preset" {
		t.Errorf("ao já no alvo: %+v", ao)
	}
	var keep []string
	for _, u := range p.Untouched {
		keep = append(keep, u.ID+"="+u.Current)
	}
	joined := strings.Join(keep, ";")
	if len(p.Untouched) != 6 || !strings.Contains(joined, "cs2.keep.res=1280×960") || !strings.Contains(joined, "cs2.keep.fpsmax=0") {
		t.Errorf("o que não muda: %s", joined)
	}
	if len(p.Active) != 0 {
		t.Errorf("nada aplicado ainda: %v", p.Active)
	}
	// The game opens: the card says so and nothing is applicable.
	m.N.AddProc(sys.ProcessInfo{PID: 4242, Name: "cs2.exe"})
	c.json("GET", "/api/cs2", nil, &g)
	if !g.Preset.Running {
		t.Error("não notou o jogo aberto")
	}
	if aa := presetItem(g, "cs2video:aa"); aa.Applicable || aa.Selected || !strings.Contains(aa.NAReason, "feche o jogo") {
		t.Errorf("aa com o jogo aberto: %+v", aa)
	}
}

func TestCS2PresetApplyAndUndo(t *testing.T) {
	m, c := setupServer(t)
	p := scanWith(t, m, c, mocksys.CS2VideoSample)
	ids := []string{"cs2video:aa", "cs2video:reflex", "cs2video:shadowquality"}
	var j Job
	c.json("POST", "/api/cs2/preset", map[string]any{"ids": ids}, &j)
	j = c.wait(j.ID)
	if j.Status != JobDone {
		t.Fatalf("aplicar: %+v", j)
	}
	var br BatchResult
	if err := decodeJobResult(j, &br); err != nil {
		t.Fatal(err)
	}
	if br.Counts["applied"] < 2 || br.Counts["failed"] != 0 {
		t.Fatalf("resultado: %+v", br.Counts)
	}
	b, _ := os.ReadFile(p)
	for k, want := range map[string]string{"setting.msaa_samples": "0", "setting.r_csgo_cmaa_enable": "1", "setting.r_low_latency": "1", "setting.videocfg_shadow_quality": "1", "setting.defaultres": "1280"} {
		if v, _, _ := games.CS2VideoValue(b, k); v != want {
			t.Errorf("%s = %s, esperado %s", k, v, want)
		}
	}
	var hist struct {
		Entries []HistoryEntry `json:"entries"`
	}
	c.json("GET", "/api/history", nil, &hist)
	n := 0
	for _, e := range hist.Entries {
		if e.BatchLabel == "Preset competitivo do CS2" {
			n++
		}
	}
	if n < 2 {
		t.Errorf("entradas do lote no histórico: %d", n)
	}
	copies, _ := os.ReadDir(filepath.Join(m.DataDir, "cs2"))
	if len(copies) != 1 {
		t.Fatalf("cópias de segurança: %d", len(copies))
	}
	if cp, _ := os.ReadFile(filepath.Join(m.DataDir, "cs2", copies[0].Name())); string(cp) != mocksys.CS2VideoSample {
		t.Error("a cópia não é o arquivo de antes")
	}
	// The card now lists what the Axion applied, ready for "Desfazer preset".
	var g CS2Guide
	c.json("GET", "/api/cs2", nil, &g)
	if len(g.Preset.Active) < 2 {
		t.Fatalf("ativos: %v", g.Preset.Active)
	}
	if aa := presetItem(g, "cs2video:aa"); aa.Status != "on" || !aa.ByAxion || aa.Drifted {
		t.Errorf("aa depois de aplicar: %+v", aa)
	}
	c.json("POST", "/api/revert", RevertRequest{IDs: g.Preset.Active}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("desfazer: %+v", j)
	}
	back, _ := os.ReadFile(p)
	if string(back) != mocksys.CS2VideoSample {
		t.Fatal("desfazer não devolveu o arquivo byte a byte")
	}
	ents, _ := os.ReadDir(filepath.Dir(p))
	for _, e := range ents {
		if strings.Contains(e.Name(), ".axion-") {
			t.Errorf("temporário sobrou: %s", e.Name())
		}
	}
}

func TestCS2PresetRefusedWhenRunning(t *testing.T) {
	m, c := setupServer(t)
	p := scanWith(t, m, c, mocksys.CS2VideoSample)
	m.N.AddProc(sys.ProcessInfo{PID: 4243, Name: "cs2.exe"})
	code, body := c.do("POST", "/api/cs2/preset", map[string]any{"ids": []string{"cs2video:aa"}}, nil)
	if code != 400 || !strings.Contains(string(body), "Feche o Counter-Strike 2") {
		t.Fatalf("%d %s", code, body)
	}
	if b, _ := os.ReadFile(p); string(b) != mocksys.CS2VideoSample {
		t.Error("arquivo mudou")
	}
	if _, running := c.s.App.Jobs.Running(); running {
		t.Error("abriu uma tarefa mesmo recusando")
	}
	if _, err := os.Stat(filepath.Join(m.DataDir, "cs2")); err == nil {
		t.Error("fez cópia de segurança mesmo recusando")
	}
	if len(c.s.App.Engine.J.Active()) != 0 {
		t.Error("entrada no histórico sem mudança")
	}
}

func TestCS2PresetRejectsForeignIDs(t *testing.T) {
	m, c := setupServer(t)
	scanWith(t, m, c, mocksys.CS2VideoSample)
	for name, ids := range map[string][]string{"id de outro item": {"game.ducking"}, "vazio": {}, "id desconhecido": {"cs2video:nao-existe"}, "misturado": {"cs2video:aa", "game.ducking"}} {
		if code, b := c.do("POST", "/api/cs2/preset", map[string]any{"ids": ids}, nil); code != 400 {
			t.Errorf("%s: %d %s", name, code, b)
		}
	}
	// The generic route does not take the preset's items either.
	if code, _ := c.do("POST", "/api/apply", ApplyRequest{IDs: []string{"cs2video:aa"}}, nil); code != 400 {
		t.Errorf("/api/apply aceitou item do preset: %d", code)
	}
}

// The filter "só se há anti-aliasing" is checked by the route too.
func TestCS2PresetRefusesAAWithoutAntialiasing(t *testing.T) {
	m, c := setupServer(t)
	b, _ := games.SetCS2VideoValue([]byte(mocksys.CS2VideoSample), "setting.msaa_samples", "0")
	p := scanWith(t, m, c, string(b))
	var g CS2Guide
	c.json("GET", "/api/cs2", nil, &g)
	if aa := presetItem(g, "cs2video:aa"); aa == nil || aa.Selected || !strings.Contains(aa.Why, "não liga") {
		t.Fatalf("aa sem anti-aliasing: %+v", aa)
	}
	if code, body := c.do("POST", "/api/cs2/preset", map[string]any{"ids": []string{"cs2video:aa"}}, nil); code != 400 || !strings.Contains(string(body), "não liga") {
		t.Errorf("%d %s", code, body)
	}
	if now, _ := os.ReadFile(p); !bytes.Equal(now, b) {
		t.Error("arquivo mudou")
	}
}

func TestCS2BackupKeepsTen(t *testing.T) {
	m, c := setupServer(t)
	p := scanWith(t, m, c, mocksys.CS2VideoSample)
	for i := 0; i < cs2BackupKeep+3; i++ {
		if _, err := c.s.App.backupCS2Video(p); err != nil {
			t.Fatal(err)
		}
	}
	ents, _ := os.ReadDir(filepath.Join(m.DataDir, "cs2"))
	if len(ents) != cs2BackupKeep {
		t.Errorf("cópias: %d, esperado %d", len(ents), cs2BackupKeep)
	}
}

// gpuShort gives the same verdicts as videoAdvice (without the text).
func TestGPUShortMatchesVideoAdvice(t *testing.T) {
	f := &facts.Facts{Displays: []sys.DisplayMode{{Monitor: "Monitor 240", MaxHzAtRes: 240, MaxHzAny: 240, Primary: true}}, GPUs: []facts.GPUInfo{{Name: "RTX 3060", Vendor: "NVIDIA"}}}
	info := games.CS2Info{Installed: true, Video: &games.CS2Video{Raw: map[string]string{"setting.msaa_samples": "2"}}}
	gpu := &diag.Measured{Perf: &perf.Summary{AvgFPS: 370, Low1FPS: 174, GPUBoundPct: 90, CPUBoundPct: 5, Limiter: "gpu"}, PerfAt: time.Now().Add(-24 * time.Hour)}
	if short, why := gpuShort(f, gpu); !short || why == "" {
		t.Errorf("GPU limitando: %v %q", short, why)
	}
	if it := videoAdvice(f, info, gpu); it.Status != "action" {
		t.Errorf("videoAdvice: %s", it.Status)
	}
	cpu := &diag.Measured{Perf: &perf.Summary{AvgFPS: 300, Low1FPS: 200, GPUBoundPct: 10, CPUBoundPct: 80, Limiter: "cpu"}, PerfAt: time.Now()}
	if short, _ := gpuShort(f, cpu); short {
		t.Error("CPU limitando: não é a placa")
	}
	if it := videoAdvice(f, info, cpu); it.Status != "ok" {
		t.Errorf("videoAdvice com CPU: %s", it.Status)
	}
	above := &diag.Measured{Perf: &perf.Summary{AvgFPS: 420, Low1FPS: 260, GPUBoundPct: 90, Limiter: "gpu"}, PerfAt: time.Now()}
	if short, _ := gpuShort(f, above); short {
		t.Error("1% low acima do monitor")
	}
	if short, _ := gpuShort(f, &diag.Measured{}); short {
		t.Error("sem medição")
	}
	// Monitor rate not read: videoAdvice says "info" and the hypotheses stay unmarked.
	noHz := &facts.Facts{GPUs: f.GPUs}
	if short, why := gpuShort(noHz, gpu); short || !strings.Contains(why, "taxa do monitor") {
		t.Errorf("sem taxa: %v %q", short, why)
	}
	if it := videoAdvice(noHz, info, gpu); it.Status != "info" {
		t.Errorf("videoAdvice sem taxa: %s", it.Status)
	}
	// A measurement older than 45 days does not mark anything.
	old := &diag.Measured{Perf: gpu.Perf, PerfAt: time.Now().Add(-60 * 24 * time.Hour)}
	if short, why := gpuShort(f, old); short || !strings.Contains(why, "45 dias") {
		t.Errorf("medição antiga: %v %q", short, why)
	}
}

func decodeJobResult(j Job, out any) error {
	b, err := json.Marshal(j.Result)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

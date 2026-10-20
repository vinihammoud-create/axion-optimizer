package app

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"axion/internal/facts"
	"axion/internal/games"
	"axion/internal/netprobe"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

// Items of the recommended plan the user unchecks are reported in the result
// and kept in the batch log of the export (the first notebook report could
// not tell why a recommended item stayed off).
func TestApplyReportsSkipped(t *testing.T) {
	_, c := setupServer(t)
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	var plan Plan
	c.json("GET", "/api/plan?tier=recomendado", nil, &plan)
	if len(plan.Items) < 3 {
		t.Fatalf("plano pequeno: %d", len(plan.Items))
	}
	apply := []string{plan.Items[0].ID}
	var skipped []string
	for _, it := range plan.Items[1:] {
		skipped = append(skipped, it.ID)
	}
	skipped = append(skipped, plan.Items[0].ID, "nao.existe") // ignored: applied / unknown
	c.json("POST", "/api/apply", ApplyRequest{IDs: apply, Skipped: skipped, Label: plan.Title}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("aplicação: %+v", j)
	}
	b, _ := json.Marshal(j.Result)
	var br BatchResult
	_ = json.Unmarshal(b, &br)
	if len(br.Skipped) != len(plan.Items)-1 || br.Skipped[0].ID != plan.Items[1].ID || br.Skipped[0].Title == "" || br.Skipped[0].Reason == "" {
		t.Fatalf("desmarcados: %+v", br.Skipped)
	}
	if !strings.Contains(strings.Join(j.Log, "\n"), "desmarcados por você") {
		t.Errorf("o log do lote deveria citar os desmarcados")
	}
	logs := c.s.App.Batches()
	if len(logs) != 1 || logs[0].Label != plan.Title || len(logs[0].Skipped) != len(br.Skipped) || logs[0].Outcomes[apply[0]] == "" {
		t.Fatalf("registro do lote: %+v", logs)
	}
	ex, _ := json.Marshal(c.s.App.exportRaw()["batches"])
	if !strings.Contains(string(ex), plan.Items[1].ID) {
		t.Errorf("o relatório exportado deveria trazer os lotes: %s", ex)
	}
	// The log survives a restart of Axion.
	a2, err := New(c.s.App.P, "test")
	if err != nil {
		t.Fatal(err)
	}
	if got := a2.Batches(); len(got) != 1 || got[0].Label != plan.Title {
		t.Errorf("lotes após reabrir: %+v", got)
	}
}

// CS2 asking for more than the connected monitor can show (the first
// notebook report: 165 Hz in cs2_video.txt and "-refresh 165" on a 120 Hz
// panel) is flagged, in the video file and in the launch options.
func TestCS2RefreshAboveMonitor(t *testing.T) {
	f := &facts.Facts{Displays: []sys.DisplayMode{{Device: `\\.\DISPLAY1`, Monitor: "Tela integrada", Width: 1920, Height: 1080, CurrentHz: 120, MaxHzAtRes: 120, Primary: true, Internal: true}}}
	info := games.CS2Info{Installed: true, Video: &games.CS2Video{Width: 1280, Height: 960, RefreshHz: 164.997, VSync: 0, LowLatency: 2}, LaunchOptions: "-high -fullscreen -refresh 165 -nojoy"}
	g := buildCS2Guide(f, info)
	by := map[string]CheckItem{}
	for _, it := range g.Settings {
		by[it.ID] = it
	}
	if it := by["cs2.refresh"]; it.Status != "action" || !strings.Contains(it.Advice, "(120 Hz, Tela integrada)") || !strings.Contains(it.Advice, "escolha 120 Hz") {
		t.Errorf("taxa do arquivo de vídeo: %+v", it)
	}
	if it := by["cs2.launch.refresh"]; it.Status != "action" || it.Current != "-refresh 165" || !strings.Contains(it.Advice, "-refresh 120") {
		t.Errorf("opção de inicialização: %+v", it)
	}
	info.LaunchOptions = "-freq 120"
	info.Video.RefreshHz = 119.88
	g = buildCS2Guide(f, info)
	for _, it := range g.Settings {
		if (it.ID == "cs2.launch.refresh" || it.ID == "cs2.refresh") && it.Status != "ok" {
			t.Errorf("120 Hz num monitor de 120 Hz: %+v", it)
		}
	}
	info.LaunchOptions = "-freq 60"
	g = buildCS2Guide(f, info)
	for _, it := range g.Settings {
		if it.ID == "cs2.launch.refresh" && (it.Status != "action" || !strings.Contains(it.Advice, "Limita o jogo a 60 Hz")) {
			t.Errorf("-freq 60 num monitor de 120 Hz: %+v", it)
		}
	}
	// Without a video file the launch option is still checked.
	info.Video = nil
	g = buildCS2Guide(f, info)
	if len(g.Settings) != 1 || g.Settings[0].ID != "cs2.launch.refresh" {
		t.Errorf("sem cs2_video.txt: %+v", g.Settings)
	}
}

// The speed-test server's Retry-After is shown and a load test before it is
// refused with the time to wait.
func TestNetRetryAfter(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(m.Platform, "test")
	if err != nil {
		t.Fatal(err)
	}
	if a.Net().RetryAt != nil {
		t.Fatal("sem recusa, sem espera")
	}
	a.noteRefusal(errors.New("outra falha"))
	a.noteRefusal(&netprobe.StatusError{Dir: "download", Code: 429, Status: "429 Too Many Requests"}) // no header: nothing to show
	if a.Net().RetryAt != nil {
		t.Fatal("sem Retry-After não há horário para mostrar")
	}
	a.noteRefusal(&netprobe.StatusError{Dir: "upload", Code: 429, Status: "429 Too Many Requests", RetryAfter: "300"})
	v := a.Net()
	if v.RetryAt == nil {
		t.Fatal("Retry-After de 300 s deveria aparecer na página")
	}
	if _, err := a.StartNetTest(NetTestRequest{Load: true}); err == nil || !strings.Contains(err.Error(), "esperar até") {
		t.Fatalf("teste com carga antes do horário: %v", err)
	}
}

// The exported driver state keeps an empty installer name empty (0.6.1
// exported "file": ".").
func TestDriverExportFileName(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(m.Platform, "test")
	if err != nil {
		t.Fatal(err)
	}
	a.updateDriverState(func(st *driverState) {
		st.GPU = &GPUCheck{Vendor: "NVIDIA", Name: "NVIDIA GeForce RTX 3050 6GB Laptop GPU", Error: "falhou"}
	})
	b, _ := json.Marshal(a.exportRaw()["drivers"])
	if strings.Contains(string(b), `"file"`) || strings.Contains(string(b), `"upToDate"`) {
		t.Errorf("sem instalador e sem versão conhecida: %s", b)
	}
	a.updateDriverState(func(st *driverState) {
		st.GPU.File = filepath.Join(a.P.DataDir, "Fulano", "downloads", "nvidia", "617.14.exe")
	})
	b, _ = json.Marshal(a.exportRaw()["drivers"])
	if !strings.Contains(string(b), `"file":"617.14.exe"`) || strings.Contains(string(b), "Fulano") {
		t.Errorf("nome do instalador sem a pasta do usuário: %s", b)
	}
}

// Review of 0.6.2: the game may run at a resolution faster than the desktop's
// (a 4K monitor on HDMI 2.0) or on a secondary monitor; neither is "above
// the monitor".
func TestCS2RefreshUsesFastestMonitor(t *testing.T) {
	f := &facts.Facts{Displays: []sys.DisplayMode{{Device: `\\.\DISPLAY1`, Monitor: "Monitor 4K", Width: 3840, Height: 2160, CurrentHz: 60, MaxHzAtRes: 60, MaxHzAny: 144, Primary: true}}}
	info := games.CS2Info{Installed: true, Video: &games.CS2Video{Width: 1920, Height: 1080, RefreshHz: 144, VSync: 0, LowLatency: 1}, LaunchOptions: "-refresh 144"}
	for _, it := range buildCS2Guide(f, info).Settings {
		if (it.ID == "cs2.refresh" || it.ID == "cs2.launch.refresh") && it.Status == "action" && strings.Contains(it.Advice, "acima") {
			t.Errorf("4K a 60 Hz na área de trabalho, 1080p a 144 Hz no jogo: %+v", it)
		}
	}
	f.Displays = []sys.DisplayMode{
		{Device: `\\.\DISPLAY1`, Monitor: "Escritório", CurrentHz: 60, MaxHzAtRes: 60, MaxHzAny: 60, Primary: true},
		{Device: `\\.\DISPLAY2`, Monitor: "Gamer 240", CurrentHz: 240, MaxHzAtRes: 240, MaxHzAny: 240},
	}
	info.Video.RefreshHz, info.LaunchOptions = 240, "-refresh 240"
	for _, it := range buildCS2Guide(f, info).Settings {
		if (it.ID == "cs2.refresh" || it.ID == "cs2.launch.refresh") && it.Status == "action" && strings.Contains(it.Advice, "acima") {
			t.Errorf("monitor de 240 Hz conectado como secundário: %+v", it)
		}
	}
}

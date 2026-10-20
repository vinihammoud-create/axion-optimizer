package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"axion/internal/apps"
	"axion/internal/profile"
)

// 0.8: the catalog of programs. The page reads what winget has installed
// (and what has a newer version), marks the items of the profile and the
// hardware, installs the marked ones one by one with the outcome of each
// (already installed, failed, installed), updates everything, and keeps
// the selection and a log.
func TestAppsPage(t *testing.T) {
	m, c := setupServer(t)
	a := c.s.App
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	c.json("POST", "/api/settings", map[string]any{"profile": profile.Competitive}, nil)
	page := func() AppsView {
		var v AppsView // a fresh struct each time: decoding into a reused slice keeps old fields
		c.json("GET", "/api/apps", nil, &v)
		return v
	}
	v := page()
	if !v.Winget.Available || v.Scanned || len(v.Items) < 60 || len(v.Categories) != len(apps.Categories) {
		t.Fatalf("página antes da leitura: winget %+v scanned %v itens %d", v.Winget, v.Scanned, len(v.Items))
	}
	rows := func(v AppsView) map[string]AppRow {
		out := map[string]AppRow{}
		for _, r := range v.Items {
			out[r.ID] = r
		}
		return out
	}
	by := rows(v)
	if by["steam"].Preset != "marcado" || by["obs"].Preset != "" || by["vcredist"].Preset != "marcado" || by["faceit"].Preset != "sugerido" {
		t.Errorf("pré-marcação do competitivo: steam %q obs %q vcredist %q faceit %q", by["steam"].Preset, by["obs"].Preset, by["vcredist"].Preset, by["faceit"].Preset)
	}
	// The mock has a Logitech receiver (VID 046D) and an NVIDIA GPU.
	if by["ghub"].Preset != "marcado" || by["nvidia"].Preset != "marcado" || v.Detected["ghub"] == "" {
		t.Errorf("hardware: ghub %q nvidia %q detectado %v", by["ghub"].Preset, by["nvidia"].Preset, v.Detected)
	}
	// Read the installed packages.
	c.json("POST", "/api/apps/refresh", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("leitura: %+v", j)
	}
	v = page()
	by = rows(v)
	if !v.Scanned || !by["steam"].Installed || by["steam"].Source != "winget" || !by["discord"].Installed || by["discord"].Available != "1.0.9230" || by["brave"].Installed || !by["7zip"].Installed {
		t.Fatalf("instalados: steam %+v discord %+v brave %v", by["steam"], by["discord"], by["brave"].Installed)
	}
	if v.Upgrades != 2 {
		t.Errorf("atualizações disponíveis: %d", v.Upgrades)
	}
	// Selection is saved and validated.
	c.json("POST", "/api/apps/select", AppsSelectRequest{IDs: []string{"brave", "steam", "x-invalido"}}, nil)
	v = page()
	if strings.Join(v.Selection, ",") != "brave,steam" || !v.HasSel {
		t.Errorf("seleção: %v", v.Selection)
	}
	// Install: Brave (new), Steam (already there), Discord (update), a manual link (ignored), one that fails.
	m.WingetFail = map[string]int{"Mozilla.Firefox": int(uint32(apps.CodeDownload))}
	c.json("POST", "/api/apps/install", AppsInstallRequest{IDs: []string{"brave", "steam", "discord", "faceit", "firefox"}}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("instalação: %+v", j)
	}
	b, _ := json.Marshal(j.Result)
	var sum AppsInstallSummary
	_ = json.Unmarshal(b, &sum)
	got := map[string]AppInstallResult{}
	for _, r := range sum.Results {
		got[r.ID] = r
	}
	if got["brave"].Action != "install" || !got["brave"].OK || got["steam"].Action != "skipped" || got["discord"].Action != "upgrade" || !got["discord"].OK || got["firefox"].OK || !strings.Contains(got["firefox"].Outcome, "download") {
		t.Errorf("resultados: %+v", got)
	}
	if _, ok := got["faceit"]; ok {
		t.Error("link manual não entra na fila")
	}
	if sum.OK != 2 || sum.Failed != 1 || sum.Skipped != 1 {
		t.Errorf("contagem: %+v", sum)
	}
	// winget got the documented arguments.
	var seen []string
	for _, call := range m.X.Calls {
		if strings.HasSuffix(strings.ToLower(call.Exe), "winget.exe") {
			seen = append(seen, strings.Join(call.Args, " "))
		}
	}
	joined := strings.Join(seen, "\n")
	for _, want := range []string{"install --id Brave.Brave -e --silent --accept-package-agreements --accept-source-agreements --disable-interactivity", "upgrade --id Discord.Discord -e --silent", "export -o "} {
		if !strings.Contains(joined, want) {
			t.Errorf("argumento ausente %q em:\n%s", want, joined)
		}
	}
	// The page shows the new state and the log.
	v = page()
	by = rows(v)
	if !by["brave"].Installed || by["discord"].Available != "" || len(v.Log) != 3 || v.Log[0].ID != "firefox" || v.Log[0].OK {
		t.Errorf("depois: brave %v discord %q log %+v", by["brave"].Installed, by["discord"].Available, v.Log)
	}
	// Upgrade all.
	c.json("POST", "/api/apps/upgrade", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("atualizar tudo: %+v", j)
	}
	v = page()
	if v.Upgrades != 0 || rows(v)["chrome"].Available != "" {
		t.Errorf("depois de atualizar tudo: %d %+v", v.Upgrades, rows(v)["chrome"])
	}
	// Invalid ids and empty queues are refused.
	if code, _ := c.do("POST", "/api/apps/install", AppsInstallRequest{IDs: []string{"nope"}}, nil); code != 400 {
		t.Errorf("id inválido: %d", code)
	}
	if code, _ := c.do("POST", "/api/apps/install", AppsInstallRequest{IDs: []string{"faceit"}}, nil); code != 400 {
		t.Errorf("só links manuais: %d", code)
	}
	// Elevated with another account: installs and "update all" are refused.
	f := a.Facts()
	f.OS.ElevatedAs = `PC\Outro`
	if code, _ := c.do("POST", "/api/apps/install", AppsInstallRequest{IDs: []string{"brave"}}, nil); code != 400 {
		t.Errorf("outra conta: %d", code)
	}
	if code, _ := c.do("POST", "/api/apps/upgrade", map[string]any{}, nil); code != 400 {
		t.Errorf("outra conta, atualizar tudo: %d", code)
	}
	f.OS.ElevatedAs = ""
	if v = page(); v.Warning != "" {
		t.Errorf("aviso indevido: %s", v.Warning)
	}
	// The saved kit is in both exports, so it survives a format.
	var raw map[string]any
	c.json("GET", "/api/export", nil, &raw)
	if ap, _ := raw["apps"].(map[string]any); ap == nil || len(ap["selection"].([]any)) != 2 {
		t.Errorf("kit no JSON: %v", raw["apps"])
	}
	html, err := a.ExportHTML(false)
	if err != nil || !strings.Contains(string(html), "Kit de programas") || !strings.Contains(string(html), "Brave.Brave") {
		t.Errorf("kit no HTML: %v", err)
	}
}

// Cancelling an install queue lets the running installer finish and stops
// before the next item, with the results so far; "update all" reports a
// partial failure as such.
func TestAppsCancelAndPartial(t *testing.T) {
	m, c := setupServer(t)
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	m.WingetDelay = 400 * time.Millisecond // set before any winget call runs
	c.json("POST", "/api/apps/install", AppsInstallRequest{IDs: []string{"brave", "firefox", "vivaldi"}}, &j)
	time.Sleep(150 * time.Millisecond) // Brave's installer is running
	if cur, _ := c.s.App.Jobs.Get(j.ID); cur.NoCancel != "" {
		t.Errorf("a fila deve aceitar cancelar enquanto um instalador roda: %q", cur.NoCancel)
	}
	if code, _ := c.do("POST", "/api/jobs/"+j.ID+"/cancel", map[string]any{}, nil); code != 200 {
		t.Fatalf("cancelar: %d", code)
	}
	if j = c.wait(j.ID); j.Status != JobCancelled {
		t.Fatalf("estado: %+v", j)
	}
	b, _ := json.Marshal(j.Result)
	var sum AppsInstallSummary
	_ = json.Unmarshal(b, &sum)
	if len(sum.Results) != 1 || sum.Results[0].ID != "brave" || !sum.Results[0].OK {
		t.Errorf("o item em andamento termina e os seguintes não começam: %+v", sum.Results)
	}
	var v AppsView
	c.json("GET", "/api/apps", nil, &v)
	by := map[string]AppRow{}
	for _, r := range v.Items {
		by[r.ID] = r
	}
	if !by["brave"].Installed || by["firefox"].Installed || len(v.Log) != 1 || !v.Log[0].OK {
		t.Errorf("depois do cancelamento: brave %v firefox %v log %+v", by["brave"].Installed, by["firefox"].Installed, v.Log)
	}
	// Update all with one failing package.
	m.WingetDelay = 0
	m.WingetFail = map[string]int{"Google.Chrome": int(uint32(apps.CodeShellExecute))}
	c.json("POST", "/api/apps/upgrade", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("atualizar tudo: %+v", j)
	}
	b, _ = json.Marshal(j.Result)
	sum = AppsInstallSummary{}
	_ = json.Unmarshal(b, &sum)
	if len(sum.Results) != 1 || !sum.Results[0].Partial || sum.Results[0].OK || !strings.Contains(sum.Results[0].Outcome, "parte das atualizações") {
		t.Errorf("parcial: %+v", sum.Results)
	}
	c.json("GET", "/api/apps", nil, &v)
	if v.Upgrades != 1 { // Chrome is still pending; Discord went
		t.Errorf("restou %d atualização", v.Upgrades)
	}
}

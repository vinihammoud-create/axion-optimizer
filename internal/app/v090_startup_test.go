package app

import (
	"net/http"
	"testing"

	"axion/internal/catalog"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

const (
	approvedRunUser = `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
	runUser         = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
)

var epicDisabled = []byte{3, 0, 0, 0, 0x10, 0x6c, 0x3e, 0x9b, 0x2c, 0x1d, 0xdb, 0x01}

func setStartup(t *testing.T, c *client, enabled bool, ids ...string) StartupView {
	t.Helper()
	var v StartupView
	c.json("POST", "/api/startup/set", StartupRequest{IDs: ids, Enabled: enabled}, &v)
	return v
}

func startupEnabled(v StartupView, id string) (enabled, found bool) {
	for _, it := range v.Items {
		if it.ID == id {
			return it.Enabled, true
		}
	}
	return false, false
}

// Turning an item on that Axion turned off undoes the entry: the original
// value (here: none) comes back and no "startup:on:" entry is created.
func TestStartupUndoExact(t *testing.T) {
	m, c := setupServer(t)
	a := c.s.App
	const id = "run-user|Discord"
	v := setStartup(t, c, false, id)
	if en, ok := startupEnabled(v, id); !ok || en {
		t.Fatalf("Discord deveria estar desligado: %+v", v)
	}
	ap, _ := m.R.GetValue(approvedRunUser, "Discord")
	if !ap.Exists || len(ap.Bin) != 12 || ap.Bin[0] != 3 {
		t.Fatalf("StartupApproved: %+v", ap)
	}
	var ft uint64
	for i := 11; i >= 4; i-- {
		ft = ft<<8 | uint64(ap.Bin[i])
	}
	if ft == 0 {
		t.Error("o FILETIME deveria estar preenchido")
	}
	if r, _ := m.R.GetValue(runUser, "Discord"); !r.Exists {
		t.Fatal("a entrada do Run foi apagada")
	}
	ent := a.Engine.J.ActiveFor(catalog.StartupID(id, false))
	if ent == nil || ent.TweakTitle != "Inicialização: desativar Discord" || ent.BatchLabel != "Inicialização" {
		t.Fatalf("entrada: %+v", ent)
	}
	for _, it := range v.Items {
		if it.ID == id && !it.ByAxion {
			t.Error("o item deveria vir marcado como alterado pelo Axion")
		}
	}

	v = setStartup(t, c, true, id)
	if en, _ := startupEnabled(v, id); !en {
		t.Fatalf("Discord deveria estar ligado de novo")
	}
	if ap, _ := m.R.GetValue(approvedRunUser, "Discord"); ap.Exists {
		t.Errorf("o valor não existia antes: deveria sumir: %+v", ap)
	}
	ent = a.Engine.J.ActiveFor(catalog.StartupID(id, false))
	if ent != nil {
		t.Errorf("a entrada deveria estar desfeita: %+v", ent)
	}
	if a.Engine.J.ActiveFor(catalog.StartupID(id, true)) != nil {
		t.Error("nenhuma entrada startup:on: deveria ser criada")
	}
	found := false
	for _, en := range a.Engine.J.Entries() {
		if en.TweakID == catalog.StartupID(id, false) {
			found = true
			if en.Status != tweaks.StatusReverted {
				t.Errorf("status da entrada: %s", en.Status)
			}
		}
		if en.TweakID == catalog.StartupID(id, true) {
			t.Errorf("entrada indevida: %+v", en)
		}
	}
	if !found {
		t.Error("a entrada desfeita deveria continuar no histórico")
	}
}

// Axion turns an item off, Task Manager turns it on, the user asks Axion to
// turn it on: no error, and the entry is closed (Task Manager's value is
// replaced by the one from before Axion).
func TestStartupEnableAfterTaskManager(t *testing.T) {
	m, c := setupServer(t)
	a := c.s.App
	const id = "run-user|Discord"
	setStartup(t, c, false, id)
	_ = m.R.SetValue(approvedRunUser, "Discord", sys.Binary([]byte{3, 0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1})) // Task Manager touched it
	v := setStartup(t, c, true, id)
	if ap, _ := m.R.GetValue(approvedRunUser, "Discord"); ap.Exists {
		t.Errorf("o valor deveria voltar a não existir: %+v", ap)
	}
	if en, _ := startupEnabled(v, id); !en {
		t.Error("o item deveria aparecer ligado")
	}
	if a.Engine.J.ActiveFor(catalog.StartupID(id, false)) != nil {
		t.Error("a entrada deveria ficar desfeita")
	}
	// Task Manager turned it back on by itself: the entry is closed too.
	setStartup(t, c, false, id)
	_ = m.R.SetValue(approvedRunUser, "Discord", sys.Binary([]byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}))
	setStartup(t, c, true, id)
	if a.Engine.J.ActiveFor(catalog.StartupID(id, false)) != nil {
		t.Error("ligado fora do Axion: a entrada antiga deveria ser fechada")
	}
}

// Turning on an item that was disabled outside Axion creates the entry
// "startup:on:", and undoing it brings back the original bytes.
func TestStartupEnableForeignDisabled(t *testing.T) {
	m, c := setupServer(t)
	a := c.s.App
	const id = "run-user|EpicGamesLauncher"
	setStartup(t, c, true, id)
	on := catalog.StartupID(id, true)
	ent := a.Engine.J.ActiveFor(on)
	if ent == nil {
		t.Fatal("a entrada startup:on: deveria existir")
	}
	ap, _ := m.R.GetValue(approvedRunUser, "EpicGamesLauncher")
	if !ap.Exists || ap.Bin[0] != 2 || len(ap.Bin) != 12 {
		t.Fatalf("valor depois de ligar: %+v", ap)
	}
	var j Job
	c.json("POST", "/api/revert", RevertRequest{IDs: []string{on}}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("desfazer: %+v", j)
	}
	ap, _ = m.R.GetValue(approvedRunUser, "EpicGamesLauncher")
	if !ap.Exists || string(ap.Bin) != string(epicDisabled) {
		t.Fatalf("os bytes originais deveriam voltar: %+v", ap)
	}
	// By the entry ID (what the Histórico sends) works the same.
	setStartup(t, c, true, id)
	ent = a.Engine.J.ActiveFor(on)
	c.json("POST", "/api/revert", RevertRequest{Entries: []string{ent.ID}}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("desfazer pela entrada: %+v", j)
	}
	if ap, _ = m.R.GetValue(approvedRunUser, "EpicGamesLauncher"); string(ap.Bin) != string(epicDisabled) {
		t.Fatalf("bytes: %+v", ap)
	}
}

// A task of logon goes through the same engine and the undo re-enables it.
func TestStartupTaskThroughEngine(t *testing.T) {
	m, c := setupServer(t)
	a := c.s.App
	const id = `task|\GoogleUpdateTaskMachineUA`
	setStartup(t, c, false, id)
	if tk, _ := m.T.Get(`\GoogleUpdateTaskMachineUA`); tk.Enabled {
		t.Fatal("a tarefa deveria estar desligada")
	}
	if a.Engine.J.ActiveFor(catalog.StartupID(id, false)) == nil {
		t.Fatal("a entrada deveria existir no histórico")
	}
	setStartup(t, c, true, id)
	if tk, _ := m.T.Get(`\GoogleUpdateTaskMachineUA`); !tk.Enabled {
		t.Fatal("a tarefa deveria ter voltado")
	}
	if a.Engine.J.ActiveFor(catalog.StartupID(id, false)) != nil {
		t.Error("a entrada deveria estar desfeita")
	}
}

// An ID made up through /api/apply is refused: the page checks the entry exists.
func TestStartupApplyViaApiRefused(t *testing.T) {
	m, c := setupServer(t)
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	code, _ := c.do("POST", "/api/apply", ApplyRequest{IDs: []string{"startup:off:run-user|Qualquer"}, Label: "x"}, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("esperava 400, obtido %d", code)
	}
	if v, _ := m.R.GetValue(approvedRunUser, "Qualquer"); v.Exists {
		t.Errorf("valor órfão gravado: %+v", v)
	}
}

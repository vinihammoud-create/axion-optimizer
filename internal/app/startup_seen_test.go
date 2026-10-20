package app

import (
	"os"
	"testing"

	"axion/internal/startup"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

const runKeyUser = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

func newStartupApp(t *testing.T, m *mocksys.Mock) *App {
	t.Helper()
	a, err := New(m.Platform, "test")
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func newNames(v StartupView) []string {
	var out []string
	for _, it := range v.Items {
		if it.New {
			out = append(out, it.Name)
		}
	}
	return out
}

func TestStartupNewSinceLastVisit(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// First visit ever: baseline, nothing is new.
	a := newStartupApp(t, m)
	if v := a.Startup(); v.NewCount != 0 || len(newNames(v)) != 0 {
		t.Fatalf("primeira visita não marca nada: %v", newNames(v))
	}
	if _, err := os.Stat(a.startupSeenPath()); err != nil {
		t.Fatalf("a linha de base deveria ter sido gravada: %v", err)
	}

	// Another opening with the same list: still nothing.
	a = newStartupApp(t, m)
	if v := a.Startup(); v.NewCount != 0 {
		t.Fatalf("lista igual: %v", newNames(v))
	}

	// A program adds itself while Axion is closed.
	if err := m.R.SetValue(runKeyUser, "NovoLauncher", sys.SZ(`C:\Novo\launcher.exe`)); err != nil {
		t.Fatal(err)
	}
	a = newStartupApp(t, m)
	v := a.Startup()
	if got := newNames(v); len(got) != 1 || got[0] != "NovoLauncher" || v.NewCount != 1 {
		t.Fatalf("entrada nova deveria aparecer marcada: %v (contagem %d)", got, v.NewCount)
	}
	// Same run, asking again (toggling a switch refreshes the list): still marked.
	if v = a.Startup(); v.NewCount != 1 {
		t.Errorf("a marca deve durar enquanto o programa está aberto: %d", v.NewCount)
	}
	// Nothing was blocked or turned off by the notice.
	for _, it := range v.Items {
		if it.Name == "NovoLauncher" && !it.Enabled {
			t.Error("o selo não pode desativar a entrada")
		}
	}

	// Next opening: it is part of the baseline, so it shows only once.
	a = newStartupApp(t, m)
	if v = a.Startup(); v.NewCount != 0 {
		t.Errorf("entrada nova só aparece uma vez: %v", newNames(v))
	}

	// Removed entry disappears from the list and from the notice.
	if err := m.R.DeleteValue(runKeyUser, "NovoLauncher"); err != nil {
		t.Fatal(err)
	}
	a = newStartupApp(t, m)
	v = a.Startup()
	for _, it := range v.Items {
		if it.Name == "NovoLauncher" {
			t.Error("entrada removida deveria sumir")
		}
	}
	if v.NewCount != 0 {
		t.Errorf("remoção não é novidade: %v", newNames(v))
	}
	// If it comes back later, it is new again.
	if err := m.R.SetValue(runKeyUser, "NovoLauncher", sys.SZ(`C:\Novo\launcher.exe`)); err != nil {
		t.Fatal(err)
	}
	a = newStartupApp(t, m)
	if v = a.Startup(); v.NewCount != 1 {
		t.Errorf("entrada que volta conta como nova: %d", v.NewCount)
	}
}

func TestStartupNewAppearsDuringTheRun(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := newStartupApp(t, m)
	a.Startup()
	if err := m.R.SetValue(runKeyUser, "Instalado", sys.SZ(`C:\X\i.exe`)); err != nil {
		t.Fatal(err)
	}
	if v := a.Startup(); v.NewCount != 1 {
		t.Errorf("entrada que surge com o programa aberto: %d", v.NewCount)
	}
}

func TestStartupSeenFileDamaged(t *testing.T) {
	for name, content := range map[string]string{
		"lixo":      "\x00\x01 não é json {{{",
		"vazio":     "",
		"versao":    `{"version": 99, "ids": ["run-user|discord"]}`,
		"tipo":      `{"version": 1, "ids": 7}`,
		"truncado":  `{"version": 1, "ids": ["run-user|disc`,
		"so-espaco": "   \n",
	} {
		t.Run(name, func(t *testing.T) {
			m, err := mocksys.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			a := newStartupApp(t, m)
			if err := os.WriteFile(a.startupSeenPath(), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			v := a.Startup()
			if len(v.Items) == 0 {
				t.Fatal("a página não pode quebrar")
			}
			if v.NewCount != 0 {
				t.Errorf("arquivo ilegível vira linha de base nova, sem marcar nada: %v", newNames(v))
			}
			b, err := os.ReadFile(a.startupSeenPath())
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := startup.ParseSeen(b); !ok {
				t.Errorf("o arquivo deveria ter sido refeito: %q", b)
			}
		})
	}
}

func TestStartupSeenDirMissing(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := newStartupApp(t, m)
	// A folder where the file should be: the save fails, the page still works.
	if err := os.MkdirAll(a.startupSeenPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if v := a.Startup(); len(v.Items) == 0 || v.NewCount != 0 {
		t.Errorf("falha ao gravar não pode derrubar a página: %+v", v.NewCount)
	}
}

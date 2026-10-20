package catalog_test

import (
	"context"
	"strings"
	"testing"

	"axion/internal/catalog"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

const duckKey = `HKCU\Software\Microsoft\Multimedia\Audio`

// "Chamadas não abaixam o volume do jogo": the value of the Communications tab
// ("Não fazer nada" = 3). Absent means the Windows default (reduce by 80%).
func TestDuckingTweak(t *testing.T) {
	m, f, e := setup(t)
	var tw *tweaks.Tweak
	for _, x := range e.Catalog.All() {
		if x.ID == "game.ducking" {
			tw = x
		}
	}
	if tw == nil {
		t.Fatal("o ajuste game.ducking não existe")
	}
	// Optional, never part of the one-click plan: it is a use preference.
	if tw.OneClick || tw.Evidence != tweaks.EvLow || tw.Reboot != tweaks.RebootSignOut || tw.Risk != tweaks.RiskSafe || len(tw.Sources) != 3 {
		t.Errorf("metadados: %+v", tw)
	}
	for _, want := range []string{"Limite conhecido", "trocar a opção e voltar", "80%", "a Microsoft não documenta o valor", "não foi medido"} {
		if !strings.Contains(tw.Detail, want) {
			t.Errorf("o texto deveria dizer %q", want)
		}
	}
	if st := e.StateOf(context.Background(), f, tw); st.Status != "off" || !st.Applicable {
		t.Fatalf("sem o valor no registro não está aplicado: %+v", st)
	}
	if r := e.Apply(context.Background(), f, []string{"game.ducking"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	if v, _ := m.R.GetValue(duckKey, "UserDuckingPreference"); !v.Exists || v.Kind != sys.RegDWord || v.Int != 3 {
		t.Fatalf("valor escrito: %+v", v)
	}
	if st := e.StateOf(context.Background(), f, tw); st.Status != "on" || !st.ByAxion {
		t.Errorf("depois de aplicar: %+v", st)
	}
	// Undone: the value was absent before, so it is absent again.
	if r := e.RevertTweaks(context.Background(), f, []string{"game.ducking"}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer: %+v", r[0])
	}
	if v, _ := m.R.GetValue(duckKey, "UserDuckingPreference"); v.Exists {
		t.Errorf("o valor deveria ter voltado a não existir: %+v", v)
	}
}

// A PC that already has 3 (this notebook does) is "already applied", and
// undoing never changes what the user had chosen before Axion.
func TestDuckingAlreadyChosenByTheUser(t *testing.T) {
	m, f, e := setup(t)
	_ = m.R.SetValue(duckKey, "UserDuckingPreference", sys.DWord(3))
	if r := e.Apply(context.Background(), f, []string{"game.ducking"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutAlready {
		t.Fatalf("já em 3: %+v", r[0])
	}
	// Another option (reduce by 50%) is changed to 3 and restored on undo.
	_ = m.R.SetValue(duckKey, "UserDuckingPreference", sys.DWord(2))
	if r := e.Apply(context.Background(), f, []string{"game.ducking"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("de 2 para 3: %+v", r[0])
	}
	if r := e.RevertTweaks(context.Background(), f, []string{"game.ducking"}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer: %+v", r[0])
	}
	if v, _ := m.R.GetValue(duckKey, "UserDuckingPreference"); v.Int != 2 {
		t.Errorf("a escolha anterior (2) deveria voltar: %+v", v)
	}
}

// Startup items (0.9): each change goes through the engine and is undone
// byte for byte, including the keys Axion created.
func TestStartupProviderRoundTrip(t *testing.T) {
	for _, id := range []string{
		"startup:off:run-user|Discord", // value absent before
		"startup:off:run-machine32|Adobe ARM",
		"startup:off:folder-user|Spotify.lnk", // the StartupFolder key may be created and pruned
		`startup:off:task|\GoogleUpdateTaskMachineUA`,
		"startup:on:run-user|EpicGamesLauncher", // seeded as disabled (03 + FILETIME)
	} {
		id := id
		t.Run(id, func(t *testing.T) {
			m, f, e := setup(t)
			e.Providers = append(e.Providers, catalog.StartupProvider)
			tw := e.Resolve(id, f)
			if tw == nil {
				t.Fatalf("o provedor não montou %s", id)
			}
			if tw.OneClick || tw.Risk != tweaks.RiskSafe || tw.Category != "inicializacao" {
				t.Errorf("metadados: %+v", tw)
			}
			roundTrip(t, m, f, e, tw)
		})
	}
}

func TestStartupProviderBytes(t *testing.T) {
	const approved = `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
	m, f, e := setup(t)
	e.Providers = append(e.Providers, catalog.StartupProvider)
	if r := e.Apply(context.Background(), f, []string{"startup:off:run-user|Discord"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	v, _ := m.R.GetValue(approved, "Discord")
	if !v.Exists || v.Kind != sys.RegBinary || len(v.Bin) != 12 || v.Bin[0] != 3 {
		t.Fatalf("StartupApproved: %+v", v)
	}
	zero := true
	for _, b := range v.Bin[4:] {
		if b != 0 {
			zero = false
		}
	}
	if zero {
		t.Error("o FILETIME deveria estar preenchido")
	}
	if r, _ := m.R.GetValue(`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "Discord"); !r.Exists {
		t.Error("a entrada do Run nunca é apagada")
	}
	// Turning on: 02 and eleven zeros.
	if r := e.Apply(context.Background(), f, []string{"startup:on:run-user|EpicGamesLauncher"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("ligar: %+v", r[0])
	}
	v, _ = m.R.GetValue(approved, "EpicGamesLauncher")
	if string(v.Bin) != string([]byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatalf("ligado: %v", v.Bin)
	}
	e.RevertAll(context.Background(), f, tweaks.RevertOpts{})
	v, _ = m.R.GetValue(approved, "EpicGamesLauncher")
	if string(v.Bin) != string([]byte{3, 0, 0, 0, 0x10, 0x6c, 0x3e, 0x9b, 0x2c, 0x1d, 0xdb, 0x01}) {
		t.Fatalf("depois do desfazer: %v", v.Bin)
	}
}

// Task Manager writes another time stamp: the item is still off (no drift).
// A real change (turned on) is drift.
func TestStartupDriftIgnoresTimestamp(t *testing.T) {
	const approved = `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
	m, f, e := setup(t)
	e.Providers = append(e.Providers, catalog.StartupProvider)
	const id = "startup:off:run-user|Discord"
	ctx := context.Background()
	if r := e.Apply(ctx, f, []string{id}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	drifts := func() bool {
		for _, st := range e.Drift(ctx, f) {
			if st.ID == id {
				return true
			}
		}
		return false
	}
	_ = m.R.SetValue(approved, "Discord", sys.Binary([]byte{3, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}))
	if drifts() {
		t.Error("outra data com o mesmo estado não é deriva")
	}
	_ = m.R.SetValue(approved, "Discord", sys.Binary([]byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}))
	if !drifts() {
		t.Error("ligar o item de novo deveria aparecer como deriva")
	}
}

func TestParseStartupID(t *testing.T) {
	item, on, ok := catalog.ParseStartupID(`startup:on:task|\Foo\Bar`)
	if !ok || !on || item != `task|\Foo\Bar` {
		t.Errorf("válido recusado: %q %v %v", item, on, ok)
	}
	if catalog.StartupID("run-user|X", false) != "startup:off:run-user|X" {
		t.Error("formato do ID")
	}
	for _, bad := range []string{"startup:", "startup:off:", "startup:maybe:x", "startup:off", "profile:a:b", ""} {
		if _, _, ok := catalog.ParseStartupID(bad); ok {
			t.Errorf("%q deveria ser recusado", bad)
		}
	}
	// A well-formed ID for an entry kind that does not exist builds nothing.
	if catalog.StartupProvider("startup:off:xyz|a", nil) != nil || catalog.StartupProvider("startup:off:lixo", nil) != nil {
		t.Error("origem desconhecida não pode virar item")
	}
}

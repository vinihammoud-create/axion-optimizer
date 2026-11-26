package catalog

import (
	"context"
	"encoding/json"
	"testing"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

const (
	testPrefKey  = `HKCU\Software\AxionTeste\Preferencias`
	testLayerKey = `HKCU\Software\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Layers`
)

func keyedEngine(t *testing.T, tws ...*tweaks.Tweak) (*mocksys.Mock, *tweaks.Engine) {
	t.Helper()
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cat, err := tweaks.NewCatalog(tws)
	if err != nil {
		t.Fatal(err)
	}
	j, err := tweaks.OpenJournal(m.DataDir, m.Key)
	if err != nil {
		t.Fatal(err)
	}
	return m, tweaks.NewEngine(m.Platform, cat, j)
}

func keyedTweak(id string, op tweaks.Op) *tweaks.Tweak {
	return &tweaks.Tweak{ID: id, Category: "jogo", Title: id, Evidence: tweaks.EvHigh, Risk: tweaks.RiskSafe,
		Ops: func(*facts.Facts) []tweaks.Op { return []tweaks.Op{op} }}
}

func keyExists(t *testing.T, m *mocksys.Mock, key string) bool {
	t.Helper()
	ok, err := m.R.KeyExists(key)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

// The game-profile operations write one option inside a registry value. When
// the value's key was not on the machine, the undo removes it again; with two
// profiles sharing the key, whichever undo empties it removes it, in any
// order.
func TestProfileOpsRemoveTheKeyTheyCreated(t *testing.T) {
	ctx, f := context.Background(), &facts.Facts{}
	for _, order := range [][]string{{"t.a", "t.b"}, {"t.b", "t.a"}} {
		m, e := keyedEngine(t,
			keyedTweak("t.a", dxPrefOp(testPrefKey, "jogo-a.exe", "GpuPreference", "2")),
			keyedTweak("t.b", dxPrefOp(testPrefKey, "jogo-b.exe", "GpuPreference", "2")))
		if keyExists(t, m, testPrefKey) {
			t.Fatal("a chave de teste não deveria existir no PC simulado")
		}
		for _, r := range e.Apply(ctx, f, []string{"t.a", "t.b"}, tweaks.ApplyOpts{}) {
			if r.Outcome != tweaks.OutApplied {
				t.Fatal(r)
			}
		}
		if r := e.RevertTweaks(ctx, f, order[:1], tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
			t.Fatal(r[0])
		}
		if !keyExists(t, m, testPrefKey) {
			t.Fatalf("ordem %v: a chave ainda guarda o valor do outro perfil e não pode sumir", order)
		}
		if r := e.RevertTweaks(ctx, f, order[1:], tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
			t.Fatal(r[0])
		}
		if keyExists(t, m, testPrefKey) {
			t.Errorf("ordem %v: a chave criada pelo Axion sobrou vazia depois de desfazer os dois perfis", order)
		}
	}

	// The compatibility layer of an executable (full-screen optimizations).
	m, e := keyedEngine(t, keyedTweak("t.layer", layerFlagOp(`D:\Jogos\Jogo\jogo.exe`, "DISABLEDXMAXIMIZEDWINDOWEDMODE")))
	hadLayers := keyExists(t, m, testLayerKey)
	if r := e.Apply(ctx, f, []string{"t.layer"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0])
	}
	if r := e.RevertTweaks(ctx, f, []string{"t.layer"}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatal(r[0])
	}
	if got := keyExists(t, m, testLayerKey); got != hadLayers {
		t.Errorf("chave Layers: existia antes = %v, depois do desfazer = %v", hadLayers, got)
	}
}

// A key that was already there, even empty, is the user's: it stays.
func TestProfileOpsKeepAKeyThatWasThere(t *testing.T) {
	ctx, f := context.Background(), &facts.Facts{}
	m, e := keyedEngine(t, keyedTweak("t.a", dxPrefOp(testPrefKey, "jogo-a.exe", "GpuPreference", "2")))
	if _, err := m.R.CreateKey(testPrefKey); err != nil {
		t.Fatal(err)
	}
	if r := e.Apply(ctx, f, []string{"t.a"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0])
	}
	if r := e.RevertTweaks(ctx, f, []string{"t.a"}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatal(r[0])
	}
	if !keyExists(t, m, testPrefKey) {
		t.Error("a chave já existia antes do ajuste e foi apagada pelo desfazer")
	}
	if v, _ := m.R.GetValue(testPrefKey, "jogo-a.exe"); v.Exists {
		t.Errorf("o valor criado pelo ajuste sobrou: %+v", v)
	}
}

// A record written before the snapshot named the key is the bare value: it
// still decodes, and names no key to remove.
func TestKeyedValueReadsOldSnapshots(t *testing.T) {
	old, err := json.Marshal(sys.SZ("~ DISABLEDXMAXIMIZEDWINDOWEDMODE"))
	if err != nil {
		t.Fatal(err)
	}
	var s keyedValue
	if err := json.Unmarshal(old, &s); err != nil {
		t.Fatal(err)
	}
	if !s.Exists || s.Str != "~ DISABLEDXMAXIMIZEDWINDOWEDMODE" || s.CreatedKey != "" {
		t.Errorf("registro antigo lido errado: %+v", s)
	}
	// And the new one keeps the value's own fields at the top level, where
	// the comparison functions (which read a bare value) look for them.
	now, _ := json.Marshal(keyedValue{RegValue: sys.SZ("x"), CreatedKey: testPrefKey})
	var bare sys.RegValue
	if err := json.Unmarshal(now, &bare); err != nil || !bare.Exists || bare.Str != "x" {
		t.Errorf("registro novo não é lido como valor: %s %+v %v", now, bare, err)
	}
}

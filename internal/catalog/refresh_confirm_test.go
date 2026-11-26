package catalog_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

// keepAnswer is the person at the screen answering "Manter" at once.
func keepAnswer(string, time.Duration) bool { return true }

const mockDisplay = `\\.\DISPLAY1`

func displayNow(t *testing.T, m *mocksys.Mock) sys.DisplayMode {
	t.Helper()
	list, err := m.N.DisplayModes()
	if err != nil || len(list) != 1 {
		t.Fatalf("monitores do PC simulado: %+v %v", list, err)
	}
	return list[0]
}

// "Manter": the new rate is tried without saving, the question comes with
// the 15 s limit, and only the answer saves it. Undo goes back and saves
// the previous rate.
func TestRefreshKeepSavesAfterConfirm(t *testing.T) {
	m, f, e := setup(t)
	var asked []string
	var limit time.Duration
	confirm := func(prompt string, timeout time.Duration) bool {
		asked, limit = append(asked, prompt), timeout
		if d := displayNow(t, m); d.CurrentHz != 240 {
			t.Errorf("durante a pergunta o monitor deveria estar a 240 Hz: %d", d.CurrentHz)
		}
		if m.N.DisplayWrites != 0 {
			t.Errorf("nada deveria estar gravado antes de Manter: %d gravações", m.N.DisplayWrites)
		}
		return true
	}
	res := e.Apply(context.Background(), f, []string{"game.refresh"}, tweaks.ApplyOpts{Confirm: confirm})
	if res[0].Outcome != tweaks.OutApplied {
		t.Fatalf("Manter deveria aplicar: %s %s", res[0].Outcome, res[0].Message)
	}
	if len(asked) != 1 || !strings.Contains(asked[0], "240 Hz") || limit != 15*time.Second {
		t.Fatalf("pergunta = %q, prazo = %v", asked, limit)
	}
	if saved, ok := m.N.SavedDisplay(mockDisplay); !ok || saved.CurrentHz != 240 || m.N.DisplayWrites != 1 {
		t.Fatalf("depois de Manter o modo deveria ser gravado uma vez: %+v %v (%d)", saved, ok, m.N.DisplayWrites)
	}
	if e.ActiveFor("game.refresh") == nil {
		t.Fatal("o histórico deveria ter o ajuste aplicado")
	}
	rev := e.RevertTweaks(context.Background(), f, []string{"game.refresh"}, tweaks.RevertOpts{})
	if rev[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer: %+v", rev[0])
	}
	if saved, _ := m.N.SavedDisplay(mockDisplay); displayNow(t, m).CurrentHz != 60 || saved.CurrentHz != 60 {
		t.Fatalf("desfazer deveria voltar e gravar 60 Hz: agora %d, gravado %d", displayNow(t, m).CurrentHz, saved.CurrentHz)
	}
}

// No "Manter" in time, or "Reverter" (the hook answers false either way):
// the monitor goes back by itself, nothing is saved and the history holds
// no applied tweak.
func TestRefreshNotConfirmedGoesBackWithoutSaving(t *testing.T) {
	m, f, e := setup(t)
	before := m.World()
	res := e.Apply(context.Background(), f, []string{"game.refresh"}, tweaks.ApplyOpts{Confirm: func(string, time.Duration) bool { return false }})
	if res[0].Outcome != tweaks.OutFailed || !strings.Contains(res[0].Message, "não foi confirmada") {
		t.Fatalf("sem confirmação deveria falhar: %s %s", res[0].Outcome, res[0].Message)
	}
	if d := displayNow(t, m); d.CurrentHz != 60 {
		t.Fatalf("o monitor deveria voltar a 60 Hz: %d", d.CurrentHz)
	}
	if m.N.DisplayWrites != 0 {
		t.Fatalf("nada deveria ser gravado no registro: %d gravações (%+v)", m.N.DisplayWrites, m.N.DisplaySaved)
	}
	if e.ActiveFor("game.refresh") != nil {
		t.Fatal("o histórico não deveria ficar com o ajuste aplicado")
	}
	if st := e.StateOf(context.Background(), f, e.Catalog.Get("game.refresh")); st.Status != "off" || st.ByAxion || st.PendingUndo {
		t.Fatalf("estado depois da volta: %s byAxion=%v pendente=%v", st.Status, st.ByAxion, st.PendingUndo)
	}
	if d := mocksys.DiffWorld(before, m.World(), "hklm\\software\\axion"); len(d) > 0 {
		t.Fatalf("o PC deveria estar como antes:\n  %s", strings.Join(d, "\n  "))
	}
}

// Nobody to ask (no interface: tests, collection, a call without the hook):
// the rate is not even tried and nothing is saved.
func TestRefreshWithoutConfirmHookChangesNothing(t *testing.T) {
	m, f, e := setup(t)
	res := e.Apply(context.Background(), f, []string{"game.refresh"}, tweaks.ApplyOpts{})
	if res[0].Outcome != tweaks.OutFailed {
		t.Fatalf("sem quem confirme deveria falhar: %s %s", res[0].Outcome, res[0].Message)
	}
	if d := displayNow(t, m); d.CurrentHz != 60 || m.N.DisplayWrites != 0 {
		t.Fatalf("nada deveria mudar: %d Hz, %d gravações", d.CurrentHz, m.N.DisplayWrites)
	}
	if e.ActiveFor("game.refresh") != nil {
		t.Fatal("o histórico não deveria ficar com o ajuste aplicado")
	}
}

// CATALOGO-3 F11: a monitor that is not connected now has no rate to read;
// it is not "already at the maximum".
func TestRefreshDisconnectedMonitorIsNotInTarget(t *testing.T) {
	m, f, e := setup(t)
	m.N.Displays = nil
	st := e.StateOf(context.Background(), f, e.Catalog.Get("game.refresh"))
	if st.Status == "on" {
		t.Fatalf("monitor desconectado não pode contar como aplicado: %+v", st.Ops)
	}
	res := e.Apply(context.Background(), f, []string{"game.refresh"}, tweaks.ApplyOpts{Confirm: keepAnswer})
	if res[0].Outcome == tweaks.OutAlready || res[0].Outcome == tweaks.OutApplied {
		t.Fatalf("aplicar sobre monitor desconectado: %s %s", res[0].Outcome, res[0].Message)
	}
}

// CATALOGO-3 F10: Win32_OptionalFeature without an InstallState (or with 4,
// "Unknown") says nothing; it is not "already disabled".
func TestFeatureUnknownStateIsNotInTarget(t *testing.T) {
	for _, row := range []sys.WMIRow{
		{"Name": "VirtualMachinePlatform"},
		{"Name": "VirtualMachinePlatform", "InstallState": int64(0)},
		{"Name": "VirtualMachinePlatform", "InstallState": int64(4)},
	} {
		m, f, e := setup(t)
		m.W.Set(`root\cimv2`, "Win32_OptionalFeature", row)
		f.Security.VMPEnabled = true
		tw := e.Catalog.Get("adv.vmp.off")
		if st := e.StateOf(context.Background(), f, tw); st.Status == "on" {
			t.Errorf("%v: estado desconhecido não pode contar como desativado: %+v", row, st.Ops)
		}
		res := e.Apply(context.Background(), f, []string{"adv.vmp.off"}, tweaks.ApplyOpts{AllowBlocked: true})
		if res[0].Outcome == tweaks.OutAlready || res[0].Outcome == tweaks.OutApplied {
			t.Errorf("%v: aplicar com estado desconhecido: %s %s", row, res[0].Outcome, res[0].Message)
		}
	}
}

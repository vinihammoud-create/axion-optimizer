package tweaks_test

import (
	"encoding/json"
	"testing"
	"time"

	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

// 0.7: failed applications left "pending" by a version without the
// carry-over, and covered by a later successful application of the same
// tweak, are closed at startup (the desktop report of 05/10 listed the same
// item four times as "undo again"). A change on a resource the newer entry
// did not touch is carried to it.
func TestReconcileHistory(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	tw := regTweak("t.usb", tweaks.Reg(testKey, "A", sys.DWord(0)))
	e := newEngine(t, m, tw)
	before, _ := json.Marshal(map[string]any{"exists": true, "kind": "dword", "int": 1})
	rec := func(target string) tweaks.OpRecord {
		return tweaks.OpRecord{Kind: "registry", Target: target, Describe: target, Before: before, Verified: true}
	}
	old1 := &tweaks.Entry{ID: "aaaa0001", TweakID: "t.usb", TweakTitle: "USB", Category: "entrada", Time: time.Now().Add(-3 * time.Hour), Status: tweaks.StatusFailed, Ops: []tweaks.OpRecord{rec(testKey + `\A`)}}
	old2 := &tweaks.Entry{ID: "aaaa0002", TweakID: "t.usb", TweakTitle: "USB", Category: "entrada", Time: time.Now().Add(-2 * time.Hour), Status: tweaks.StatusPartialRev, Ops: []tweaks.OpRecord{rec(testKey + `\A`), rec(testKey + `\Other`)}}
	other := &tweaks.Entry{ID: "aaaa0003", TweakID: "t.x", TweakTitle: "X", Time: time.Now().Add(-2 * time.Hour), Status: tweaks.StatusFailed, Ops: []tweaks.OpRecord{rec(testKey + `\X`)}}
	newest := &tweaks.Entry{ID: "aaaa0004", TweakID: "t.usb", TweakTitle: "USB", Category: "entrada", Time: time.Now().Add(-time.Hour), Status: tweaks.StatusApplied, Ops: []tweaks.OpRecord{rec(testKey + `\A`)}}
	for _, x := range []*tweaks.Entry{old1, old2, other, newest} {
		if err := e.J.Add(x); err != nil {
			t.Fatal(err)
		}
	}
	if !old1.Active() || !old2.Active() || !newest.Active() {
		t.Fatal("entradas pendentes antes da reconciliação")
	}
	if n := e.ReconcileHistory(); n != 2 {
		t.Fatalf("reconciliadas: %d", n)
	}
	if old1.Status != tweaks.StatusSuperseded || old2.Status != tweaks.StatusSuperseded || old1.Active() || old2.Active() {
		t.Errorf("antigas: %s %s", old1.Status, old2.Status)
	}
	if other.Status != tweaks.StatusFailed || !other.Active() {
		t.Errorf("outro item não deve ser tocado: %s", other.Status)
	}
	carried := false
	for _, r := range newest.Ops {
		if r.Target == testKey+`\Other` && r.CarriedFrom == "aaaa0002" {
			carried = true
		}
	}
	if !carried || len(newest.Ops) != 2 {
		t.Errorf("registro não coberto deveria ser carregado para a aplicação mais nova: %+v", newest.Ops)
	}
	// Idempotent, and persisted.
	if n := e.ReconcileHistory(); n != 0 {
		t.Errorf("segunda passagem: %d", n)
	}
	e2 := newEngine(t, m, tw)
	if got := e2.J.Get("aaaa0001"); got == nil || got.Status != tweaks.StatusSuperseded {
		t.Errorf("não persistiu: %+v", got)
	}
	// A newer failed attempt after the applied one is not closed by it.
	later := &tweaks.Entry{ID: "aaaa0005", TweakID: "t.usb", TweakTitle: "USB", Time: time.Now(), Status: tweaks.StatusFailed, Ops: []tweaks.OpRecord{rec(testKey + `\A`)}}
	_ = e2.J.Add(later)
	if n := e2.ReconcileHistory(); n != 0 || later.Status != tweaks.StatusFailed {
		t.Errorf("tentativa posterior: %d %s", n, later.Status)
	}
}

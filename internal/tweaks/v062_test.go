package tweaks_test

import (
	"context"
	"testing"
	"time"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

// A tweak whose values Windows may ignore reports whether it is in effect:
// "reboot" right after Axion applied it, "ignored" when it still does not
// work after a restart (the first notebook report showed the VBS switch "on"
// with VBS running). The status itself keeps describing the values.
func TestEffectOfAppliedTweak(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	working := false
	tw := regTweak("t.effect", tweaks.Reg(testKey, "E", sys.DWord(0)))
	tw.Reboot = tweaks.RebootRestart // the VBS switch: only a restart can make it take effect
	tw.Effect = func(*facts.Facts) (bool, string) { return working, "uma política mantém ligado" }
	now := regTweak("t.now", tweaks.Reg(testKey, "G", sys.DWord(0)))
	now.Reboot = tweaks.RebootNone
	now.Effect = tw.Effect
	e := newEngine(t, m, tw, now)
	ctx := context.Background()
	f := &facts.Facts{}
	f.OS.LastBoot = time.Now().Add(-time.Hour)

	if st := e.StateOf(ctx, f, tw); st.Status != "off" || st.Effect != "" {
		t.Fatalf("antes de aplicar: %+v", st)
	}
	if r := e.Apply(ctx, f, []string{"t.effect"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	st := e.StateOf(ctx, f, tw)
	if st.Status != "on" || st.Effect != "reboot" || st.EffectNote == "" || st.Drifted {
		t.Fatalf("aplicado depois do último boot: %+v", st)
	}
	f.OS.LastBoot = time.Now().Add(time.Minute) // the PC restarted since
	if st := e.StateOf(ctx, f, tw); st.Effect != "ignored" || st.EffectNote != "uma política mantém ligado" || st.Status != "on" {
		t.Fatalf("ignorado após reiniciar: %+v", st)
	}
	working = true
	if st := e.StateOf(ctx, f, tw); st.Effect != "ok" {
		t.Fatalf("valendo: %+v", st)
	}
	// Values set by someone else, not working: ignored, no reboot guess.
	working = false
	_ = m.R.SetValue(testKey, "F", sys.DWord(0))
	other := regTweak("t.other", tweaks.Reg(testKey, "F", sys.DWord(0)))
	other.Effect = tw.Effect
	if st := e.StateOf(ctx, f, other); st.Status != "on" || st.ByAxion || st.Effect != "ignored" {
		t.Fatalf("aplicado fora do Axion: %+v", st)
	}
	// A change that needs no restart (0.7: the active hours with the
	// automatic adjustment on) is ignored at once, never "reboot".
	f.OS.LastBoot = time.Now().Add(-time.Hour)
	if r := e.Apply(ctx, f, []string{"t.now"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar sem reinício: %+v", r[0])
	}
	if st := e.StateOf(ctx, f, now); st.Effect != "ignored" || st.EffectNote != "uma política mantém ligado" {
		t.Fatalf("sem reinício, ignorado na hora: %+v", st)
	}
}

// Post gets the targets of the operations that changed, on apply and on
// revert, so a network tweak restarts only the adapters it touched.
func TestPostGetsTouchedTargets(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	_ = m.R.SetValue(testKey, "Same", sys.DWord(1))
	changed := tweaks.Reg(testKey, "New", sys.DWord(1))
	same := tweaks.Reg(testKey, "Same", sys.DWord(1))
	var got [][]string
	tw := regTweak("t.post", changed, same)
	tw.Post = func(c *tweaks.Ctx, reverted bool) error {
		got = append(got, append([]string(nil), c.Touched...))
		if !c.TouchedTarget(changed.Target()) || c.TouchedTarget(same.Target()) {
			t.Errorf("alvos tocados (revert=%v): %v", reverted, c.Touched)
		}
		return nil
	}
	e := newEngine(t, m, tw)
	ctx, f := context.Background(), &facts.Facts{}
	if r := e.Apply(ctx, f, []string{"t.post"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	if r := e.RevertTweaks(ctx, f, []string{"t.post"}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer: %+v", r[0])
	}
	if len(got) != 2 || len(got[0]) != 1 || len(got[1]) != 1 {
		t.Fatalf("Post chamado com %v", got)
	}
}

func TestUltimateCopyName(t *testing.T) {
	for name, want := range map[string]bool{"Desempenho Máximo": true, "desempenho  máximo": true, "Ultimate Performance": true,
		"Máximo rendimiento": true, "Desempenho Máximo (cópia)": false, "Alto desempenho": false, "": false} {
		if tweaks.UltimateCopyName(name) != want {
			t.Errorf("%q: esperado %v", name, want)
		}
	}
}

// A tweak whose resources depend on the machine (one per network adapter)
// gains a resource it did not have when it was applied: the status is
// partial, but nothing Axion did was undone, so it is not a drift (0.6.2 reads
// unplugged adapters too; entries from 0.6.1 must not raise a false alarm).
// A recorded resource changed by someone else still is.
func TestDriftOnlyOnRecordedResources(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	targets := []string{"A"}
	tw := &tweaks.Tweak{ID: "t.dyn", Category: "rede", Title: "t.dyn", Evidence: tweaks.EvHigh, Risk: tweaks.RiskSafe,
		Ops: func(*facts.Facts) []tweaks.Op {
			var ops []tweaks.Op
			for _, n := range targets {
				ops = append(ops, tweaks.Reg(testKey, n, sys.DWord(0)))
			}
			return ops
		}}
	e := newEngine(t, m, tw)
	ctx, f := context.Background(), &facts.Facts{}
	if r := e.Apply(ctx, f, []string{"t.dyn"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	targets = []string{"A", "B"} // a second adapter is now read
	_ = m.R.SetValue(testKey, "B", sys.DWord(1))
	st := e.StateOf(ctx, f, tw)
	if st.Status != "partial" || st.Drifted {
		t.Fatalf("recurso novo não é desfeito: %+v", st)
	}
	_ = m.R.SetValue(testKey, "A", sys.DWord(1)) // someone undid Axion's change
	if st := e.StateOf(ctx, f, tw); !st.Drifted {
		t.Fatalf("recurso gravado pelo Axion e alterado depois é desfeito: %+v", st)
	}
}

// Review of 0.6.2: applied by Axion, nothing undone, but every resource the
// tweak covers now is new: the status is partial (pending), not off.
func TestNewResourcesOnlyArePending(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	targets := []string{"A"}
	tw := &tweaks.Tweak{ID: "t.new", Category: "rede", Title: "t.new", Evidence: tweaks.EvHigh, Risk: tweaks.RiskSafe,
		Ops: func(*facts.Facts) []tweaks.Op {
			var ops []tweaks.Op
			for _, n := range targets {
				ops = append(ops, tweaks.Reg(testKey, n, sys.DWord(0)))
			}
			return ops
		}}
	e := newEngine(t, m, tw)
	ctx, f := context.Background(), &facts.Facts{}
	if r := e.Apply(ctx, f, []string{"t.new"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	targets = []string{"C"} // only a resource the entry never had
	_ = m.R.SetValue(testKey, "C", sys.DWord(1))
	if st := e.StateOf(ctx, f, tw); st.Status != "partial" || st.Drifted || !st.ByAxion {
		t.Fatalf("só recursos novos: %+v", st)
	}
}

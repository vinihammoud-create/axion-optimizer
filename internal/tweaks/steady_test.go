package tweaks_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

func restoreNamed(c *tweaks.Ctx, name string, raw json.RawMessage) error {
	var v sys.RegValue
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	if !v.Exists {
		return c.P.Reg.DeleteValue(testKey, name)
	}
	return c.P.Reg.SetValue(testKey, name, v)
}

// ctxOp writes one value. Like the operations that run a Windows tool, it
// fails once the context it is given is cancelled.
func ctxOp(name string, want uint64, onApply, onRestore func()) *tweaks.FuncOp {
	return &tweaks.FuncOp{KindName: "ctx", ID: name, Desc: "grava " + name,
		SnapFn: func(c *tweaks.Ctx) (any, error) { return c.P.Reg.GetValue(testKey, name) },
		InFn: func(c *tweaks.Ctx) (bool, error) {
			v, err := c.P.Reg.GetValue(testKey, name)
			return v.Exists && v.Int == want, err
		},
		ApplyFn: func(c *tweaks.Ctx) error {
			if onApply != nil {
				onApply()
			}
			if err := c.Context.Err(); err != nil {
				return err
			}
			return c.P.Reg.SetValue(testKey, name, sys.DWord(uint32(want)))
		},
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			if onRestore != nil {
				onRestore()
			}
			if err := c.Context.Err(); err != nil {
				return err
			}
			return restoreNamed(c, name, raw)
		},
	}
}

// A cancellation that arrives while one entry is being undone does not stop
// that undo halfway: the entry is undone to the end.
func TestCancelDoesNotInterruptAnUndo(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	_ = m.R.SetValue(testKey, "A", sys.DWord(1))
	_ = m.R.SetValue(testKey, "B", sys.DWord(2))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Undo runs last to first: restoring B cancels, then A is restored.
	e := newEngine(t, m, regTweak("t.two", ctxOp("A", 11, nil, nil), ctxOp("B", 12, nil, cancel)))
	f := &facts.Facts{}
	if r := e.Apply(ctx, f, []string{"t.two"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0])
	}
	r := e.RevertEntries(ctx, f, e.J.Active(), tweaks.RevertOpts{})
	if ctx.Err() == nil {
		t.Fatal("o teste deveria ter cancelado no meio do desfazer")
	}
	if r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("o desfazer foi interrompido pelo cancelamento: %+v", r[0])
	}
	if a, b := regInt(t, m, "A"), regInt(t, m, "B"); a != 1 || b != 2 {
		t.Errorf("valores depois do desfazer: A=%d B=%d, esperado 1 e 2", a, b)
	}
	if len(e.J.Active()) != 0 {
		t.Error("a entrada deveria estar desfeita")
	}
}

// Cancelled between entries, the ones not started stay applied and in the
// history, and the result says so.
func TestCancelBetweenEntriesKeepsTheRest(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	_ = m.R.SetValue(testKey, "A", sys.DWord(1))
	_ = m.R.SetValue(testKey, "B", sys.DWord(2))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := newEngine(t, m, regTweak("t.a", ctxOp("A", 11, nil, nil)), regTweak("t.b", ctxOp("B", 12, nil, cancel)))
	f := &facts.Facts{}
	for _, r := range e.Apply(ctx, f, []string{"t.a", "t.b"}, tweaks.ApplyOpts{}) {
		if r.Outcome != tweaks.OutApplied {
			t.Fatal(r)
		}
	}
	// Newest first: t.b is undone (and cancels), t.a is not started.
	r := e.RevertAll(ctx, f, tweaks.RevertOpts{})
	if len(r) != 2 || r[0].Outcome != tweaks.OutReverted || r[1].Outcome != tweaks.OutFailed {
		t.Fatalf("resultados: %+v", r)
	}
	if a, b := regInt(t, m, "A"), regInt(t, m, "B"); a != 11 || b != 2 {
		t.Errorf("A=%d B=%d: esperado A ainda aplicado (11) e B desfeito (2)", a, b)
	}
	if act := e.J.Active(); len(act) != 1 || act[0].TweakID != "t.a" {
		t.Errorf("t.a deveria continuar no histórico como aplicado: %+v", act)
	}
}

// An apply that fails after a cancellation still rolls back what it wrote.
func TestCancelDoesNotBreakTheRollback(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	_ = m.R.SetValue(testKey, "A", sys.DWord(1))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bad := &tweaks.FuncOp{KindName: "fail", ID: "x", Desc: "cancela e falha",
		SnapFn:    func(*tweaks.Ctx) (any, error) { return 0, nil },
		InFn:      func(*tweaks.Ctx) (bool, error) { return false, nil },
		ApplyFn:   func(*tweaks.Ctx) error { cancel(); return errors.New("a ferramenta foi encerrada") },
		RestoreFn: func(*tweaks.Ctx, json.RawMessage) error { return nil },
	}
	e := newEngine(t, m, regTweak("t.roll", ctxOp("A", 11, nil, nil), bad))
	r := e.Apply(ctx, &facts.Facts{}, []string{"t.roll"}, tweaks.ApplyOpts{})
	if r[0].Outcome != tweaks.OutFailed {
		t.Fatalf("a aplicação deveria falhar: %+v", r[0])
	}
	if got := regInt(t, m, "A"); got != 1 {
		t.Errorf("a reversão automática não devolveu A: %d, esperado 1", got)
	}
	if len(e.J.Active()) != 0 {
		t.Errorf("nada deveria ficar pendente no histórico: %+v", e.J.Active())
	}
}

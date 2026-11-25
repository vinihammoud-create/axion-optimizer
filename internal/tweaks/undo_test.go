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

func restoreRegA(c *tweaks.Ctx, raw json.RawMessage) error {
	var v sys.RegValue
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	return c.P.Reg.SetValue(testKey, "A", v)
}

// A value of a type Axion has no special handling for (REG_NONE here) must
// come back with the same type and the same bytes after an undo.
func TestRevertRestoresRawTypeAndBytes(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	orig := sys.RegValue{Exists: true, Kind: sys.RegNone, Bin: []byte{0xDE, 0xAD, 0x00, 0xBE, 0xEF}}
	_ = m.R.SetValue(testKey, "Raw", orig)
	e := newEngine(t, m, regTweak("t.raw", tweaks.Reg(testKey, "Raw", sys.DWord(1))))
	ctx, f := context.Background(), &facts.Facts{}
	if r := e.Apply(ctx, f, []string{"t.raw"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0])
	}
	if r := e.RevertAll(ctx, f, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatal(r[0])
	}
	got, _ := m.R.GetValue(testKey, "Raw")
	if !got.Equal(orig) {
		t.Fatalf("depois do desfazer: %+v, esperado %+v", got, orig)
	}
}

// An entry with no "after" (apply interrupted) whose value was then changed
// by someone else must not be overwritten by an undo without Force.
func TestRevertInterruptedEntryConflictsWithThirdParty(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	_ = m.R.SetValue(testKey, "A", sys.DWord(1))
	op := &tweaks.FuncOp{KindName: "crash", ID: "a", Desc: "grava A e o processo morre",
		SnapFn:    func(c *tweaks.Ctx) (any, error) { return c.P.Reg.GetValue(testKey, "A") },
		InFn:      func(c *tweaks.Ctx) (bool, error) { v, _ := c.P.Reg.GetValue(testKey, "A"); return v.Int == 9, nil },
		ApplyFn:   func(c *tweaks.Ctx) error { panic("queda de energia") },
		RestoreFn: restoreRegA,
	}
	tw := regTweak("t.int", op)
	e := newEngine(t, m, tw)
	ctx, f := context.Background(), &facts.Facts{}
	func() {
		defer func() { _ = recover() }()
		e.Apply(ctx, f, []string{"t.int"}, tweaks.ApplyOpts{})
	}()
	e2 := newEngine(t, m, tw)
	e2.J.RecoverInterrupted()

	_ = m.R.SetValue(testKey, "A", sys.DWord(7)) // another program
	r := e2.RevertAll(ctx, f, tweaks.RevertOpts{})
	if len(r) != 1 || r[0].Outcome != tweaks.OutConflict {
		t.Fatalf("esperava conflito, obtido %+v", r)
	}
	if v := regInt(t, m, "A"); v != 7 {
		t.Fatalf("o valor de terceiros foi sobrescrito: A=%d", v)
	}
	r = e2.RevertAll(ctx, f, tweaks.RevertOpts{Force: true})
	if len(r) != 1 || r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("com Force deveria desfazer: %+v", r)
	}
	if v := regInt(t, m, "A"); v != 1 {
		t.Fatalf("A=%d depois do Force, esperado 1", v)
	}
}

// When the current state cannot be read, the undo does not know whether
// someone else changed it, so without Force it reports a conflict.
func TestRevertWithUnreadableStateConflicts(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	_ = m.R.SetValue(testKey, "A", sys.DWord(1))
	failRead := false
	op := &tweaks.FuncOp{KindName: "rd", ID: "a", Desc: "grava A",
		SnapFn: func(c *tweaks.Ctx) (any, error) {
			if failRead {
				return nil, errors.New("leitura falhou")
			}
			return c.P.Reg.GetValue(testKey, "A")
		},
		InFn:      func(c *tweaks.Ctx) (bool, error) { v, _ := c.P.Reg.GetValue(testKey, "A"); return v.Int == 9, nil },
		ApplyFn:   func(c *tweaks.Ctx) error { return c.P.Reg.SetValue(testKey, "A", sys.DWord(9)) },
		RestoreFn: restoreRegA,
	}
	e := newEngine(t, m, regTweak("t.rd", op))
	ctx, f := context.Background(), &facts.Facts{}
	if r := e.Apply(ctx, f, []string{"t.rd"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0])
	}
	failRead = true
	r := e.RevertAll(ctx, f, tweaks.RevertOpts{})
	if len(r) != 1 || r[0].Outcome != tweaks.OutConflict {
		t.Fatalf("esperava conflito, obtido %+v", r)
	}
	if v := regInt(t, m, "A"); v != 9 {
		t.Fatalf("A=%d: nada deveria ter sido gravado", v)
	}
}

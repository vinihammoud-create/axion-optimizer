package tweaks_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

const testKey = `HKCU\Software\AxionTest`

func newEngine(t *testing.T, m *mocksys.Mock, tws ...*tweaks.Tweak) *tweaks.Engine {
	t.Helper()
	cat, err := tweaks.NewCatalog(tws)
	if err != nil {
		t.Fatal(err)
	}
	j, err := tweaks.OpenJournal(m.DataDir, m.Key)
	if err != nil {
		t.Fatal(err)
	}
	return tweaks.NewEngine(m.Platform, cat, j)
}

func regTweak(id string, ops ...tweaks.Op) *tweaks.Tweak {
	return &tweaks.Tweak{ID: id, Category: "jogo", Title: id, Evidence: tweaks.EvHigh, Risk: tweaks.RiskSafe,
		Ops: func(*facts.Facts) []tweaks.Op { return ops }}
}

func regInt(t *testing.T, m *mocksys.Mock, name string) int64 {
	t.Helper()
	v, err := m.R.GetValue(testKey, name)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Exists {
		return -1
	}
	return int64(v.Int)
}

// Applying a tweak again after part of it was undone by something else must
// keep the value from before Axion for the parts still applied: undoing
// everything returns the machine to where it was before Axion.
func TestReapplyKeepsOriginalValues(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	_ = m.R.SetValue(testKey, "A", sys.DWord(126))
	_ = m.R.SetValue(testKey, "B", sys.DWord(62))
	tw := regTweak("t.two", tweaks.Reg(testKey, "A", sys.DWord(122)), tweaks.Reg(testKey, "B", sys.DWord(58)))
	e := newEngine(t, m, tw)
	ctx, f := context.Background(), &facts.Facts{}
	if r := e.Apply(ctx, f, []string{"t.two"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0])
	}
	// The user turns one of them back on in Settings.
	_ = m.R.SetValue(testKey, "B", sys.DWord(62))
	st := e.StateOf(ctx, f, tw)
	if !st.Drifted {
		t.Fatalf("esperava 'desfeito depois': %+v", st)
	}
	if r := e.Apply(ctx, f, []string{"t.two"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0])
	}
	if n := len(e.J.Active()); n != 1 {
		t.Fatalf("esperava 1 entrada ativa depois de reaplicar, obtido %d", n)
	}
	for _, r := range e.RevertAll(ctx, f, tweaks.RevertOpts{}) {
		if r.Outcome != tweaks.OutReverted {
			t.Fatalf("reversão: %+v", r)
		}
	}
	if a, b := regInt(t, m, "A"), regInt(t, m, "B"); a != 126 || b != 62 {
		t.Fatalf("valores originais perdidos: A=%d (126) B=%d (62)", a, b)
	}
}

// Power settings belong to a plan: applying a power tweak on one plan,
// switching plans and applying it again must keep both plans' original
// values.
func TestReapplyAcrossPowerPlans(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	p := m.Platform.Power
	bal, high := sys.SchemeBalanced, sys.SchemeHighPerformance
	_ = p.SetActiveScheme(bal)
	_ = p.WriteValue(bal, tweaks.SubUSB, tweaks.SetUSBSuspend, true, 1)
	_ = p.WriteValue(high, tweaks.SubUSB, tweaks.SetUSBSuspend, true, 1)
	tw := &tweaks.Tweak{ID: "t.usb", Category: "energia", Title: "USB", Evidence: tweaks.EvLow, Risk: tweaks.RiskSafe,
		Ops: func(*facts.Facts) []tweaks.Op {
			return []tweaks.Op{tweaks.PowerSetting("USB", tweaks.SubUSB, tweaks.SetUSBSuspend, tweaks.U32(0), nil)}
		}}
	e := newEngine(t, m, tw)
	ctx, f := context.Background(), &facts.Facts{}
	if r := e.Apply(ctx, f, []string{"t.usb"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0])
	}
	_ = p.SetActiveScheme(high)
	if st := e.StateOf(ctx, f, tw); st.Status == "on" {
		t.Fatalf("no plano novo o ajuste não está ativo: %+v", st)
	}
	if r := e.Apply(ctx, f, []string{"t.usb"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0])
	}
	for _, r := range e.RevertAll(ctx, f, tweaks.RevertOpts{}) {
		if r.Outcome != tweaks.OutReverted {
			t.Fatalf("reversão: %+v", r)
		}
	}
	for _, sch := range []string{bal, high} {
		if v, _ := p.ReadValue(sch, tweaks.SubUSB, tweaks.SetUSBSuspend, true); v != 1 {
			t.Errorf("plano %s ficou com %d (original 1)", sch, v)
		}
	}
}

// The history is never lost to a crash in the middle of a save: a missing
// or damaged journal.json is recovered from its temporary or backup copy
// (files left by older versions, which renamed the journal away first).
func TestJournalRecoversCopies(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	_ = m.R.SetValue(testKey, "A", sys.DWord(1))
	e := newEngine(t, m, regTweak("t.j", tweaks.Reg(testKey, "A", sys.DWord(5))))
	if r := e.Apply(context.Background(), &facts.Facts{}, []string{"t.j"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0])
	}
	p := filepath.Join(m.DataDir, "journal.json")
	good, _ := os.ReadFile(p)
	// State an old version could leave: journal renamed to .bak, new one
	// still in .tmp.
	_ = os.WriteFile(p+".tmp", good, 0o644)
	_ = os.Rename(p, p+".bak")
	j2, err := tweaks.OpenJournal(m.DataDir, m.Key)
	if err != nil || len(j2.Active()) != 1 {
		t.Fatalf("histórico não recuperado da cópia temporária: %v, %d ativas", err, len(j2.Active()))
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("journal.json deveria ser regravado depois de recuperar: %v", err)
	}
	for _, en := range j2.Entries() {
		if !en.Verified() {
			t.Errorf("entrada recuperada perdeu a assinatura: %+v", en)
		}
	}
	// Zero-filled journal (power loss without flush) with a valid backup.
	_ = os.Remove(p + ".tmp")
	_ = os.WriteFile(p, make([]byte, len(good)), 0o644)
	_ = os.WriteFile(p+".bak", good, 0o644)
	j3, err := tweaks.OpenJournal(m.DataDir, m.Key)
	if err != nil || len(j3.Active()) != 1 {
		t.Fatalf("diário corrompido não recuperado do .bak: %v, %d ativas", err, len(j3.Active()))
	}
	if matches, _ := filepath.Glob(p + ".corrupt-*"); len(matches) == 0 {
		t.Errorf("o arquivo corrompido deveria ser guardado para análise")
	}
}

// If the previous value cannot be saved, the machine is not changed.
func TestWriteAheadFailureChangesNothing(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	_ = m.R.SetValue(testKey, "A", sys.DWord(1))
	e := newEngine(t, m, regTweak("t.w", tweaks.Reg(testKey, "A", sys.DWord(5))))
	// Every save fails: the temporary file path is a directory.
	tmp := filepath.Join(m.DataDir, "journal.json.tmp")
	_ = os.MkdirAll(filepath.Join(tmp, "x"), 0o755)
	r := e.Apply(context.Background(), &facts.Facts{}, []string{"t.w"}, tweaks.ApplyOpts{})
	if r[0].Outcome != tweaks.OutFailed || !strings.Contains(r[0].Message, "nada foi alterado") {
		t.Fatalf("esperava falha sem mudança: %+v", r[0])
	}
	if a := regInt(t, m, "A"); a != 1 {
		t.Fatalf("o PC foi alterado sem o valor anterior salvo: A=%d", a)
	}
	if len(e.J.Active()) != 0 {
		t.Errorf("nenhuma entrada deveria ficar ativa")
	}
}

type flakyRestore struct {
	*tweaks.RegOp
	fail *int // restores that fail before one succeeds
}

func (o *flakyRestore) Restore(c *tweaks.Ctx, raw json.RawMessage) error {
	if *o.fail > 0 {
		*o.fail--
		return errors.New("acesso negado (transitório)")
	}
	return o.RegOp.Restore(c, raw)
}

// An undo that fails for one item can be retried: the entry stays active
// until every value is back.
func TestPartialRevertIsRetryable(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	_ = m.R.SetValue(testKey, "A", sys.DWord(1))
	_ = m.R.SetValue(testKey, "B", sys.DWord(1))
	fails := 0
	op := &flakyRestore{tweaks.Reg(testKey, "B", sys.DWord(2)), &fails}
	tw := regTweak("t.p", tweaks.Reg(testKey, "A", sys.DWord(2)), op)
	e := newEngine(t, m, tw)
	ctx, f := context.Background(), &facts.Facts{}
	if r := e.Apply(ctx, f, []string{"t.p"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0])
	}
	fails = 1
	r1 := e.RevertTweaks(ctx, f, []string{"t.p"}, tweaks.RevertOpts{})
	if r1[0].Outcome != tweaks.OutFailed {
		t.Fatalf("primeira reversão deveria falhar em parte: %+v", r1[0])
	}
	if a, b := regInt(t, m, "A"), regInt(t, m, "B"); a != 1 || b != 2 {
		t.Fatalf("depois da falha: A=%d (1) B=%d (2)", a, b)
	}
	st := e.StateOf(ctx, f, tw)
	if !st.ByAxion || !st.PendingUndo {
		t.Fatalf("a alteração que sobrou deveria continuar desfazível: %+v", st)
	}
	r2 := e.RevertTweaks(ctx, f, []string{"t.p"}, tweaks.RevertOpts{})
	if r2[0].Outcome != tweaks.OutReverted {
		t.Fatalf("nova tentativa: %+v", r2[0])
	}
	if b := regInt(t, m, "B"); b != 1 {
		t.Fatalf("B não voltou: %d", b)
	}
	if len(e.J.Active()) != 0 {
		t.Errorf("nada deveria ficar ativo")
	}
}

// A failed apply whose rollback also fails leaves the change undoable.
func TestFailedRollbackIsRetryable(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	_ = m.R.SetValue(testKey, "A", sys.DWord(1))
	fails := 1
	a := &flakyRestore{tweaks.Reg(testKey, "A", sys.DWord(5)), &fails}
	bad := &tweaks.FuncOp{KindName: "fail", ID: "x", Desc: "sempre falha",
		SnapFn:    func(*tweaks.Ctx) (any, error) { return 0, nil },
		InFn:      func(*tweaks.Ctx) (bool, error) { return false, nil },
		ApplyFn:   func(*tweaks.Ctx) error { return errors.New("dispositivo removido") },
		RestoreFn: func(*tweaks.Ctx, json.RawMessage) error { return nil },
	}
	tw := regTweak("t.r", a, bad)
	e := newEngine(t, m, tw)
	ctx, f := context.Background(), &facts.Facts{}
	r := e.Apply(ctx, f, []string{"t.r"}, tweaks.ApplyOpts{})
	if r[0].Outcome != tweaks.OutFailed || !strings.Contains(r[0].Message, "Desfazer") {
		t.Fatalf("esperava falha com orientação para desfazer: %+v", r[0])
	}
	if v := regInt(t, m, "A"); v != 5 {
		t.Fatalf("pré-condição: o rollback deveria ter falhado (A=%d)", v)
	}
	res := e.RevertAll(ctx, f, tweaks.RevertOpts{})
	if len(res) != 1 || res[0].Outcome != tweaks.OutReverted {
		t.Fatalf("a mudança que sobrou deveria ser desfeita: %+v", res)
	}
	if v := regInt(t, m, "A"); v != 1 {
		t.Fatalf("A=%d (esperado 1)", v)
	}
}

// A crash in the middle of an operation, then applying the same tweak
// again: the value captured before the crash is the real original.
func TestReapplyAfterInterruption(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	_ = m.R.SetValue(testKey, "A", sys.DWord(1))
	crash := true
	half := &tweaks.FuncOp{KindName: "half", ID: "a", Desc: "grava A em duas etapas",
		SnapFn: func(c *tweaks.Ctx) (any, error) { return c.P.Reg.GetValue(testKey, "A") },
		InFn:   func(c *tweaks.Ctx) (bool, error) { v, _ := c.P.Reg.GetValue(testKey, "A"); return v.Int == 9, nil },
		ApplyFn: func(c *tweaks.Ctx) error {
			_ = c.P.Reg.SetValue(testKey, "A", sys.DWord(4)) // half written
			if crash {
				panic("queda de energia")
			}
			return c.P.Reg.SetValue(testKey, "A", sys.DWord(9))
		},
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var v sys.RegValue
			if err := json.Unmarshal(raw, &v); err != nil {
				return err
			}
			return c.P.Reg.SetValue(testKey, "A", v)
		},
	}
	tw := regTweak("t.i", half)
	e := newEngine(t, m, tw)
	ctx, f := context.Background(), &facts.Facts{}
	func() {
		defer func() { _ = recover() }()
		e.Apply(ctx, f, []string{"t.i"}, tweaks.ApplyOpts{})
	}()
	e2 := newEngine(t, m, tw)
	e2.J.RecoverInterrupted()
	crash = false
	if r := e2.Apply(ctx, f, []string{"t.i"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0])
	}
	if n := len(e2.J.Active()); n != 1 {
		t.Fatalf("a entrada interrompida deveria ser substituída: %d ativas", n)
	}
	if r := e2.RevertAll(ctx, f, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatal(r[0])
	}
	if v := regInt(t, m, "A"); v != 1 {
		t.Fatalf("A=%d: o valor pela metade (4) virou o 'anterior'", v)
	}
}

// A record that this installation did not sign is never carried into a new
// (signed) entry, where it would be trusted.
func TestUnverifiedRecordIsNotCarried(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	_ = m.R.SetValue(testKey, "A", sys.DWord(1))
	_ = m.R.SetValue(testKey, "B", sys.DWord(1))
	tw := regTweak("t.u", tweaks.Reg(testKey, "A", sys.DWord(5)), tweaks.Reg(testKey, "B", sys.DWord(5)))
	e := newEngine(t, m, tw)
	ctx, f := context.Background(), &facts.Facts{}
	if r := e.Apply(ctx, f, []string{"t.u"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0])
	}
	// Someone edits the journal: an extra record pointing at IFEO.
	path := filepath.Join(m.DataDir, "journal.json")
	raw, _ := os.ReadFile(path)
	var doc map[string]any
	_ = json.Unmarshal(raw, &doc)
	ifeo := `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options\sethc.exe`
	for _, x := range doc["entries"].([]any) {
		en := x.(map[string]any)
		ops := en["ops"].([]any)
		en["ops"] = append(ops, map[string]any{"kind": "registry", "target": "reg:" + strings.ToLower(ifeo) + "|debugger", "describe": "forjado",
			"before": json.RawMessage(`{"value":{"exists":true,"kind":1,"str":"C:\\evil.exe"}}`)})
	}
	raw, _ = json.Marshal(doc)
	_ = os.WriteFile(path, raw, 0o644)
	e2 := newEngine(t, m, tw)
	_ = m.R.SetValue(testKey, "B", sys.DWord(1)) // drift, so the tweak is applied again
	if r := e2.Apply(ctx, f, []string{"t.u"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0])
	}
	res := e2.RevertAll(ctx, f, tweaks.RevertOpts{Force: true})
	if ok, _ := m.R.KeyExists(ifeo); ok {
		t.Fatal("registro forjado foi levado para uma entrada assinada e executado")
	}
	// The new (signed) entry is undone; the tampered one is refused as a
	// whole and stays listed for a manual undo.
	if b := regInt(t, m, "B"); b != 1 {
		t.Errorf("B deveria voltar pela entrada nova: %d", b)
	}
	var refused bool
	for _, r := range res {
		if r.Outcome == tweaks.OutFailed && strings.Contains(r.Message, "Desfaça manualmente") {
			refused = true
		}
	}
	if !refused {
		t.Errorf("a entrada adulterada deveria ser recusada: %+v", res)
	}
}

func TestRecommendableRules(t *testing.T) {
	base := tweaks.Tweak{OneClick: true, Risk: tweaks.RiskSafe, Category: "sistema", Evidence: tweaks.EvHigh}
	if !base.Recommendable() {
		t.Fatal("safe, high-evidence one-click item must be recommendable")
	}
	for name, mut := range map[string]func(*tweaks.Tweak){
		"low evidence":  func(x *tweaks.Tweak) { x.Evidence = tweaks.EvLow },
		"advanced":      func(x *tweaks.Tweak) { x.Category = "avancado" },
		"risk":          func(x *tweaks.Tweak) { x.Risk = tweaks.RiskModerate },
		"not one-click": func(x *tweaks.Tweak) { x.OneClick = false },
	} {
		x := base
		mut(&x)
		if x.Recommendable() {
			t.Errorf("%s: must not be recommendable", name)
		}
	}
	low := base
	low.Evidence = tweaks.EvLow
	if !low.OptionalEligible() {
		t.Error("low-evidence safe item may still be offered unchecked in the complete tier")
	}
}

// Undoing a plan switch whose previous plan was deleted since: Balanced is
// activated and the entry is closed, not left pending forever.
func TestRevertPlanWhenPreviousPlanDeleted(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	high := "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c"
	_ = m.Platform.Power.SetActiveScheme(high)
	op := tweaks.Scheme(sys.SchemeBalanced, "Equilibrado")
	tw := &tweaks.Tweak{ID: "t.plan", Category: "energia", Title: "Plano", Summary: "s", Detail: "d", Evidence: tweaks.EvHigh, Risk: tweaks.RiskSafe,
		Ops: func(*facts.Facts) []tweaks.Op { return []tweaks.Op{op} }}
	cat, err := tweaks.NewCatalog([]*tweaks.Tweak{tw})
	if err != nil {
		t.Fatal(err)
	}
	j, err := tweaks.OpenJournal(m.DataDir, m.Key)
	if err != nil {
		t.Fatal(err)
	}
	e := tweaks.NewEngine(m.Platform, cat, j)
	f := &facts.Facts{}
	if r := e.Apply(context.Background(), f, []string{"t.plan"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("apply: %+v", r[0])
	}
	if err := m.Platform.Power.DeleteScheme(high); err != nil {
		t.Fatal(err)
	}
	r := e.RevertTweaks(context.Background(), f, []string{"t.plan"}, tweaks.RevertOpts{})
	if r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("revert: %+v", r[0])
	}
	if st := e.StateOf(context.Background(), f, tw); st.PendingUndo {
		t.Fatal("entry still pending")
	}
}

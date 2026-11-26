package tweaks_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

// flagOp is a per-user operation that is not a RegOp (the shape of the
// catalog's registry-flag, registry-bits, registry-merge, registry-layer and
// mouse operations): its target does not start with "reg:".
func flagOp() *tweaks.FuncOp {
	return &tweaks.FuncOp{KindName: "registry-flag", ID: strings.ToLower(testKey) + "|a", Desc: testKey + ` → "A" = 122`, PerUser: true,
		SnapFn:    func(c *tweaks.Ctx) (any, error) { return c.P.Reg.GetValue(testKey, "A") },
		InFn:      func(c *tweaks.Ctx) (bool, error) { v, err := c.P.Reg.GetValue(testKey, "A"); return v.Int == 122, err },
		ApplyFn:   func(c *tweaks.Ctx) error { return c.P.Reg.SetValue(testKey, "A", sys.DWord(122)) },
		RestoreFn: restoreRegA,
	}
}

func TestPerUserTarget(t *testing.T) {
	for _, tg := range []string{
		`reg:hkcu\software\x|a`, `registry-flag:hkcu\control panel\accessibility\stickykeys|flags`,
		`registry-bits:hkcu\control panel\accessibility\keyboard response|flags|4`,
		`registry-merge:hkcu\software\microsoft\directx\usergpupreferences|directxuserglobalsettings|swapeffectupgradeenable`,
		`registry-layer:c:\jogos\x.exe|disabledxmaximizedwindowedmode`, `mouse:acceleration`,
	} {
		if !tweaks.PerUserTarget(tg) {
			t.Errorf("%q é do perfil do usuário", tg)
		}
	}
	for _, tg := range []string{`reg:hklm\software\x|a`, `registry-flag:hklm\system\x|flags`, `svc:sysmain`, `bcd:useplatformclock`, `dns:{guid}`, `display:\\.\display1`, ""} {
		if tweaks.PerUserTarget(tg) {
			t.Errorf("%q é da máquina", tg)
		}
	}
	if op := flagOp(); !tweaks.PerUserTarget(op.Target()) {
		t.Errorf("alvo da operação de teste não reconhecido: %s", op.Target())
	}
}

// The undo guard covers every per-user operation, not only plain registry
// values: undone from another account, the tweak would write account A's
// "before" into account B's profile.
func TestRevertRefusesOtherUserForFuncOp(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	_ = m.R.SetValue(testKey, "A", sys.DWord(126))
	e := newEngine(t, m, regTweak("t.flag", flagOp()))
	ctx, f := context.Background(), &facts.Facts{}
	if r := e.Apply(ctx, f, []string{"t.flag"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0])
	}
	ent := e.J.Active()[0]

	m.SwitchUser(otherUserSID)
	_ = m.R.SetValue(testKey, "A", sys.DWord(122)) // B set the same value on their own
	r := e.RevertEntries(ctx, f, []*tweaks.Entry{ent}, tweaks.RevertOpts{})
	if r[0].Outcome == tweaks.OutReverted || !strings.Contains(r[0].Message, "outro usuário do Windows") {
		t.Fatalf("o desfazer como outro usuário devia recusar: %+v", r[0])
	}
	if got := regInt(t, m, "A"); got != 122 {
		t.Errorf("o perfil de B foi alterado: A = %d", got)
	}

	m.SwitchUser(mocksys.DefaultUserSID)
	if r := e.RevertEntries(ctx, f, []*tweaks.Entry{ent}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer como A: %+v", r[0])
	}
	if got := regInt(t, m, "A"); got != 126 {
		t.Errorf("A = %d, esperado 126", got)
	}
}

// What Axion changed in A's profile is not B's state: B does not see the
// tweak as applied, B's own apply keeps B's own "before", and each account
// undoes only its own entry.
func TestAccountsKeepTheirOwnEntries(t *testing.T) {
	for name, op := range map[string]func() tweaks.Op{
		"registro": func() tweaks.Op { return tweaks.Reg(testKey, "A", sys.DWord(122)) },
		"funcop":   func() tweaks.Op { return flagOp() },
	} {
		t.Run(name, func(t *testing.T) {
			m, _ := mocksys.New(t.TempDir())
			_ = m.R.SetValue(testKey, "A", sys.DWord(126))
			tw := regTweak("t.user", op())
			e := newEngine(t, m, tw)
			ctx, f := context.Background(), &facts.Facts{}
			if r := e.Apply(ctx, f, []string{"t.user"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
				t.Fatal(r[0])
			}
			entA := e.J.Active()[0]

			m.SwitchUser(otherUserSID)
			_ = m.R.SetValue(testKey, "A", sys.DWord(7)) // B's own value
			if st := e.StateOf(ctx, f, tw); st.ByAxion || st.EntryID != "" || st.Status == "on" {
				t.Fatalf("para B o ajuste não foi aplicado pelo Axion: %+v", st)
			}
			if e.ActiveFor("t.user") != nil {
				t.Fatal("a entrada de A não é a entrada ativa de B")
			}
			if r := e.RevertTweaks(ctx, f, []string{"t.user"}, tweaks.RevertOpts{}); r[0].Outcome == tweaks.OutReverted || !strings.Contains(r[0].Message, "outro usuário do Windows") {
				t.Fatalf("B não desfaz o que é de A: %+v", r[0])
			}
			if got := regInt(t, m, "A"); got != 7 {
				t.Fatalf("o perfil de B foi alterado pelo desfazer recusado: A = %d", got)
			}
			if r := e.Apply(ctx, f, []string{"t.user"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
				t.Fatal(r[0])
			}
			if entA.Status != tweaks.StatusApplied {
				t.Fatalf("a entrada de A foi substituída pela de B: %s", entA.Status)
			}
			entB := e.ActiveFor("t.user")
			if entB == nil || entB.ID == entA.ID {
				t.Fatal("B deveria ter a própria entrada")
			}
			for _, r := range entB.Ops {
				if r.CarriedFrom != "" {
					t.Errorf("o registro de A foi levado para a entrada de B: %+v", r)
				}
			}
			if r := e.RevertTweaks(ctx, f, []string{"t.user"}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
				t.Fatalf("B desfaz o que é seu: %+v", r[0])
			}
			if got := regInt(t, m, "A"); got != 7 {
				t.Errorf("perfil de B: A = %d, esperado 7 (o valor de B, não o de A)", got)
			}

			m.SwitchUser(mocksys.DefaultUserSID)
			if st := e.StateOf(ctx, f, tw); !st.ByAxion || st.EntryID != entA.ID {
				t.Fatalf("para A o ajuste continua aplicado: %+v", st)
			}
			if r := e.RevertTweaks(ctx, f, []string{"t.user"}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
				t.Fatalf("A desfaz o que é seu: %+v", r[0])
			}
			if got := regInt(t, m, "A"); got != 126 {
				t.Errorf("perfil de A: A = %d, esperado 126", got)
			}
		})
	}
}

// Reconciling the history never moves per-user records into an entry of
// another account.
func TestReconcileHistoryKeepsAccountsApart(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	tw := regTweak("t.usb", tweaks.Reg(testKey, "A", sys.DWord(0)))
	e := newEngine(t, m, tw)
	before, _ := json.Marshal(map[string]any{"exists": true, "kind": "dword", "int": 1})
	target := "reg:" + strings.ToLower(testKey) + "|"
	rec := func(name string) tweaks.OpRecord {
		return tweaks.OpRecord{Kind: "registry", Target: target + name, Describe: name, Before: before, Verified: true}
	}
	old := &tweaks.Entry{ID: "bbbb0001", TweakID: "t.usb", TweakTitle: "USB", Time: time.Now().Add(-2 * time.Hour), Status: tweaks.StatusFailed,
		UserSID: mocksys.DefaultUserSID, Ops: []tweaks.OpRecord{rec("a"), rec("other")}}
	newest := &tweaks.Entry{ID: "bbbb0002", TweakID: "t.usb", TweakTitle: "USB", Time: time.Now().Add(-time.Hour), Status: tweaks.StatusApplied,
		UserSID: otherUserSID, Ops: []tweaks.OpRecord{rec("a")}}
	for _, x := range []*tweaks.Entry{old, newest} {
		if err := e.J.Add(x); err != nil {
			t.Fatal(err)
		}
	}
	if n := e.ReconcileHistory(); n != 0 {
		t.Fatalf("entradas de contas diferentes foram reconciliadas: %d", n)
	}
	if old.Status != tweaks.StatusFailed || len(newest.Ops) != 1 {
		t.Errorf("a entrada de A mudou (%s) ou a de B ganhou registros (%d)", old.Status, len(newest.Ops))
	}
}

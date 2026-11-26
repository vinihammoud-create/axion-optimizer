package catalog_test

import (
	"context"
	"strings"
	"testing"

	"axion/internal/catalog"
	"axion/internal/collect"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// The history knows that a change belongs to one Windows account by the
// target of its operation. Every operation of the catalog that writes under
// HKEY_CURRENT_USER (or says it is per-user) has to be recognized, or its
// undo could run for the wrong account.
func TestPerUserOpsAreRecognized(t *testing.T) {
	_, f, _ := setup(t)
	old := catalog.GamePath
	catalog.GamePath = func(string) string { return `D:\Jogos\Jogo\jogo.exe` }
	defer func() { catalog.GamePath = old }()
	all := catalog.All()
	for _, feature := range []string{"priority", "gpu", "fso", "nvpstate"} {
		if tw := catalog.ProfileProvider(catalog.ProfileID("jogo.exe", feature), f); tw != nil {
			all = append(all, tw)
		}
	}
	seen := 0
	for _, tw := range all {
		for _, op := range tw.Ops(f) {
			perUser := tweaks.PerUserTarget(op.Target())
			if strings.Contains(strings.ToUpper(op.Describe()), `HKCU\`) && !perUser {
				t.Errorf("%s: a operação grava em HKCU e o alvo %q não é reconhecido como do usuário (tweaks.perUserPrefixes)", tw.ID, op.Target())
			}
			if fo, ok := op.(*tweaks.FuncOp); ok && fo.PerUser != perUser {
				t.Errorf("%s: FuncOp %q com PerUser=%v, mas o alvo é reconhecido como do usuário=%v", tw.ID, op.Target(), fo.PerUser, perUser)
			}
			if perUser {
				seen++
			}
		}
	}
	if seen < 10 {
		t.Errorf("só %d operações do usuário encontradas: o teste não está vendo o catálogo", seen)
	}
}

// hiberbootRun makes `powercfg /hibernate on` create HiberbootEnabled, as
// Windows may do when hibernation comes back.
type hiberbootRun struct {
	sys.Runner
	reg sys.Registry
}

const keySessionPowerTest = `HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Power`

func (r hiberbootRun) Run(ctx context.Context, exe string, args []string, opts sys.RunOpts) (sys.Result, error) {
	res, err := r.Runner.Run(ctx, exe, args, opts)
	if err == nil && res.ExitCode == 0 && len(args) == 2 && strings.EqualFold(args[0], "/hibernate") && strings.EqualFold(args[1], "on") {
		_ = r.reg.SetValue(keySessionPowerTest, "HiberbootEnabled", sys.DWord(1))
	}
	return res, err
}

// HiberbootEnabled that was not on the machine before the tweak is not left
// behind by the undo, even when turning hibernation back on creates it.
func TestHibernateUndoRemovesHiberbootItDidNotFind(t *testing.T) {
	m, _, e := setup(t)
	_ = m.R.DeleteValue(keySessionPowerTest, "HiberbootEnabled")
	m.Run = hiberbootRun{Runner: m.Run, reg: m.R}
	f := collect.Collect(context.Background(), m.Platform, nil)
	ctx := context.Background()
	if r := e.Apply(ctx, f, []string{"power.hibernate"}, tweaks.ApplyOpts{AllowBlocked: true}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	if r := e.RevertTweaks(ctx, f, []string{"power.hibernate"}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer: %+v", r[0])
	}
	if v, _ := m.R.GetValue(keySessionPowerTest, "HiberbootEnabled"); v.Exists {
		t.Errorf("HiberbootEnabled não existia antes e sobrou depois do desfazer: %+v", v)
	}
	if v, _ := m.R.GetValue(`HKLM\SYSTEM\CurrentControlSet\Control\Power`, "HibernateEnabled"); !v.Exists || v.Int != 1 {
		t.Errorf("a hibernação deveria ter voltado: %+v", v)
	}
}

// HibernateEnabled that cannot be read is not "hibernation off": the tweak
// is neither shown as already applied nor recorded as applied.
func TestHibernateUnreadableIsNotOff(t *testing.T) {
	m, _, e := setup(t)
	f := collect.Collect(context.Background(), m.Platform, nil)
	m.Reg = failingRegValue{Registry: m.R, name: "HibernateEnabled"}
	tw := e.Catalog.Get("power.hibernate")
	if st := e.StateOf(context.Background(), f, tw); st.Status == "on" {
		t.Errorf("leitura que falhou não pode mostrar o ajuste como aplicado: %+v", st)
	}
	r := e.Apply(context.Background(), f, []string{"power.hibernate"}, tweaks.ApplyOpts{AllowBlocked: true})
	if r[0].Outcome == tweaks.OutApplied || r[0].Outcome == tweaks.OutAlready {
		t.Errorf("sem ler o estado, o ajuste não pode sair como aplicado: %+v", r[0])
	}
	if v, _ := m.R.GetValue(`HKLM\SYSTEM\CurrentControlSet\Control\Power`, "HibernateEnabled"); !v.Exists || v.Int != 1 {
		t.Errorf("a hibernação foi alterada sem o estado ter sido lido: %+v", v)
	}
}

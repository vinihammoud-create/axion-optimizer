package tweaks_test

import (
	"context"
	"strings"
	"testing"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

func powerPlanTweak() *tweaks.Tweak {
	return &tweaks.Tweak{ID: "t.ultimate", Category: "energia", Title: "Desempenho Máximo", Evidence: tweaks.EvHigh, Risk: tweaks.RiskSafe,
		Ops: func(*facts.Facts) []tweaks.Op {
			return []tweaks.Op{tweaks.Scheme(sys.SchemeUltimateTemplate, "Desempenho Máximo")}
		}}
}

func schemeGUIDs(t *testing.T, m *mocksys.Mock) []string {
	t.Helper()
	list, err := m.Platform.Power.Schemes()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range list {
		out = append(out, strings.ToLower(s.GUID))
	}
	return out
}

func keysOutsideAxion(m *mocksys.Mock) int {
	return m.R.KeyCount() - m.R.KeyCountUnder(`HKLM\SOFTWARE\Axion`)
}

func assertWorldRestored(t *testing.T, before, after map[string]string) {
	t.Helper()
	if d := mocksys.DiffWorld(before, after, "hklm\\software\\axion"); len(d) > 0 {
		t.Errorf("o estado do PC simulado não voltou ao original:\n  %s", strings.Join(d, "\n  "))
	}
}

// The Ultimate Performance copy is created before its marker is written. If
// the marker cannot be written, no copy may stay behind (the undo only
// recognises copies by the marker) and the active plan must not change; once
// the write works again, the same tweak applies and reverts exactly.
func TestUltimatePlanMarkerWriteFailureLeavesNoCopy(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tw := powerPlanTweak()
	e := newEngine(t, m, tw)
	ctx, f := context.Background(), &facts.Facts{}
	before := m.World()
	plansBefore := schemeGUIDs(t, m)
	activeBefore, _ := m.Platform.Power.ActiveScheme()

	m.R.FailWrites[`HKLM\SOFTWARE\Axion\Power`] = true
	res := e.Apply(ctx, f, []string{"t.ultimate"}, tweaks.ApplyOpts{})
	if res[0].Outcome != tweaks.OutFailed {
		t.Fatalf("esperava falha ao gravar o marcador: %+v", res[0])
	}
	if got := schemeGUIDs(t, m); strings.Join(got, ",") != strings.Join(plansBefore, ",") {
		t.Errorf("a cópia do plano ficou para trás: antes %v, depois %v", plansBefore, got)
	}
	if act, _ := m.Platform.Power.ActiveScheme(); !strings.EqualFold(act, activeBefore) {
		t.Errorf("plano ativo mudou: %s -> %s", activeBefore, act)
	}
	assertWorldRestored(t, before, m.World())
	if st := e.StateOf(ctx, f, tw); st.Status == "on" || st.PendingUndo {
		t.Errorf("falha não pode deixar o ajuste ativo nem com desfazer pendente: %+v", st)
	}

	delete(m.R.FailWrites, `HKLM\SOFTWARE\Axion\Power`)
	if res = e.Apply(ctx, f, []string{"t.ultimate"}, tweaks.ApplyOpts{}); res[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar depois da falha: %+v", res[0])
	}
	if rev := e.RevertTweaks(ctx, f, []string{"t.ultimate"}, tweaks.RevertOpts{}); rev[0].Outcome != tweaks.OutReverted {
		t.Fatalf("reverter: %+v", rev[0])
	}
	if got := schemeGUIDs(t, m); strings.Join(got, ",") != strings.Join(plansBefore, ",") {
		t.Errorf("a reversão deixou a cópia do plano: antes %v, depois %v", plansBefore, got)
	}
	assertWorldRestored(t, before, m.World())
}

// A REG_NONE value (type 0, with or without data) must come back with the
// same type and bytes after the undo, not as REG_BINARY or as absent.
func TestRegNoneValueRoundTrip(t *testing.T) {
	cases := map[string]sys.RegValue{
		"sem dados": {Exists: true, Kind: sys.RegNone},
		"com dados": {Exists: true, Kind: sys.RegNone, Bin: []byte{0xDE, 0xAD, 0xBE, 0xEF}},
	}
	for name, orig := range cases {
		orig := orig
		t.Run(name, func(t *testing.T) {
			m, err := mocksys.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := m.R.SetValue(testKey, "None", orig); err != nil {
				t.Fatal(err)
			}
			tw := regTweak("t.none", tweaks.Reg(testKey, "None", sys.DWord(7)))
			e := newEngine(t, m, tw)
			ctx, f := context.Background(), &facts.Facts{}
			before := m.World()
			if r := e.Apply(ctx, f, []string{"t.none"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
				t.Fatalf("aplicar: %+v", r[0])
			}
			if v, _ := m.R.GetValue(testKey, "None"); !v.Equal(sys.DWord(7)) {
				t.Fatalf("valor após aplicar: %+v", v)
			}
			if r := e.RevertTweaks(ctx, f, []string{"t.none"}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
				t.Fatalf("reverter: %+v", r[0])
			}
			v, _ := m.R.GetValue(testKey, "None")
			if !v.Exists || v.Kind != sys.RegNone || string(v.Bin) != string(orig.Bin) {
				t.Errorf("REG_NONE não voltou igual: antes %+v, depois %+v", orig, v)
			}
			assertWorldRestored(t, before, m.World())
		})
	}
}

// Two tweaks write into a key neither found on the machine. Whichever order
// they are undone in, the key Axion created must be gone at the end.
func TestSharedNewKeyBetweenTweaks(t *testing.T) {
	const shared = testKey + `\Novo\Compartilhada`
	orders := map[string]struct {
		ids  []string
		skip string
	}{
		"primeiro A, depois B": {ids: []string{"t.a", "t.b"}, skip: "achado: desfazer A e depois B deixa 3 chaves vazias no registro (AxionTest, Novo e Compartilhada); a chave só some se B for desfeito antes de A (internal/tweaks/ops.go, RegOp.Capture/Restore)"},
		"primeiro B, depois A": {ids: []string{"t.b", "t.a"}},
	}
	for name, tc := range orders {
		tc := tc
		t.Run(name, func(t *testing.T) {
			if tc.skip != "" {
				t.Skip(tc.skip)
			}
			m, err := mocksys.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			a := regTweak("t.a", tweaks.Reg(shared, "A", sys.DWord(1)))
			b := regTweak("t.b", tweaks.Reg(shared, "B", sys.DWord(2)))
			e := newEngine(t, m, a, b)
			ctx, f := context.Background(), &facts.Facts{}
			before, keysBefore := m.World(), keysOutsideAxion(m)
			if r := e.Apply(ctx, f, []string{"t.a", "t.b"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied || r[1].Outcome != tweaks.OutApplied {
				t.Fatalf("aplicar: %+v", r)
			}
			for _, id := range tc.ids {
				if r := e.RevertTweaks(ctx, f, []string{id}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
					t.Fatalf("reverter %s: %+v", id, r[0])
				}
			}
			assertWorldRestored(t, before, m.World())
			if kc := keysOutsideAxion(m); kc != keysBefore {
				t.Errorf("a reversão deixou chaves criadas pelo Axion: %d -> %d", keysBefore, kc)
			}
		})
	}
}

package tweaks_test

import (
	"context"
	"testing"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

// A value whose bytes change on every write (StartupApproved carries the
// FILETIME of the change) is in target by meaning, not by bytes, and the undo
// still brings back the exact bytes.
func TestRegMatchInTarget(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	const key = testKey + `\Approved`
	const name = "Item"
	disabled := func(v sys.RegValue) bool { return v.Exists && len(v.Bin) > 0 && v.Bin[0]&1 == 1 }
	op := tweaks.RegMatch(key, name, sys.Binary([]byte{3, 0, 0, 0, 9, 9, 9, 9, 9, 9, 9, 9}), disabled)
	ctx, f := context.Background(), &facts.Facts{}
	c := &tweaks.Ctx{Context: ctx, P: m.Platform, F: f}

	_ = m.R.SetValue(key, name, sys.Binary([]byte{3, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}))
	if in, err := op.InTarget(c); err != nil || !in {
		t.Fatalf("bytes de outra data deveriam contar como no alvo: %v %v", in, err)
	}
	_ = m.R.SetValue(key, name, sys.Binary([]byte{2, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}))
	if in, err := op.InTarget(c); err != nil || in {
		t.Fatalf("valor ligado não pode contar como no alvo: %v %v", in, err)
	}

	// Through the engine: the value is there (enabled) and the key too.
	orig := sys.Binary([]byte{2, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8})
	tw := regTweak("t.match", op)
	e := newEngine(t, m, tw)
	if r := e.Apply(ctx, f, []string{"t.match"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	if v, _ := m.R.GetValue(key, name); !disabled(v) {
		t.Fatalf("valor gravado: %+v", v)
	}
	// Another time stamp: still in target, no drift.
	_ = m.R.SetValue(key, name, sys.Binary([]byte{3, 0, 0, 0, 7, 7, 7, 7, 7, 7, 7, 7}))
	if st := e.StateOf(ctx, f, tw); st.Status != "on" || st.Drifted {
		t.Fatalf("mesmo estado com outra data não é deriva: %+v", st)
	}
	_ = m.R.SetValue(key, name, sys.Binary([]byte{3, 0, 0, 0, 9, 9, 9, 9, 9, 9, 9, 9}))
	if r := e.RevertAll(ctx, f, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer: %+v", r[0])
	}
	if v, _ := m.R.GetValue(key, name); !v.Equal(orig) {
		t.Fatalf("o valor deveria voltar byte a byte: %+v", v)
	}

	// Absent before: absent after, and the key Axion created is pruned.
	m2, _ := mocksys.New(t.TempDir())
	keysBefore := m2.R.KeyCount()
	op2 := tweaks.RegMatch(testKey+`\Novo\Sub`, name, sys.Binary([]byte{3, 0, 0, 0, 9, 9, 9, 9, 9, 9, 9, 9}), disabled)
	e2 := newEngine(t, m2, regTweak("t.match2", op2))
	if r := e2.Apply(ctx, f, []string{"t.match2"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	if r := e2.RevertAll(ctx, f, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer: %+v", r[0])
	}
	if v, _ := m2.R.GetValue(testKey+`\Novo\Sub`, name); v.Exists {
		t.Fatalf("o valor deveria ter voltado a não existir: %+v", v)
	}
	if kc := m2.R.KeyCount() - m2.R.KeyCountUnder(`HKLM\SOFTWARE\Axion`); kc > keysBefore {
		t.Fatalf("a chave criada deveria ser podada: %d -> %d", keysBefore, kc)
	}
}

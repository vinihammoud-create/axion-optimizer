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

// A crash in the middle of a tweak must leave the previous values on disk,
// so the change can be undone after the program restarts.
func TestWriteAheadSurvivesCrash(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const key = `HKCU\Software\AxionTest`
	_ = m.R.SetValue(key, "A", sys.DWord(1))
	_ = m.R.SetValue(key, "B", sys.DWord(1))
	crash := &tweaks.FuncOp{KindName: "crash", ID: "b", Desc: "grava B e o processo morre",
		SnapFn:  func(c *tweaks.Ctx) (any, error) { return c.P.Reg.GetValue(key, "B") },
		InFn:    func(c *tweaks.Ctx) (bool, error) { v, _ := c.P.Reg.GetValue(key, "B"); return v.Int == 9, nil },
		ApplyFn: func(c *tweaks.Ctx) error { _ = c.P.Reg.SetValue(key, "B", sys.DWord(9)); panic("queda de energia") },
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var v sys.RegValue
			if err := json.Unmarshal(raw, &v); err != nil {
				return err
			}
			return c.P.Reg.SetValue(key, "B", v)
		},
		CurFn: func(*tweaks.Ctx) string { return "" },
	}
	tw := &tweaks.Tweak{ID: "test.crash", Category: "jogo", Title: "Teste", Evidence: tweaks.EvHigh, Risk: tweaks.RiskSafe,
		Ops: func(*facts.Facts) []tweaks.Op {
			return []tweaks.Op{tweaks.Reg(key, "A", sys.DWord(5)), crash}
		}}
	cat, err := tweaks.NewCatalog([]*tweaks.Tweak{tw})
	if err != nil {
		t.Fatal(err)
	}
	j, _ := tweaks.OpenJournal(m.DataDir, m.Key)
	e := tweaks.NewEngine(m.Platform, cat, j)
	func() {
		defer func() { _ = recover() }()
		e.Apply(context.Background(), &facts.Facts{}, []string{"test.crash"}, tweaks.ApplyOpts{})
	}()
	if v, _ := m.R.GetValue(key, "A"); v.Int != 5 {
		t.Fatalf("pré-condição: A deveria ter sido gravado (%d)", v.Int)
	}

	// "Restart": a new journal read from disk.
	j2, err := tweaks.OpenJournal(m.DataDir, m.Key)
	if err != nil {
		t.Fatal(err)
	}
	if n := j2.RecoverInterrupted(); n != 1 {
		t.Fatalf("esperava 1 aplicação interrompida, obtido %d", n)
	}
	e2 := tweaks.NewEngine(m.Platform, cat, j2)
	st := e2.StateOf(context.Background(), &facts.Facts{}, tw)
	if !st.ByAxion {
		t.Errorf("entrada interrompida deveria contar como alteração do Axion")
	}
	res := e2.RevertAll(context.Background(), &facts.Facts{}, tweaks.RevertOpts{})
	if len(res) != 1 || res[0].Outcome != tweaks.OutReverted {
		t.Fatalf("reversão após interrupção: %+v", res)
	}
	a, _ := m.R.GetValue(key, "A")
	b, _ := m.R.GetValue(key, "B")
	if a.Int != 1 || b.Int != 1 {
		t.Fatalf("estado original não restaurado: A=%d B=%d", a.Int, b.Int)
	}
}

func TestFailedOpRollsBack(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	const key = `HKCU\Software\AxionTest`
	_ = m.R.SetValue(key, "A", sys.DWord(1))
	fail := &tweaks.FuncOp{KindName: "fail", ID: "x", Desc: "sempre falha",
		SnapFn:    func(*tweaks.Ctx) (any, error) { return 0, nil },
		InFn:      func(*tweaks.Ctx) (bool, error) { return false, nil },
		ApplyFn:   func(*tweaks.Ctx) error { return errors.New("acesso negado") },
		RestoreFn: func(*tweaks.Ctx, json.RawMessage) error { return nil },
		CurFn:     func(*tweaks.Ctx) string { return "" },
	}
	tw := &tweaks.Tweak{ID: "test.fail", Category: "jogo", Title: "Teste", Evidence: tweaks.EvHigh, Risk: tweaks.RiskSafe,
		Ops: func(*facts.Facts) []tweaks.Op { return []tweaks.Op{tweaks.Reg(key, "A", sys.DWord(5)), fail} }}
	cat, _ := tweaks.NewCatalog([]*tweaks.Tweak{tw})
	j, _ := tweaks.OpenJournal(m.DataDir, m.Key)
	e := tweaks.NewEngine(m.Platform, cat, j)
	res := e.Apply(context.Background(), &facts.Facts{}, []string{"test.fail"}, tweaks.ApplyOpts{})
	if res[0].Outcome != tweaks.OutFailed {
		t.Fatalf("esperava falha: %+v", res[0])
	}
	if v, _ := m.R.GetValue(key, "A"); v.Int != 1 {
		t.Fatalf("a primeira operação deveria ter sido desfeita, A=%d", v.Int)
	}
	if len(j.Active()) != 0 {
		t.Fatalf("falha não pode deixar entrada ativa")
	}
	ents := j.Snapshot()
	if len(ents) != 1 || ents[0].Status != tweaks.StatusFailed {
		t.Fatalf("histórico da falha: %+v", ents)
	}
}

// A journal edited outside Axion (or written before 0.3) is never trusted
// to write arbitrary registry values with administrator rights: only what
// the current catalog describes is undone, corrections are never undone.
func TestUnverifiedJournalIsContained(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const key = `HKCU\Software\AxionTest`
	_ = m.R.SetValue(key, "A", sys.DWord(1))
	_ = m.R.SetValue(key, "C", sys.DWord(1))
	game := &tweaks.Tweak{ID: "test.game", Category: "jogo", Title: "Jogo", Evidence: tweaks.EvHigh, Risk: tweaks.RiskSafe,
		Ops: func(*facts.Facts) []tweaks.Op { return []tweaks.Op{tweaks.Reg(key, "A", sys.DWord(5))} }}
	fix := &tweaks.Tweak{ID: "test.fix", Category: "correcoes", Title: "Correção", Evidence: tweaks.EvHigh, Risk: tweaks.RiskSafe,
		Ops: func(*facts.Facts) []tweaks.Op { return []tweaks.Op{tweaks.Reg(key, "C", sys.DWord(0))} }}
	cat, _ := tweaks.NewCatalog([]*tweaks.Tweak{game, fix})
	j, _ := tweaks.OpenJournal(m.DataDir, m.Key)
	e := tweaks.NewEngine(m.Platform, cat, j)
	for _, id := range []string{"test.game", "test.fix"} {
		if r := e.Apply(context.Background(), &facts.Facts{}, []string{id}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
			t.Fatalf("%s: %+v", id, r[0])
		}
	}
	// Reopened with the same key: everything is verified.
	j2, _ := tweaks.OpenJournal(m.DataDir, m.Key)
	for _, en := range j2.Entries() {
		if !en.Verified() {
			t.Fatalf("entrada da própria instalação não verificada: %+v", en)
		}
	}
	// Someone edits the journal: the game entry gets an extra operation that
	// points at an Image File Execution Options debugger.
	path := filepath.Join(m.DataDir, "journal.json")
	raw, _ := os.ReadFile(path)
	var doc map[string]any
	_ = json.Unmarshal(raw, &doc)
	ifeo := `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options\sethc.exe`
	for _, x := range doc["entries"].([]any) {
		en := x.(map[string]any)
		if en["tweakId"] == "test.game" {
			ops := en["ops"].([]any)
			ops = append(ops, map[string]any{"kind": "registry", "target": "reg:" + ifeo + "|Debugger", "describe": "forjado",
				"before": json.RawMessage(`{"exists":true,"kind":1,"str":"C:\\evil.exe"}`), "after": json.RawMessage(`{"exists":false}`)})
			en["ops"] = ops
		}
		if en["tweakId"] == "test.fix" {
			delete(en, "mac") // as if written by an older version
		}
	}
	raw, _ = json.Marshal(doc)
	_ = os.WriteFile(path, raw, 0o644)
	j3, _ := tweaks.OpenJournal(m.DataDir, m.Key)
	e3 := tweaks.NewEngine(m.Platform, cat, j3)
	res := e3.RevertAll(context.Background(), &facts.Facts{}, tweaks.RevertOpts{Force: true})
	if ok, _ := m.R.KeyExists(ifeo); ok {
		t.Fatal("registro forjado gravou um depurador em IFEO com direitos de administrador")
	}
	if v, _ := m.R.GetValue(key, "C"); v.Int != 0 {
		t.Errorf("correção desfeita a partir de registro não verificado (C=%d)", v.Int)
	}
	for _, r := range res {
		if r.Outcome == tweaks.OutReverted {
			t.Errorf("reversão aceita de registro adulterado: %+v", r)
		}
		if r.TweakID == "test.fix" && !strings.Contains(r.Message, "Correções") {
			t.Errorf("motivo da recusa da correção: %q", r.Message)
		}
	}
	// An untouched entry from an older version (no MAC at all) still undoes
	// what the catalog describes.
	var doc2 map[string]any
	raw, _ = os.ReadFile(path)
	_ = json.Unmarshal(raw, &doc2)
	var keep []any
	for _, x := range doc2["entries"].([]any) {
		en := x.(map[string]any)
		delete(en, "mac")
		if en["tweakId"] == "test.game" {
			ops := en["ops"].([]any)
			en["ops"] = ops[:1]
		}
		keep = append(keep, en)
	}
	doc2["entries"] = keep
	raw, _ = json.Marshal(doc2)
	_ = os.WriteFile(path, raw, 0o644)
	j4, _ := tweaks.OpenJournal(m.DataDir, m.Key)
	e4 := tweaks.NewEngine(m.Platform, cat, j4)
	var gameRes tweaks.Result
	for _, r := range e4.RevertAll(context.Background(), &facts.Facts{}, tweaks.RevertOpts{}) {
		if r.TweakID == "test.game" {
			gameRes = r
		}
	}
	if gameRes.Outcome != tweaks.OutReverted {
		t.Fatalf("entrada antiga de ajuste do catálogo deveria ser desfeita: %+v", gameRes)
	}
	if v, _ := m.R.GetValue(key, "A"); v.Int != 1 {
		t.Errorf("A não voltou ao valor anterior: %d", v.Int)
	}
}

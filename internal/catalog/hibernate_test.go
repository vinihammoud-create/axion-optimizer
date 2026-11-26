package catalog_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"axion/internal/collect"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

// Hibernation chosen by the user: a reduced hiberfile or one with a size of
// its own. Undoing power.hibernate has to bring the same file back, not the
// default one that `powercfg /hibernate on` creates.
func TestHibernateRoundTripKeepsTypeAndSize(t *testing.T) {
	cases := map[string][][]string{
		"tipo reduzido":   {{"/hibernate", "/size", "0"}, {"/hibernate", "/type", "reduced"}},
		"tamanho próprio": {{"/hibernate", "/size", "75"}},
		"tipo completo":   {{"/hibernate", "/type", "full"}},
	}
	for name, cmds := range cases {
		cmds := cmds
		t.Run(name, func(t *testing.T) {
			m, _, e := setup(t)
			for _, args := range cmds {
				res, err := m.Platform.Run.Run(context.Background(), "powercfg.exe", args, sysRunOpts)
				if err != nil || res.ExitCode != 0 {
					t.Fatalf("powercfg %v: %v %+v", args, err, res)
				}
			}
			f := collect.Collect(context.Background(), m.Platform, nil)
			roundTrip(t, m, f, e, e.Catalog.Get("power.hibernate"))
		})
	}
}

// A custom size with no HiberFileType recorded (set outside powercfg, or by a
// Windows that does not write the type). The documented rule says a custom
// size is a full hiberfile, so the undo must give the size back, never turn
// the file into a reduced one with the size managed by the OS.
func TestHibernateUndoKeepsCustomSizeWithoutType(t *testing.T) {
	m, _, e := setup(t)
	const key = `HKLM\SYSTEM\CurrentControlSet\Control\Power`
	_ = m.R.SetValue(key, "HibernateEnabled", sys.DWord(1))
	_ = m.R.DeleteValue(key, "HiberFileType")
	_ = m.R.SetValue(key, "HiberFileSizePercent", sys.DWord(30))
	f := collect.Collect(context.Background(), m.Platform, nil)
	ctx := context.Background()
	if r := e.Apply(ctx, f, []string{"power.hibernate"}, tweaks.ApplyOpts{AllowBlocked: true}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	if r := e.RevertTweaks(ctx, f, []string{"power.hibernate"}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer: %+v", r[0])
	}
	sz, _ := m.R.GetValue(key, "HiberFileSizePercent")
	ft, _ := m.R.GetValue(key, "HiberFileType")
	if !sz.Exists || sz.Int != 30 {
		t.Errorf("o tamanho próprio de 30%% não voltou: %+v", sz)
	}
	if ft.Exists && ft.Int == 1 {
		t.Error("o desfazer transformou um arquivo de tamanho próprio em reduzido")
	}
}

// An entry applied by a version that only kept on/off (no "fileType" and
// "sizePercent" in the snapshots) must still be undone normally, not taken as
// changed by another program because the new snapshot has more fields.
func TestHibernateUndoOfEntryFromBeforeTypeAndSize(t *testing.T) {
	m, _, e := setup(t)
	for _, args := range [][]string{{"/hibernate", "/size", "0"}, {"/hibernate", "/type", "reduced"}} {
		if res, err := m.Platform.Run.Run(context.Background(), "powercfg.exe", args, sysRunOpts); err != nil || res.ExitCode != 0 {
			t.Fatalf("powercfg %v: %v %+v", args, err, res)
		}
	}
	f := collect.Collect(context.Background(), m.Platform, nil)
	ctx := context.Background()
	if r := e.Apply(ctx, f, []string{"power.hibernate"}, tweaks.ApplyOpts{AllowBlocked: true}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}

	path := filepath.Join(m.DataDir, "journal.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, x := range doc["entries"].([]any) {
		en := x.(map[string]any)
		delete(en, "mac")
		for _, o := range en["ops"].([]any) {
			op := o.(map[string]any)
			for _, k := range []string{"before", "after"} {
				if s, ok := op[k].(map[string]any); ok {
					delete(s, "fileType")
					delete(s, "sizePercent")
				}
			}
		}
	}
	raw, _ = json.Marshal(doc)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	j, err := tweaks.OpenJournal(m.DataDir, m.Key)
	if err != nil {
		t.Fatal(err)
	}
	e = tweaks.NewEngine(m.Platform, e.Catalog, j)

	r := e.RevertTweaks(ctx, f, []string{"power.hibernate"}, tweaks.RevertOpts{})
	if r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("entrada antiga devia desfazer como antes: %+v", r[0])
	}
	if v, _ := m.R.GetValue(`HKLM\SYSTEM\CurrentControlSet\Control\Power`, "HibernateEnabled"); !v.Exists || v.Int != 1 {
		t.Errorf("a hibernação não voltou: %+v", v)
	}
}

// The simulated powercfg follows the documented rule: a hiberfile with a
// custom size cannot become reduced before the OS manages the size again.
func TestMockPowercfgRefusesReducedWithCustomSize(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) int {
		res, err := m.Platform.Run.Run(context.Background(), "powercfg.exe", args, sysRunOpts)
		if err != nil {
			t.Fatalf("powercfg %v: %v", args, err)
		}
		return res.ExitCode
	}
	if code := run("/hibernate", "/size", "80"); code != 0 {
		t.Fatalf("/size 80 deveria funcionar, saída %d", code)
	}
	if code := run("/hibernate", "/type", "reduced"); code == 0 {
		t.Error("/type reduced deveria falhar enquanto o tamanho é próprio")
	}
	if code := run("/hibernate", "/size", "0"); code != 0 {
		t.Fatalf("/size 0 deveria funcionar, saída %d", code)
	}
	if code := run("/hibernate", "/type", "reduced"); code != 0 {
		t.Errorf("/type reduced depois de /size 0 deveria funcionar, saída %d", code)
	}
}

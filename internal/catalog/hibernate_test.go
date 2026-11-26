package catalog_test

import (
	"context"
	"testing"

	"axion/internal/collect"
	"axion/internal/sys/mocksys"
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

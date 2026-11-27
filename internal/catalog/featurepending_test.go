package catalog_test

import (
	"context"
	"strings"
	"testing"

	"axion/internal/collect"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// G-021: DISM disables an optional feature with 3010 (restart required) and
// Windows may keep reporting it as enabled until the restart. The change was
// accepted: the engine must not read it as failed and undo it right away.
func TestFeatureDisablePendingRestartIsNotRolledBack(t *testing.T) {
	m, _, e := setup(t)
	m.W.Set(`root\cimv2`, "Win32_OptionalFeature", sys.WMIRow{"Name": "VirtualMachinePlatform", "InstallState": int64(1)})
	prev := m.X.Handler
	m.X.Handler = func(exe string, args []string, ps string) (sys.Result, bool) {
		if strings.EqualFold(exe, "dism.exe") && strings.Contains(strings.ToLower(strings.Join(args, " ")), "/featurename:virtualmachineplatform") {
			return sys.Result{ExitCode: 3010}, true // accepted, state unchanged until restart
		}
		return prev(exe, args, ps)
	}
	f := collect.Collect(context.Background(), m.Platform, nil)
	tw := e.Catalog.Get("adv.vmp.off")
	if tw == nil {
		t.Fatal("adv.vmp.off não está no catálogo")
	}
	if ok, why := tw.IsApplicable(f); !ok {
		t.Fatalf("deveria se aplicar: %s", why)
	}
	res := e.Apply(context.Background(), f, []string{tw.ID}, tweaks.ApplyOpts{AllowBlocked: true})
	if len(res) != 1 || res[0].Outcome != tweaks.OutApplied {
		t.Fatalf("desativação aceita pelo DISM (3010) tratada como falha: %+v", res)
	}
	for _, c := range m.X.Recorded() {
		if strings.EqualFold(c.Exe, "dism.exe") && strings.Contains(strings.ToLower(strings.Join(c.Args, " ")), "/enable-feature") {
			t.Fatalf("o motor desfez a desativação logo depois de aplicar: %v", c.Args)
		}
	}
	if st := e.StateOf(context.Background(), f, tw); st.Status != "on" {
		t.Errorf("estado com o reinício pendente = %s, quero on", st.Status)
	}
}

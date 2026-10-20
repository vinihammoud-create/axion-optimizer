package catalog_test

import (
	"context"
	"strings"
	"testing"

	"axion/internal/sys"
	"axion/internal/tweaks"
)

// The power mode is read and written for the power source in use. Applied on
// AC, it cannot be undone on battery (that would write the battery's mode and
// leave the AC one changed): the undo waits for the charger, then restores.
func TestPowerModeUndoNeedsAC(t *testing.T) {
	m, f, e := setup(t)
	m.N.HasBattery = true
	r := e.Apply(context.Background(), f, []string{"power.mode.best"}, tweaks.ApplyOpts{})
	if r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar na tomada: %+v", r[0])
	}
	if o, _ := m.P.ActiveOverlay(); o != sys.OverlayBestPerformance {
		t.Fatalf("modo: %q", o)
	}
	m.N.OnBattery = true
	r = e.RevertTweaks(context.Background(), f, []string{"power.mode.best"}, tweaks.RevertOpts{})
	if r[0].Outcome == tweaks.OutReverted || !strings.Contains(r[0].Message, "carregador") {
		t.Fatalf("desfazer na bateria deveria esperar o carregador: %+v", r[0])
	}
	if o, _ := m.P.ActiveOverlay(); o != sys.OverlayBestPerformance {
		t.Fatalf("o modo mudou na bateria: %q", o)
	}
	m.N.OnBattery = false
	r = e.RevertTweaks(context.Background(), f, []string{"power.mode.best"}, tweaks.RevertOpts{})
	if r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer na tomada: %+v", r[0])
	}
	if o, _ := m.P.ActiveOverlay(); o != "" {
		t.Fatalf("modo depois de desfazer: %q", o)
	}
}

//go:build windows

package winsys

import "testing"

// Read-only call on this machine: CallNtPowerInformation answers with one
// entry per logical processor, a maximum frequency and a limit no higher
// than it.
func TestProcessorPowerReal(t *testing.T) {
	procs, err := (&Native{}).ProcessorPower()
	if err != nil {
		t.Fatal(err)
	}
	if len(procs) == 0 {
		t.Fatal("nenhum processador")
	}
	if procs[0].MaxMhz == 0 {
		// Some virtual machines (CI runners) do not report frequencies.
		t.Skipf("frequência não informada nesta máquina: %+v", procs[0])
	}
	for i, p := range procs {
		if p.MaxMhz <= 0 || p.MhzLimit <= 0 || p.MhzLimit > p.MaxMhz {
			t.Errorf("processador %d: %+v", i, p)
		}
	}
	t.Logf("%d processadores, %+v", len(procs), procs[0])
}

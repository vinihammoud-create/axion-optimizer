package perf

import "testing"

// The share of readings with the thermal limit below the maximum; readings
// without the limit (0 or -1) do not count, so old samples stay quiet.
func TestSummarizeCPUThermalLimit(t *testing.T) {
	ss := []CPUSample{{FreqPct: 100, UtilPct: 50}, {FreqPct: 100, UtilPct: 50}, {FreqPct: 100, UtilPct: 50}}
	if c := SummarizeCPU(ss); c.LimitSamples != 0 || c.ThermalLimitPct != 0 {
		t.Fatalf("sem leitura de limite: %+v", c)
	}
	ss = []CPUSample{{FreqPct: 90, LimitPct: 100}, {FreqPct: 80, LimitPct: 75}, {FreqPct: 85, LimitPct: 100}, {FreqPct: 70, LimitPct: 80}, {FreqPct: 60, LimitPct: -1}}
	c := SummarizeCPU(ss)
	if c.LimitSamples != 4 || c.ThermalLimitPct != 50 || c.MinLimitPct != 75 {
		t.Fatalf("%+v", c)
	}
}

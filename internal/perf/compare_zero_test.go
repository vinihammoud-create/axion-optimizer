package perf

import "testing"

// Com a média de A igual a zero o percentual não existe: a diferença não pode
// ser tratada como pequena por causa dele.
func TestCompareZeroBaseline(t *testing.T) {
	mk := func(vals ...float64) []Summary {
		var out []Summary
		for _, v := range vals {
			out = append(out, Summary{AvgFPS: -1, Low1FPS: -1, Low01FPS: -1, FTP99: -1, FTStdDev: -1, StutterPct: v, InputLatencyAvg: -1, PCLatencyAvg: -1})
		}
		return out
	}
	stutter := func(c Comparison) Row {
		for _, r := range c.Rows {
			if r.Key == "stutterPct" {
				return r
			}
		}
		t.Fatal("linha de engasgos ausente")
		return Row{}
	}

	if r := stutter(Compare("antes", mk(0, 0, 0), "depois", mk(5, 6, 5))); r.Verdict == "igual" {
		t.Errorf("0%% → ~5%% de engasgos saiu %q (delta=%v p=%v pAdj=%v)", r.Verdict, r.Delta, r.P, r.PAdj)
	} else if r.Verdict != "pior" {
		t.Errorf("0%% → ~5%% de engasgos: esperado \"pior\", veio %q", r.Verdict)
	}
	if r := stutter(Compare("antes", mk(0, 0, 0), "depois", mk(0, 0, 0))); r.Verdict != "igual" {
		t.Errorf("0 x 0: %q", r.Verdict)
	}
	if r := stutter(Compare("antes", mk(0, 0, 0), "depois", mk(0.01, 0.02, 0.01))); r.Verdict != "igual" {
		t.Errorf("diferença absoluta abaixo do mínimo: %q", r.Verdict)
	}
}

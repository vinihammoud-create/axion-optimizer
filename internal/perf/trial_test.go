package perf

import (
	"reflect"
	"testing"
	"time"
)

func TestTrialVerdict(t *testing.T) {
	cases := []struct {
		name string
		a, b []Summary
		cond bool
		want string
	}{
		{"melhor", sums(400, 404, 398), sums(430, 428, 433), false, TrialBetter},
		{"melhor com condição mudada", sums(400, 404, 398), sums(430, 428, 433), true, TrialInconclusive},
		{"pior", sums(430, 428, 433), sums(400, 404, 398), false, TrialWorse},
		{"igual", sums(400, 420, 390), sums(405, 395, 415), false, TrialSame},
		{"igual com condição mudada", sums(400, 420, 390), sums(405, 395, 415), true, TrialInconclusive},
		{"uma medição de cada lado", sums(400), sums(440), false, TrialInconclusive},
	}
	for _, tc := range cases {
		c := Compare("antes", tc.a, "depois", tc.b)
		if got := TrialVerdict(c, tc.cond); got != tc.want {
			t.Errorf("%s: %s, esperado %s", tc.name, got, tc.want)
		}
	}

	// Misto: o FPS médio melhora e o 1% low piora ao mesmo tempo.
	mk := func(avg, low1 float64) Summary {
		return Summary{AvgFPS: avg, Low1FPS: low1, Low01FPS: -1, FTP99: -1, FTStdDev: -1, StutterPct: -1, InputLatencyAvg: -1, PCLatencyAvg: -1}
	}
	a := []Summary{mk(400, 200), mk(404, 202), mk(398, 199)}
	b := []Summary{mk(430, 180), mk(428, 181), mk(433, 179)}
	if got := TrialVerdict(Compare("antes", a, "depois", b), false); got != TrialMixed {
		t.Errorf("misto: %s", got)
	}
}

func TestTrialMetricsOrder(t *testing.T) {
	c := Compare("antes", sums(400, 404, 398), "depois", sums(430, 428, 433))
	var keys []string
	for _, m := range TrialMetrics(c) {
		keys = append(keys, m.Key)
	}
	if want := []string{"low1Fps", "avgFps", "ftP99"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("ordem: %v", keys)
	}
	// Só as presentes: sem 1% low nem P99.
	only := func(v float64) Summary {
		return Summary{AvgFPS: v, Low1FPS: -1, Low01FPS: -1, FTP99: -1, FTStdDev: -1, StutterPct: -1, InputLatencyAvg: -1, PCLatencyAvg: -1}
	}
	c = Compare("antes", []Summary{only(400), only(404)}, "depois", []Summary{only(430), only(428)})
	ms := TrialMetrics(c)
	if len(ms) != 1 || ms[0].Key != "avgFps" {
		t.Errorf("só as presentes: %+v", ms)
	}
}

func TestConditionsDiffer(t *testing.T) {
	base := func() *Conditions {
		return &Conditions{PowerScheme: "Equilibrado", PowerMode: "Equilibrado", DisplayHz: 144}
	}
	mut := func(f func(c *Conditions)) *Conditions { c := base(); f(c); return c }
	cases := []struct {
		name string
		a, b *Conditions
		want bool
	}{
		{"igual", base(), base(), false},
		{"bateria", base(), mut(func(c *Conditions) { c.OnBattery = true }), true},
		{"plano", base(), mut(func(c *Conditions) { c.PowerScheme = "Alto desempenho" }), true},
		{"modo", base(), mut(func(c *Conditions) { c.PowerMode = "Melhor desempenho" }), true},
		{"economia", base(), mut(func(c *Conditions) { c.PowerSaver = true }), true},
		{"Hz", base(), mut(func(c *Conditions) { c.DisplayHz = 240 }), true},
		{"Hz desconhecido", base(), mut(func(c *Conditions) { c.DisplayHz = 0 }), false},
		{"só programas ocupados", base(), mut(func(c *Conditions) { c.Busy = []Busy{{Name: "x.exe", Pct: 80}} }), false},
		{"nil", nil, base(), false},
		{"nil depois", base(), nil, false},
	}
	for _, tc := range cases {
		if got := ConditionsDiffer(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
}

func TestTrialTagSurvivesSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	out := &TrialOutcome{ID: "prova-0123456789ab", TweakID: "power.plan.high", Verdict: TrialSame, Action: TrialAutoUndone,
		Metrics: []TrialMetric{{Key: "low1Fps", Label: "1% low", Unit: "FPS", Higher: true, A: 200, B: 201, DeltaPct: 0.5, PAdj: 0.9, Verdict: "igual"}}}
	run := &Run{ID: NewID(time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local)), Label: "antes (prova)", Process: "cs2.exe", Started: time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local),
		Trial: &TrialTag{ID: "prova-0123456789ab", TweakID: "power.plan.high", TweakTitle: "Plano Alto desempenho", Phase: TrialPhaseBefore, Seq: 1, Outcome: out}}
	if err := Save(dir, run); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Trial, run.Trial) {
		t.Errorf("Load: %+v", got.Trial)
	}
	l := List(dir)
	if len(l) != 1 || !reflect.DeepEqual(l[0].Trial, run.Trial) {
		t.Errorf("List: %+v", l)
	}
	// Medição comum: o campo não aparece no JSON.
	plain := &Run{ID: NewID(time.Date(2026, 10, 1, 12, 5, 0, 0, time.Local)), Process: "cs2.exe"}
	if err := Save(dir, plain); err != nil {
		t.Fatal(err)
	}
	if g, _ := Load(dir, plain.ID); g == nil || g.Trial != nil {
		t.Errorf("medição comum: %+v", g)
	}
}

package perf

import (
	"fmt"
	"math"
	"sort"
)

// Comparison of two groups of runs (e.g. before and after optimizing). A
// single run per side cannot separate a real change from run-to-run
// variation (scene, background tasks, boost clocks), so the verdict needs at
// least two runs on each side; the test is Welch's two-sample t-test (unequal
// variances, NIST/SEMATECH e-Handbook of Statistical Methods, 1.3.5.3).
// Several metrics are tested at once, so the p-values are adjusted with
// Holm's step-down method before the 5% threshold, and a difference smaller
// than a practical minimum (1% and a small absolute amount) is "igual" even
// when it is statistically clear.

// GroupStat summarizes one metric over the runs of a group.
type GroupStat struct {
	N      int       `json:"n"`
	Mean   float64   `json:"mean"`
	SD     float64   `json:"sd"` // -1 with a single run
	Values []float64 `json:"values"`
}

// Row is one metric of the comparison.
type Row struct {
	Key      string    `json:"key"`
	Label    string    `json:"label"`
	Unit     string    `json:"unit"`
	Higher   bool      `json:"higher"` // higher is better
	A        GroupStat `json:"a"`
	B        GroupStat `json:"b"`
	Delta    float64   `json:"delta"`    // B - A
	DeltaPct float64   `json:"deltaPct"` // relative to A
	P        float64   `json:"p"`        // two-sided p-value; -1 = not computed
	PAdj     float64   `json:"pAdj"`     // Holm-adjusted over the metrics compared; -1 = not computed
	Verdict  string    `json:"verdict"`  // melhor | pior | igual | inconclusivo
}

// Comparison is the result shown in the interface.
type Comparison struct {
	Rows     []Row    `json:"rows"`
	Warnings []string `json:"warnings"`
	Enough   bool     `json:"enough"` // at least 2 runs on each side
}

type metric struct {
	key, label, unit string
	higher           bool
	get              func(Summary) float64
}

// minAbs is the smallest absolute difference that matters, per unit.
func minAbs(unit string) float64 {
	switch unit {
	case "FPS":
		return 0.5
	case "ms":
		return 0.05
	case "%":
		return 0.05
	}
	return 0
}

var metrics = []metric{
	{"avgFps", "FPS médio", "FPS", true, func(s Summary) float64 { return s.AvgFPS }},
	{"low1Fps", "1% low", "FPS", true, func(s Summary) float64 { return s.Low1FPS }},
	{"low01Fps", "0,1% low", "FPS", true, func(s Summary) float64 { return s.Low01FPS }},
	{"ftP99", "Tempo de quadro P99", "ms", false, func(s Summary) float64 { return s.FTP99 }},
	{"ftStdDev", "Desvio-padrão do tempo de quadro", "ms", false, func(s Summary) float64 { return s.FTStdDev }},
	{"stutterPct", "Tempo em engasgos", "%", false, func(s Summary) float64 { return s.StutterPct }},
	{"inputLatency", "Latência entrada → tela (estimada)", "ms", false, func(s Summary) float64 { return s.InputLatencyAvg }},
	{"pcLatency", "Latência do PC (Reflex)", "ms", false, func(s Summary) float64 { return s.PCLatencyAvg }},
}

func group(vals []float64) GroupStat {
	g := GroupStat{N: len(vals), Values: vals, SD: -1}
	if g.N == 0 {
		return g
	}
	g.Mean = meanOf(vals)
	if g.N > 1 {
		sq := 0.0
		for _, v := range vals {
			sq += (v - g.Mean) * (v - g.Mean)
		}
		g.SD = math.Sqrt(sq / float64(g.N-1))
	}
	return g
}

// Compare compares group A (reference, e.g. "antes") with group B.
func Compare(labelA string, a []Summary, labelB string, b []Summary) Comparison {
	cmp := Comparison{Enough: len(a) >= 2 && len(b) >= 2}
	for _, m := range metrics {
		var va, vb []float64
		for _, s := range a {
			if v := m.get(s); v >= 0 {
				va = append(va, v)
			}
		}
		for _, s := range b {
			if v := m.get(s); v >= 0 {
				vb = append(vb, v)
			}
		}
		if len(va) == 0 || len(vb) == 0 {
			continue // metric not available in one of the groups
		}
		r := Row{Key: m.key, Label: m.label, Unit: m.unit, Higher: m.higher, A: group(va), B: group(vb), P: -1, PAdj: -1}
		r.Delta = r.B.Mean - r.A.Mean
		if r.A.Mean != 0 {
			r.DeltaPct = 100 * r.Delta / r.A.Mean
		}
		if r.A.N >= 2 && r.B.N >= 2 {
			r.P = WelchP(r.A, r.B)
		}
		cmp.Rows = append(cmp.Rows, r)
	}
	holm(cmp.Rows)
	for i := range cmp.Rows {
		r := &cmp.Rows[i]
		better := (r.Delta > 0) == r.Higher
		small := math.Abs(r.DeltaPct) < 1 || math.Abs(r.Delta) < minAbs(r.Unit)
		switch {
		case r.A.N < 2 || r.B.N < 2:
			r.Verdict = "inconclusivo"
		case r.Delta == 0 || small:
			r.Verdict = "igual" // too small to matter, whatever the test says
		case r.PAdj < 0:
			r.Verdict = "inconclusivo" // no variation at all: no test possible
		case r.PAdj < 0.05 && better:
			r.Verdict = "melhor"
		case r.PAdj < 0.05:
			r.Verdict = "pior"
		default:
			r.Verdict = "igual"
		}
		r.Delta, r.DeltaPct = round(r.Delta, 3), round(r.DeltaPct, 2)
		if r.P >= 0 {
			r.P = round(r.P, 4)
		}
		if r.PAdj >= 0 {
			r.PAdj = round(r.PAdj, 4)
		}
		r.A.Mean, r.B.Mean = round(r.A.Mean, 3), round(r.B.Mean, 3)
		if r.A.SD >= 0 {
			r.A.SD = round(r.A.SD, 3)
		}
		if r.B.SD >= 0 {
			r.B.SD = round(r.B.SD, 3)
		}
	}
	if cmp.Enough && (len(a) == 2 || len(b) == 2) {
		cmp.Warnings = append(cmp.Warnings, "Com 2 medições em um dos grupos só diferenças grandes aparecem como reais; 3 ou mais de cada lado deixam a comparação mais sensível.")
	}
	if !cmp.Enough {
		cmp.Warnings = append(cmp.Warnings, "Com só uma medição em algum dos grupos não dá para separar a mudança da variação natural entre medições. Grave pelo menos 2 (de preferência 3) de cada lado, no mesmo cenário.")
	}
	for _, grp := range []struct {
		name string
		runs []Summary
	}{{labelA, a}, {labelB, b}} {
		fpsVals := make([]float64, 0, len(grp.runs))
		for _, s := range grp.runs {
			fpsVals = append(fpsVals, s.AvgFPS)
		}
		st := group(fpsVals)
		if st.N >= 2 && st.Mean > 0 && st.SD/st.Mean > 0.05 {
			cmp.Warnings = append(cmp.Warnings, fmt.Sprintf("As medições do grupo \"%s\" variam %.0f%% entre si: o cenário não está se repetindo (mapa, posição, jogadores ou programas em segundo plano).", grp.name, 100*st.SD/st.Mean))
		}
	}
	modeOf := func(g []Summary) map[string]bool {
		m := map[string]bool{}
		for _, s := range g {
			if s.PresentMode != "" {
				m[s.PresentMode] = true
			}
		}
		return m
	}
	ma, mb := modeOf(a), modeOf(b)
	differ := len(ma) != len(mb)
	for k := range mb {
		if !ma[k] {
			differ = true
		}
	}
	if differ && len(ma) > 0 && len(mb) > 0 {
		cmp.Warnings = append(cmp.Warnings, "O modo de apresentação mudou entre os grupos (ex.: tela cheia × janela): isso sozinho muda FPS e latência.")
	}
	return cmp
}

// holm fills PAdj with Holm's step-down adjustment of the rows' p-values
// (rows without a p-value are left out of the family).
func holm(rows []Row) {
	var idx []int
	for i := range rows {
		rows[i].PAdj = -1
		if rows[i].P >= 0 {
			idx = append(idx, i)
		}
	}
	sort.Slice(idx, func(a, b int) bool { return rows[idx[a]].P < rows[idx[b]].P })
	m := len(idx)
	prev := 0.0
	for k, i := range idx {
		adj := math.Min(1, float64(m-k)*rows[i].P)
		if adj < prev {
			adj = prev // keep the adjusted values monotone
		}
		rows[i].PAdj, prev = adj, adj
	}
}

// WelchP returns the two-sided p-value of Welch's t-test.
func WelchP(a, b GroupStat) float64 {
	if a.N < 2 || b.N < 2 {
		return -1
	}
	va, vb := a.SD*a.SD/float64(a.N), b.SD*b.SD/float64(b.N)
	se := math.Sqrt(va + vb)
	if se == 0 {
		// No variation in either group (often values rounded to the same
		// number): the test is undefined; never report certainty.
		if a.Mean == b.Mean {
			return 1
		}
		return -1
	}
	t := (b.Mean - a.Mean) / se
	df := (va + vb) * (va + vb) / (va*va/float64(a.N-1) + vb*vb/float64(b.N-1))
	return 2 * studentTail(math.Abs(t), df)
}

// studentTail is P(T > t) for Student's t with df degrees of freedom:
// 0.5 * I_{df/(df+t^2)}(df/2, 1/2).
func studentTail(t, df float64) float64 {
	if df <= 0 || math.IsNaN(t) {
		return math.NaN()
	}
	x := df / (df + t*t)
	return 0.5 * regIncBeta(df/2, 0.5, x)
}

// regIncBeta is the regularized incomplete beta function I_x(a, b),
// evaluated with the continued fraction (modified Lentz), as in Numerical
// Recipes, 6.4.
func regIncBeta(a, b, x float64) float64 {
	switch {
	case x <= 0:
		return 0
	case x >= 1:
		return 1
	}
	la, _ := math.Lgamma(a + b)
	lb, _ := math.Lgamma(a)
	lc, _ := math.Lgamma(b)
	front := math.Exp(la - lb - lc + a*math.Log(x) + b*math.Log(1-x))
	if x < (a+1)/(a+b+2) {
		return front * betaCF(a, b, x) / a
	}
	return 1 - front*betaCF(b, a, 1-x)/b
}

func betaCF(a, b, x float64) float64 {
	const (
		maxIter = 300
		eps     = 1e-14
		tiny    = 1e-300
	)
	qab, qap, qam := a+b, a+1, a-1
	c, d := 1.0, 1-qab*x/qap
	if math.Abs(d) < tiny {
		d = tiny
	}
	d = 1 / d
	h := d
	for m := 1; m <= maxIter; m++ {
		fm := float64(m)
		m2 := 2 * fm
		aa := fm * (b - fm) * x / ((qam + m2) * (a + m2))
		d = 1 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		h *= d * c
		aa = -(a + fm) * (qab + fm) * x / ((a + m2) * (qap + m2))
		d = 1 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < eps {
			break
		}
	}
	return h
}

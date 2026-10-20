package periph

import (
	"math"
	"sort"
)

// Standard polling rates offered by mice and receivers.
var standardRates = []int{125, 250, 500, 1000, 2000, 4000, 8000}

// Snap returns the standard rate within ±20% of hz, or 0.
func Snap(hz float64) int {
	if hz <= 0 {
		return 0
	}
	best, bestD := 0, math.Log(1.2)
	for _, r := range standardRates {
		d := math.Abs(math.Log(hz / float64(r)))
		if d < bestD {
			best, bestD = r, d
		}
	}
	return best
}

// Bin is one histogram bucket of intervals, relative to the reference period.
type Bin struct {
	Label string  `json:"label"`
	LoUS  float64 `json:"loUs"`
	HiUS  float64 `json:"hiUs"`
	N     int     `json:"n"`
	Pct   float64 `json:"pct"`
}

// Stats summarises the intervals between consecutive reports.
type Stats struct {
	Samples   int     `json:"samples"` // intervals analysed (pauses excluded)
	SpanMS    float64 `json:"spanMs"`
	RateHz    float64 `json:"rateHz"` // 1e6 / mean interval, pauses excluded
	MedianUS  float64 `json:"medianUs"`
	MeanUS    float64 `json:"meanUs"`
	P1US      float64 `json:"p1Us"`
	P5US      float64 `json:"p5Us"`
	P95US     float64 `json:"p95Us"`
	P99US     float64 `json:"p99Us"`
	StdDevUS  float64 `json:"stdDevUs"`
	MaxUS     float64 `json:"maxUs"`     // longest interval that is not a pause
	Pauses    int     `json:"pauses"`    // intervals ≥ max(4×median, 1 ms): motion stopped or the host stalled
	PauseMS   float64 `json:"pauseMs"`   // time spent in those pauses
	Coalesced int     `json:"coalesced"` // intervals under 5% of the median: reports delivered together
	RefUS     float64 `json:"refUs"`     // reference period (nominal when known, else the median)
	OnTimePct float64 `json:"onTimePct"` // intervals within ±25% of the reference period
	Snapped   int     `json:"snapped"`   // nearest standard rate for the median period (0 = none)
	Hist      []Bin   `json:"hist"`
}

// Analyze computes the interval statistics of a timestamp series (ticks of a
// clock running at freq Hz). nominalHz, when known, is the rate the port is
// scheduled for and becomes the reference period. It returns nil when there
// is not enough data.
func Analyze(stamps []int64, freq int64, nominalHz float64) *Stats {
	if freq <= 0 || len(stamps) < 3 {
		return nil
	}
	ts := append([]int64(nil), stamps...)
	sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })
	var all []float64
	for i := 1; i < len(ts); i++ {
		d := float64(ts[i]-ts[i-1]) * 1e6 / float64(freq)
		if d < 0 || d > 250_000 { // > 250 ms: the mouse was left still
			continue
		}
		all = append(all, d)
	}
	if len(all) < 2 {
		return nil
	}
	sorted := append([]float64(nil), all...)
	sort.Float64s(sorted)
	med := percentile(sorted, 50)
	thr := math.Max(4*med, 1000)
	st := &Stats{MedianUS: med}
	var steady []float64
	for _, d := range all {
		if d >= thr {
			st.Pauses++
			st.PauseMS += d / 1000
			continue
		}
		steady = append(steady, d)
		if d < 0.05*med {
			st.Coalesced++
		}
	}
	if len(steady) == 0 {
		return nil
	}
	sort.Float64s(steady)
	sum := 0.0
	for _, d := range steady {
		sum += d
	}
	st.Samples = len(steady)
	st.MeanUS = sum / float64(len(steady))
	if st.MeanUS > 0 {
		st.RateHz = 1e6 / st.MeanUS
	}
	varSum := 0.0
	for _, d := range steady {
		varSum += (d - st.MeanUS) * (d - st.MeanUS)
	}
	st.StdDevUS = math.Sqrt(varSum / float64(len(steady)))
	st.P1US = percentile(steady, 1)
	st.P5US = percentile(steady, 5)
	st.P95US = percentile(steady, 95)
	st.P99US = percentile(steady, 99)
	st.MaxUS = steady[len(steady)-1]
	st.SpanMS = float64(ts[len(ts)-1]-ts[0]) * 1000 / float64(freq)
	if med > 0 {
		st.Snapped = Snap(1e6 / med)
	}
	st.RefUS = med
	if nominalHz > 0 {
		st.RefUS = 1e6 / nominalHz
	}
	on := 0
	for _, d := range steady {
		if math.Abs(d-st.RefUS) <= 0.25*st.RefUS {
			on++
		}
	}
	st.OnTimePct = 100 * float64(on) / float64(len(steady))
	st.Hist = histogram(steady, st.RefUS, st.Pauses)
	return st
}

// histogram buckets intervals relative to the reference period.
func histogram(steady []float64, ref float64, pauses int) []Bin {
	edges := []struct {
		label  string
		lo, hi float64
	}{
		{"agrupados", 0, 0.25},
		{"adiantados", 0.25, 0.75},
		{"no ritmo", 0.75, 1.25},
		{"atrasados", 1.25, 1.75},
		{"~2× (1 pulado)", 1.75, 2.5},
		{"vários pulados", 2.5, math.Inf(1)},
	}
	total := len(steady) + pauses
	bins := make([]Bin, 0, len(edges)+1)
	for _, e := range edges {
		b := Bin{Label: e.label, LoUS: e.lo * ref, HiUS: e.hi * ref}
		if math.IsInf(b.HiUS, 1) {
			b.HiUS = 0
		}
		for _, d := range steady {
			if d >= e.lo*ref && d < e.hi*ref {
				b.N++
			}
		}
		bins = append(bins, b)
	}
	bins = append(bins, Bin{Label: "pausas", N: pauses})
	for i := range bins {
		if total > 0 {
			bins[i].Pct = 100 * float64(bins[i].N) / float64(total)
		}
	}
	return bins
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)) * p / 100)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	if idx < 0 {
		idx = 0
	}
	return sorted[idx]
}

// Lag estimates the delay between USB completion and raw input arrival:
// for every raw report, the time since the latest USB completion before it.
// Both series must use the same clock (QueryPerformanceCounter).
type Lag struct {
	MedianUS float64 `json:"medianUs"`
	P95US    float64 `json:"p95Us"`
	Samples  int     `json:"samples"`
}

// StackLag computes Lag, or nil when the series do not overlap.
func StackLag(usb, raw []int64, freq int64) *Lag {
	if freq <= 0 || len(usb) < 10 || len(raw) < 10 {
		return nil
	}
	u := append([]int64(nil), usb...)
	sort.Slice(u, func(i, j int) bool { return u[i] < u[j] })
	var lags []float64
	for _, r := range raw {
		// latest completion at or before r
		k := sort.Search(len(u), func(i int) bool { return u[i] > r }) - 1
		if k < 0 {
			continue
		}
		d := float64(r-u[k]) * 1e6 / float64(freq)
		if d > 20_000 { // not related: more than 20 ms apart
			continue
		}
		lags = append(lags, d)
	}
	if len(lags) < 10 {
		return nil
	}
	sort.Float64s(lags)
	return &Lag{MedianUS: percentile(lags, 50), P95US: percentile(lags, 95), Samples: len(lags)}
}

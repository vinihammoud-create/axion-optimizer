// Package netdiag turns network probes into the numbers and verdicts of the
// Rede page: latency statistics, the path to the internet, Valve's relays,
// bufferbloat, DNS and the recommendations that follow from them. It has no
// I/O: the app feeds it the measurements.
package netdiag

import (
	"math"
	"sort"
)

// Lost marks a probe without reply in a sample series.
const Lost = -1.0

// Stats summarizes one series of round-trip times (ms).
type Stats struct {
	Sent    int     `json:"sent"`
	Recv    int     `json:"recv"`
	LossPct float64 `json:"lossPct"`
	Min     float64 `json:"min"`
	Median  float64 `json:"median"`
	Avg     float64 `json:"avg"`
	P95     float64 `json:"p95"`
	Max     float64 `json:"max"`
	// Jitter is the mean distance between consecutive replies (the
	// definition of Cloudflare's speed test); lost probes are skipped.
	Jitter float64 `json:"jitter"`
	StdDev float64 `json:"stdDev"`
}

// OK reports whether the series has replies.
func (s Stats) OK() bool { return s.Recv > 0 }

// Compute summarizes samples in ms (Lost = no reply).
func Compute(samples []float64) Stats {
	st := Stats{Sent: len(samples)}
	var ok []float64
	prev, havePrev := 0.0, false
	var jsum float64
	var jn int
	for _, v := range samples {
		if v < 0 || math.IsNaN(v) {
			continue
		}
		ok = append(ok, v)
		if havePrev {
			jsum += math.Abs(v - prev)
			jn++
		}
		prev, havePrev = v, true
	}
	st.Recv = len(ok)
	if st.Sent > 0 {
		st.LossPct = 100 * float64(st.Sent-st.Recv) / float64(st.Sent)
	}
	if len(ok) == 0 {
		return st
	}
	if jn > 0 {
		st.Jitter = jsum / float64(jn)
	}
	sorted := append([]float64(nil), ok...)
	sort.Float64s(sorted)
	st.Min, st.Max = sorted[0], sorted[len(sorted)-1]
	st.Median = Percentile(sorted, 0.5)
	st.P95 = Percentile(sorted, 0.95)
	var sum float64
	for _, v := range ok {
		sum += v
	}
	st.Avg = sum / float64(len(ok))
	if len(ok) > 1 {
		var ss float64
		for _, v := range ok {
			ss += (v - st.Avg) * (v - st.Avg)
		}
		st.StdDev = math.Sqrt(ss / float64(len(ok)-1))
	}
	return st
}

// Percentile of sorted values with linear interpolation (p in 0..1).
func Percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	pos := p * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	return sorted[lo] + (sorted[hi]-sorted[lo])*(pos-float64(lo))
}

// Downsample keeps at most n points of a series for charts, taking the
// worst (highest) value of each bucket so spikes stay visible; a bucket with
// only lost probes stays Lost.
func Downsample(samples []float64, n int) []float64 {
	if n <= 0 || len(samples) <= n {
		return append([]float64(nil), samples...)
	}
	out := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		a := i * len(samples) / n
		b := (i + 1) * len(samples) / n
		worst := Lost
		for _, v := range samples[a:b] {
			if v > worst {
				worst = v
			}
		}
		out = append(out, round2(worst))
	}
	return out
}

func round2(v float64) float64 {
	if v < 0 {
		return v
	}
	return math.Round(v*100) / 100
}

// Round rounds every field of s to 2 decimals (for storage and display).
func (s Stats) Round() Stats {
	s.LossPct = math.Round(s.LossPct*100) / 100
	for _, p := range []*float64{&s.Min, &s.Median, &s.Avg, &s.P95, &s.Max, &s.Jitter, &s.StdDev} {
		*p = round2(*p)
	}
	return s
}

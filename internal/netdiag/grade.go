package netdiag

import (
	"math"
	"sort"
	"time"
)

// Bufferbloat grade by the latency added under load (ms), on the scale of
// the Waveform bufferbloat test that bufferbloat.net recommends (below "B"
// is a problem).
func BloatGrade(addedMs float64) string {
	switch {
	case addedMs < 5:
		return "A+"
	case addedMs < 30:
		return "A"
	case addedMs < 60:
		return "B"
	case addedMs < 200:
		return "C"
	case addedMs < 400:
		return "D"
	}
	return "F"
}

// threshold maps v to points: below edges[0] -> pts[0], and so on (the
// scale used by Cloudflare's speed test).
func threshold(v float64, edges, pts []float64) float64 {
	for i, e := range edges {
		if v < e {
			return pts[i]
		}
	}
	return pts[len(pts)-1]
}

// AIM is Cloudflare's Aggregated Internet Measurement classification for
// online gaming: idle latency, packet loss and the latency added under load,
// each worth points, summed and bucketed (the thresholds of the open-source
// engine of speed.cloudflare.com).
type AIM struct {
	Points int    `json:"points"`
	Class  string `json:"class"` // ruim, fraca, média, boa, ótima
}

// AIMGaming classifies a connection for games. lossFrac is 0..1.
func AIMGaming(idleMs, lossFrac, addedMs float64) AIM {
	p := threshold(lossFrac, []float64{0.01, 0.05, 0.25, 0.5}, []float64{10, 5, 0, -10, -20}) +
		threshold(idleMs, []float64{10, 20, 50, 100, 500}, []float64{20, 10, 5, 0, -10, -20}) +
		threshold(addedMs, []float64{10, 20, 50, 100, 500}, []float64{20, 10, 5, 0, -10, -20})
	if p < 0 {
		p = 0
	}
	classes := []string{"ruim", "fraca", "média", "boa", "ótima"}
	idx := int(threshold(p, []float64{5, 15, 25, 30}, []float64{0, 1, 2, 3, 4}))
	return AIM{Points: int(p), Class: classes[idx]}
}

// ByteSample is the cumulative byte count at an instant of a load phase.
type ByteSample struct {
	At    time.Duration
	Bytes int64
}

// Throughput is the steady rate (Mbit/s) of a load phase: the first part
// (connection and TCP ramp-up) is left out, like the speed tests do.
func Throughput(samples []ByteSample, skip time.Duration) float64 {
	if len(samples) < 2 {
		return 0
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].At < samples[j].At })
	first := samples[0]
	for _, s := range samples {
		if s.At >= skip {
			first = s
			break
		}
	}
	last := samples[len(samples)-1]
	dt := (last.At - first.At).Seconds()
	if dt <= 0.2 {
		// Too little after the ramp-up: use the whole phase.
		first = samples[0]
		dt = (last.At - first.At).Seconds()
		if dt <= 0 {
			return 0
		}
	}
	return math.Round(float64(last.Bytes-first.Bytes)*8/dt/1e6*10) / 10
}

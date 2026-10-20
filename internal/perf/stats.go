package perf

import (
	"math"
)

// Definitions (sources in the Desempenho page):
//   - Average FPS: frames / total time (frames x 1000 / sum of frame times).
//   - 1% and 0.1% low: the slowest 1% (0.1%) of frames averaged, as defined by
//     NVIDIA FrameView ("takes the slowest 1% frames and averages them"),
//     shown as FPS = 1000 / mean frame time of those frames.
//   - P1 / P0.1: the FPS separating the slowest 1% (0.1%) from the rest
//     (percentile), with linear interpolation between ranks.
//   - Stutter: a frame longer than 2.5 x the average frame time (default
//     factor of CapFrameX).
//   - Limiter of a frame: GPU when GPU Busy covers >= 90% of the frame time
//     (GPU Busy ~ frame time = GPU-bound; much shorter = CPU or other limit);
//     CPU when the CPU did not wait (CPU Wait <= 10% of the frame); otherwise
//     a frame limiter or sync (fps_max, V-Sync, Reflex limiting) held the frame.
const (
	StutterFactor = 2.5
	gpuBoundShare = 0.90
	cpuWaitShare  = 0.10
)

// Summary holds the numbers of one capture. -1 means "not available".
type Summary struct {
	Frames  int     `json:"frames"`
	Seconds float64 `json:"seconds"`

	AvgFPS   float64 `json:"avgFps"`
	Low1FPS  float64 `json:"low1Fps"`
	Low01FPS float64 `json:"low01Fps"`
	P1FPS    float64 `json:"p1Fps"`
	P01FPS   float64 `json:"p01Fps"`

	FTAvg    float64 `json:"ftAvg"`
	FTMedian float64 `json:"ftMedian"`
	FTP99    float64 `json:"ftP99"`
	FTP999   float64 `json:"ftP999"`
	FTMax    float64 `json:"ftMax"`
	FTStdDev float64 `json:"ftStdDev"`

	Stutters   int     `json:"stutters"`
	StutterPct float64 `json:"stutterPct"` // % of the time spent in stutter frames

	DisplayedFPS float64 `json:"displayedFps"`
	DroppedPct   float64 `json:"droppedPct"` // frames never shown on screen

	CPUBusyAvg  float64 `json:"cpuBusyAvg"`
	GPUBusyAvg  float64 `json:"gpuBusyAvg"`
	GPUBoundPct float64 `json:"gpuBoundPct"`
	CPUBoundPct float64 `json:"cpuBoundPct"`
	WaitPct     float64 `json:"waitPct"`
	Limiter     string  `json:"limiter"` // gpu | cpu (CPU or the game's own limiter) | limite | misto | "" (unknown)
	// CapLikely: limited by the CPU side with a very steady frame time,
	// the signature of an in-game FPS limit (fps_max) rather than a slow CPU.
	CapLikely bool `json:"capLikely,omitempty"`

	PresentMode    string  `json:"presentMode"`
	PresentModePct float64 `json:"presentModePct"`
	TearingPct     float64 `json:"tearingPct"`
	SyncInterval   int     `json:"syncInterval"`

	UntilDisplayedAvg float64 `json:"untilDisplayedAvg"`
	InputLatencyAvg   float64 `json:"inputLatencyAvg"`
	ClickLatencyAvg   float64 `json:"clickLatencyAvg"`
	PCLatencyAvg      float64 `json:"pcLatencyAvg"`
}

func round(v float64, d int) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return -1
	}
	p := math.Pow(10, float64(d))
	return math.Round(v*p) / p
}

// percentile of sorted values (linear interpolation between closest ranks).
func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return math.NaN()
	}
	if n == 1 {
		return sorted[0]
	}
	pos := p * float64(n-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if hi >= n {
		hi = n - 1
	}
	return sorted[lo] + (sorted[hi]-sorted[lo])*(pos-float64(lo))
}

// slowestMean averages the slowest share of the sorted frame times.
func slowestMean(sorted []float64, share float64) float64 {
	k := int(math.Ceil(float64(len(sorted)) * share))
	if k < 1 {
		return math.NaN()
	}
	sum := 0.0
	for _, v := range sorted[len(sorted)-k:] {
		sum += v
	}
	return sum / float64(k)
}

func meanOf(vals []float64) float64 {
	if len(vals) == 0 {
		return math.NaN()
	}
	s := 0.0
	for _, v := range vals {
		s += v
	}
	return s / float64(len(vals))
}

func fps(ms float64) float64 {
	if math.IsNaN(ms) || ms <= 0 {
		return math.NaN()
	}
	return 1000 / ms
}

// Summarize computes the summary of a capture. It is an Accum over one capture,
// so the automatic match report (several windows) gives the same numbers.
func Summarize(c *Capture) Summary {
	var a Accum
	a.Add(c)
	return a.Summary()
}

// Series downsamples the frame times for a chart drawn over time: at most
// points values, bucket i covering the i-th slice of the recording's
// duration (by accumulated frame time, so a hitch takes the width of its
// real duration), each holding the slowest frame of its slice so spikes
// stay visible. Slices without a frame repeat the previous value.
func Series(c *Capture, points int) []float32 {
	ft := make([]float64, len(c.Frames))
	for i, f := range c.Frames {
		ft[i] = f.FrameTime
	}
	return seriesOf(ft, points)
}

package perf

import (
	"math"
	"sort"
)

// meanAcc is a running mean: the sum is made frame by frame, in order, which is
// exactly what meanOf does over a slice, so both give the same float.
type meanAcc struct {
	sum float64
	n   int
}

func (m *meanAcc) add(v float64) { m.sum += v; m.n++ }

// mean is NaN when nothing was added (like meanOf of an empty slice).
func (m meanAcc) mean() float64 {
	if m.n == 0 {
		return math.NaN()
	}
	return m.sum / float64(m.n)
}

// Accum gathers the frames of one or more captures (the windows of the
// automatic match report) and summarizes them as one capture. Summarize is an
// Accum over a single capture, so both always give the same numbers. It keeps
// 8 bytes per frame (the frame time); everything else is counted on arrival.
type Accum struct {
	ft    []float64
	total float64

	disp, cpu, gpu, ud, in, cl, pcl meanAcc

	shown, known                  int
	gpuB, cpuB, waitB, classified int
	modes                         map[string]int
	syncs                         map[int]int
	tearing                       int
}

// Add takes the frames of a capture. The sums are made in the same order as
// the old Summarize (frame by frame), so the floats come out identical.
func (a *Accum) Add(c *Capture) {
	if c == nil {
		return
	}
	if a.modes == nil {
		a.modes, a.syncs = map[string]int{}, map[int]int{}
	}
	for _, f := range c.Frames {
		a.ft = append(a.ft, f.FrameTime)
		a.total += f.FrameTime
		if f.Mode != "" {
			a.modes[f.Mode]++
		}
		a.syncs[f.SyncInterval]++
		if f.Tearing {
			a.tearing++
		}
		if f.Mode != "" || !math.IsNaN(f.Displayed) {
			a.known++
			if !math.IsNaN(f.Displayed) && f.Displayed > 0 {
				a.shown++
				a.disp.add(f.Displayed)
			}
		}
		if !math.IsNaN(f.CPUBusy) {
			a.cpu.add(f.CPUBusy)
		}
		if !math.IsNaN(f.GPUBusy) {
			a.gpu.add(f.GPUBusy)
		}
		if !math.IsNaN(f.UntilDisp) && f.UntilDisp > 0 {
			a.ud.add(f.UntilDisp)
		}
		if !math.IsNaN(f.InputPhoton) && f.InputPhoton > 0 {
			a.in.add(f.InputPhoton)
		}
		if !math.IsNaN(f.ClickPhoton) && f.ClickPhoton > 0 {
			a.cl.add(f.ClickPhoton)
		}
		if !math.IsNaN(f.PCLatency) && f.PCLatency > 0 {
			a.pcl.add(f.PCLatency)
		}
		if !math.IsNaN(f.GPUBusy) && !math.IsNaN(f.CPUWait) {
			a.classified++
			switch {
			case f.GPUBusy >= gpuBoundShare*f.FrameTime:
				a.gpuB++
			case f.CPUWait <= cpuWaitShare*f.FrameTime:
				a.cpuB++
			default:
				a.waitB++
			}
		}
	}
}

// Frames is how many frames were added.
func (a *Accum) Frames() int { return len(a.ft) }

// Seconds is the recorded time (sum of the frame times).
func (a *Accum) Seconds() float64 { return a.total / 1000 }

// Series downsamples the frame times of everything added (see Series).
func (a *Accum) Series(points int) []float32 { return seriesOf(a.ft, points) }

// Summary computes the numbers of everything added, as Summarize does for one
// capture.
func (a *Accum) Summary() Summary {
	n := len(a.ft)
	s := Summary{Frames: n}
	if n == 0 {
		return s
	}
	ft, total := a.ft, a.total
	sorted := append([]float64(nil), ft...)
	sort.Float64s(sorted)
	avg := total / float64(n)
	s.Seconds = round(total/1000, 2)
	s.AvgFPS = round(float64(n)*1000/total, 1)
	s.FTAvg = round(avg, 3)
	s.FTMedian = round(percentile(sorted, 0.5), 3)
	s.FTP99 = round(percentile(sorted, 0.99), 3)
	s.FTMax = round(sorted[n-1], 3)
	s.Low1FPS, s.P1FPS, s.Low01FPS, s.P01FPS, s.FTP999 = -1, -1, -1, -1, -1
	if n >= 100 {
		s.Low1FPS = round(fps(slowestMean(sorted, 0.01)), 1)
		s.P1FPS = round(fps(percentile(sorted, 0.99)), 1)
	}
	if n >= 1000 {
		s.Low01FPS = round(fps(slowestMean(sorted, 0.001)), 1)
		s.P01FPS = round(fps(percentile(sorted, 0.999)), 1)
		s.FTP999 = round(percentile(sorted, 0.999), 3)
	}
	sq := 0.0
	for _, v := range ft {
		sq += (v - avg) * (v - avg)
	}
	if n > 1 {
		s.FTStdDev = round(math.Sqrt(sq/float64(n-1)), 3)
	}
	stutterTime := 0.0
	for _, v := range ft {
		if v > StutterFactor*avg {
			s.Stutters++
			stutterTime += v
		}
	}
	s.StutterPct = round(100*stutterTime/total, 2)

	s.DisplayedFPS, s.DroppedPct = -1, -1
	if a.known > 0 && a.disp.n > 0 {
		s.DisplayedFPS = round(fps(a.disp.mean()), 1)
		s.DroppedPct = round(100*float64(a.known-a.shown)/float64(a.known), 2)
	}
	s.CPUBusyAvg, s.GPUBusyAvg = round(a.cpu.mean(), 3), round(a.gpu.mean(), 3)
	s.UntilDisplayedAvg, s.InputLatencyAvg = round(a.ud.mean(), 2), round(a.in.mean(), 2)
	s.ClickLatencyAvg, s.PCLatencyAvg = round(a.cl.mean(), 2), round(a.pcl.mean(), 2)
	s.GPUBoundPct, s.CPUBoundPct, s.WaitPct = -1, -1, -1
	if a.classified > 0 {
		c := float64(a.classified)
		s.GPUBoundPct = round(100*float64(a.gpuB)/c, 1)
		s.CPUBoundPct = round(100*float64(a.cpuB)/c, 1)
		s.WaitPct = round(100*float64(a.waitB)/c, 1)
		switch {
		case s.GPUBoundPct >= 50:
			s.Limiter = "gpu"
		case s.CPUBoundPct >= 50:
			// PresentMon counts the time a game's own frame limiter
			// (fps_max, Reflex) sleeps as CPU busy: a very steady frame time
			// points to a limiter rather than a slow CPU.
			s.Limiter = "cpu"
			if s.AvgFPS > 0 && s.FTStdDev >= 0 && s.FTStdDev < 0.05*1000/s.AvgFPS {
				s.CapLikely = true
			}
		case s.WaitPct >= 50:
			s.Limiter = "limite"
		default:
			s.Limiter = "misto"
		}
	}
	best := 0
	for m, k := range a.modes {
		if k > best || (k == best && m < s.PresentMode) {
			s.PresentMode, best = m, k
		}
	}
	if best > 0 {
		s.PresentModePct = round(100*float64(best)/float64(n), 1)
	}
	s.TearingPct = round(100*float64(a.tearing)/float64(n), 1)
	bestSync := -1
	for v, k := range a.syncs {
		if k > bestSync || (k == bestSync && v < s.SyncInterval) {
			s.SyncInterval, bestSync = v, k
		}
	}
	return s
}

// seriesOf downsamples frame times (see Series). The total is summed in a
// first loop, because the bucket of each frame depends on the final total.
func seriesOf(ft []float64, points int) []float32 {
	n := len(ft)
	if n == 0 || points <= 0 {
		return nil
	}
	if n <= points {
		out := make([]float32, n)
		for i, v := range ft {
			out[i] = float32(math.Round(v*100) / 100)
		}
		return out
	}
	total := 0.0
	for _, v := range ft {
		total += v
	}
	out := make([]float32, points)
	filled := make([]bool, points)
	acc := 0.0
	for _, v := range ft {
		b := int(acc / total * float64(points))
		if b >= points {
			b = points - 1
		}
		acc += v
		if x := float32(math.Round(v*100) / 100); x > out[b] {
			out[b] = x
		}
		filled[b] = true
	}
	for i := range out {
		if !filled[i] && i > 0 {
			out[i] = out[i-1]
		}
	}
	return out
}

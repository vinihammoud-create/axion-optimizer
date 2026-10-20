package perf

import (
	"math"
	"sort"
)

// legacySummarize is the body of Summarize before the Accum refactor, kept
// verbatim as the reference the new code must equal.
func legacySummarize(c *Capture) Summary {
	fr := c.Frames
	n := len(fr)
	s := Summary{Frames: n}
	if n == 0 {
		return s
	}
	ft := make([]float64, n)
	total := 0.0
	for i, f := range fr {
		ft[i] = f.FrameTime
		total += f.FrameTime
	}
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

	// Display: frames shown and how often the screen changed.
	var disp, cpu, gpu, ud, in, cl, pcl []float64
	shown, known := 0, 0
	var gpuB, cpuB, waitB, classified int
	modes := map[string]int{}
	syncs := map[int]int{}
	tearing := 0
	for _, f := range fr {
		if f.Mode != "" {
			modes[f.Mode]++
		}
		syncs[f.SyncInterval]++
		if f.Tearing {
			tearing++
		}
		if f.Mode != "" || !math.IsNaN(f.Displayed) {
			known++
			if !math.IsNaN(f.Displayed) && f.Displayed > 0 {
				shown++
				disp = append(disp, f.Displayed)
			}
		}
		if !math.IsNaN(f.CPUBusy) {
			cpu = append(cpu, f.CPUBusy)
		}
		if !math.IsNaN(f.GPUBusy) {
			gpu = append(gpu, f.GPUBusy)
		}
		if !math.IsNaN(f.UntilDisp) && f.UntilDisp > 0 {
			ud = append(ud, f.UntilDisp)
		}
		if !math.IsNaN(f.InputPhoton) && f.InputPhoton > 0 {
			in = append(in, f.InputPhoton)
		}
		if !math.IsNaN(f.ClickPhoton) && f.ClickPhoton > 0 {
			cl = append(cl, f.ClickPhoton)
		}
		if !math.IsNaN(f.PCLatency) && f.PCLatency > 0 {
			pcl = append(pcl, f.PCLatency)
		}
		if !math.IsNaN(f.GPUBusy) && !math.IsNaN(f.CPUWait) {
			classified++
			switch {
			case f.GPUBusy >= gpuBoundShare*f.FrameTime:
				gpuB++
			case f.CPUWait <= cpuWaitShare*f.FrameTime:
				cpuB++
			default:
				waitB++
			}
		}
	}
	s.DisplayedFPS, s.DroppedPct = -1, -1
	if known > 0 && len(disp) > 0 {
		s.DisplayedFPS = round(fps(meanOf(disp)), 1)
		s.DroppedPct = round(100*float64(known-shown)/float64(known), 2)
	}
	s.CPUBusyAvg, s.GPUBusyAvg = round(meanOf(cpu), 3), round(meanOf(gpu), 3)
	s.UntilDisplayedAvg, s.InputLatencyAvg = round(meanOf(ud), 2), round(meanOf(in), 2)
	s.ClickLatencyAvg, s.PCLatencyAvg = round(meanOf(cl), 2), round(meanOf(pcl), 2)
	s.GPUBoundPct, s.CPUBoundPct, s.WaitPct = -1, -1, -1
	if classified > 0 {
		c := float64(classified)
		s.GPUBoundPct = round(100*float64(gpuB)/c, 1)
		s.CPUBoundPct = round(100*float64(cpuB)/c, 1)
		s.WaitPct = round(100*float64(waitB)/c, 1)
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
	for m, k := range modes {
		if k > best || (k == best && m < s.PresentMode) {
			s.PresentMode, best = m, k
		}
	}
	if best > 0 {
		s.PresentModePct = round(100*float64(best)/float64(n), 1)
	}
	s.TearingPct = round(100*float64(tearing)/float64(n), 1)
	bestSync := -1
	for v, k := range syncs {
		if k > bestSync || (k == bestSync && v < s.SyncInterval) {
			s.SyncInterval, bestSync = v, k
		}
	}
	return s
}

// legacySeries is the old Series, kept as reference. Series downsamples the frame times for a chart drawn over time: at most
// points values, bucket i covering the i-th slice of the recording's
// duration (by accumulated frame time, so a hitch takes the width of its
// real duration), each holding the slowest frame of its slice so spikes
// stay visible. Slices without a frame repeat the previous value.
func legacySeries(c *Capture, points int) []float32 {
	n := len(c.Frames)
	if n == 0 || points <= 0 {
		return nil
	}
	if n <= points {
		out := make([]float32, n)
		for i, f := range c.Frames {
			out[i] = float32(math.Round(f.FrameTime*100) / 100)
		}
		return out
	}
	total := 0.0
	for _, f := range c.Frames {
		total += f.FrameTime
	}
	out := make([]float32, points)
	filled := make([]bool, points)
	acc := 0.0
	for _, f := range c.Frames {
		b := int(acc / total * float64(points))
		if b >= points {
			b = points - 1
		}
		acc += f.FrameTime
		if v := float32(math.Round(f.FrameTime*100) / 100); v > out[b] {
			out[b] = v
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

package mocksys

import (
	"context"
	"errors"
	"time"

	"axion/internal/sys"
)

// dpcRow describes the simulated cost of one module's routines: how many ran
// per second, and how those runs spread over the duration buckets (weights,
// one per sys.DPCHistBoundsUS edge plus the open bucket).
type dpcRow struct {
	module  string
	kind    string
	perSec  float64
	weights [14]float64
	maxUS   float64
}

// A plausible gaming desktop at rest: the graphics driver is the only one with
// DPCs well past the 100 µs Microsoft recommends, which is what the diagnostic
// has to point at.
var defaultDPCRows = []dpcRow{
	{"ntoskrnl.exe", sys.KindDPC, 1800, [14]float64{40, 30, 18, 7, 3, 1.5, .4, .1}, 180},
	{"ndis.sys", sys.KindDPC, 260, [14]float64{2, 8, 30, 30, 20, 8, 1.5, .5}, 240},
	{"tcpip.sys", sys.KindDPC, 190, [14]float64{3, 12, 35, 30, 14, 5, 1}, 95},
	{"nvlddmkm.sys", sys.KindDPC, 150, [14]float64{1, 4, 12, 20, 25, 20, 10, 6, 1.5, .5, .1}, 1400},
	{"USBXHCI.SYS", sys.KindDPC, 1100, [14]float64{20, 40, 30, 8, 2}, 24},
	{"storport.sys", sys.KindDPC, 60, [14]float64{5, 15, 30, 30, 15, 4, 1}, 95},
	{"Wdf01000.sys", sys.KindDPC, 90, [14]float64{10, 30, 40, 15, 5}, 24},
	{"nvlddmkm.sys", sys.KindISR, 140, [14]float64{10, 30, 40, 15, 4, 1}, 32},
	{"USBXHCI.SYS", sys.KindISR, 1000, [14]float64{35, 45, 15, 5}, 9},
	{"ACPI.sys", sys.KindISR, 30, [14]float64{5, 20, 40, 30, 5}, 14},
}

func syntheticDPC(d time.Duration, cpus int) sys.DPCCapture {
	secs := d.Seconds()
	c := sys.DPCCapture{Duration: d, CPUs: cpus}
	for _, r := range defaultDPCRows {
		var sum float64
		for _, w := range r.weights {
			sum += w
		}
		total := uint64(r.perSec * secs)
		if total == 0 || sum == 0 {
			continue
		}
		st := sys.DPCStat{Module: r.module, Kind: r.kind, Hist: make([]uint64, len(sys.DPCHistBoundsUS)+1)}
		var left = total
		for i, w := range r.weights {
			n := uint64(float64(total) * w / sum)
			if n > left {
				n = left
			}
			st.Hist[i] = n
			left -= n
		}
		st.Hist[0] += left // rounding
		// Time: each bucket at the middle of its range; the slowest routine alone sets the maximum.
		lower := 0.0
		for i, n := range st.Hist {
			if n == 0 {
				if i < len(sys.DPCHistBoundsUS) {
					lower = sys.DPCHistBoundsUS[i]
				}
				continue
			}
			upper := r.maxUS
			if i < len(sys.DPCHistBoundsUS) && sys.DPCHistBoundsUS[i] < upper {
				upper = sys.DPCHistBoundsUS[i]
			}
			if upper < lower {
				upper = lower
			}
			st.TotalUS += float64(n) * (lower + upper) / 2
			if i < len(sys.DPCHistBoundsUS) {
				lower = sys.DPCHistBoundsUS[i]
			}
		}
		st.Count = total
		st.MaxUS = r.maxUS
		c.Stats = append(c.Stats, st)
		c.Events += total
	}
	return c
}

// CPUFreqPct implements sys.Native.
func (n *Native) CPUFreqPct() (float64, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.CPUFreqErr != nil {
		return 0, n.CPUFreqErr
	}
	if n.CPUFreq == 0 {
		return 112, nil
	}
	return n.CPUFreq, nil
}

// CaptureDPC implements sys.Native with a synthetic capture.
func (n *Native) CaptureDPC(ctx context.Context, d time.Duration) (sys.DPCCapture, error) {
	n.mu.Lock()
	fn := n.DPCFn
	n.DPCCaptures++
	elevated := n.Elevated
	n.mu.Unlock()
	if fn != nil {
		return fn(ctx, d)
	}
	if !elevated {
		return sys.DPCCapture{}, errors.New("é preciso executar como administrador")
	}
	if d <= 0 {
		d = 30 * time.Second
	}
	// A short pause so the preview shows progress while "capturing".
	select {
	case <-ctx.Done():
		return sys.DPCCapture{Duration: d, Cancelled: true}, nil
	case <-time.After(300 * time.Millisecond):
	}
	return syntheticDPC(d, 16), nil
}

// KernelModules implements sys.Native.
func (n *Native) KernelModules() ([]string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ModulesErr != nil {
		return nil, n.ModulesErr
	}
	if n.Modules == nil {
		return []string{"ntoskrnl.exe", "hal.dll", "ndis.sys", "tcpip.sys", "nvlddmkm.sys", "storport.sys"}, nil
	}
	return append([]string(nil), n.Modules...), nil
}

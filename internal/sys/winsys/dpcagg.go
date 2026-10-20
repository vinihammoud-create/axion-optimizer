//go:build windows

package winsys

import (
	"sort"
	"sync"

	"axion/internal/sys"
	"axion/internal/sys/etw"
)

// routineKey identifies one DPC or ISR routine by its address.
type routineKey struct {
	addr uint64
	isr  bool
}

// routineAgg accumulates what one routine cost, in clock ticks.
type routineAgg struct {
	count      uint64
	totalTicks int64
	maxTicks   int64
	hist       [len(sys.DPCHistBoundsUS) + 1]uint64
}

// dpcCollector receives the DPC and ISR events on the ProcessTrace thread and
// keeps one small accumulator per routine, so a capture of millions of events
// stays a few hundred entries.
type dpcCollector struct {
	freq int64 // ticks per second of the session clock (QueryPerformanceCounter)

	mu        sync.Mutex
	routines  map[routineKey]*routineAgg
	events    uint64
	discarded uint64
}

func newDPCCollector(freq int64) *dpcCollector {
	return &dpcCollector{freq: freq, routines: map[routineKey]*routineAgg{}}
}

// event handles one PerfInfo event. ts is the event's time stamp, which is the
// END of the routine; the payload carries the entry time and the address.
func (c *dpcCollector) event(opcode uint8, ts int64, ptr int, payload []byte) {
	dpc, isr := etw.IsRoutineEvent(opcode)
	if !dpc && !isr {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events++
	entry, routine, ok := etw.ParseRoutine(payload, ptr)
	if !ok || routine == 0 {
		c.discarded++
		return
	}
	// A routine cannot end before it started, and none lasts a second: such a
	// value means the entry time is not in the clock assumed here.
	d := ts - int64(entry)
	if d < 0 || d > c.freq {
		c.discarded++
		return
	}
	k := routineKey{routine, isr}
	r := c.routines[k]
	if r == nil {
		r = &routineAgg{}
		c.routines[k] = r
	}
	r.count++
	r.totalTicks += d
	if d > r.maxTicks {
		r.maxTicks = d
	}
	r.hist[bucketOf(float64(d)*1e6/float64(c.freq))]++
}

// bucketOf returns the index of the duration bucket (µs) a routine falls in.
func bucketOf(us float64) int {
	for i, edge := range sys.DPCHistBoundsUS {
		if us <= edge {
			return i
		}
	}
	return len(sys.DPCHistBoundsUS)
}

// stats groups the routines by kernel module and kind. A routine whose
// address matches no loaded module goes under the empty name.
func (c *dpcCollector) stats(mm *etw.ModuleMap) []sys.DPCStat {
	c.mu.Lock()
	defer c.mu.Unlock()
	type key struct {
		module, kind string
	}
	byKey := map[key]*sys.DPCStat{}
	toUS := 1e6 / float64(c.freq)
	for k, r := range c.routines {
		kind := sys.KindDPC
		if k.isr {
			kind = sys.KindISR
		}
		kk := key{mm.Lookup(k.addr), kind}
		st := byKey[kk]
		if st == nil {
			st = &sys.DPCStat{Module: kk.module, Kind: kind, Hist: make([]uint64, len(sys.DPCHistBoundsUS)+1)}
			byKey[kk] = st
		}
		st.Count += r.count
		st.TotalUS += float64(r.totalTicks) * toUS
		if m := float64(r.maxTicks) * toUS; m > st.MaxUS {
			st.MaxUS = m
		}
		for i, n := range r.hist {
			st.Hist[i] += n
		}
	}
	out := make([]sys.DPCStat, 0, len(byKey))
	for _, st := range byKey {
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalUS != out[j].TotalUS {
			return out[i].TotalUS > out[j].TotalUS
		}
		return out[i].Module+out[i].Kind < out[j].Module+out[j].Kind
	})
	return out
}

// counts returns how many events were read and how many were discarded.
func (c *dpcCollector) counts() (events, discarded uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.events, c.discarded
}

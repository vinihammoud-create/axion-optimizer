//go:build windows

package winsys

import (
	"encoding/binary"
	"math"
	"testing"

	"axion/internal/sys"
	"axion/internal/sys/etw"
)

const testFreq = 10_000_000 // 10 MHz: 10 ticks are 1 µs

func payload(entry uint64, routine uint64) []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b, entry)
	binary.LittleEndian.PutUint64(b[8:], routine)
	return b
}

// us converts microseconds to ticks at testFreq.
func us(v float64) int64 { return int64(v * 10) }

func TestBucketBoundaries(t *testing.T) {
	// An edge belongs to the bucket it closes ("up to 100 µs").
	for _, c := range []struct {
		us   float64
		want int
	}{{0, 0}, {1, 0}, {1.0001, 1}, {2, 1}, {99.9, 6}, {100, 6}, {100.1, 7}, {10000, 12}, {10000.1, 13}, {5e6, 13}} {
		if got := bucketOf(c.us); got != c.want {
			t.Errorf("bucketOf(%v) = %d, esperado %d", c.us, got, c.want)
		}
	}
	if len(routineAgg{}.hist) != len(sys.DPCHistBoundsUS)+1 {
		t.Error("o histograma precisa de uma faixa a mais que as bordas")
	}
}

func TestCollectorMeasuresDurations(t *testing.T) {
	c := newDPCCollector(testFreq)
	const ndis, nv = 0xfffff80412001000, 0xfffff80413002000
	end := int64(5_000_000)
	// Three DPCs of ndis (3 µs, 40 µs, 600 µs), one ISR of the same driver, and a DPC of the graphics driver.
	for _, d := range []float64{3, 40, 600} {
		c.event(etw.OpDPC, end, 8, payload(uint64(end-us(d)), ndis))
	}
	c.event(etw.OpISR, end, 8, payload(uint64(end-us(8)), ndis))
	c.event(etw.OpTimerDPC, end, 8, payload(uint64(end-us(120)), nv))

	mm := etw.NewModuleMap([]etw.Module{{Base: 0xfffff80412000000, Size: 0x100000, Name: "ndis.sys"}, {Base: 0xfffff80413000000, Size: 0x100000, Name: "nvlddmkm.sys"}})
	stats := c.stats(mm)
	by := map[string]sys.DPCStat{}
	for _, s := range stats {
		by[s.Module+"/"+s.Kind] = s
	}
	n := by["ndis.sys/dpc"]
	if n.Count != 3 || math.Abs(n.MaxUS-600) > 0.01 || math.Abs(n.TotalUS-643) > 0.01 {
		t.Errorf("ndis DPC: %+v", n)
	}
	// 3 µs -> bucket of 5; 40 µs -> 50; 600 µs -> 1000.
	if n.Hist[2] != 1 || n.Hist[5] != 1 || n.Hist[9] != 1 {
		t.Errorf("histograma do ndis: %v", n.Hist)
	}
	if i := by["ndis.sys/isr"]; i.Count != 1 || math.Abs(i.MaxUS-8) > 0.01 || i.Hist[3] != 1 {
		t.Errorf("ndis ISR: %+v", i)
	}
	if g := by["nvlddmkm.sys/dpc"]; g.Count != 1 || math.Abs(g.MaxUS-120) > 0.01 || g.Hist[7] != 1 {
		t.Errorf("gráfica (TimerDPC conta como DPC): %+v", g)
	}
	// Sorted by the time spent: ndis DPC (643 µs) first.
	if stats[0].Module != "ndis.sys" || stats[0].Kind != sys.KindDPC {
		t.Errorf("ordem: %+v", stats[0])
	}
	if ev, disc := c.counts(); ev != 5 || disc != 0 {
		t.Errorf("contagem: %d eventos, %d descartados", ev, disc)
	}
}

func TestCollectorDiscardsWhatCannotBeRight(t *testing.T) {
	c := newDPCCollector(testFreq)
	end := int64(5_000_000)
	good := payload(uint64(end-us(10)), 0xfffff80412001000)
	c.event(etw.OpDPC, end, 8, good)                                                // ok
	c.event(etw.OpDPC, end, 8, payload(uint64(end+us(5)), 0xfffff80412001000))      // entered after it ended
	c.event(etw.OpDPC, end, 8, payload(uint64(end-testFreq-1), 0xfffff80412001000)) // longer than a second
	c.event(etw.OpDPC, end, 8, payload(uint64(end-us(10)), 0))                      // no routine address
	c.event(etw.OpDPC, end, 8, good[:12])                                           // truncated
	c.event(etw.OpDPC, end, 3, good)                                                // invalid pointer size
	c.event(51, end, 8, good)                                                       // not a DPC/ISR (a system call): not even counted
	if ev, disc := c.counts(); ev != 6 || disc != 5 {
		t.Errorf("eventos=%d descartados=%d (esperado 6 e 5)", ev, disc)
	}
	var count uint64
	for _, s := range c.stats(etw.NewModuleMap()) {
		count += s.Count
		if s.Module != "" {
			t.Errorf("endereço sem módulo deveria ficar sem nome: %+v", s)
		}
	}
	if count != 1 {
		t.Errorf("só o evento bom conta: %d", count)
	}
}

func TestKernelModulesAreReadable(t *testing.T) {
	// Needs no privilege: the kernel image and a few drivers are always there.
	mods := kernelModules()
	if len(mods) < 10 {
		t.Skipf("lista de módulos do kernel não disponível (%d)", len(mods))
	}
	mm := etw.NewModuleMap(mods)
	var kernel bool
	for _, m := range mods {
		if m.Name == "ntoskrnl.exe" {
			kernel = true
			if got := mm.Lookup(m.Base + 100); got != "ntoskrnl.exe" {
				t.Errorf("Lookup dentro do kernel: %q", got)
			}
		}
	}
	if !kernel {
		t.Error("ntoskrnl.exe não está na lista")
	}
}

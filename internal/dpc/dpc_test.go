package dpc_test

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"axion/internal/dpc"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

// stat builds a stat whose routines all fall in the given bucket.
func stat(module, kind string, bucket int, n uint64, maxUS float64) sys.DPCStat {
	h := make([]uint64, len(sys.DPCHistBoundsUS)+1)
	h[bucket] = n
	edge := 0.0
	if bucket < len(sys.DPCHistBoundsUS) {
		edge = sys.DPCHistBoundsUS[bucket]
	}
	return sys.DPCStat{Module: module, Kind: kind, Count: n, TotalUS: float64(n) * math.Min(edge, maxUS) / 2, MaxUS: maxUS, Hist: h}
}

func capture(secs int, stats ...sys.DPCStat) sys.DPCCapture {
	c := sys.DPCCapture{Duration: time.Duration(secs) * time.Second, CPUs: 8, Stats: stats}
	for _, s := range stats {
		c.Events += s.Count
	}
	return c
}

// mockCaptures keeps the simulated captures: each one waits 300 ms on purpose
// (so the preview shows progress), which adds up over a test of 40 of them.
var mockCaptures = map[int]sys.DPCCapture{}

func mockCapture(t *testing.T, secs int) sys.DPCCapture {
	t.Helper()
	base, ok := mockCaptures[secs]
	if !ok {
		var err error
		base, err = mocksys.NewNative().CaptureDPC(context.Background(), time.Duration(secs)*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		mockCaptures[secs] = base
	}
	// A copy: tests change the stats of what they get.
	c := base
	c.Stats = make([]sys.DPCStat, len(base.Stats))
	for i, st := range base.Stats {
		st.Hist = append([]uint64(nil), st.Hist...)
		c.Stats[i] = st
	}
	return c
}

func TestMockCaptureIsConsistent(t *testing.T) {
	c := mockCapture(t, 30)
	if c.CPUs != 16 || c.Events == 0 || len(c.Stats) == 0 {
		t.Fatalf("captura: %+v", c)
	}
	for _, s := range c.Stats {
		var n uint64
		for _, h := range s.Hist {
			n += h
		}
		if n != s.Count {
			t.Errorf("%s/%s: histograma soma %d, contagem %d", s.Module, s.Kind, n, s.Count)
		}
		// The maximum must lie in the highest bucket that has routines.
		top := -1
		for i, h := range s.Hist {
			if h > 0 {
				top = i
			}
		}
		lo, hi := 0.0, math.Inf(1)
		if top > 0 {
			lo = sys.DPCHistBoundsUS[top-1]
		}
		if top < len(sys.DPCHistBoundsUS) {
			hi = sys.DPCHistBoundsUS[top]
		}
		if s.MaxUS <= lo && lo > 0 || s.MaxUS > hi {
			t.Errorf("%s/%s: máximo %.0f µs fora da última faixa usada (%.0f, %.0f]", s.Module, s.Kind, s.MaxUS, lo, hi)
		}
	}
}

func TestSummarizeMockPointsAtTheGraphicsDriver(t *testing.T) {
	s := dpc.Summarize(mockCapture(t, 30))
	if !s.Reliable || s.Verdict != dpc.VerdictAttention {
		t.Fatalf("veredito: reliable=%v %s (%s)", s.Reliable, s.Verdict, s.Headline)
	}
	if !strings.Contains(s.Headline, "nvlddmkm.sys") {
		t.Errorf("a manchete deveria apontar o driver gráfico: %q", s.Headline)
	}
	// Numbers in the texts use the decimal comma.
	if !strings.Contains(s.Headline, "1,4 ms") || strings.Contains(s.Headline, "1.4") {
		t.Errorf("vírgula decimal: %q", s.Headline)
	}
	if s.TotalPct <= 0 || s.TotalPct >= 5 {
		t.Errorf("tempo total em rotinas: %.2f%%", s.TotalPct)
	}
	var nv *dpc.Driver
	for i := range s.Drivers {
		if s.Drivers[i].Module == "nvlddmkm.sys" {
			nv = &s.Drivers[i]
		}
		if i > 0 && s.Drivers[i].SharePct > s.Drivers[i-1].SharePct {
			t.Errorf("drivers fora de ordem: %v antes de %v", s.Drivers[i-1].Module, s.Drivers[i].Module)
		}
	}
	if nv == nil || nv.Verdict != dpc.VerdictAttention || nv.DPC.Slow == 0 || nv.DPC.Over == 0 || nv.ISR.Slow != 0 {
		t.Fatalf("nvlddmkm: %+v", nv)
	}
	// The others are fine: occasional routines over the limit are not a finding.
	for _, d := range s.Drivers {
		if d.Module != "nvlddmkm.sys" && d.Verdict != dpc.VerdictOK {
			t.Errorf("%s não devia ser alerta: %s (%s)", d.Module, d.Verdict, d.Why)
		}
	}
}

func TestLimitsFollowMicrosoftsNumbers(t *testing.T) {
	// 150 µs is above the 100 µs DPC limit but below the 4 ms severe line.
	s := dpc.Summarize(capture(30, stat("a.sys", sys.KindDPC, 7, 5000, 240)))
	if s.DPC.Over != 5000 || s.DPC.Slow != 0 {
		t.Errorf("DPC de 100 a 250 µs: over=%d slow=%d", s.DPC.Over, s.DPC.Slow)
	}
	// 40 µs is fine for a DPC and over the 25 µs ISR limit.
	s = dpc.Summarize(capture(30, stat("a.sys", sys.KindDPC, 5, 5000, 50), stat("b.sys", sys.KindISR, 5, 5000, 50)))
	if s.DPC.Over != 0 || s.ISR.Over != 5000 {
		t.Errorf("40 µs: dpc over=%d isr over=%d", s.DPC.Over, s.ISR.Over)
	}
	// Exactly 100 µs is not above the limit (the bucket closes at 100).
	s = dpc.Summarize(capture(30, stat("a.sys", sys.KindDPC, 6, 5000, 100)))
	if s.DPC.Over != 0 {
		t.Errorf("100 µs não passa do limite: over=%d", s.DPC.Over)
	}
}

func TestOccasionalSlowRoutinesAreNotAFinding(t *testing.T) {
	mk := func(over uint64, total uint64) dpc.Driver {
		fast := stat("d.sys", sys.KindDPC, 3, total-over, 9)
		slow := stat("d.sys", sys.KindDPC, 7, over, 240)
		fast.Count += slow.Count
		for i := range fast.Hist {
			fast.Hist[i] += slow.Hist[i]
		}
		fast.MaxUS = 240
		s := dpc.Summarize(capture(30, fast))
		return s.Drivers[0]
	}
	// Over Microsoft's 100 µs is shown (Over) but never the verdict on its own,
	// however many: a healthy PC goes over it all the time.
	for _, c := range []struct{ over, total uint64 }{{19, 10_000}, {20, 100_000}, {20, 1_000}, {900, 1_000}} {
		d := mk(c.over, c.total)
		if d.Verdict != dpc.VerdictOK || d.DPC.Over != c.over {
			t.Errorf("%d de %d acima do limite: veredito %s, contagem %d", c.over, c.total, d.Verdict, d.DPC.Over)
		}
	}
}

// A real capture of this project's author's laptop, idle and healthy (10 s, 16
// logical processors): hundreds of DPCs over 100 µs, thousands of ISRs over
// 25 µs, the longest routine 608 µs. It must not be called a problem.
func TestHealthyIdleLaptopFromARealCapture(t *testing.T) {
	hist := func(counts map[int]uint64) []uint64 {
		h := make([]uint64, len(sys.DPCHistBoundsUS)+1)
		var total uint64
		for i, n := range counts {
			h[i] = n
			total += n
		}
		return h
	}
	mk := func(module, kind string, counts map[int]uint64, maxUS, totalUS float64) sys.DPCStat {
		var n uint64
		for _, c := range counts {
			n += c
		}
		return sys.DPCStat{Module: module, Kind: kind, Count: n, TotalUS: totalUS, MaxUS: maxUS, Hist: hist(counts)}
	}
	c := sys.DPCCapture{Duration: 10 * time.Second, CPUs: 16, Discarded: 1, Stats: []sys.DPCStat{
		mk("Wdf01000.sys", sys.KindISR, map[int]uint64{0: 9000, 1: 7000, 2: 3000, 3: 1000, 4: 628, 5: 2000, 6: 500, 7: 115}, 111, 540_000),
		mk("Wdf01000.sys", sys.KindDPC, map[int]uint64{1: 2000, 2: 2500, 3: 1000, 4: 400, 6: 84}, 462, 100_000),
		mk("ntoskrnl.exe", sys.KindDPC, map[int]uint64{0: 2000, 1: 2500, 2: 2000, 3: 800, 4: 400, 5: 89, 6: 0, 7: 400, 8: 60, 9: 17}, 608, 190_000),
		mk("ndis.sys", sys.KindDPC, map[int]uint64{2: 60, 3: 40, 8: 1}, 508, 4_000),
	}}
	for _, st := range c.Stats {
		c.Events += st.Count
	}
	s := dpc.Summarize(c)
	if !s.Reliable || s.Verdict != dpc.VerdictOK {
		t.Fatalf("PC parado e saudável: %s (%s)", s.Verdict, s.Headline)
	}
	if s.DPC.Over < 400 || s.ISR.Over < 2000 {
		t.Errorf("as contagens acima dos limites continuam à mostra: dpc=%d isr=%d", s.DPC.Over, s.ISR.Over)
	}
	if !strings.Contains(s.Headline, "Sem atrasos relevantes") {
		t.Errorf("manchete: %q", s.Headline)
	}
	for _, d := range s.Drivers {
		if d.Verdict != dpc.VerdictOK {
			t.Errorf("%s: %s (%s)", d.Module, d.Verdict, d.Why)
		}
	}
}

func TestADriverTakingARealSliceIsAttention(t *testing.T) {
	// 5% of 8 CPUs for 10 s is 4 s inside one driver, even with no long routine.
	st := stat("busy.sys", sys.KindDPC, 3, 1000, 10)
	st.TotalUS = 4e6
	s := dpc.Summarize(capture(10, st))
	if d := s.Drivers[0]; d.Verdict != dpc.VerdictAttention || !strings.Contains(d.Why, "5,0%") {
		t.Errorf("%+v", d)
	}
	st.TotalUS = 3.9e6
	if d := dpc.Summarize(capture(10, st)).Drivers[0]; d.Verdict != dpc.VerdictOK {
		t.Errorf("4,9%%: %s", d.Verdict)
	}
}

func TestSlowAndSevereRoutines(t *testing.T) {
	// Longer than 1 ms: attention, even a single one.
	s := dpc.Summarize(capture(30, stat("x.sys", sys.KindDPC, 3, 5000, 8), stat("g.sys", sys.KindDPC, 10, 1, 1800)))
	var g dpc.Driver
	for _, d := range s.Drivers {
		if d.Module == "g.sys" {
			g = d
		}
	}
	if g.Verdict != dpc.VerdictAttention || s.Verdict != dpc.VerdictAttention {
		t.Errorf("1,8 ms: driver %s, geral %s", g.Verdict, s.Verdict)
	}
	// Longer than 4 ms: important.
	s = dpc.Summarize(capture(30, stat("x.sys", sys.KindDPC, 3, 5000, 8), stat("g.sys", sys.KindISR, 12, 1, 8000)))
	if s.Verdict != dpc.VerdictImportant || !strings.Contains(s.Headline, "g.sys") {
		t.Errorf("8 ms: %s (%s)", s.Verdict, s.Headline)
	}
}

func TestShareOfTheMachineAndPercentiles(t *testing.T) {
	// 8 CPUs for 10 s = 80 s of processor time. 4 s in DPCs is 5%.
	st := stat("a.sys", sys.KindDPC, 3, 1000, 10)
	st.TotalUS = 4e6
	s := dpc.Summarize(capture(10, st))
	if math.Abs(s.TotalPct-5) > 0.001 || s.Verdict != dpc.VerdictAttention {
		t.Errorf("5%% do processador: %.3f %s", s.TotalPct, s.Verdict)
	}
	if math.Abs(s.Drivers[0].SharePct-5) > 0.001 {
		t.Errorf("parcela do driver: %.3f", s.Drivers[0].SharePct)
	}
	st.TotalUS = 12e6 // 15%
	if v := dpc.Summarize(capture(10, st)).Verdict; v != dpc.VerdictImportant {
		t.Errorf("15%%: %s", v)
	}
	// 99% of 1000 routines in the 5 µs bucket: percentile answers 5 µs (capped by the maximum).
	if p := dpc.Summarize(capture(10, stat("a.sys", sys.KindDPC, 2, 1000, 5))).DPC.P99US; p != 5 {
		t.Errorf("P99 = %v", p)
	}
}

func TestInconclusiveCapturesSayWhy(t *testing.T) {
	good := stat("a.sys", sys.KindDPC, 3, 5000, 9)
	for name, c := range map[string]sys.DPCCapture{
		"vazia":                  {Duration: 30 * time.Second, CPUs: 8},
		"poucos":                 capture(30, stat("a.sys", sys.KindDPC, 3, 300, 9)),
		"cancelada":              func() sys.DPCCapture { c := capture(30, good); c.Cancelled = true; return c }(),
		"relógio":                func() sys.DPCCapture { c := capture(30, good); c.Discarded = c.Events; return c }(),
		"20% e um":               func() sys.DPCCapture { c := capture(30, good); c.Discarded = c.Events/5 + 50; return c }(),
		"sem estatística válida": {Duration: 30 * time.Second, CPUs: 8, Events: 9999, Stats: []sys.DPCStat{{Module: "a", Kind: "dpc", Count: 9999, Hist: []uint64{1, 2}}}},
	} {
		s := dpc.Summarize(c)
		if s.Reliable || s.Verdict != dpc.VerdictUnknown || s.Headline == "" {
			t.Errorf("%s: reliable=%v verdict=%s headline=%q", name, s.Reliable, s.Verdict, s.Headline)
		}
	}
	// A little discarded is only noted; lost events and missing drivers are noted too.
	c := capture(30, good)
	c.Discarded, c.LostEvents, c.NoModules = 10, uint32(c.Events/4), true
	s := dpc.Summarize(c)
	if !s.Reliable || len(s.Notes) != 3 {
		t.Errorf("notas: reliable=%v %v", s.Reliable, s.Notes)
	}
}

func TestUnknownModuleIsKeptNotInvented(t *testing.T) {
	c := capture(30, stat("", sys.KindDPC, 3, 5000, 9), stat("", sys.KindISR, 2, 3000, 4))
	c.NoModules = true
	s := dpc.Summarize(c)
	if len(s.Drivers) != 1 || s.Drivers[0].Module != "" || s.Drivers[0].DPC.Count != 5000 || s.Drivers[0].ISR.Count != 3000 {
		t.Errorf("sem módulos: %+v", s.Drivers)
	}
}

func TestStoreHistoryAndCompare(t *testing.T) {
	dir := t.TempDir()
	save := func(id, label string, c sys.DPCCapture, ago time.Duration) *dpc.Run {
		r := &dpc.Run{ID: id, At: time.Now().Add(-ago), Label: label, Seconds: 30, Capture: c}
		if err := dpc.Save(dir, r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	before := save("aaaaaaaa0001", "antes", mockCapture(t, 30), time.Hour)
	// After the fix: the graphics driver's slow DPCs are gone.
	fixed := mockCapture(t, 30)
	for i, st := range fixed.Stats {
		if st.Module == "nvlddmkm.sys" && st.Kind == sys.KindDPC {
			fixed.Stats[i] = stat("nvlddmkm.sys", sys.KindDPC, 5, st.Count, 48)
		}
	}
	after := save("aaaaaaaa0002", "depois", fixed, time.Minute)

	lines := dpc.List(dir)
	if len(lines) != 2 || lines[0].ID != "aaaaaaaa0002" || lines[1].Label != "antes" {
		t.Fatalf("histórico: %+v", lines)
	}
	if lines[1].WorstDriver != "nvlddmkm.sys" || lines[1].Verdict != dpc.VerdictAttention || lines[0].Verdict != dpc.VerdictOK {
		t.Errorf("linhas: %+v | %+v", lines[1], lines[0])
	}
	got, err := dpc.Load(dir, before.ID)
	if err != nil || got.Summary.Verdict != dpc.VerdictAttention {
		t.Fatalf("Load: %v %+v", err, got)
	}
	ra, _ := dpc.Load(dir, before.ID)
	rb, _ := dpc.Load(dir, after.ID)
	verdict := map[string]string{}
	for _, row := range dpc.Compare(ra, rb) {
		verdict[row.Label] = row.Verdict
	}
	if verdict["Maior DPC"] != "melhor" || verdict["nvlddmkm.sys — maior rotina"] != "melhor" {
		t.Errorf("comparação: %v", verdict)
	}
	if verdict["Maior interrupção (ISR)"] != "igual" {
		t.Errorf("a ISR não mudou: %v", verdict["Maior interrupção (ISR)"])
	}
	// A capture that cannot be trusted has nothing to compare.
	bad := save("aaaaaaaa0003", "ruim", sys.DPCCapture{Duration: 30 * time.Second, CPUs: 8}, 0)
	rc, _ := dpc.Load(dir, bad.ID)
	for _, row := range dpc.Compare(ra, rc) {
		if row.Verdict != "sem dados" {
			t.Errorf("sem dados esperado: %+v", row)
		}
	}
	// Relabel, bad ids, delete.
	if r, err := dpc.Relabel(dir, after.ID, "  teste\x01  "); err != nil || r.Label != "teste" {
		t.Errorf("Relabel: %v %+v", err, r)
	}
	for _, id := range []string{"", "../x", "ZZZZZZZZ", "abc"} {
		if _, err := dpc.Load(dir, id); err == nil {
			t.Errorf("id %q aceito", id)
		}
		if dpc.Delete(dir, id) == nil {
			t.Errorf("Delete aceitou %q", id)
		}
	}
	if err := dpc.Delete(dir, bad.ID); err != nil || len(dpc.List(dir)) != 2 {
		t.Errorf("Delete: %v, restam %d", err, len(dpc.List(dir)))
	}
	// The history is trimmed.
	for i := 0; i < dpc.MaxRuns+3; i++ {
		save(fmt.Sprintf("bbbbbbbb%04x", i), "x", mockCapture(t, 5), time.Duration(i)*time.Second)
	}
	if n := len(dpc.List(dir)); n != dpc.MaxRuns {
		t.Errorf("histórico guarda %d, esperado %d", n, dpc.MaxRuns)
	}
}

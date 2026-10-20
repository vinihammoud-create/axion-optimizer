package diag_test

import (
	"strings"
	"testing"
	"time"

	"axion/internal/diag"
	"axion/internal/dpc"
	"axion/internal/profile"
	"axion/internal/sys"
)

// dpcStat builds a stat whose routines all fall in the given bucket.
func dpcStat(module, kind string, bucket int, n uint64, maxUS float64) sys.DPCStat {
	h := make([]uint64, len(sys.DPCHistBoundsUS)+1)
	h[bucket] = n
	return sys.DPCStat{Module: module, Kind: kind, Count: n, TotalUS: float64(n) * 5, MaxUS: maxUS, Hist: h}
}

func dpcSummary(stats ...sys.DPCStat) *dpc.Summary {
	c := sys.DPCCapture{Duration: 30 * time.Second, CPUs: 8, Stats: stats}
	for _, s := range stats {
		c.Events += s.Count
	}
	s := dpc.Summarize(c)
	return &s
}

func dpcFinding(t *testing.T, m *diag.Measured) *diag.Finding {
	t.Helper()
	return find(diag.BuildWith(base(), nil, m), "dpc.")
}

func TestDPCFindingNamesTheSlowDriver(t *testing.T) {
	slow := dpcSummary(dpcStat("ndis.sys", sys.KindDPC, 3, 5000, 9), dpcStat("nvlddmkm.sys", sys.KindDPC, 10, 3, 1800))
	x := dpcFinding(t, &diag.Measured{DPC: slow, DPCAt: time.Now().Add(-2 * time.Hour), DPCLabel: "antes"})
	if x == nil {
		t.Fatal("sem achado para um driver com DPC de 1,8 ms")
	}
	if x.ID != "dpc.slow" || x.Severity != diag.SevWarning || x.Area != diag.AreaLatencia || x.Title != "Rotinas lentas de driver: nvlddmkm.sys" {
		t.Errorf("achado: %+v", x)
	}
	if !strings.Contains(x.Evidence, "(antes)") || !strings.Contains(x.Explanation, "Placa de vídeo") || !strings.Contains(x.Explanation, "1 ms") {
		t.Errorf("texto: %q | %q", x.Evidence, x.Explanation)
	}
	pages := map[string]bool{}
	for _, a := range x.Actions {
		pages[a.Page] = true
	}
	if !pages["dpc"] || !pages["drivers"] || len(x.Sources) == 0 || !strings.HasPrefix(x.Sources[0].URL, "https://learn.microsoft.com/") {
		t.Errorf("ações e fontes: %+v %+v", x.Actions, x.Sources)
	}
	// A routine over 4 ms is stronger wording, still a warning (it must not cap the score as a critical would).
	bad := dpcSummary(dpcStat("USBXHCI.SYS", sys.KindISR, 12, 2, 8000), dpcStat("ndis.sys", sys.KindDPC, 3, 5000, 9))
	y := dpcFinding(t, &diag.Measured{DPC: bad, DPCAt: time.Now()})
	if y == nil || y.Title != "Rotinas muito lentas de driver: USBXHCI.SYS" || y.Severity != diag.SevWarning {
		t.Errorf("acima de 4 ms: %+v", y)
	}
}

func TestDPCFindingStaysQuiet(t *testing.T) {
	good := dpcSummary(dpcStat("ndis.sys", sys.KindDPC, 3, 5000, 9))
	slow := dpcSummary(dpcStat("ndis.sys", sys.KindDPC, 3, 5000, 9), dpcStat("nvlddmkm.sys", sys.KindDPC, 10, 3, 1800))
	unreliable := dpcSummary() // nothing read
	for name, m := range map[string]*diag.Measured{
		"sem medição":          {},
		"medição sem problema": {DPC: good, DPCAt: time.Now()},
		"medição inconclusiva": {DPC: unreliable, DPCAt: time.Now()},
		"medição antiga":       {DPC: slow, DPCAt: time.Now().Add(-60 * 24 * time.Hour)},
	} {
		if x := dpcFinding(t, m); x != nil {
			t.Errorf("%s: não devia haver achado, veio %+v", name, x)
		}
	}
	// A measurement with no date still counts (nothing says it is old).
	if x := dpcFinding(t, &diag.Measured{DPC: slow}); x == nil {
		t.Error("medição sem data deveria valer")
	}
}

func TestDPCFindingWithoutADriverToBlame(t *testing.T) {
	// Two drivers at 3,1% each (7,5e6 µs of the 240e6 µs of processor time in 30 s on 8 CPUs):
	// none reaches 5%, but together they take 6,2%.
	a, b := dpcStat("a.sys", sys.KindDPC, 3, 5000, 9), dpcStat("b.sys", sys.KindDPC, 3, 5000, 9)
	a.TotalUS, b.TotalUS = 7.5e6, 7.5e6
	x := dpcFinding(t, &diag.Measured{DPC: dpcSummary(a, b), DPCAt: time.Now()})
	if x == nil || !strings.Contains(x.Title, "gastam muito do processador") {
		t.Errorf("sem um driver para culpar: %+v", x)
	}
	// One driver with the slice and no long routine: the title says it takes a lot, not that it is slow.
	c := dpcStat("c.sys", sys.KindDPC, 3, 5000, 9)
	c.TotalUS = 15e6
	y := dpcFinding(t, &diag.Measured{DPC: dpcSummary(c), DPCAt: time.Now()})
	if y == nil || y.Title != "Driver ocupando muito do processador: c.sys" {
		t.Errorf("driver que ocupa muito: %+v", y)
	}
}

// The finding is about games and streaming: on a PC used for work it stays
// out of the score.
func TestDPCFindingIsGamingScope(t *testing.T) {
	if got := profile.FindingScope("dpc.slow"); got != profile.ScopeGaming {
		t.Errorf("escopo: %s", got)
	}
}

package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/games"
	"axion/internal/netdiag"
	"axion/internal/perf"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// 0.6.3: the diagnostic uses what the Axion measured. A saved FPS capture of
// CS2 limited by the GPU and the newest complete network test reach the
// report (and the export), and the hardware errors go to the export with
// what the classification used.
func TestReportUsesMeasurements(t *testing.T) {
	_, c := setupServer(t)
	a := c.s.App
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	if m := a.measured(); m.Net != nil || m.Perf != nil {
		t.Fatalf("sem medições: %+v", m)
	}
	// A network test with a grade and an older one without: the graded one wins.
	old := &netdiag.Run{ID: "aaaa0001", At: time.Now().Add(-6 * 24 * time.Hour), Label: "antes"}
	if err := netdiag.Save(a.netDir(), old); err != nil {
		t.Fatal(err)
	}
	phase := func(dir string, mbps, added float64) netdiag.LoadPhase {
		return netdiag.LoadPhase{Dir: dir, Mbps: mbps, AddedMs: added, LoadedMs: 12 + added, Internet: netdiag.Stats{Sent: 20}}
	}
	graded := &netdiag.Run{ID: "aaaa0002", At: time.Now().Add(-5 * 24 * time.Hour), Label: "QoS 700/100",
		Bloat: &netdiag.Bloat{Grade: "C", AddedMs: 125.2, Phases: []netdiag.LoadPhase{phase("download", 485.7, 21.8), phase("upload", 24.7, 125.2)}}}
	if err := netdiag.Save(a.netDir(), graded); err != nil {
		t.Fatal(err)
	}
	newest := &netdiag.Run{ID: "aaaa0003", At: time.Now().Add(-4 * 24 * time.Hour), Label: "incompleto"}
	if err := netdiag.Save(a.netDir(), newest); err != nil {
		t.Fatal(err)
	}
	// A CS2 capture limited by the GPU.
	run := &perf.Run{ID: perf.NewID(time.Now().Add(-2 * 24 * time.Hour)), Process: "cs2.exe", Started: time.Now().Add(-2 * 24 * time.Hour), Requested: 60, PresentMon: "2.6.0",
		Summary: perf.Summary{Frames: 22232, Seconds: 59.98, AvgFPS: 370.6, Low1FPS: 173.9, GPUBoundPct: 64.5, CPUBoundPct: 28.2, Limiter: "gpu"}}
	if err := perf.Save(a.perfDir(), run); err != nil {
		t.Fatal(err)
	}
	a.measurementsChanged() // the files were written directly, not through the app
	m := a.measured()
	if m.Net == nil || m.Net.Grade != "C" || m.Net.Label != "QoS 700/100" || m.Perf == nil || m.Perf.GPUBoundPct != 64.5 || m.PerfGame != "Counter-Strike 2" {
		t.Fatalf("medições escolhidas: net %+v perf %+v", m.Net, m.Perf)
	}
	// A short capture does not count.
	short := &perf.Run{ID: perf.NewID(time.Now().Add(-time.Hour)), Process: "cs2.exe", Started: time.Now().Add(-time.Hour), Summary: perf.Summary{Frames: 300, Seconds: 5, GPUBoundPct: 90, Limiter: "gpu"}}
	if err := perf.Save(a.perfDir(), short); err != nil {
		t.Fatal(err)
	}
	a.measurementsChanged()
	if m := a.measured(); m.Perf == nil || m.Perf.Frames != 22232 || m.NetLatest == nil || m.NetLatest.Label != "incompleto" {
		t.Errorf("medição curta não vale; o teste mais novo é o incompleto: %+v %+v", m.Perf, m.NetLatest)
	}
	rep := a.Report()
	var bloat *diag.Finding
	for i := range rep.Findings {
		if rep.Findings[i].ID == "net.bloat" {
			bloat = &rep.Findings[i]
		}
	}
	if bloat == nil || bloat.Severity != diag.SevWarning || !strings.Contains(bloat.Evidence, "QoS 700/100") {
		t.Fatalf("bufferbloat no diagnóstico: %+v", bloat)
	}
	ex := a.exportRaw()
	b, _ := json.Marshal(ex["measured"])
	if mm := ex["measured"].(*diag.Measured); mm.Net == nil || mm.Net.Grade != "C" || !strings.Contains(string(b), "gpuBoundPct") {
		t.Errorf("medições exportadas: %s", b)
	}
	rows, ok := ex["whea"].([]diag.WHEARow)
	if !ok || len(rows) != 1 || rows[0].Kind != "cpu" || rows[0].Fatal {
		t.Errorf("eventos WHEA exportados: %+v", ex["whea"])
	}
	if ex["cs2Page"] == nil {
		t.Errorf("página do CS2 exportada")
	}
	// Deleting the graded test takes its finding out of the report at once.
	if err := a.DeleteNetRun(NetIDRequest{ID: "aaaa0002"}); err != nil {
		t.Fatal(err)
	}
	for _, x := range a.Report().Findings {
		if x.ID == "net.bloat" {
			t.Errorf("achado de um teste apagado: %+v", x)
		}
	}
	// A dismissed finding is not resurrected by the rebuild: nothing to check
	// here beyond the scan being free of fake driver problems from the mock.
	for _, x := range a.Report().Findings {
		if x.ID == "pnp.problems" {
			t.Errorf("o mock não deve gerar dispositivos com problema: %+v", x)
		}
	}
}

// 0.6.3: the FPS cap item shows the fps_max the account saved.
func TestFPSCapCurrent(t *testing.T) {
	f := &facts.Facts{}
	info := games.CS2Info{Installed: true, Convars: map[string]string{"fps_max": "350"}}
	by := map[string]CheckItem{}
	for _, it := range buildCS2Guide(f, info).Tips {
		by[it.ID] = it
	}
	if by["tip.fpscap"].Current != "fps_max 350" {
		t.Errorf("fps_max: %+v", by["tip.fpscap"])
	}
	info.Convars["fps_max"] = "0"
	for _, it := range buildCS2Guide(f, info).Tips {
		if it.ID == "tip.fpscap" && it.Current != "fps_max 0 (sem limite)" {
			t.Errorf("sem limite: %+v", it)
		}
	}
	info.Convars = nil
	for _, it := range buildCS2Guide(f, info).Tips {
		if it.ID == "tip.fpscap" && it.Current != "" {
			t.Errorf("não lido: %+v", it)
		}
	}
}

// 0.6.3: after an apply, the stability findings are recomputed from the
// events of the analysis with the new states, so the ASPM test leaves the
// WHEA finding once the plan runs without PCIe power saving.
func TestStabilityRecomputedWithStates(t *testing.T) {
	_, c := setupServer(t)
	a := c.s.App
	boot := time.Now().Add(-10 * 24 * time.Hour)
	gpu := map[string]string{"PrimaryDeviceName": `PCI\VEN_10DE&DEV_2504&SUBSYS_397D1462&REV_A1`}
	evs := []sys.Event{{Provider: "Microsoft-Windows-Kernel-General", ID: 12, Level: 4, Time: boot}}
	for i := 1; i <= 3; i++ {
		evs = append(evs, sys.Event{Provider: "Microsoft-Windows-WHEA-Logger", ID: 17, Level: 3, Time: boot.Add(time.Duration(i) * 30 * time.Hour), Data: gpu})
	}
	a.mu.Lock()
	a.events = evs
	a.mu.Unlock()
	f := &facts.Facts{OS: facts.OSInfo{LastBoot: boot}}
	whea := func(st map[string]tweaks.State) diag.Finding {
		for _, x := range a.restab(f, st) {
			if x.ID == "stab.whea" {
				return x
			}
		}
		t.Fatal("sem achado WHEA")
		return diag.Finding{}
	}
	if x := whea(nil); len(x.Actions) != 1 || x.Severity != diag.SevInfo {
		t.Fatalf("antes: %+v", x)
	}
	if x := whea(map[string]tweaks.State{"power.pcie.aspm": {ID: "power.pcie.aspm", Status: "on"}}); len(x.Actions) != 0 {
		t.Fatalf("com o ASPM aplicado: %+v", x)
	}
	// Without events (an older Axion kept none) the findings of the analysis stay.
	a.mu.Lock()
	a.events, a.stability = nil, []diag.Finding{{ID: "stab.whea", Title: "guardado"}}
	a.mu.Unlock()
	if got := a.restab(f, nil); len(got) != 1 || got[0].Title != "guardado" {
		t.Fatalf("sem eventos: %+v", got)
	}
}

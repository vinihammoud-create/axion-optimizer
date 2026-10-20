package app

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"axion/internal/perf"
	"axion/internal/sys"
)

func TestGPUFieldsHaveBothNames(t *testing.T) {
	for _, prefix := range []string{gpuThrottleOld, gpuThrottleNew} {
		f := gpuFields(prefix)
		if n := strings.Count(f, ",") + 1; n != gpuFieldCount {
			t.Errorf("%s: %d campos, esperado %d", prefix, n, gpuFieldCount)
		}
		for _, want := range []string{prefix + ".hw_thermal_slowdown", prefix + ".sw_power_cap", "temperature.gpu", "clocks.max.sm"} {
			if !strings.Contains(f, want) {
				t.Errorf("faltou %s em %s", want, f)
			}
		}
	}
}

func TestParseGPULine(t *testing.T) {
	// A real line of this project's author's notebook (idle): only GPU idle active, power limit not reported.
	s, ok := parseGPULine("Not Active, Not Active, Not Active, Not Active, Not Active, 36, 1732, 2100, [N/A], 2")
	if !ok || s.Thermal || s.HWSlowdown || s.PowerCap || s.TempC != 36 || s.ClockMHz != 1732 || s.MaxClockMHz != 2100 || s.PowerW != -1 || s.UtilPct != 2 {
		t.Errorf("repouso: %+v %v", s, ok)
	}
	s, ok = parseGPULine("Not Active, Active, Not Active, Active, Active, 88, 1500, 2100, 55.40, 99\r")
	if !ok || !s.Thermal || !s.HWSlowdown || !s.PowerCap || s.TempC != 88 || s.PowerW != 55.4 || s.UtilPct != 99 {
		t.Errorf("carga: %+v %v", s, ok)
	}
	// Hardware thermal alone counts as thermal; brake alone as hardware slowdown.
	if s, _ := parseGPULine("Active, Not Active, Not Active, Not Active, Not Active, 90, 1, 1, 1, 1"); !s.Thermal || s.HWSlowdown {
		t.Errorf("térmico de hardware: %+v", s)
	}
	if s, _ := parseGPULine("Not Active, Not Active, Not Active, Active, Not Active, 90, 1, 1, 1, 1"); s.Thermal || !s.HWSlowdown {
		t.Errorf("freio de potência: %+v", s)
	}
	// "[Not Supported]" and a header or broken line are not readings.
	if s, ok := parseGPULine("Not Active, Not Active, Not Active, Not Active, Not Active, [Not Supported], 1, 1, [Not Supported], [Not Supported]"); !ok || s.TempC != 0 || s.PowerW != -1 || s.UtilPct != -1 {
		t.Errorf("não suportado: %+v %v", s, ok)
	}
	for _, bad := range []string{"", "NVIDIA-SMI has failed", "1, 2, 3"} {
		if _, ok := parseGPULine(bad); ok {
			t.Errorf("%q aceito", bad)
		}
	}
}

func TestBusyPrograms(t *testing.T) {
	window := 10 * time.Second // 1e8 units of 100 ns: 1e8 = one core for the whole window
	before := map[string]int64{"chrome.exe": 1_000, "OneDrive.exe": 0, "cs2.exe": 0, "System": 0, "tiny.exe": 0, "restarted.exe": 9_000_000_000}
	after := map[string]int64{"chrome.exe": 1_000 + 90_000_000, "OneDrive.exe": 30_000_000, "cs2.exe": 95_000_000, "System": 50_000_000,
		"tiny.exe": 2_000_000, "restarted.exe": 100, "MsMpEng.exe": 26_000_000, "PresentMon-2.6.0-x64.exe": 40_000_000, "dwm.exe": 60_000_000}
	got := busyPrograms(before, after, window, busySkipped("cs2.exe"))
	var names []string
	for _, b := range got {
		names = append(names, b.Name)
	}
	if strings.Join(names, ",") != "chrome.exe,OneDrive.exe,MsMpEng.exe" {
		t.Fatalf("ocupados: %v", got)
	}
	if got[0].Pct < 89.9 || got[0].Pct > 90.1 || got[1].Pct < 29.9 || got[1].Pct > 30.1 {
		t.Errorf("percentuais: %+v", got)
	}
	// A short window or a missing reading says nothing.
	if busyPrograms(before, after, time.Second, busySkipped("")) != nil || busyPrograms(nil, after, window, busySkipped("")) != nil || busyPrograms(before, nil, window, busySkipped("")) != nil {
		t.Error("janela curta ou leitura ausente não pode listar nada")
	}
	// Only the five busiest are kept.
	big := map[string]int64{}
	for i := 0; i < 9; i++ {
		big[string(rune('a'+i))+".exe"] = int64(30_000_000 + i*1_000_000)
	}
	if got := busyPrograms(map[string]int64{}, big, window, busySkipped("")); len(got) != 5 || got[0].Name != "i.exe" {
		t.Errorf("limite de cinco: %+v", got)
	}
}

func TestConditionsNow(t *testing.T) {
	m, c := setupServer(t)
	a := c.s.App
	m.N.HasBattery = true
	cond := a.conditionsNow()
	if !cond.HasBattery || cond.OnBattery || cond.BatteryPct != 100 || cond.DisplayHz != 60 || cond.PowerScheme == "" {
		t.Errorf("na tomada: %+v", cond)
	}
	m.N.OnBattery = true
	cond = a.conditionsNow()
	if !cond.OnBattery || cond.BatteryPct != 58 {
		t.Errorf("na bateria: %+v", cond)
	}
	// A desktop has no battery: never "on battery".
	m.N.HasBattery = false
	if cond = a.conditionsNow(); cond.OnBattery || cond.HasBattery || cond.BatteryPct != -1 {
		t.Errorf("desktop: %+v", cond)
	}
}

type perfRunWithSensors struct {
	Conditions *perf.Conditions `json:"conditions"`
	Sensors    *perf.Sensors    `json:"sensors"`
	Remarks    []perf.Remark    `json:"remarks"`
}

func TestCaptureRecordsConditionsAndSensors(t *testing.T) {
	m, c := setupServer(t)
	a := c.s.App
	a.sensorEvery = 20 * time.Millisecond
	m.N.HasBattery, m.N.OnBattery = true, true
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	c.wait(j.ID)
	// In a real game nvidia-smi prints a line a second; here the lines arrive
	// after a short wait, so they fall inside the recording.
	prev := m.X.Handler
	m.X.Handler = func(exe string, args []string, ps string) (sys.Result, bool) {
		if strings.EqualFold(filepath.Base(exe), "nvidia-smi.exe") {
			for _, x := range args {
				if x == "-l" {
					time.Sleep(150 * time.Millisecond)
				}
			}
		}
		return prev(exe, args, ps)
	}
	c.json("POST", "/api/perf/capture", PerfCaptureRequest{Process: "cs2.exe", Seconds: 20, Label: "antes"}, &j)
	done := c.wait(j.ID)
	if done.Status != JobDone {
		t.Fatalf("medição: %+v", done)
	}
	b, _ := json.Marshal(done.Result)
	var r perfRunWithSensors
	_ = json.Unmarshal(b, &r)
	if r.Conditions == nil || !r.Conditions.OnBattery || r.Conditions.BatteryPct != 58 || r.Conditions.DisplayHz != 60 {
		t.Fatalf("condições: %+v", r.Conditions)
	}
	if r.Sensors == nil || r.Sensors.GPU == nil || r.Sensors.GPU.Samples < 3 {
		t.Fatalf("sensores da GPU: %+v", r.Sensors)
	}
	g := r.Sensors.GPU
	if g.PowerCapPct < 99 || g.ThermalPct != 0 || g.MaxTempC != 71 || g.AvgClockMHz != 1950 || g.RatedClockMHz != 2100 || g.AvgPowerW < 55 || g.AvgPowerW > 56 {
		t.Errorf("GPU: %+v", g)
	}
	// The run on disk carries them, with the remarks derived from them.
	var list struct {
		Runs []perf.Run `json:"runs"`
	}
	c.json("GET", "/api/perf", nil, &list)
	if len(list.Runs) != 1 || list.Runs[0].Conditions == nil || len(list.Runs[0].Remarks) < 2 {
		t.Fatalf("a medição guardada: %+v", list.Runs)
	}
	warn, info := 0, 0
	for _, x := range list.Runs[0].Remarks {
		switch x.Level {
		case "warn":
			warn++
		case "info":
			info++
		}
	}
	if warn != 1 || info < 2 { // the battery; the power cap and the GPU line
		t.Errorf("observações: %+v", list.Runs[0].Remarks)
	}

	// The second capture, on the mains: the comparison says the conditions differ.
	m.N.OnBattery = false
	c.json("POST", "/api/perf/capture", PerfCaptureRequest{Process: "cs2.exe", Seconds: 20, Label: "depois"}, &j)
	if done := c.wait(j.ID); done.Status != JobDone {
		t.Fatalf("segunda medição: %+v", done)
	}
	c.json("GET", "/api/perf", nil, &list)
	if len(list.Runs) != 2 {
		t.Fatalf("medições: %d", len(list.Runs))
	}
	var cmp PerfComparison
	c.json("POST", "/api/perf/compare", PerfCompareRequest{A: []string{list.Runs[1].ID}, B: []string{list.Runs[0].ID}, LabelA: "antes", LabelB: "depois"}, &cmp)
	joined := strings.Join(cmp.Warnings, " | ")
	if !strings.Contains(joined, "na bateria e a outra na tomada") || !strings.Contains(joined, "Em \"antes\"") || !strings.Contains(joined, "O PC estava na bateria") {
		t.Errorf("avisos da comparação: %s", joined)
	}
}

// The page says before the measurement, not only after, that the PC is in a
// condition that changes the result.
func TestPerfPageWarnsBeforeMeasuring(t *testing.T) {
	m, c := setupServer(t)
	var v PerfView
	c.json("GET", "/api/perf", nil, &v)
	if len(v.Preflight) != 0 {
		t.Errorf("PC normal: %+v", v.Preflight)
	}
	m.N.HasBattery, m.N.OnBattery = true, true
	c.json("GET", "/api/perf", nil, &v)
	if len(v.Preflight) != 1 || !strings.Contains(v.Preflight[0].Title, "bateria") || v.Preflight[0].Level != "warn" {
		t.Errorf("na bateria: %+v", v.Preflight)
	}
}

func TestCaptureWithoutSensorsStillWorks(t *testing.T) {
	// A PC without the frequency counter and without nvidia-smi: the capture is unaffected.
	m, c := setupServer(t)
	c.s.App.sensorEvery = 10 * time.Millisecond
	m.N.CPUFreqErr = sys.ErrTimeout
	prev := m.X.Handler
	m.X.Handler = func(exe string, args []string, ps string) (sys.Result, bool) {
		if strings.EqualFold(filepath.Base(exe), "nvidia-smi.exe") {
			return sys.Result{ExitCode: 9, Stderr: "NVIDIA-SMI has failed"}, true
		}
		return prev(exe, args, ps)
	}
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	c.wait(j.ID)
	c.json("POST", "/api/perf/capture", PerfCaptureRequest{Process: "cs2.exe", Seconds: 20}, &j)
	done := c.wait(j.ID)
	if done.Status != JobDone {
		t.Fatalf("medição: %+v", done)
	}
	b, _ := json.Marshal(done.Result)
	var r perfRunWithSensors
	_ = json.Unmarshal(b, &r)
	if r.Conditions == nil || r.Sensors != nil {
		t.Errorf("sem sensores: conditions=%+v sensors=%+v", r.Conditions, r.Sensors)
	}
}

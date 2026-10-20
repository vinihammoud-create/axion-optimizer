package perf

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

// Header of PresentMon 2.6.0 without --v1_metrics/--v2_metrics
// (PresentMon/CsvOutput.cpp at the v2.6.0 tag), with --track_pc_latency.
const header26 = "\ufeffApplication,ProcessID,SwapChainAddress,PresentRuntime,SyncInterval,PresentFlags,AllowsTearing,PresentMode,TimeInMs,MsBetweenSimulationStart,MsBetweenPresents,MsBetweenDisplayChange,MsInPresentAPI,MsRenderPresentLatency,MsUntilDisplayed,MsPCLatency,CPUStartTimeInMs,MsBetweenAppStart,MsCPUBusy,MsCPUWait,MsGPULatency,MsGPUTime,MsGPUBusy,MsGPUWait,MsAnimationError,AnimationTime,MsFlipDelay,MsAllInputToPhotonLatency,MsClickToPhotonLatency\n"

func row(app, swap string, t, ft, cpuBusy, cpuWait, gpuBusy float64, mode string, input string) string {
	return fmt.Sprintf("%s,4242,%s,DXGI,0,512,1,%s,%.4f,NA,%.4f,%.4f,0.1000,%.4f,1.5000,NA,%.4f,%.4f,%.4f,%.4f,0.2000,%.4f,%.4f,0.1000,0.0100,NA,NA,%s,NA\n",
		app, swap, mode, t, ft, ft, ft*0.8, t, ft, cpuBusy, cpuWait, gpuBusy, gpuBusy, input)
}

func TestParseCSVKeepsGameSwapchain(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(header26)
	// First frame of the chain (no previous Present) is dropped.
	sb.WriteString(row("cs2.exe", "0xA", 0, 0, 0, 0, 0, "Hardware: Independent Flip", "NA"))
	tm := 0.0
	for i := 0; i < 500; i++ {
		tm += 2.5
		sb.WriteString(row("cs2.exe", "0xA", tm, 2.5, 1.0, 1.4, 2.4, "Hardware: Independent Flip", "NA"))
	}
	// An overlay swap chain in the same process and another program.
	for i := 0; i < 50; i++ {
		sb.WriteString(row("cs2.exe", "0xB", float64(i)*16, 16.6, 1, 15, 1, "Composed: Flip", "NA"))
		sb.WriteString(row("Discord.exe", "0xC", float64(i)*16, 16.6, 1, 15, 1, "Composed: Flip", "NA"))
	}
	sb.WriteString("cs2.exe,4242,0xA,DXGI,0,512,1,Hardware: Indep") // truncated last line
	c, err := ParseCSV(strings.NewReader(sb.String()), "CS2")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Frames) != 500 || c.Swapchains != 2 || c.Process != "cs2.exe" || c.Runtime != "DXGI" {
		t.Fatalf("captura: %d quadros, %d swap chains, %q", len(c.Frames), c.Swapchains, c.Process)
	}
	f := c.Frames[10]
	if f.FrameTime != 2.5 || f.GPUBusy != 2.4 || f.CPUWait != 1.4 || !f.Tearing || f.Mode != "Hardware: Independent Flip" || !math.IsNaN(f.PCLatency) || f.UntilDisp != 1.5 {
		t.Errorf("quadro: %+v", f)
	}
	if _, err := ParseCSV(strings.NewReader(header26), "cs2.exe"); err != ErrNoFrames {
		t.Errorf("CSV sem quadros: %v", err)
	}
	if _, err := ParseCSV(strings.NewReader("a,b,c\n1,2,3\n"), ""); err == nil || err == ErrNoFrames {
		t.Errorf("CSV estranho deveria dar erro de formato: %v", err)
	}
}

// 990 frames of 2.5 ms and 10 hitches of 10 ms: the average hides the
// hitches, the 1% low (mean of the slowest 1%) shows them, and the P1
// percentile falls between the two groups.
func TestSummaryNumbers(t *testing.T) {
	c := &Capture{}
	for i := 0; i < 1000; i++ {
		ft := 2.5
		if i%100 == 50 {
			ft = 10
		}
		c.Frames = append(c.Frames, Frame{FrameTime: ft, Displayed: ft, GPUBusy: 2.4, CPUWait: 0.8, CPUBusy: 1.7, InputPhoton: math.NaN(), ClickPhoton: math.NaN(), PCLatency: math.NaN(), UntilDisp: 2, Mode: "Hardware: Independent Flip"})
	}
	s := Summarize(c)
	check := func(name string, got, want float64) {
		t.Helper()
		if math.Abs(got-want) > 0.05 {
			t.Errorf("%s = %v, esperado %v", name, got, want)
		}
	}
	check("FPS médio", s.AvgFPS, 388.3)
	check("1% low", s.Low1FPS, 100)
	check("0,1% low", s.Low01FPS, 100)
	check("P1", s.P1FPS, 388.3)
	check("P99 (ms)", s.FTP99, 2.575)
	check("máximo", s.FTMax, 10)
	if s.Stutters != 10 {
		t.Errorf("engasgos = %d", s.Stutters)
	}
	check("tempo em engasgos", s.StutterPct, 3.88)
	if s.Limiter != "gpu" || s.GPUBoundPct != 99 || s.PresentMode != "Hardware: Independent Flip" || s.PresentModePct != 100 {
		t.Errorf("limitador/modo: %+v", s)
	}
	if s.InputLatencyAvg != -1 || s.PCLatencyAvg != -1 || s.UntilDisplayedAvg != 2 || s.DroppedPct != 0 {
		t.Errorf("latências: %+v", s)
	}

	// CPU-bound (no waiting, GPU idle part of the frame) and frame-capped.
	for i := range c.Frames {
		c.Frames[i].GPUBusy, c.Frames[i].CPUWait = 1.0, 0.1
	}
	if s := Summarize(c); s.Limiter != "cpu" {
		t.Errorf("CPU: %+v", s)
	}
	for i := range c.Frames {
		c.Frames[i].GPUBusy, c.Frames[i].CPUWait = 1.0, 1.2
	}
	if s := Summarize(c); s.Limiter != "limite" {
		t.Errorf("limitador de FPS: %+v", s)
	}
	// Short captures do not pretend to know the 0.1% low.
	c.Frames = c.Frames[:500]
	if s := Summarize(c); s.Low01FPS != -1 || s.Low1FPS <= 0 {
		t.Errorf("amostra curta: %+v", s)
	}
}

func TestStudentT(t *testing.T) {
	for _, c := range []struct{ t, df, p float64 }{
		{2.776445, 4, 0.05}, {4.604095, 4, 0.01}, {2.228139, 10, 0.05}, {1.959964, 1e6, 0.05}, {0, 5, 1},
	} {
		if got := 2 * studentTail(c.t, c.df); math.Abs(got-c.p) > 2e-5 {
			t.Errorf("t=%v df=%v: p=%v, esperado %v", c.t, c.df, got, c.p)
		}
	}
}

func sums(fpsVals ...float64) []Summary {
	var out []Summary
	for _, v := range fpsVals {
		out = append(out, Summary{AvgFPS: v, Low1FPS: v / 2, Low01FPS: -1, FTP99: 1000 / v * 1.5, FTStdDev: 0.3, StutterPct: 0.1, InputLatencyAvg: -1, PCLatencyAvg: -1, PresentMode: "Hardware: Independent Flip"})
	}
	return out
}

func TestCompare(t *testing.T) {
	// Clear gain, three runs each.
	c := Compare("antes", sums(400, 404, 398), "depois", sums(430, 428, 433))
	r := c.Rows[0]
	if r.Key != "avgFps" || r.Verdict != "melhor" || r.P >= 0.05 || math.Abs(r.DeltaPct-7.5) > 0.3 || !c.Enough {
		t.Errorf("ganho real: %+v", r)
	}
	for _, row := range c.Rows {
		if row.Key == "low01Fps" || row.Key == "inputLatency" {
			t.Errorf("métrica indisponível não deveria aparecer: %s", row.Key)
		}
		if row.Key == "ftP99" && row.Verdict != "melhor" {
			t.Errorf("P99 menor é melhor: %+v", row)
		}
	}
	// Within run-to-run variation.
	if r := Compare("antes", sums(400, 420, 390), "depois", sums(405, 395, 415)).Rows[0]; r.Verdict != "igual" {
		t.Errorf("dentro do ruído: %+v", r)
	}
	// One run each: no verdict, and a warning explaining why.
	c = Compare("antes", sums(400), "depois", sums(440))
	if c.Rows[0].Verdict != "inconclusivo" || c.Enough || len(c.Warnings) == 0 {
		t.Errorf("sem repetições: %+v", c)
	}
	// Noisy scenario is flagged.
	c = Compare("antes", sums(300, 400, 350), "depois", sums(420, 425, 430))
	found := false
	for _, w := range c.Warnings {
		if strings.Contains(w, "\"antes\"") {
			found = true
		}
	}
	if !found {
		t.Errorf("variação alta não sinalizada: %v", c.Warnings)
	}
}

func TestStore(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.Local)
	var ids []string
	for i := 0; i < 4; i++ {
		r := &Run{ID: NewID(now.Add(time.Duration(i) * time.Minute)), Label: "antes", Process: "cs2.exe", Summary: Summary{AvgFPS: float64(400 + i)}, Series: []float32{2.5, 2.6}}
		r.CSV = r.ID + ".csv"
		_ = os.WriteFile(dir+"/"+r.CSV, []byte("x"), 0o644)
		if err := Save(dir, r); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.ID)
	}
	list := List(dir)
	if len(list) != 4 || list[0].ID != ids[3] || list[0].Series != nil {
		t.Fatalf("lista: %+v", list)
	}
	if r, err := Load(dir, ids[1]); err != nil || len(r.Series) != 2 || r.Summary.AvgFPS != 401 {
		t.Fatalf("carregar: %+v %v", r, err)
	}
	if n := Prune(dir, 2); n != 2 || len(List(dir)) != 2 {
		t.Fatalf("poda: %d", n)
	}
	if _, err := os.Stat(dir + "/" + ids[0] + ".csv"); !os.IsNotExist(err) {
		t.Error("CSV da medição apagada deveria sair junto")
	}
	for _, bad := range []string{"../x", "20260926-120000-zzzzzz", "", "20260926-120000-abcdef/../../x"} {
		if ValidID(bad) {
			t.Errorf("ID inválido aceito: %q", bad)
		}
	}
	if CleanLabel("  depois   do\tajuste \x07"+strings.Repeat("x", 60)) != "depois do ajuste "+strings.Repeat("x", 23) {
		t.Errorf("rótulo: %q", CleanLabel("  depois   do\tajuste \x07"+strings.Repeat("x", 60)))
	}
}

// Values rounded to the same number give no variance: a tiny difference is
// "igual", a big one without variance is "inconclusivo" (never certain).
func TestCompareZeroVariance(t *testing.T) {
	mk := func(fps ...float64) []Summary {
		var out []Summary
		for _, v := range fps {
			out = append(out, Summary{AvgFPS: v, Low1FPS: -1, Low01FPS: -1, FTP99: -1, FTStdDev: -1, StutterPct: -1, InputLatencyAvg: -1, PCLatencyAvg: -1})
		}
		return out
	}
	row := func(c Comparison) Row {
		for _, r := range c.Rows {
			if r.Key == "avgFps" {
				return r
			}
		}
		t.Fatal("linha de FPS ausente")
		return Row{}
	}
	if r := row(Compare("antes", mk(400, 400), "depois", mk(400.1, 400.1))); r.Verdict != "igual" {
		t.Errorf("400 x 400,1 sem variação: %s (p=%v)", r.Verdict, r.P)
	}
	if r := row(Compare("antes", mk(400, 400), "depois", mk(420, 420))); r.Verdict != "inconclusivo" || r.P != -1 {
		t.Errorf("sem variação não há teste: %s (p=%v)", r.Verdict, r.P)
	}
	if r := row(Compare("antes", mk(400, 402, 401), "depois", mk(430, 433, 431))); r.Verdict != "melhor" || r.PAdj < r.P {
		t.Errorf("ganho claro: %s p=%v pAdj=%v", r.Verdict, r.P, r.PAdj)
	}
}

func TestHolmAdjustment(t *testing.T) {
	rows := []Row{{P: 0.01}, {P: 0.04}, {P: -1}, {P: 0.03}}
	holm(rows)
	// m = 3: 0.01*3 = 0.03; 0.03*2 = 0.06; 0.04*1 = 0.04 -> monotone 0.06
	want := []float64{0.03, 0.06, -1, 0.06}
	for i, w := range want {
		if math.Abs(rows[i].PAdj-w) > 1e-9 {
			t.Errorf("linha %d: pAdj=%v, esperado %v", i, rows[i].PAdj, w)
		}
	}
}

// The chart's x axis is time: a 500 ms hitch among 2 ms frames takes the
// width of 500 ms, not of one frame.
func TestSeriesByTime(t *testing.T) {
	c := &Capture{}
	for i := 0; i < 1000; i++ {
		c.Frames = append(c.Frames, Frame{FrameTime: 2})
	}
	c.Frames[500].FrameTime = 500 // total 2498 ms; the hitch is ~20% of it
	s := Series(c, 100)
	if len(s) != 100 {
		t.Fatalf("pontos: %d", len(s))
	}
	wide := 0
	for _, v := range s {
		if v >= 500 {
			wide++
		}
	}
	if wide < 15 || wide > 25 {
		t.Errorf("o engasgo deveria ocupar ~20%% do gráfico, ocupou %d de 100 pontos", wide)
	}
}

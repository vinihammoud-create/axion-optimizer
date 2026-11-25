package perf

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

// bigCapture is a deterministic capture that touches every branch of the
// summary: hitches, undisplayed frames, the three limiters, two present modes,
// missing latency and the odd tearing frame.
func bigCapture(n int) *Capture {
	c := &Capture{Process: "cs2.exe", Runtime: "DXGI", Swapchains: 2}
	nan := math.NaN()
	for i := 0; i < n; i++ {
		ft := 2.4 + 0.01*float64(i%7)
		if i%300 == 299 {
			ft = 12
		}
		f := Frame{FrameTime: ft, Displayed: ft, CPUBusy: ft * 0.6, CPUWait: nan, GPUBusy: nan, UntilDisp: 1.5, InputPhoton: nan, ClickPhoton: nan, PCLatency: nan, Mode: "Hardware: Independent Flip", SyncInterval: 0}
		if i%50 == 7 {
			f.Displayed = nan
		}
		if i%2 == 1 {
			f.Mode = "Composed: Flip"
			f.PCLatency = 7 + float64(i%5)
		}
		if i%11 == 0 {
			f.InputPhoton = 8 + float64(i%3)
		}
		if i%97 == 0 {
			f.ClickPhoton = 9
		}
		if i%13 == 0 {
			f.Tearing = true
			f.SyncInterval = 1
		}
		switch i % 3 { // GPU-bound, CPU-bound, waiting on a limit
		case 0:
			f.GPUBusy, f.CPUWait = ft*0.95, ft*0.02
		case 1:
			f.GPUBusy, f.CPUWait = ft*0.5, ft*0.05
		default:
			if i%9 != 2 {
				f.GPUBusy, f.CPUWait = ft*0.4, ft*0.4
			}
		}
		c.Frames = append(c.Frames, f)
	}
	return c
}

func slice(c *Capture, from, to int) *Capture {
	x := *c
	x.Frames = c.Frames[from:to]
	return &x
}

func totalMs(c *Capture) float64 {
	t := 0.0
	for _, f := range c.Frames {
		t += f.FrameTime
	}
	return t
}

// legacySummary is the old Summarize plus the one field added after it, taken
// straight from the frames, so the comparison with the new code still covers
// every other number.
func legacySummary(c *Capture) Summary {
	s := legacySummarize(c)
	if s.Frames == 0 {
		return s
	}
	k := 0
	for _, f := range c.Frames {
		if !math.IsNaN(f.GPUBusy) && !math.IsNaN(f.CPUWait) {
			k++
		}
	}
	s.ClassifiedFrac = -1
	if k > 0 {
		s.ClassifiedFrac = round(float64(k)/float64(s.Frames), 3)
	}
	return s
}

func TestAccumMatchesLegacySummarize(t *testing.T) {
	whole := bigCapture(5000)
	acc := Accum{}
	acc.Add(slice(whole, 0, 2000))
	acc.Add(slice(whole, 2000, 3500))
	acc.Add(nil)
	acc.Add(slice(whole, 3500, 5000))
	want := legacySummary(whole)
	if !reflect.DeepEqual(acc.Summary(), want) {
		t.Fatalf("Accum em 3 partes difere do Summarize antigo:\n%+v\n%+v", acc.Summary(), want)
	}
	if !reflect.DeepEqual(Summarize(whole), want) {
		t.Fatalf("Summarize novo difere do antigo:\n%+v\n%+v", Summarize(whole), want)
	}
	for _, p := range []int{1200, 10, len(whole.Frames) + 5, 1, 0, -3} {
		if got, w := acc.Series(p), legacySeries(whole, p); !reflect.DeepEqual(got, w) {
			t.Errorf("Series(%d) do Accum difere do antigo", p)
		}
		if got, w := Series(whole, p), legacySeries(whole, p); !reflect.DeepEqual(got, w) {
			t.Errorf("Series(%d) novo difere do antigo", p)
		}
	}
	if acc.Frames() != 5000 || math.Abs(acc.Seconds()-totalMs(whole)/1000) > 1e-9 {
		t.Errorf("Frames/Seconds: %d, %v", acc.Frames(), acc.Seconds())
	}
	// Edge sizes around the thresholds of the percentiles.
	for _, n := range []int{0, 1, 2, 99, 100, 999, 1000, 1001} {
		c := bigCapture(n)
		if !reflect.DeepEqual(Summarize(c), legacySummary(c)) {
			t.Errorf("n=%d: Summarize difere do antigo", n)
		}
	}
}

func TestAccumEmpty(t *testing.T) {
	var a Accum
	if s := a.Summary(); s.Frames != 0 || !reflect.DeepEqual(s, Summary{}) {
		t.Errorf("Accum vazio: %+v", s)
	}
	if a.Series(10) != nil || a.Frames() != 0 || a.Seconds() != 0 {
		t.Error("Accum vazio deveria dar série nula e zero quadros")
	}
}

// The same equality over the CSV the simulator writes (PresentMon 2.6.0
// format), read by ParseCSV: the path the real captures take.
func TestAccumMatchesLegacyOnSimulatedCSV(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "x.csv")
	res, err := m.X.Run(context.Background(), `C:\x\PresentMon-2.6.0-x64.exe`, []string{"--process_name", "cs2.exe", "--output_file", path, "--timed", "20"}, sys.RunOpts{})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("simulador: %v %+v", err, res)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c, err := ParseCSV(f, "cs2.exe")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Frames) < 1000 {
		t.Fatalf("poucos quadros: %d", len(c.Frames))
	}
	if !reflect.DeepEqual(Summarize(c), legacySummary(c)) {
		t.Error("Summarize difere do antigo na captura do simulador")
	}
	h := len(c.Frames) / 2
	var acc Accum
	acc.Add(slice(c, 0, h))
	acc.Add(slice(c, h, len(c.Frames)))
	if !reflect.DeepEqual(acc.Summary(), legacySummary(c)) {
		t.Error("Accum em 2 partes difere do antigo na captura do simulador")
	}
	if !reflect.DeepEqual(acc.Series(1200), legacySeries(c, 1200)) {
		t.Error("Series difere do antigo na captura do simulador")
	}
}

func TestSessionRunRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := bigCapture(3000)
	seg := func(from float64, a, b int) SessionSegment {
		return SessionSegment{From: from, Summary: Summarize(slice(c, a, b))}
	}
	started := time.Date(2026, 10, 18, 19, 55, 0, 0, time.UTC)
	want := &Run{ID: NewID(started), Process: "cs2.exe", Started: started, PresentMon: "2.6.0", Summary: Summarize(c), Series: Series(c, 100),
		Session: &SessionInfo{Name: "Counter-Strike 2", Ended: time.Date(2026, 10, 18, 20, 0, 0, 0, time.UTC), Reason: "fim", SegmentSecs: 180,
			Segments: []SessionSegment{seg(0, 0, 1500), seg(3.6, 1500, 3000)}, WallSecs: 7.3, Setting: "fps_max 400"}}
	if err := Save(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir, want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Session == nil || !got.Session.Ended.Equal(want.Session.Ended) {
		t.Fatalf("Session perdida: %+v", got.Session)
	}
	g, w := *got.Session, *want.Session
	g.Ended, w.Ended = time.Time{}, time.Time{}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("Session diferente depois de salvar e ler:\n%+v\n%+v", g, w)
	}
	// An old run (no session) keeps Session nil.
	old := &Run{ID: NewID(started.Add(time.Hour)), Process: "cs2.exe", Started: started.Add(time.Hour), Summary: Summarize(c)}
	if err := Save(dir, old); err != nil {
		t.Fatal(err)
	}
	l := List(dir)
	if len(l) != 2 {
		t.Fatalf("List: %d", len(l))
	}
	for _, r := range l {
		if r.Series != nil {
			t.Error("List deveria tirar a série")
		}
		if (r.ID == want.ID) != (r.Session != nil) {
			t.Errorf("Session em %s: %v", r.ID, r.Session)
		}
	}
	// A sub-folder (partidas) never shows in the list of the parent folder.
	if err := Save(filepath.Join(dir, "partidas"), want); err != nil {
		t.Fatal(err)
	}
	if n := len(List(dir)); n != 2 {
		t.Errorf("subpasta apareceu na lista: %d", n)
	}
}

// classifiedCapture has n frames of 2.5 ms; the first `classified` carry GPU
// Busy and CPU Wait (GPU-bound), the rest have no such columns.
func classifiedCapture(n, classified int) *Capture {
	nan := math.NaN()
	c := &Capture{}
	for i := 0; i < n; i++ {
		f := Frame{FrameTime: 2.5, Displayed: 2.5, CPUBusy: 1.7, CPUWait: nan, GPUBusy: nan, UntilDisp: 2, InputPhoton: nan, ClickPhoton: nan, PCLatency: nan}
		if i < classified {
			f.GPUBusy, f.CPUWait = 2.4, 0.8
		}
		c.Frames = append(c.Frames, f)
	}
	return c
}

func TestLimiterNeedsEnoughClassifiedFrames(t *testing.T) {
	s := Summarize(classifiedCapture(1000, 1))
	if s.Limiter != "" {
		t.Errorf("1 quadro classificado em 1000 não basta para o veredito: %q", s.Limiter)
	}
	if s.ClassifiedFrac != 0.001 {
		t.Errorf("fração classificada = %v, esperado 0.001", s.ClassifiedFrac)
	}
	if s.GPUBoundPct != 100 || s.CPUBoundPct != 0 || s.WaitPct != 0 {
		t.Errorf("os percentuais não mudam: %+v", s)
	}

	s = Summarize(classifiedCapture(1000, 499))
	if s.Limiter != "" {
		t.Errorf("499 de 1000 (abaixo de 50%%): %q", s.Limiter)
	}
	s = Summarize(classifiedCapture(1000, 500))
	if s.Limiter != "gpu" || s.ClassifiedFrac != 0.5 {
		t.Errorf("500 de 1000 (50%%) já dá o veredito: %q, fração %v", s.Limiter, s.ClassifiedFrac)
	}
	s = Summarize(classifiedCapture(1000, 1000))
	if s.Limiter != "gpu" || s.ClassifiedFrac != 1 || s.GPUBoundPct != 100 {
		t.Errorf("todos classificados: %+v", s)
	}
}

func TestLimiterThresholdOverAccumulatedCaptures(t *testing.T) {
	var a Accum
	a.Add(classifiedCapture(600, 0))
	a.Add(classifiedCapture(400, 400))
	if s := a.Summary(); s.Limiter != "" || s.ClassifiedFrac != 0.4 {
		t.Errorf("40%% classificados em duas janelas: %q, fração %v", s.Limiter, s.ClassifiedFrac)
	}
	a.Add(classifiedCapture(1000, 1000))
	if s := a.Summary(); s.Limiter != "gpu" || s.ClassifiedFrac != 0.7 {
		t.Errorf("70%% classificados: %q, fração %v", s.Limiter, s.ClassifiedFrac)
	}
}

func TestClassifiedFracWithoutColumns(t *testing.T) {
	s := Summarize(classifiedCapture(1000, 0))
	if s.ClassifiedFrac != -1 || s.Limiter != "" || s.GPUBoundPct != -1 {
		t.Errorf("sem colunas: %+v", s)
	}
}

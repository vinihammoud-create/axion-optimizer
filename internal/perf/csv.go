// Package perf reads PresentMon captures and turns them into the numbers
// reviewers use (average FPS, 1% and 0.1% lows, frame-time percentiles,
// stutters), finds what limited each frame (CPU, GPU or a frame limiter) and
// compares runs taking run-to-run variation into account.
package perf

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// Frame is one frame presented by the measured swap chain. Unknown values
// are NaN.
type Frame struct {
	Time         float64 // ms since the start of the capture
	FrameTime    float64 // ms between this Present() and the previous one
	Displayed    float64 // ms between display changes; NaN = never displayed
	CPUBusy      float64 // ms the CPU worked on the frame before presenting
	CPUWait      float64 // ms the CPU waited before starting the next frame
	GPUBusy      float64 // ms at least one GPU engine worked on the frame
	UntilDisp    float64 // ms from Present() to display
	InputPhoton  float64 // ms from keyboard/mouse input to display (frames with input)
	ClickPhoton  float64 // ms from a mouse click to display
	PCLatency    float64 // ms from input to display reported by PC Latency (Reflex) events
	Mode         string  // PresentMode
	Tearing      bool    // AllowsTearing
	SyncInterval int
}

// Capture is a parsed PresentMon CSV (the swap chain with most frames).
type Capture struct {
	Process    string
	Runtime    string
	Swapchains int // swap chains seen for the process (overlays add their own)
	Frames     []Frame
}

// ErrNoFrames means the CSV has no frame of the process.
var ErrNoFrames = errors.New("nenhum quadro capturado")

// Column names of PresentMon 2.x (default output) with the 1.x and
// "--v2_metrics" equivalents as fallbacks (PresentMon README, CSV columns).
var (
	colApp       = []string{"Application"}
	colPID       = []string{"ProcessID"}
	colSwap      = []string{"SwapChainAddress"}
	colRuntime   = []string{"PresentRuntime", "Runtime"}
	colSync      = []string{"SyncInterval"}
	colTearing   = []string{"AllowsTearing"}
	colMode      = []string{"PresentMode"}
	colFrameTime = []string{"MsBetweenPresents", "FrameTime", "MsBetweenAppStart"}
	colDisplayed = []string{"MsBetweenDisplayChange", "DisplayedTime"}
	colCPUBusy   = []string{"MsCPUBusy", "CPUBusy"}
	colCPUWait   = []string{"MsCPUWait", "CPUWait"}
	colGPUBusy   = []string{"MsGPUBusy", "GPUBusy"}
	colUntilDisp = []string{"MsUntilDisplayed"}
	colInput     = []string{"MsAllInputToPhotonLatency", "AllInputToPhotonLatency"}
	colClick     = []string{"MsClickToPhotonLatency", "ClickToPhotonLatency"}
	colPCL       = []string{"MsPCLatency"}
	// Time columns: milliseconds or seconds depending on the options.
	colTimeMs  = []string{"CPUStartTimeInMs", "CPUStartTime", "TimeInMs"}
	colTimeSec = []string{"CPUStartTimeInSeconds", "TimeInSeconds"}
)

func num(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "NA") {
		return math.NaN()
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(v, 0) {
		return math.NaN()
	}
	return v
}

// ParseCSV reads a PresentMon CSV. Only the swap chain of process (by exe
// name, case-insensitive; empty = any) that presented the most frames is kept,
// so an overlay's own swap chain does not mix with the game's frames.
func ParseCSV(r io.Reader, process string) (*Capture, error) {
	cr := csv.NewReader(bufio.NewReaderSize(r, 1<<16))
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	cr.ReuseRecord = false
	head, err := cr.Read()
	if err == io.EOF {
		return nil, ErrNoFrames
	}
	if err != nil {
		return nil, fmt.Errorf("CSV do PresentMon: %w", err)
	}
	idx := map[string]int{}
	for i, h := range head {
		h = strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))
		if _, dup := idx[strings.ToLower(h)]; !dup {
			idx[strings.ToLower(h)] = i
		}
	}
	col := func(names []string) int {
		for _, n := range names {
			if i, ok := idx[strings.ToLower(n)]; ok {
				return i
			}
		}
		return -1
	}
	ft := col(colFrameTime)
	if ft < 0 {
		return nil, errors.New("formato de CSV do PresentMon não reconhecido (sem coluna de tempo de quadro)")
	}
	cApp, cPID, cSwap, cRT := col(colApp), col(colPID), col(colSwap), col(colRuntime)
	cSync, cTear, cMode := col(colSync), col(colTearing), col(colMode)
	cDisp, cCB, cCW, cGB := col(colDisplayed), col(colCPUBusy), col(colCPUWait), col(colGPUBusy)
	cUD, cIn, cCl, cPCL := col(colUntilDisp), col(colInput), col(colClick), col(colPCL)
	cT, tScale := col(colTimeMs), 1.0
	if cT < 0 {
		if cT = col(colTimeSec); cT >= 0 {
			tScale = 1000
		}
	}
	field := func(rec []string, i int) string {
		if i < 0 || i >= len(rec) {
			return ""
		}
		return rec[i]
	}
	fnum := func(rec []string, i int) float64 { return num(field(rec, i)) }

	type chain struct {
		app, runtime string
		frames       []Frame
	}
	chains := map[string]*chain{}
	var order []string
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				continue // a truncated last line when the capture was stopped
			}
			return nil, fmt.Errorf("CSV do PresentMon: %w", err)
		}
		app := strings.TrimSpace(field(rec, cApp))
		if process != "" && app != "" && !sameExe(app, process) {
			continue
		}
		key := app + "|" + field(rec, cPID) + "|" + field(rec, cSwap)
		c := chains[key]
		if c == nil {
			c = &chain{app: app, runtime: strings.TrimSpace(field(rec, cRT))}
			chains[key] = c
			order = append(order, key)
		}
		f := Frame{
			Time: fnum(rec, cT) * tScale, FrameTime: fnum(rec, ft), Displayed: fnum(rec, cDisp),
			CPUBusy: fnum(rec, cCB), CPUWait: fnum(rec, cCW), GPUBusy: fnum(rec, cGB),
			UntilDisp: fnum(rec, cUD), InputPhoton: fnum(rec, cIn), ClickPhoton: fnum(rec, cCl), PCLatency: fnum(rec, cPCL),
			Mode: strings.TrimSpace(field(rec, cMode)), Tearing: strings.TrimSpace(field(rec, cTear)) == "1",
		}
		if s := fnum(rec, cSync); !math.IsNaN(s) {
			f.SyncInterval = int(s)
		}
		// The first frame of a swap chain has no previous Present().
		if math.IsNaN(f.FrameTime) || f.FrameTime <= 0 {
			continue
		}
		c.frames = append(c.frames, f)
	}
	var best *chain
	for _, k := range order {
		if c := chains[k]; best == nil || len(c.frames) > len(best.frames) {
			best = c
		}
	}
	if best == nil || len(best.frames) == 0 {
		return nil, ErrNoFrames
	}
	return &Capture{Process: best.app, Runtime: best.runtime, Swapchains: len(chains), Frames: best.frames}, nil
}

// sameExe compares process names the way PresentMon does: case-insensitive,
// without directory or extension.
func sameExe(a, b string) bool {
	canon := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		if i := strings.LastIndexAny(s, `\/`); i >= 0 {
			s = s[i+1:]
		}
		return strings.TrimSuffix(s, ".exe")
	}
	return canon(a) == canon(b)
}

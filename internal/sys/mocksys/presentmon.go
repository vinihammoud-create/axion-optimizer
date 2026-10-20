package mocksys

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"axion/internal/sys"
)

// simulatePresentMon writes the CSV PresentMon 2.6.0 would write for a game
// at ~420 FPS on a 240 Hz monitor: mostly GPU-bound, a few hitches, an
// overlay swap chain, Reflex latency events. Each run is a little faster than
// the previous one, so "antes × depois" has something to show.
func (m *Mock) simulatePresentMon(args []string) sys.Result {
	get := func(flag string) string {
		for i := 0; i+1 < len(args); i++ {
			if strings.EqualFold(args[i], flag) {
				return args[i+1]
			}
		}
		return ""
	}
	has := func(flag string) bool {
		for _, a := range args {
			if strings.EqualFold(a, flag) {
				return true
			}
		}
		return false
	}
	if has("--terminate_existing_session") {
		return sys.Result{}
	}
	m.mu.Lock()
	fail, noGame, baseFn, seedFn, hook, needs := m.PMFail, m.PMNoGame, m.PMBase, m.PMSeed, m.PMHook, m.PMNeedsProc
	m.PMRuns++
	run := m.PMRuns
	m.mu.Unlock()
	if hook != nil {
		hook(run, args) // outside the lock: the test may touch the Mock
	}
	if fail != "" {
		return sys.Result{ExitCode: 6, Stderr: fail + "\r\n"}
	}
	out := sys.Result{Stdout: "Started recording.\r\nStopped recording.\r\n"}
	proc := get("--process_name")
	if noGame || (needs && has("--terminate_on_proc_exit") && len(m.N.PIDsByName(proc)) == 0) {
		return out
	}
	path := get("--output_file")
	if has("--hotkey") {
		path = strings.TrimSuffix(path, ".csv") + "-1.csv"
	}
	secs, _ := strconv.Atoi(get("--timed"))
	if secs <= 0 {
		secs = 60
	}
	f, err := os.Create(path)
	if err != nil {
		return sys.Result{ExitCode: 1, Stderr: err.Error()}
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	w.WriteString("\ufeffApplication,ProcessID,SwapChainAddress,PresentRuntime,SyncInterval,PresentFlags,AllowsTearing,PresentMode,TimeInMs,MsBetweenSimulationStart,MsBetweenPresents,MsBetweenDisplayChange,MsInPresentAPI,MsRenderPresentLatency,MsUntilDisplayed,MsPCLatency,CPUStartTimeInMs,MsBetweenAppStart,MsCPUBusy,MsCPUWait,MsGPULatency,MsGPUTime,MsGPUBusy,MsGPUWait,MsAnimationError,AnimationTime,MsFlipDelay,MsAllInputToPhotonLatency,MsClickToPhotonLatency\n")
	seedRun := run
	if seedFn != nil {
		seedRun = seedFn(run)
	}
	seed := uint64(12345 + seedRun*7919)
	rnd := func() float64 { // xorshift64*
		seed ^= seed >> 12
		seed ^= seed << 25
		seed ^= seed >> 27
		return float64((seed*2685821657736338717)>>11) / float64(1<<53)
	}
	base, scale := 2.42-0.012*float64(run%10), 1.0
	if baseFn != nil {
		base = baseFn(run)
		scale = base / 2.42 // hitches scale with the frame time too
	}
	t := 0.0
	for i := 0; t < float64(secs)*1000; i++ {
		ft := base * (0.96 + 0.08*rnd())
		if rnd() < 0.003 {
			ft = (8 + 6*rnd()) * scale // hitch: shader compile, asset streaming
		}
		gpu := ft * (0.9 + 0.08*rnd())
		cpuBusy := ft * (0.5 + 0.1*rnd())
		input := "NA"
		if rnd() < 0.2 {
			input = fmt.Sprintf("%.4f", 6+3*rnd())
		}
		if i == 0 {
			ft = 0 // first frame has no previous present
		}
		fmt.Fprintf(w, "%s,4242,0x1F2A3B40,DXGI,0,512,1,Hardware: Independent Flip,%.4f,NA,%.4f,%.4f,0.0800,%.4f,%.4f,%.4f,%.4f,%.4f,%.4f,%.4f,0.2000,%.4f,%.4f,%.4f,0.0100,NA,NA,%s,NA\n",
			proc, t, ft, ft, gpu, 1.1+0.5*rnd(), 7+3*rnd(), t, ft, cpuBusy, ft-cpuBusy, gpu+0.05, gpu, ft-gpu, input)
		t += ft
		if i%1000 == 500 { // Steam overlay's own swap chain
			fmt.Fprintf(w, "%s,4242,0x7FF0A000,DXGI,1,0,0,Composed: Flip,%.4f,NA,16.6000,16.6000,0.1,1.0,9.0,NA,%.4f,16.6,1.0,15.6,0.1,1.0,1.0,15.6,0.0,NA,NA,NA,NA\n", proc, t, t)
		}
	}
	return out
}

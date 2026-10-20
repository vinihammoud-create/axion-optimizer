package app

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"axion/internal/collect"
	"axion/internal/perf"
	"axion/internal/sys"
)

// What a capture records besides the frames (0.9): the state of the PC when it
// started (battery, power plan, programs eating the processor) and, once a
// second while the game is drawn, the temperature and throttling of the
// NVIDIA GPU and the frequency of the processors. A "before" and an "after"
// taken in different conditions say nothing about the setting that changed.

// Windows' "Power saver" plan.
const powerSaverGUID = "a1841308-3541-4fab-bc81-f71556f20b4a"

const (
	gpuThrottleOld = "clocks_throttle_reasons"
	gpuThrottleNew = "clocks_event_reasons"
	gpuFieldCount  = 10 // the columns of the query below
)

// gpuReasons are the throttle reasons read, in the order parseGPULine expects.
var gpuReasons = []string{"hw_thermal_slowdown", "sw_thermal_slowdown", "hw_slowdown", "hw_power_brake_slowdown", "sw_power_cap"}

// gpuFields builds the nvidia-smi query. The reasons are called
// clocks_throttle_reasons in older drivers and clocks_event_reasons in newer
// ones (both are accepted today, but only one may be tomorrow).
func gpuFields(prefix string) string {
	var f []string
	for _, r := range gpuReasons {
		f = append(f, prefix+"."+r)
	}
	f = append(f, "temperature.gpu", "clocks.sm", "clocks.max.sm", "power.draw", "utilization.gpu")
	return strings.Join(f, ",")
}

// procTimes sums the processor time (100 ns) of every program by name.
func (a *App) procTimes() map[string]int64 {
	list, err := a.P.Native.Processes()
	if err != nil {
		return nil
	}
	m := map[string]int64{}
	for _, p := range list {
		m[p.Name] += p.CPUTime
	}
	return m
}

// busySkipped are the programs never listed as busy: the ones that are not
// another program's doing or that nobody could close.
func busySkipped(game string) func(string) bool {
	self := strings.ToLower(filepath.Base(os.Args[0]))
	return func(name string) bool {
		n := strings.ToLower(name)
		switch n {
		case "system", "system idle process", "idle", "registry", "memory compression", "dwm.exe", "csrss.exe", "":
			return true
		}
		return n == strings.ToLower(game) || n == self || strings.HasPrefix(n, "presentmon")
	}
}

// busyPrograms lists the programs that used at least perf.BusyMinPct of a core
// between two readings of procTimes. A window shorter than two seconds says
// nothing.
func busyPrograms(before, after map[string]int64, window time.Duration, skip func(string) bool) []perf.Busy {
	if window < 2*time.Second || before == nil || after == nil {
		return nil
	}
	span := float64(window / 100) // 100-ns units
	var out []perf.Busy
	for name, t1 := range after {
		d := t1 - before[name]
		if d <= 0 || skip(name) {
			continue
		}
		if pct := float64(d) / span * 100; pct >= perf.BusyMinPct {
			out = append(out, perf.Busy{Name: name, Pct: pct})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pct != out[j].Pct {
			return out[i].Pct > out[j].Pct
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

// conditionsNow reads the state of the PC right now.
func (a *App) conditionsNow() *perf.Conditions {
	c := &perf.Conditions{At: time.Now(), BatteryPct: -1}
	if has, onAC, pct, err := a.P.Native.PowerStatus(); err == nil {
		c.HasBattery = has
		c.OnBattery = has && !onAC
		if has {
			c.BatteryPct = pct
		}
	}
	norm := func(s string) string { return strings.ToLower(strings.Trim(strings.TrimSpace(s), "{}")) }
	if g, err := a.P.Power.ActiveScheme(); err == nil {
		c.PowerSaver = norm(g) == powerSaverGUID
		if list, err := a.P.Power.Schemes(); err == nil {
			for _, s := range list {
				if norm(s.GUID) == norm(g) {
					c.PowerScheme = s.Name
				}
			}
		}
	}
	if ov, err := a.P.Power.ActiveOverlay(); err == nil {
		switch norm(ov) {
		case sys.OverlayBestPerformance:
			c.PowerMode = "Melhor desempenho"
		case sys.OverlayBestEfficiency:
			c.PowerMode, c.Efficiency = "Melhor eficiência energética", true
		default:
			c.PowerMode = "Equilibrado"
		}
	}
	if modes, err := a.P.Native.DisplayModes(); err == nil {
		for _, m := range modes {
			if m.Primary {
				c.DisplayHz = m.CurrentHz
			}
		}
	}
	return c
}

type timedGPU struct {
	at time.Time
	s  perf.GPUSample
}

type timedCPU struct {
	at time.Time
	s  perf.CPUSample
}

// sensorRun follows one capture.
type sensorRun struct {
	a      *App
	game   string
	cancel context.CancelFunc
	wg     sync.WaitGroup
	cond   *perf.Conditions
	start  time.Time
	every  time.Duration

	mu     sync.Mutex
	rec    time.Time // when the recording began (zero until it did)
	procs0 map[string]int64
	at0    time.Time
	procsR map[string]int64
	atR    time.Time
	gpu    []timedGPU
	cpu    []timedCPU
}

// startSensors takes the conditions and starts reading the sensors. Call
// Finish when the recording is over (or cancel when it is abandoned).
func (a *App) startSensors(ctx context.Context, game string) *sensorRun {
	sctx, cancel := context.WithCancel(ctx)
	every := a.sensorEvery
	if every <= 0 {
		every = time.Second
	}
	s := &sensorRun{a: a, game: game, cancel: cancel, start: time.Now(), every: every, cond: a.conditionsNow()}
	s.procs0, s.at0 = a.procTimes(), time.Now()
	s.wg.Add(2)
	go func() { defer s.wg.Done(); s.gpuLoop(sctx) }()
	go func() { defer s.wg.Done(); s.cpuLoop(sctx) }()
	return s
}

// Recording says the frames started to be recorded: only what is read from
// then on speaks for the measurement.
func (s *sensorRun) Recording(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.rec.IsZero() {
		return
	}
	s.rec = t
	s.procsR, s.atR = s.a.procTimes(), time.Now()
}

// Finish stops the readings and returns the conditions and the sensors of the
// recording (the sensors are nil when nothing could be read).
func (s *sensorRun) Finish() (*perf.Conditions, *perf.Sensors) {
	end := time.Now()
	procsEnd := s.a.procTimes()
	s.cancel()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	from, before, t0 := s.rec, s.procsR, s.atR
	if from.IsZero() {
		from, before, t0 = s.start, s.procs0, s.at0
	}
	cond := *s.cond
	cond.Busy = busyPrograms(before, procsEnd, end.Sub(t0), busySkipped(s.game))
	var gs []perf.GPUSample
	for _, x := range s.gpu {
		if !x.at.Before(from) {
			gs = append(gs, x.s)
		}
	}
	var cs []perf.CPUSample
	for _, x := range s.cpu {
		if !x.at.Before(from) {
			cs = append(cs, x.s)
		}
	}
	sens := &perf.Sensors{GPU: perf.SummarizeGPU(gs), CPU: perf.SummarizeCPU(cs)}
	if sens.GPU == nil && sens.CPU == nil {
		sens = nil
	}
	return &cond, sens
}

// parseNum reads a number that nvidia-smi may print as [N/A] or Not Supported.
func parseNum(s string) float64 {
	s = strings.TrimSpace(s)
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return -1
}

// parseGPULine reads one line of the query built by gpuFields.
func parseGPULine(line string) (perf.GPUSample, bool) {
	p := strings.Split(strings.TrimSpace(line), ",")
	if len(p) < gpuFieldCount {
		return perf.GPUSample{}, false
	}
	on := func(i int) bool { return strings.EqualFold(strings.TrimSpace(p[i]), "Active") }
	s := perf.GPUSample{
		Thermal:    on(0) || on(1),
		HWSlowdown: on(2) || on(3),
		PowerCap:   on(4),
		PowerW:     parseNum(p[8]),
		UtilPct:    int(parseNum(p[9])),
	}
	if t := parseNum(p[5]); t >= 0 {
		s.TempC = int(t)
	}
	if c := parseNum(p[6]); c > 0 {
		s.ClockMHz = int(c)
	}
	if c := parseNum(p[7]); c > 0 {
		s.MaxClockMHz = int(c)
	}
	return s, true
}

// nvidiaSMI is the path of nvidia-smi when the PC has an NVIDIA GPU.
func (a *App) nvidiaSMI() string {
	f := a.Facts()
	if f == nil || !f.HasGPUVendor("NVIDIA") {
		return ""
	}
	if a.P.IsMock {
		return "nvidia-smi.exe"
	}
	return collect.NvidiaSMIPath(a.P.Native.Paths())
}

// gpuLoop reads the GPU once a second with a single nvidia-smi process that
// loops (one process, not one per second: spawning a program every second in
// the middle of a game would disturb what is being measured).
func (s *sensorRun) gpuLoop(ctx context.Context) {
	exe := s.a.nvidiaSMI()
	if exe == "" {
		return
	}
	fields := ""
	for _, prefix := range []string{gpuThrottleOld, gpuThrottleNew} {
		pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		res, err := s.a.P.Run.Run(pctx, exe, []string{"--query-gpu=" + gpuFields(prefix), "--format=csv,noheader,nounits"}, sys.RunOpts{Timeout: 10 * time.Second})
		cancel()
		if err != nil || res.ExitCode != 0 {
			continue
		}
		if first := strings.Split(strings.ReplaceAll(res.Stdout, "\r", ""), "\n")[0]; first != "" {
			if _, ok := parseGPULine(first); ok {
				fields = gpuFields(prefix)
				break
			}
		}
	}
	if fields == "" || ctx.Err() != nil {
		return
	}
	secs := strconv.Itoa(int((s.every + time.Second/2) / time.Second))
	if secs == "0" {
		secs = "1"
	}
	_, _ = s.a.P.Run.Run(ctx, exe, []string{"--query-gpu=" + fields, "--format=csv,noheader,nounits", "-l", secs}, sys.RunOpts{Timeout: 6 * time.Hour, OnLine: func(line string) {
		if g, ok := parseGPULine(line); ok {
			s.mu.Lock()
			s.gpu = append(s.gpu, timedGPU{time.Now(), g})
			s.mu.Unlock()
		}
	}})
}

// cpuLoop reads the processors' frequency (a performance counter) and use
// (the system's processor times) once a second.
func (s *sensorRun) cpuLoop(ctx context.Context) {
	n := s.a.P.Native
	_, _ = n.CPUFreqPct() // the first reading only starts the counter
	idle, kernel, user, err := n.CPUTimes()
	if err != nil {
		return
	}
	prevIdle, prevTotal := idle, kernel+user
	tick := time.NewTicker(s.every)
	defer tick.Stop()
	failed := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		freq, err := n.CPUFreqPct()
		if err != nil {
			if failed++; failed >= 3 {
				return // the counter is not there on this PC
			}
			continue
		}
		failed = 0
		idle, kernel, user, err := n.CPUTimes()
		if err != nil {
			continue
		}
		total := kernel + user
		dTotal, dIdle := float64(total-prevTotal), float64(idle-prevIdle)
		prevIdle, prevTotal = idle, total
		if dTotal <= 0 {
			continue
		}
		util := (dTotal - dIdle) / dTotal * 100
		if util < 0 {
			util = 0
		} else if util > 100 {
			util = 100
		}
		s.mu.Lock()
		s.cpu = append(s.cpu, timedCPU{time.Now(), perf.CPUSample{FreqPct: freq, UtilPct: util, LimitPct: thermalLimitPct(n)}})
		s.mu.Unlock()
	}
}

// thermalLimitPct is the lowest thermal frequency limit Windows puts on the
// logical processors, as a share of their maximum specified frequency
// (CallNtPowerInformation; -1 when it cannot be read). It is a system call,
// cheap enough for once a second during a game.
func thermalLimitPct(n sys.Native) float64 {
	procs, err := n.ProcessorPower()
	if err != nil || len(procs) == 0 {
		return -1
	}
	low := -1.0
	for _, p := range procs {
		if p.MaxMhz <= 0 || p.MhzLimit <= 0 {
			continue
		}
		pct := float64(p.MhzLimit) / float64(p.MaxMhz) * 100
		if pct > 100 {
			pct = 100
		}
		if low < 0 || pct < low {
			low = pct
		}
	}
	return low
}

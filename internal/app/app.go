// Package app wires the platform, the scan, the diagnostic, the tweak engine
// and the games library behind a small JSON API used by the interface.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"axion/internal/catalog"
	"axion/internal/collect"
	"axion/internal/diag"
	"axion/internal/dpc"
	"axion/internal/facts"
	"axion/internal/games"
	"axion/internal/netdiag"
	"axion/internal/obs"
	"axion/internal/perf"
	"axion/internal/periph"
	"axion/internal/profile"
	"axion/internal/programs"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// Settings persisted in DataDir/settings.json.
type Settings struct {
	RestorePoint bool     `json:"restorePoint"` // create a restore point before batches
	ManualGames  []string `json:"manualGames"`  // executables added by the user
	Accepted     bool     `json:"accepted"`     // first-run notice accepted
	Dismissed    []string `json:"dismissed"`    // finding IDs the user no longer wants to be warned about
	// SkipParts are parts of the analysis that ended the collection on this
	// PC with SkipVersion; a new version tries them again.
	SkipParts   []string `json:"skipParts,omitempty"`
	SkipVersion string   `json:"skipVersion,omitempty"`
	// Profile is what the PC is used for (profile package); empty = never
	// chosen, read as the default. Simple shows the one-screen mode.
	Profile string `json:"profile,omitempty"`
	Simple  bool   `json:"simple,omitempty"`
	// Apps is the saved selection of the catalog of programs (0.8): what
	// to install after a format.
	Apps []string `json:"apps,omitempty"`
	// KeptTrials are the "Prova no seu PC" runs the user chose to leave
	// applied after they were interrupted without a verdict.
	KeptTrials []string `json:"keptTrials,omitempty"`
	// AutoReport are the executables (lower case) whose sessions are recorded
	// automatically while Axion is open (the match report), in the order
	// they were turned on. Off for every game by default.
	AutoReport []string `json:"autoReport,omitempty"`
}

// profile returns the use profile chosen (the default before the choice).
func (a *App) profile() profile.Info {
	a.mu.RLock()
	id := a.settings.Profile
	a.mu.RUnlock()
	return profile.Get(id)
}

// SetProfile stores the use profile and reads the diagnostic again for it
// (the machine did not change; what counts did).
func (a *App) SetProfile(id string) error {
	if !profile.Valid(id) {
		return fmt.Errorf("%w: perfil desconhecido", errBadRequest)
	}
	if err := a.UpdateSettings(func(s *Settings) { s.Profile = id }); err != nil {
		return err
	}
	a.rebuildReport()
	return nil
}

// App is the application core.
type App struct {
	P       *sys.Platform
	Engine  *tweaks.Engine
	Jobs    *Jobs
	Scanner *games.Scanner
	Version string

	mu        sync.RWMutex
	facts     *facts.Facts
	states    map[string]tweaks.State
	report    *diag.Report
	stability []diag.Finding
	events    []sys.Event    // the stability events of the last analysis (errors, symptoms and boots)
	meas      *diag.Measured // what the diagnostic uses from the measurements (nil = read again)
	lastScan  time.Time
	settings  Settings
	lastEval  *EvalResult // the last full evaluation of this session
	// trials caches the outcomes of "Prova no seu PC" read from the
	// measurements; trialsOK false = read again (cleared with meas).
	trials   []perf.TrialOutcome
	trialsOK bool

	gmu   sync.RWMutex
	games []games.Game
	cs2   games.CS2Info

	pmu      sync.Mutex
	measures []periph.Measurement // polling tests, newest first (nil = not loaded)

	drv        driverStore
	batches    batchStore
	appsSt     appsState // what winget reported (0.8)
	netWait    netWait
	clean      cleanupCache
	rep        repairCache
	progs      programState
	startVisit startupVisit // startup entries seen at the last visit (0.9.x)
	// NvidiaAPI overrides NVIDIA's endpoints and HTTP client (tests).
	NvidiaAPI NvidiaEndpoints
	// netTimingOverride shortens the network test (tests).
	netTimingOverride *netTiming
	// sensorEvery is the pause between readings of the sensors during a
	// capture (0 = one second; tests shorten it).
	sensorEvery time.Duration
	// sess follows the games of the automatic match report (session.go).
	sess sessionWatch
	// sessTimingOverride shortens the match report's waits (tests).
	sessTimingOverride *sessionTiming
	// pmOwner is who runs PresentMon now: "" free, "perf" a manual
	// measurement (or the full evaluation), "sessao" a window of the match
	// report. pmWant counts manual measurements waiting for the report to let go.
	pmMu    sync.Mutex
	pmOwner string
	pmWant  atomic.Int32
	// Collector runs the collection (nil: in this process).
	Collector Collector
}

// ErrNoScan is returned when data that needs a scan is requested before one.
var ErrNoScan = errors.New("faça a análise do PC primeiro")

// New builds the application over a platform.
func New(p *sys.Platform, version string) (*App, error) {
	j, err := tweaks.OpenJournal(p.DataDir, p.Key)
	if err != nil {
		return nil, fmt.Errorf("histórico: %w", err)
	}
	cat, err := tweaks.NewCatalog(catalog.All())
	if err != nil {
		return nil, err
	}
	e := tweaks.NewEngine(p, cat, j)
	e.Providers = append(e.Providers, catalog.ProfileProvider, catalog.ActiveHoursProvider, catalog.StartupProvider, catalog.DNSProvider, catalog.CS2VideoProvider)
	if n := j.RecoverInterrupted(); n > 0 {
		log.Printf("%d aplicação(ões) interrompida(s) encontrada(s) no histórico", n)
	}
	if n := e.ReconcileHistory(); n > 0 {
		log.Printf("histórico: %d aplicação(ões) com pendência substituída(s) por uma aplicação posterior do mesmo item", n)
	}
	a := &App{P: p, Engine: e, Jobs: NewJobs(), Scanner: games.NewScanner(p.Reg), Version: version,
		settings: Settings{RestorePoint: true}}
	a.loadSettings()
	catalog.GamePath = a.gamePath
	catalog.CS2VideoPath = a.cs2VideoPath
	catalog.ProcessRunning = func(exe string) bool { return len(p.Native.PIDsByName(exe)) > 0 }
	if n := programs.Purge(a.quarantineRoot(), time.Now()); n > 0 {
		log.Printf("quarentena: %d lote(s) com mais de 30 dias apagado(s)", n)
	}
	go func() { a.cleanupPresentMon(); a.ensureWatcher() }() // the watcher never races the cleanup
	go a.cleanupUninstallTasks()
	return a, nil
}

// uninstallTaskPrefix names the one-shot tasks that run a per-user
// uninstaller with the user's own rights (see StartUninstall).
const uninstallTaskPrefix = "Desinstalar "

// cleanupUninstallTasks removes one-shot uninstall tasks left by an Axion
// closed in the middle of an uninstall (a finished run deletes its own). It
// runs at startup, before the interface can start an uninstall, and skips
// the cleanup if one is already running.
func (a *App) cleanupUninstallTasks() {
	log.Printf("início: procurando tarefas de desinstalação antigas")
	defer log.Printf("início: tarefas de desinstalação conferidas")
	list, err := a.P.Tasks.List(`\Axion`, false)
	if err != nil {
		return
	}
	for _, t := range list {
		if !strings.HasPrefix(t.Name, uninstallTaskPrefix) {
			continue
		}
		if j, running := a.Jobs.Running(); running && j.Kind == "uninstall" {
			return
		}
		if err := a.P.Tasks.Delete(t.Path); err != nil {
			log.Printf("tarefa de desinstalação antiga %s: %v", t.Path, err)
		} else {
			log.Printf("tarefa de desinstalação antiga removida: %s", t.Path)
		}
	}
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

func (a *App) settingsPath() string { return filepath.Join(a.P.DataDir, "settings.json") }

func (a *App) loadSettings() {
	b, err := os.ReadFile(a.settingsPath())
	if err != nil {
		return
	}
	var s Settings
	if json.Unmarshal(b, &s) == nil {
		a.settings = s
	}
}

// Settings returns a copy of the settings.
func (a *App) Settings() Settings {
	a.mu.RLock()
	defer a.mu.RUnlock()
	s := a.settings
	s.ManualGames = append([]string(nil), a.settings.ManualGames...)
	s.Dismissed = append([]string(nil), a.settings.Dismissed...)
	s.SkipParts = append([]string(nil), a.settings.SkipParts...)
	s.Apps = append([]string(nil), a.settings.Apps...)
	s.KeptTrials = append([]string(nil), a.settings.KeptTrials...)
	s.AutoReport = append([]string(nil), a.settings.AutoReport...)
	return s
}

// dismissedSet returns the finding IDs the user dismissed.
func (a *App) dismissedSet() map[string]bool {
	out := map[string]bool{}
	for _, id := range a.Settings().Dismissed {
		out[id] = true
	}
	return out
}

// UpdateSettings changes and persists the settings.
func (a *App) UpdateSettings(fn func(s *Settings)) error {
	a.mu.Lock()
	fn(&a.settings)
	a.mu.Unlock()
	return saveFile(a.settingsPath(), func() ([]byte, error) {
		a.mu.RLock()
		s := a.settings
		a.mu.RUnlock()
		return json.MarshalIndent(s, "", "  ")
	})
}

// ---------------------------------------------------------------------------
// Snapshots for the API
// ---------------------------------------------------------------------------

// Facts returns the latest machine profile (nil before the first scan).
func (a *App) Facts() *facts.Facts {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.facts
}

// Report returns the latest diagnostic.
func (a *App) Report() *diag.Report {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.report
}

// States returns the cached tweak states.
func (a *App) States() map[string]tweaks.State {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(map[string]tweaks.State, len(a.states))
	for k, v := range a.states {
		out[k] = v
	}
	return out
}

// LastScan returns when the last scan finished.
func (a *App) LastScan() time.Time {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastScan
}

// Games returns the detected games.
func (a *App) Games() ([]games.Game, games.CS2Info) {
	a.gmu.RLock()
	defer a.gmu.RUnlock()
	return append([]games.Game(nil), a.games...), a.cs2
}

func (a *App) gamePath(exe string) string {
	a.gmu.RLock()
	defer a.gmu.RUnlock()
	for _, g := range a.games {
		if g.Exe != "" && strings.EqualFold(g.ExeName, exe) {
			return g.Exe
		}
	}
	return ""
}

// gameName names a game by its executable (the executable itself when the
// library does not know it).
func (a *App) gameName(exe string) string {
	if strings.EqualFold(exe, "cs2.exe") {
		return "Counter-Strike 2"
	}
	a.gmu.RLock()
	defer a.gmu.RUnlock()
	for _, g := range a.games {
		if g.Name != "" && strings.EqualFold(g.ExeName, exe) {
			return g.Name
		}
	}
	return exe
}

// ---------------------------------------------------------------------------
// Scan and refresh
// ---------------------------------------------------------------------------

// stabilityXPath selects hardware errors, unexpected shutdowns, blue screens
// and display driver resets from the last 30 days.
const stabilityXPath = "*[System[Provider[@Name='Microsoft-Windows-WHEA-Logger'] and TimeCreated[timediff(@SystemTime) <= 2592000000]]]"

// symptomXPath selects the symptoms of the last 30 days (unexpected
// shutdowns, blue screens, display driver resets). A query of its own, so a
// link logging corrected errors every few minutes never pushes a blue
// screen out of the cap of the first.
const symptomXPath = "*[System[((Provider[@Name='Microsoft-Windows-Kernel-Power'] and EventID=41) or (Provider[@Name='Microsoft-Windows-WER-SystemErrorReporting'] and EventID=1001) or (Provider[@Name='Display'] and EventID=4101)) and TimeCreated[timediff(@SystemTime) <= 2592000000]]]"

// bootXPath selects when the PC started or woke up in the last 30 days (OS
// start, boot type, resume from sleep and from Modern Standby), to tell
// hardware errors at startup from errors during use. It is a query of its
// own so frequent resumes never push errors out of the cap of the first.
const bootXPath = "*[System[((Provider[@Name='Microsoft-Windows-Kernel-General'] and EventID=12) or (Provider[@Name='Microsoft-Windows-Kernel-Boot'] and EventID=27) or (Provider[@Name='Microsoft-Windows-Kernel-Power'] and (EventID=107 or EventID=507))) and TimeCreated[timediff(@SystemTime) <= 2592000000]]]"

// thermalXPath selects the thermal events of the last 30 days in the System
// log: critical thermal shutdowns (Kernel-Power 86) and the firmware
// limiting the processors (Kernel-Processor-Power 37). The passive cooling
// events (114) live in a channel of their own (thermalOpXPath).
const thermalXPath = "*[System[((Provider[@Name='Microsoft-Windows-Kernel-Power'] and EventID=86) or (Provider[@Name='Microsoft-Windows-Kernel-Processor-Power'] and EventID=37)) and TimeCreated[timediff(@SystemTime) <= 2592000000]]]"

const thermalOpXPath = "*[System[EventID=114 and TimeCreated[timediff(@SystemTime) <= 2592000000]]]"

// Caps of the queries (newest first).
const (
	stabilityMax = 1000
	symptomMax   = 500
	bootMax      = 2000
	thermalMax   = 500
)

// ScanSummary is the result of a scan job.
type ScanSummary struct {
	Seconds  float64 `json:"seconds"`
	Overall  int     `json:"overall"`
	Grade    string  `json:"grade"`
	Findings int     `json:"findings"`
	Games    int     `json:"games"`
	Errors   int     `json:"errors"`
}

// StartScan starts a full scan job.
func (a *App) StartScan() (Job, error) {
	return a.Jobs.Start("scan", "Análise do PC", true, a.scan)
}

func (a *App) scan(ctx context.Context, c *JobCtl) (any, error) {
	start := time.Now()
	f, err := a.collectFacts(ctx, c, func(step string, done, total int) {
		log.Printf("análise: %s", step)
		c.Progress(0.75*float64(done)/float64(total), step)
	})
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	for _, e := range f.Errors {
		c.Logf("aviso: %s", e)
	}
	c.Progress(0.78, "Jogos instalados")
	log.Printf("análise: jogos instalados")
	a.rescanGames()
	c.Progress(0.84, "Histórico de estabilidade (30 dias)")
	log.Printf("análise: histórico de estabilidade")
	var evs []sys.Event
	if e, err := a.P.Events.Query("System", stabilityXPath, stabilityMax); err == nil {
		evs = e
		if sym, err := a.P.Events.Query("System", symptomXPath, symptomMax); err == nil {
			evs = append(evs, sym...)
		} else {
			c.Logf("aviso: sintomas no log de eventos: %v", err)
		}
		if boots, err := a.P.Events.Query("System", bootXPath, bootMax); err == nil {
			evs = append(evs, boots...)
		} else {
			c.Logf("aviso: horários de inicialização no log de eventos: %v", err)
		}
		if th, err := a.P.Events.Query("System", thermalXPath, thermalMax); err == nil {
			evs = append(evs, th...)
		} else {
			c.Logf("aviso: eventos térmicos no log de eventos: %v", err)
		}
		// The channel exists from Windows 8 on; without it there is just no
		// passive cooling history to read.
		if th, err := a.P.Events.Query(diag.ChanThermal, thermalOpXPath, thermalMax); err == nil {
			evs = append(evs, th...)
		}
	} else {
		c.Logf("aviso: log de eventos: %v", err)
	}
	c.Progress(0.9, "Avaliando otimizações")
	log.Printf("análise: avaliando otimizações")
	st := a.computeStates(ctx, f)
	stab := diag.StabilityWith(evs, f, st)
	rep := a.buildReport(f, st, stab)
	a.mu.Lock()
	a.facts, a.states, a.report, a.stability, a.events, a.lastScan = f, st, rep, stab, evs, time.Now()
	a.mu.Unlock()
	gl, _ := a.Games()
	c.Logf("análise concluída: nota %d (%s), %d achados, %d jogos", rep.Overall, rep.Grade, len(rep.Findings), len(gl))
	return ScanSummary{Seconds: time.Since(start).Seconds(), Overall: rep.Overall, Grade: rep.Grade, Findings: len(rep.Findings), Games: len(gl), Errors: len(f.Errors)}, nil
}

// patchFacts updates part of the machine profile after a quick change made
// from Axion (startup items, closed programs, a retrim) and rebuilds the
// diagnostic from it, so the score and the findings do not wait for a new
// analysis. fn gets a copy of the profile and must replace, not modify in
// place, any slice or map it changes. A scan that finished meanwhile wins.
func (a *App) patchFacts(fn func(f *facts.Facts)) {
	a.mu.RLock()
	cur, st := a.facts, a.states
	stab := append([]diag.Finding(nil), a.stability...)
	a.mu.RUnlock()
	if cur == nil {
		return
	}
	nf := *cur
	fn(&nf)
	rep := a.buildReport(&nf, st, stab)
	a.mu.Lock()
	if a.facts == cur {
		a.facts, a.report = &nf, rep
	}
	a.mu.Unlock()
}

// rebuildReport runs the diagnostic again on the profile already known,
// after a measurement that it uses (network test, FPS capture) was saved.
func (a *App) rebuildReport() { a.patchFacts(func(*facts.Facts) {}) }

// buildReport runs the diagnostic on a profile with the tweak states, the
// stability findings and what the Axion measured on this PC (the last
// complete network test, the last FPS capture of the competitive game).
func (a *App) buildReport(f *facts.Facts, st map[string]tweaks.State, stab []diag.Finding) *diag.Report {
	return diag.Finalize(diag.BuildWith(f, st, a.measured(), stab...), a.profile().ID, a.dismissedSet())
}

// restab recomputes the stability findings from the events of the last
// analysis with the current tweak states (the PCIe power-saving test is
// not offered once it is applied).
func (a *App) restab(f *facts.Facts, st map[string]tweaks.State) []diag.Finding {
	a.mu.RLock()
	evs := a.events
	old := append([]diag.Finding(nil), a.stability...)
	a.mu.RUnlock()
	if evs == nil {
		return old
	}
	return diag.StabilityWith(evs, f, st)
}

// measured gathers the measurements the diagnostic uses: the newest network
// test with a grade (the queue under load) and the newest test of all (the
// route to Valve), plus the newest FPS capture of the competitive game long
// enough to mean something. The result is cached until a test or capture is
// saved, renamed or deleted.
func (a *App) measured() *diag.Measured {
	a.mu.RLock()
	m := a.meas
	a.mu.RUnlock()
	if m != nil {
		return m
	}
	m = &diag.Measured{}
	if a.P == nil {
		return m
	}
	runs := netdiag.List(a.netDir())
	if len(runs) > 0 {
		m.NetLatest = &runs[0]
	}
	for i := range runs {
		if runs[i].Grade != "" {
			m.Net = &runs[i]
			break
		}
	}
	if m.Net == nil && len(runs) > 0 {
		m.Net = &runs[0]
	}
	// A timed capture sums a little under the seconds asked for (59,98 s
	// for 60): the bound stays under the shortest option the UI offers (30 s).
	for _, r := range perf.List(a.perfDir()) {
		// A capture "depois" of a trial whose item was undone does not
		// describe the PC now.
		if r.Trial != nil && r.Trial.Phase == perf.TrialPhaseAfter && (r.Trial.Outcome == nil || r.Trial.Outcome.Action != perf.TrialKept) {
			continue
		}
		// The PCIe link read with any game drawing (0.7): the newest one.
		if m.Link == nil && r.GPULink != nil {
			l := *r.GPULink
			m.Link, m.LinkGame = &l, a.gameName(r.Process)
		}
		if m.Perf != nil || !strings.EqualFold(r.Process, "cs2.exe") || r.Summary.Frames < 1000 || r.Summary.Seconds < 25 {
			continue
		}
		sum := r.Summary
		m.Perf, m.PerfAt, m.PerfGame = &sum, r.Started, "Counter-Strike 2"
	}
	// The newest stream (or recording) the OBS logs tell about.
	if ss, _ := obs.Read(a.obsDir(), time.Now()); len(ss) > 0 {
		if s := obs.Pick(ss); s != nil {
			up := 0.0
			if m.Net != nil && m.Net.UpMbps > 0 {
				up = m.Net.UpMbps
			} else if m.NetLatest != nil && m.NetLatest.UpMbps > 0 {
				up = m.NetLatest.UpMbps
			}
			sess := *s
			an := obs.Analyze(&sess, up)
			m.OBS, m.OBSAnalysis = &sess, &an
		}
	}
	// The newest capture of the kernel's DPCs and interrupts that can be trusted.
	for _, ln := range dpc.List(a.dpcDir()) {
		if !ln.Reliable {
			continue
		}
		if r, err := dpc.Load(a.dpcDir(), ln.ID); err == nil {
			sum := r.Summary
			m.DPC, m.DPCAt, m.DPCLabel = &sum, r.At, r.Label
			break
		}
	}
	a.mu.Lock()
	a.meas = m
	a.mu.Unlock()
	return m
}

// measurementsChanged forgets the cached measurements (a test or capture
// was saved, renamed or deleted) and rebuilds the report from them.
func (a *App) measurementsChanged() {
	a.mu.Lock()
	a.meas = nil
	a.trials, a.trialsOK = nil, false
	a.mu.Unlock()
	a.rebuildReport()
}

// refreshBackground re-reads the startup items and the background programs
// (after closing programs or uninstalling one).
func (a *App) refreshBackground() {
	procs, err := collect.ProcSummaries(a.P)
	startup := collect.StartupSummary(a.P)
	a.patchFacts(func(f *facts.Facts) {
		if err == nil {
			f.Processes = procs
		}
		f.Startup = startup
	})
}

// refreshAfterChange re-reads the machine at the end of a job that changed
// it. The job's cancellation does not apply (what was done before the
// cancel is on the PC and must show), but the read is bounded in time.
func (a *App) refreshAfterChange(ctx context.Context) bool {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	return a.refresh(rctx)
}

// refresh re-reads the machine after a change (keeps stability and games).
//
// Callers pass a context that is not cancelled with the job (a batch that
// was cancelled half-way still changed the PC). A profile from an
// interrupted collection never replaces a complete one: the states are then
// recomputed on the profile already known, so what was applied still shows.
func (a *App) refresh(ctx context.Context) bool {
	f, err := a.collectFacts(ctx, nil, nil)
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		log.Printf("releitura depois da alteração: %v", err)
		if old := a.Facts(); old != nil {
			st := a.computeStates(context.Background(), old)
			stab := a.restab(old, st)
			rep := a.buildReport(old, st, stab)
			a.mu.Lock()
			a.states, a.report, a.stability = st, rep, stab
			a.mu.Unlock()
		}
		return false
	}
	st := a.computeStates(ctx, f)
	stab := a.restab(f, st)
	rep := a.buildReport(f, st, stab)
	a.mu.Lock()
	a.facts, a.states, a.report, a.stability = f, st, rep, stab
	a.mu.Unlock()
	a.gmu.Lock()
	a.cs2 = a.scanCS2(a.games)
	a.gmu.Unlock()
	return true
}

func (a *App) computeStates(ctx context.Context, f *facts.Facts) map[string]tweaks.State {
	out := map[string]tweaks.State{}
	for _, s := range a.Engine.States(ctx, f) {
		out[s.ID] = s
	}
	return out
}

// scanCS2 reads the CS2 installation (with the demo's simulated one).
func (a *App) scanCS2(list []games.Game) games.CS2Info {
	cs2 := a.Scanner.CS2(list)
	if a.P.IsMock && cs2.Video == nil {
		cs2 = mockCS2(list)
		// A cs2_video.txt seeded in the data folder replaces the fake path
		// (without one, the demo is as before). Never call cs2VideoPath here:
		// the caller holds the games lock for writing.
		if p := mockCS2VideoPath(a.P.DataDir); p != "" {
			if b, err := os.ReadFile(p); err == nil {
				if v, ok := games.ParseCS2Video(string(b)); ok {
					v.Path = p
					if st, err := os.Stat(p); err == nil {
						v.Modified = st.ModTime()
					}
					cs2.Video, cs2.AccountID, cs2.VideoCloud = v, "123", "não"
				}
			}
		}
	}
	return cs2
}

// rescanGames refreshes the games list (launchers + manual entries).
func (a *App) rescanGames() {
	list := a.Scanner.Scan()
	if a.P.IsMock && len(list) == 0 {
		list = mockGames() // the demo's library, kept when games are added
	}
	for _, exe := range a.Settings().ManualGames {
		if st, err := os.Stat(exe); err == nil && !st.IsDir() || a.P.IsMock {
			dup := false
			for _, g := range list {
				if strings.EqualFold(g.Exe, exe) {
					dup = true
				}
			}
			if !dup {
				list = append(list, games.Manual(exe))
			}
		}
	}
	cs2 := a.scanCS2(list)
	a.gmu.Lock()
	a.games, a.cs2 = list, cs2
	a.gmu.Unlock()
}

// ---------------------------------------------------------------------------
// Apply / revert
// ---------------------------------------------------------------------------

// ApplyRequest is the body of POST /api/apply.
type ApplyRequest struct {
	IDs          []string `json:"ids"`
	AllowBlocked bool     `json:"allowBlocked"`
	RestorePoint bool     `json:"restorePoint"`
	Label        string   `json:"label"`
	// Skipped are the plan items the user unchecked: not applied, but shown
	// in the result and kept in the batch log.
	Skipped []string `json:"skipped,omitempty"`
}

// BatchResult summarises an apply or revert batch.
type BatchResult struct {
	Results      []tweaks.Result `json:"results"`
	Counts       map[string]int  `json:"counts"`
	Reboot       bool            `json:"reboot"`
	RestorePoint string          `json:"restorePoint"` // created | failed: ... | skipped
	// Score before and after the batch (diagnostic grade), when both exist.
	ScoreBefore int `json:"scoreBefore,omitempty"`
	ScoreAfter  int `json:"scoreAfter,omitempty"`
	// Reread is false when the PC could not be read again after the batch
	// (the score then comes from the previous analysis).
	Reread bool `json:"reread"`
	// Skipped are the plan items the user unchecked (apply batches only).
	Skipped []SkippedItem `json:"skipped,omitempty"`
}

// scoreNow is the current overall diagnostic score (0 when unknown).
func (a *App) scoreNow() int {
	if r := a.Report(); r != nil {
		return r.Overall
	}
	return 0
}

func summarize(res []tweaks.Result) BatchResult {
	br := BatchResult{Results: res, Counts: map[string]int{}}
	for _, r := range res {
		br.Counts[r.Outcome]++
		if r.Reboot == tweaks.RebootRestart.String() && (r.Outcome == tweaks.OutApplied || r.Outcome == tweaks.OutReverted) {
			br.Reboot = true
		}
	}
	return br
}

// StartApply applies tweaks in a job.
func (a *App) StartApply(req ApplyRequest) (Job, error) {
	if a.Facts() == nil {
		return Job{}, ErrNoScan
	}
	if len(req.IDs) == 0 {
		return Job{}, fmt.Errorf("%w: nenhuma otimização selecionada", errBadRequest)
	}
	for _, id := range req.IDs {
		// Startup items are chosen on the Inicialização page, which checks the
		// entry exists (an ID made up here would write an orphan value).
		if strings.HasPrefix(id, "startup:") {
			return Job{}, fmt.Errorf("%w: use a página Inicialização para ligar ou desligar programas", errBadRequest)
		}
		// The DNS change is offered only by the network test (dns.fail or
		// dns.slow of the latest one). Undoing it is never gated.
		if catalog.IsDNSID(id) && !a.netDNSOffered(id) {
			return Job{}, fmt.Errorf("%w: a troca de DNS só é oferecida quando o último teste de rede mostra o DNS atual falhando ou lento", errBadRequest)
		}
		// The CS2 preset goes through its own route (game closed, copy of the
		// file, the filter of each item). Undoing it is never gated.
		if strings.HasPrefix(id, catalog.CS2PresetPrefix) {
			return Job{}, fmt.Errorf("%w: use o cartão Preset competitivo da página CS2", errBadRequest)
		}
	}
	return a.startApply(req)
}

// startApply runs the apply job; the callers did the checks of their route.
func (a *App) startApply(req ApplyRequest) (Job, error) {
	title := "Aplicando otimizações"
	if req.Label != "" {
		title = req.Label
	}
	return a.Jobs.Start("apply", title, true, func(ctx context.Context, c *JobCtl) (any, error) {
		defer func() {
			if r := recover(); r != nil {
				// Keep the write-ahead entries revertible in this session too.
				a.Engine.J.RecoverInterrupted()
				panic(r)
			}
		}()
		f := a.Facts()
		before := a.scoreNow()
		skipped := a.skippedItems(f, req.Skipped, req.IDs)
		if len(skipped) > 0 {
			var names []string
			for _, s := range skipped {
				names = append(names, s.Title)
			}
			c.Logf("desmarcados por você (não aplicados): %s", strings.Join(names, "; "))
		}
		rp := "skipped"
		if req.RestorePoint {
			c.Progress(0.02, "Criando ponto de restauração do Windows")
			if err := a.P.Native.CreateRestorePoint("Axion Optimizer — antes de otimizar"); err != nil {
				rp = "failed: " + err.Error()
				c.Logf("aviso: ponto de restauração não criado: %v (o histórico do Axion continua permitindo desfazer cada mudança)", err)
			} else {
				rp = "created"
				c.Logf("ponto de restauração criado")
			}
		}
		res := a.Engine.Apply(ctx, f, req.IDs, tweaks.ApplyOpts{
			AllowBlocked: req.AllowBlocked,
			Label:        req.Label,
			Progress: func(done, total int, t string) {
				if total > 0 {
					c.Progress(0.08+0.8*float64(done)/float64(total), t)
				}
			},
			Log: c.Logf,
		})
		for _, r := range res {
			c.Logf("%s: %s %s", r.Title, outcomeText(r.Outcome), r.Message)
		}
		a.recordBatch(batchLog(req, skipped, res))
		c.Progress(0.9, "Atualizando o diagnóstico")
		reread := a.refreshAfterChange(ctx)
		br := summarize(res)
		br.RestorePoint = rp
		br.ScoreBefore, br.ScoreAfter, br.Reread = before, a.scoreNow(), reread
		br.Skipped = skipped
		return br, nil
	})
}

// RevertRequest is the body of POST /api/revert.
type RevertRequest struct {
	IDs     []string `json:"ids"`     // tweak IDs
	Entries []string `json:"entries"` // journal entry IDs
	All     bool     `json:"all"`
	Force   bool     `json:"force"`
}

// StartRevert reverts in a job.
func (a *App) StartRevert(req RevertRequest) (Job, error) {
	if !req.All && len(req.IDs) == 0 && len(req.Entries) == 0 {
		return Job{}, fmt.Errorf("%w: nada selecionado para desfazer", errBadRequest)
	}
	return a.Jobs.Start("revert", "Desfazendo alterações", true, func(ctx context.Context, c *JobCtl) (any, error) {
		f := a.Facts()
		if f == nil {
			var err error
			if f, err = a.collectFacts(ctx, c, nil); err != nil {
				return nil, err
			}
		}
		before := a.scoreNow()
		o := tweaks.RevertOpts{Force: req.Force, Log: c.Logf, Progress: func(done, total int, t string) {
			if total > 0 {
				c.Progress(0.05+0.8*float64(done)/float64(total), t)
			}
		}}
		var res []tweaks.Result
		switch {
		case req.All:
			res = a.Engine.RevertAll(ctx, f, o)
		case len(req.Entries) > 0:
			var ents []*tweaks.Entry
			for _, id := range req.Entries {
				if e := a.Engine.J.Get(id); e != nil {
					ents = append(ents, e)
				}
			}
			res = a.Engine.RevertEntries(ctx, f, ents, o)
		default:
			res = a.Engine.RevertTweaks(ctx, f, req.IDs, o)
		}
		for _, r := range res {
			c.Logf("%s: %s %s", r.Title, outcomeText(r.Outcome), r.Message)
		}
		c.Progress(0.9, "Atualizando o diagnóstico")
		reread := a.refreshAfterChange(ctx)
		br := summarize(res)
		br.ScoreBefore, br.ScoreAfter, br.Reread = before, a.scoreNow(), reread
		return br, nil
	})
}

// StartRestorePoint creates a restore point in a job.
func (a *App) StartRestorePoint() (Job, error) {
	return a.Jobs.Start("restorepoint", "Criando ponto de restauração", true, func(ctx context.Context, c *JobCtl) (any, error) {
		c.Progress(0.2, "Pedindo ao Windows um ponto de restauração")
		if err := a.P.Native.CreateRestorePoint("Axion Optimizer — ponto manual"); err != nil {
			return nil, err
		}
		c.Logf("ponto de restauração criado")
		return map[string]string{"restorePoint": "created"}, nil
	})
}

func outcomeText(o string) string {
	switch o {
	case tweaks.OutApplied:
		return "aplicado"
	case tweaks.OutAlready:
		return "já estava aplicado"
	case tweaks.OutNA:
		return "não se aplica"
	case tweaks.OutBlocked:
		return "bloqueado"
	case tweaks.OutFailed:
		return "falhou"
	case tweaks.OutReverted:
		return "desfeito"
	case tweaks.OutConflict:
		return "conflito"
	case tweaks.OutNothing:
		return "nada a desfazer"
	}
	return o
}

// RebootPending reports changes applied or reverted since the last boot that
// only take effect after a restart.
func (a *App) RebootPending() bool {
	f := a.Facts()
	if f == nil || f.OS.LastBoot.IsZero() {
		return false
	}
	boot := f.OS.LastBoot
	need := tweaks.RebootRestart.String()
	for _, e := range a.Engine.J.Snapshot() {
		if e.Reboot != need || !e.Changed() {
			continue
		}
		if e.Status == tweaks.StatusApplied && e.Time.After(boot) {
			return true
		}
		if e.Status == tweaks.StatusReverted && e.RevertedAt != nil && e.RevertedAt.After(boot) && e.Time.Before(boot) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// TweakView joins a tweak's metadata with its live state.
type TweakView struct {
	ID            string          `json:"id"`
	Category      string          `json:"category"`
	Title         string          `json:"title"`
	Summary       string          `json:"summary"`
	Detail        string          `json:"detail"`
	Evidence      string          `json:"evidence"`
	EvidenceLevel int             `json:"evidenceLevel"`
	Risk          string          `json:"risk"`
	RiskLevel     int             `json:"riskLevel"`
	RiskNote      string          `json:"riskNote,omitempty"`
	Reboot        string          `json:"reboot"`
	Tags          []string        `json:"tags"`
	OneClick      bool            `json:"oneClick"`
	Group         string          `json:"group,omitempty"`
	Profile       string          `json:"profile,omitempty"`
	Sources       []tweaks.Source `json:"sources"`
	State         tweaks.State    `json:"state"`
	// "Prova no seu PC": whether the tweak can be proved now (and why not),
	// and the latest outcome measured on this PC. Only TweakViews fills them.
	TrialOK  bool               `json:"trialOk,omitempty"`
	TrialWhy string             `json:"trialWhy,omitempty"`
	Trial    *perf.TrialOutcome `json:"trial,omitempty"`
	// Recommended is what the recommended plan applies for the use profile
	// (the same rule as the plan and the menu counter); ProfileNote says why
	// the profile sets aside an item this PC would otherwise recommend.
	Recommended bool   `json:"recommended"`
	ProfileNote string `json:"profileNote,omitempty"`
}

func view(t *tweaks.Tweak, st tweaks.State) TweakView {
	return TweakView{ID: t.ID, Category: t.Category, Title: t.Title, Summary: t.Summary, Detail: t.Detail,
		Evidence: t.Evidence.String(), EvidenceLevel: int(t.Evidence), Risk: t.Risk.String(), RiskLevel: int(t.Risk), RiskNote: t.RiskNote,
		Reboot: t.Reboot.String(), Tags: t.Tags, OneClick: t.Recommendable(), Group: t.Group, Profile: t.Profile, Sources: t.Sources, State: st}
}

// TweakViews returns every catalog tweak with its cached state.
func (a *App) TweakViews(refresh bool) ([]TweakView, error) {
	f := a.Facts()
	if f == nil {
		return nil, ErrNoScan
	}
	st := a.States()
	if refresh {
		st = a.computeStates(context.Background(), f)
		a.mu.Lock()
		a.states = st
		a.mu.Unlock()
	}
	latest := map[string]perf.TrialOutcome{}
	for _, o := range a.trialOutcomes() {
		if _, ok := latest[o.TweakID]; !ok {
			latest[o.TweakID] = o
		}
	}
	prof := a.profile()
	var out []TweakView
	for _, t := range a.Engine.Catalog.All() {
		v := view(t, st[t.ID])
		v.Recommended, v.ProfileNote = recommendedFor(t, st[t.ID], prof)
		if trialCategories[t.Category] && t.Category != "correcoes" {
			v.TrialOK, v.TrialWhy = a.trialEligible(f, t, st[t.ID])
		}
		if o, ok := latest[t.ID]; ok {
			o := o
			v.Trial = &o
		}
		out = append(out, v)
	}
	return out, nil
}

// GameView is a game with its profile options.
type GameView struct {
	games.Game
	Options []TweakView `json:"options"`
}

// GameViews returns games with the live state of each profile option.
func (a *App) GameViews() ([]GameView, games.CS2Info, error) {
	f := a.Facts()
	if f == nil {
		return nil, games.CS2Info{}, ErrNoScan
	}
	gl, cs2 := a.Games()
	var out []GameView
	for _, g := range gl {
		gv := GameView{Game: g}
		if g.ExeName != "" {
			for _, feat := range catalog.ProfileFeatures {
				t := a.Engine.Resolve(catalog.ProfileID(g.ExeName, feat), f)
				if t == nil {
					continue
				}
				ts := a.Engine.StateOf(context.Background(), f, t)
				v := view(t, ts)
				v.Recommended, v.ProfileNote = recommendedFor(t, ts, a.profile())
				gv.Options = append(gv.Options, v)
			}
		}
		out = append(out, gv)
	}
	return out, cs2, nil
}

// AddManualGame registers an executable chosen by the user.
func (a *App) AddManualGame(exe string) error {
	exe = strings.TrimSpace(exe)
	if !strings.HasSuffix(strings.ToLower(exe), ".exe") {
		return errors.New("escolha um arquivo .exe")
	}
	if catalog.SystemExe(filepath.Base(strings.ReplaceAll(exe, `\`, "/"))) || underWindowsDir(exe, a.P.Native.Paths().WindowsDir) {
		return errors.New("este é um programa do Windows, não um jogo: perfis de jogo não alteram processos do sistema")
	}
	if !a.P.IsMock {
		if st, err := os.Stat(exe); err != nil || st.IsDir() {
			return errors.New("arquivo não encontrado")
		}
	}
	err := a.UpdateSettings(func(s *Settings) {
		for _, e := range s.ManualGames {
			if strings.EqualFold(e, exe) {
				return
			}
		}
		s.ManualGames = append(s.ManualGames, exe)
		sort.Strings(s.ManualGames)
	})
	if err != nil {
		return err
	}
	a.rescanGames()
	return nil
}

// underWindowsDir reports whether path is inside the Windows folder.
func underWindowsDir(path, win string) bool {
	if win == "" {
		win = `C:\Windows`
	}
	p := strings.ToLower(strings.ReplaceAll(path, "/", `\`))
	w := strings.ToLower(strings.TrimRight(win, `\`)) + `\`
	return strings.HasPrefix(p, w)
}

// RemoveManualGame forgets a manual game (profile changes stay in the history).
func (a *App) RemoveManualGame(exe string) error {
	err := a.UpdateSettings(func(s *Settings) {
		var keep []string
		for _, e := range s.ManualGames {
			if !strings.EqualFold(e, exe) {
				keep = append(keep, e)
			}
		}
		s.ManualGames = keep
	})
	if err != nil {
		return err
	}
	a.rescanGames()
	return nil
}

// DismissRequest dismisses (or restores) a finding.
type DismissRequest struct {
	ID      string `json:"id"`
	Dismiss bool   `json:"dismiss"`
}

// DismissFinding stops (or resumes) warning about a finding and rescores
// the current report without a new scan.
func (a *App) DismissFinding(req DismissRequest) (*diag.Report, error) {
	a.mu.RLock()
	f, st, rep := a.facts, a.states, a.report
	stab := append([]diag.Finding(nil), a.stability...)
	a.mu.RUnlock()
	if f == nil || rep == nil {
		return nil, ErrNoScan
	}
	if req.Dismiss {
		var found *diag.Finding
		for i := range rep.Findings {
			if rep.Findings[i].ID == req.ID {
				found = &rep.Findings[i]
			}
		}
		if found == nil {
			return nil, fmt.Errorf("%w: ponto do diagnóstico não encontrado", errBadRequest)
		}
		if !diag.Dismissible(*found) {
			return nil, fmt.Errorf("%w: problemas importantes não podem ser dispensados", errBadRequest)
		}
	}
	err := a.UpdateSettings(func(s *Settings) {
		out := s.Dismissed[:0:0]
		for _, id := range s.Dismissed {
			if id != req.ID {
				out = append(out, id)
			}
		}
		if req.Dismiss {
			out = append(out, req.ID)
		}
		s.Dismissed = out
	})
	if err != nil {
		return nil, err
	}
	nrep := a.buildReport(f, st, stab)
	a.mu.Lock()
	a.report = nrep
	a.mu.Unlock()
	return nrep, nil
}

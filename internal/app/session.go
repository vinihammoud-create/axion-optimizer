package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"axion/internal/games"
	"axion/internal/perf"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// The automatic match report (0.9.x). With it turned on for a game, a
// goroutine looks at the process list every few seconds (a read-only
// snapshot, by name: no handle on any process). When the game shows up the
// session is recorded in windows of PresentMon, the same program and options
// as the manual measurement, with its own ETW session; the frames of every
// window go into one perf.Accum. When the game closes the result is saved as
// a perf.Run with the Session field. Nothing is drawn over the game, nothing
// is hooked or written into it, and the match report never changes the PC.

const (
	sessionKeep     = 50 // reports kept on disk
	sessionMaxGames = 20 // games with the automatic report on
)

type sessionTiming struct {
	Poll    time.Duration // pause between looks at the process list
	Segment time.Duration // length of each PresentMon window
	Max     time.Duration // longest session recorded
	MinRec  time.Duration // shortest recording saved as a report
	Gone    int           // consecutive looks without the game that end the session
}

var defaultSessionTiming = sessionTiming{Poll: 5 * time.Second, Segment: 3 * time.Minute, Max: 3 * time.Hour, MinRec: time.Minute, Gone: 2}

type sessionWatch struct {
	mu         sync.Mutex
	cancel     context.CancelFunc
	done       chan struct{}
	wake       chan struct{} // buffered 1: settings changed, look now
	stopped    bool          // StopSessions ran: never start again
	live       *SessionLive
	liveCancel context.CancelFunc
	segCancel  context.CancelFunc // cuts the window being recorded (a manual measurement asked for PresentMon)
	polls      int
	pollTime   time.Duration // measured with Native.Monotonic (Go's clock on Windows is coarse)
	notices    []SessionNotice
	noticesOK  bool
}

// SessionLive is the session being recorded now.
type SessionLive struct {
	Exe     string    `json:"exe"`
	Name    string    `json:"name"`
	Started time.Time `json:"started"`
	Seconds float64   `json:"seconds"` // recorded so far (finished windows)
	Windows int       `json:"windows"`
	Waiting string    `json:"waiting,omitempty"` // why it is not recording right now
}

// SessionNotice is a report not opened yet (shown at the next look at the window).
type SessionNotice struct {
	ID      string    `json:"id"`
	Exe     string    `json:"exe"`
	Name    string    `json:"name"`
	Ended   time.Time `json:"ended"`
	AvgFPS  float64   `json:"avgFps"`
	Low1FPS float64   `json:"low1Fps"`
}

type SessionGame struct {
	Exe         string `json:"exe"`
	Name        string `json:"name"`
	Running     bool   `json:"running"`
	Auto        bool   `json:"auto"`
	Competitive bool   `json:"competitive"`
	AntiCheat   string `json:"antiCheat,omitempty"`
}

type SessionsView struct {
	Ready     bool            `json:"ready"`
	Why       string          `json:"why,omitempty"`
	Live      *SessionLive    `json:"live,omitempty"`
	Games     []SessionGame   `json:"games"`
	Reports   []perf.Run      `json:"reports"` // newest first, without series and windows
	Polls     int             `json:"polls"`
	PollAvgMs float64         `json:"pollAvgMs"` // measured cost of one look at the process list
	PollEvery int             `json:"pollEvery"` // seconds
	Segment   int             `json:"segment"`   // seconds per window
	Honest    string          `json:"honest"`
	Sources   []tweaks.Source `json:"sources"`
}

type AutoReportRequest struct {
	Exe string `json:"exe"`
	On  bool   `json:"on"`
}

type SessionReport struct {
	Run      *perf.Run       `json:"run"`
	Previous *perf.Run       `json:"previous,omitempty"` // the session before, same game (no series)
	Compare  *PerfComparison `json:"compare,omitempty"`
}

var srcToolhelp = tweaks.Source{Title: "Microsoft Learn — CreateToolhelp32Snapshot: foto da lista de processos; \"o acesso à foto é somente leitura\"", URL: "https://learn.microsoft.com/windows/win32/api/tlhelp32/nf-tlhelp32-createtoolhelp32snapshot"}

const sessionHonest = "O relatório da partida usa o mesmo PresentMon da medição manual: ele só lê eventos do Windows (ETW) e não injeta nada no jogo. " +
	"Para saber que o jogo abriu, o Axion tira a cada poucos segundos uma foto da lista de processos do Windows e procura o nome do executável, sem abrir o processo do jogo. " +
	"No início e no fim da partida, para saber quais programas disputaram o processador, o Axion consulta (só leitura) o tempo de processador de cada programa, como na medição manual. " +
	"Nada é desenhado sobre o jogo e nada no jogo ou no Windows é alterado. " +
	"O Axion precisa ficar aberto (pode ficar minimizado): fechado, ele não acompanha nada. " +
	"Se você iniciar uma medição manual durante a partida, a gravação do relatório pausa até a medição terminar. " +
	"Uma partida não é um cenário repetido, porque mapa, rodada e número de jogadores mudam o FPS. " +
	"Use o relatório para ver como a partida rodou e para notar mudanças grandes (engasgos, temperatura, limite da placa). " +
	"Para saber se um ajuste funcionou, use a medição manual no mesmo cenário."

func (a *App) sessionTiming() sessionTiming {
	if a.sessTimingOverride != nil {
		return *a.sessTimingOverride
	}
	return defaultSessionTiming
}

func (a *App) sessionDir() string { return filepath.Join(a.perfDir(), "partidas") }

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (a *App) autoReportSet() map[string]bool {
	m := map[string]bool{}
	for _, e := range a.Settings().AutoReport {
		m[strings.ToLower(e)] = true
	}
	return m
}

// durText writes a duration for the user: "45 s", "3 min", "1 min 30 s", "3 h".
func durText(secs int) string {
	switch {
	case secs < 60:
		return fmt.Sprintf("%d s", secs)
	case secs%3600 == 0:
		return fmt.Sprintf("%d h", secs/3600)
	case secs%60 == 0:
		return fmt.Sprintf("%d min", secs/60)
	}
	return fmt.Sprintf("%d min %d s", secs/60, secs%60)
}

// ensureWatcher starts the watcher once some game is turned on (and wakes it
// when the list changes). It reads the settings BEFORE taking sess.mu.
func (a *App) ensureWatcher() {
	n := len(a.Settings().AutoReport)
	a.sess.mu.Lock()
	defer a.sess.mu.Unlock()
	if a.sess.stopped {
		return
	}
	if a.sess.cancel != nil {
		select {
		case a.sess.wake <- struct{}{}:
		default:
		}
		return
	}
	if n == 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done, wake := make(chan struct{}), make(chan struct{}, 1)
	a.sess.cancel, a.sess.done, a.sess.wake = cancel, done, wake
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				log.Printf("relatório da partida: falha interna: %v\n%s", r, debug.Stack())
				cancel()
				a.releasePM("sessao")
				a.sess.mu.Lock()
				if a.sess.done == done {
					a.sess.cancel = nil
				}
				a.sess.mu.Unlock()
			}
		}()
		a.watchGames(ctx, wake)
	}()
}

// StopSessions ends the watcher (the window is closing): a session being
// recorded is saved with what was recorded. It reports whether it ended in time.
func (a *App) StopSessions(max time.Duration) bool {
	a.sess.mu.Lock()
	a.sess.stopped = true
	cancel, done := a.sess.cancel, a.sess.done
	a.sess.cancel = nil
	a.sess.mu.Unlock()
	if cancel == nil {
		return true
	}
	cancel()
	select {
	case <-done:
		return true
	case <-time.After(max):
		return false
	}
}

// pollGames takes one read-only snapshot of the process list per game turned
// on (look by name, no process handle). The cost is measured with Native.Monotonic.
func (a *App) pollGames(order []string) map[string]bool {
	t0 := a.P.Native.Monotonic()
	out := map[string]bool{}
	for _, exe := range order {
		out[exe] = len(a.P.Native.PIDsByName(exe)) > 0
	}
	d := a.P.Native.Monotonic() - t0
	a.sess.mu.Lock()
	a.sess.polls++
	a.sess.pollTime += d
	a.sess.mu.Unlock()
	return out
}

func (a *App) watchGames(ctx context.Context, wake chan struct{}) {
	a.removeSessionLeftovers()
	t := a.sessionTiming()
	absent := map[string]bool{} // seen closed at least once since following began (else: the report starts late)
	hold := map[string]bool{}   // a session ended without the game closing (limit, failure): wait for it to close
	tick := time.NewTicker(t.Poll)
	defer tick.Stop()
	for {
		// Without the analysis there is no GPU identified (no sensors) and no game names.
		if order := a.Settings().AutoReport; len(order) > 0 && a.P.Native.IsElevated() && a.Facts() != nil {
			running := a.pollGames(order)
			for _, exe := range order {
				if !running[exe] {
					absent[exe] = true
					delete(hold, exe)
				}
			}
			for _, exe := range order {
				if !running[exe] || hold[exe] {
					continue
				}
				reason := a.recordSession(ctx, exe, !absent[exe])
				if reason == "limite" || reason == "falha" {
					hold[exe] = true
				}
				absent[exe] = false
				break // one game at a time
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-wake:
		}
	}
}

func (a *App) setSegCancel(c context.CancelFunc) {
	a.sess.mu.Lock()
	a.sess.segCancel = c
	a.sess.mu.Unlock()
}

// claimPerfPM gives PresentMon to a manual measurement. When a window of the
// match report holds it, the window is cut (what it recorded is kept) and the
// report waits until the measurement ends. It gives up after 30 s.
func (a *App) claimPerfPM(ctx context.Context) bool {
	if a.claimPM("perf") {
		return true
	}
	a.pmWant.Add(1)
	defer a.pmWant.Add(-1)
	deadline := time.Now().Add(30 * time.Second)
	for {
		a.sess.mu.Lock()
		cut := a.sess.segCancel
		a.sess.mu.Unlock()
		if cut != nil {
			cut()
		}
		if a.claimPM("perf") {
			return true
		}
		if time.Now().After(deadline) || !sleepCtx(ctx, 100*time.Millisecond) {
			return false
		}
	}
}

func (a *App) recordSession(parent context.Context, exe string, late bool) string {
	t := a.sessionTiming()
	pm, err := a.presentMonPath()
	if err != nil {
		log.Printf("relatório da partida: %v", err)
		return "falha"
	}
	dir := a.sessionDir()
	if err := a.P.Native.ProtectDir(dir); err != nil {
		log.Printf("relatório da partida: pasta: %v", err)
		return "falha"
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	started := time.Now()
	id := perf.NewID(started)
	name := a.gameName(exe)
	a.setLive(&SessionLive{Exe: exe, Name: name, Started: started}, cancel)
	defer a.setLive(nil, nil)
	log.Printf("relatório da partida: %s aberto, gravando (%s)", name, id)
	sensors := a.startSensors(ctx, exe)
	sensors.Recording(sensors.start) // the whole session counts
	defer sensors.cancel()
	info := perf.SessionInfo{Name: name, Late: late, SegmentSecs: int(t.Segment / time.Second)}
	var acc perf.Accum
	var runtime, reason string
	var swaps, fails, gone, n int
	var first, lastEnd time.Time
loop:
	for {
		switch {
		case parent.Err() != nil:
			reason = "fechado"
			break loop
		case ctx.Err() != nil || !a.autoReportSet()[strings.ToLower(exe)]:
			reason = "desligado"
			break loop
		case time.Since(started) >= t.Max || acc.Seconds() >= t.Max.Seconds():
			reason = "limite"
			break loop
		}
		if len(a.P.Native.PIDsByName(exe)) == 0 {
			if gone++; gone >= t.Gone {
				reason = "fim"
				break loop
			}
			sleepCtx(ctx, t.Poll)
			continue
		}
		gone = 0
		// The cut of the window is set before the claim, so a manual
		// measurement that asks for PresentMon always finds it.
		segCtx, segCancel := context.WithCancel(ctx)
		a.setSegCancel(segCancel)
		if a.pmWant.Load() > 0 || !a.claimPM("sessao") {
			segCancel()
			a.setSegCancel(nil)
			info.Paused = true
			a.updateLive(func(l *SessionLive) { l.Waiting = "uma medição manual está usando o PresentMon" })
			sleepCtx(ctx, t.Poll)
			continue
		}
		a.updateLive(func(l *SessionLive) { l.Waiting = "" })
		n++
		if first.IsZero() {
			first = time.Now()
		}
		capt, err := func() (*perf.Capture, error) {
			defer a.releasePM("sessao")
			return a.sessionSegment(segCtx, pm, dir, id, n, exe, t.Segment)
		}()
		cut := segCtx.Err() != nil && ctx.Err() == nil // a manual measurement took PresentMon
		segCancel()
		a.setSegCancel(nil)
		if cut {
			info.Paused = true
		}
		switch {
		case err == nil:
			fails = 0
			lastEnd = time.Now()
			info.Segments = append(info.Segments, perf.SessionSegment{From: math.Round(acc.Seconds()*10) / 10, Summary: perf.Summarize(capt)})
			acc.Add(capt)
			if runtime == "" {
				runtime = capt.Runtime
			}
			if capt.Swapchains > swaps {
				swaps = capt.Swapchains
			}
			a.updateLive(func(l *SessionLive) { l.Seconds, l.Windows = acc.Seconds(), len(info.Segments) })
		case errors.Is(err, perf.ErrNoFrames):
			fails = 0
			if len(a.P.Native.PIDsByName(exe)) > 0 {
				info.Empty++ // open but drawing nothing (minimized, loading)
				sleepCtx(ctx, t.Poll)
			}
		case cut || ctx.Err() != nil:
			// the head of the loop says why
		default:
			log.Printf("relatório da partida: janela %d de %s: %v", n, name, err)
			if fails++; fails >= 3 {
				reason = "falha"
				break loop
			}
			sleepCtx(ctx, t.Poll)
		}
	}
	cond, sens := sensors.Finish()
	info.Ended, info.Reason = time.Now(), reason
	if !lastEnd.IsZero() {
		info.WallSecs = math.Round(lastEnd.Sub(first).Seconds()*10) / 10
	}
	a.saveSession(id, exe, started, &acc, info, runtime, swaps, cond, sens)
	log.Printf("relatório da partida: %s terminou (%s), %.0f s gravados em %d janela(s)", name, reason, acc.Seconds(), len(info.Segments))
	return reason
}

// sessionSegment records one window. A window cut short (the game closed,
// Axion closing, a manual measurement) still returns what was recorded; the
// CSV is always deleted.
func (a *App) sessionSegment(ctx context.Context, pm, dir, id string, n int, exe string, seg time.Duration) (*perf.Capture, error) {
	csvPath := filepath.Join(dir, fmt.Sprintf("%s-j%03d.csv", id, n))
	defer os.Remove(csvPath)
	secs := int(seg / time.Second)
	if secs < 1 {
		secs = 1
	}
	args := []string{"--process_name", exe, "--output_file", csvPath, "--timed", strconv.Itoa(secs), "--terminate_after_timed",
		"--terminate_on_proc_exit", "--no_console_stats", "--stop_existing_session", "--session_name", sessionPMName, "--track_pc_latency"}
	res, err := a.P.Run.Run(ctx, pm, args, sys.RunOpts{Timeout: seg + time.Minute})
	if err != nil {
		// A killed PresentMon leaves its ETW session open. The wait is short when Axion is closing.
		wait := 20 * time.Second
		if ctx.Err() != nil {
			wait = 5 * time.Second
		}
		a.stopPMSessionNamed(pm, sessionPMName, wait)
	}
	capt, perr := readCapture(csvPath, exe)
	switch {
	case perr == nil:
		return capt, nil
	case !errors.Is(perr, perf.ErrNoFrames):
		return nil, perr
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case err != nil:
		return nil, fmt.Errorf("PresentMon: %w", err)
	case res.ExitCode != 0:
		return nil, fmt.Errorf("o PresentMon terminou com erro (código %d): %s", res.ExitCode, lastLine(res.Stderr+"\n"+res.Stdout))
	}
	return nil, perf.ErrNoFrames
}

func (a *App) saveSession(id, exe string, started time.Time, acc *perf.Accum, info perf.SessionInfo, runtime string, swaps int, cond *perf.Conditions, sens *perf.Sensors) {
	t := a.sessionTiming()
	if acc.Frames() < 1000 || acc.Seconds() < t.MinRec.Seconds() {
		log.Printf("relatório da partida: %s com %.0f s gravados (mínimo %.0f s): relatório não gerado", info.Name, acc.Seconds(), t.MinRec.Seconds())
		return
	}
	if strings.EqualFold(exe, "cs2.exe") {
		_, cs2 := a.Games()
		info.Setting = fpsCapLabel(cs2)
	}
	run := &perf.Run{ID: id, Process: exe, Started: started, Runtime: runtime, Swapchains: swaps, PresentMon: presentMonVersion,
		Summary: acc.Summary(), Series: acc.Series(1200), Note: sessionNote(info, t.Max), Conditions: cond, Sensors: sens, Session: &info}
	dir := a.sessionDir()
	if err := perf.Save(dir, run); err != nil {
		log.Printf("relatório da partida: salvar: %v", err)
		return
	}
	if k := perf.Prune(dir, sessionKeep); k > 0 {
		log.Printf("relatório da partida: %d relatório(s) antigo(s) apagado(s)", k)
	}
	_ = a.P.Native.ProtectDir(dir)
	a.noticesChanged()
}

func sessionNote(si perf.SessionInfo, max time.Duration) string {
	var out []string
	if si.Late {
		out = append(out, "o jogo já estava aberto quando o Axion começou a acompanhar: o começo da partida ficou de fora")
	}
	switch si.Reason {
	case "limite":
		out = append(out, "gravadas só as primeiras "+durText(int(max/time.Second))+" da partida (limite do Axion)")
	case "fechado":
		out = append(out, "o Axion foi fechado durante o jogo: o relatório vai até esse momento")
	case "desligado":
		out = append(out, "o relatório automático deste jogo foi desligado durante a partida: o relatório vai até esse momento")
	case "falha":
		out = append(out, "o PresentMon falhou 3 vezes seguidas: o relatório vai até a última janela gravada")
	}
	if si.Paused {
		out = append(out, "a gravação parou enquanto uma medição manual usava o PresentMon")
	}
	if si.Empty > 0 {
		out = append(out, fmt.Sprintf("%d janela(s) sem quadros com o jogo aberto (minimizado, em carregamento ou parado)", si.Empty))
	}
	return strings.Join(out, "; ")
}

func (a *App) setLive(l *SessionLive, cancel context.CancelFunc) {
	a.sess.mu.Lock()
	a.sess.live, a.sess.liveCancel = l, cancel
	a.sess.mu.Unlock()
}

func (a *App) updateLive(fn func(l *SessionLive)) {
	a.sess.mu.Lock()
	if a.sess.live != nil {
		fn(a.sess.live)
	}
	a.sess.mu.Unlock()
}

func (a *App) sessionLive() *SessionLive {
	a.sess.mu.Lock()
	defer a.sess.mu.Unlock()
	if a.sess.live == nil {
		return nil
	}
	l := *a.sess.live
	return &l
}

func (a *App) liveName() string {
	if l := a.sessionLive(); l != nil {
		return l.Name
	}
	return "um jogo"
}

// removeSessionLeftovers deletes window CSVs left by an Axion that ended mid-session.
func (a *App) removeSessionLeftovers() {
	m, _ := filepath.Glob(filepath.Join(a.sessionDir(), "*-j*.csv"))
	for _, f := range m {
		if i := strings.Index(filepath.Base(f), "-j"); i > 0 && perf.ValidID(filepath.Base(f)[:i]) {
			_ = os.Remove(f)
		}
	}
}

func (a *App) Sessions() SessionsView {
	t := a.sessionTiming()
	v := SessionsView{PollEvery: int(t.Poll / time.Second), Segment: int(t.Segment / time.Second), Honest: sessionHonest,
		Sources: []tweaks.Source{srcPMReadme, srcToolhelp, srcFrameView, srcCapFrameX, srcGPUBusy}}
	switch _, err := a.presentMonPath(); {
	case !a.P.Native.IsElevated():
		v.Why = "execute o Axion como administrador: o PresentMon lê eventos do Windows que exigem essa permissão"
	case err != nil:
		v.Why = err.Error()
	case a.Facts() == nil:
		v.Why = "faça a análise do PC primeiro: ela identifica a placa de vídeo e os jogos"
	default:
		v.Ready = true
	}
	auto := a.autoReportSet()
	lib, _ := a.Games()
	meta := map[string]games.Game{}
	for _, g := range lib {
		meta[strings.ToLower(g.ExeName)] = g
	}
	listed := map[string]bool{}
	for _, p := range a.perfCandidates() {
		k := strings.ToLower(p.Exe)
		listed[k] = true
		g := meta[k]
		v.Games = append(v.Games, SessionGame{Exe: p.Exe, Name: p.Name, Running: len(a.P.Native.PIDsByName(p.Exe)) > 0, Auto: auto[k], Competitive: g.Competitive || k == "cs2.exe", AntiCheat: g.AntiCheat})
	}
	// A game turned on that left the library (uninstalled) can still be turned off.
	for _, exe := range a.Settings().AutoReport {
		if !listed[strings.ToLower(exe)] {
			v.Games = append(v.Games, SessionGame{Exe: exe, Name: a.gameName(exe), Auto: true, Running: len(a.P.Native.PIDsByName(exe)) > 0})
		}
	}
	sort.SliceStable(v.Games, func(i, j int) bool { return v.Games[i].Auto && !v.Games[j].Auto })
	v.Reports = []perf.Run{}
	for _, r := range perf.List(a.sessionDir()) {
		if r.Session == nil {
			continue
		}
		si := *r.Session
		si.Segments = nil
		r.Session = &si
		v.Reports = append(v.Reports, r)
	}
	a.sess.mu.Lock()
	if a.sess.live != nil {
		l := *a.sess.live
		v.Live = &l
	}
	v.Polls = a.sess.polls
	if a.sess.polls > 0 {
		v.PollAvgMs = math.Round(float64(a.sess.pollTime)/float64(a.sess.polls)/1e3) / 1e3
	}
	a.sess.mu.Unlock()
	return v
}

func (a *App) SetAutoReport(req AutoReportRequest) (SessionsView, error) {
	exe := strings.TrimSpace(req.Exe)
	if !strings.HasSuffix(strings.ToLower(exe), ".exe") {
		exe += ".exe"
	}
	if !validExe(exe) {
		return SessionsView{}, fmt.Errorf("%w: nome de executável inválido", errBadRequest)
	}
	key := strings.ToLower(exe)
	if req.On {
		known := false
		for _, p := range a.perfCandidates() {
			known = known || strings.EqualFold(p.Exe, key)
		}
		if !known {
			return SessionsView{}, fmt.Errorf("%w: %s não está na biblioteca de jogos; adicione-o em Jogos › Biblioteca de jogos", errBadRequest, exe)
		}
		if len(a.Settings().AutoReport) >= sessionMaxGames && !a.autoReportSet()[key] {
			return SessionsView{}, fmt.Errorf("%w: no máximo %d jogos com relatório automático", errBadRequest, sessionMaxGames)
		}
	}
	if err := a.UpdateSettings(func(s *Settings) {
		var out []string
		for _, e := range s.AutoReport {
			if !strings.EqualFold(e, key) {
				out = append(out, e)
			}
		}
		if req.On {
			out = append(out, key)
		}
		s.AutoReport = out
	}); err != nil {
		return SessionsView{}, err
	}
	if !req.On {
		a.sess.mu.Lock()
		if a.sess.live != nil && strings.EqualFold(a.sess.live.Exe, key) && a.sess.liveCancel != nil {
			a.sess.liveCancel()
		}
		a.sess.mu.Unlock()
	}
	a.ensureWatcher()
	return a.Sessions(), nil
}

func (a *App) SessionReport(id string) (*SessionReport, error) {
	if !perf.ValidID(id) {
		return nil, fmt.Errorf("%w: partida inválida", errBadRequest)
	}
	dir := a.sessionDir()
	r, err := perf.Load(dir, id)
	if err != nil || r.Session == nil {
		return nil, fmt.Errorf("%w: partida não encontrada", errBadRequest)
	}
	out := &SessionReport{Run: r}
	for _, p := range perf.List(dir) { // newest first: the first older one of the same game is the previous
		if p.ID != r.ID && p.Session != nil && strings.EqualFold(p.Process, r.Process) && p.Started.Before(r.Started) {
			// List drops the chart series and keeps the windows, which the comparison needs.
			prev := p
			cmp := a.compareSessions(&prev, r)
			si := *prev.Session
			si.Segments = nil
			prev.Session = &si
			out.Previous, out.Compare = &prev, &cmp
			break
		}
	}
	return out, nil
}

// compareSessions compares two sessions of the same game with the windows as
// samples. Sessions are not repeated scenarios, so no row gets a verdict of
// "better" or "worse": every row is only an indication.
func (a *App) compareSessions(prev, cur *perf.Run) PerfComparison {
	windows := func(r *perf.Run) []perf.Summary {
		var out []perf.Summary
		min := 0.5 * float64(r.Session.SegmentSecs)
		for _, s := range r.Session.Segments {
			if s.Summary.Frames >= 1000 && s.Summary.Seconds >= min {
				out = append(out, s.Summary)
			}
		}
		if len(out) == 0 {
			out = []perf.Summary{r.Summary}
		}
		return out
	}
	la, lb := "partida anterior", "esta partida"
	c := perf.Compare(la, windows(prev), lb, windows(cur))
	for i := range c.Rows {
		c.Rows[i].P, c.Rows[i].PAdj, c.Rows[i].Verdict = -1, -1, "indicativo"
	}
	warn := []string{"Cada lado são as janelas de " + durText(cur.Session.SegmentSecs) + " de uma partida, não medições repetidas do mesmo cenário: mapa, rodada, número de jogadores e o que acontece na tela mudam o FPS. Leia como indicação. Para saber se um ajuste funcionou, use a medição manual (aba Desempenho) no mesmo cenário."}
	for _, w := range c.Warnings {
		switch {
		case strings.HasPrefix(w, "Com 2 medições"):
			continue
		case strings.HasPrefix(w, "Com só uma medição"):
			w = "Uma das partidas tem só uma janela completa de gravação: os números aparecem lado a lado, mas não dá para separar uma mudança da variação natural."
		case strings.HasPrefix(w, "As medições do grupo"):
			name := lb
			if strings.Contains(w, "\""+la+"\"") {
				name = la
			}
			w = fmt.Sprintf("As janelas de \"%s\" variam bastante entre si: é normal numa partida (rodadas, mapa, jogadores).", name)
		}
		warn = append(warn, w)
	}
	warn = append(warn, perf.CompareConditions(prev.Conditions, cur.Conditions)...)
	for _, g := range []struct {
		name string
		r    *perf.Run
	}{{la, prev}, {lb, cur}} {
		for _, x := range perf.Remarks(g.r) {
			if x.Level == "warn" {
				warn = append(warn, "Em \""+g.name+"\" ("+g.r.Started.Local().Format("02/01 15:04")+"): "+x.Title+".")
				break
			}
		}
	}
	c.Warnings = warn
	strip := func(r *perf.Run) perf.Run {
		x := *r
		x.Series = nil
		if x.Session != nil {
			si := *x.Session
			si.Segments = nil
			x.Session = &si
		}
		return x
	}
	return PerfComparison{Comparison: c, LabelA: la, LabelB: lb, RunsA: []perf.Run{strip(prev)}, RunsB: []perf.Run{strip(cur)}}
}

func (a *App) MarkSessionSeen(req PerfIDRequest) error {
	if !perf.ValidID(req.ID) {
		return fmt.Errorf("%w: partida inválida", errBadRequest)
	}
	r, err := perf.Load(a.sessionDir(), req.ID)
	if err != nil || r.Session == nil {
		return fmt.Errorf("%w: partida não encontrada", errBadRequest)
	}
	if r.Session.Seen {
		return nil
	}
	r.Session.Seen = true
	if err := perf.Save(a.sessionDir(), r); err != nil {
		return err
	}
	a.noticesChanged()
	return nil
}

func (a *App) DeleteSession(req PerfIDRequest) error {
	if !perf.ValidID(req.ID) {
		return fmt.Errorf("%w: partida inválida", errBadRequest)
	}
	if err := perf.Delete(a.sessionDir(), req.ID); err != nil {
		return fmt.Errorf("%w: partida não encontrada", errBadRequest)
	}
	a.noticesChanged()
	return nil
}

// sessionNotices lists the reports not opened yet (newest 5), cached until a
// report is saved, opened or deleted.
func (a *App) sessionNotices() []SessionNotice {
	a.sess.mu.Lock()
	if a.sess.noticesOK {
		out := append([]SessionNotice(nil), a.sess.notices...)
		a.sess.mu.Unlock()
		return out
	}
	a.sess.mu.Unlock()
	var out []SessionNotice
	for _, r := range perf.List(a.sessionDir()) {
		if r.Session == nil || r.Session.Seen {
			continue
		}
		out = append(out, SessionNotice{ID: r.ID, Exe: r.Process, Name: r.Session.Name, Ended: r.Session.Ended, AvgFPS: r.Summary.AvgFPS, Low1FPS: r.Summary.Low1FPS})
		if len(out) == 5 {
			break
		}
	}
	a.sess.mu.Lock()
	a.sess.notices, a.sess.noticesOK = out, true
	a.sess.mu.Unlock()
	return out
}

func (a *App) noticesChanged() {
	a.sess.mu.Lock()
	a.sess.noticesOK = false
	a.sess.mu.Unlock()
}

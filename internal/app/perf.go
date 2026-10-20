package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"axion/internal/collect"
	"axion/internal/perf"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// PresentMon (Intel, MIT license) ships next to Axion.exe. Before each use
// the file is checked against this SHA-256 (the signed release of GitHub,
// signer "Intel Corporation") and copied into the administrators-only folder
// %ProgramData%\Axion\bin, from where it runs.
const (
	presentMonName    = "PresentMon-2.6.0-x64.exe"
	presentMonVersion = "2.6.0"
	presentMonSHA256  = "b2a706bc6ad475749e3b7e3409263aa1e6906d45bdcf993f6dbc0f660188f1af"
	perfSession       = "AxionPresentMon"
	sessionPMName     = "AxionPartida" // ETW session of the match report (the manual one is perfSession)
	perfKeep          = 100            // runs kept on disk
)

var (
	srcPMReadme  = tweaks.Source{Title: "Intel PresentMon (GitHub) — aplicativo de console, opções e definição das colunas", URL: "https://github.com/GameTechDev/PresentMon/blob/main/README-ConsoleApplication.md"}
	srcPMIntel   = tweaks.Source{Title: "Intel — PresentMon: métrica GPU Busy para ver o equilíbrio CPU × GPU", URL: "https://game.intel.com/us/stories/intel-presentmon/"}
	srcGPUBusy   = tweaks.Source{Title: "TechSpot — Como usar o Intel PresentMon: tempo de quadro muito maior que o GPU Busy indica limite de CPU", URL: "https://www.techspot.com/article/2723-intel-presentmon/"}
	srcFrameView = tweaks.Source{Title: "NVIDIA FrameView — definição de 1% low e 0,1% low (média dos quadros mais lentos) e de percentil", URL: "https://images.nvidia.com/content/geforce/technologies/frameview/frameview-1-4-user-guide-web-version.pdf"}
	srcCapFrameX = tweaks.Source{Title: "CapFrameX (código-fonte) — engasgo = quadro acima de 2,5 × o tempo médio (padrão)", URL: "https://github.com/CXWorld/CapFrameX/blob/master/source/CapFrameX.Configuration/CapFrameXConfiguration.cs"}
	srcNISTt     = tweaks.Source{Title: "NIST/SEMATECH e-Handbook of Statistical Methods — teste t para duas amostras (variâncias diferentes)", URL: "https://www.itl.nist.gov/div898/handbook/eda/section3/eda353.htm"}
	srcFlipModel = tweaks.Source{Title: "Microsoft DirectX Blog — modelo flip: independent flip tem a latência da tela cheia exclusiva; composto passa pelo DWM", URL: "https://devblogs.microsoft.com/directx/dxgi-flip-model/"}
)

var (
	// Hotkeys PresentMon 2.6.0 really registers: its parser keeps only the
	// last modifier of a combination ("ALT+SHIFT+F11" becomes SHIFT+F11,
	// which then never matches Alt+Shift+F11), so only single keys or one
	// modifier are offered.
	perfHotkeys = []string{"SCROLLLOCK", "PAUSE", "SHIFT+F11"}
	pmCopyMu    sync.Mutex // one copy of PresentMon into the data folder at a time
	// Executable names: words separated by single spaces, no word starting
	// with "-" (never an option for PresentMon), a single ".exe" at the end.
	reExeName = regexp.MustCompile(`(?i)^[a-z0-9][a-z0-9._()+-]*( [a-z0-9._()+][a-z0-9._()+-]*)*\.exe$`)
)

func validExe(name string) bool {
	return len(name) <= 64 && reExeName.MatchString(name) && strings.Count(strings.ToLower(name), ".exe") == 1
}

func (a *App) perfDir() string { return filepath.Join(a.P.DataDir, "desempenho") }

func sha256File(path string) (string, []byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), b, nil
}

// presentMonPath returns the verified PresentMon inside the protected folder.
func (a *App) presentMonPath() (string, error) {
	if a.P.IsMock {
		return presentMonName, nil
	}
	pmCopyMu.Lock()
	defer pmCopyMu.Unlock()
	bin := filepath.Join(a.P.DataDir, "bin")
	dst := filepath.Join(bin, presentMonName)
	if h, _, err := sha256File(dst); err == nil && h == presentMonSHA256 {
		return dst, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	src := filepath.Join(filepath.Dir(exe), presentMonName)
	h, data, err := sha256File(src)
	if err != nil {
		return "", fmt.Errorf("%s não está ao lado do Axion.exe: extraia o ZIP inteiro", presentMonName)
	}
	if h != presentMonSHA256 {
		return "", fmt.Errorf("%s ao lado do Axion.exe foi alterado (SHA-256 não confere); baixe o pacote de novo", presentMonName)
	}
	if err := a.P.Native.ProtectDir(bin); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(bin, "pm-*.tmp")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(tmp)
		return "", werr
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return dst, nil
}

// PerfProcess is a suggestion for the process to measure.
type PerfProcess struct {
	Exe     string `json:"exe"`
	Name    string `json:"name"`
	Running bool   `json:"running"`
}

// PerfView is the Desempenho page.
type PerfView struct {
	Ready     bool            `json:"ready"`
	Why       string          `json:"why,omitempty"`
	Version   string          `json:"version"`
	Elevated  bool            `json:"elevated"`
	Runs      []perf.Run      `json:"runs"`
	Labels    []string        `json:"labels"`
	Processes []PerfProcess   `json:"processes"`
	Hotkeys   []string        `json:"hotkeys"`
	Honest    string          `json:"honest"`
	Sources   []tweaks.Source `json:"sources"`
	// Preflight lists what is wrong with the PC right now for a measurement
	// (on the battery, an economy power plan): said before it is taken, not
	// only after.
	Preflight []perf.Remark `json:"preflight,omitempty"`
}

// Perf builds the Desempenho page.
func (a *App) Perf() PerfView {
	v := PerfView{Version: presentMonVersion, Elevated: a.P.Native.IsElevated(), Hotkeys: perfHotkeys,
		Sources: []tweaks.Source{srcPMReadme, srcPMIntel, srcGPUBusy, srcFrameView, srcCapFrameX, srcNISTt, srcFlipModel},
		Honest: "A medição usa o PresentMon, da Intel, o mesmo que revisores usam: ele só lê eventos do Windows (ETW) e não injeta nada no jogo. " +
			"Um número isolado de FPS diz pouco: compare sempre o mesmo cenário (mesmo mapa e trajeto, de preferência um mapa de benchmark), com o jogo já carregado, " +
			"e grave de 2 a 3 medições antes e depois de mudar algo. Só assim dá para separar uma melhora real da variação natural entre medições."}
	if _, err := a.presentMonPath(); err != nil {
		v.Why = err.Error()
	} else {
		v.Ready = true
	}
	v.Runs = perf.List(a.perfDir())
	seen := map[string]bool{}
	for _, r := range v.Runs {
		if r.Label != "" && !seen[r.Label] {
			seen[r.Label] = true
			v.Labels = append(v.Labels, r.Label)
		}
	}
	v.Processes = a.perfProcesses()
	for _, r := range perf.Remarks(&perf.Run{Conditions: a.conditionsNow()}) {
		if r.Level == "warn" {
			v.Preflight = append(v.Preflight, r)
		}
	}
	return v
}

// perfProcesses suggests the games of the library (CS2 first) and marks the
// ones running now.
func (a *App) perfProcesses() []PerfProcess {
	out := a.perfCandidates()
	running := map[string]bool{}
	if list, err := a.P.Native.Processes(); err == nil {
		for _, p := range list {
			running[strings.ToLower(p.Name)] = true
		}
	}
	for i := range out {
		out[i].Running = running[strings.ToLower(out[i].Exe)]
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Running && !out[j].Running })
	return out
}

// perfCandidates lists the games of the library (CS2 first) without looking
// at the processes: Running is left false. The match report uses it, so the
// page never queries the process of a game that is being played.
func (a *App) perfCandidates() []PerfProcess {
	gl, _ := a.Games()
	seen := map[string]bool{}
	var out []PerfProcess
	add := func(exe, name string) {
		k := strings.ToLower(exe)
		if exe == "" || seen[k] || !validExe(exe) {
			return
		}
		seen[k] = true
		out = append(out, PerfProcess{Exe: exe, Name: name})
	}
	add("cs2.exe", "Counter-Strike 2")
	for _, g := range gl {
		add(g.ExeName, g.Name)
	}
	return out
}

// PerfCaptureRequest starts a measurement.
type PerfCaptureRequest struct {
	Process string `json:"process"`
	Seconds int    `json:"seconds"`
	Mode    string `json:"mode"` // hotkey | delay
	Hotkey  string `json:"hotkey"`
	Delay   int    `json:"delay"`
	Label   string `json:"label"`
	// trial tags the run as part of a "Prova no seu PC"; set by the trial,
	// never from the HTTP body (unexported, ignored by encoding/json).
	trial *perf.TrialTag
}

func (req *PerfCaptureRequest) normalize() error {
	req.Process = strings.TrimSpace(req.Process)
	if !strings.HasSuffix(strings.ToLower(req.Process), ".exe") {
		req.Process += ".exe"
	}
	if !validExe(req.Process) {
		return fmt.Errorf("%w: nome de processo inválido (ex.: cs2.exe)", errBadRequest)
	}
	if req.Seconds == 0 {
		req.Seconds = 60
	}
	if req.Seconds < 10 || req.Seconds > 300 {
		return fmt.Errorf("%w: a duração deve ficar entre 10 e 300 segundos", errBadRequest)
	}
	switch req.Mode {
	case "", "hotkey":
		req.Mode = "hotkey"
		if req.Hotkey == "" {
			req.Hotkey = perfHotkeys[0]
		}
		ok := false
		for _, h := range perfHotkeys {
			ok = ok || h == req.Hotkey
		}
		if !ok {
			return fmt.Errorf("%w: tecla de atalho não suportada", errBadRequest)
		}
		req.Delay = 0
	case "delay":
		req.Hotkey = ""
		if req.Delay == 0 {
			req.Delay = 10
		}
		if req.Delay < 3 || req.Delay > 60 {
			return fmt.Errorf("%w: a contagem regressiva deve ficar entre 3 e 60 segundos", errBadRequest)
		}
	default:
		return fmt.Errorf("%w: modo de início inválido", errBadRequest)
	}
	req.Label = perf.CleanLabel(req.Label)
	return nil
}

func hotkeyText(h string) string {
	switch h {
	case "SCROLLLOCK":
		return "Scroll Lock"
	case "PAUSE":
		return "Pause"
	}
	parts := strings.Split(h, "+")
	for i, p := range parts {
		switch p {
		case "ALT":
			parts[i] = "Alt"
		case "SHIFT":
			parts[i] = "Shift"
		case "CTRL":
			parts[i] = "Ctrl"
		}
	}
	return strings.Join(parts, "+")
}

// cleanupPresentMon stops a PresentMon left running by an Axion closed in
// the middle of a measurement (in hotkey mode it would wait forever).
func (a *App) cleanupPresentMon() {
	if a.P.IsMock {
		return
	}
	log.Printf("início: procurando PresentMon de medições anteriores")
	defer log.Printf("início: PresentMon conferido")
	list, err := a.P.Native.Processes()
	if err != nil {
		return
	}
	bin := strings.ToLower(filepath.Join(a.P.DataDir, "bin") + string(filepath.Separator))
	killed := false
	for _, p := range list {
		if strings.EqualFold(p.Name, presentMonName) && strings.HasPrefix(strings.ToLower(p.Path), bin) {
			if err := a.P.Native.KillProcess(p.PID, p.Path); err == nil {
				killed = true
				log.Printf("PresentMon de uma medição anterior (PID %d) encerrado", p.PID)
			}
		}
	}
	if killed {
		if pm, err := a.presentMonPath(); err == nil {
			a.stopPMSessionNamed(pm, perfSession, 20*time.Second)
			a.stopPMSessionNamed(pm, sessionPMName, 20*time.Second)
		}
	}
}

// stopPresentMonSession ends the ETW session a killed PresentMon leaves open.
func (a *App) stopPresentMonSession(pm string) {
	a.stopPMSessionNamed(pm, perfSession, 20*time.Second)
}

// stopPMSessionNamed ends the ETW session called name (the manual measurement
// and the match report each have their own).
func (a *App) stopPMSessionNamed(pm, name string, timeout time.Duration) {
	res, err := a.P.Run.Run(context.Background(), pm, []string{"--terminate_existing_session", "--session_name", name}, sys.RunOpts{Timeout: timeout})
	if err != nil || res.ExitCode != 0 {
		log.Printf("PresentMon: encerrar sessão ETW %s: %v %s", name, err, strings.TrimSpace(res.Stdout+res.Stderr))
	}
}

// claimPM makes owner the one that runs PresentMon now ("perf" a manual
// measurement or the full evaluation, "sessao" a window of the match report).
// It fails when the other owner holds it: cleanupPresentMon ends any
// PresentMon of the data folder, so the two never run together.
func (a *App) claimPM(owner string) bool {
	a.pmMu.Lock()
	defer a.pmMu.Unlock()
	if a.pmOwner != "" && a.pmOwner != owner {
		return false
	}
	a.pmOwner = owner
	return true
}

func (a *App) releasePM(owner string) {
	a.pmMu.Lock()
	if a.pmOwner == owner {
		a.pmOwner = ""
	}
	a.pmMu.Unlock()
}

func (a *App) pmUser() string {
	a.pmMu.Lock()
	defer a.pmMu.Unlock()
	return a.pmOwner
}

// StartPerfCapture records frames of a game with PresentMon and saves the
// analysis as a run.
func (a *App) StartPerfCapture(req PerfCaptureRequest) (Job, error) {
	body, err := a.perfCaptureJob(&req)
	if err != nil {
		return Job{}, err
	}
	return a.Jobs.Start("perf", "Medindo "+req.Process, true, body)
}

// perfCaptureJob checks the request and returns the body of the capture,
// so it runs alone (StartPerfCapture) or as a stage of the full evaluation.
func (a *App) perfCaptureJob(req *PerfCaptureRequest) (func(ctx context.Context, c *JobCtl) (any, error), error) {
	if err := a.requireAdmin("medir o desempenho"); err != nil {
		return nil, err
	}
	if err := req.normalize(); err != nil {
		return nil, err
	}
	pm, err := a.presentMonPath()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errBadRequest, err)
	}
	dir := a.perfDir()
	if err := a.P.Native.ProtectDir(dir); err != nil {
		return nil, fmt.Errorf("pasta das medições: %w", err)
	}
	return func(ctx context.Context, c *JobCtl) (any, error) {
		// A manual measurement has priority over the match report: the
		// window being recorded is cut (what it recorded is kept) and the
		// report waits for the measurement to end.
		if !a.claimPerfPM(ctx) {
			return nil, errors.New("o relatório automático da partida não liberou o PresentMon a tempo; meça de novo em instantes")
		}
		defer a.releasePM("perf")
		started := time.Now()
		id := perf.NewID(started)
		base := filepath.Join(dir, id+".csv")
		args := []string{"--process_name", req.Process, "--output_file", base, "--timed", strconv.Itoa(req.Seconds),
			"--terminate_after_timed", "--terminate_on_proc_exit", "--no_console_stats",
			"--stop_existing_session", "--session_name", perfSession, "--track_pc_latency"}
		timeout := time.Duration(req.Delay+req.Seconds+90) * time.Second
		key := hotkeyText(req.Hotkey)
		if req.Mode == "hotkey" {
			args = append(args, "--hotkey", req.Hotkey)
			timeout = 15*time.Minute + time.Duration(req.Seconds)*time.Second
			c.Progress(0.02, fmt.Sprintf("Abra o jogo e aperte %s quando o teste começar. A gravação para sozinha depois de %d s, ou aperte %s de novo para parar antes.", key, req.Seconds, key))
		} else {
			args = append(args, "--delay", strconv.Itoa(req.Delay))
			c.Progress(0.02, fmt.Sprintf("Volte para o jogo: a gravação começa em %d s e dura %d s.", req.Delay, req.Seconds))
		}
		c.Logf("PresentMon %s %s", presentMonVersion, strings.Join(args, " "))
		a.cleanupPresentMon()
		csvPath := base
		if req.Mode == "hotkey" {
			csvPath = strings.TrimSuffix(base, ".csv") + "-1.csv"
		}

		// PresentMon keeps running after a second press of the hotkey
		// (it stops recording and waits for another press; its --timed
		// limit only ends it when the time runs out while recording). The
		// watcher below ends PresentMon when the recording is over: the
		// file stopped growing (early stop) or the time passed.
		runCtx, stopRun := context.WithCancel(ctx)
		defer stopRun()
		// The state of the PC and the sensors (temperature, throttling,
		// processor frequency) while the game is recorded.
		sensors := a.startSensors(ctx, req.Process)
		defer sensors.cancel()
		var stoppedEarly atomic.Bool
		recStart := make(chan time.Time, 1)
		done := make(chan struct{})
		var (
			linkMu    sync.Mutex
			link      *perf.GPULink
			linkTried atomic.Bool
			linkWG    sync.WaitGroup
		)
		go func() {
			var t0, lastGrow time.Time
			var lastSize int64 = -1
			tick := time.NewTicker(500 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-done:
					return
				case t := <-recStart:
					if t0.IsZero() {
						t0 = t
						sensors.Recording(t0)
					}
				case <-tick.C:
					// PresentMon's console output is buffered when redirected;
					// the CSV appears with the first recorded frame.
					st, err := os.Stat(csvPath)
					if t0.IsZero() {
						if err == nil {
							t0 = time.Now()
							sensors.Recording(t0)
						} else {
							continue
						}
					}
					now := time.Now()
					if err == nil && st.Size() != lastSize {
						lastSize, lastGrow = st.Size(), now
					}
					el := now.Sub(t0).Seconds()
					frac := math.Min(el/float64(req.Seconds), 1)
					idle := now.Sub(lastGrow)
					// The PCIe link of the GPU is read once while the game draws
					// (after a few seconds of recording, so the GPU is working).
					if !linkTried.Load() && el >= math.Min(8, float64(req.Seconds)*0.4) {
						linkTried.Store(true)
						linkWG.Add(1)
						go func() {
							defer linkWG.Done()
							if l := a.sampleGPULink(ctx); l != nil {
								linkMu.Lock()
								link = l
								linkMu.Unlock()
								c.Logf("link PCIe da GPU durante o jogo: x%d de x%d, geração %d de %d, %d%% de uso (%s)", l.Width, l.WidthMax, l.Gen, l.GenMax, l.UtilPct, l.PState)
							}
						}()
					}
					switch {
					case req.Mode == "hotkey" && el >= 5 && el < float64(req.Seconds)-1 && idle >= 5*time.Second:
						// No frame written for 5 s before the end: the key was
						// pressed again (or the game stopped drawing).
						stoppedEarly.Store(true)
						c.Progress(0.9, "Gravação encerrada antes do tempo: analisando o que foi gravado")
						stopRun()
						return
					case el > float64(req.Seconds)+10:
						// PresentMon should have ended by itself; end it.
						stopRun()
						return
					}
					c.Progress(0.05+0.85*frac, fmt.Sprintf("Gravando %s: %d de %d s. Continue jogando normalmente.", req.Process, int(el), req.Seconds))
				}
			}
		}()
		res, err := a.P.Run.Run(runCtx, pm, args, sys.RunOpts{Timeout: timeout, OnLine: func(line string) {
			c.Logf("PresentMon: %s", line)
			if strings.Contains(strings.ToLower(line), "started recording") {
				select {
				case recStart <- time.Now():
				default:
				}
			}
		}})
		close(done)
		cond, sens := sensors.Finish()
		// A capture too short for the reading above (or PresentMon ending
		// before the watcher saw the file): read the link now, while the game
		// is most likely still drawing.
		if !linkTried.Swap(true) && ctx.Err() == nil {
			if l := a.sampleGPULink(ctx); l != nil {
				linkMu.Lock()
				link = l
				linkMu.Unlock()
				c.Logf("link PCIe da GPU logo após a gravação: x%d de x%d, geração %d de %d, %d%% de uso (%s)", l.Width, l.WidthMax, l.Gen, l.GenMax, l.UtilPct, l.PState)
			}
		}
		cleanup := func() {
			if m, _ := filepath.Glob(filepath.Join(dir, id+"*.csv")); len(m) > 0 {
				for _, f := range m {
					_ = os.Remove(f)
				}
			}
		}
		if ctx.Err() != nil {
			// Cancelled by the user: nothing is kept.
			a.stopPresentMonSession(pm)
			cleanup()
			return nil, ctx.Err()
		}
		ended := runCtx.Err() != nil // ended by the watcher (the recording was over)
		note := ""
		switch {
		case ended:
			a.stopPresentMonSession(pm)
			if stoppedEarly.Load() {
				note = "gravação encerrada antes do tempo pedido"
			}
		case err != nil:
			a.stopPresentMonSession(pm)
			if _, serr := os.Stat(csvPath); serr != nil {
				cleanup()
				if errors.Is(err, sys.ErrTimeout) && req.Mode == "hotkey" {
					return nil, fmt.Errorf("a tecla %s não foi pressionada em 15 minutos; a medição foi cancelada", key)
				}
				return nil, fmt.Errorf("PresentMon: %w", err)
			}
			// Something was recorded before the failure: analyze it.
			note = "o PresentMon não terminou normalmente (" + err.Error() + "); resultado com o que foi gravado"
		case res.ExitCode != 0:
			cleanup()
			return nil, fmt.Errorf("o PresentMon terminou com erro (código %d): %s", res.ExitCode, lastLine(res.Stderr+"\n"+res.Stdout))
		}
		c.Progress(0.93, "Analisando os quadros")
		capt, err := readCapture(csvPath, req.Process)
		if err != nil {
			cleanup()
			if errors.Is(err, perf.ErrNoFrames) {
				why := fmt.Sprintf("nenhum quadro de %s foi gravado: o jogo estava aberto, em primeiro plano e fora de menus pausados?", req.Process)
				if req.Mode == "delay" {
					why += fmt.Sprintf(" A gravação começou %d s depois de você clicar em Iniciar.", req.Delay)
				}
				return nil, errors.New(why)
			}
			return nil, err
		}
		if csvPath != base {
			if err := os.Rename(csvPath, base); err == nil {
				csvPath = base
			}
		}
		// Later recordings of the same session (the key pressed again to
		// record more) are not part of this measurement.
		if m, _ := filepath.Glob(filepath.Join(dir, id+"-*.csv")); len(m) > 0 {
			for _, f := range m {
				_ = os.Remove(f)
			}
		}
		// A reading still in flight (nvidia-smi has a 10 s timeout) is given
		// a moment; the run is not held longer than that.
		waited := make(chan struct{})
		go func() { linkWG.Wait(); close(waited) }()
		select {
		case <-waited:
		case <-time.After(5 * time.Second):
		}
		linkMu.Lock()
		gpuLink := link
		linkMu.Unlock()
		run := &perf.Run{ID: id, Label: req.Label, Process: req.Process, Started: started, Requested: req.Seconds,
			Runtime: capt.Runtime, Swapchains: capt.Swapchains, CSV: filepath.Base(csvPath), PresentMon: presentMonVersion,
			Summary: perf.Summarize(capt), Series: perf.Series(capt, 1200), Note: note, GPULink: gpuLink,
			Conditions: cond, Sensors: sens}
		if req.trial != nil {
			tg := *req.trial
			run.Trial = &tg
		}
		for _, r := range perf.Remarks(run) {
			if r.Level == "warn" {
				c.Logf("condição da medição: %s", r.Title)
			}
		}
		if err := perf.Save(dir, run); err != nil {
			return nil, fmt.Errorf("salvar a medição: %w", err)
		}
		// The diagnostic speaks from the newest capture (GPU or CPU limit).
		a.measurementsChanged()
		if n := perf.Prune(dir, perfKeep); n > 0 {
			c.Logf("%d medição(ões) antiga(s) apagada(s)", n)
		}
		_ = a.P.Native.ProtectDir(dir)
		s := run.Summary
		if note != "" {
			c.Logf("Obs.: %s", note)
		}
		c.Logf("%d quadros em %.1f s: %.1f FPS médio, 1%% low %.1f, P99 %.2f ms", s.Frames, s.Seconds, s.AvgFPS, s.Low1FPS, s.FTP99)
		c.Progress(1, "Concluído")
		return run, nil
	}, nil
}

// sampleGPULink reads the PCIe link of the NVIDIA GPU now (nvidia-smi):
// during a capture the GPU is working, so the link shows what it trained
// to, not the width reduced to save power. Nil when there is no NVIDIA GPU,
// no nvidia-smi or the query fails.
func (a *App) sampleGPULink(ctx context.Context) *perf.GPULink {
	f := a.Facts()
	if f == nil || !f.HasGPUVendor("NVIDIA") {
		return nil
	}
	exe := collect.NvidiaSMIPath(a.P.Native.Paths())
	if a.P.IsMock {
		exe = "nvidia-smi.exe"
	}
	if exe == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := a.P.Run.Run(ctx, exe, []string{"--query-gpu=pcie.link.gen.current,pcie.link.gen.max,pcie.link.width.current,pcie.link.width.max,utilization.gpu,pstate", "--format=csv,noheader,nounits"}, sys.RunOpts{Timeout: 10 * time.Second})
	if err != nil || res.ExitCode != 0 {
		return nil
	}
	parts := strings.Split(strings.TrimSpace(strings.Split(strings.ReplaceAll(res.Stdout, "\r", ""), "\n")[0]), ",")
	if len(parts) < 4 {
		return nil
	}
	num := func(i int) int {
		if i < len(parts) {
			n, _ := strconv.Atoi(strings.TrimSpace(parts[i]))
			return n
		}
		return 0
	}
	l := &perf.GPULink{At: time.Now(), Gen: num(0), GenMax: num(1), Width: num(2), WidthMax: num(3), UtilPct: -1}
	if len(parts) > 4 {
		if u, err := strconv.Atoi(strings.TrimSpace(parts[4])); err == nil {
			l.UtilPct = u
		}
	}
	if len(parts) > 5 {
		if ps := strings.ToUpper(strings.TrimSpace(parts[5])); strings.HasPrefix(ps, "P") {
			l.PState = ps
		}
	}
	if l.Width <= 0 || l.WidthMax <= 0 {
		return nil
	}
	return l
}

func readCapture(path, process string) (*perf.Capture, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, perf.ErrNoFrames // PresentMon creates the file at the first frame
		}
		return nil, err
	}
	defer f.Close()
	return perf.ParseCSV(io.LimitReader(f, 512<<20), process)
}

func lastLine(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return "sem mensagem"
}

// PerfRun returns one run with its chart series.
func (a *App) PerfRun(id string) (*perf.Run, error) {
	if !perf.ValidID(id) {
		return nil, fmt.Errorf("%w: medição inválida", errBadRequest)
	}
	r, err := perf.Load(a.perfDir(), id)
	if err != nil {
		return nil, fmt.Errorf("%w: medição não encontrada", errBadRequest)
	}
	return r, nil
}

// PerfIDRequest names a run.
type PerfIDRequest struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// DeletePerfRun removes a run.
func (a *App) DeletePerfRun(req PerfIDRequest) error {
	if !perf.ValidID(req.ID) {
		return fmt.Errorf("%w: medição inválida", errBadRequest)
	}
	if err := perf.Delete(a.perfDir(), req.ID); err != nil {
		return err
	}
	a.measurementsChanged()
	return nil
}

// LabelPerfRun renames the label of a run.
func (a *App) LabelPerfRun(req PerfIDRequest) (*perf.Run, error) {
	r, err := a.PerfRun(req.ID)
	if err != nil {
		return nil, err
	}
	r.Label = perf.CleanLabel(req.Label)
	if err := perf.Save(a.perfDir(), r); err != nil {
		return nil, err
	}
	a.measurementsChanged()
	r.Series = nil
	return r, nil
}

// PerfCompareRequest compares two groups of runs.
type PerfCompareRequest struct {
	A      []string `json:"a"`
	B      []string `json:"b"`
	LabelA string   `json:"labelA"`
	LabelB string   `json:"labelB"`
}

// PerfComparison adds the runs used to the comparison.
type PerfComparison struct {
	perf.Comparison
	LabelA string     `json:"labelA"`
	LabelB string     `json:"labelB"`
	RunsA  []perf.Run `json:"runsA"`
	RunsB  []perf.Run `json:"runsB"`
}

// ComparePerf compares group A (reference) with group B.
func (a *App) ComparePerf(req PerfCompareRequest) (PerfComparison, error) {
	load := func(ids []string) ([]perf.Run, []perf.Summary, error) {
		var runs []perf.Run
		var sums []perf.Summary
		seen := map[string]bool{}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			r, err := a.PerfRun(id)
			if err != nil {
				return nil, nil, err
			}
			r.Series = nil
			runs = append(runs, *r)
			sums = append(sums, r.Summary)
		}
		return runs, sums, nil
	}
	if len(req.A) == 0 || len(req.B) == 0 || len(req.A)+len(req.B) > 40 {
		return PerfComparison{}, fmt.Errorf("%w: escolha as medições dos dois grupos", errBadRequest)
	}
	for _, id := range req.A {
		for _, id2 := range req.B {
			if id == id2 {
				return PerfComparison{}, fmt.Errorf("%w: uma medição não pode estar nos dois grupos", errBadRequest)
			}
		}
	}
	ra, sa, err := load(req.A)
	if err != nil {
		return PerfComparison{}, err
	}
	rb, sb, err := load(req.B)
	if err != nil {
		return PerfComparison{}, err
	}
	la, lb := perf.CleanLabel(req.LabelA), perf.CleanLabel(req.LabelB)
	if la == "" {
		la = "A"
	}
	if lb == "" {
		lb = "B"
	}
	out := PerfComparison{Comparison: perf.Compare(la, sa, lb, sb), LabelA: la, LabelB: lb, RunsA: ra, RunsB: rb}
	procs := map[string]bool{}
	for _, r := range append(append([]perf.Run(nil), ra...), rb...) {
		procs[strings.ToLower(r.Process)] = true
	}
	if len(procs) > 1 {
		out.Warnings = append(out.Warnings, "Os grupos têm jogos diferentes: a comparação não faz sentido.")
	}
	// A difference in the conditions can explain a change in the FPS better
	// than the setting that was changed (the first run of each group speaks).
	out.Warnings = append(out.Warnings, perf.CompareConditions(ra[0].Conditions, rb[0].Conditions)...)
	for _, g := range []struct {
		name string
		runs []perf.Run
	}{{la, ra}, {lb, rb}} {
		for _, r := range g.runs {
			for _, x := range perf.Remarks(&r) {
				if x.Level == "warn" {
					out.Warnings = append(out.Warnings, "Em \""+g.name+"\" ("+r.Started.Local().Format("02/01 15:04")+"): "+x.Title+".")
					break
				}
			}
		}
	}
	return out, nil
}

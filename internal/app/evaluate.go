package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"axion/internal/diag"
	"axion/internal/profile"
)

// The full evaluation (0.7): one button runs the analysis, the network
// test, the FPS capture of the competitive game (when it is open) and the
// polling test of the mouse, and the result is read in three scores for
// the use profile: configuration (the diagnostic), capacity (what the
// hardware can do for that use) and measurement (what was measured).

// Evaluation is the three readings of the PC.
type Evaluation struct {
	At          time.Time    `json:"at"`
	Profile     profile.Info `json:"profile"`
	Config      *diag.Report `json:"config"`
	Capacity    *diag.Report `json:"capacity"`
	Measurement *diag.Report `json:"measurement"`
	// Missing lists what the profile measures and was not measured yet.
	Missing []string `json:"missing,omitempty"`
	// LastRun summarises the last full evaluation of this session.
	LastRun *EvalResult `json:"lastRun,omitempty"`
	// CanPerf: the competitive game is running now, so the full evaluation
	// can capture its FPS; CS2Found: it is installed.
	CanPerf  bool   `json:"canPerf"`
	CS2Found bool   `json:"cs2Found"`
	PerfWhy  string `json:"perfWhy,omitempty"` // why the capture is not available
	NetWhy   string `json:"netWhy,omitempty"`  // why the network test is not available
	Note     string `json:"note"`
}

// EvalRequest says which stages the full evaluation runs (the analysis
// always runs).
type EvalRequest struct {
	Net         bool `json:"net"`
	Perf        bool `json:"perf"`
	PerfSeconds int  `json:"perfSeconds"`
	Periph      bool `json:"periph"`
}

// EvalStep is one stage of the full evaluation.
type EvalStep struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"` // done | skipped | failed | cancelled
	Note   string `json:"note,omitempty"`
}

// EvalResult is the result of the full evaluation job.
type EvalResult struct {
	At      time.Time   `json:"at"`
	Seconds float64     `json:"seconds"`
	Steps   []EvalStep  `json:"steps"`
	Scores  *Evaluation `json:"scores,omitempty"`
}

const evalNote = "Três notas, cada uma com a mesma régua do diagnóstico (100 menos 12 por problema importante, 5 por atenção e 1 por sugestão). Configuração é o que o Windows e os drivers estão fazendo com o seu hardware; capacidade é o que o hardware consegue para o seu uso, pelos requisitos publicados dos programas que definem esse uso; medição é o que o Axion mediu neste PC. O que não foi medido não entra na nota."

// Evaluation computes the three readings from what is known now.
func (a *App) Evaluation() (*Evaluation, error) {
	f := a.Facts()
	if f == nil {
		return nil, ErrNoScan
	}
	prof := a.profile()
	ev := &Evaluation{At: time.Now(), Profile: prof, Config: a.Report(), Note: evalNote}
	ev.Capacity = diag.Score(diag.Capacity(f, prof.ID))
	fs, missing := diag.Measurement(f, prof.ID, a.measured(), a.Measurements())
	ev.Measurement = diag.Score(fs)
	ev.Missing = missing
	if len(fs) == 0 {
		ev.Measurement.Unscore("Nada foi medido neste PC ainda. A avaliação completa (ou o teste de rede, a medição de FPS e a taxa do mouse, nas páginas de cada um) preenche esta nota.")
	}
	ev.Capacity.Profile, ev.Measurement.Profile = prof.ID, prof.ID
	a.mu.RLock()
	ev.LastRun = a.lastEval
	a.mu.RUnlock()
	if a.P.Net == nil {
		ev.NetWhy = "teste de rede indisponível nesta plataforma"
	} else if _, _, err := a.netAdapter(); err != nil {
		ev.NetWhy = err.Error()
	}
	ev.CS2Found = a.gamePath("cs2.exe") != ""
	if _, err := a.presentMonPath(); err != nil {
		ev.PerfWhy = err.Error()
	} else if !a.P.Native.IsElevated() {
		ev.PerfWhy = "a medição de FPS precisa do Axion como administrador"
	} else {
		for _, p := range a.perfProcesses() {
			if strings.EqualFold(p.Exe, "cs2.exe") && p.Running {
				ev.CanPerf = true
			}
		}
		if !ev.CanPerf {
			ev.PerfWhy = "o CS2 não está aberto: abra o jogo numa partida (ou no mapa de treino) e a avaliação mede o FPS"
		}
	}
	return ev, nil
}

// StartEvaluation runs the full evaluation in one job.
func (a *App) StartEvaluation(req EvalRequest) (Job, error) {
	if req.PerfSeconds <= 0 {
		req.PerfSeconds = 60
	}
	return a.Jobs.Start("evaluate", "Avaliação completa", true, func(ctx context.Context, c *JobCtl) (any, error) {
		start := time.Now()
		res := &EvalResult{At: start}
		// Shares of the progress bar: the analysis and the polling test are
		// short, the network test and the capture long.
		weights := map[string]float64{"scan": 1, "net": 2, "perf": 2.5, "periph": 0.6}
		total := weights["scan"]
		if req.Net {
			total += weights["net"]
		}
		if req.Perf {
			total += weights["perf"]
		}
		if req.Periph {
			total += weights["periph"]
		}
		pos := 0.0
		stage := func(id string) *JobCtl {
			w := weights[id] / total
			sub := c.span(pos, w, map[string]string{"scan": "Análise", "net": "Rede", "perf": "FPS", "periph": "Mouse"}[id])
			pos += w
			return sub
		}
		step := func(id, title, status, note string) {
			res.Steps = append(res.Steps, EvalStep{ID: id, Title: title, Status: status, Note: note})
			if note != "" {
				c.Logf("%s: %s (%s)", title, status, note)
			} else {
				c.Logf("%s: %s", title, status)
			}
		}
		fail := func(id, title string, err error) error {
			if ctx.Err() != nil {
				step(id, title, "cancelled", "")
				return ctx.Err()
			}
			step(id, title, "failed", err.Error())
			return nil
		}
		// 1. The analysis.
		if _, err := a.scan(ctx, stage("scan")); err != nil {
			if e := fail("scan", "Análise do PC", err); e != nil {
				return nil, e
			}
			if a.Facts() == nil {
				return nil, fmt.Errorf("a análise não terminou: %w", err)
			}
		} else {
			step("scan", "Análise do PC", "done", "")
		}

		// 2. The network, with load.
		if req.Net {
			sub := stage("net")
			body, err := a.netTestJob(NetTestRequest{Label: "avaliação", Load: true, DNS: true})
			if err != nil {
				step("net", "Teste de rede", "skipped", strings.TrimPrefix(err.Error(), errBadRequest.Error()+": "))
			} else if _, err := body(ctx, sub); err != nil {
				if e := fail("net", "Teste de rede", err); e != nil {
					return nil, e
				}
			} else {
				step("net", "Teste de rede", "done", "")
			}
		}

		// 3. The FPS of the competitive game, when it is open.
		if req.Perf {
			sub := stage("perf")
			running := false
			for _, p := range a.perfProcesses() {
				if strings.EqualFold(p.Exe, "cs2.exe") && p.Running {
					running = true
				}
			}
			if !running {
				step("perf", "Medição de FPS no CS2", "skipped", "o CS2 não está aberto")
			} else {
				preq := PerfCaptureRequest{Process: "cs2.exe", Seconds: req.PerfSeconds, Mode: "delay", Delay: 15, Label: "avaliação"}
				body, err := a.perfCaptureJob(&preq)
				if err != nil {
					step("perf", "Medição de FPS no CS2", "skipped", strings.TrimPrefix(err.Error(), errBadRequest.Error()+": "))
				} else if _, err := body(ctx, sub); err != nil {
					if e := fail("perf", "Medição de FPS no CS2", err); e != nil {
						return nil, e
					}
				} else {
					step("perf", "Medição de FPS no CS2", "done", "")
				}
			}
		}

		// 4. The mouse.
		if req.Periph {
			sub := stage("periph")
			if _, err := a.measureJob(MeasureRequest{Seconds: 6, USBTrace: true})(ctx, sub); err != nil {
				if e := fail("periph", "Taxa do mouse", err); e != nil {
					return nil, e
				}
			} else {
				step("periph", "Taxa do mouse", "done", "")
			}
		}

		c.Progress(0.99, "Calculando as três notas")
		ev, err := a.Evaluation()
		if err != nil {
			return nil, err
		}
		res.Seconds = time.Since(start).Seconds()
		ev.LastRun = nil
		res.Scores = ev
		a.mu.Lock()
		a.lastEval = res
		a.mu.Unlock()
		c.Logf("configuração %d, capacidade %d, medição %s", ev.Config.Overall, ev.Capacity.Overall, scoreText(ev.Measurement))
		return res, nil
	})
}

// scoreText prints a score, or "—" for a report with nothing to grade.
func scoreText(r *diag.Report) string {
	if r == nil || r.Unscored {
		return "—"
	}
	return strconv.Itoa(r.Overall)
}

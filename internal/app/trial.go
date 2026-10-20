package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"axion/internal/facts"
	"axion/internal/perf"
	"axion/internal/tweaks"
)

// "Prova no seu PC" (0.9.x): the user picks one optional tweak and a game
// that is open. Axion records N captures before, applies the tweak through
// the engine (write-ahead in the journal), records N captures after and
// compares them with the same perf.Compare as the Desempenho page. The tweak
// stays only when the result is better with significance; in any other case,
// or when the trial is cancelled or fails after the tweak was applied, it is
// undone through the journal. All limits below are the criterion of Axion.

const (
	trialBatchLabel = "Prova no seu PC"
	trialIDPrefix   = "prova-"

	trialDefaultRuns = 3
	trialMinRuns     = 2 // perf.Compare only computes with >= 2 per side
	trialMaxRuns     = 5

	trialDefaultSeconds = 60
	trialMinSeconds     = 30
	trialMaxSeconds     = 180

	// trialDriftPct: the rise of the average FPS along the "antes" captures
	// from which the trial warns about a trend.
	trialDriftPct = 2.0
	// trialMinDuration: a capture shorter than this share of the time asked
	// for is not a valid measurement of the trial.
	trialMinDuration = 0.9
)

var reTrialID = regexp.MustCompile(`^prova-[0-9a-f]{12}$`)

// trialCategories are the categories whose tweaks change what PresentMon
// measures. Hardware and advanced tweaks stay out of the first version.
var trialCategories = map[string]bool{"jogo": true, "energia": true}

const trialRule = "Critério do Axion: as medições de antes e de depois são comparadas pelo teste t de Welch (bicaudal), com os valores de p ajustados pelo método de Holm, porque várias métricas são testadas juntas. Uma métrica só conta como melhor ou pior com p ajustado abaixo de 0,05 e diferença de pelo menos 1% (e de 0,5 FPS ou 0,05 ms). O ajuste fica aplicado só se o 1% low, o FPS médio ou o tempo de quadro P99 melhorar assim e nenhuma métrica principal (FPS médio, 1% low, 0,1% low, P99 e tempo em engasgos) piorar. Em qualquer outro caso, o Axion desfaz o ajuste pelo histórico."

const trialHonest = "O resultado vale para este jogo, este cenário e este PC, no dia da prova. \"Sem diferença detectável\" não prova que o ajuste não faz nada: quer dizer que, com estas medições, a diferença ficou menor que a variação natural entre uma medição e outra. As medições de antes vêm primeiro e as de depois em seguida, então deixe o jogo rodar alguns minutos antes de começar, para que o jogo já tenha terminado de carregar."

// TrialRequest is the body of POST /api/trial/start.
type TrialRequest struct {
	TweakID string `json:"tweakId"`
	Process string `json:"process"`
	Seconds int    `json:"seconds"`
	Runs    int    `json:"runs"`
	Mode    string `json:"mode"` // hotkey | delay
	Hotkey  string `json:"hotkey"`
	Delay   int    `json:"delay"`
}

// TrialIDRequest names a trial (undo, keep).
type TrialIDRequest struct {
	Trial string `json:"trial"`
}

// TrialResult is the result of the trial job.
type TrialResult struct {
	Outcome    perf.TrialOutcome `json:"outcome"`
	Stage      string            `json:"stage"` // antes | aplicar | depois | comparar | desfazer | fim
	Applied    *tweaks.Result    `json:"applied,omitempty"`
	Reverted   *tweaks.Result    `json:"reverted,omitempty"`
	Comparison *PerfComparison   `json:"comparison,omitempty"`
}

// TrialCandidate is a tweak in the list of the Prova page.
type TrialCandidate struct {
	ID        string                `json:"id"`
	Title     string                `json:"title"`
	Summary   string                `json:"summary"`
	Category  string                `json:"category"`
	Evidence  string                `json:"evidence"`
	Risk      string                `json:"risk"`
	RiskLevel int                   `json:"riskLevel"`
	Rec       tweaks.Recommendation `json:"rec"`
	OK        bool                  `json:"ok"`
	Why       string                `json:"why,omitempty"`
}

// TrialInterrupted is a trial whose tweak stayed applied with no verdict.
type TrialInterrupted struct {
	Trial   string    `json:"trial"`
	EntryID string    `json:"entryId"`
	TweakID string    `json:"tweakId"`
	Title   string    `json:"title"`
	At      time.Time `json:"at"`
}

// TrialView is the Prova no seu PC page.
type TrialView struct {
	Ready          bool                `json:"ready"`
	Why            string              `json:"why,omitempty"`
	Elevated       bool                `json:"elevated"`
	Version        string              `json:"version"`
	Candidates     []TrialCandidate    `json:"candidates"`
	Processes      []PerfProcess       `json:"processes"`
	Hotkeys        []string            `json:"hotkeys"`
	Preflight      []perf.Remark       `json:"preflight,omitempty"`
	Interrupted    []TrialInterrupted  `json:"interrupted"`
	Recent         []perf.TrialOutcome `json:"recent"`
	DefaultRuns    int                 `json:"defaultRuns"`
	DefaultSeconds int                 `json:"defaultSeconds"`
	Rule           string              `json:"rule"`
	Honest         string              `json:"honest"`
	Sources        []tweaks.Source     `json:"sources"`
}

// trialEligible tells whether a tweak can be proved on this PC now, and if
// not, why (the text is shown to the user).
func (a *App) trialEligible(f *facts.Facts, t *tweaks.Tweak, st tweaks.State) (bool, string) {
	noMeasure := "não muda o que o PresentMon mede (FPS e tempo de quadro)"
	switch {
	case t.Category == "correcoes":
		return false, "correções não são provadas: elas desfazem algo prejudicial"
	case t.Profile != "":
		return false, "ajustes por jogo ainda não entram na prova"
	case !trialCategories[t.Category], !slices.Contains(t.Tags, "fps") && !slices.Contains(t.Tags, "latencia"):
		return false, noMeasure
	case t.Risk == tweaks.RiskAdvanced:
		return false, "ajustes de risco avançado não entram na prova"
	case t.Reboot != tweaks.RebootNone:
		return false, "pede " + t.Reboot.String() + ": a prova não consegue continuar depois disso"
	}
	if f != nil {
		for _, op := range t.Ops(f) {
			if op.Kind() == "display" {
				return false, "muda a taxa de atualização do monitor, que é uma das condições da medição"
			}
		}
	}
	switch {
	case !st.Applicable:
		if st.NAReason != "" {
			return false, st.NAReason
		}
		return false, "não se aplica a este PC"
	case st.Status == "error":
		return false, "não foi possível ler o estado atual deste ajuste"
	case st.Status == "partial":
		return false, "parte deste ajuste já está aplicada: desfaça ou aplique pelo catálogo antes de provar"
	case st.Status != "off" || st.ByAxion:
		return false, "já está ativo neste PC"
	case st.Rec.Level == tweaks.RecBlocked:
		return false, "bloqueado neste PC: " + st.Rec.Reason
	case t.Recommendable() && st.Rec.Level == tweaks.RecYes:
		return false, "já é recomendado para o seu PC, com evidência publicada: aplique pelo catálogo"
	}
	if t.Group != "" {
		for _, ent := range a.Engine.J.Active() {
			if o := a.Engine.Resolve(ent.TweakID, f); o != nil && o.ID != t.ID && o.Group == t.Group {
				return false, "desfaça antes \"" + o.Title + "\": aplicar este trocaria aquele, e o desfazer da prova não o traria de volta"
			}
		}
	}
	return true, ""
}

// processRunning tells whether the game is open. It only reads the list of
// processes (a read-only snapshot): no handle is opened on the game.
func (a *App) processRunning(exe string) bool {
	return len(a.P.Native.PIDsByName(exe)) > 0
}

// StartTrial checks the request and starts the trial job.
func (a *App) StartTrial(req TrialRequest) (Job, error) {
	f := a.Facts()
	if f == nil {
		return Job{}, ErrNoScan
	}
	if err := a.requireAdmin("provar um ajuste"); err != nil {
		return Job{}, err
	}
	t := a.Engine.Catalog.Get(req.TweakID) // static catalog only
	if t == nil {
		return Job{}, fmt.Errorf("%w: ajuste desconhecido", errBadRequest)
	}
	st := a.Engine.StateOf(context.Background(), f, t)
	if ok, why := a.trialEligible(f, t, st); !ok {
		return Job{}, fmt.Errorf("%w: %s", errBadRequest, why)
	}
	if req.Runs == 0 {
		req.Runs = trialDefaultRuns
	}
	if req.Runs < trialMinRuns || req.Runs > trialMaxRuns {
		return Job{}, fmt.Errorf("%w: escolha entre 2 e 5 medições de cada lado", errBadRequest)
	}
	if req.Seconds == 0 {
		req.Seconds = trialDefaultSeconds
	}
	if req.Seconds < trialMinSeconds || req.Seconds > trialMaxSeconds {
		return Job{}, fmt.Errorf("%w: a duração de cada medição deve ficar entre 30 e 180 segundos", errBadRequest)
	}
	if c := a.conditionsNow(); c.OnBattery {
		return Job{}, fmt.Errorf("%w: conecte o carregador: na bateria o notebook limita a CPU e a GPU, e o resultado não valeria para jogar na tomada", errBadRequest)
	}
	base := PerfCaptureRequest{Process: req.Process, Seconds: req.Seconds, Mode: req.Mode, Hotkey: req.Hotkey, Delay: req.Delay}
	if err := base.normalize(); err != nil {
		return Job{}, err
	}
	if !a.processRunning(base.Process) {
		return Job{}, fmt.Errorf("%w: abra o jogo (%s) antes de começar: as medições de antes e de depois precisam do jogo no mesmo lugar", errBadRequest, base.Process)
	}
	probe := base
	if _, err := a.perfCaptureJob(&probe); err != nil { // PresentMon e a pasta das medições
		return Job{}, err
	}
	id := trialIDPrefix + tweaks.NewID()
	return a.Jobs.Start("trial", "Prova: "+t.Title, true, func(ctx context.Context, c *JobCtl) (any, error) {
		return a.runTrial(ctx, c, f, t, base, req.Runs, id)
	})
}

// runTrial: antes 1..n -> aplicar -> depois 1..n -> comparar -> manter | desfazer.
// A failure or a cancellation after the tweak was applied undoes it through
// the journal.
func (a *App) runTrial(ctx context.Context, c *JobCtl, f *facts.Facts, t *tweaks.Tweak, base PerfCaptureRequest, n int, id string) (any, error) {
	defer func() {
		if r := recover(); r != nil {
			// Keep the write-ahead entry revertible in this session too.
			a.Engine.J.RecoverInterrupted()
			panic(r)
		}
	}()
	res := &TrialResult{Stage: "antes", Outcome: perf.TrialOutcome{ID: id, TweakID: t.ID, TweakTitle: t.Title, Process: base.Process, Runs: n, Seconds: base.Seconds, At: time.Now()}}
	total, pos := float64(2*n)+0.4, 0.0
	stage := func(w float64, prefix string) *JobCtl {
		sub := c.span(pos/total, w/total, prefix)
		pos += w
		return sub
	}
	var ra, rb []*perf.Run
	capture := func(phase string, i int) (*perf.Run, error) {
		req := base
		req.Label = phase + " (prova)"
		req.trial = &perf.TrialTag{ID: id, TweakID: t.ID, TweakTitle: t.Title, Phase: phase, Seq: i}
		body, err := a.perfCaptureJob(&req)
		if err != nil {
			return nil, err
		}
		name := "Antes"
		if phase == perf.TrialPhaseAfter {
			name = "Depois"
		}
		v, err := body(ctx, stage(1, fmt.Sprintf("%s %d de %d", name, i, n)))
		if err != nil {
			return nil, err
		}
		run, _ := v.(*perf.Run)
		if run == nil {
			return nil, errors.New("a medição não foi salva")
		}
		// A capture ended early (the key pressed again, or the game stopped
		// drawing) covers another stretch of the game: it would bias the test.
		if run.Summary.Seconds < trialMinDuration*float64(base.Seconds) {
			_ = perf.Delete(a.perfDir(), run.ID)
			a.measurementsChanged()
			return nil, fmt.Errorf("a medição durou %.0f s de %d s pedidos (a tecla foi apertada de novo ou o jogo parou de desenhar)", run.Summary.Seconds, base.Seconds)
		}
		return run, nil
	}
	ids := func() (runsA, runsB []string) {
		for _, r := range ra {
			runsA = append(runsA, r.ID)
		}
		for _, r := range rb {
			runsB = append(runsB, r.ID)
		}
		return
	}
	drop := func() {
		x, y := ids()
		a.dropTrialRuns(append(x, y...))
	}
	undo := func(why string) {
		res.Stage = "desfazer"
		ur, ok := a.trialUndo(ctx, stage(0.1, "Desfazendo"), f, res.Outcome.EntryID, why)
		res.Reverted = &ur
		res.Outcome.Action = perf.TrialAutoUndone
		if !ok {
			res.Outcome.Action = perf.TrialUndoFailed
		}
	}
	c.Logf("prova %s: %s, %d medição(ões) de %d s de cada lado em %s", id, t.Title, n, base.Seconds, base.Process)
	for i := 1; i <= n; i++ {
		r, err := capture(perf.TrialPhaseBefore, i)
		if err != nil {
			drop()
			if ctx.Err() != nil {
				return res, ctx.Err()
			}
			return res, fmt.Errorf("medição %d de antes: %v. Nada foi alterado no PC", i, err)
		}
		ra = append(ra, r)
	}
	if ctx.Err() != nil {
		drop()
		return res, ctx.Err()
	}
	res.Stage = "aplicar"
	stage(0.2, "Aplicando").Progress(0.1, "Aplicando \""+t.Title+"\" (o valor de antes fica guardado no histórico)")
	c.NoCancel("o ajuste está sendo aplicado e registrado no histórico")
	ar := a.Engine.Apply(context.WithoutCancel(ctx), f, []string{t.ID}, tweaks.ApplyOpts{Batch: id, Label: trialBatchLabel, Log: c.Logf})
	c.NoCancel("")
	a.recordBatch(batchLog(ApplyRequest{IDs: []string{t.ID}, Label: trialBatchLabel}, nil, ar))
	if len(ar) == 1 {
		x := ar[0]
		res.Applied = &x
	}
	if res.Applied == nil || res.Applied.Outcome != tweaks.OutApplied {
		drop()
		a.refreshAfterChange(ctx)
		msg := "sem resultado"
		tail := "A prova parou, e o que o Axion tentou mudar foi desfeito."
		if res.Applied != nil {
			msg = strings.TrimSpace(outcomeText(res.Applied.Outcome) + " " + res.Applied.Message)
			if e := a.Engine.J.Get(res.Applied.EntryID); e != nil && e.Pending() {
				tail = "A prova parou, mas algumas mudanças podem não ter sido desfeitas: confira o Histórico."
			}
		}
		return res, fmt.Errorf("o ajuste não pôde ser aplicado (%s). %s", msg, tail)
	}
	res.Outcome.EntryID = res.Applied.EntryID
	res.Stage = "depois"
	for i := 1; i <= n; i++ {
		r, err := capture(perf.TrialPhaseAfter, i)
		if err == nil && ctx.Err() == nil {
			rb = append(rb, r)
			continue
		}
		cancelled := ctx.Err() != nil
		if err == nil {
			rb = append(rb, r)
		}
		why := "A medição falhou: desfazendo pelo histórico"
		if cancelled {
			why = "Prova cancelada: desfazendo pelo histórico"
		}
		undo(why)
		if res.Outcome.Action == perf.TrialUndoFailed {
			// The tweak is still on: keep the captures and the record, so the
			// trial shows in "Provas anteriores" with the Desfazer button.
			o := &res.Outcome
			o.RunsA, o.RunsB = ids()
			o.Verdict, o.At = perf.TrialInconclusive, time.Now()
			if serr := a.stampTrial(*o); serr != nil {
				c.Logf("o resultado da prova não foi gravado nas medições: %v", serr)
			}
		} else {
			drop()
		}
		a.refreshAfterChange(ctx)
		if cancelled {
			return res, ctx.Err()
		}
		if res.Outcome.Action == perf.TrialUndoFailed {
			return res, fmt.Errorf("medição %d de depois: %v. O Axion não conseguiu desfazer o ajuste (%s): use Desfazer no Histórico", i, err, res.Reverted.Message)
		}
		return res, fmt.Errorf("medição %d de depois: %v. O ajuste foi desfeito pelo histórico", i, err)
	}
	res.Stage = "comparar"
	stage(0.1, "Comparando").Progress(0.5, "Comparando antes × depois")
	cmp, changed := a.trialCompare(t, f, ra, rb)
	res.Comparison = &cmp
	o := &res.Outcome
	o.Verdict = perf.TrialVerdict(cmp.Comparison, changed)
	o.Metrics = perf.TrialMetrics(cmp.Comparison)
	o.Warnings = cmp.Warnings
	o.RunsA, o.RunsB = ids()
	c.Logf("veredito: %s", o.Verdict)
	for _, m := range o.Metrics {
		c.Logf("%s: %+.2f%% (p ajustado %.4f, %s)", m.Label, m.DeltaPct, m.PAdj, m.Verdict)
	}
	if o.Verdict == perf.TrialBetter {
		o.Action = perf.TrialKept
		stage(0.1, "Concluído").Progress(1, "Melhorou com significância: o ajuste fica aplicado")
	} else {
		undo("Sem melhora comprovada: desfazendo pelo histórico")
	}
	o.At = time.Now()
	if err := a.stampTrial(*o); err != nil {
		c.Logf("o resultado da prova não foi gravado nas medições: %v", err)
	}
	c.Progress(0.98, "Atualizando o diagnóstico")
	a.refreshAfterChange(ctx)
	res.Stage = "fim"
	return res, nil
}

// trialUndo reverts the trial's journal entry; it is never cancelled midway.
func (a *App) trialUndo(ctx context.Context, sub *JobCtl, f *facts.Facts, entryID, why string) (tweaks.Result, bool) {
	ent := a.Engine.J.Get(entryID)
	if ent == nil {
		return tweaks.Result{Outcome: tweaks.OutFailed, Message: "o registro da aplicação não está no histórico"}, false
	}
	sub.NoCancel("o ajuste está sendo desfeito pelo histórico")
	defer sub.NoCancel("")
	sub.Progress(0.2, why)
	rs := a.Engine.RevertEntries(context.WithoutCancel(ctx), f, []*tweaks.Entry{ent}, tweaks.RevertOpts{Log: sub.Logf})
	if len(rs) != 1 {
		return tweaks.Result{Outcome: tweaks.OutFailed, Message: "nada foi desfeito"}, false
	}
	return rs[0], rs[0].Outcome == tweaks.OutReverted
}

// trialMaskOf tells which conditions the item itself changes (a plan changes
// the plan; that is the point of it, not a confounder).
func trialMaskOf(ops []tweaks.Op) (power, display bool) {
	for _, op := range ops {
		switch op.Kind() {
		case "powerscheme", "power-overlay":
			power = true
		case "display":
			display = true
		}
	}
	return
}

func maskConditions(c, ref *perf.Conditions, power, display bool) *perf.Conditions {
	if c == nil || ref == nil {
		return c
	}
	x := *c
	if power {
		x.PowerScheme, x.PowerSaver, x.PowerMode, x.Efficiency = ref.PowerScheme, ref.PowerSaver, ref.PowerMode, ref.Efficiency
	}
	if display {
		x.DisplayHz = ref.DisplayHz
	}
	return &x
}

// trialCompare compares the captures (A = antes, B = depois) and tells if a
// condition that alone changes the FPS differed between them.
func (a *App) trialCompare(t *tweaks.Tweak, f *facts.Facts, ra, rb []*perf.Run) (PerfComparison, bool) {
	var sa, sb []perf.Summary
	var runsA, runsB []perf.Run
	for _, r := range ra {
		sa = append(sa, r.Summary)
		x := *r
		x.Series = nil
		runsA = append(runsA, x)
	}
	for _, r := range rb {
		sb = append(sb, r.Summary)
		x := *r
		x.Series = nil
		runsB = append(runsB, x)
	}
	out := PerfComparison{Comparison: perf.Compare("antes", sa, "depois", sb), LabelA: "antes", LabelB: "depois", RunsA: runsA, RunsB: runsB}
	power, display := trialMaskOf(t.Ops(f))
	ref := ra[0].Conditions
	changed, seen := false, map[string]bool{}
	add := func(w string) {
		if !seen[w] {
			seen[w] = true
			out.Warnings = append(out.Warnings, w)
		}
	}
	for _, r := range append(append([]*perf.Run(nil), ra[1:]...), rb...) {
		cnd := maskConditions(r.Conditions, ref, power, display)
		if perf.ConditionsDiffer(ref, cnd) {
			changed = true
		}
		for _, w := range perf.CompareConditions(ref, cnd) {
			add(w)
		}
	}
	for _, g := range []struct {
		name string
		runs []*perf.Run
	}{{"antes", ra}, {"depois", rb}} {
		for i, r := range g.runs {
			for _, x := range perf.Remarks(r) {
				if x.Level == "warn" {
					add(fmt.Sprintf("Em \"%s %d\": %s.", g.name, i+1, x.Title))
					break
				}
			}
		}
	}
	if len(ra) >= 3 {
		up := true
		for i := 1; i < len(ra); i++ {
			if ra[i].Summary.AvgFPS <= ra[i-1].Summary.AvgFPS {
				up = false
			}
		}
		if first := ra[0].Summary.AvgFPS; up && first > 0 && 100*(ra[len(ra)-1].Summary.AvgFPS-first)/first > trialDriftPct {
			add("O FPS subiu a cada medição de antes; isso pode ser o jogo ainda carregando (por exemplo, shaders compilando) e favorece o depois. Deixe o jogo rodar alguns minutos e prove de novo.")
		}
	}
	return out, changed
}

// stampTrial writes the outcome into every capture of the trial (Load keeps
// the series; List would erase them).
func (a *App) stampTrial(o perf.TrialOutcome) error {
	dir := a.perfDir()
	var first error
	for _, id := range append(append([]string(nil), o.RunsA...), o.RunsB...) {
		r, err := perf.Load(dir, id)
		if err != nil || r.Trial == nil || r.Trial.ID != o.ID {
			continue
		}
		cp := o
		r.Trial.Outcome = &cp
		if err := perf.Save(dir, r); err != nil && first == nil {
			first = err
		}
	}
	_ = a.P.Native.ProtectDir(dir)
	a.measurementsChanged()
	return first
}

// dropTrialRuns removes the captures of a trial that did not finish.
func (a *App) dropTrialRuns(ids []string) {
	for _, id := range ids {
		_ = perf.Delete(a.perfDir(), id)
	}
	if len(ids) > 0 {
		a.measurementsChanged()
	}
}

// trialOutcomes lists the decided trials, newest first (cached with the
// measurements).
func (a *App) trialOutcomes() []perf.TrialOutcome {
	a.mu.RLock()
	l, ok := a.trials, a.trialsOK
	a.mu.RUnlock()
	if ok {
		return l
	}
	seen := map[string]bool{}
	var out []perf.TrialOutcome
	for _, r := range perf.List(a.perfDir()) {
		if r.Trial == nil || r.Trial.Outcome == nil || seen[r.Trial.ID] {
			continue
		}
		seen[r.Trial.ID] = true
		out = append(out, *r.Trial.Outcome)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	a.mu.Lock()
	a.trials, a.trialsOK = out, true
	a.mu.Unlock()
	return out
}

// Trial builds the Prova no seu PC page.
func (a *App) Trial() (TrialView, error) {
	f := a.Facts()
	if f == nil {
		return TrialView{}, ErrNoScan
	}
	v := TrialView{Version: presentMonVersion, Elevated: a.P.Native.IsElevated(), Hotkeys: perfHotkeys, DefaultRuns: trialDefaultRuns, DefaultSeconds: trialDefaultSeconds,
		Rule: trialRule, Honest: trialHonest, Sources: []tweaks.Source{srcNISTt, srcPMReadme, srcFrameView},
		Candidates: []TrialCandidate{}, Interrupted: []TrialInterrupted{}, Recent: []perf.TrialOutcome{}}
	if _, err := a.presentMonPath(); err != nil {
		v.Why = err.Error()
	} else {
		v.Ready = true
	}
	st := a.States()
	for _, t := range a.Engine.Catalog.All() {
		if t.Category == "correcoes" || !trialCategories[t.Category] {
			continue
		}
		ok, why := a.trialEligible(f, t, st[t.ID])
		v.Candidates = append(v.Candidates, TrialCandidate{ID: t.ID, Title: t.Title, Summary: t.Summary, Category: t.Category, Evidence: t.Evidence.String(), Risk: t.Risk.String(), RiskLevel: int(t.Risk), Rec: st[t.ID].Rec, OK: ok, Why: why})
	}
	sort.SliceStable(v.Candidates, func(i, j int) bool { return v.Candidates[i].OK && !v.Candidates[j].OK })
	v.Processes = a.perfProcesses()
	for _, r := range perf.Remarks(&perf.Run{Conditions: a.conditionsNow()}) {
		if r.Level == "warn" {
			v.Preflight = append(v.Preflight, r)
		}
	}
	outs := a.trialOutcomes()
	done := map[string]bool{}
	for _, o := range outs {
		done[o.ID] = true
	}
	if len(outs) > 20 {
		outs = outs[:20]
	}
	v.Recent = append([]perf.TrialOutcome{}, outs...)
	kept := map[string]bool{}
	for _, k := range a.Settings().KeptTrials {
		kept[k] = true
	}
	if j, running := a.Jobs.Running(); !(running && j.Kind == "trial") { // a running trial is not interrupted
		for _, e := range a.Engine.J.Active() {
			if e.BatchLabel != trialBatchLabel || !reTrialID.MatchString(e.Batch) || done[e.Batch] || kept[e.Batch] {
				continue
			}
			v.Interrupted = append(v.Interrupted, TrialInterrupted{Trial: e.Batch, EntryID: e.ID, TweakID: e.TweakID, Title: e.TweakTitle, At: e.Time})
		}
	}
	return v, nil
}

// StartTrialUndo is the one-click undo of a kept trial (kind "revert", so the
// interface shows the usual result).
func (a *App) StartTrialUndo(req TrialIDRequest) (Job, error) {
	if a.Facts() == nil {
		return Job{}, ErrNoScan
	}
	if !reTrialID.MatchString(req.Trial) {
		return Job{}, fmt.Errorf("%w: prova inválida", errBadRequest)
	}
	var o *perf.TrialOutcome
	for _, x := range a.trialOutcomes() {
		if x.ID == req.Trial {
			cp := x
			o = &cp
			break
		}
	}
	if o == nil {
		return Job{}, fmt.Errorf("%w: prova não encontrada", errBadRequest)
	}
	if o.Action != perf.TrialKept && o.Action != perf.TrialUndoFailed {
		return Job{}, fmt.Errorf("%w: o ajuste desta prova já foi desfeito", errBadRequest)
	}
	ent := a.Engine.J.Get(o.EntryID)
	if ent == nil {
		return Job{}, fmt.Errorf("%w: o registro desta prova não está no histórico", errBadRequest)
	}
	return a.Jobs.Start("revert", "Desfazendo: "+o.TweakTitle, true, func(ctx context.Context, c *JobCtl) (any, error) {
		before := a.scoreNow()
		res := a.Engine.RevertEntries(ctx, a.Facts(), []*tweaks.Entry{ent}, tweaks.RevertOpts{Log: c.Logf})
		for _, r := range res {
			c.Logf("%s: %s %s", r.Title, outcomeText(r.Outcome), r.Message)
		}
		if len(res) == 1 && res[0].Outcome == tweaks.OutConflict {
			c.Logf("o valor foi mudado depois da prova; use Forçar desfazer no Histórico e depois Desfazer aqui de novo para atualizar o registro")
		}
		if len(res) == 1 && (res[0].Outcome == tweaks.OutReverted || res[0].Outcome == tweaks.OutNothing) {
			o.Action = perf.TrialUndone
			if err := a.stampTrial(*o); err != nil {
				c.Logf("o resultado da prova não foi atualizado: %v", err)
			}
		}
		c.Progress(0.9, "Atualizando o diagnóstico")
		reread := a.refreshAfterChange(ctx)
		br := summarize(res)
		br.ScoreBefore, br.ScoreAfter, br.Reread = before, a.scoreNow(), reread
		return br, nil
	})
}

// KeepTrial records that the user keeps the change of an interrupted trial.
func (a *App) KeepTrial(req TrialIDRequest) error {
	if !reTrialID.MatchString(req.Trial) {
		return fmt.Errorf("%w: prova inválida", errBadRequest)
	}
	return a.UpdateSettings(func(s *Settings) {
		if !slices.Contains(s.KeptTrials, req.Trial) {
			s.KeptTrials = append(s.KeptTrials, req.Trial)
		}
	})
}

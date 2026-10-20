package perf

import "time"

// "Prova no seu PC": um item opcional medido neste PC, N capturas antes e N
// depois de aplicá-lo, lidas pela mesma comparação da página Desempenho
// (Welch + Holm, compare.go). Critério do Axion para manter: alguma métrica
// principal (1% low, FPS médio, P99) "melhor" e nenhuma guarda "pior".
// Todos os limites deste arquivo são critério do Axion, não valores de fonte.

// Fases de uma captura da prova.
const (
	TrialPhaseBefore = "antes"
	TrialPhaseAfter  = "depois"
)

// Veredito da prova. Os três primeiros são os mesmos rótulos de Row.Verdict.
const (
	TrialBetter       = "melhor"
	TrialSame         = "igual"
	TrialWorse        = "pior"
	TrialMixed        = "misto"
	TrialInconclusive = "inconclusivo"
)

// Ação tomada com o ajuste ao fim da prova.
const (
	TrialKept       = "mantido"
	TrialAutoUndone = "desfeito-auto"
	TrialUndone     = "desfeito"
	TrialUndoFailed = "falha-desfazer"
)

// trialKeys decide o ganho; trialGuards não podem piorar. As chaves são as de
// metrics em compare.go.
var (
	trialKeys   = []string{"low1Fps", "avgFps", "ftP99"}
	trialGuards = []string{"avgFps", "low1Fps", "low01Fps", "ftP99", "stutterPct"}
)

// TrialTag marks a capture of "Prova no seu PC": which trial, which item,
// before or after, and the outcome once it is decided.
type TrialTag struct {
	ID         string        `json:"id"`
	TweakID    string        `json:"tweakId"`
	TweakTitle string        `json:"tweakTitle"`
	Phase      string        `json:"phase"`
	Seq        int           `json:"seq"`
	Outcome    *TrialOutcome `json:"outcome,omitempty"`
}

// TrialMetric is one headline metric of a trial (A = antes, B = depois).
type TrialMetric struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Unit     string  `json:"unit"`
	Higher   bool    `json:"higher"`
	A        float64 `json:"a"`
	B        float64 `json:"b"`
	DeltaPct float64 `json:"deltaPct"`
	PAdj     float64 `json:"pAdj"`
	Verdict  string  `json:"verdict"`
}

// TrialOutcome is the decided result of a trial; it is stored in every
// capture of the trial (no separate database).
type TrialOutcome struct {
	ID         string        `json:"id"`
	TweakID    string        `json:"tweakId"`
	TweakTitle string        `json:"tweakTitle"`
	Process    string        `json:"process"`
	At         time.Time     `json:"at"`
	Runs       int           `json:"runs"`
	Seconds    int           `json:"seconds"`
	Verdict    string        `json:"verdict"`
	Action     string        `json:"action"`
	EntryID    string        `json:"entryId,omitempty"`
	Metrics    []TrialMetric `json:"metrics"`
	Warnings   []string      `json:"warnings"`
	RunsA      []string      `json:"runsA,omitempty"`
	RunsB      []string      `json:"runsB,omitempty"`
}

// TrialMetrics extracts the headline rows of a comparison, in the order of
// trialKeys, keeping only the ones present.
func TrialMetrics(c Comparison) []TrialMetric {
	by := map[string]Row{}
	for _, r := range c.Rows {
		by[r.Key] = r
	}
	var out []TrialMetric
	for _, k := range trialKeys {
		r, ok := by[k]
		if !ok {
			continue
		}
		out = append(out, TrialMetric{Key: r.Key, Label: r.Label, Unit: r.Unit, Higher: r.Higher, A: r.A.Mean, B: r.B.Mean, DeltaPct: r.DeltaPct, PAdj: r.PAdj, Verdict: r.Verdict})
	}
	return out
}

// TrialVerdict reads a comparison (A = antes, B = depois) for the trial.
// condChanged: the conditions changed between the captures (see
// ConditionsDiffer); a result then says nothing about the item, whatever it
// is, so it is never "melhor", "pior" nor "igual".
func TrialVerdict(c Comparison, condChanged bool) string {
	if !c.Enough {
		return TrialInconclusive
	}
	by := map[string]Row{}
	for _, r := range c.Rows {
		by[r.Key] = r
	}
	worse := false
	for _, k := range trialGuards {
		if r, ok := by[k]; ok && r.Verdict == "pior" {
			worse = true
		}
	}
	better, same, seen := false, 0, 0
	for _, k := range trialKeys {
		r, ok := by[k]
		if !ok {
			continue
		}
		seen++
		switch r.Verdict {
		case "melhor":
			better = true
		case "igual":
			same++
		}
	}
	switch {
	case seen == 0:
		return TrialInconclusive
	case condChanged:
		return TrialInconclusive
	case better && worse:
		return TrialMixed
	case worse:
		return TrialWorse
	case better:
		return TrialBetter
	case same == seen:
		return TrialSame
	}
	return TrialInconclusive
}

// ConditionsDiffer reports a difference that alone changes the FPS
// (battery, an economy plan or mode, the plan, the monitor's refresh rate).
// Busy programs vary from one capture to the next: CompareConditions warns
// about them, they do not void the comparison.
func ConditionsDiffer(a, b *Conditions) bool {
	if a == nil || b == nil {
		return false
	}
	return a.OnBattery != b.OnBattery ||
		(a.PowerSaver || a.Efficiency) != (b.PowerSaver || b.Efficiency) ||
		(a.PowerScheme != "" && b.PowerScheme != "" && a.PowerScheme != b.PowerScheme) ||
		(a.PowerMode != "" && b.PowerMode != "" && a.PowerMode != b.PowerMode) ||
		(a.DisplayHz > 0 && b.DisplayHz > 0 && a.DisplayHz != b.DisplayHz)
}

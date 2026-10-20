// Package dpc turns a capture of the kernel's deferred procedure calls (DPC)
// and interrupt service routines (ISR) into something a person can act on:
// which driver spends the most time there, how long its slowest routines run
// and whether that is a problem.
//
// Two limits come from Microsoft ("Example 15: Measuring DPC/ISR Time"): a DPC
// should not run longer than 100 microseconds and an ISR not longer than 25.
// They are advice to driver authors, and healthy PCs go over them all the time,
// so the counts are shown but do not decide the verdict. What does is Axion's
// own criterion, not a published threshold: a routine over 1 ms (a good part of
// a frame) is worth a look and one over 4 ms is serious, as is a driver that
// takes 5% of the machine. No public measurement ties a limit to game stutter.
package dpc

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"axion/internal/sys"
)

// Limits in microseconds.
const (
	DPCLimitUS = 100  // Microsoft's recommendation for a DPC
	ISRLimitUS = 25   // Microsoft's recommendation for an ISR
	SlowUS     = 1000 // Axion: a routine this long can be felt in a frame
	SevereUS   = 4000 // Axion: it holds a processor for most of a 240 Hz frame
)

// Verdicts.
const (
	VerdictOK        = "ok"
	VerdictAttention = "atencao"
	VerdictImportant = "importante"
	VerdictUnknown   = "inconclusivo"
)

// Index of the first duration bucket above each limit (the buckets close at
// sys.DPCHistBoundsUS: ..., 25, 50, 100, 250, ..., 1000, 2500, ...).
var (
	overDPCFrom  = bucketAbove(DPCLimitUS)
	overISRFrom  = bucketAbove(ISRLimitUS)
	overSlowFrom = bucketAbove(SlowUS)
	overSevFrom  = bucketAbove(SevereUS)
)

// bucketAbove returns the index of the first bucket whose range lies entirely
// above us.
func bucketAbove(us float64) int {
	for i, edge := range sys.DPCHistBoundsUS {
		if edge > us {
			return i
		}
	}
	return len(sys.DPCHistBoundsUS)
}

// KindStats is the cost of one kind of routine (DPC or ISR).
type KindStats struct {
	Count   uint64  `json:"count"`
	TotalUS float64 `json:"totalUs"`
	MaxUS   float64 `json:"maxUs"`
	P99US   float64 `json:"p99Us"`  // upper edge of the bucket holding the 99th percentile
	P999US  float64 `json:"p999Us"` // same for the 99.9th
	Over    uint64  `json:"over"`   // routines above Microsoft's limit for this kind
	Slow    uint64  `json:"slow"`   // routines longer than 1 ms
	Severe  uint64  `json:"severe"` // routines longer than 4 ms
}

// Driver is one kernel module's share.
type Driver struct {
	Module   string    `json:"module"` // "" = the address matched no loaded module
	DPC      KindStats `json:"dpc"`
	ISR      KindStats `json:"isr"`
	SharePct float64   `json:"sharePct"` // % of the machine's processor time spent in its DPCs and ISRs
	Verdict  string    `json:"verdict"`
	Why      string    `json:"why,omitempty"`
}

// Summary is what a capture means.
type Summary struct {
	Seconds     float64   `json:"seconds"`
	CPUs        int       `json:"cpus"`
	Events      uint64    `json:"events"`
	Discarded   uint64    `json:"discarded"`
	LostEvents  uint32    `json:"lostEvents"`
	LostBuffers uint32    `json:"lostBuffers"`
	NoModules   bool      `json:"noModules,omitempty"`
	Cancelled   bool      `json:"cancelled,omitempty"`
	DPC         KindStats `json:"dpc"`
	ISR         KindStats `json:"isr"`
	TotalPct    float64   `json:"totalPct"` // % of the machine's processor time in DPCs and ISRs
	Drivers     []Driver  `json:"drivers"`  // by time spent, at most MaxDrivers
	Reliable    bool      `json:"reliable"`
	Verdict     string    `json:"verdict"`
	Headline    string    `json:"headline"`
	Notes       []string  `json:"notes,omitempty"`
}

// MaxDrivers is how many drivers a summary lists.
const MaxDrivers = 15

// minEvents is the least a capture must read to say anything: a few hundred
// DPCs and ISRs happen in a fraction of a second on any running PC.
const minEvents = 500

func kindStats(hist []uint64, total uint64, totalUS, maxUS float64, limitFrom int) KindStats {
	k := KindStats{Count: total, TotalUS: totalUS, MaxUS: maxUS}
	for i, n := range hist {
		if i >= limitFrom {
			k.Over += n
		}
		if i >= overSlowFrom {
			k.Slow += n
		}
		if i >= overSevFrom {
			k.Severe += n
		}
	}
	k.P99US = percentile(hist, total, maxUS, 0.99)
	k.P999US = percentile(hist, total, maxUS, 0.999)
	return k
}

// percentile returns the upper edge of the bucket that holds the quantile q
// (an upper bound: the real value is between that edge and the one before).
// The open last bucket answers with the largest value seen.
func percentile(hist []uint64, total uint64, maxUS, q float64) float64 {
	if total == 0 {
		return 0
	}
	need := uint64(math.Ceil(q * float64(total)))
	var cum uint64
	for i, n := range hist {
		cum += n
		if cum >= need {
			if i < len(sys.DPCHistBoundsUS) {
				return math.Min(sys.DPCHistBoundsUS[i], math.Max(maxUS, 0))
			}
			return maxUS
		}
	}
	return maxUS
}

// merge adds two histograms.
func merge(a, b []uint64) []uint64 {
	if a == nil {
		a = make([]uint64, len(sys.DPCHistBoundsUS)+1)
	}
	for i := range a {
		if i < len(b) {
			a[i] += b[i]
		}
	}
	return a
}

// Summarize interprets a capture.
func Summarize(c sys.DPCCapture) Summary {
	s := Summary{Seconds: c.Duration.Seconds(), CPUs: c.CPUs, Events: c.Events, Discarded: c.Discarded,
		LostEvents: c.LostEvents, LostBuffers: c.LostBuffers, NoModules: c.NoModules, Cancelled: c.Cancelled}
	if s.CPUs < 1 {
		s.CPUs = 1
	}
	want := len(sys.DPCHistBoundsUS) + 1

	type acc struct {
		hist    [2][]uint64
		count   [2]uint64
		totalUS [2]float64
		maxUS   [2]float64
	}
	by := map[string]*acc{}
	var allHist [2][]uint64
	var allCount [2]uint64
	var allTotal, allMax [2]float64
	for _, st := range c.Stats {
		if len(st.Hist) != want {
			continue // not a histogram this code wrote: ignore rather than guess
		}
		k := 0
		if st.Kind == sys.KindISR {
			k = 1
		}
		a := by[st.Module]
		if a == nil {
			a = &acc{}
			by[st.Module] = a
		}
		a.hist[k] = merge(a.hist[k], st.Hist)
		a.count[k] += st.Count
		a.totalUS[k] += st.TotalUS
		a.maxUS[k] = math.Max(a.maxUS[k], st.MaxUS)
		allHist[k] = merge(allHist[k], st.Hist)
		allCount[k] += st.Count
		allTotal[k] += st.TotalUS
		allMax[k] = math.Max(allMax[k], st.MaxUS)
	}
	s.DPC = kindStats(allHist[0], allCount[0], allTotal[0], allMax[0], overDPCFrom)
	s.ISR = kindStats(allHist[1], allCount[1], allTotal[1], allMax[1], overISRFrom)
	span := s.Seconds * 1e6 * float64(s.CPUs)
	if span > 0 {
		s.TotalPct = (allTotal[0] + allTotal[1]) / span * 100
	}

	for mod, a := range by {
		d := Driver{Module: mod,
			DPC: kindStats(a.hist[0], a.count[0], a.totalUS[0], a.maxUS[0], overDPCFrom),
			ISR: kindStats(a.hist[1], a.count[1], a.totalUS[1], a.maxUS[1], overISRFrom)}
		if span > 0 {
			d.SharePct = (a.totalUS[0] + a.totalUS[1]) / span * 100
		}
		d.Verdict, d.Why = judge(d)
		s.Drivers = append(s.Drivers, d)
	}
	sort.Slice(s.Drivers, func(i, j int) bool {
		if s.Drivers[i].SharePct != s.Drivers[j].SharePct {
			return s.Drivers[i].SharePct > s.Drivers[j].SharePct
		}
		return s.Drivers[i].Module < s.Drivers[j].Module
	})
	// A driver with a verdict to act on is never cut by the size limit.
	if len(s.Drivers) > MaxDrivers {
		keep := s.Drivers[:MaxDrivers]
		for _, d := range s.Drivers[MaxDrivers:] {
			if d.Verdict != VerdictOK {
				keep = append(keep, d)
			}
		}
		s.Drivers = keep
	}
	s.finish()
	return s
}

func pct(n, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}

// comma writes a number the Brazilian way (1,4 and not 1.4).
func comma(s string) string { return strings.Replace(s, ".", ",", 1) }

func fmtUS(us float64) string {
	switch {
	case us >= 1000:
		return comma(fmt.Sprintf("%.1f", us/1000)) + " ms"
	case us >= 10:
		return fmt.Sprintf("%.0f µs", us)
	default:
		return comma(fmt.Sprintf("%.1f", us)) + " µs"
	}
}

func commaPct(v float64) string {
	return comma(fmt.Sprintf("%.1f", v))
}

// ShareAttentionPct is the share of the machine's processor time in one
// driver's DPCs and ISRs from which it is worth a look. Axion's own criterion.
const ShareAttentionPct = 5.0

// judge classifies one driver. Going over Microsoft's 100 µs (DPC) or 25 µs
// (ISR) is NOT a finding on its own: a real capture of a healthy, idle laptop
// had hundreds of DPCs over 100 µs and thousands of ISRs over 25 µs (the
// longest 608 µs), so counting them would alarm nearly every PC. They stay
// in the numbers shown; the verdict needs a routine long enough to be felt in
// a frame (over 1 ms) or a driver that takes a real slice of the processor.
func judge(d Driver) (verdict, why string) {
	worst := math.Max(d.DPC.MaxUS, d.ISR.MaxUS)
	switch {
	case d.DPC.Severe+d.ISR.Severe > 0:
		return VerdictImportant, fmt.Sprintf("tem rotinas acima de %s (a maior durou %s): segura um processador por boa parte de um quadro", fmtUS(SevereUS), fmtUS(worst))
	case d.DPC.Slow+d.ISR.Slow > 0:
		return VerdictAttention, fmt.Sprintf("tem rotinas acima de 1 ms (a maior durou %s)", fmtUS(worst))
	case d.SharePct >= ShareAttentionPct:
		return VerdictAttention, fmt.Sprintf("ocupa %s%% do tempo de todos os processadores em DPCs e interrupções", commaPct(d.SharePct))
	}
	return VerdictOK, ""
}

// finish decides whether the capture can be trusted and the overall verdict.
func (s *Summary) finish() {
	ratio := 0.0
	if s.Events > 0 {
		ratio = float64(s.Discarded) / float64(s.Events)
	}
	lostRatio := 0.0
	if t := float64(s.Events) + float64(s.LostEvents); t > 0 {
		lostRatio = float64(s.LostEvents) / t
	}
	switch {
	case s.Cancelled:
		s.Verdict = VerdictUnknown
		s.Headline = "A medição foi cancelada antes do fim."
		s.Notes = append(s.Notes, "Medições canceladas não servem para comparar.")
		return
	case s.Events == 0:
		s.Verdict = VerdictUnknown
		s.Headline = "O Windows não entregou nenhum evento de DPC ou interrupção."
		s.Notes = append(s.Notes, "Isso não significa que o PC esteja bem: a leitura falhou. Rode de novo como administrador; se persistir, envie o relatório exportado.")
		return
	case s.DPC.Count+s.ISR.Count == 0:
		// Events were read but none made it into a usable histogram.
		s.Verdict = VerdictUnknown
		s.Headline = "Os dados da medição não puderam ser interpretados."
		s.Notes = append(s.Notes, "Isso não significa que o PC esteja bem. Meça de novo; se persistir, envie o relatório exportado.")
		return
	case s.Events < minEvents:
		s.Verdict = VerdictUnknown
		s.Headline = fmt.Sprintf("Só %d eventos foram lidos: pouco para concluir.", s.Events)
		s.Notes = append(s.Notes, "Um PC em uso gera milhares de eventos por segundo. Meça por mais tempo.")
		return
	case ratio > 0.2:
		s.Verdict = VerdictUnknown
		s.Headline = "A leitura não está confiável."
		s.Notes = append(s.Notes, fmt.Sprintf("%s%% dos eventos tinham duração impossível (negativa ou acima de 1 s): o relógio dos eventos não bate com o esperado neste Windows. Os números abaixo não devem ser usados.", commaPct(ratio*100)))
		return
	}
	s.Reliable = true
	if ratio > 0 {
		s.Notes = append(s.Notes, fmt.Sprintf("%d eventos (%s%%) foram descartados por duração impossível.", s.Discarded, commaPct(ratio*100)))
	}
	if lostRatio > 0.05 {
		s.Notes = append(s.Notes, fmt.Sprintf("O Windows perdeu %s%% dos eventos (a medição gera muitos): os tempos são, no mínimo, os mostrados.", commaPct(lostRatio*100)))
	}
	if s.NoModules {
		s.Notes = append(s.Notes, "Não foi possível ler a lista de drivers carregados, então não dá para dizer qual driver executou cada rotina. Os totais e as durações continuam valendo.")
	}

	s.Verdict = VerdictOK
	var worst *Driver
	for i := range s.Drivers {
		d := &s.Drivers[i]
		if rank(d.Verdict) > rank(s.Verdict) {
			s.Verdict = d.Verdict
		}
		if d.Verdict != VerdictOK && (worst == nil || rank(d.Verdict) > rank(worst.Verdict) ||
			(rank(d.Verdict) == rank(worst.Verdict) && math.Max(d.DPC.MaxUS, d.ISR.MaxUS) > math.Max(worst.DPC.MaxUS, worst.ISR.MaxUS))) {
			worst = d
		}
	}
	// Total time in routines: a PC at rest is well under 1%.
	switch {
	case s.TotalPct >= 15 && rank(s.Verdict) < rank(VerdictImportant):
		s.Verdict = VerdictImportant
	case s.TotalPct >= 5 && rank(s.Verdict) < rank(VerdictAttention):
		s.Verdict = VerdictAttention
	}
	switch {
	case worst != nil && !s.NoModules && worst.Module != "":
		s.Headline = fmt.Sprintf("%s é o ponto de atenção: %s.", worst.Module, worst.Why)
	case worst != nil:
		s.Headline = "Há rotinas lentas: " + worst.Why + "."
	case s.TotalPct >= 5:
		s.Headline = fmt.Sprintf("Os drivers gastam %s%% do processador em DPCs e interrupções: acima do normal.", commaPct(s.TotalPct))
	default:
		s.Headline = fmt.Sprintf("Sem atrasos relevantes: %s%% do processador em DPCs e interrupções; a maior rotina durou %s.", commaPct(s.TotalPct), fmtUS(math.Max(s.DPC.MaxUS, s.ISR.MaxUS)))
	}
}

func rank(v string) int {
	switch v {
	case VerdictImportant:
		return 3
	case VerdictAttention:
		return 2
	case VerdictOK:
		return 1
	}
	return 0
}

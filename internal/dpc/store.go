package dpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"axion/internal/sys"
)

// Run is one stored capture. Only the raw capture is stored: the summary is
// made again on every read, so an improvement of the rules reaches old
// measurements too.
type Run struct {
	ID      string         `json:"id"`
	At      time.Time      `json:"at"`
	Label   string         `json:"label"`
	Seconds float64        `json:"seconds"` // length that was asked for
	Capture sys.DPCCapture `json:"capture"`
	Summary Summary        `json:"summary"` // derived; rebuilt from Capture on Load
}

// Line is one row of the history.
type Line struct {
	ID          string    `json:"id"`
	At          time.Time `json:"at"`
	Label       string    `json:"label"`
	Seconds     float64   `json:"seconds"`
	Verdict     string    `json:"verdict"`
	Reliable    bool      `json:"reliable"`
	Events      uint64    `json:"events"`
	TotalPct    float64   `json:"totalPct"`
	WorstDPCUS  float64   `json:"worstDpcUs"`
	WorstISRUS  float64   `json:"worstIsrUs"`
	WorstDriver string    `json:"worstDriver"` // the driver of the slowest routine
	OverPerSec  float64   `json:"overPerSec"`  // DPCs above 100 µs per second
	Headline    string    `json:"headline"`
}

// Line builds the history row of a run.
func (r *Run) Line() Line {
	s := r.Summary
	l := Line{ID: r.ID, At: r.At, Label: r.Label, Seconds: s.Seconds, Verdict: s.Verdict, Reliable: s.Reliable,
		Events: s.Events, TotalPct: s.TotalPct, WorstDPCUS: s.DPC.MaxUS, WorstISRUS: s.ISR.MaxUS, Headline: s.Headline}
	if s.Seconds > 0 {
		l.OverPerSec = float64(s.DPC.Over) / s.Seconds
	}
	var worst float64
	for _, d := range s.Drivers {
		if m := math.Max(d.DPC.MaxUS, d.ISR.MaxUS); m > worst {
			worst, l.WorstDriver = m, d.Module
		}
	}
	return l
}

var validID = regexp.MustCompile(`^[0-9a-f]{8,40}$`)

// MaxRuns is how many captures the history keeps.
const MaxRuns = 40

// Save writes a run and trims the history to MaxRuns.
func Save(dir string, r *Run) error {
	if !validID.MatchString(r.ID) {
		return errors.New("identificador inválido")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	r.Summary = Summarize(r.Capture)
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, r.ID+".json.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, r.ID+".json")); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	runs := List(dir)
	for i := MaxRuns; i < len(runs); i++ {
		_ = os.Remove(filepath.Join(dir, runs[i].ID+".json"))
	}
	return nil
}

// Load reads one run and rebuilds its summary.
func Load(dir, id string) (*Run, error) {
	if !validID.MatchString(id) {
		return nil, errors.New("identificador inválido")
	}
	b, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return nil, err
	}
	var r Run
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("medição %s ilegível", id)
	}
	r.Summary = Summarize(r.Capture)
	return &r, nil
}

// List returns the history, newest first.
func List(dir string) []Line {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Line
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		r, err := Load(dir, strings.TrimSuffix(name, ".json"))
		if err != nil {
			continue
		}
		out = append(out, r.Line())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

// Delete removes a run.
func Delete(dir, id string) error {
	if !validID.MatchString(id) {
		return errors.New("identificador inválido")
	}
	return os.Remove(filepath.Join(dir, id+".json"))
}

// Relabel changes the label of a run.
func Relabel(dir, id, label string) (*Run, error) {
	r, err := Load(dir, id)
	if err != nil {
		return nil, err
	}
	r.Label = CleanLabel(label)
	return r, Save(dir, r)
}

// CleanLabel trims a label to something short and printable.
func CleanLabel(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if len([]rune(s)) > 40 {
		s = string([]rune(s)[:40])
	}
	return s
}

// CompareRow is one metric of a before x after table.
type CompareRow struct {
	Label   string  `json:"label"`
	Unit    string  `json:"unit"`
	A       float64 `json:"a"`
	B       float64 `json:"b"`
	Delta   float64 `json:"delta"`
	Verdict string  `json:"verdict"` // melhor, pior, igual, sem dados
}

// Compare lists the key metrics of two captures (A = before, B = after). A
// change counts only when it exceeds a practical minimum: two captures of the
// same PC differ a little from one minute to the next. An unreliable capture
// has no data to compare.
func Compare(a, b *Run) []CompareRow {
	type metric struct {
		label, unit string
		a, b        float64
		abs, rel    float64 // smallest change that counts: absolute and relative
	}
	sa, sb := a.Summary, b.Summary
	rate := func(s Summary) float64 {
		if s.Seconds <= 0 {
			return 0
		}
		return float64(s.DPC.Over) / s.Seconds
	}
	ms := []metric{
		{"Tempo do processador em DPCs e interrupções", "%", sa.TotalPct, sb.TotalPct, 0.1, 0.2},
		{"Maior DPC", "µs", sa.DPC.MaxUS, sb.DPC.MaxUS, 50, 0.25},
		{"Maior interrupção (ISR)", "µs", sa.ISR.MaxUS, sb.ISR.MaxUS, 10, 0.25},
		{"DPCs acima de 100 µs por segundo", "/s", rate(sa), rate(sb), 0.5, 0.25},
		{"99% dos DPCs duram até", "µs", sa.DPC.P99US, sb.DPC.P99US, 10, 0.2},
	}
	// The three drivers that matter most in either capture.
	share := map[string][2]float64{}
	worst := map[string][2]float64{}
	for i, s := range []Summary{sa, sb} {
		for _, d := range s.Drivers {
			v := share[d.Module]
			v[i] = d.SharePct
			share[d.Module] = v
			w := worst[d.Module]
			w[i] = math.Max(d.DPC.MaxUS, d.ISR.MaxUS)
			worst[d.Module] = w
		}
	}
	mods := make([]string, 0, len(share))
	for m := range share {
		mods = append(mods, m)
	}
	sort.Slice(mods, func(i, j int) bool {
		si, sj := math.Max(share[mods[i]][0], share[mods[i]][1]), math.Max(share[mods[j]][0], share[mods[j]][1])
		if si != sj {
			return si > sj
		}
		return mods[i] < mods[j]
	})
	if len(mods) > 3 {
		mods = mods[:3]
	}
	for _, m := range mods {
		name := m
		if name == "" {
			name = "(driver não identificado)"
		}
		ms = append(ms,
			metric{name + " — tempo do processador", "%", share[m][0], share[m][1], 0.05, 0.25},
			metric{name + " — maior rotina", "µs", worst[m][0], worst[m][1], 25, 0.25})
	}
	ok := sa.Reliable && sb.Reliable
	var out []CompareRow
	for _, x := range ms {
		row := CompareRow{Label: x.label, Unit: x.unit, A: x.a, B: x.b}
		if !ok {
			row.Verdict = "sem dados"
			out = append(out, row)
			continue
		}
		row.Delta = math.Round((x.b-x.a)*100) / 100
		minChange := math.Max(x.abs, x.rel*math.Abs(x.a))
		switch {
		case math.Abs(row.Delta) < minChange:
			row.Verdict = "igual"
		case row.Delta < 0:
			row.Verdict = "melhor"
		default:
			row.Verdict = "pior"
		}
		out = append(out, row)
	}
	return out
}

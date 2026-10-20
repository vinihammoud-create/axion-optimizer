package perf

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Run is one saved measurement.
type Run struct {
	ID         string    `json:"id"`
	Label      string    `json:"label"`
	Process    string    `json:"process"`
	Started    time.Time `json:"started"`
	Requested  int       `json:"requested"` // seconds asked for
	Runtime    string    `json:"runtime,omitempty"`
	Swapchains int       `json:"swapchains"`
	CSV        string    `json:"csv,omitempty"` // file name next to the run
	PresentMon string    `json:"presentMon"`
	Summary    Summary   `json:"summary"`
	Series     []float32 `json:"series,omitempty"`
	Note       string    `json:"note,omitempty"` // e.g. recording ended before the time asked for
	// GPULink is the PCIe link of the GPU read while the game was drawing
	// (0.7): the reading of the analysis, with the GPU idle, may show a link
	// narrowed to save power.
	GPULink *GPULink `json:"gpuLink,omitempty"`
	// Conditions and Sensors (0.9): the state of the PC when the capture started
	// and what was read during it (temperature, throttling, processor
	// frequency). Remarks is derived from them on every Load.
	Conditions *Conditions `json:"conditions,omitempty"`
	Sensors    *Sensors    `json:"sensors,omitempty"`
	Remarks    []Remark    `json:"remarks,omitempty"`
	// Trial marks a capture of "Prova no seu PC" (0.9.x): which trial,
	// which item, before or after, and the outcome once it is decided.
	Trial *TrialTag `json:"trial,omitempty"`
	// Session marks the automatic report of a whole game session
	// (nil = manual measurement).
	Session *SessionInfo `json:"session,omitempty"`
}

// GPULink is one nvidia-smi reading of the PCIe link during a capture.
type GPULink struct {
	At       time.Time `json:"at"`
	Gen      int       `json:"gen"`      // current generation
	GenMax   int       `json:"genMax"`   // what the card supports
	Width    int       `json:"width"`    // current lanes
	WidthMax int       `json:"widthMax"` // what the card supports
	UtilPct  int       `json:"utilPct"`  // GPU load at the reading (-1 unknown)
	PState   string    `json:"pstate,omitempty"`
}

// UnderLoad tells whether the reading caught the GPU working (the link is
// not reduced to save power): load reported or the performance state P0.
func (l *GPULink) UnderLoad() bool {
	return l != nil && (l.UtilPct >= 50 || l.PState == "P0")
}

var reRunID = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[0-9a-f]{6}$`)

// ValidID tells whether id has the format NewID produces (no path tricks).
func ValidID(id string) bool { return reRunID.MatchString(id) }

// NewID returns a sortable, unique run ID: date, time, milliseconds (3 hex
// digits) and 3 random hex digits.
func NewID(t time.Time) string {
	var b [2]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s-%03x%s", t.Format("20060102-150405"), t.Nanosecond()/1e6, hex.EncodeToString(b[:])[:3])
}

// CleanLabel limits a user label to 40 printable characters.
func CleanLabel(s string) string {
	s = strings.Map(func(c rune) rune {
		if c < 32 || c == 127 {
			if c == '\t' || c == '\n' || c == '\r' {
				return ' '
			}
			return -1
		}
		return c
	}, s)
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) > 40 {
		r = r[:40]
	}
	return strings.TrimSpace(string(r))
}

// Save writes the run (atomically) into dir.
func Save(dir string, r *Run) error {
	if !ValidID(r.ID) {
		return errors.New("identificador de medição inválido")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, r.ID+".json.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, r.ID+".json"))
}

// Load reads one run.
func Load(dir, id string) (*Run, error) {
	if !ValidID(id) {
		return nil, errors.New("identificador de medição inválido")
	}
	b, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return nil, err
	}
	var r Run
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("medição %s: %w", id, err)
	}
	if r.ID != id {
		return nil, fmt.Errorf("medição %s: arquivo inconsistente", id)
	}
	r.Remarks = Remarks(&r)
	return &r, nil
}

// List returns the runs, newest first, without the chart series.
func List(dir string) []Run {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Run
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		r, err := Load(dir, strings.TrimSuffix(name, ".json"))
		if err != nil {
			continue
		}
		r.Series = nil
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Started.Equal(out[j].Started) {
			return out[i].Started.After(out[j].Started)
		}
		return out[i].ID > out[j].ID
	})
	return out
}

// Delete removes a run and its CSV.
func Delete(dir, id string) error {
	r, err := Load(dir, id)
	if err != nil {
		return err
	}
	if r.CSV != "" && filepath.Base(r.CSV) == r.CSV {
		_ = os.Remove(filepath.Join(dir, r.CSV))
	}
	return os.Remove(filepath.Join(dir, id+".json"))
}

// Prune keeps the newest keep runs.
func Prune(dir string, keep int) int {
	runs := List(dir)
	n := 0
	for i := keep; i < len(runs); i++ {
		if Delete(dir, runs[i].ID) == nil {
			n++
		}
	}
	return n
}

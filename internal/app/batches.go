package app

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"axion/internal/facts"
	"axion/internal/tweaks"
)

// SkippedItem is an item of a one-click plan the user unchecked.
type SkippedItem struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Reason string `json:"reason,omitempty"` // why it was proposed for this PC
}

// BatchLog records one apply batch: what was asked, what the user left out
// of the plan and how each item ended. The history only keeps what changed
// the PC; this log also keeps what was refused, failed or left out, so an
// exported report shows why a recommended item is still off (the first real
// report could not tell).
type BatchLog struct {
	At        time.Time         `json:"at"`
	Label     string            `json:"label,omitempty"`
	Requested []string          `json:"requested"`
	Skipped   []SkippedItem     `json:"skipped,omitempty"`
	Outcomes  map[string]string `json:"outcomes"`
	Messages  map[string]string `json:"messages,omitempty"`
}

// maxBatchLogs is how many batches the log keeps.
const maxBatchLogs = 30

type batchStore struct {
	mu     sync.Mutex
	loaded bool
	list   []BatchLog // newest first
}

func (a *App) batchesPath() string { return filepath.Join(a.P.DataDir, "lotes.json") }

func (a *App) loadBatchesLocked() {
	if a.batches.loaded {
		return
	}
	a.batches.loaded = true
	if b, err := os.ReadFile(a.batchesPath()); err == nil {
		_ = json.Unmarshal(b, &a.batches.list)
	}
}

// Batches returns the batch log, newest first.
func (a *App) Batches() []BatchLog {
	a.batches.mu.Lock()
	defer a.batches.mu.Unlock()
	a.loadBatchesLocked()
	return append([]BatchLog(nil), a.batches.list...)
}

func (a *App) recordBatch(l BatchLog) {
	a.batches.mu.Lock()
	a.loadBatchesLocked()
	a.batches.list = append([]BatchLog{l}, a.batches.list...)
	if len(a.batches.list) > maxBatchLogs {
		a.batches.list = a.batches.list[:maxBatchLogs]
	}
	a.batches.mu.Unlock()
	if err := saveFile(a.batchesPath(), func() ([]byte, error) {
		a.batches.mu.Lock()
		defer a.batches.mu.Unlock()
		return json.MarshalIndent(a.batches.list, "", " ")
	}); err != nil {
		log.Printf("registro de lotes não foi salvo: %v", err)
	}
}

// skippedItems resolves the plan items the user unchecked (unknown IDs and
// IDs also being applied are ignored).
func (a *App) skippedItems(f *facts.Facts, ids, applying []string) []SkippedItem {
	in := map[string]bool{}
	for _, id := range applying {
		in[id] = true
	}
	st := a.States()
	var out []SkippedItem
	for _, id := range ids {
		if in[id] || len(out) >= 200 {
			continue
		}
		in[id] = true
		t := a.Engine.Resolve(id, f)
		if t == nil {
			continue
		}
		it := SkippedItem{ID: id, Title: t.Title}
		if s, ok := st[id]; ok {
			it.Reason = s.Rec.Reason
		} else if f != nil {
			it.Reason = t.Recommendation(f).Reason
		}
		out = append(out, it)
	}
	return out
}

// batchLog builds the log entry of an apply batch.
func batchLog(req ApplyRequest, skipped []SkippedItem, res []tweaks.Result) BatchLog {
	l := BatchLog{At: time.Now(), Label: req.Label, Requested: append([]string(nil), req.IDs...), Skipped: skipped, Outcomes: map[string]string{}}
	for _, r := range res {
		l.Outcomes[r.TweakID] = r.Outcome
		if r.Message != "" && r.Outcome != tweaks.OutApplied && r.Outcome != tweaks.OutAlready {
			if l.Messages == nil {
				l.Messages = map[string]string{}
			}
			l.Messages[r.TweakID] = r.Message
		}
	}
	return l
}

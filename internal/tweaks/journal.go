package tweaks

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Entry statuses.
const (
	StatusApplied    = "applied"  // changes written and verified
	StatusFailed     = "failed"   // an op failed; completed ops were rolled back
	StatusReverted   = "reverted" // restored to the captured previous state
	StatusConflict   = "conflict" // revert refused: something else changed the value
	StatusPartialRev = "partial-revert"
	// StatusApplying is written before the first change (write-ahead), so an
	// apply interrupted by a crash or power loss still has its "before" values.
	StatusApplying = "applying"
	// StatusInterrupted marks an apply that never finished (found at startup).
	StatusInterrupted = "interrupted"
	// StatusSuperseded marks an entry whose remaining changes moved to a
	// newer application of the same tweak.
	StatusSuperseded = "superseded"
)

// revertible reports entries whose changes may still be on the machine:
// applied (or interrupted) entries that changed something, and failed or
// partially reverted entries with changes an undo could not restore yet
// (the undo can then be retried instead of being stranded).
func revertible(e *Entry) bool {
	switch e.Status {
	case StatusApplied, StatusInterrupted:
		return e.Changed()
	case StatusFailed, StatusPartialRev:
		return e.Pending()
	}
	return false
}

// Active reports whether the entry's changes may still be on the machine
// (it can be undone).
func (e *Entry) Active() bool { return revertible(e) }

// Pending reports whether a failed or partially reverted entry still holds
// changes that a rollback or undo could not restore.
func (e *Entry) Pending() bool {
	for i := range e.Ops {
		if onMachine(&e.Ops[i]) {
			return true
		}
	}
	return false
}

// onMachine reports whether a record's change may still be on the machine:
// something was captured before writing and no undo has confirmed the
// previous value since.
func onMachine(r *OpRecord) bool {
	return !r.Skipped && len(r.Before) > 0 && !r.RevertVerified
}

// OpRecord is the audit trail of one operation.
type OpRecord struct {
	Kind           string          `json:"kind"`
	Target         string          `json:"target"`
	Describe       string          `json:"describe"`
	Spec           json.RawMessage `json:"spec,omitempty"` // FuncOp parameters (see RegisterKind)
	Before         json.RawMessage `json:"before,omitempty"`
	After          json.RawMessage `json:"after,omitempty"`
	Skipped        bool            `json:"skipped"`  // already in target: nothing written
	Verified       bool            `json:"verified"` // read-back matched the target
	RevertVerified bool            `json:"revertVerified"`
	Error          string          `json:"error,omitempty"`
	// CarriedFrom names the older entry this record came from: when a tweak
	// is applied again, the changes an older entry made that are still on
	// the machine move to the new entry with their original "before" values.
	CarriedFrom string `json:"carriedFrom,omitempty"`
}

// Entry is one application of a tweak.
type Entry struct {
	ID         string     `json:"id"`
	Batch      string     `json:"batch,omitempty"`
	BatchLabel string     `json:"batchLabel,omitempty"`
	TweakID    string     `json:"tweakId"`
	TweakTitle string     `json:"tweakTitle"`
	Category   string     `json:"category"`
	Time       time.Time  `json:"time"`
	Status     string     `json:"status"`
	Ops        []OpRecord `json:"ops"`
	Error      string     `json:"error,omitempty"`
	RevertedAt *time.Time `json:"revertedAt,omitempty"`
	RevertNote string     `json:"revertNote,omitempty"`
	Reboot     string     `json:"reboot,omitempty"`
	WinBuild   int        `json:"winBuild,omitempty"`
	// MAC authenticates the entry with the installation's record key
	// (HMAC-SHA256 of the entry without this field). An entry without a
	// valid MAC (written by an older version, or edited outside Axion) is
	// kept and shown, but its undo only restores what the current catalog
	// itself describes.
	MAC string `json:"mac,omitempty"`

	verified bool
}

// Verified reports whether this installation of Axion wrote the entry.
func (e *Entry) Verified() bool { return e.verified }

func entryMAC(key []byte, e *Entry) string {
	c := *e
	c.MAC = ""
	b, err := json.Marshal(&c)
	if err != nil || len(key) == 0 {
		return ""
	}
	m := hmac.New(sha256.New, key)
	m.Write(b)
	return hex.EncodeToString(m.Sum(nil))
}

// Changed reports whether the entry wrote anything.
func (e *Entry) Changed() bool {
	for _, o := range e.Ops {
		if !o.Skipped {
			return true
		}
	}
	return false
}

type journalFile struct {
	Version int      `json:"version"`
	Entries []*Entry `json:"entries"`
}

// Journal persists every change Axion makes. It is the single source of truth
// for "undo": each entry keeps the exact previous state of every resource.
type Journal struct {
	mu      sync.Mutex
	path    string
	key     []byte
	entries []*Entry
}

// OpenJournal loads (or creates) the journal at dir/journal.json. key
// authenticates the entries (see Entry.MAC).
//
// journal.json is only ever replaced in one step by a complete, flushed file
// (see saveLocked). Files left by older versions, which renamed the journal
// away before putting the new one in place, can still be missing or damaged
// after a crash: the newest readable copy among journal.json, journal.json.tmp
// and journal.json.bak is used, so a crash never silently empties the history.
func OpenJournal(dir string, key []byte) (*Journal, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	j := &Journal{path: filepath.Join(dir, "journal.json"), key: append([]byte(nil), key...)}
	f, src, err := readJournal(j.path)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return j, nil
	}
	for _, e := range f.Entries {
		if e == nil {
			continue
		}
		want := entryMAC(j.key, e)
		e.verified = want != "" && hmac.Equal([]byte(e.MAC), []byte(want))
		j.entries = append(j.entries, e)
	}
	if src != j.path {
		// Recovered from a copy: put it back in place right away.
		j.mu.Lock()
		_ = j.saveLocked()
		j.mu.Unlock()
	} else if b, err := os.ReadFile(j.path); err == nil {
		// One previous generation (from the last start) for forensics and
		// manual recovery.
		_ = writeFileSync(j.path+".bak", b)
	}
	return j, nil
}

// readJournal returns the first readable journal among the file and its
// temporary/backup copies (nil when there is none), and which file it was.
// An unreadable journal.json is kept aside for forensics.
func readJournal(path string) (*journalFile, string, error) {
	var firstErr error
	for i, p := range []string{path, path + ".tmp", path + ".bak"} {
		b, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		var f journalFile
		if err := json.Unmarshal(b, &f); err != nil || f.Version == 0 {
			if i == 0 {
				_ = os.Rename(p, p+".corrupt-"+time.Now().Format("20060102-150405"))
			}
			continue
		}
		return &f, p, nil
	}
	if firstErr != nil {
		// The journal exists but cannot be read (locked, no permission):
		// never start over with an empty history in that case.
		return nil, "", firstErr
	}
	return nil, "", nil
}

// NewID returns a random 12-hex-char identifier.
func NewID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (j *Journal) saveLocked() error {
	for _, e := range j.entries {
		if e.verified {
			e.MAC = entryMAC(j.key, e) // unverified entries keep their (invalid) MAC
		}
	}
	f := journalFile{Version: 1, Entries: j.entries}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	// Write-ahead safety: the complete new journal is flushed to disk first,
	// then replaces the old one in a single rename. At any instant
	// journal.json is a complete generation (old or new), never missing.
	tmp := j.path + ".tmp"
	if err := writeFileSync(tmp, b); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return renameRetry(tmp, j.path)
}

// writeFileSync writes b to path and flushes it to the disk.
func writeFileSync(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// renameRetry replaces dst with src, retrying briefly: antivirus scanners
// and backup tools open freshly written files for a moment, and a rename
// over a file open without FILE_SHARE_DELETE fails on Windows.
func renameRetry(src, dst string) error {
	var err error
	for i := 0; i < 8; i++ {
		if err = os.Rename(src, dst); err == nil {
			return nil
		}
		time.Sleep(time.Duration(25*(i+1)) * time.Millisecond)
	}
	return err
}

// Add appends an entry and persists.
func (j *Journal) Add(e *Entry) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	e.verified = true
	j.entries = append(j.entries, e)
	return j.saveLocked()
}

// Update persists after an in-place change to an entry obtained from this journal.
func (j *Journal) Update(fn func()) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	fn()
	return j.saveLocked()
}

// Remove drops an entry that was added but could not be saved (nothing was
// changed on its behalf).
func (j *Journal) Remove(e *Entry) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for i, x := range j.entries {
		if x == e {
			j.entries = append(j.entries[:i], j.entries[i+1:]...)
			return
		}
	}
}

// View runs fn under the journal lock without saving (to read entries that
// the engine may be changing).
func (j *Journal) View(fn func()) {
	j.mu.Lock()
	defer j.mu.Unlock()
	fn()
}

// Entries returns a copy of all entries, newest first.
func (j *Journal) Entries() []*Entry {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]*Entry, len(j.entries))
	copy(out, j.entries)
	sort.SliceStable(out, func(a, b int) bool { return out[a].Time.After(out[b].Time) })
	return out
}

// Get returns an entry by ID.
func (j *Journal) Get(id string) *Entry {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, e := range j.entries {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// ActiveFor returns the newest applied (not reverted) entry that changed something for a tweak.
func (j *Journal) ActiveFor(tweakID string) *Entry {
	j.mu.Lock()
	defer j.mu.Unlock()
	var best *Entry
	for _, e := range j.entries {
		if e.TweakID == tweakID && revertible(e) {
			if best == nil || e.Time.After(best.Time) {
				best = e
			}
		}
	}
	return best
}

// Active returns all applied, not-reverted entries that changed something, newest first.
func (j *Journal) Active() []*Entry {
	all := j.Entries()
	var out []*Entry
	for _, e := range all {
		if revertible(e) {
			out = append(out, e)
		}
	}
	return out
}

// RecoverInterrupted marks entries left in "applying" by a previous run
// (crash, power loss, killed process) as interrupted; they stay revertible.
func (j *Journal) RecoverInterrupted() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	n := 0
	for _, e := range j.entries {
		if e.Status == StatusApplying {
			e.Status = StatusInterrupted
			e.Error = "a aplicação foi interrompida (programa fechado à força ou queda de energia); desfaça para voltar ao estado anterior"
			n++
		}
	}
	if n > 0 {
		_ = j.saveLocked()
	}
	return n
}

// Snapshot returns deep copies of all entries, newest first (safe to
// serialize while the engine keeps working).
func (j *Journal) Snapshot() []Entry {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]Entry, 0, len(j.entries))
	for _, e := range j.entries {
		c := *e
		c.Ops = append([]OpRecord(nil), e.Ops...)
		if e.RevertedAt != nil {
			t := *e.RevertedAt
			c.RevertedAt = &t
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Time.After(out[b].Time) })
	return out
}

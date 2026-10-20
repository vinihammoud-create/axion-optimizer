package tweaks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"axion/internal/facts"
	"axion/internal/sys"
)

// Catalog is the ordered set of tweaks.
type Catalog struct {
	list []*Tweak
	byID map[string]*Tweak
}

// NewCatalog indexes tweaks; IDs must be unique.
func NewCatalog(ts []*Tweak) (*Catalog, error) {
	c := &Catalog{byID: map[string]*Tweak{}}
	for _, t := range ts {
		if _, dup := c.byID[t.ID]; dup {
			return nil, fmt.Errorf("id duplicado no catálogo: %s", t.ID)
		}
		c.byID[t.ID] = t
		c.list = append(c.list, t)
	}
	return c, nil
}

// All returns tweaks in catalog order.
func (c *Catalog) All() []*Tweak { return c.list }

// Get returns a tweak by ID.
func (c *Catalog) Get(id string) *Tweak { return c.byID[id] }

// Provider builds a runtime tweak for an ID outside the static catalog
// (per-game profiles, DNS choices...). It returns nil for unknown IDs.
type Provider func(id string, f *facts.Facts) *Tweak

// Engine applies and reverts tweaks with full audit trail.
type Engine struct {
	P         *sys.Platform
	Catalog   *Catalog
	J         *Journal
	Providers []Provider
	mu        sync.Mutex // serializes every mutation of the machine
}

// NewEngine wires an engine.
func NewEngine(p *sys.Platform, c *Catalog, j *Journal) *Engine {
	return &Engine{P: p, Catalog: c, J: j}
}

// Resolve finds a tweak in the catalog or through the providers.
func (e *Engine) Resolve(id string, f *facts.Facts) *Tweak {
	if t := e.Catalog.Get(id); t != nil {
		return t
	}
	for _, p := range e.Providers {
		if t := p(id, f); t != nil {
			return t
		}
	}
	return nil
}

// OpState is the live state of one operation.
type OpState struct {
	Kind     string `json:"kind"`
	Describe string `json:"describe"`
	Current  string `json:"current"`
	InTarget bool   `json:"inTarget"`
	Error    string `json:"error,omitempty"`
}

// State is the live state of one tweak on this machine.
type State struct {
	ID         string         `json:"id"`
	Applicable bool           `json:"applicable"`
	NAReason   string         `json:"naReason,omitempty"`
	Status     string         `json:"status"` // on | off | partial | na | error
	Rec        Recommendation `json:"rec"`
	Ops        []OpState      `json:"ops"`
	ByAxion    bool           `json:"byAxion"` // an active journal entry exists
	AppliedAt  *time.Time     `json:"appliedAt,omitempty"`
	Drifted    bool           `json:"drifted"` // applied by Axion but no longer in target
	// Effect is set for tweaks that can check the result on the machine
	// (Tweak.Effect) when the values are in place: "ok", "reboot" (applied
	// by Axion after the last boot: restart to take effect) or "ignored"
	// (Windows is not honouring it; EffectNote says why).
	Effect     string `json:"effect,omitempty"`
	EffectNote string `json:"effectNote,omitempty"`
	// PendingUndo: an apply failed and its rollback, or an undo, could not
	// restore every value; undoing again retries what is left.
	PendingUndo bool   `json:"pendingUndo,omitempty"`
	EntryID     string `json:"entryId,omitempty"`
}

func (e *Engine) ctx(ctx context.Context, f *facts.Facts, log func(string, ...any)) *Ctx {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Ctx{Context: ctx, P: e.P, F: f, Log: log}
}

// StateOf computes the live state of a tweak (read-only).
func (e *Engine) StateOf(ctx context.Context, f *facts.Facts, t *Tweak) State {
	st := State{ID: t.ID, Rec: t.Recommendation(f)}
	ok, why := t.IsApplicable(f)
	ent := e.J.ActiveFor(t.ID)
	st.Applicable = ok
	if ent != nil {
		st.ByAxion = true
		// The entry may be updated by a running apply/undo: read it under
		// the journal lock.
		var at time.Time
		e.J.View(func() {
			at = ent.Time
			st.EntryID = ent.ID
			st.PendingUndo = ent.Status == StatusFailed || ent.Status == StatusPartialRev
		})
		st.AppliedAt = &at
	}
	if !ok {
		if ent == nil {
			st.Status, st.NAReason = "na", why
			return st
		}
		// Applied by Axion and the condition that made it applicable is gone
		// (e.g. monitor now at max Hz): report it as on, keep it revertible.
		st.Applicable, st.Status, st.NAReason = true, "on", why
		return st
	}
	c := e.ctx(ctx, f, nil)
	ops := t.Ops(f)
	in, total, errs := 0, 0, 0
	for _, op := range ops {
		os := OpState{Kind: op.Kind(), Describe: op.Describe()}
		v, err := op.InTarget(c)
		if err != nil {
			os.Error = err.Error()
			errs++
		}
		os.InTarget = v
		os.Current = op.Current(c)
		if v {
			in++
		}
		total++
		st.Ops = append(st.Ops, os)
	}
	switch {
	case total == 0 && ent != nil:
		st.Status = "on"
	case total == 0:
		st.Status = "na"
		st.NAReason = "nada a fazer neste PC"
		st.Applicable = false
	case errs == total:
		st.Status = "error"
	case in == total:
		st.Status = "on"
	case in == 0:
		st.Status = "off"
	default:
		st.Status = "partial"
	}
	if ent != nil && !st.PendingUndo {
		st.Drifted = drifted(e.J, ent, ops, st)
		// Applied by Axion, nothing undone, but every resource it covers now
		// is new (an adapter setting that changed later): pending, not off.
		if !st.Drifted && st.Status == "off" {
			st.Status = "partial"
		}
	}
	effectOf(&st, t, f)
	return st
}

// drifted reports whether a resource the active entry recorded is no longer
// in target. A resource the tweak covers only now (an adapter that appeared,
// or one an older version did not read) leaves the status partial: nothing
// Axion did was undone. Entries without records keep the old rule.
func drifted(j *Journal, ent *Entry, ops []Op, st State) bool {
	recorded := map[string]bool{}
	j.View(func() {
		for i := range ent.Ops {
			recorded[ent.Ops[i].Target] = true
		}
	})
	if len(recorded) == 0 || len(st.Ops) != len(ops) {
		return st.Status != "on"
	}
	for i, op := range ops {
		if recorded[op.Target()] && !st.Ops[i].InTarget {
			return true
		}
	}
	return false
}

// effectOf asks a tweak whose values are in place whether Windows honours
// them. A change Axion made after the last boot only takes effect after a
// restart: that is not reported as ignored.
func effectOf(st *State, t *Tweak, f *facts.Facts) {
	if t.Effect == nil || f == nil || st.Status != "on" {
		return
	}
	ok, why := t.Effect(f)
	switch {
	case ok:
		st.Effect = "ok"
	case t.Reboot == RebootNone:
		// A change that needs no restart is ignored for the reason given,
		// whenever it was applied.
		st.Effect, st.EffectNote = "ignored", why
	case st.AppliedAt != nil && (f.OS.LastBoot.IsZero() || st.AppliedAt.After(f.OS.LastBoot)):
		// Applied by Axion after the last boot (or the boot time is unknown).
		st.Effect, st.EffectNote = "reboot", "reinicie o PC para a mudança valer"
	default:
		st.Effect, st.EffectNote = "ignored", why
	}
}

// States computes the state of every tweak.
func (e *Engine) States(ctx context.Context, f *facts.Facts) []State {
	out := make([]State, 0, len(e.Catalog.list))
	for _, t := range e.Catalog.list {
		out = append(out, e.StateOf(ctx, f, t))
	}
	return out
}

// ApplyOpts tunes a batch.
type ApplyOpts struct {
	Batch        string
	Label        string // human name of the batch ("Otimização recomendada")
	AllowBlocked bool   // user explicitly overrode a RecBlocked
	Progress     func(done, total int, title string)
	Log          func(format string, args ...any)
}

// Outcomes.
const (
	OutApplied  = "applied"
	OutAlready  = "already"
	OutNA       = "na"
	OutBlocked  = "blocked"
	OutFailed   = "failed"
	OutReverted = "reverted"
	OutConflict = "conflict"
	OutNothing  = "nothing"
)

// Result of applying or reverting one tweak.
type Result struct {
	TweakID string     `json:"tweakId"`
	Title   string     `json:"title"`
	Outcome string     `json:"outcome"`
	Message string     `json:"message,omitempty"`
	EntryID string     `json:"entryId,omitempty"`
	Reboot  string     `json:"reboot,omitempty"`
	Ops     []OpRecord `json:"ops,omitempty"`
}

// ErrUnknownTweak is returned for an unknown ID.
var ErrUnknownTweak = errors.New("otimização desconhecida")

// Apply applies the given tweaks in catalog order.
func (e *Engine) Apply(ctx context.Context, f *facts.Facts, ids []string, o ApplyOpts) []Result {
	e.mu.Lock()
	defer e.mu.Unlock()
	want := map[string]bool{}
	var order []string
	for _, id := range ids {
		if !want[id] {
			want[id] = true
			order = append(order, id)
		}
	}
	var todo []*Tweak
	var results []Result
	seen := map[string]bool{}
	// Catalog order first (stable, dependency-friendly), then runtime tweaks.
	for _, t := range e.Catalog.list {
		if want[t.ID] {
			todo = append(todo, t)
			seen[t.ID] = true
		}
	}
	for _, id := range order {
		if seen[id] {
			continue
		}
		if t := e.Resolve(id, f); t != nil {
			todo = append(todo, t)
		} else {
			results = append(results, Result{TweakID: id, Outcome: OutFailed, Message: ErrUnknownTweak.Error()})
		}
	}
	// Two members of the same group cannot be applied together.
	groups := map[string]string{}
	filtered := todo[:0]
	for _, t := range todo {
		if t.Group != "" {
			if other, ok := groups[t.Group]; ok {
				results = append(results, Result{TweakID: t.ID, Title: t.Title, Outcome: OutFailed, Message: "conflita com \"" + other + "\" (escolha apenas uma opção do grupo)"})
				continue
			}
			groups[t.Group] = t.Title
		}
		filtered = append(filtered, t)
	}
	todo = filtered
	if o.Batch == "" {
		o.Batch = NewID()
	}
	for i, t := range todo {
		if ctx != nil && ctx.Err() != nil {
			results = append(results, Result{TweakID: t.ID, Title: t.Title, Outcome: OutFailed, Message: "cancelado"})
			continue
		}
		if o.Progress != nil {
			o.Progress(i, len(todo), t.Title)
		}
		results = append(results, e.applyOne(ctx, f, t, o))
	}
	if o.Progress != nil {
		o.Progress(len(todo), len(todo), "")
	}
	return results
}

func (e *Engine) applyOne(ctx context.Context, f *facts.Facts, t *Tweak, o ApplyOpts) Result {
	res := Result{TweakID: t.ID, Title: t.Title, Reboot: t.Reboot.String()}
	if ok, why := t.IsApplicable(f); !ok {
		res.Outcome, res.Message = OutNA, why
		return res
	}
	rec := t.Recommendation(f)
	if rec.Level == RecBlocked && !o.AllowBlocked {
		res.Outcome, res.Message = OutBlocked, rec.Reason
		return res
	}
	c := e.ctx(ctx, f, o.Log)
	// Mutually exclusive group: revert the currently active member first.
	if t.Group != "" {
		for _, ent := range e.J.Active() {
			if ent.TweakID == t.ID {
				continue
			}
			if other := e.Resolve(ent.TweakID, f); other != nil && other.Group == t.Group {
				r := e.revertOne(ctx, f, ent, RevertOpts{Log: o.Log})
				if r.Outcome != OutReverted {
					res.Outcome = OutFailed
					res.Message = "não foi possível desfazer \"" + other.Title + "\" antes de aplicar esta opção: " + r.Message
					return res
				}
			}
		}
	}
	ops := t.Ops(f)
	entry := &Entry{
		ID: NewID(), Batch: o.Batch, BatchLabel: o.Label, TweakID: t.ID, TweakTitle: t.Title, Category: t.Category,
		Time: time.Now(), Reboot: t.Reboot.String(), Status: StatusApplying,
	}
	if f != nil {
		entry.WinBuild = f.OS.Build
	}
	// Older entries of this tweak whose changes are still on the machine hold
	// the only copy of the values from before Axion. Their records move into
	// the new entry (see carry below), so superseding them loses nothing.
	// Only entries this installation signed are carried: a record that cannot
	// be vouched for never gets into a signed entry (it stays where it is,
	// with its limited undo).
	var olds []*Entry
	for _, old := range e.activeEntries(t.ID) {
		if old.Verified() {
			olds = append(olds, old)
		}
	}
	prevRec := func(key string) *OpRecord {
		for _, old := range olds {
			var found *OpRecord
			e.J.View(func() {
				for i := range old.Ops {
					r := &old.Ops[i]
					if onMachine(r) && recordKey(r) == key {
						cp := *r
						if cp.CarriedFrom == "" {
							cp.CarriedFrom = old.ID
						}
						found = &cp
						return
					}
				}
			})
			if found != nil {
				return found
			}
		}
		return nil
	}
	covered := map[string]bool{}  // resources the new entry accounts for
	carryAt := map[int]OpRecord{} // in-target ops still holding an older apply's change
	// Once the entry is in the journal every mutation goes through J.Update
	// (persisted and safe for concurrent readers).
	inJournal := false
	mut := func(fn func()) error {
		if inJournal {
			return e.J.Update(fn)
		}
		fn()
		return nil
	}
	type done struct {
		op  Op
		idx int // index into entry.Ops (indices stay valid across appends)
	}
	var changed []done
	var failure error
	for _, op := range ops {
		r := OpRecord{Kind: op.Kind(), Target: op.Target(), Describe: op.Describe(), Spec: specOf(op)}
		key := liveKey(c, op)
		in, err := op.InTarget(c)
		if err == nil && in {
			r.Skipped, r.Verified = true, true
			idx := 0
			_ = mut(func() { entry.Ops = append(entry.Ops, r); idx = len(entry.Ops) - 1 })
			if p := prevRec(key); p != nil {
				// Still as an older apply left it: that apply's record (with
				// the original "before") takes this place if this one succeeds.
				carryAt[idx] = *p
			}
			covered[key] = true
			continue
		}
		before, err := op.Capture(c)
		if err != nil {
			failure = fmt.Errorf("%s: não foi possível ler o estado atual: %w", op.Describe(), err)
			r.Error = failure.Error()
			_ = mut(func() { entry.Ops = append(entry.Ops, r) })
			break
		}
		if p := prevRec(key); p != nil && len(p.After) == 0 {
			// An earlier apply was interrupted (or failed) in the middle of
			// this very operation: the value now may be half written, the
			// value it captured is the real original.
			before = p.Before
			r.CarriedFrom = p.CarriedFrom
		}
		covered[key] = true
		r.Before = before
		idx := 0
		// Write-ahead: the previous value is on disk before the machine
		// changes; if it cannot be saved, nothing is changed.
		var werr error
		if !inJournal {
			entry.Ops = append(entry.Ops, r)
			idx = len(entry.Ops) - 1
			if werr = e.J.Add(entry); werr == nil {
				inJournal = true
			} else {
				e.J.Remove(entry)
			}
		} else {
			werr = e.J.Update(func() { entry.Ops = append(entry.Ops, r); idx = len(entry.Ops) - 1 })
		}
		if werr != nil {
			failure = fmt.Errorf("%s: o histórico não pôde ser salvo antes da mudança, então nada foi alterado: %w", op.Describe(), werr)
			_ = mut(func() {
				entry.Ops[idx].Before = nil // not applied: nothing to restore
				entry.Ops[idx].Error = werr.Error()
			})
			break
		}
		if err := op.Apply(c); err != nil {
			failure = fmt.Errorf("%s: %w", op.Describe(), err)
			_ = mut(func() { entry.Ops[idx].Error = err.Error() })
			// The op may have partially applied: restore it too.
			changed = append(changed, done{op, idx})
			break
		}
		after, _ := op.Capture(c)
		ok, verr := op.InTarget(c)
		verified := verr == nil && ok
		_ = mut(func() { entry.Ops[idx].After, entry.Ops[idx].Verified = after, verified })
		changed = append(changed, done{op, idx})
		if !verified {
			failure = fmt.Errorf("%s: a gravação não foi confirmada na releitura (o Windows ou uma política pode estar bloqueando)", op.Describe())
			_ = mut(func() { entry.Ops[idx].Error = "não verificado" })
			break
		}
	}
	if failure != nil {
		// Roll back everything this tweak changed, newest first.
		for i := len(changed) - 1; i >= 0; i-- {
			d := changed[i]
			var before json.RawMessage
			e.J.View(func() { before = entry.Ops[d.idx].Before })
			if len(before) == 0 {
				continue
			}
			rerr := d.op.Restore(c, before)
			if rerr != nil {
				c.logf("falha ao desfazer %s: %v", d.op.Describe(), rerr)
			}
			// Confirmed by reading back, even after an error: a write that
			// was refused may have left the original value in place.
			if now, err := captureLike(c, d.op, before); err == nil {
				ok := snapEqual(d.op, now, before)
				_ = mut(func() { entry.Ops[d.idx].RevertVerified = ok })
			}
		}
		_ = mut(func() {
			entry.Status = StatusFailed
			entry.Error = failure.Error()
			if entry.Pending() {
				entry.Error += " — algumas mudanças não puderam ser desfeitas automaticamente; use Desfazer no Histórico para tentar de novo."
			}
		})
		if !inJournal && entry.Changed() {
			_ = e.J.Add(entry) // audit trail of the failure (nothing was changed)
			inJournal = true
		}
		if inJournal {
			res.EntryID = entry.ID
		}
		res.Outcome, res.Message, res.Ops = OutFailed, entry.Error, copyOps(e.J, entry)
		return res
	}
	if !inJournal {
		// Nothing was written: every operation was already in target.
		res.Outcome, res.Ops = OutAlready, entry.Ops
		res.Message = "já estava aplicado"
		return res
	}
	// Carry the older entries' remaining changes (resources this apply did
	// not touch, e.g. a setting of the power plan that was active then) and
	// supersede them, in one save.
	serr := e.J.Update(func() {
		for idx, p := range carryAt {
			entry.Ops[idx] = p
		}
		for _, old := range olds {
			for _, r := range old.Ops {
				if !onMachine(&r) {
					continue
				}
				k := recordKey(&r)
				if covered[k] {
					continue
				}
				if r.CarriedFrom == "" {
					r.CarriedFrom = old.ID
				}
				entry.Ops = append(entry.Ops, r)
				covered[k] = true
			}
			old.Status = StatusSuperseded
			old.RevertNote = "substituída pela aplicação " + entry.ID + " (os valores anteriores passaram para ela)"
		}
		entry.Status = StatusApplied
	})
	if serr != nil {
		res.Message = "aplicado, mas o histórico não pôde ser salvo: " + serr.Error()
	}
	if t.Post != nil {
		for _, d := range changed {
			c.Touched = append(c.Touched, d.op.Target())
		}
		if err := t.Post(c, false); err != nil {
			res.Message = strings.TrimSpace(res.Message + " Aviso: " + err.Error())
		}
	}
	res.Outcome, res.EntryID, res.Ops = OutApplied, entry.ID, copyOps(e.J, entry)
	return res
}

// activeEntries returns the entries of a tweak whose changes may still be on
// the machine, newest first.
func (e *Engine) activeEntries(tweakID string) []*Entry {
	var out []*Entry
	for _, ent := range e.J.Entries() {
		if ent.TweakID != tweakID {
			continue
		}
		var act bool
		e.J.View(func() { act = revertible(ent) })
		if act {
			out = append(out, ent)
		}
	}
	return out
}

// recordKey identifies the resource a record changed. Power settings are
// written to one plan: the same setting in another plan is another
// resource, so switching plans never mixes up their original values.
func recordKey(r *OpRecord) string {
	if r.Kind == "power" {
		var s powerSnap
		if json.Unmarshal(r.Before, &s) == nil && s.Scheme != "" {
			return r.Target + "@" + strings.ToLower(s.Scheme)
		}
	}
	return r.Target
}

// liveKey is recordKey for an op about to be applied now.
func liveKey(c *Ctx, op Op) string {
	if p, ok := op.(*PowerOp); ok {
		if sch, err := c.P.Power.ActiveScheme(); err == nil && sch != "" {
			return p.Target() + "@" + strings.ToLower(sch)
		}
	}
	return op.Target()
}

// copyOps snapshots an entry's ops under the journal lock.
func copyOps(j *Journal, ent *Entry) []OpRecord {
	var out []OpRecord
	j.View(func() { out = append([]OpRecord(nil), ent.Ops...) })
	return out
}

// RevertOpts tunes a revert.
type RevertOpts struct {
	Force    bool // overwrite values changed after Axion applied them
	Progress func(done, total int, title string)
	Log      func(format string, args ...any)
}

// RevertTweaks reverts the active entries of the given tweak IDs.
func (e *Engine) RevertTweaks(ctx context.Context, f *facts.Facts, ids []string, o RevertOpts) []Result {
	var entries []*Entry
	var results []Result
	for _, id := range ids {
		if ent := e.J.ActiveFor(id); ent != nil {
			entries = append(entries, ent)
		} else {
			title := id
			if t := e.Resolve(id, f); t != nil {
				title = t.Title
			}
			results = append(results, Result{TweakID: id, Title: title, Outcome: OutNothing, Message: "nenhuma alteração do Axion para desfazer"})
		}
	}
	return append(results, e.RevertEntries(ctx, f, entries, o)...)
}

// RevertAll reverts every active entry, newest first.
func (e *Engine) RevertAll(ctx context.Context, f *facts.Facts, o RevertOpts) []Result {
	return e.RevertEntries(ctx, f, e.J.Active(), o)
}

// RevertEntries reverts specific journal entries (newest first).
func (e *Engine) RevertEntries(ctx context.Context, f *facts.Facts, entries []*Entry, o RevertOpts) []Result {
	e.mu.Lock()
	defer e.mu.Unlock()
	sort.SliceStable(entries, func(a, b int) bool { return entries[a].Time.After(entries[b].Time) })
	var results []Result
	for i, ent := range entries {
		if o.Progress != nil {
			o.Progress(i, len(entries), ent.TweakTitle)
		}
		results = append(results, e.revertOne(ctx, f, ent, o))
	}
	if o.Progress != nil {
		o.Progress(len(entries), len(entries), "")
	}
	return results
}

// opFor rebuilds the op that produced a record, matching by target.
func (e *Engine) opFor(f *facts.Facts, t *Tweak, target string) Op {
	if t == nil {
		return nil
	}
	for _, op := range t.Ops(f) {
		if op.Target() == target {
			return op
		}
	}
	return nil
}

func (e *Engine) revertOne(ctx context.Context, f *facts.Facts, ent *Entry, o RevertOpts) Result {
	res := Result{TweakID: ent.TweakID, Title: ent.TweakTitle, EntryID: ent.ID}
	var active bool
	var status string
	var recs []OpRecord
	e.J.View(func() {
		active, status = revertible(ent), ent.Status
		recs = append([]OpRecord(nil), ent.Ops...)
	})
	if !active {
		res.Outcome, res.Message = OutNothing, "esta alteração já não está ativa ("+status+")"
		return res
	}
	t := e.Resolve(ent.TweakID, f)
	if !ent.Verified() && (t == nil || t.Category == "correcoes") {
		// Undoing a correction brings back a broken or unsafe setting; from
		// a record this installation cannot vouch for, that is never done.
		res.Outcome = OutFailed
		res.Message = unverifiedNote + " Correções e itens que saíram do catálogo não são desfeitos a partir dele."
		return res
	}
	c := e.ctx(ctx, f, o.Log)
	res.Reboot = ""
	if t != nil {
		res.Reboot = t.Reboot.String()
	}
	// Resolve ops; fall back to generic ops reconstructed from the record.
	// Records an earlier undo already confirmed are done.
	type pair struct {
		op  Op
		idx int
	}
	var pairs []pair
	for i := range recs {
		r := &recs[i]
		if !onMachine(r) {
			continue
		}
		op := e.opFor(f, t, r.Target)
		if op == nil && ent.Verified() {
			op = reconstruct(r)
		}
		if op == nil {
			res.Outcome = OutFailed
			res.Message = "não foi possível reconstruir a operação " + r.Describe
			if !ent.Verified() {
				res.Message = unverifiedNote + " Desfaça manualmente: " + r.Describe + "."
			}
			return res
		}
		pairs = append(pairs, pair{op, i})
	}
	// Conflict detection: every resource must still hold what Axion wrote.
	if !o.Force {
		var conflicts []string
		for _, p := range pairs {
			r := &recs[p.idx]
			now, err := captureLike(c, p.op, r.Before)
			if err != nil {
				continue
			}
			if len(r.After) > 0 && !snapEqual(p.op, now, r.After) {
				if snapEqual(p.op, now, r.Before) {
					continue // already back to the original: nothing to do for this op
				}
				conflicts = append(conflicts, r.Describe+" (atual: "+p.op.Current(c)+")")
			}
		}
		if len(conflicts) > 0 {
			res.Outcome = OutConflict
			res.Message = "outro programa ou o próprio Windows alterou estes itens depois do Axion; confirme para sobrescrever: " + strings.Join(conflicts, "; ")
			return res
		}
	}
	var errs []string
	for i := len(pairs) - 1; i >= 0; i-- {
		p := pairs[i]
		r := recs[p.idx]
		rerr := p.op.Restore(c, r.Before)
		ok := false
		if now, err := captureLike(c, p.op, r.Before); err == nil {
			// Read back even after an error: the value may already be the
			// original one (then there is nothing left to undo).
			ok = snapEqual(p.op, now, r.Before)
		}
		if rerr != nil && !ok {
			errs = append(errs, r.Describe+": "+rerr.Error())
		}
		idx := p.idx
		_ = e.J.Update(func() { ent.Ops[idx].RevertVerified = ok })
	}
	now := time.Now()
	var left bool
	_ = e.J.Update(func() {
		ent.RevertedAt = &now
		left = ent.Pending()
		if !left {
			ent.Status = StatusReverted
			ent.RevertNote = ""
		} else {
			ent.Status = StatusPartialRev
			ent.RevertNote = strings.Join(errs, "; ")
			if ent.RevertNote == "" {
				ent.RevertNote = "restaurado, mas a releitura não confirmou todos os itens"
			}
			ent.RevertNote += " — use Desfazer de novo para tentar o que falta."
		}
	})
	res.Ops = copyOps(e.J, ent)
	if t != nil && t.Post != nil && len(pairs) > 0 {
		for _, p := range pairs {
			c.Touched = append(c.Touched, p.op.Target())
		}
		if err := t.Post(c, true); err != nil {
			res.Message = "Aviso: " + err.Error()
		}
	}
	if !left {
		res.Outcome = OutReverted
		return res
	}
	res.Outcome = OutFailed
	if len(errs) > 0 {
		res.Message = strings.Join(errs, "; ")
	} else {
		res.Message = "restaurado, mas a releitura não confirmou todos os itens"
	}
	return res
}

// unverifiedNote explains why an unauthenticated history entry is limited.
const unverifiedNote = "Este registro do histórico não tem a assinatura desta instalação do Axion (veio de uma versão anterior à 0.3 ou foi alterado fora do Axion), então só o que o catálogo atual descreve pode ser desfeito automaticamente."

// Reconstruct is exported for tests.
func Reconstruct(r *OpRecord) Op { return reconstruct(r) }

// reconstruct builds a generic op from a journal record when the catalog no
// longer contains the tweak (e.g. after an Axion update). Registry, service
// and power records are self-describing enough to restore.
func reconstruct(r *OpRecord) Op {
	switch r.Kind {
	case "registry":
		t := strings.TrimPrefix(r.Target, "reg:")
		idx := strings.LastIndex(t, "|")
		if idx < 0 {
			return nil
		}
		return &RegOp{Path: t[:idx], Name: t[idx+1:]}
	case "service":
		return &SvcOp{Name: strings.TrimPrefix(r.Target, "svc:")}
	case "power":
		t := strings.TrimPrefix(r.Target, "power:")
		parts := strings.Split(t, "/")
		if len(parts) != 2 {
			return nil
		}
		return &PowerOp{Sub: parts[0], Setting: parts[1], Label: "configuração de energia"}
	case "powerscheme":
		return &SchemeOp{}
	}
	if b, ok := kindBuilders[r.Kind]; ok && len(r.Spec) > 0 {
		if op, err := b(r.Spec); err == nil && op != nil && op.Target() == r.Target {
			return op
		}
	}
	return nil
}

// Drift lists active entries whose resources no longer match what Axion wrote
// (typically reset by a Windows feature update).
func (e *Engine) Drift(ctx context.Context, f *facts.Facts) []State {
	var out []State
	for _, ent := range e.J.Active() {
		t := e.Resolve(ent.TweakID, f)
		if t == nil {
			continue
		}
		st := e.StateOf(ctx, f, t)
		if st.Drifted {
			out = append(out, st)
		}
	}
	return out
}

// MarshalSnapshot is exported for tests.
func MarshalSnapshot(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// ReconcileHistory closes failed or partially reverted entries that a
// later, successful application of the same tweak covered: every change
// they still hold is on a resource the newer entry also recorded, so the
// newer entry's undo is the one that restores it. Entries written before
// the carry-over existed (0.6) were left "pending" forever and showed up
// as repeated "undo again" items. The remaining changes on resources the
// newer entry did not touch are carried to it, as an apply does. Returns
// how many entries were closed.
func (e *Engine) ReconcileHistory() int {
	n := 0
	_ = e.J.Update(func() {
		byTweak := map[string][]*Entry{}
		for _, ent := range e.J.entries {
			byTweak[ent.TweakID] = append(byTweak[ent.TweakID], ent)
		}
		for _, list := range byTweak {
			// The newest applied entry still on the machine.
			var newest *Entry
			for _, ent := range list {
				if ent.Status == StatusApplied && ent.Changed() && (newest == nil || ent.Time.After(newest.Time)) {
					newest = ent
				}
			}
			// Only signed entries take part: carrying records of an unsigned
			// entry into a signed one would sign them on the next save, and
			// carrying signed records into an unsigned one would lose the
			// exact undo that only signed records get.
			if newest == nil || !newest.verified {
				continue
			}
			covered := map[string]bool{}
			for i := range newest.Ops {
				covered[recordKey(&newest.Ops[i])] = true
			}
			for _, old := range list {
				if old == newest || !old.verified || !old.Time.Before(newest.Time) || (old.Status != StatusFailed && old.Status != StatusPartialRev) || !old.Pending() {
					continue
				}
				for i := range old.Ops {
					r := old.Ops[i]
					if !onMachine(&r) {
						continue
					}
					if k := recordKey(&r); !covered[k] {
						if r.CarriedFrom == "" {
							r.CarriedFrom = old.ID
						}
						newest.Ops = append(newest.Ops, r)
						covered[k] = true
					}
				}
				old.Status = StatusSuperseded
				old.RevertNote = "substituída pela aplicação " + newest.ID + " (os valores anteriores que ainda restavam passaram para ela)"
				n++
			}
		}
	})
	return n
}

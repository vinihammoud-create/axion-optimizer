package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"axion/internal/tweaks"
)

// Job states.
const (
	JobRunning   = "running"
	JobDone      = "done"
	JobFailed    = "failed"
	JobCancelled = "cancelled"
)

// ErrBusy is returned when an exclusive job is already running.
var ErrBusy = errors.New("outra tarefa está em andamento; aguarde terminar")

// Job is a long-running operation (scan, apply, revert...). The UI polls it.
type Job struct {
	ID       string     `json:"id"`
	Kind     string     `json:"kind"`
	Title    string     `json:"title"`
	Status   string     `json:"status"`
	Progress float64    `json:"progress"` // 0..1
	Step     string     `json:"step"`
	Log      []string   `json:"log"`
	Result   any        `json:"result,omitempty"`
	Error    string     `json:"error,omitempty"`
	Started  time.Time  `json:"started"`
	Ended    *time.Time `json:"ended,omitempty"`
	// NoCancel explains why the current step cannot be interrupted (e.g. a
	// Windows Update operation); the interface hides its Cancel button.
	NoCancel string `json:"noCancel,omitempty"`

	cancel context.CancelFunc
}

// JobCtl is handed to the job function to report progress.
type JobCtl struct {
	m  *Jobs
	id string
	// A span maps the progress of a step of a composite job (the full
	// evaluation runs the analysis, the network test, the capture and the
	// polling test one after the other) into its share of the whole.
	base, size float64
	prefix     string
}

// span returns a control whose progress 0..1 fills base..base+size of the
// job, with the step text prefixed (the name of the stage). The root
// control (size 0) spans the whole job.
func (c *JobCtl) span(base, size float64, prefix string) *JobCtl {
	whole := c.size
	if whole == 0 {
		whole = 1
	}
	return &JobCtl{m: c.m, id: c.id, base: c.base + whole*base, size: whole * size, prefix: prefix}
}

func (c *JobCtl) frac(f float64) float64 {
	if c.size == 0 {
		return f
	}
	return c.base + c.size*f
}

func (c *JobCtl) text(step string) string {
	if c.prefix != "" && step != "" {
		return c.prefix + " · " + step
	}
	return step
}

// Progress sets the fraction done and the current step.
func (c *JobCtl) Progress(frac float64, step string) {
	frac = c.frac(frac)
	step = c.text(step)
	c.m.update(c.id, func(j *Job) {
		if frac < 0 {
			frac = 0
		}
		if frac > 1 {
			frac = 1
		}
		j.Progress = frac
		if step != "" {
			j.Step = step
		}
	})
}

// NoCancel marks the job as not interruptible from now on ("" allows it again).
func (c *JobCtl) NoCancel(reason string) {
	c.m.update(c.id, func(j *Job) { j.NoCancel = reason })
}

// Step changes the current step text, keeping the progress.
func (c *JobCtl) Step(step string) {
	step = c.text(step)
	c.m.update(c.id, func(j *Job) { j.Step = step })
}

// Logf appends a line to the job log (capped).
func (c *JobCtl) Logf(format string, args ...any) {
	line := time.Now().Format("15:04:05") + "  " + fmt.Sprintf(format, args...)
	c.m.update(c.id, func(j *Job) {
		if len(j.Log) < 2000 {
			j.Log = append(j.Log, line)
		}
	})
}

// Jobs keeps recent jobs in memory.
type Jobs struct {
	mu        sync.Mutex
	jobs      map[string]*Job
	exclusive string // id of the running exclusive job
	held      int    // quick changes running through RunExclusive
	OnDone    func(j Job)
}

// NewJobs builds a job manager.
func NewJobs() *Jobs { return &Jobs{jobs: map[string]*Job{}} }

func (m *Jobs) update(id string, fn func(j *Job)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.jobs[id]; ok {
		fn(j)
	}
}

// Start runs fn in a goroutine. Exclusive jobs refuse to start while another
// exclusive job runs (every job that changes the machine is exclusive).
func (m *Jobs) Start(kind, title string, exclusive bool, fn func(ctx context.Context, c *JobCtl) (any, error)) (Job, error) {
	m.mu.Lock()
	if exclusive && m.busyLocked() {
		m.mu.Unlock()
		return Job{}, ErrBusy
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &Job{ID: tweaks.NewID(), Kind: kind, Title: title, Status: JobRunning, Started: time.Now(), cancel: cancel}
	m.jobs[j.ID] = j
	if exclusive {
		m.exclusive = j.ID
	}
	m.pruneLocked()
	first := snapshot(j)
	m.mu.Unlock()

	ctl := &JobCtl{m: m, id: j.ID}
	log.Printf("tarefa iniciada: %s (%s)", title, kind)
	go func() {
		var res any
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("falha interna: %v", r)
				}
			}()
			res, err = fn(ctx, ctl)
		}()
		now := time.Now()
		var done Job
		m.mu.Lock()
		j.Ended = &now
		j.Result = res
		switch {
		case ctx.Err() != nil && err != nil:
			j.Status, j.Error = JobCancelled, "cancelado"
		case err != nil:
			j.Status, j.Error = JobFailed, err.Error()
		default:
			j.Status, j.Progress = JobDone, 1
		}
		if m.exclusive == j.ID {
			m.exclusive = ""
		}
		done = snapshot(j)
		cb := m.OnDone
		m.mu.Unlock()
		if done.Error != "" {
			log.Printf("tarefa terminada: %s (%s): %s", done.Title, done.Status, done.Error)
		} else {
			log.Printf("tarefa terminada: %s (%s)", done.Title, done.Status)
		}
		cancel()
		if cb != nil {
			cb(done)
		}
	}()
	return first, nil
}

// busyLocked: an exclusive job or a quick exclusive change is running.
func (m *Jobs) busyLocked() bool {
	if m.held > 0 {
		return true
	}
	if m.exclusive == "" {
		return false
	}
	j, ok := m.jobs[m.exclusive]
	return ok && j.Status == JobRunning
}

// RunExclusive runs a quick change (startup items, quarantine, a driver
// hide...) synchronously while holding the exclusive slot, so it never
// overlaps a job that changes the machine: an apply records the value it
// found before changing it, and a change made in between would be lost from
// that record. It fails with ErrBusy while such a job runs.
func (m *Jobs) RunExclusive(fn func() error) error {
	m.mu.Lock()
	if m.busyLocked() {
		m.mu.Unlock()
		return ErrBusy
	}
	m.held++
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.held--
		m.mu.Unlock()
	}()
	return fn()
}

func snapshot(j *Job) Job {
	c := *j
	c.Log = append([]string(nil), j.Log...)
	c.cancel = nil
	return c
}

// Get returns a copy of a job.
func (m *Jobs) Get(id string) (Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return Job{}, false
	}
	return snapshot(j), true
}

// Held reports whether a quick exclusive change (RunExclusive) is running.
func (m *Jobs) Held() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.held > 0
}

// Running returns the running exclusive job, if any.
func (m *Jobs) Running() (Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.exclusive == "" {
		return Job{}, false
	}
	j, ok := m.jobs[m.exclusive]
	if !ok || j.Status != JobRunning {
		return Job{}, false
	}
	return snapshot(j), true
}

// Cancel requests cancellation (the job stops at the next safe point).
func (m *Jobs) Cancel(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.jobs[id]; ok && j.Status == JobRunning && j.cancel != nil && j.NoCancel == "" {
		j.cancel()
		return true
	}
	return false
}

// Recent lists jobs newest first (without logs).
func (m *Jobs) Recent(n int) []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Job
	for _, j := range m.jobs {
		c := snapshot(j)
		c.Log = nil
		c.Result = nil
		out = append(out, c)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Started.After(out[b].Started) })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func (m *Jobs) pruneLocked() {
	if len(m.jobs) <= 40 {
		return
	}
	var ids []*Job
	for _, j := range m.jobs {
		if j.Status != JobRunning {
			ids = append(ids, j)
		}
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a].Started.Before(ids[b].Started) })
	for i := 0; i < len(ids) && len(m.jobs) > 30; i++ {
		delete(m.jobs, ids[i].ID)
	}
}

// exitCancellable are the job kinds stopped when the interface closes: they
// only read, measure, download or delete temporary files, and stop at a safe
// point. Cancelling an uninstall only stops Axion's wait (the program's own
// uninstaller keeps running). Applying, undoing, the restore point and
// driver installs are left to finish, so their history entry is saved.
var exitCancellable = map[string]bool{
	"scan": true, "measure": true, "perf": true, "retrim": true, "cleanup": true, "uninstall": true, "net": true, "dpc": true, "evaluate": true, "apps-refresh": true,
	"drivers-scan": true, "drivers-download": true, "gpu-download": true,
	// A trial that is cancelled undoes its tweak (through the journal, with
	// the cancel refused while the undo runs) before it ends.
	"trial": true,
}

// Shutdown is called when the interface closes. It cancels the running job
// when that is safe, waits for it (and for a quick exclusive change) to end,
// up to max, and calls slow once, with what is running, if that takes more
// than a moment. It reports whether nothing is running anymore.
func (a *App) Shutdown(max time.Duration, slow func(title string)) bool {
	start := time.Now()
	notified := false
	for {
		j, running := a.Jobs.Running()
		held := a.Jobs.Held()
		if !running && !held {
			return true
		}
		if running && exitCancellable[j.Kind] {
			a.Jobs.Cancel(j.ID) // refused while the step cannot be interrupted
		}
		if time.Since(start) >= max {
			return false
		}
		if !notified && slow != nil && time.Since(start) >= 1500*time.Millisecond {
			notified = true
			title := "uma alteração rápida"
			if running {
				title = j.Title
			}
			slow(title)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

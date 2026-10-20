//go:build windows

package winsys

import (
	"fmt"
	"strings"
	"sync"
	"time"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"

	"axion/internal/sys"
)

// Tasks implements sys.Tasks through the Schedule.Service COM object.
type Tasks struct {
	COM *COM
	mu  sync.Mutex
	gen uint64
	svc *ole.IDispatch // for the COM worker of generation gen
}

const taskTimeout = 30 * time.Second

func (t *Tasks) service() (*ole.IDispatch, error) {
	t.mu.Lock()
	if g := t.COM.Gen(); t.gen != g {
		t.svc, t.gen = nil, g // left to the abandoned worker
	}
	svc := t.svc
	t.mu.Unlock()
	if svc != nil {
		return svc, nil
	}
	unk, err := oleutil.CreateObject("Schedule.Service")
	if err != nil {
		return nil, err
	}
	defer unk.Release()
	d, err := unk.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, err
	}
	if _, err := oleutil.CallMethod(d, "Connect"); err != nil {
		d.Release()
		return nil, err
	}
	t.mu.Lock()
	if t.gen == t.COM.Gen() {
		t.svc = d
	}
	t.mu.Unlock()
	return d, nil
}

func splitTaskPath(p string) (folder, name string) {
	p = strings.TrimSpace(p)
	i := strings.LastIndex(p, `\`)
	if i <= 0 {
		return `\`, strings.TrimPrefix(p, `\`)
	}
	return p[:i], p[i+1:]
}

// isNotFoundCOM tells whether a Task Scheduler call failed because the task
// or folder does not exist (ERROR_FILE_NOT_FOUND / ERROR_PATH_NOT_FOUND). The
// code is inside the IDispatch exception: the first real report (0.3.2) got
// "Exceção. (<nil>)" for a task removed by another optimizer.
func isNotFoundCOM(err error) bool {
	switch hresultOf(err) {
	case 0x80070002, 0x80070003:
		return true
	}
	s := err.Error()
	return strings.Contains(s, "0x80070002") || strings.Contains(s, "0x80070003")
}

func (t *Tasks) folder(path string) (*ole.IDispatch, error) {
	svc, err := t.service()
	if err != nil {
		return nil, err
	}
	raw, err := oleutil.CallMethod(svc, "GetFolder", path)
	if err != nil {
		return nil, err
	}
	return raw.ToIDispatch(), nil
}

// taskStr reads a property as text and frees the returned VARIANT (BSTR).
func taskStr(d *ole.IDispatch, name string) string {
	v, err := oleutil.GetProperty(d, name)
	if err != nil {
		return ""
	}
	defer v.Clear()
	return v.ToString()
}

// taskVal reads a scalar property (bool, integers) and frees the VARIANT.
func taskVal(d *ole.IDispatch, name string) (any, bool) {
	v, err := oleutil.GetProperty(d, name)
	if err != nil {
		return nil, false
	}
	defer v.Clear()
	return v.Value(), true
}

// taskObj reads an object property; the caller releases it.
func taskObj(d *ole.IDispatch, name string) (*ole.IDispatch, error) {
	v, err := oleutil.GetProperty(d, name)
	if err != nil {
		return nil, err
	}
	x := v.ToIDispatch()
	if x == nil {
		_ = v.Clear()
		return nil, fmt.Errorf("%s: objeto vazio", name)
	}
	return x, nil
}

// taskDate converts a Task Scheduler DATE (local wall-clock time, decoded by
// go-ole with a UTC label) to a local time. Tasks that never ran report
// 30/11/1999; that and anything before 2000 is returned as zero.
func taskDate(v any) time.Time {
	t, ok := v.(time.Time)
	if !ok || t.Year() < 2000 {
		return time.Time{}
	}
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.Local)
}

func readTask(task *ole.IDispatch) sys.TaskInfo {
	info := sys.TaskInfo{Exists: true, Path: taskStr(task, "Path"), Name: taskStr(task, "Name")}
	if v, ok := taskVal(task, "Enabled"); ok {
		if b, ok := v.(bool); ok {
			info.Enabled = b
		}
	}
	if v, ok := taskVal(task, "State"); ok {
		info.State = int((sys.WMIRow{"v": v}).Int("v"))
	}
	if v, ok := taskVal(task, "LastRunTime"); ok {
		info.LastRun = taskDate(v)
	}
	if v, ok := taskVal(task, "LastTaskResult"); ok {
		info.LastResult = int(int32((sys.WMIRow{"v": v}).Int("v")))
	}
	def, err := taskObj(task, "Definition")
	if err != nil {
		return info
	}
	defer def.Release()
	if d, err := taskObj(def, "RegistrationInfo"); err == nil {
		info.Author = taskStr(d, "Author")
		d.Release()
	}
	if d, err := taskObj(def, "Settings"); err == nil {
		if v, ok := taskVal(d, "Hidden"); ok {
			if b, ok := v.(bool); ok {
				info.Hidden = b
			}
		}
		d.Release()
	}
	if d, err := taskObj(def, "Triggers"); err == nil {
		_ = oleutil.ForEach(d, func(v *ole.VARIANT) error {
			x := v.ToIDispatch()
			if x == nil {
				return nil
			}
			defer x.Release()
			if ty, ok := taskVal(x, "Type"); ok {
				info.Triggers = append(info.Triggers, int((sys.WMIRow{"v": ty}).Int("v")))
			}
			return nil
		})
		d.Release()
	}
	if d, err := taskObj(def, "Actions"); err == nil {
		_ = oleutil.ForEach(d, func(v *ole.VARIANT) error {
			x := v.ToIDispatch()
			if x == nil {
				return nil
			}
			defer x.Release()
			ty, ok := taskVal(x, "Type")
			if !ok || (sys.WMIRow{"v": ty}).Int("v") != 0 {
				return nil
			}
			s := taskStr(x, "Path")
			if a := taskStr(x, "Arguments"); a != "" {
				s += " " + a
			}
			info.Actions = append(info.Actions, s)
			return nil
		})
		d.Release()
	}
	return info
}

// Get implements sys.Tasks.
func (t *Tasks) Get(path string) (sys.TaskInfo, error) {
	info := sys.TaskInfo{Path: path}
	err := t.COM.Do(taskTimeout, func() error {
		dir, name := splitTaskPath(path)
		f, err := t.folder(dir)
		if err != nil {
			if isNotFoundCOM(err) {
				return nil
			}
			return err
		}
		defer f.Release()
		raw, err := oleutil.CallMethod(f, "GetTask", name)
		if err != nil {
			if isNotFoundCOM(err) {
				return nil
			}
			return err
		}
		task := raw.ToIDispatch()
		defer task.Release()
		info = readTask(task)
		return nil
	})
	return info, comErr(err)
}

// SetEnabled implements sys.Tasks.
func (t *Tasks) SetEnabled(path string, enabled bool) error {
	return comErr(t.COM.Do(taskTimeout, func() error {
		dir, name := splitTaskPath(path)
		f, err := t.folder(dir)
		if err != nil {
			return err
		}
		defer f.Release()
		raw, err := oleutil.CallMethod(f, "GetTask", name)
		if err != nil {
			return err
		}
		task := raw.ToIDispatch()
		defer task.Release()
		_, err = oleutil.PutProperty(task, "Enabled", enabled)
		return err
	}))
}

// List implements sys.Tasks.
func (t *Tasks) List(folder string, recursive bool) ([]sys.TaskInfo, error) {
	// One COM call per folder, each with its own timeout: a machine with
	// hundreds of tasks no longer hits one global limit (which abandoned
	// the COM worker and lost everything read so far). Results of a call
	// are read only when it finished in time.
	type item struct {
		path  string
		depth int
	}
	var out []sys.TaskInfo
	queue := []item{{folder, 0}}
	deadline := time.Now().Add(3 * time.Minute)
	for len(queue) > 0 && time.Now().Before(deadline) {
		cur := queue[0]
		queue = queue[1:]
		type result struct {
			tasks []sys.TaskInfo
			subs  []string
		}
		res := &result{}
		err := t.COM.Do(taskTimeout, func() error {
			f, err := t.folder(cur.path)
			if err != nil {
				if isNotFoundCOM(err) {
					return nil
				}
				return err
			}
			defer f.Release()
			if raw, err := oleutil.CallMethod(f, "GetTasks", 1); err == nil { // TASK_ENUM_HIDDEN
				col := raw.ToIDispatch()
				_ = oleutil.ForEach(col, func(v *ole.VARIANT) error {
					task := v.ToIDispatch()
					defer task.Release()
					res.tasks = append(res.tasks, readTask(task))
					return nil
				})
				col.Release()
			}
			if !recursive || cur.depth > 8 {
				return nil
			}
			raw, err := oleutil.CallMethod(f, "GetFolders", 0)
			if err != nil {
				return nil
			}
			col := raw.ToIDispatch()
			defer col.Release()
			_ = oleutil.ForEach(col, func(v *ole.VARIANT) error {
				sf := v.ToIDispatch()
				if sf == nil {
					return nil
				}
				defer sf.Release()
				if p := taskStr(sf, "Path"); p != "" {
					res.subs = append(res.subs, p)
				}
				return nil
			})
			return nil
		})
		if err != nil {
			if cur.path == folder {
				return out, comErr(err)
			}
			continue // one unreadable folder does not hide the others
		}
		out = append(out, res.tasks...)
		for _, sub := range res.subs {
			queue = append(queue, item{sub, cur.depth + 1})
		}
	}
	return out, nil
}

// Register implements sys.Tasks (creates or replaces the task).
func (t *Tasks) Register(path, description, exe, args string, trigger int, at string, weekday int, highest bool) error {
	return comErr(t.COM.Do(taskTimeout, func() error {
		svc, err := t.service()
		if err != nil {
			return err
		}
		dir, name := splitTaskPath(path)
		root, err := t.folder(`\`)
		if err != nil {
			return err
		}
		defer root.Release()
		f, err := t.folder(dir)
		if err != nil {
			if !isNotFoundCOM(err) {
				return err
			}
			raw, err := oleutil.CallMethod(root, "CreateFolder", strings.TrimPrefix(dir, `\`), nil)
			if err != nil {
				return err
			}
			f = raw.ToIDispatch()
		}
		defer f.Release()
		raw, err := oleutil.CallMethod(svc, "NewTask", 0)
		if err != nil {
			return err
		}
		def := raw.ToIDispatch()
		defer def.Release()
		ri, err := taskObj(def, "RegistrationInfo")
		if err != nil {
			return err
		}
		_, _ = oleutil.PutProperty(ri, "Description", description)
		_, _ = oleutil.PutProperty(ri, "Author", "Axion Optimizer")
		ri.Release()
		pr, err := taskObj(def, "Principal")
		if err != nil {
			return err
		}
		if highest {
			_, _ = oleutil.PutProperty(pr, "RunLevel", 1) // TASK_RUNLEVEL_HIGHEST
		}
		_, _ = oleutil.PutProperty(pr, "LogonType", 3) // TASK_LOGON_INTERACTIVE_TOKEN
		// A task with the user's normal rights runs as the person signed in,
		// also when Axion was elevated with another administrator account.
		var userID any
		if u := consoleUser(); !highest && u.Differs && u.Name != "" {
			userID = u.Name
			_, _ = oleutil.PutProperty(pr, "UserId", u.Name)
		}
		pr.Release()
		st, err := taskObj(def, "Settings")
		if err != nil {
			return err
		}
		_, _ = oleutil.PutProperty(st, "StartWhenAvailable", true)
		_, _ = oleutil.PutProperty(st, "DisallowStartIfOnBatteries", false)
		_, _ = oleutil.PutProperty(st, "StopIfGoingOnBatteries", false)
		_, _ = oleutil.PutProperty(st, "ExecutionTimeLimit", "PT2H")
		st.Release()
		trs, err := taskObj(def, "Triggers")
		if err != nil {
			return err
		}
		trRaw, err := oleutil.CallMethod(trs, "Create", trigger)
		trs.Release()
		if err != nil {
			return err
		}
		tr := trRaw.ToIDispatch()
		start := time.Now()
		if at != "" {
			if hm, err := time.Parse("15:04", at); err == nil {
				start = time.Date(start.Year(), start.Month(), start.Day(), hm.Hour(), hm.Minute(), 0, 0, time.Local)
			}
		}
		switch trigger {
		case sys.TriggerDaily:
			_, _ = oleutil.PutProperty(tr, "StartBoundary", start.Format("2006-01-02T15:04:05"))
			_, _ = oleutil.PutProperty(tr, "DaysInterval", 1)
		case sys.TriggerWeekly:
			_, _ = oleutil.PutProperty(tr, "StartBoundary", start.Format("2006-01-02T15:04:05"))
			_, _ = oleutil.PutProperty(tr, "WeeksInterval", 1)
			_, _ = oleutil.PutProperty(tr, "DaysOfWeek", 1<<uint(weekday%7))
		case sys.TriggerLogon:
			_, _ = oleutil.PutProperty(tr, "Delay", "PT30S")
		}
		tr.Release()
		acs, err := taskObj(def, "Actions")
		if err != nil {
			return err
		}
		acRaw, err := oleutil.CallMethod(acs, "Create", 0) // TASK_ACTION_EXEC
		acs.Release()
		if err != nil {
			return err
		}
		ac := acRaw.ToIDispatch()
		_, _ = oleutil.PutProperty(ac, "Path", exe)
		_, _ = oleutil.PutProperty(ac, "Arguments", args)
		ac.Release()
		// TASK_CREATE_OR_UPDATE = 6, TASK_LOGON_INTERACTIVE_TOKEN = 3
		reg, err := oleutil.CallMethod(f, "RegisterTaskDefinition", name, def, 6, userID, nil, 3, nil)
		if err != nil {
			return fmt.Errorf("registrar tarefa: %w", err)
		}
		_ = reg.Clear() // releases the IRegisteredTask it returns
		return nil
	}))
}

// Delete implements sys.Tasks.
func (t *Tasks) Delete(path string) error {
	return comErr(t.COM.Do(taskTimeout, func() error {
		dir, name := splitTaskPath(path)
		f, err := t.folder(dir)
		if err != nil {
			if isNotFoundCOM(err) {
				return nil
			}
			return err
		}
		defer f.Release()
		if _, err := oleutil.CallMethod(f, "DeleteTask", name, 0); err != nil && !isNotFoundCOM(err) {
			return err
		}
		return nil
	}))
}

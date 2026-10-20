//go:build windows

package winsys

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"

	"axion/internal/sys"
)

// comWorker runs COM calls on a dedicated OS thread (COM apartments are
// per-thread). A hung call (broken WMI repository) only blocks that worker:
// the next request after a timeout starts a fresh worker.
type comWorker struct {
	reqs chan func()
	dead bool
}

func newCOMWorker() *comWorker {
	w := &comWorker{reqs: make(chan func(), 16)}
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
			// S_FALSE (already initialized) is returned as an error by go-ole
			// in some versions; continue either way.
			_ = err
		}
		close(ready)
		for fn := range w.reqs {
			fn()
		}
		ole.CoUninitialize()
	}()
	<-ready
	return w
}

// COM serializes COM work with timeouts.
type COM struct {
	mu     sync.Mutex
	worker *comWorker
	gen    uint64 // bumped when a worker is abandoned
}

// Gen returns the current worker generation: objects cached by a previous
// generation belong to an abandoned worker (whose call may still be
// running) and must not be reused or shared with the new one.
func (c *COM) Gen() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen
}

// Do runs fn on the COM thread with a timeout.
func (c *COM) Do(timeout time.Duration, fn func() error) error {
	c.mu.Lock()
	if c.worker == nil || c.worker.dead {
		c.worker = newCOMWorker()
	}
	w := c.worker
	c.mu.Unlock()
	done := make(chan error, 1)
	select {
	case w.reqs <- func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("falha COM: %v", r)
			}
		}()
		done <- fn()
	}:
	case <-time.After(timeout):
		c.abandon(w)
		return errors.New("COM ocupado (tempo esgotado)")
	}
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		c.abandon(w)
		return errors.New("tempo esgotado aguardando o Windows (WMI/COM)")
	}
}

func (c *COM) abandon(w *comWorker) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.worker == w {
		w.dead = true
		c.worker = nil
		c.gen++
	}
}

// ---------------------------------------------------------------------------
// WMI
// ---------------------------------------------------------------------------

// WMI implements sys.WMI through WbemScripting.SWbemLocator.
type WMI struct {
	COM      *COM
	Timeout  time.Duration
	mu       sync.Mutex
	gen      uint64
	services map[string]*ole.IDispatch // per namespace, for the worker of generation gen
}

func (w *WMI) service(ns string) (*ole.IDispatch, error) {
	key := strings.ToLower(ns)
	w.mu.Lock()
	if g := w.COM.Gen(); w.services == nil || w.gen != g {
		// New worker: the old connections stay with the abandoned one
		// (never released from here, it may still be using them).
		w.services, w.gen = map[string]*ole.IDispatch{}, g
	}
	s, ok := w.services[key]
	w.mu.Unlock()
	if ok {
		return s, nil
	}
	unk, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return nil, err
	}
	defer unk.Release()
	loc, err := unk.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, err
	}
	defer loc.Release()
	raw, err := oleutil.CallMethod(loc, "ConnectServer", nil, ns)
	if err != nil {
		return nil, fmt.Errorf("namespace %s: %w", ns, err)
	}
	s = raw.ToIDispatch()
	w.mu.Lock()
	if w.gen == w.COM.Gen() {
		w.services[key] = s
	}
	w.mu.Unlock()
	return s, nil
}

func (w *WMI) timeout() time.Duration {
	if w.Timeout > 0 {
		return w.Timeout
	}
	return 25 * time.Second
}

// selectColumns parses "SELECT a, b FROM" (nil for "*").
func selectColumns(wql string) []string {
	up := strings.ToUpper(wql)
	i := strings.Index(up, "SELECT ")
	j := strings.Index(up, " FROM ")
	if i < 0 || j < 0 || j < i {
		return nil
	}
	cols := strings.TrimSpace(wql[i+7 : j])
	if cols == "*" {
		return nil
	}
	var out []string
	for _, c := range strings.Split(cols, ",") {
		if c = strings.TrimSpace(c); c != "" {
			out = append(out, c)
		}
	}
	return out
}

func variantValue(v *ole.VARIANT) any {
	if v == nil {
		return nil
	}
	if v.VT&ole.VT_ARRAY != 0 {
		arr := v.ToArray()
		if arr == nil {
			return nil
		}
		return arr.ToValueArray()
	}
	switch v.VT {
	case ole.VT_NULL, ole.VT_EMPTY:
		return nil
	case ole.VT_DISPATCH, ole.VT_UNKNOWN:
		// An object would be released with the VARIANT (callers Clear it):
		// never hand out a pointer that is about to dangle.
		return nil
	}
	return v.Value()
}

// Query implements sys.WMI.
func (w *WMI) Query(ns, wql string) ([]sys.WMIRow, error) {
	var rows []sys.WMIRow
	err := w.COM.Do(w.timeout(), func() error {
		svc, err := w.service(ns)
		if err != nil {
			return err
		}
		// wbemFlagForwardOnly | wbemFlagReturnImmediately
		raw, err := oleutil.CallMethod(svc, "ExecQuery", wql, "WQL", 48)
		if err != nil {
			return err
		}
		result := raw.ToIDispatch()
		defer result.Release()
		cols := selectColumns(wql)
		return oleutil.ForEach(result, func(v *ole.VARIANT) error {
			item := v.ToIDispatch()
			defer item.Release()
			row := sys.WMIRow{}
			if cols == nil {
				pr, err := oleutil.GetProperty(item, "Properties_")
				if err != nil {
					return err
				}
				props := pr.ToIDispatch()
				defer props.Release()
				_ = oleutil.ForEach(props, func(pv *ole.VARIANT) error {
					p := pv.ToIDispatch()
					defer p.Release()
					// Every VARIANT returned is cleared (the BSTRs are
					// allocated per call): SELECT * over hundreds of rows
					// would otherwise leak thousands of strings per scan.
					name, err := oleutil.GetProperty(p, "Name")
					if err != nil {
						if name != nil {
							_ = name.Clear()
						}
						return nil
					}
					key := name.ToString()
					_ = name.Clear()
					val, err := oleutil.GetProperty(p, "Value")
					if err == nil {
						row[key] = variantValue(val)
					}
					if val != nil {
						_ = val.Clear()
					}
					return nil
				})
			} else {
				for _, c := range cols {
					val, err := oleutil.GetProperty(item, c)
					if err != nil {
						row[c] = nil
						continue
					}
					row[c] = variantValue(val)
					_ = val.Clear()
				}
			}
			rows = append(rows, row)
			return nil
		})
	})
	return rows, err
}

// Call implements sys.WMI.
func (w *WMI) Call(ns, objectPath, method string, args ...any) (any, error) {
	var out any
	err := w.COM.Do(w.timeout(), func() error {
		svc, err := w.service(ns)
		if err != nil {
			return err
		}
		raw, err := oleutil.CallMethod(svc, "Get", objectPath)
		if err != nil {
			return fmt.Errorf("%s: %w", objectPath, err)
		}
		obj := raw.ToIDispatch()
		defer obj.Release()
		res, err := oleutil.CallMethod(obj, method, args...)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", objectPath, method, err)
		}
		out = variantValue(res)
		_ = res.Clear()
		return nil
	})
	return out, err
}

// objectRow reads every property of an SWbemObject into a row. A property
// that holds an embedded object (the out parameter of PS_MMAgent.Get is an
// MMAgentComponents instance) becomes a nested row.
func objectRow(item *ole.IDispatch) (sys.WMIRow, error) {
	pr, err := oleutil.GetProperty(item, "Properties_")
	if err != nil {
		return nil, err
	}
	props := pr.ToIDispatch()
	defer props.Release()
	row := sys.WMIRow{}
	err = oleutil.ForEach(props, func(pv *ole.VARIANT) error {
		p := pv.ToIDispatch()
		defer p.Release()
		name, err := oleutil.GetProperty(p, "Name")
		if err != nil {
			if name != nil {
				_ = name.Clear()
			}
			return nil
		}
		key := name.ToString()
		_ = name.Clear()
		val, err := oleutil.GetProperty(p, "Value")
		if err != nil {
			if val != nil {
				_ = val.Clear()
			}
			return nil
		}
		defer val.Clear()
		if val.VT == ole.VT_DISPATCH && val.Val != 0 {
			// Read before the VARIANT is cleared (which releases the object).
			if nested, err := objectRow(val.ToIDispatch()); err == nil {
				row[key] = nested
			}
			return nil
		}
		if val.VT&ole.VT_ARRAY != 0 && val.VT&0xFFF == ole.VT_DISPATCH {
			return nil // an array of objects would hand out pointers that dangle
		}
		row[key] = variantValue(val)
		return nil
	})
	return row, err
}

// CallOut implements sys.WMI: the input parameters are set by name on an
// instance of the method's in-parameter class, the method runs through
// SWbemObject.ExecMethod_ (it works for static methods on a class object) and
// the output parameters come back as a row.
func (w *WMI) CallOut(ns, objectPath, method string, in map[string]any) (sys.WMIRow, error) {
	var out sys.WMIRow
	err := w.COM.Do(w.timeout(), func() error {
		svc, err := w.service(ns)
		if err != nil {
			return err
		}
		raw, err := oleutil.CallMethod(svc, "Get", objectPath)
		if err != nil {
			return fmt.Errorf("%s: %w", objectPath, comErr(err))
		}
		obj := raw.ToIDispatch()
		defer obj.Release()
		var res *ole.VARIANT
		if len(in) == 0 {
			res, err = oleutil.CallMethod(obj, "ExecMethod_", method)
		} else {
			ms, merr := oleutil.GetProperty(obj, "Methods_")
			if merr != nil {
				return fmt.Errorf("%s.%s: %w", objectPath, method, comErr(merr))
			}
			methods := ms.ToIDispatch()
			defer methods.Release()
			mm, merr := oleutil.CallMethod(methods, "Item", method)
			if merr != nil {
				return fmt.Errorf("%s.%s: %w", objectPath, method, comErr(merr))
			}
			meth := mm.ToIDispatch()
			defer meth.Release()
			ip, merr := oleutil.GetProperty(meth, "InParameters")
			if merr != nil {
				return fmt.Errorf("%s.%s: %w", objectPath, method, comErr(merr))
			}
			if ip.VT != ole.VT_DISPATCH || ip.Val == 0 {
				_ = ip.Clear()
				return fmt.Errorf("%s.%s: o método não tem parâmetros de entrada", objectPath, method)
			}
			inDef := ip.ToIDispatch()
			defer inDef.Release()
			sp, merr := oleutil.CallMethod(inDef, "SpawnInstance_")
			if merr != nil {
				return fmt.Errorf("%s.%s: %w", objectPath, method, comErr(merr))
			}
			inst := sp.ToIDispatch()
			defer inst.Release()
			for k, v := range in {
				if _, perr := oleutil.PutProperty(inst, k, v); perr != nil {
					return fmt.Errorf("%s.%s: parâmetro %s: %w", objectPath, method, k, comErr(perr))
				}
			}
			res, err = oleutil.CallMethod(obj, "ExecMethod_", method, inst)
		}
		if err != nil {
			return fmt.Errorf("%s.%s: %w", objectPath, method, comErr(err))
		}
		outObj := res.ToIDispatch()
		defer outObj.Release()
		out, err = objectRow(outObj)
		return err
	})
	return out, err
}

// Put implements sys.WMI: it sets the property on every object the query
// returns and saves it with SWbemObject.Put_.
func (w *WMI) Put(ns, wql, property string, value any) (int, error) {
	if !sys.IsSelectStar(wql) {
		return 0, fmt.Errorf("WMI: gravação exige SELECT * (instância completa): %s", wql)
	}
	n := 0
	err := w.COM.Do(w.timeout(), func() error {
		svc, err := w.service(ns)
		if err != nil {
			return err
		}
		raw, err := oleutil.CallMethod(svc, "ExecQuery", wql, "WQL", 48)
		if err != nil {
			return err
		}
		result := raw.ToIDispatch()
		defer result.Release()
		return oleutil.ForEach(result, func(v *ole.VARIANT) error {
			item := v.ToIDispatch()
			defer item.Release()
			if _, err := oleutil.PutProperty(item, property, value); err != nil {
				return fmt.Errorf("%s: %w", property, err)
			}
			if _, err := oleutil.CallMethod(item, "Put_"); err != nil {
				return fmt.Errorf("Put_: %w", comErr(err))
			}
			n++
			return nil
		})
	})
	return n, err
}

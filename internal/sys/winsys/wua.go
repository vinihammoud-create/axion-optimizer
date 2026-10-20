//go:build windows

package winsys

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"

	"axion/internal/sys"
)

// WindowsUpdate implements sys.Updates with the Windows Update Agent API
// (Microsoft.Update.Session), the engine behind Settings > Windows Update.
// It runs on its own COM thread: downloads and installations take minutes
// and must not block the WMI queries of the rest of the app.
//
// No COM object outlives a call: operations on chosen updates search again
// by UpdateID, first in the local Windows Update cache (Online = false,
// fast) and only then online.
type WindowsUpdate struct {
	COM *COM
}

const wuClientID = "Axion Optimizer"

func (u *WindowsUpdate) com() *COM {
	if u.COM == nil {
		u.COM = &COM{}
	}
	return u.COM
}

// hresultOf extracts the HRESULT of a COM error (see sys.HResult).
func hresultOf(err error) uint32 { return sys.HResult(err) }

// comErr makes a COM error readable: the real HRESULT in hex plus the
// description, instead of "Exceção. (<nil>)".
func comErr(err error) error {
	if err == nil {
		return nil
	}
	var oe *ole.OleError
	if !errors.As(err, &oe) {
		return err
	}
	hr := hresultOf(err)
	desc := strings.TrimSpace(oe.Description())
	if desc == "" || desc == "<nil>" {
		desc = ""
		if hr>>16 == 0x8007 { // HRESULT_FROM_WIN32
			desc = windows.Errno(hr & 0xFFFF).Error()
		}
	}
	// Keep the context added around the COM error ("registrar tarefa: ...").
	prefix := strings.TrimSuffix(err.Error(), oe.Error())
	if desc == "" {
		return fmt.Errorf("%serro COM 0x%08X", prefix, hr)
	}
	return fmt.Errorf("%s%s (0x%08X)", prefix, desc, hr)
}

// wuErr turns a COM error into a readable one.
func wuErr(what string, err error) error {
	if err == nil {
		return nil
	}
	var oe *ole.OleError
	if errors.As(err, &oe) {
		// IDispatch exceptions carry the real HRESULT in the EXCEPINFO.
		if hr := uint32(oe.Code()); hr != 0 {
			if ex, ok := oe.SubError().(ole.EXCEPINFO); ok && ex.SCODE() != 0 {
				hr = ex.SCODE()
			}
			return fmt.Errorf("%s: %s", what, sys.WUError(hr))
		}
	}
	return fmt.Errorf("%s: %w", what, err)
}

func newWUSession() (*ole.IDispatch, error) {
	unk, err := oleutil.CreateObject("Microsoft.Update.Session")
	if err != nil {
		return nil, wuErr("Windows Update indisponível", err)
	}
	defer unk.Release()
	s, err := unk.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, err
	}
	_, _ = oleutil.PutProperty(s, "ClientApplicationID", wuClientID)
	return s, nil
}

func propDisp(d *ole.IDispatch, name string, params ...any) (*ole.IDispatch, error) {
	v, err := oleutil.GetProperty(d, name, params...)
	if err != nil {
		return nil, err
	}
	if v.VT != ole.VT_DISPATCH || v.Val == 0 {
		_ = v.Clear()
		return nil, fmt.Errorf("%s: objeto vazio", name)
	}
	return v.ToIDispatch(), nil // the VARIANT's reference now belongs to the caller
}

func propStr(d *ole.IDispatch, name string) string {
	v, err := oleutil.GetProperty(d, name)
	if err != nil {
		return ""
	}
	defer v.Clear()
	if v.VT == ole.VT_BSTR {
		return strings.TrimSpace(v.ToString())
	}
	return ""
}

func propBool(d *ole.IDispatch, name string) bool {
	v, err := oleutil.GetProperty(d, name)
	if err != nil {
		return false
	}
	defer v.Clear()
	b, _ := v.Value().(bool)
	return b
}

func propInt(d *ole.IDispatch, name string) int64 {
	v, err := oleutil.GetProperty(d, name)
	if err != nil {
		return 0
	}
	defer v.Clear()
	return variantInt(v)
}

func variantInt(v *ole.VARIANT) int64 {
	switch v.VT {
	case ole.VT_DECIMAL:
		raw := (*[16]byte)(unsafe.Pointer(v))
		n, err := sys.DecodeDecimal(raw[:])
		if err != nil {
			return 0
		}
		return n
	case ole.VT_R8:
		return int64(math.Float64frombits(uint64(v.Val)))
	case ole.VT_R4:
		return int64(math.Float32frombits(uint32(v.Val)))
	}
	switch x := v.Value().(type) {
	case int8:
		return int64(x)
	case uint8:
		return int64(x)
	case int16:
		return int64(x)
	case uint16:
		return int64(x)
	case int32:
		return int64(x)
	case uint32:
		return int64(x)
	case int64:
		return x
	case uint64:
		if x > math.MaxInt64 {
			return math.MaxInt64
		}
		return int64(x)
	case int:
		return int64(x)
	case uint:
		return int64(x)
	}
	return 0
}

func propDate(d *ole.IDispatch, name string) time.Time {
	v, err := oleutil.GetProperty(d, name)
	if err != nil {
		return time.Time{}
	}
	defer v.Clear()
	if v.VT == ole.VT_DATE {
		return sys.OLEDate(math.Float64frombits(uint64(v.Val)))
	}
	return time.Time{}
}

// search runs IUpdateSearcher.Search and returns the IUpdateCollection.
// Hidden updates are only returned with IncludePotentiallySupersededUpdates
// (IUpdateSearcher::Search documentation, IsHidden criterion).
func search(session *ole.IDispatch, criteria string, online, withHidden bool) (*ole.IDispatch, error) {
	raw, err := oleutil.CallMethod(session, "CreateUpdateSearcher")
	if err != nil {
		return nil, wuErr("criar a busca", err)
	}
	searcher := raw.ToIDispatch()
	defer searcher.Release()
	_, _ = oleutil.PutProperty(searcher, "Online", online)
	if withHidden {
		_, _ = oleutil.PutProperty(searcher, "IncludePotentiallySupersededUpdates", true)
	}
	res, err := oleutil.CallMethod(searcher, "Search", criteria)
	if err != nil {
		return nil, wuErr("buscar no Windows Update", err)
	}
	result := res.ToIDispatch()
	defer result.Release()
	if code := propInt(result, "ResultCode"); code == sys.WUFailed || code == sys.WUAborted {
		return nil, fmt.Errorf("a busca no Windows Update terminou com o código %d", code)
	}
	return propDisp(result, "Updates")
}

func forEachUpdate(coll *ole.IDispatch, fn func(u *ole.IDispatch) error) error {
	n := int(propInt(coll, "Count"))
	for i := 0; i < n; i++ {
		u, err := propDisp(coll, "Item", i)
		if err != nil {
			return err
		}
		err = fn(u)
		u.Release()
		if err != nil {
			return err
		}
	}
	return nil
}

func updateID(u *ole.IDispatch) (string, int) {
	id, err := propDisp(u, "Identity")
	if err != nil {
		return "", 0
	}
	defer id.Release()
	return strings.ToLower(propStr(id, "UpdateID")), int(propInt(id, "RevisionNumber"))
}

func readDriverUpdate(u *ole.IDispatch) sys.DriverUpdate {
	id, rev := updateID(u)
	d := sys.DriverUpdate{
		UpdateID: id, Revision: rev,
		Title:        propStr(u, "Title"),
		Description:  propStr(u, "Description"),
		Class:        propStr(u, "DriverClass"),
		HardwareID:   propStr(u, "DriverHardwareID"),
		Manufacturer: propStr(u, "DriverManufacturer"),
		Model:        propStr(u, "DriverModel"),
		Provider:     propStr(u, "DriverProvider"),
		VerDate:      propDate(u, "DriverVerDate"),
		SizeBytes:    propInt(u, "MaxDownloadSize"),
		Downloaded:   propBool(u, "IsDownloaded"),
		Hidden:       propBool(u, "IsHidden"),
		BrowseOnly:   propBool(u, "BrowseOnly"),
		SupportURL:   propStr(u, "SupportUrl"),
	}
	if ib, err := propDisp(u, "InstallationBehavior"); err == nil {
		d.RebootBehavior = int(propInt(ib, "RebootBehavior"))
		ib.Release()
	}
	return d
}

// SearchDrivers implements sys.Updates: an online search for the drivers
// Windows offers, then the hidden ones from the refreshed local cache.
func (u *WindowsUpdate) SearchDrivers(ctx context.Context) ([]sys.DriverUpdate, error) {
	var out []sys.DriverUpdate
	err := u.com().Do(10*time.Minute, func() error {
		s, err := newWUSession()
		if err != nil {
			return err
		}
		defer s.Release()
		seen := map[string]bool{}
		collect := func(coll *ole.IDispatch) error {
			defer coll.Release()
			return forEachUpdate(coll, func(up *ole.IDispatch) error {
				if d := readDriverUpdate(up); d.UpdateID != "" && !seen[d.UpdateID] {
					seen[d.UpdateID] = true
					out = append(out, d)
				}
				return nil
			})
		}
		coll, err := search(s, "IsInstalled=0 and Type='Driver' and IsHidden=0", true, false)
		if err != nil {
			return err
		}
		if err := collect(coll); err != nil {
			return err
		}
		// Hidden ones: best effort, the list above is what matters.
		if hidden, err := search(s, "IsInstalled=0 and Type='Driver' and IsHidden=1", false, true); err == nil {
			_ = collect(hidden)
		}
		return nil
	})
	return out, err
}

// pick builds an IUpdateCollection with the requested updates, searching
// by UpdateID in the local cache first and online when something is missing.
func pick(session *ole.IDispatch, ids []string, prepare func(u *ole.IDispatch) error) (coll *ole.IDispatch, titles map[string]string, order []string, err error) {
	want := map[string]bool{}
	var terms []string
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if !validGUID(id) || want[id] {
			continue
		}
		want[id] = true
		terms = append(terms, "UpdateID='"+id+"'")
	}
	if len(terms) == 0 {
		return nil, nil, nil, errors.New("nenhuma atualização válida escolhida")
	}
	criteria := strings.Join(terms, " or ")
	for _, online := range []bool{false, true} {
		found, err := search(session, criteria, online, true)
		if err != nil {
			if online {
				return nil, nil, nil, err
			}
			continue
		}
		unk, err := oleutil.CreateObject("Microsoft.Update.UpdateColl")
		if err != nil {
			found.Release()
			return nil, nil, nil, wuErr("criar a lista de atualizações", err)
		}
		c, err := unk.QueryInterface(ole.IID_IDispatch)
		unk.Release()
		if err != nil {
			found.Release()
			return nil, nil, nil, err
		}
		titles = map[string]string{}
		order = nil
		err = forEachUpdate(found, func(up *ole.IDispatch) error {
			id, _ := updateID(up)
			if !want[id] || titles[id] != "" {
				return nil
			}
			if prepare != nil {
				if err := prepare(up); err != nil {
					return err
				}
			}
			if _, err := oleutil.CallMethod(c, "Add", up); err != nil {
				return wuErr("montar a lista", err)
			}
			titles[id] = propStr(up, "Title")
			if titles[id] == "" {
				titles[id] = id
			}
			order = append(order, id)
			return nil
		})
		found.Release()
		if err != nil {
			c.Release()
			return nil, nil, nil, err
		}
		if len(order) == len(want) || online {
			if len(order) == 0 {
				c.Release()
				return nil, nil, nil, errors.New("as atualizações escolhidas não são mais oferecidas pelo Windows Update; procure de novo")
			}
			return c, titles, order, nil
		}
		c.Release()
	}
	return nil, nil, nil, errors.New("atualizações não encontradas")
}

func validGUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
				return false
			}
		}
	}
	return true
}

func acceptEula(up *ole.IDispatch) error {
	if !propBool(up, "EulaAccepted") {
		if _, err := oleutil.CallMethod(up, "AcceptEula"); err != nil {
			return wuErr("aceitar a licença do driver", err)
		}
	}
	return nil
}

// results reads GetUpdateResult(i) for each update of a job result.
func results(res *ole.IDispatch, order []string, titles map[string]string, withReboot bool) []sys.UpdateResult {
	out := make([]sys.UpdateResult, 0, len(order))
	for i, id := range order {
		r := sys.UpdateResult{UpdateID: id, Title: titles[id], Code: sys.WUFailed}
		if raw, err := oleutil.CallMethod(res, "GetUpdateResult", i); err == nil {
			ur := raw.ToIDispatch()
			r.Code = int(propInt(ur, "ResultCode"))
			r.HResult = int32(propInt(ur, "HResult"))
			if withReboot {
				r.RebootRequired = propBool(ur, "RebootRequired")
			}
			ur.Release()
		}
		out = append(out, r)
	}
	return out
}

// DownloadDrivers implements sys.Updates.
func (u *WindowsUpdate) DownloadDrivers(ctx context.Context, ids []string) ([]sys.UpdateResult, error) {
	var out []sys.UpdateResult
	err := u.com().Do(90*time.Minute, func() error {
		s, err := newWUSession()
		if err != nil {
			return err
		}
		defer s.Release()
		coll, titles, order, err := pick(s, ids, acceptEula)
		if err != nil {
			return err
		}
		defer coll.Release()
		raw, err := oleutil.CallMethod(s, "CreateUpdateDownloader")
		if err != nil {
			return wuErr("criar o download", err)
		}
		dl := raw.ToIDispatch()
		defer dl.Release()
		if _, err := oleutil.PutProperty(dl, "Updates", coll); err != nil {
			return wuErr("preparar o download", err)
		}
		_, _ = oleutil.PutProperty(dl, "Priority", 3) // dpHigh: the user is waiting
		res, err := oleutil.CallMethod(dl, "Download")
		if err != nil {
			return wuErr("baixar", err)
		}
		r := res.ToIDispatch()
		defer r.Release()
		out = results(r, order, titles, false)
		return nil
	})
	return out, err
}

// InstallDrivers implements sys.Updates.
func (u *WindowsUpdate) InstallDrivers(ctx context.Context, ids []string) ([]sys.UpdateResult, bool, error) {
	var out []sys.UpdateResult
	reboot := false
	err := u.com().Do(60*time.Minute, func() error {
		s, err := newWUSession()
		if err != nil {
			return err
		}
		defer s.Release()
		coll, titles, order, err := pick(s, ids, func(up *ole.IDispatch) error {
			if !propBool(up, "IsDownloaded") {
				return errors.New("\"" + propStr(up, "Title") + "\" ainda não foi baixada")
			}
			return acceptEula(up)
		})
		if err != nil {
			return err
		}
		defer coll.Release()
		raw, err := oleutil.CallMethod(s, "CreateUpdateInstaller")
		if err != nil {
			return wuErr("criar a instalação", err)
		}
		inst := raw.ToIDispatch()
		defer inst.Release()
		if propBool(inst, "IsBusy") {
			return errors.New("o Windows Update já está instalando outra coisa; tente de novo quando terminar")
		}
		if propBool(inst, "RebootRequiredBeforeInstallation") {
			return errors.New("o Windows precisa reiniciar antes de instalar novas atualizações")
		}
		_, _ = oleutil.PutProperty(inst, "AllowSourcePrompts", false)
		if _, err := oleutil.PutProperty(inst, "Updates", coll); err != nil {
			return wuErr("preparar a instalação", err)
		}
		res, err := oleutil.CallMethod(inst, "Install")
		if err != nil {
			return wuErr("instalar", err)
		}
		r := res.ToIDispatch()
		defer r.Release()
		out = results(r, order, titles, true)
		reboot = propBool(r, "RebootRequired")
		return nil
	})
	return out, reboot, err
}

// SetHidden implements sys.Updates.
func (u *WindowsUpdate) SetHidden(ctx context.Context, id string, hidden bool) error {
	return u.com().Do(10*time.Minute, func() error {
		s, err := newWUSession()
		if err != nil {
			return err
		}
		defer s.Release()
		coll, _, _, err := pick(s, []string{id}, func(up *ole.IDispatch) error {
			if _, err := oleutil.PutProperty(up, "IsHidden", hidden); err != nil {
				return wuErr("ocultar a atualização", err)
			}
			return nil
		})
		if err != nil {
			return err
		}
		coll.Release()
		return nil
	})
}

var _ sys.Updates = (*WindowsUpdate)(nil)

//go:build windows

package winsys

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"axion/internal/sys"
)

// NVIDIA driver profiles through nvapi64.dll (System32 only, the DLL the
// NVIDIA Control Panel and the games themselves load). The DLL exports one
// function, nvapi_QueryInterface(id), that returns the pointer of each
// documented function (0 when the driver does not have it). No cgo, no
// kernel driver, no other process touched: the profile database is changed
// only by the driver's own public API (SetSetting, DeleteProfileSetting,
// RestoreProfileDefaultSetting + SaveSettings), as the Control Panel does.
//
// Rules of the unsafe package: every uintptr(unsafe.Pointer(x)) conversion
// stays inside the syscall.SyscallN call, so nvSt wraps the returned values
// instead of a helper that receives converted pointers.
var (
	modNvapi             = windows.NewLazySystemDLL("nvapi64.dll")
	procNvQueryInterface = modNvapi.NewProc("nvapi_QueryInterface")
	nvOnce               sync.Once
	nvFns                nvapiFuncs
	nvErr                error
	nvMu                 sync.Mutex // one DRS session at a time in the process
)

type nvapiFuncs struct {
	createSession, destroySession, loadSettings, saveSettings, getBase, getGlobal, findApp, getSetting, setSetting,
	restoreDefault, deleteSetting, getProfileInfo, enumSettings, getDisplayID, getVRR uintptr
}

// nvSt extracts the NvAPI_Status from the values SyscallN returns.
func nvSt(r1, _ uintptr, _ syscall.Errno) int32 { return int32(uint32(r1)) }

// nvapiLoad initializes NVAPI once per process (NvAPI_Initialize) and
// resolves the DRS functions. NvAPI_Unload is never called: the process
// keeps the DLL until it exits.
func nvapiLoad() (*nvapiFuncs, error) {
	nvOnce.Do(func() {
		if runtime.GOARCH != "amd64" {
			nvErr = sys.ErrNoNVAPI // a 32-bit build would need nvapi.dll
			return
		}
		if procNvQueryInterface.Find() != nil {
			nvErr = sys.ErrNoNVAPI // no NVIDIA driver on this PC
			return
		}
		q := func(id uint32) uintptr {
			r, _, _ := procNvQueryInterface.Call(uintptr(id))
			return r
		}
		fnInit := q(nvidInitialize)
		if fnInit == 0 {
			nvErr = sys.ErrNoNVAPI
			return
		}
		if st := nvSt(syscall.SyscallN(fnInit)); st != sys.NVAPIOK {
			nvErr = &sys.NVAPIError{Func: "NvAPI_Initialize", Status: st}
			return
		}
		f := &nvFns
		f.createSession, f.destroySession, f.loadSettings, f.saveSettings = q(nvidCreateSession), q(nvidDestroySession), q(nvidLoadSettings), q(nvidSaveSettings)
		f.getBase, f.getGlobal, f.findApp = q(nvidGetBaseProfile), q(nvidGetCurrentGlobalProfile), q(nvidFindApplicationByName)
		f.getSetting, f.setSetting, f.restoreDefault, f.deleteSetting = q(nvidGetSetting), q(nvidSetSetting), q(nvidRestoreProfileDefaultSetting), q(nvidDeleteProfileSetting)
		f.getProfileInfo, f.enumSettings = q(nvidGetProfileInfo), q(nvidEnumSettings)
		// Missing in drivers before R525: only the VRR stays unknown.
		f.getDisplayID, f.getVRR = q(nvidGetDisplayIdByDisplayName), q(nvidGetVRRInfo)
		for _, p := range []uintptr{f.createSession, f.destroySession, f.loadSettings, f.saveSettings, f.getBase, f.getGlobal, f.findApp,
			f.getSetting, f.setSetting, f.restoreDefault, f.deleteSetting, f.getProfileInfo, f.enumSettings} {
			if p == 0 {
				nvErr = &sys.NVAPIError{Func: "NvAPI_DRS", Status: sys.NVAPINoImplementation}
				return
			}
		}
	})
	if nvErr != nil {
		return nil, nvErr
	}
	return &nvFns, nil
}

// session runs fn inside CreateSession → LoadSettings → fn → DestroySession.
// The session is always destroyed, even when fn fails.
func (f *nvapiFuncs) session(fn func(h uintptr) error) error {
	nvMu.Lock()
	defer nvMu.Unlock()
	h := new(uintptr)
	if st := nvSt(syscall.SyscallN(f.createSession, uintptr(unsafe.Pointer(h)))); st != sys.NVAPIOK {
		return &sys.NVAPIError{Func: "NvAPI_DRS_CreateSession", Status: st}
	}
	defer syscall.SyscallN(f.destroySession, *h)
	if st := nvSt(syscall.SyscallN(f.loadSettings, *h)); st != sys.NVAPIOK {
		return &sys.NVAPIError{Func: "NvAPI_DRS_LoadSettings", Status: st}
	}
	return fn(*h)
}

// profile resolves a ref to a profile handle; found=false without an error
// when the profile or the game does not exist in the database.
func (f *nvapiFuncs) profile(h uintptr, ref string) (uintptr, bool, error) {
	p := new(uintptr)
	switch {
	case ref == sys.NVDRSBase:
		if st := nvSt(syscall.SyscallN(f.getBase, h, uintptr(unsafe.Pointer(p)))); st != sys.NVAPIOK {
			return 0, false, &sys.NVAPIError{Func: "NvAPI_DRS_GetBaseProfile", Status: st}
		}
	case ref == sys.NVDRSGlobal:
		if st := nvSt(syscall.SyscallN(f.getGlobal, h, uintptr(unsafe.Pointer(p)))); st != sys.NVAPIOK {
			return 0, false, &sys.NVAPIError{Func: "NvAPI_DRS_GetCurrentGlobalProfile", Status: st}
		}
	case strings.HasPrefix(ref, sys.NVDRSAppPrefix):
		exe := strings.TrimPrefix(ref, sys.NVDRSAppPrefix)
		u, err := windows.UTF16FromString(exe)
		if err != nil || len(u) >= 2048 || exe == "" {
			return 0, false, fmt.Errorf("nome de executável inválido: %q", exe)
		}
		name := new([2048]uint16)
		copy(name[:], u)
		app := &nvdrsApplication{Version: nvdrsApplicationVer}
		st := nvSt(syscall.SyscallN(f.findApp, h, uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(app))))
		switch st {
		case sys.NVAPIOK:
		case sys.NVAPIExecutableNotFound, sys.NVAPIProfileNotFound, sys.NVAPIPathAmbiguous:
			return 0, false, nil
		default:
			return 0, false, &sys.NVAPIError{Func: "NvAPI_DRS_FindApplicationByName", Status: st}
		}
	default:
		return 0, false, fmt.Errorf("perfil desconhecido: %q", ref)
	}
	return *p, *p != 0, nil
}

// get reads one setting; NVAPI_SETTING_NOT_FOUND is "not in this profile",
// not an error.
func (f *nvapiFuncs) get(h, p uintptr, ref string, id uint32) (sys.NVDRSSetting, error) {
	s := &nvdrsSetting{Version: nvdrsSettingVer}
	st := nvSt(syscall.SyscallN(f.getSetting, h, p, uintptr(id), uintptr(unsafe.Pointer(s))))
	if st == sys.NVAPISettingNotFound {
		return sys.NVDRSSetting{ID: id}, nil
	}
	if st != sys.NVAPIOK {
		return sys.NVDRSSetting{ID: id}, &sys.NVAPIError{Func: "NvAPI_DRS_GetSetting", Status: st}
	}
	out := settingFromRaw(ref, s)
	out.ID = id
	return out, nil
}

// enumUser lists the settings of a profile that hold a user value. One call
// with room for every setting of the profile (the count GetProfileInfo
// reports, with a margin): paging by startIndex came back with overlapping
// pages on a real driver (R566), and nvidiaProfileInspector also reads the
// profile in one call. The capacity is capped at 512 (criterio do Axion).
func (f *nvapiFuncs) enumUser(h, p uintptr, ref string, numSettings uint32) ([]sys.NVDRSSetting, error) {
	capacity := numSettings + 16
	if capacity < 32 {
		capacity = 32
	}
	if capacity > 512 {
		capacity = 512
	}
	for {
		buf := make([]nvdrsSetting, capacity) // 12 KB each
		for i := range buf {
			buf[i] = nvdrsSetting{Version: nvdrsSettingVer}
		}
		n := new(uint32)
		*n = capacity
		st := nvSt(syscall.SyscallN(f.enumSettings, h, p, uintptr(0), uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(&buf[0]))))
		if st == sys.NVAPIEndEnumeration {
			return nil, nil
		}
		if st != sys.NVAPIOK {
			return nil, &sys.NVAPIError{Func: "NvAPI_DRS_EnumSettings", Status: st}
		}
		got := int(*n)
		if got > int(capacity) {
			got = int(capacity)
		}
		if got == int(capacity) && capacity < 512 {
			capacity = 512 // the profile grew past the count reported: read it whole
			continue
		}
		var out []sys.NVDRSSetting
		seen := map[uint32]bool{}
		for i := 0; i < got; i++ {
			if s := settingFromRaw(ref, &buf[i]); s.User() && !seen[s.ID] {
				seen[s.ID] = true
				out = append(out, s)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out, nil
	}
}

// NVDRSProfiles implements sys.Native: every ref is read in one DRS session.
func (n *Native) NVDRSProfiles(refs []string, ids []uint32, withUser bool) ([]sys.NVDRSProfile, error) {
	out := make([]sys.NVDRSProfile, 0, len(refs))
	f, err := nvapiLoad()
	if err != nil {
		return out, err
	}
	err = f.session(func(h uintptr) error {
		for _, ref := range refs {
			prof := sys.NVDRSProfile{Ref: ref}
			p, found, err := f.profile(h, ref)
			if err != nil {
				return err
			}
			if !found {
				out = append(out, prof)
				continue
			}
			prof.Found = true
			info := &nvdrsProfileInfo{Version: nvdrsProfileVer}
			if st := nvSt(syscall.SyscallN(f.getProfileInfo, h, p, uintptr(unsafe.Pointer(info)))); st == sys.NVAPIOK {
				prof.Name = windows.UTF16ToString(info.ProfileName[:])
				prof.Predefined = info.IsPredefined != 0
			}
			for _, id := range ids {
				s, err := f.get(h, p, ref, id)
				if err != nil {
					return err
				}
				prof.Settings = append(prof.Settings, s)
			}
			if withUser {
				if prof.User, err = f.enumUser(h, p, ref, info.NumOfSettings); err != nil {
					return err
				}
			}
			out = append(out, prof)
		}
		return nil
	})
	return out, err
}

// nvdrsWrite opens a session, resolves the profile, runs op and saves.
func (n *Native) nvdrsWrite(ref, fn string, op func(f *nvapiFuncs, h, p uintptr) int32) error {
	f, err := nvapiLoad()
	if err != nil {
		return err
	}
	return f.session(func(h uintptr) error {
		p, found, err := f.profile(h, ref)
		if err != nil {
			return err
		}
		if !found {
			return &sys.NVAPIError{Func: fn, Status: sys.NVAPIProfileNotFound}
		}
		if st := op(f, h, p); st != sys.NVAPIOK && st != sys.NVAPISettingNotFound {
			return &sys.NVAPIError{Func: fn, Status: st}
		}
		if st := nvSt(syscall.SyscallN(f.saveSettings, h)); st != sys.NVAPIOK {
			return &sys.NVAPIError{Func: "NvAPI_DRS_SaveSettings", Status: st}
		}
		return nil
	})
}

// NVDRSSet implements sys.Native (DWORD values only).
func (n *Native) NVDRSSet(ref string, id, value uint32) error {
	return n.nvdrsWrite(ref, "NvAPI_DRS_SetSetting", func(f *nvapiFuncs, h, p uintptr) int32 {
		s := &nvdrsSetting{Version: nvdrsSettingVer, SettingID: id, SettingType: sys.NVDRSTypeDWORD, SettingLocation: sys.NVDRSLocCurrent}
		binary.LittleEndian.PutUint32(s.Current[:4], value)
		st := nvSt(syscall.SyscallN(f.setSetting, h, p, uintptr(unsafe.Pointer(s))))
		if st == sys.NVAPISettingNotFound {
			return sys.NVAPIGenericError // in a Set, "not found" is an error
		}
		return st
	})
}

// NVDRSRestoreDefault implements sys.Native.
func (n *Native) NVDRSRestoreDefault(ref string, id uint32) error {
	return n.nvdrsWrite(ref, "NvAPI_DRS_RestoreProfileDefaultSetting", func(f *nvapiFuncs, h, p uintptr) int32 {
		return nvSt(syscall.SyscallN(f.restoreDefault, h, p, uintptr(id)))
	})
}

// NVDRSDelete implements sys.Native.
func (n *Native) NVDRSDelete(ref string, id uint32) error {
	return n.nvdrsWrite(ref, "NvAPI_DRS_DeleteProfileSetting", func(f *nvapiFuncs, h, p uintptr) int32 {
		return nvSt(syscall.SyscallN(f.deleteSetting, h, p, uintptr(id)))
	})
}

// NVVRR implements sys.Native.
func (n *Native) NVVRR(device string) (sys.NVVRRInfo, error) {
	out := sys.NVVRRInfo{Device: device}
	f, err := nvapiLoad()
	if err != nil {
		return out, err
	}
	if f.getDisplayID == 0 || f.getVRR == 0 {
		return out, &sys.NVAPIError{Func: "NvAPI_Disp_GetVRRInfo", Status: sys.NVAPINoImplementation}
	}
	nvMu.Lock()
	defer nvMu.Unlock()
	name, err := windows.BytePtrFromString(device) // ANSI const char*, e.g. \\.\DISPLAY1
	if err != nil {
		return out, err
	}
	id := new(uint32)
	if st := nvSt(syscall.SyscallN(f.getDisplayID, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(id)))); st != sys.NVAPIOK {
		return out, &sys.NVAPIError{Func: "NvAPI_DISP_GetDisplayIdByDisplayName", Status: st}
	}
	info := &nvVRRInfo{Version: nvVRRInfoVer}
	if st := nvSt(syscall.SyscallN(f.getVRR, uintptr(*id), uintptr(unsafe.Pointer(info)))); st != sys.NVAPIOK {
		return out, &sys.NVAPIError{Func: "NvAPI_Disp_GetVRRInfo", Status: st}
	}
	out.Read = true
	out.Enabled, out.Possible, out.Requested, out.InVRRMode = info.Flags&1 != 0, info.Flags&2 != 0, info.Flags&4 != 0, info.Flags&16 != 0
	return out, nil
}

//go:build windows

package winsys

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"axion/internal/sys"
)

var (
	modPowrprof                  = windows.NewLazySystemDLL("powrprof.dll")
	procPowerGetActiveScheme     = modPowrprof.NewProc("PowerGetActiveScheme")
	procPowerSetActiveScheme     = modPowrprof.NewProc("PowerSetActiveScheme")
	procPowerEnumerate           = modPowrprof.NewProc("PowerEnumerate")
	procPowerReadFriendlyName    = modPowrprof.NewProc("PowerReadFriendlyName")
	procPowerReadACValueIndex    = modPowrprof.NewProc("PowerReadACValueIndex")
	procPowerReadDCValueIndex    = modPowrprof.NewProc("PowerReadDCValueIndex")
	procPowerWriteACValueIndex   = modPowrprof.NewProc("PowerWriteACValueIndex")
	procPowerWriteDCValueIndex   = modPowrprof.NewProc("PowerWriteDCValueIndex")
	procPowerDuplicateScheme     = modPowrprof.NewProc("PowerDuplicateScheme")
	procPowerDeleteScheme        = modPowrprof.NewProc("PowerDeleteScheme")
	procPowerGetEffectiveOverlay = modPowrprof.NewProc("PowerGetEffectiveOverlayScheme")
	procPowerGetActualOverlay    = modPowrprof.NewProc("PowerGetActualOverlayScheme")
	procPowerSetActiveOverlay    = modPowrprof.NewProc("PowerSetActiveOverlayScheme")
	procLocalFree                = windows.NewLazySystemDLL("kernel32.dll").NewProc("LocalFree")
)

const (
	accessScheme       = 16 // ACCESS_SCHEME
	errorNoMoreItems   = 259
	powerSchemesKey    = `HKLM\SYSTEM\CurrentControlSet\Control\Power\User\PowerSchemes`
	overlayACValueName = "ActiveOverlayAcPowerScheme"
)

// Power implements sys.Power with powrprof.dll.
type Power struct{ Reg sys.Registry }

func parseGUID(s string) (windows.GUID, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		s = "{" + s + "}"
	}
	return windows.GUIDFromString(s)
}

func guidString(g windows.GUID) string {
	return strings.ToLower(strings.Trim(g.String(), "{}"))
}

func powerErr(fn string, r uintptr) error {
	if r == 0 {
		return nil
	}
	return fmt.Errorf("%s: %w", fn, syscall.Errno(r))
}

// ActiveScheme implements sys.Power.
func (Power) ActiveScheme() (string, error) {
	var p *windows.GUID
	r, _, _ := procPowerGetActiveScheme.Call(0, uintptr(unsafe.Pointer(&p)))
	if err := powerErr("PowerGetActiveScheme", r); err != nil {
		return "", err
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(p)))
	return guidString(*p), nil
}

// SetActiveScheme implements sys.Power.
func (Power) SetActiveScheme(guid string) error {
	g, err := parseGUID(guid)
	if err != nil {
		return err
	}
	r, _, _ := procPowerSetActiveScheme.Call(0, uintptr(unsafe.Pointer(&g)))
	return powerErr("PowerSetActiveScheme", r)
}

// Schemes implements sys.Power.
func (Power) Schemes() ([]sys.PowerScheme, error) {
	var out []sys.PowerScheme
	for i := uint32(0); i < 64; i++ {
		var g windows.GUID
		size := uint32(unsafe.Sizeof(g))
		r, _, _ := procPowerEnumerate.Call(0, 0, 0, accessScheme, uintptr(i), uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&size)))
		if r == errorNoMoreItems {
			break
		}
		if r != 0 {
			return out, powerErr("PowerEnumerate", r)
		}
		out = append(out, sys.PowerScheme{GUID: guidString(g), Name: friendlyName(&g)})
	}
	return out, nil
}

func friendlyName(g *windows.GUID) string {
	var size uint32
	procPowerReadFriendlyName.Call(0, uintptr(unsafe.Pointer(g)), 0, 0, 0, uintptr(unsafe.Pointer(&size)))
	if size == 0 || size > 4096 {
		return guidString(*g)
	}
	buf := make([]uint16, size/2+1)
	r, _, _ := procPowerReadFriendlyName.Call(0, uintptr(unsafe.Pointer(g)), 0, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r != 0 {
		return guidString(*g)
	}
	return windows.UTF16ToString(buf)
}

func (Power) guids(scheme, sub, setting string) (s, sb, st windows.GUID, err error) {
	if s, err = parseGUID(scheme); err != nil {
		return
	}
	if sb, err = parseGUID(sub); err != nil {
		return
	}
	st, err = parseGUID(setting)
	return
}

// ReadValue implements sys.Power.
func (p Power) ReadValue(scheme, sub, setting string, ac bool) (uint32, error) {
	s, sb, st, err := p.guids(scheme, sub, setting)
	if err != nil {
		return 0, err
	}
	proc := procPowerReadDCValueIndex
	if ac {
		proc = procPowerReadACValueIndex
	}
	var v uint32
	r, _, _ := proc.Call(0, uintptr(unsafe.Pointer(&s)), uintptr(unsafe.Pointer(&sb)), uintptr(unsafe.Pointer(&st)), uintptr(unsafe.Pointer(&v)))
	if r == 2 { // ERROR_FILE_NOT_FOUND: setting not present in this scheme
		return 0, sys.ErrNotFound
	}
	return v, powerErr("PowerReadValueIndex", r)
}

// WriteValue implements sys.Power.
func (p Power) WriteValue(scheme, sub, setting string, ac bool, value uint32) error {
	s, sb, st, err := p.guids(scheme, sub, setting)
	if err != nil {
		return err
	}
	proc := procPowerWriteDCValueIndex
	if ac {
		proc = procPowerWriteACValueIndex
	}
	r, _, _ := proc.Call(0, uintptr(unsafe.Pointer(&s)), uintptr(unsafe.Pointer(&sb)), uintptr(unsafe.Pointer(&st)), uintptr(value))
	return powerErr("PowerWriteValueIndex", r)
}

// DuplicateScheme implements sys.Power.
func (Power) DuplicateScheme(source string) (string, error) {
	g, err := parseGUID(source)
	if err != nil {
		return "", err
	}
	var dst *windows.GUID
	r, _, _ := procPowerDuplicateScheme.Call(0, uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&dst)))
	if err := powerErr("PowerDuplicateScheme", r); err != nil {
		return "", err
	}
	if dst == nil {
		return "", errors.New("PowerDuplicateScheme não retornou o novo plano")
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(dst)))
	return guidString(*dst), nil
}

// DeleteScheme implements sys.Power.
func (Power) DeleteScheme(guid string) error {
	g, err := parseGUID(guid)
	if err != nil {
		return err
	}
	r, _, _ := procPowerDeleteScheme.Call(0, uintptr(unsafe.Pointer(&g)))
	return powerErr("PowerDeleteScheme", r)
}

// ActiveOverlay implements sys.Power (power mode on AC).
func (p Power) ActiveOverlay() (string, error) {
	for _, proc := range []*windows.LazyProc{procPowerGetEffectiveOverlay, procPowerGetActualOverlay} {
		if proc.Find() != nil {
			continue
		}
		var g windows.GUID
		r, _, _ := proc.Call(uintptr(unsafe.Pointer(&g)))
		if r == 0 {
			s := guidString(g)
			if s == sys.OverlayBalanced {
				return "", nil
			}
			return s, nil
		}
	}
	v, err := p.Reg.GetValue(powerSchemesKey, overlayACValueName)
	if err != nil || !v.Exists {
		return "", err
	}
	s := strings.ToLower(strings.Trim(v.Str, "{} "))
	if s == sys.OverlayBalanced {
		s = ""
	}
	return s, nil
}

// SetActiveOverlay implements sys.Power.
func (p Power) SetActiveOverlay(guid string) error {
	if guid == "" {
		guid = sys.OverlayBalanced
	}
	g, err := parseGUID(guid)
	if err != nil {
		return err
	}
	if procPowerSetActiveOverlay.Find() == nil {
		// GUID is passed by value; on x64 a 16-byte struct is passed by reference.
		// When the API exists, its answer is final: writing the registry value
		// instead would not switch the power mode and would report a success
		// that ActiveOverlay (API first) then contradicts.
		r, _, _ := procPowerSetActiveOverlay.Call(uintptr(unsafe.Pointer(&g)))
		return powerErr("PowerSetActiveOverlayScheme", r)
	}
	// Builds without the API (older Windows 10): the registry value is what
	// the power mode is read from there.
	return p.Reg.SetValue(powerSchemesKey, overlayACValueName, sys.SZ(guidString(g)))
}

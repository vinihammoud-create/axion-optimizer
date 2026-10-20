//go:build windows

package winsys

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procGetDisplayConfigBufferSizes = modUser32.NewProc("GetDisplayConfigBufferSizes")
	procQueryDisplayConfig          = modUser32.NewProc("QueryDisplayConfig")
	procDisplayConfigGetDeviceInfo  = modUser32.NewProc("DisplayConfigGetDeviceInfo")
)

const (
	qdcOnlyActivePaths        = 0x2
	dcDeviceInfoGetSourceName = 1
	dcDeviceInfoGetTargetName = 2
)

// displayTarget is what the display configuration says about the monitor
// behind a GDI device (\\.\DISPLAY1): its EDID name and connection.
type displayTarget struct {
	name     string
	internal bool
}

// displayTargets maps each active GDI device name (upper case) to its
// monitor. Any failure returns what was read so far (names are a bonus: the
// mode list does not depend on them).
func displayTargets() map[string]displayTarget {
	out := map[string]displayTarget{}
	if procGetDisplayConfigBufferSizes.Find() != nil || procQueryDisplayConfig.Find() != nil || procDisplayConfigGetDeviceInfo.Find() != nil {
		return out
	}
	var paths []dcPathInfo
	var np uint32
	for try := 0; try < 3; try++ {
		var nm uint32
		if r, _, _ := procGetDisplayConfigBufferSizes.Call(qdcOnlyActivePaths, uintptr(unsafe.Pointer(&np)), uintptr(unsafe.Pointer(&nm))); r != 0 || np == 0 {
			return out
		}
		paths = make([]dcPathInfo, np)
		modes := make([]dcModeInfo, nm+1)
		r, _, _ := procQueryDisplayConfig.Call(qdcOnlyActivePaths, uintptr(unsafe.Pointer(&np)), uintptr(unsafe.Pointer(&paths[0])),
			uintptr(unsafe.Pointer(&nm)), uintptr(unsafe.Pointer(&modes[0])), 0)
		if r == 0 {
			break
		}
		if r != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) || try == 2 {
			return out // the layout changed between the two calls: try again
		}
	}
	if int(np) < len(paths) {
		paths = paths[:np]
	}
	for i := range paths {
		p := &paths[i]
		src := dcSourceName{Header: dcHeader{Type: dcDeviceInfoGetSourceName, AdapterID: p.SourceAdapter, ID: p.SourceID}}
		src.Header.Size = uint32(unsafe.Sizeof(src))
		if r, _, _ := procDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&src))); r != 0 {
			continue
		}
		tgt := dcTargetName{Header: dcHeader{Type: dcDeviceInfoGetTargetName, AdapterID: p.TargetAdapter, ID: p.TargetID}}
		tgt.Header.Size = uint32(unsafe.Sizeof(tgt))
		if r, _, _ := procDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&tgt))); r != 0 {
			continue
		}
		gdi := strings.ToUpper(windows.UTF16ToString(src.ViewGdiDeviceName[:]))
		if _, dup := out[gdi]; dup || gdi == "" {
			continue // a cloned display: the first monitor names it
		}
		out[gdi] = displayTarget{
			name:     strings.TrimSpace(windows.UTF16ToString(tgt.MonitorFriendlyDeviceName[:])),
			internal: internalOutput(tgt.OutputTechnology) || internalOutput(p.OutputTechnology),
		}
	}
	return out
}

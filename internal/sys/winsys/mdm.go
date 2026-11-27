//go:build windows

package winsys

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modMDMRegistration                   = windows.NewLazySystemDLL("mdmregistration.dll")
	procIsDeviceRegisteredWithManagement = modMDMRegistration.NewProc("IsDeviceRegisteredWithManagement")
)

// MDMRegistered implements sys.Native with IsDeviceRegisteredWithManagement
// (mdmregistration.h, Windows 8.1 or newer): "Checks whether the device is
// registered with an MDM service". With pszUPN NULL the function only fills
// the BOOL and returns ERROR_SUCCESS; any other return, or a DLL that is not
// there (Windows Server), is an error, never "not registered".
func (n *Native) MDMRegistered() (bool, error) {
	if err := procIsDeviceRegisteredWithManagement.Find(); err != nil {
		return false, err
	}
	var registered int32
	hr, _, _ := procIsDeviceRegisteredWithManagement.Call(uintptr(unsafe.Pointer(&registered)), 0, 0)
	if hr != 0 {
		return false, fmt.Errorf("IsDeviceRegisteredWithManagement: 0x%08X", uint32(hr))
	}
	return registered != 0, nil
}

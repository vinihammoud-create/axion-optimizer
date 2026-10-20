//go:build windows

package winsys

import (
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modComdlg32              = windows.NewLazySystemDLL("comdlg32.dll")
	procGetOpenFileNameW     = modComdlg32.NewProc("GetOpenFileNameW")
	procCommDlgExtendedError = modComdlg32.NewProc("CommDlgExtendedError")
	procGetFirmwareType      = modKernel32.NewProc("GetFirmwareType")
)

// OwnerWindow is the main window handle, used as owner of native dialogs.
var OwnerWindow uintptr

type openFileNameW struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	Flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	FlagsEx           uint32
}

const (
	ofnHideReadOnly    = 0x00000004
	ofnNoChangeDir     = 0x00000008
	ofnPathMustExist   = 0x00000800
	ofnFileMustExist   = 0x00001000
	ofnExplorer        = 0x00080000
	ofnDontAddToRecent = 0x02000000
)

// PickFile implements sys.Native with GetOpenFileNameW on a dedicated STA thread.
func (n *Native) PickFile(title, filterName, pattern string) (string, error) {
	type result struct {
		path string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		const coinitApartmentThreaded = 0x2
		_ = windows.CoInitializeEx(0, coinitApartmentThreaded)
		defer windows.CoUninitialize()

		filter := make([]uint16, 0, 128)
		for _, s := range []string{filterName, pattern} {
			u, _ := windows.UTF16FromString(s)
			filter = append(filter, u...) // includes the terminating NUL
		}
		filter = append(filter, 0)
		buf := make([]uint16, 1024)
		t, _ := windows.UTF16PtrFromString(title)
		ofn := openFileNameW{
			hwndOwner:   OwnerWindow,
			lpstrFilter: &filter[0],
			lpstrFile:   &buf[0],
			nMaxFile:    uint32(len(buf)),
			lpstrTitle:  t,
			Flags:       ofnExplorer | ofnFileMustExist | ofnPathMustExist | ofnHideReadOnly | ofnNoChangeDir | ofnDontAddToRecent,
		}
		ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
		r, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
		if r == 0 {
			code, _, _ := procCommDlgExtendedError.Call()
			if code == 0 {
				ch <- result{} // cancelled
				return
			}
			ch <- result{err: errors.New("falha ao abrir a janela de seleção de arquivo")}
			return
		}
		ch <- result{path: windows.UTF16ToString(buf)}
	}()
	res := <-ch
	return res.path, res.err
}

// FirmwareType implements sys.Native.
func (n *Native) FirmwareType() string {
	if procGetFirmwareType.Find() != nil {
		return ""
	}
	var t uint32
	r, _, _ := procGetFirmwareType.Call(uintptr(unsafe.Pointer(&t)))
	if r == 0 {
		return ""
	}
	switch t {
	case 1:
		return "BIOS"
	case 2:
		return "UEFI"
	}
	return ""
}

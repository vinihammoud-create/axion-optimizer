//go:build windows

package winsys

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procShellExecuteExW = modShell32.NewProc("ShellExecuteExW")

// shellExecuteInfo is SHELLEXECUTEINFOW (112 bytes on x64).
type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     windows.Handle
}

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	seeMaskFlagNoUI       = 0x00000400
)

// RunVisible implements sys.Native: ShellExecuteEx with the process handle,
// then waits (in slices, so the context can stop the wait).
func (n *Native) RunVisible(ctx context.Context, path, args string) (int, error) {
	// Bare names (msiexec.exe, rundll32.exe) come from the system folder,
	// never from the shell's search path (see systemExe).
	path, err := systemExe(path)
	if err != nil {
		return -1, err
	}
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		return -1, fmt.Errorf("programa não encontrado: %s", path)
	}
	verb, _ := windows.UTF16PtrFromString("open")
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return -1, err
	}
	info := shellExecuteInfo{fMask: seeMaskNoCloseProcess | seeMaskNoAsync | seeMaskFlagNoUI, lpVerb: verb, lpFile: file, nShow: windows.SW_SHOWNORMAL}
	info.cbSize = uint32(unsafe.Sizeof(info))
	if args != "" {
		info.lpParameters, _ = windows.UTF16PtrFromString(args)
	}
	if dir := filepath.Dir(path); filepath.IsAbs(dir) {
		info.lpDirectory, _ = windows.UTF16PtrFromString(dir)
	}
	if r, _, e := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		return -1, fmt.Errorf("não foi possível abrir %s: %v", filepath.Base(path), e)
	}
	if info.hProcess == 0 {
		return 0, nil // handed to an existing process (DDE); nothing to wait for
	}
	defer windows.CloseHandle(info.hProcess)
	for {
		ev, err := windows.WaitForSingleObject(info.hProcess, 500)
		if err != nil {
			return -1, err
		}
		if ev == windows.WAIT_OBJECT_0 {
			var code uint32
			if err := windows.GetExitCodeProcess(info.hProcess, &code); err != nil {
				return -1, err
			}
			return int(code), nil
		}
		if ctx.Err() != nil {
			return -1, ctx.Err()
		}
	}
}

// ProtectDir implements sys.Native.
func (n *Native) ProtectDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	return HardenDir(path)
}

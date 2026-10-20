//go:build windows

package main

import (
	"fmt"
	"log"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modKernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	procSetUnhandledExceptionFilter = modKernel32.NewProc("SetUnhandledExceptionFilter")
	procGetModuleHandleExW          = modKernel32.NewProc("GetModuleHandleExW")
)

// exceptionRecord is EXCEPTION_RECORD (x64).
type exceptionRecord struct {
	Code             uint32
	Flags            uint32
	Record           uintptr
	Address          uintptr
	NumberParameters uint32
	_                uint32
	Information      [15]uintptr
}

// exceptionPointers is EXCEPTION_POINTERS.
type exceptionPointers struct {
	Record  *exceptionRecord
	Context uintptr
}

// installNativeCrashLog records in axion.log an exception that reaches the
// top-level filter: one raised on a thread of a Windows component loaded in
// the process, which the Go runtime does not handle. Without this, such a
// crash ends the process with no trace at all when Windows Error Reporting is
// off. Exceptions on Axion's own threads — Go panics and faults inside a
// Windows DLL called by Axion — are printed by the Go runtime to crash.log
// (see setupCrashOutput) and never reach this filter. The filter only writes
// the line; Windows then ends the process as it would anyway.
func installNativeCrashLog() {
	cb := windows.NewCallback(func(ep *exceptionPointers) uintptr {
		if ep == nil || ep.Record == nil {
			log.Printf("FALHA NATIVA (sem detalhes da exceção)")
			return 0
		}
		r := ep.Record
		detail := ""
		if r.Code == 0xC0000005 && r.NumberParameters >= 2 {
			op := map[uintptr]string{0: "leitura", 1: "escrita", 8: "execução"}[r.Information[0]]
			detail = fmt.Sprintf(", %s em 0x%X", op, r.Information[1])
		}
		log.Printf("FALHA NATIVA: exceção 0x%08X em %s%s (thread %d)", r.Code, moduleOf(r.Address), detail, windows.GetCurrentThreadId())
		return 0 // EXCEPTION_CONTINUE_SEARCH
	})
	procSetUnhandledExceptionFilter.Call(cb)
}

// moduleOf names the module (DLL or exe) that contains addr, with the offset.
func moduleOf(addr uintptr) string {
	const fromAddress, unchangedRefcount = 0x4, 0x2
	var h windows.Handle
	if r, _, _ := procGetModuleHandleExW.Call(fromAddress|unchangedRefcount, addr, uintptr(unsafe.Pointer(&h))); r == 0 {
		return fmt.Sprintf("0x%X (fora de um módulo)", addr)
	}
	var buf [windows.MAX_PATH]uint16
	n, err := windows.GetModuleFileName(h, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 {
		return fmt.Sprintf("0x%X", addr)
	}
	return fmt.Sprintf("%s+0x%X", filepath.Base(windows.UTF16ToString(buf[:n])), addr-uintptr(h))
}

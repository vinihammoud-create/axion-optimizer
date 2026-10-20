//go:build windows

package winsys

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"axion/internal/sys"
	"axion/internal/sys/etw"
)

// DPC and ISR trace: the NT Kernel Logger with the DPC and interrupt flags,
// the method Microsoft documents in "Example 15: Measuring DPC/ISR Time". It
// is a real-time session on the QueryPerformanceCounter clock; every routine
// is logged when it ends with the time it was entered, so its duration is the
// difference, and the address of the routine says which driver ran it.
//
// There is only one NT Kernel Logger on a PC: if another tool (LatencyMon,
// xperf, Windows Performance Recorder...) holds it, this capture says so and
// does not touch that session.

var (
	activeDPC  atomic.Pointer[dpcCollector]
	dpcRunning atomic.Bool
)

// systemModuleInformation is the class of NtQuerySystemInformation that lists
// the loaded kernel modules.
const systemModuleInformation = 11

// withPrivilege enables a privilege of the process token while fn runs and
// turns it off again afterwards.
func withPrivilege(name string, fn func()) {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok); err != nil {
		fn()
		return
	}
	defer tok.Close()
	var luid windows.LUID
	p, _ := windows.UTF16PtrFromString(name)
	if windows.LookupPrivilegeValue(nil, p, &luid) != nil {
		fn()
		return
	}
	set := func(attr uint32) {
		tp := windows.Tokenprivileges{PrivilegeCount: 1}
		tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: attr}
		_ = windows.AdjustTokenPrivileges(tok, false, &tp, 0, nil, nil)
	}
	set(windows.SE_PRIVILEGE_ENABLED)
	defer set(0)
	fn()
}

// kernelModules lists the loaded drivers with their address ranges. Windows
// hides the base addresses (they read as zero) from a caller without
// administrator rights, and the list then comes back empty: the debug
// privilege, which an elevated token has, only switched on for this call, is
// what shows them.
func kernelModules() []etw.Module {
	var mods []etw.Module
	withPrivilege("SeDebugPrivilege", func() {
		size := 1 << 20
		for try := 0; try < 4; try++ {
			buf := make([]byte, size)
			var ret uint32
			if err := windows.NtQuerySystemInformation(systemModuleInformation, unsafe.Pointer(&buf[0]), uint32(len(buf)), &ret); err == nil {
				mods = etw.ParseModules(buf)
				return
			}
			size *= 4
		}
	})
	return mods
}

func activeProcessors() int {
	if n := int(windows.GetActiveProcessorCount(windows.ALL_PROCESSOR_GROUPS)); n > 0 {
		return n
	}
	return runtime.NumCPU()
}

// dpcTrace is a running kernel capture.
type dpcTrace struct {
	session  uint64
	consumer uint64
	props    []uint64
	logfile  *etw.EventTraceLogfile
	col      *dpcCollector
	done     chan uint32
}

func startDPCTrace(freq int64) (*dpcTrace, error) {
	if err := procStartTraceW.Find(); err != nil {
		return nil, err
	}
	traceCbOnce.Do(func() { traceCallback = syscall.NewCallback(eventCallback) })
	// The kernel session needs this privilege on the token; the elevated
	// token has it, only disabled.
	EnablePrivileges("SeSystemProfilePrivilege")

	store, p := newProps()
	p.Wnode.GUID = etw.SystemTraceControlGUID
	p.Wnode.Flags = etw.WnodeFlagTracedGUID
	p.Wnode.ClientContext = etw.ClientContextQPC
	p.LogFileMode = etw.EventTraceRealTimeMode
	p.BufferSize = 256 // KB: DPC and ISR tracing produces events at a high rate
	p.MinimumBuffers = 16
	p.MaximumBuffers = 64
	p.FlushTimer = 1
	p.EnableFlags = etw.FlagDPC | etw.FlagInterrupt
	name, _ := windows.UTF16PtrFromString(etw.KernelLoggerName)
	var session uint64
	r, _, _ := procStartTraceW.Call(uintptr(unsafe.Pointer(&session)), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(p)))
	runtime.KeepAlive(store)
	if r != 0 {
		switch syscall.Errno(r) {
		case windows.ERROR_ACCESS_DENIED:
			return nil, errors.New("é preciso executar como administrador")
		case windows.ERROR_ALREADY_EXISTS:
			return nil, errors.New(`outro programa já está usando o registro de eventos do kernel (LatencyMon, xperf, o Gravador de Desempenho do Windows...). Feche-o e tente de novo; se nenhum estiver aberto, execute "logman stop "NT Kernel Logger" -ets" como administrador`)
		}
		return nil, fmt.Errorf("StartTrace (kernel): %w", syscall.Errno(r))
	}
	t := &dpcTrace{session: session, props: store, done: make(chan uint32, 1), col: newDPCCollector(freq)}
	activeDPC.Store(t.col)

	lf := &etw.EventTraceLogfile{
		LoggerName:          name,
		ProcessTraceMode:    etw.ProcessTraceModeRealTime | etw.ProcessTraceModeEventRecord | etw.ProcessTraceModeRawTime,
		EventRecordCallback: traceCallback,
	}
	t.logfile = lf
	h, _, _ := procOpenTraceW.Call(uintptr(unsafe.Pointer(lf)))
	if uint64(h) == invalidProcessTraceHandle {
		t.abort()
		return nil, errors.New("OpenTrace (kernel) falhou")
	}
	t.consumer = uint64(h)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		handle := t.consumer
		r, _, _ := procProcessTrace.Call(uintptr(unsafe.Pointer(&handle)), 1, 0, 0)
		t.done <- uint32(r)
	}()
	return t, nil
}

func (t *dpcTrace) abort() {
	_, _ = controlStop(t.session, etw.KernelLoggerName)
	if t.consumer != 0 {
		procCloseTrace.Call(uintptr(t.consumer))
		select {
		case <-t.done:
		case <-time.After(3 * time.Second):
		}
	}
	activeDPC.CompareAndSwap(t.col, nil)
}

// stop ends the session, waits for the consumer to drain its buffers and
// returns how many events the system dropped.
func (t *dpcTrace) stop() (lost, lostBuf uint32, err error) {
	p, cerr := controlStop(t.session, etw.KernelLoggerName)
	if p != nil {
		lost, lostBuf = p.EventsLost, p.RealTimeBuffersLost
	}
	select {
	case <-t.done:
		procCloseTrace.Call(uintptr(t.consumer))
	case <-time.After(5 * time.Second):
		procCloseTrace.Call(uintptr(t.consumer))
		select {
		case <-t.done:
		case <-time.After(3 * time.Second):
		}
	}
	activeDPC.CompareAndSwap(t.col, nil)
	if ev, _ := t.col.counts(); cerr != nil && ev == 0 {
		return lost, lostBuf, fmt.Errorf("ControlTrace (kernel): %w", cerr)
	}
	return lost, lostBuf, nil
}

// CaptureDPC implements sys.Native.
func (n *Native) CaptureDPC(ctx context.Context, d time.Duration) (sys.DPCCapture, error) {
	if !n.IsElevated() {
		return sys.DPCCapture{}, errors.New("é preciso executar como administrador")
	}
	if !dpcRunning.CompareAndSwap(false, true) {
		return sys.DPCCapture{}, errors.New("já há uma medição de DPC em andamento")
	}
	defer dpcRunning.Store(false)
	if d <= 0 {
		d = 30 * time.Second
	}
	tr, err := startDPCTrace(qpcFreq())
	if err != nil {
		return sys.DPCCapture{}, err
	}
	before := kernelModules()
	res := sys.DPCCapture{Duration: d, CPUs: activeProcessors()}
	timer := time.NewTimer(d)
	select {
	case <-ctx.Done():
		timer.Stop()
		res.Cancelled = true
	case <-timer.C:
	}
	// Read the module list again: a driver loaded during the capture still
	// owns its routines. The union keeps both.
	mm := etw.NewModuleMap(before, kernelModules())
	res.NoModules = mm.Len() == 0
	lost, lostBuf, err := tr.stop()
	res.LostEvents, res.LostBuffers = lost, lostBuf
	res.Events, res.Discarded = tr.col.counts()
	res.Stats = tr.col.stats(mm)
	if err != nil {
		return res, err
	}
	return res, nil
}

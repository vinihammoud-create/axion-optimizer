//go:build windows

package winsys

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"axion/internal/sys"
	"axion/internal/sys/etw"
)

// USB polling trace: a private real-time ETW session enables the USB host
// controller extension (UCX) transfer events and the USB 3 hub driver
// rundown. Every interrupt transfer the mouse completes is timestamped with
// QueryPerformanceCounter by ETW itself, which is the same method DeepPoll
// uses through logman, done here with the native API and without files.

var (
	modAdvapi32         = windows.NewLazySystemDLL("advapi32.dll")
	procStartTraceW     = modAdvapi32.NewProc("StartTraceW")
	procControlTraceW   = modAdvapi32.NewProc("ControlTraceW")
	procEnableTraceEx2  = modAdvapi32.NewProc("EnableTraceEx2")
	procOpenTraceW      = modAdvapi32.NewProc("OpenTraceW")
	procProcessTrace    = modAdvapi32.NewProc("ProcessTrace")
	procCloseTrace      = modAdvapi32.NewProc("CloseTrace")
	procQueryPerfFreq   = modKernel32.NewProc("QueryPerformanceFrequency")
	procQueryPerfCount  = modKernel32.NewProc("QueryPerformanceCounter")
	usbTraceSessionName = "Axion-USB-Polling"
)

const invalidProcessTraceHandle = ^uint64(0)

func qpc() int64 {
	var v int64
	procQueryPerfCount.Call(uintptr(unsafe.Pointer(&v)))
	return v
}

func qpcFreq() int64 {
	var v int64
	procQueryPerfFreq.Call(uintptr(unsafe.Pointer(&v)))
	return v
}

type pipeID struct{ dev, pipe uint64 }

// traceCollector receives events on the ProcessTrace thread.
type traceCollector struct {
	mu      sync.Mutex
	series  map[pipeID][]int64
	devices map[uint64]sys.USBTraceDevice
	events  int
}

var (
	activeTrace   atomic.Pointer[traceCollector]
	traceCallback uintptr
	traceCbOnce   sync.Once
)

func eventCallback(rec *etw.EventRecord) uintptr {
	if rec == nil || rec.UserData == nil || rec.UserDataLength == 0 {
		return 0
	}
	h := &rec.Header
	data := unsafe.Slice((*byte)(rec.UserData), int(rec.UserDataLength))
	// Kernel DPC and ISR events of the NT Kernel Logger (see dpctrace.go).
	if h.ProviderID == etw.ProviderPerfInfo {
		if dc := activeDPC.Load(); dc != nil {
			dc.event(h.Descriptor.Opcode, h.TimeStamp, h.PointerSize(), data)
		}
		return 0
	}
	tc := activeTrace.Load()
	if tc == nil {
		return 0
	}
	switch h.ProviderID {
	case etw.ProviderUCX:
		if h.Descriptor.Task != etw.TaskBulkOrInterrupt || h.Descriptor.Opcode != etw.OpcodeStop {
			return 0
		}
		dev, pipe, _, ok := etw.ParseURB(data, h.PointerSize())
		if !ok || dev == 0 {
			return 0
		}
		tc.mu.Lock()
		k := pipeID{dev, pipe}
		tc.series[k] = append(tc.series[k], h.TimeStamp)
		tc.events++
		tc.mu.Unlock()
	case etw.ProviderUSBHUB3:
		if id := h.Descriptor.ID; id != etw.EventHubDeviceRundown && id != etw.EventHubPrepareHW {
			return 0
		}
		handle, port, desc, path, ok := etw.ParseHubDevice(data, h.PointerSize())
		if !ok || handle == 0 {
			return 0
		}
		tc.mu.Lock()
		tc.devices[handle] = sys.USBTraceDevice{Handle: handle, Port: port, Desc: desc, Path: path}
		tc.mu.Unlock()
	}
	return 0
}

// usbTrace is a running capture.
type usbTrace struct {
	session  uint64
	consumer uint64
	props    []uint64 // backing store of EVENT_TRACE_PROPERTIES + names
	logfile  *etw.EventTraceLogfile
	tc       *traceCollector
	done     chan uint32
}

// newProps allocates EVENT_TRACE_PROPERTIES followed by room for the session
// name (8-byte aligned, as the API requires).
func newProps() ([]uint64, *etw.TraceProperties) {
	const extra = 1024
	store := make([]uint64, (int(unsafe.Sizeof(etw.TraceProperties{}))+extra+7)/8)
	p := (*etw.TraceProperties)(unsafe.Pointer(&store[0]))
	p.Wnode.BufferSize = uint32(len(store) * 8)
	p.LoggerNameOffset = uint32(unsafe.Sizeof(etw.TraceProperties{}))
	return store, p
}

func controlStop(session uint64, name string) (*etw.TraceProperties, error) {
	store, p := newProps()
	n, _ := windows.UTF16PtrFromString(name)
	var np uintptr
	if session == 0 {
		np = uintptr(unsafe.Pointer(n))
	}
	r, _, _ := procControlTraceW.Call(uintptr(session), np, uintptr(unsafe.Pointer(p)), etw.EventTraceControlStop)
	// The conversion above is stored in a variable, so //go:uintptrescapes on
	// Call does not cover n: keep the name buffer alive until the call is over
	// (ControlTraceW matches a session by name when no handle is given).
	runtime.KeepAlive(n)
	runtime.KeepAlive(store)
	if r != 0 {
		return p, syscall.Errno(r)
	}
	return p, nil
}

func enableProvider(session uint64, provider *etw.GUID, code uint32, keywords uint64) error {
	r, _, _ := procEnableTraceEx2.Call(uintptr(session), uintptr(unsafe.Pointer(provider)), uintptr(code),
		uintptr(etw.LevelInformation), uintptr(keywords), 0, 0, 0)
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

func startUSBTrace() (*usbTrace, error) {
	if err := procStartTraceW.Find(); err != nil {
		return nil, err
	}
	traceCbOnce.Do(func() { traceCallback = syscall.NewCallback(eventCallback) })

	// A session left behind by a crash would make StartTrace fail.
	_, _ = controlStop(0, usbTraceSessionName)

	store, p := newProps()
	if g, err := windows.GenerateGUID(); err == nil {
		p.Wnode.GUID = etw.GUID{Data1: g.Data1, Data2: g.Data2, Data3: g.Data3, Data4: g.Data4}
	}
	p.Wnode.Flags = etw.WnodeFlagTracedGUID
	p.Wnode.ClientContext = etw.ClientContextQPC
	p.LogFileMode = etw.EventTraceRealTimeMode
	p.BufferSize = 128 // KB
	p.MinimumBuffers = 32
	p.MaximumBuffers = 128
	p.FlushTimer = 1
	name, _ := windows.UTF16PtrFromString(usbTraceSessionName)
	var session uint64
	r, _, _ := procStartTraceW.Call(uintptr(unsafe.Pointer(&session)), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(p)))
	if r != 0 {
		if syscall.Errno(r) == windows.ERROR_ACCESS_DENIED {
			return nil, errors.New("é preciso executar como administrador")
		}
		return nil, fmt.Errorf("StartTrace: %w", syscall.Errno(r))
	}
	t := &usbTrace{session: session, props: store, done: make(chan uint32, 1),
		tc: &traceCollector{series: map[pipeID][]int64{}, devices: map[uint64]sys.USBTraceDevice{}}}
	activeTrace.Store(t.tc)

	// Consumer first, so the hub rundown emitted on enable is not lost.
	lf := &etw.EventTraceLogfile{
		LoggerName:          name,
		ProcessTraceMode:    etw.ProcessTraceModeRealTime | etw.ProcessTraceModeEventRecord | etw.ProcessTraceModeRawTime,
		EventRecordCallback: traceCallback,
	}
	t.logfile = lf
	h, _, _ := procOpenTraceW.Call(uintptr(unsafe.Pointer(lf)))
	if uint64(h) == invalidProcessTraceHandle {
		t.abort()
		return nil, errors.New("OpenTrace falhou")
	}
	t.consumer = uint64(h)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		handle := t.consumer
		r, _, _ := procProcessTrace.Call(uintptr(unsafe.Pointer(&handle)), 1, 0, 0)
		t.done <- uint32(r)
	}()

	if err := enableProvider(session, &etw.ProviderUCX, etw.ControlCodeEnableProvider, etw.KeywordHeadersBusTrace); err != nil {
		t.abort()
		return nil, fmt.Errorf("provedor USB UCX indisponível (%v); o PC usa a pilha USB 3 do Windows?", err)
	}
	// Rundown names the devices already connected; PnP (event 43) names
	// the ones plugged in during the capture.
	hubKeywords := uint64(etw.KeywordRundown | etw.KeywordPnP)
	if err := enableProvider(session, &etw.ProviderUSBHUB3, etw.ControlCodeEnableProvider, hubKeywords); err == nil {
		_ = enableProvider(session, &etw.ProviderUSBHUB3, etw.ControlCodeCaptureState, hubKeywords)
	}
	return t, nil
}

func (t *usbTrace) abort() {
	_, _ = controlStop(t.session, usbTraceSessionName)
	if t.consumer != 0 {
		procCloseTrace.Call(uintptr(t.consumer))
		select {
		case <-t.done:
		case <-time.After(3 * time.Second):
		}
	}
	activeTrace.CompareAndSwap(t.tc, nil)
}

// stop ends the session, waits for the consumer to drain and returns the data.
func (t *usbTrace) stop() ([]sys.USBPipeSeries, []sys.USBTraceDevice, uint32, uint32, error) {
	_ = enableProvider(t.session, &etw.ProviderUCX, etw.ControlCodeDisableProvider, 0)
	_ = enableProvider(t.session, &etw.ProviderUSBHUB3, etw.ControlCodeDisableProvider, 0)
	p, err := controlStop(t.session, usbTraceSessionName)
	var lost, lostBuf uint32
	if p != nil {
		lost, lostBuf = p.EventsLost, p.RealTimeBuffersLost
	}
	// ProcessTrace returns once the stopped session has delivered its
	// remaining buffers; CloseTrace also releases it if it lingers.
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
	activeTrace.CompareAndSwap(t.tc, nil)

	t.tc.mu.Lock()
	defer t.tc.mu.Unlock()
	var series []sys.USBPipeSeries
	for k, st := range t.tc.series {
		series = append(series, sys.USBPipeSeries{Device: k.dev, Pipe: k.pipe, Stamps: st})
	}
	var devs []sys.USBTraceDevice
	for _, d := range t.tc.devices {
		devs = append(devs, d)
	}
	if err != nil && t.tc.events == 0 {
		return series, devs, lost, lostBuf, fmt.Errorf("ControlTrace: %w", err)
	}
	return series, devs, lost, lostBuf, nil
}

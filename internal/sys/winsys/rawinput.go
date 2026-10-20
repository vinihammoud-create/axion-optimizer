//go:build windows

package winsys

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"axion/internal/sys"
)

// Raw input capture: a focused, topmost window registers for mouse raw input
// and stamps every WM_INPUT with QueryPerformanceCounter the moment it is
// dispatched. Windows 11 throttles raw input sent to background windows, so
// the window keeps the focus and reports when it loses it.

var (
	modGdi32                    = windows.NewLazySystemDLL("gdi32.dll")
	procRegisterClassExW        = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW         = modUser32.NewProc("CreateWindowExW")
	procDefWindowProcW          = modUser32.NewProc("DefWindowProcW")
	procDestroyWindow           = modUser32.NewProc("DestroyWindow")
	procShowWindow              = modUser32.NewProc("ShowWindow")
	procUpdateWindow            = modUser32.NewProc("UpdateWindow")
	procSetForegroundWindow     = modUser32.NewProc("SetForegroundWindow")
	procSetFocus                = modUser32.NewProc("SetFocus")
	procGetMessageW             = modUser32.NewProc("GetMessageW")
	procTranslateMessage        = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW        = modUser32.NewProc("DispatchMessageW")
	procPostMessageW            = modUser32.NewProc("PostMessageW")
	procPostQuitMessage         = modUser32.NewProc("PostQuitMessage")
	procSetTimer                = modUser32.NewProc("SetTimer")
	procKillTimer               = modUser32.NewProc("KillTimer")
	procInvalidateRect          = modUser32.NewProc("InvalidateRect")
	procBeginPaint              = modUser32.NewProc("BeginPaint")
	procEndPaint                = modUser32.NewProc("EndPaint")
	procFillRect                = modUser32.NewProc("FillRect")
	procDrawTextW               = modUser32.NewProc("DrawTextW")
	procGetClientRect           = modUser32.NewProc("GetClientRect")
	procGetWindowRect           = modUser32.NewProc("GetWindowRect")
	procClipCursor              = modUser32.NewProc("ClipCursor")
	procLoadCursorW             = modUser32.NewProc("LoadCursorW")
	procRegisterRawInputDevices = modUser32.NewProc("RegisterRawInputDevices")
	procGetRawInputData         = modUser32.NewProc("GetRawInputData")
	procGetRawInputDeviceInfoW  = modUser32.NewProc("GetRawInputDeviceInfoW")
	procMonitorFromWindow       = modUser32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW         = modUser32.NewProc("GetMonitorInfoW")
	procGetDpiForWindow         = modUser32.NewProc("GetDpiForWindow")
	procAttachThreadInput       = modUser32.NewProc("AttachThreadInput")
	procIsWindow                = modUser32.NewProc("IsWindow")
	procGetModuleHandleW        = modKernel32.NewProc("GetModuleHandleW")
	procCreateSolidBrush        = modGdi32.NewProc("CreateSolidBrush")
	procCreateFontW             = modGdi32.NewProc("CreateFontW")
	procSelectObject            = modGdi32.NewProc("SelectObject")
	procDeleteObject            = modGdi32.NewProc("DeleteObject")
	procSetTextColor            = modGdi32.NewProc("SetTextColor")
	procSetBkMode               = modGdi32.NewProc("SetBkMode")
)

const (
	wmDestroy     = 0x0002
	wmActivate    = 0x0006
	wmClose       = 0x0010
	wmPaint       = 0x000F
	wmEraseBkgnd  = 0x0014
	wmInput       = 0x00FF
	wmKeyDown     = 0x0100
	wmTimer       = 0x0113
	wsPopup       = 0x80000000
	wsBorder      = 0x00800000
	wsExTopmost   = 0x00000008
	swShow        = 5
	ridInput      = 0x10000003
	ridiName      = 0x20000007
	rimTypeMouse  = 0
	ridevRemove   = 0x00000001
	dtCenter      = 0x0001
	dtVCenter     = 0x0004
	dtWordBreak   = 0x0010
	dtSingleLine  = 0x0020
	dtNoPrefix    = 0x0800
	idcCross      = 32515
	vkEscape      = 0x1B
	bgColor       = 0x00100B09 // RGB(9, 11, 16), the app background
	textColor     = 0x00F5EBE6
	accentColor   = 0x00FFC04A
	captureTimer  = 1
	monitorNearst = 2
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type point struct{ X, Y int32 }

type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}

type rect struct{ Left, Top, Right, Bottom int32 }

type paintStruct struct {
	Hdc       uintptr
	Erase     int32
	RcPaint   rect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

type rawInputDevice struct {
	UsagePage uint16
	Usage     uint16
	Flags     uint32
	Target    uintptr
}

// rawCapture is the state of the running test window (one at a time).
type rawCapture struct {
	hwnd      uintptr
	clip      rect
	start     time.Time
	deadline  time.Time
	title     string
	message   string
	series    map[uintptr]*sys.RawInputSeries
	moves     int
	focusLost bool
	cancelled bool
	brush     uintptr
	fontTitle uintptr
	fontBody  uintptr
	fontBig   uintptr
}

var (
	activeRaw     atomic.Pointer[rawCapture]
	rawClassOnce  sync.Once
	rawClassErr   error
	rawClassName  = windows.StringToUTF16Ptr("AxionInputTest")
	rawWndProcPtr uintptr
	captureMu     sync.Mutex
)

func loword(v uintptr) uint16 { return uint16(v & 0xFFFF) }

func rawWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	c := activeRaw.Load()
	switch msg {
	case wmInput:
		if c != nil {
			stamp := qpc()
			c.onInput(lparam, stamp)
		}
	case wmTimer:
		if c != nil {
			if time.Now().After(c.deadline) {
				procDestroyWindow.Call(hwnd)
				return 0
			}
			if time.Since(c.start) > 400*time.Millisecond {
				if fg := uintptr(windows.GetForegroundWindow()); fg != hwnd {
					c.focusLost = true
				}
			}
			procInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0
	case wmActivate:
		if c != nil {
			if loword(wparam) == 0 { // WA_INACTIVE: never keep the cursor trapped
				procClipCursor.Call(0)
				if time.Since(c.start) > 400*time.Millisecond {
					c.focusLost = true
				}
			} else if c.clip.Right > c.clip.Left {
				procClipCursor.Call(uintptr(unsafe.Pointer(&c.clip)))
			}
		}
	case wmKeyDown:
		if wparam == vkEscape && c != nil {
			c.cancelled = true
			procDestroyWindow.Call(hwnd)
			return 0
		}
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		if c != nil {
			c.paint(hwnd)
			return 0
		}
	case wmDestroy:
		procKillTimer.Call(hwnd, captureTimer)
		procClipCursor.Call(0)
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
}

func (c *rawCapture) onInput(h uintptr, stamp int64) {
	var buf [8]uint64 // RAWINPUTHEADER (24) + RAWMOUSE (24)
	size := uint32(unsafe.Sizeof(buf))
	r, _, _ := procGetRawInputData.Call(h, ridInput, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 24)
	if int32(r) <= 0 {
		return
	}
	b := (*[64]byte)(unsafe.Pointer(&buf[0]))
	if *(*uint32)(unsafe.Pointer(&b[0])) != rimTypeMouse {
		return
	}
	dev := *(*uintptr)(unsafe.Pointer(&b[8]))
	dx := *(*int32)(unsafe.Pointer(&b[36]))
	dy := *(*int32)(unsafe.Pointer(&b[40]))
	s := c.series[dev]
	if s == nil {
		s = &sys.RawInputSeries{Kind: "mouse"}
		c.series[dev] = s
	}
	s.Events++
	if dx != 0 || dy != 0 {
		s.Stamps = append(s.Stamps, stamp)
		c.moves++
	}
}

func (c *rawCapture) paint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var rc rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), c.brush)
	procSetBkMode.Call(hdc, 1) // TRANSPARENT
	h := rc.Bottom - rc.Top
	draw := func(font uintptr, color uint32, text string, top, bottom int32, flags uintptr) {
		procSelectObject.Call(hdc, font)
		procSetTextColor.Call(hdc, uintptr(color))
		r := rect{Left: rc.Left + h/12, Top: top, Right: rc.Right - h/12, Bottom: bottom}
		t, _ := windows.UTF16FromString(text)
		procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&t[0])), ^uintptr(0), uintptr(unsafe.Pointer(&r)), flags|dtCenter|dtNoPrefix)
	}
	left := time.Until(c.deadline).Seconds()
	if left < 0 {
		left = 0
	}
	draw(c.fontTitle, textColor, c.title, rc.Top+h/10, rc.Top+h/10+h/8, dtSingleLine|dtVCenter)
	draw(c.fontBody, textColor, c.message, rc.Top+h/4, rc.Top+h/2, dtWordBreak)
	draw(c.fontBig, accentColor, fmt.Sprintf("%.0f s", left+0.49), rc.Top+h/2, rc.Top+h/2+h/5, dtSingleLine|dtVCenter)
	draw(c.fontBody, textColor, fmt.Sprintf("%d relatórios de movimento recebidos", c.moves), rc.Top+h/2+h/5, rc.Top+h/2+h/5+h/10, dtSingleLine|dtVCenter)
}

func registerRawClass() error {
	rawClassOnce.Do(func() {
		rawWndProcPtr = syscall.NewCallback(rawWndProc)
		inst, _, _ := procGetModuleHandleW.Call(0)
		cur, _, _ := procLoadCursorW.Call(0, idcCross)
		wc := wndClassEx{WndProc: rawWndProcPtr, Instance: inst, Cursor: cur, ClassName: rawClassName}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			rawClassErr = fmt.Errorf("RegisterClassEx: %v", err)
		}
	})
	return rawClassErr
}

func mkFont(px int32, weight int32) uintptr {
	face, _ := windows.UTF16PtrFromString("Segoe UI")
	f, _, _ := procCreateFontW.Call(uintptr(-px), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0x20, uintptr(unsafe.Pointer(face)))
	return f
}

// runRawWindow shows the test window for d and returns the captured series.
func runRawWindow(ctx context.Context, opts sys.CaptureOpts, d time.Duration) (*rawCapture, error) {
	if err := registerRawClass(); err != nil {
		return nil, err
	}
	type outcome struct {
		c   *rawCapture
		err error
	}
	res := make(chan outcome, 1)
	hwndCh := make(chan uintptr, 1)
	go func() {
		// The thread stays locked on purpose: when this goroutine returns,
		// Go terminates the thread together with its message queue, so no
		// stray WM_QUIT or window state leaks into a pooled thread.
		runtime.LockOSThread()
		c := &rawCapture{title: opts.Title, message: opts.Message, series: map[uintptr]*sys.RawInputSeries{}}
		brush, _, _ := procCreateSolidBrush.Call(bgColor)
		c.brush = brush
		defer procDeleteObject.Call(brush)

		// Centre on the monitor of the main window, 60% of its work area.
		var mi monitorInfo
		mi.Size = uint32(unsafe.Sizeof(mi))
		mon, _, _ := procMonitorFromWindow.Call(OwnerWindow, monitorNearst)
		procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
		ww, wh := (mi.Work.Right-mi.Work.Left)*6/10, (mi.Work.Bottom-mi.Work.Top)*6/10
		if ww < 640 {
			ww, wh = 640, 400
		}
		x := mi.Work.Left + (mi.Work.Right-mi.Work.Left-ww)/2
		y := mi.Work.Top + (mi.Work.Bottom-mi.Work.Top-wh)/2
		title, _ := windows.UTF16PtrFromString("Axion — teste do mouse")
		inst, _, _ := procGetModuleHandleW.Call(0)
		hwnd, _, err := procCreateWindowExW.Call(wsExTopmost, uintptr(unsafe.Pointer(rawClassName)), uintptr(unsafe.Pointer(title)),
			wsPopup|wsBorder, uintptr(x), uintptr(y), uintptr(ww), uintptr(wh), 0, 0, inst, 0)
		if hwnd == 0 {
			hwndCh <- 0
			res <- outcome{err: fmt.Errorf("CreateWindowEx: %v", err)}
			return
		}
		c.hwnd = hwnd
		dpi := int32(96)
		if procGetDpiForWindow.Find() == nil {
			if v, _, _ := procGetDpiForWindow.Call(hwnd); v != 0 {
				dpi = int32(v)
			}
		}
		c.fontTitle, c.fontBody, c.fontBig = mkFont(30*dpi/96, 600), mkFont(19*dpi/96, 400), mkFont(56*dpi/96, 700)
		defer procDeleteObject.Call(c.fontTitle)
		defer procDeleteObject.Call(c.fontBody)
		defer procDeleteObject.Call(c.fontBig)

		rid := rawInputDevice{UsagePage: 0x01, Usage: 0x02, Target: hwnd}
		if r, _, err := procRegisterRawInputDevices.Call(uintptr(unsafe.Pointer(&rid)), 1, unsafe.Sizeof(rid)); r == 0 {
			procDestroyWindow.Call(hwnd)
			hwndCh <- 0
			res <- outcome{err: fmt.Errorf("RegisterRawInputDevices: %v", err)}
			return
		}
		defer func() {
			off := rawInputDevice{UsagePage: 0x01, Usage: 0x02, Flags: ridevRemove}
			procRegisterRawInputDevices.Call(uintptr(unsafe.Pointer(&off)), 1, unsafe.Sizeof(off))
		}()

		c.start = time.Now()
		c.deadline = c.start.Add(d)
		activeRaw.Store(c)
		defer activeRaw.Store(nil)
		hwndCh <- hwnd // cancellation works from here on
		procShowWindow.Call(hwnd, swShow)
		procUpdateWindow.Call(hwnd)
		procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&c.clip)))
		// Take the foreground. Attaching input queues is only done with a
		// thread of this process (the main window): attaching to another
		// process's thread would block here if that program hangs.
		fg := windows.GetForegroundWindow()
		var fgThread, fgPID uint32
		if fg != 0 {
			fgThread, _ = windows.GetWindowThreadProcessId(fg, &fgPID)
		}
		me := windows.GetCurrentThreadId()
		attach := fgThread != 0 && fgThread != me && fgPID == windows.GetCurrentProcessId()
		if attach {
			procAttachThreadInput.Call(uintptr(me), uintptr(fgThread), 1)
		}
		procSetForegroundWindow.Call(hwnd)
		procSetFocus.Call(hwnd)
		if attach {
			procAttachThreadInput.Call(uintptr(me), uintptr(fgThread), 0)
		}
		if uintptr(windows.GetForegroundWindow()) == hwnd {
			procClipCursor.Call(uintptr(unsafe.Pointer(&c.clip)))
		}
		procSetTimer.Call(hwnd, captureTimer, 100, 0)

		var m winMsg
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
		if r, _, _ := procIsWindow.Call(hwnd); r != 0 {
			procDestroyWindow.Call(hwnd)
		}
		procClipCursor.Call(0)
		res <- outcome{c: c}
	}()

	hwnd := <-hwndCh
	finished := make(chan struct{})
	defer close(finished)
	if hwnd != 0 {
		go func() {
			select {
			case <-ctx.Done():
				procPostMessageW.Call(hwnd, wmClose, 0, 0)
			case <-finished:
			}
		}()
	}
	out := <-res
	if out.c != nil && ctx.Err() != nil {
		out.c.cancelled = true
	}
	return out.c, out.err
}

func rawDeviceName(h uintptr) string {
	var size uint32
	procGetRawInputDeviceInfoW.Call(h, ridiName, 0, uintptr(unsafe.Pointer(&size)))
	if size == 0 || size > 4096 {
		return ""
	}
	buf := make([]uint16, size+1)
	r, _, _ := procGetRawInputDeviceInfoW.Call(h, ridiName, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if int32(r) <= 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

// CaptureInput implements sys.Native.
func (n *Native) CaptureInput(ctx context.Context, opts sys.CaptureOpts) (sys.InputCapture, error) {
	if !captureMu.TryLock() {
		return sys.InputCapture{}, errors.New("já existe um teste de periféricos em andamento")
	}
	defer captureMu.Unlock()
	d := opts.Duration
	if d <= 0 {
		d = 5 * time.Second
	}
	res := sys.InputCapture{Freq: qpcFreq(), Duration: d}
	var trace *usbTrace
	if opts.USBTrace {
		t, err := startUSBTrace()
		if err != nil {
			res.TraceError = err.Error()
		} else {
			trace = t
		}
	}
	c, err := runRawWindow(ctx, opts, d)
	if trace != nil {
		series, devs, lost, lostBuf, terr := trace.stop()
		res.USB, res.USBDevices, res.LostEvents, res.LostBuffers = series, devs, lost, lostBuf
		res.USBTraced = terr == nil
		if terr != nil {
			res.TraceError = terr.Error()
		}
	}
	if err != nil {
		return res, err
	}
	res.FocusLost, res.Cancelled = c.focusLost, c.cancelled
	for h, s := range c.series {
		s.Device = rawDeviceName(h)
		res.RawInput = append(res.RawInput, *s)
	}
	sort.Slice(res.RawInput, func(i, j int) bool { return len(res.RawInput[i].Stamps) > len(res.RawInput[j].Stamps) })
	return res, nil
}

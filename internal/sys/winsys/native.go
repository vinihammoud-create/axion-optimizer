//go:build windows

package winsys

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"axion/internal/sys"
)

var (
	modUser32                    = windows.NewLazySystemDLL("user32.dll")
	procEnumDisplayDevicesW      = modUser32.NewProc("EnumDisplayDevicesW")
	procEnumDisplaySettingsExW   = modUser32.NewProc("EnumDisplaySettingsExW")
	procChangeDisplaySettingsExW = modUser32.NewProc("ChangeDisplaySettingsExW")
	procSystemParametersInfoW    = modUser32.NewProc("SystemParametersInfoW")
	procSendMessageTimeoutW      = modUser32.NewProc("SendMessageTimeoutW")
	modKernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx     = modKernel32.NewProc("GlobalMemoryStatusEx")
	procGetSystemTimes           = modKernel32.NewProc("GetSystemTimes")
	procGetSystemPowerStatus     = modKernel32.NewProc("GetSystemPowerStatus")
	procK32GetProcessMemoryInfo  = modKernel32.NewProc("K32GetProcessMemoryInfo")
	modNtdll                     = windows.NewLazySystemDLL("ntdll.dll")
	procNtQueryTimerResolution   = modNtdll.NewProc("NtQueryTimerResolution")
	procNtSetTimerResolution     = modNtdll.NewProc("NtSetTimerResolution")
	procNtSuspendProcess         = modNtdll.NewProc("NtSuspendProcess")
	procNtResumeProcess          = modNtdll.NewProc("NtResumeProcess")
	procNtQuerySystemInformation = modNtdll.NewProc("NtQuerySystemInformation")
	procNtSetSystemInformation   = modNtdll.NewProc("NtSetSystemInformation")
	modIphlpapi                  = windows.NewLazySystemDLL("iphlpapi.dll")
	procIcmpCreateFile           = modIphlpapi.NewProc("IcmpCreateFile")
	procIcmpSendEcho             = modIphlpapi.NewProc("IcmpSendEcho")
	procIcmpCloseHandle          = modIphlpapi.NewProc("IcmpCloseHandle")
	procGetBestInterface         = modIphlpapi.NewProc("GetBestInterface")
	procSetInterfaceDnsSettings  = modIphlpapi.NewProc("SetInterfaceDnsSettings")
	procGetInterfaceDnsSettings  = modIphlpapi.NewProc("GetInterfaceDnsSettings")
	procFreeInterfaceDnsSettings = modIphlpapi.NewProc("FreeInterfaceDnsSettings")
	modShell32                   = windows.NewLazySystemDLL("shell32.dll")
	procSHQueryRecycleBinW       = modShell32.NewProc("SHQueryRecycleBinW")
	procSHEmptyRecycleBinW       = modShell32.NewProc("SHEmptyRecycleBinW")
)

// Native implements sys.Native.
type Native struct {
	WMI *WMI
	Reg sys.Registry

	privOnce sync.Once
	timerMu  sync.Mutex
	timerSet bool
}

// EnablePrivileges enables the privileges Axion needs on its own token.
func EnablePrivileges(names ...string) {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok); err != nil {
		return
	}
	defer tok.Close()
	for _, n := range names {
		var luid windows.LUID
		p, _ := windows.UTF16PtrFromString(n)
		if windows.LookupPrivilegeValue(nil, p, &luid) != nil {
			continue
		}
		tp := windows.Tokenprivileges{PrivilegeCount: 1}
		tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
		_ = windows.AdjustTokenPrivileges(tok, false, &tp, 0, nil, nil)
	}
}

func (n *Native) privs() {
	n.privOnce.Do(func() {
		EnablePrivileges("SeProfileSingleProcessPrivilege", "SeIncreaseQuotaPrivilege", "SeShutdownPrivilege")
	})
}

// IsElevated implements sys.Native.
func (n *Native) IsElevated() bool { return windows.GetCurrentProcessToken().IsElevated() }

// WindowsBuild implements sys.Native.
func (n *Native) WindowsBuild() (int, int, int, int) {
	v := windows.RtlGetVersion()
	ubr := 0
	if val, err := n.Reg.GetValue(`HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion`, "UBR"); err == nil && val.Exists {
		ubr = int(val.Int)
	}
	return int(v.MajorVersion), int(v.MinorVersion), int(v.BuildNumber), ubr
}

// ---------------------------------------------------------------------------
// Displays
// ---------------------------------------------------------------------------

type displayDevice struct {
	Cb           uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

type devMode struct {
	DeviceName         [32]uint16
	SpecVersion        uint16
	DriverVersion      uint16
	Size               uint16
	DriverExtra        uint16
	Fields             uint32
	PositionX          int32
	PositionY          int32
	DisplayOrientation uint32
	DisplayFixedOutput uint32
	Color              int16
	Duplex             int16
	YResolution        int16
	TTOption           int16
	Collate            int16
	FormName           [32]uint16
	LogPixels          uint16
	BitsPerPel         uint32
	PelsWidth          uint32
	PelsHeight         uint32
	DisplayFlags       uint32
	DisplayFrequency   uint32
	ICMMethod          uint32
	ICMIntent          uint32
	MediaType          uint32
	DitherType         uint32
	Reserved1          uint32
	Reserved2          uint32
	PanningWidth       uint32
	PanningHeight      uint32
}

const (
	ddAttachedToDesktop = 0x1
	ddPrimaryDevice     = 0x4
	enumCurrentSettings = 0xFFFFFFFF
	dmBitsPerPel        = 0x40000
	dmPelsWidth         = 0x80000
	dmPelsHeight        = 0x100000
	dmDisplayFrequency  = 0x400000
	cdsUpdateRegistry   = 0x1
	cdsTest             = 0x2
)

// DisplayModes implements sys.Native.
func (n *Native) DisplayModes() ([]sys.DisplayMode, error) {
	var out []sys.DisplayMode
	targets := displayTargets()
	for i := uint32(0); i < 16; i++ {
		var dd displayDevice
		dd.Cb = uint32(unsafe.Sizeof(dd))
		r, _, _ := procEnumDisplayDevicesW.Call(0, uintptr(i), uintptr(unsafe.Pointer(&dd)), 0)
		if r == 0 {
			break
		}
		if dd.StateFlags&ddAttachedToDesktop == 0 {
			continue
		}
		dev := windows.UTF16ToString(dd.DeviceName[:])
		devPtr := &dd.DeviceName[0]
		var cur devMode
		cur.Size = uint16(unsafe.Sizeof(cur))
		r, _, _ = procEnumDisplaySettingsExW.Call(uintptr(unsafe.Pointer(devPtr)), enumCurrentSettings, uintptr(unsafe.Pointer(&cur)), 0)
		if r == 0 {
			continue
		}
		m := sys.DisplayMode{
			Device: dev, Width: int(cur.PelsWidth), Height: int(cur.PelsHeight),
			CurrentHz: int(cur.DisplayFrequency), BitsPerPel: int(cur.BitsPerPel), Primary: dd.StateFlags&ddPrimaryDevice != 0,
		}
		for j := uint32(0); j < 4096; j++ {
			var dm devMode
			dm.Size = uint16(unsafe.Sizeof(dm))
			r, _, _ := procEnumDisplaySettingsExW.Call(uintptr(unsafe.Pointer(devPtr)), uintptr(j), uintptr(unsafe.Pointer(&dm)), 0)
			if r == 0 {
				break
			}
			hz := int(dm.DisplayFrequency)
			if hz <= 1 || hz > 1000 {
				continue
			}
			if hz > m.MaxHzAny {
				m.MaxHzAny = hz
			}
			if int(dm.PelsWidth) == m.Width && int(dm.PelsHeight) == m.Height && int(dm.BitsPerPel) == m.BitsPerPel && hz > m.MaxHzAtRes {
				m.MaxHzAtRes = hz
			}
		}
		// Monitor friendly name (child device of the adapter output).
		var mon displayDevice
		mon.Cb = uint32(unsafe.Sizeof(mon))
		if r, _, _ := procEnumDisplayDevicesW.Call(uintptr(unsafe.Pointer(devPtr)), 0, uintptr(unsafe.Pointer(&mon)), 0); r != 0 {
			m.Monitor = windows.UTF16ToString(mon.DeviceString[:])
		}
		// The name in the monitor's EDID and the connection come from the
		// display configuration ("Generic PnP Monitor" says nothing).
		if t, ok := targets[strings.ToUpper(dev)]; ok {
			m.Internal = t.internal
			if t.name != "" {
				m.Monitor = t.name
			}
		}
		if m.Monitor == "" || strings.EqualFold(m.Monitor, "Generic PnP Monitor") || strings.Contains(strings.ToLower(m.Monitor), "genérico") {
			if m.Internal {
				m.Monitor = "Tela integrada"
			} else {
				m.Monitor = fmt.Sprintf("Monitor %d", len(out)+1)
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// SetDisplayMode implements sys.Native.
func (n *Native) SetDisplayMode(device string, width, height, hz int) error {
	name, err := windows.UTF16PtrFromString(device)
	if err != nil {
		return err
	}
	var dm devMode
	dm.Size = uint16(unsafe.Sizeof(dm))
	if r, _, _ := procEnumDisplaySettingsExW.Call(uintptr(unsafe.Pointer(name)), enumCurrentSettings, uintptr(unsafe.Pointer(&dm)), 0); r == 0 {
		return errors.New("não foi possível ler o modo atual do monitor")
	}
	dm.PelsWidth, dm.PelsHeight, dm.DisplayFrequency = uint32(width), uint32(height), uint32(hz)
	dm.Fields = dmPelsWidth | dmPelsHeight | dmDisplayFrequency
	r, _, _ := procChangeDisplaySettingsExW.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&dm)), 0, cdsTest, 0)
	if int32(r) != 0 {
		return fmt.Errorf("o monitor recusou %dx%d a %d Hz (código %d)", width, height, hz, int32(r))
	}
	r, _, _ = procChangeDisplaySettingsExW.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&dm)), 0, cdsUpdateRegistry, 0)
	switch int32(r) {
	case 0:
		return nil
	case 1:
		return errors.New("o Windows pediu reinício para aplicar a nova taxa de atualização")
	}
	return fmt.Errorf("ChangeDisplaySettingsEx falhou (código %d)", int32(r))
}

// ---------------------------------------------------------------------------
// Memory
// ---------------------------------------------------------------------------

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// systemMemoryListInformation mirrors SYSTEM_MEMORY_LIST_INFORMATION (x64).
type systemMemoryListInformation struct {
	ZeroPageCount             uintptr
	FreePageCount             uintptr
	ModifiedPageCount         uintptr
	ModifiedNoWritePageCount  uintptr
	BadPageCount              uintptr
	PageCountByPriority       [8]uintptr
	RepurposedPagesByPriority [8]uintptr
	ModifiedPageCountPageFile uintptr
}

const (
	systemMemoryListInformation_ = 80
	memoryPurgeStandbyList       = 4
	memoryPurgeLowPriStandby     = 5
	pageSize                     = 4096
)

// MemoryLists implements sys.Native.
func (n *Native) MemoryLists() (sys.MemoryLists, error) {
	n.privs()
	var ms memoryStatusEx
	ms.Length = uint32(unsafe.Sizeof(ms))
	if r, _, e := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms))); r == 0 {
		return sys.MemoryLists{}, e
	}
	out := sys.MemoryLists{Total: ms.TotalPhys, Avail: ms.AvailPhys, InUse: ms.TotalPhys - ms.AvailPhys, Source: "globalmemorystatus"}
	var info systemMemoryListInformation
	var ret uint32
	r, _, _ := procNtQuerySystemInformation.Call(systemMemoryListInformation_, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), uintptr(unsafe.Pointer(&ret)))
	if r == 0 {
		var standby uint64
		for _, c := range info.PageCountByPriority {
			standby += uint64(c)
		}
		out.Standby = standby * pageSize
		out.Free = uint64(info.FreePageCount+info.ZeroPageCount) * pageSize
		out.Modified = uint64(info.ModifiedPageCount) * pageSize
		out.Source = "ntquery"
	}
	return out, nil
}

// PurgeStandbyList implements sys.Native.
func (n *Native) PurgeStandbyList(lowPriorityOnly bool) error {
	n.privs()
	cmd := uint32(memoryPurgeStandbyList)
	if lowPriorityOnly {
		cmd = memoryPurgeLowPriStandby
	}
	r, _, _ := procNtSetSystemInformation.Call(systemMemoryListInformation_, uintptr(unsafe.Pointer(&cmd)), 4)
	if r != 0 {
		return fmt.Errorf("NtSetSystemInformation: NTSTATUS 0x%08X", uint32(r))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Timer resolution
// ---------------------------------------------------------------------------

// TimerResolution implements sys.Native.
func (n *Native) TimerResolution() (sys.TimerResolution, error) {
	var coarsest, finest, cur uint32
	r, _, _ := procNtQueryTimerResolution.Call(uintptr(unsafe.Pointer(&coarsest)), uintptr(unsafe.Pointer(&finest)), uintptr(unsafe.Pointer(&cur)))
	if r != 0 {
		return sys.TimerResolution{}, fmt.Errorf("NtQueryTimerResolution: 0x%08X", uint32(r))
	}
	return sys.TimerResolution{Coarsest: coarsest, Finest: finest, Current: cur}, nil
}

// SetTimerResolution implements sys.Native.
func (n *Native) SetTimerResolution(res uint32, set bool) (uint32, error) {
	n.timerMu.Lock()
	defer n.timerMu.Unlock()
	var actual uint32
	s := uintptr(0)
	if set {
		s = 1
	}
	r, _, _ := procNtSetTimerResolution.Call(uintptr(res), s, uintptr(unsafe.Pointer(&actual)))
	if r != 0 && uint32(r) != 0xC0000245 { // STATUS_TIMER_RESOLUTION_NOT_SET on release
		return 0, fmt.Errorf("NtSetTimerResolution: 0x%08X", uint32(r))
	}
	n.timerSet = set
	return actual, nil
}

// ---------------------------------------------------------------------------
// Processes
// ---------------------------------------------------------------------------

type processMemoryCountersEx struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

// Processes implements sys.Native.
func (n *Native) Processes() ([]sys.ProcessInfo, error) {
	n.privs()
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	var out []sys.ProcessInfo
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		p := sys.ProcessInfo{PID: pe.ProcessID, PPID: pe.ParentProcessID, Name: windows.UTF16ToString(pe.ExeFile[:]), Threads: pe.Threads}
		if p.PID > 4 {
			if h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, p.PID); err == nil {
				var c, e, k, u windows.Filetime
				if windows.GetProcessTimes(h, &c, &e, &k, &u) == nil {
					ft := func(f windows.Filetime) int64 { return int64(uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime)) }
					p.CPUTime = ft(k) + ft(u)
				}
				var mc processMemoryCountersEx
				mc.Cb = uint32(unsafe.Sizeof(mc))
				if r, _, _ := procK32GetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&mc)), uintptr(mc.Cb)); r != 0 {
					p.WorkingSet = uint64(mc.WorkingSetSize)
					p.PrivateMem = uint64(mc.PrivateUsage)
				}
				buf := make([]uint16, windows.MAX_LONG_PATH)
				size := uint32(len(buf))
				if windows.QueryFullProcessImageName(h, 0, &buf[0], &size) == nil {
					p.Path = windows.UTF16ToString(buf[:size])
				}
				if pc, err := windows.GetPriorityClass(h); err == nil {
					p.Priority = pc
				}
				var sid uint32
				if windows.ProcessIdToSessionId(p.PID, &sid) == nil {
					p.Session = sid
				}
				windows.CloseHandle(h)
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// KillProcess implements sys.Native. The open handle keeps the process
// object alive, so the image checked is the one that is ended.
func (n *Native) KillProcess(pid uint32, expect string) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	if expect != "" {
		buf := make([]uint16, windows.MAX_LONG_PATH)
		size := uint32(len(buf))
		if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
			return fmt.Errorf("não foi possível conferir o processo %d: %w", pid, err)
		}
		if !sys.SameProcessImage(windows.UTF16ToString(buf[:size]), expect) {
			return fmt.Errorf("o processo %d não é mais %s", pid, expect)
		}
	}
	return windows.TerminateProcess(h, 1)
}

// SetPriorityClass implements sys.Native.
func (n *Native) SetPriorityClass(pid uint32, class uint32) error {
	h, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.SetPriorityClass(h, class)
}

const processSuspendResume = 0x0800

// SuspendProcess implements sys.Native.
func (n *Native) SuspendProcess(pid uint32) error {
	h, err := windows.OpenProcess(processSuspendResume, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	if r, _, _ := procNtSuspendProcess.Call(uintptr(h)); r != 0 {
		return fmt.Errorf("NtSuspendProcess: 0x%08X", uint32(r))
	}
	return nil
}

// ResumeProcess implements sys.Native.
func (n *Native) ResumeProcess(pid uint32) error {
	h, err := windows.OpenProcess(processSuspendResume, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	if r, _, _ := procNtResumeProcess.Call(uintptr(h)); r != 0 {
		return fmt.Errorf("NtResumeProcess: 0x%08X", uint32(r))
	}
	return nil
}

// PIDsByName implements sys.Native.
func (n *Native) PIDsByName(name string) []uint32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	var out []uint32
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), name) {
			out = append(out, pe.ProcessID)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// ICMP
// ---------------------------------------------------------------------------

type ipOptionInformation struct {
	TTL         uint8
	TOS         uint8
	Flags       uint8
	OptionsSize uint8
	_           uint32
	OptionsData uintptr
}

type icmpEchoReply struct {
	Address       uint32
	Status        uint32
	RoundTripTime uint32
	DataSize      uint16
	Reserved      uint16
	Data          uintptr
	Options       ipOptionInformation
}

const (
	ipFlagDF            = 0x2
	ipSuccess           = 0
	ipBufTooSmall       = 11001
	ipDestNetUnreach    = 11002
	ipDestHostUnreach   = 11003
	ipPacketTooBig      = 11009
	ipReqTimedOut       = 11010
	ipTTLExpiredTransit = 11013
)

// Ping implements sys.Native (one IPv4 echo).
func (n *Native) Ping(ip string, ttl int, size int, dontFragment bool, timeout time.Duration) (sys.PingReply, error) {
	addr := net.ParseIP(ip).To4()
	if addr == nil {
		return sys.PingReply{}, fmt.Errorf("endereço IPv4 inválido: %s", ip)
	}
	h, _, e := procIcmpCreateFile.Call()
	if h == uintptr(windows.InvalidHandle) || h == 0 {
		return sys.PingReply{}, e
	}
	defer procIcmpCloseHandle.Call(h)
	if size < 0 {
		size = 0
	}
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte('a' + i%23)
	}
	opt := ipOptionInformation{TTL: 128}
	if ttl > 0 {
		opt.TTL = uint8(ttl)
	}
	if dontFragment {
		opt.Flags = ipFlagDF
	}
	replySize := int(unsafe.Sizeof(icmpEchoReply{})) + size + 8 + 64
	reply := make([]byte, replySize)
	dst := binary.LittleEndian.Uint32(addr)
	// A real pointer until the call itself: converted to uintptr inside the
	// Call expression, the buffer stays alive and on the heap while the API
	// reads it (a uintptr taken earlier could go stale).
	var data unsafe.Pointer
	if size > 0 {
		data = unsafe.Pointer(&payload[0])
	}
	// QueryPerformanceCounter, not time.Now: Go's monotonic clock on Windows
	// moves only at the timer tick (up to 15.6 ms).
	start := n.Monotonic()
	r, _, callErr := procIcmpSendEcho.Call(h, uintptr(dst), uintptr(data), uintptr(size), uintptr(unsafe.Pointer(&opt)), uintptr(unsafe.Pointer(&reply[0])), uintptr(replySize), uintptr(timeout.Milliseconds()))
	elapsed := n.Monotonic() - start
	runtime.KeepAlive(payload)
	if r == 0 {
		code := uint32(0)
		if errno, ok := callErr.(syscall.Errno); ok {
			code = uint32(errno)
		}
		switch code {
		case ipReqTimedOut:
			return sys.PingReply{Timeout: true, Status: code}, nil
		case ipPacketTooBig:
			return sys.PingReply{TooBig: true, Status: code}, nil
		}
		return sys.PingReply{Status: code, Timeout: code == 0}, nil
	}
	rep := (*icmpEchoReply)(unsafe.Pointer(&reply[0]))
	var from [4]byte
	binary.LittleEndian.PutUint32(from[:], rep.Address)
	out := sys.PingReply{From: net.IP(from[:]).String(), Status: rep.Status}
	switch rep.Status {
	case ipSuccess:
		out.OK = true
		// The counter gives sub-millisecond precision (the API's
		// RoundTripTime is in whole milliseconds and 0 below 1 ms).
		out.RTT = elapsed
		if rt := time.Duration(rep.RoundTripTime) * time.Millisecond; rt > out.RTT {
			out.RTT = rt
		}
	case ipTTLExpiredTransit:
		out.TTLHop = true
		out.RTT = elapsed
	case ipPacketTooBig:
		out.TooBig = true
	case ipReqTimedOut:
		out.Timeout = true
	}
	return out, nil
}

// BestInterface returns the index of the interface Windows routes ip through
// (IPv4): the adapter the traffic really takes, even with VPNs, virtual
// switches or Wi-Fi and cable connected at once.
func (n *Native) BestInterface(ip string) (int, error) {
	addr := net.ParseIP(ip).To4()
	if addr == nil {
		return 0, fmt.Errorf("endereço IPv4 inválido: %s", ip)
	}
	var idx uint32
	// IPAddr is in network byte order, the same encoding Ping uses.
	r, _, _ := procGetBestInterface.Call(uintptr(binary.LittleEndian.Uint32(addr)), uintptr(unsafe.Pointer(&idx)))
	if r != 0 {
		return 0, syscall.Errno(r)
	}
	return int(idx), nil
}

var qpcHz = sync.OnceValue(qpcFreq)

// Monotonic implements sys.Native with QueryPerformanceCounter.
func (n *Native) Monotonic() time.Duration {
	c, f := qpc(), qpcHz()
	if f <= 0 {
		return time.Duration(time.Now().UnixNano())
	}
	// Split to avoid overflowing int64 (counter * 1e9).
	return time.Duration(c/f)*time.Second + time.Duration(c%f)*time.Second/time.Duration(f)
}

// ---------------------------------------------------------------------------
// Misc
// ---------------------------------------------------------------------------

const (
	spiSetMouse       = 0x0004
	spifUpdateIniFile = 0x01
	spifSendChange    = 0x02
)

// SetMouseParams implements sys.Native.
func (n *Native) SetMouseParams(t1, t2, speed int) error {
	params := [3]int32{int32(t1), int32(t2), int32(speed)}
	// SPIF_UPDATEINIFILE writes the process account's profile: when Axion
	// runs as another administrator, the values are persisted through the
	// registry of the person signed in (redirected HKCU) and this call only
	// applies them to the live session.
	flags := uintptr(spifUpdateIniFile | spifSendChange)
	if consoleUser().Differs {
		flags = spifSendChange
	}
	r, _, e := procSystemParametersInfoW.Call(spiSetMouse, 0, uintptr(unsafe.Pointer(&params[0])), flags)
	if r == 0 {
		return e
	}
	return nil
}

// BroadcastSettingChange implements sys.Native.
func (n *Native) BroadcastSettingChange() {
	for _, area := range []string{"Policy", "Environment", "ImmersiveColorSet"} {
		p, _ := windows.UTF16PtrFromString(area)
		var res uintptr
		procSendMessageTimeoutW.Call(0xFFFF, 0x001A, 0, uintptr(unsafe.Pointer(p)), 0x0002, 2000, uintptr(unsafe.Pointer(&res)))
	}
}

// OpenURL implements sys.Native. explorer.exe opens the URL in the user's
// normal (non-elevated) browser.
func (n *Native) OpenURL(u string) error {
	// Folders from the API, not from %SystemRoot%: a program without
	// administrator rights can set user environment variables that the
	// elevated Axion inherits.
	sys32, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	win, err := windows.GetWindowsDirectory()
	if err != nil {
		return err
	}
	explorer := filepath.Join(win, "explorer.exe")
	tools := map[string][]string{
		"tool:devmgmt":  {filepath.Join(sys32, "mmc.exe"), "devmgmt.msc"},
		"tool:rstrui":   {filepath.Join(sys32, "rstrui.exe")},
		"tool:sysprot":  {filepath.Join(sys32, "SystemPropertiesProtection.exe")},
		"tool:eventvwr": {filepath.Join(sys32, "eventvwr.exe")},
	}
	if t, ok := tools[u]; ok {
		return exec.Command(t[0], t[1:]...).Start()
	}
	if strings.HasPrefix(u, "folder:") {
		p := strings.TrimPrefix(u, "folder:")
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			return errors.New("pasta não encontrada")
		}
		return exec.Command(explorer, p).Start()
	}
	if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "ms-settings:") && !sys.IsSteamUninstallURL(u) {
		return errors.New("URL não permitida")
	}
	// explorer.exe hands the URL to the user's shell, so the browser does not
	// inherit Axion's elevated token.
	cmd := exec.Command(explorer, u)
	return cmd.Start()
}

// FileVersion implements sys.Native.
func (n *Native) FileVersion(path string) string {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return ""
	}
	var fixed *windows.VS_FIXEDFILEINFO
	var l uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\`, unsafe.Pointer(&fixed), &l); err != nil || fixed == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", fixed.FileVersionMS>>16, fixed.FileVersionMS&0xFFFF, fixed.FileVersionLS>>16, fixed.FileVersionLS&0xFFFF)
}

// CPUTimes implements sys.Native (100-ns units).
func (n *Native) CPUTimes() (uint64, uint64, uint64, error) {
	var idle, kernel, user windows.Filetime
	r, _, e := procGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if r == 0 {
		return 0, 0, 0, e
	}
	ft := func(f windows.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }
	return ft(idle), ft(kernel), ft(user), nil
}

// CreateRestorePoint implements sys.Native through WMI (SystemRestore class,
// the same provider used by Checkpoint-Computer). The 24-hour frequency limit
// is lifted for this call and restored afterwards.
func (n *Native) CreateRestorePoint(description string) error {
	const key = `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\SystemRestore`
	prev, _ := n.Reg.GetValue(key, "SystemRestorePointCreationFrequency")
	_ = n.Reg.SetValue(key, "SystemRestorePointCreationFrequency", sys.DWord(0))
	defer func() {
		if prev.Exists {
			_ = n.Reg.SetValue(key, "SystemRestorePointCreationFrequency", prev)
		} else {
			_ = n.Reg.DeleteValue(key, "SystemRestorePointCreationFrequency")
		}
	}()
	call := func() (int64, error) {
		r, err := n.WMI.Call(`root\default`, "SystemRestore", "CreateRestorePoint", description, int32(12), int32(100))
		if err != nil {
			return -1, err
		}
		return sys.WMIRow{"v": r}.Int("v"), nil
	}
	code, err := call()
	if err == nil && code == 0 {
		return nil
	}
	// System Protection off on the system drive is the user's choice (it
	// reserves space for shadow copies): it is reported, never switched on
	// behind the user's back. The Axion history still undoes every change.
	if err != nil {
		return err
	}
	return fmt.Errorf("a Proteção do Sistema parece desligada no disco do Windows (código %d); para ter pontos de restauração, ligue-a em Painel de Controle > Sistema > Proteção do Sistema", code)
}

type shQueryRBInfo struct {
	Size     uint32
	_        uint32
	Bytes    int64
	NumItems int64
}

// RecycleBin implements sys.Native.
func (n *Native) RecycleBin() (uint64, uint64, error) {
	info := shQueryRBInfo{Size: uint32(unsafe.Sizeof(shQueryRBInfo{}))}
	r, _, _ := procSHQueryRecycleBinW.Call(0, uintptr(unsafe.Pointer(&info)))
	if r != 0 {
		return 0, 0, fmt.Errorf("SHQueryRecycleBin: 0x%08X", uint32(r))
	}
	return uint64(info.Bytes), uint64(info.NumItems), nil
}

// EmptyRecycleBin implements sys.Native.
func (n *Native) EmptyRecycleBin() error {
	r, _, _ := procSHEmptyRecycleBinW.Call(0, 0, 0x1|0x2|0x4)
	if r != 0 && uint32(r) != 0x8000FFFF { // E_UNEXPECTED when already empty
		return fmt.Errorf("SHEmptyRecycleBin: 0x%08X", uint32(r))
	}
	return nil
}

// Adapters implements sys.Native (GetAdaptersAddresses).
func (n *Native) Adapters() ([]sys.NetAdapter, error) {
	size := uint32(32 * 1024)
	var buf []byte
	var err error
	for i := 0; i < 4; i++ {
		buf = make([]byte, size)
		err = windows.GetAdaptersAddresses(windows.AF_UNSPEC, windows.GAA_FLAG_INCLUDE_GATEWAYS, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil || !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			break
		}
	}
	if err != nil {
		// Never walk a buffer the call did not fill.
		return nil, err
	}
	var out []sys.NetAdapter
	for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); a != nil; a = a.Next {
		na := sys.NetAdapter{
			Name: windows.UTF16PtrToString(a.FriendlyName), Description: windows.UTF16PtrToString(a.Description),
			GUID: windows.BytePtrToString(a.AdapterName), IfIndex: int(a.IfIndex), IfType: int(a.IfType),
			Up: a.OperStatus == windows.IfOperStatusUp, SpeedBps: a.TransmitLinkSpeed, MTU: int(a.Mtu),
			DHCP: a.Flags&ipAdapterDHCPEnabled != 0,
		}
		if a.PhysicalAddressLength > 0 {
			var parts []string
			for i := uint32(0); i < a.PhysicalAddressLength && i < 8; i++ {
				parts = append(parts, fmt.Sprintf("%02X", a.PhysicalAddress[i]))
			}
			na.MAC = strings.Join(parts, "-")
		}
		for g := a.FirstGatewayAddress; g != nil; g = g.Next {
			if ip := g.Address.IP(); ip != nil && ip.To4() != nil {
				na.Gateways = append(na.Gateways, ip.String())
			}
		}
		for d := a.FirstDnsServerAddress; d != nil; d = d.Next {
			if ip := d.Address.IP(); ip != nil {
				na.DNS = append(na.DNS, ip.String())
			}
		}
		for u := a.FirstUnicastAddress; u != nil; u = u.Next {
			if ip := u.Address.IP(); ip != nil && ip.To4() != nil {
				na.IPv4 = append(na.IPv4, ip.String())
			}
		}
		desc := strings.ToLower(na.Description)
		virtual := strings.Contains(desc, "virtual") || strings.Contains(desc, "hyper-v") || strings.Contains(desc, "vpn") ||
			strings.Contains(desc, "tap-") || strings.Contains(desc, "wintun") || strings.Contains(desc, "loopback") ||
			strings.Contains(desc, "bluetooth") || strings.Contains(desc, "vmware") || strings.Contains(desc, "virtualbox") ||
			strings.Contains(desc, "zerotier") || strings.Contains(desc, "tailscale") || strings.Contains(desc, "radmin") || strings.Contains(desc, "hamachi")
		na.Physical = (na.IfType == 6 || na.IfType == 71) && a.PhysicalAddressLength == 6 && !virtual
		out = append(out, na)
	}
	return out, nil
}

// adapterExists tells whether an adapter is known to Windows. One turned off
// in Network Connections disappears from GetAdaptersAddresses but keeps its
// static configuration under Tcpip\Parameters\Interfaces, so that key counts
// too: the DNS written there must still be readable and restorable.
func (n *Native) adapterExists(guid string) bool {
	if list, err := n.Adapters(); err == nil {
		for _, a := range list {
			if strings.EqualFold(a.GUID, guid) {
				return true
			}
		}
	} else {
		return true // let the API call report the real error
	}
	if ok, err := n.Reg.KeyExists(keyTcpipIfaces + guid); err == nil && ok {
		return true
	}
	return false
}

// DNSConfig implements sys.Native (GetInterfaceDnsSettings, Windows 10 2004+).
func (n *Native) DNSConfig(guid string) (sys.DNSConfig, error) {
	var c sys.DNSConfig
	if procGetInterfaceDnsSettings.Find() != nil {
		return c, errors.New("GetInterfaceDnsSettings não disponível (requer Windows 10 2004 ou mais novo)")
	}
	if !n.adapterExists(guid) {
		return c, sys.ErrNotFound
	}
	g, err := parseGUID(guid)
	if err != nil {
		return c, err
	}
	// The GUID is passed by value in C; the x64 ABI passes a 16-byte struct by
	// reference to a copy, so the pointer is the right argument (amd64 only:
	// build.ps1 builds GOARCH=amd64).
	// Version 3 also says whether DNS over HTTPS is set (build 19645+). Learn:
	// "set only the Version member; Flags must be empty".
	s3 := dnsInterfaceSettings3{Version: dnsInterfaceSettingsVersion3}
	if r, _, _ := procGetInterfaceDnsSettings.Call(uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&s3))); r == 0 {
		c.V4 = splitServers(windows.UTF16PtrToString(s3.NameServer))
		c.DoH = s3.CServerProperties > 0 || s3.CProfileServerProperties > 0
		procFreeInterfaceDnsSettings.Call(uintptr(unsafe.Pointer(&s3)))
	} else {
		s1 := dnsInterfaceSettings{Version: dnsInterfaceSettingsVersion1}
		r, _, _ := procGetInterfaceDnsSettings.Call(uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&s1)))
		if r != 0 {
			return c, fmt.Errorf("GetInterfaceDnsSettings: %w", syscall.Errno(r))
		}
		c.V4 = splitServers(windows.UTF16PtrToString(s1.NameServer))
		procFreeInterfaceDnsSettings.Call(uintptr(unsafe.Pointer(&s1)))
	}
	// The static IPv4 list lives in the NameServer value of the adapter under
	// Tcpip\Parameters\Interfaces (empty on DHCP). If the registry says there
	// is none, the API must not hand back the servers the router gave: they
	// would be pinned as static when the DNS is restored.
	if len(c.V4) > 0 {
		if v, err := n.Reg.GetValue(keyTcpipIfaces+guid, "NameServer"); err == nil && len(splitServers(v.Str)) == 0 {
			c.V4 = nil
		}
	}
	// IPv6: Get is documented only for the IPv4 view (Flags must be empty), so
	// the static IPv6 list is read where the IPv6 stack keeps it.
	if v, err := n.Reg.GetValue(keyTcpip6Ifaces+guid, "NameServer"); err == nil && v.Exists {
		c.V6 = splitServers(v.Str)
	}
	if !c.DoH {
		for _, sub := range []string{`\Doh`, `\Doh6`} {
			if ks, err := n.Reg.SubKeys(keyDohIfaces(guid) + sub); err == nil && len(ks) > 0 {
				c.DoH = true
			}
		}
	}
	return c, nil
}

func sameServers(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !net.ParseIP(a[i]).Equal(net.ParseIP(b[i])) {
			return false
		}
	}
	return true
}

// SetDNSConfig implements sys.Native (SetInterfaceDnsSettings, Windows 10
// 2004+). Only the stack whose list differs from the current one is written,
// so asking for what is already there changes nothing.
func (n *Native) SetDNSConfig(guid string, v4, v6 []string) error {
	if procSetInterfaceDnsSettings.Find() != nil {
		return errors.New("SetInterfaceDnsSettings não disponível (requer Windows 10 2004 ou mais novo)")
	}
	for _, s := range v4 {
		if ip := net.ParseIP(s); ip == nil || ip.To4() == nil {
			return fmt.Errorf("servidor DNS IPv4 inválido: %s", s)
		}
	}
	for _, s := range v6 {
		if ip := net.ParseIP(s); ip == nil || ip.To4() != nil {
			return fmt.Errorf("servidor DNS IPv6 inválido: %s", s)
		}
	}
	g, err := parseGUID(guid)
	if err != nil {
		return err
	}
	cur, err := n.DNSConfig(guid)
	if err != nil {
		return err
	}
	set := func(flags uint64, list []string) error {
		// Learn: set the options in Flags, fill only those fields, zero the
		// rest. An empty string clears the static list (automatic).
		ns, err := windows.UTF16PtrFromString(strings.Join(list, ","))
		if err != nil {
			return err
		}
		st := dnsInterfaceSettings{Version: dnsInterfaceSettingsVersion1, Flags: flags, NameServer: ns}
		r, _, _ := procSetInterfaceDnsSettings.Call(uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&st)))
		runtime.KeepAlive(ns)
		if r != 0 {
			return fmt.Errorf("SetInterfaceDnsSettings: %w", syscall.Errno(r))
		}
		return nil
	}
	if !sameServers(cur.V4, v4) {
		if err := set(dnsSettingNameServer, v4); err != nil {
			return fmt.Errorf("IPv4: %w", err)
		}
	}
	if !sameServers(cur.V6, v6) {
		if err := set(dnsSettingNameServer|dnsSettingIPv6, v6); err != nil {
			// IPv4 may already have changed: the engine restores the snapshot.
			return fmt.Errorf("IPv6 (o IPv6 pode estar desligado neste adaptador): %w", err)
		}
	}
	// Read it back: an empty list must mean automatic. If the Windows kept a
	// list, say so instead of reporting a restore that did not happen.
	after, err := n.DNSConfig(guid)
	if err != nil {
		return err
	}
	if !sameServers(after.V4, v4) || !sameServers(after.V6, v6) {
		return fmt.Errorf("o Windows não gravou a lista pedida (IPv4: %v; IPv6: %v)", after.V4, after.V6)
	}
	return nil
}

type systemPowerStatus struct {
	ACLineStatus        uint8
	BatteryFlag         uint8
	BatteryLifePercent  uint8
	SystemStatusFlag    uint8
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

// PowerStatus implements sys.Native.
func (n *Native) PowerStatus() (bool, bool, int, error) {
	var s systemPowerStatus
	r, _, e := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&s)))
	if r == 0 {
		return false, true, 0, e
	}
	hasBattery := s.BatteryFlag != 128 && s.BatteryFlag != 255
	pct := int(s.BatteryLifePercent)
	if pct > 100 {
		pct = -1
	}
	return hasBattery, s.ACLineStatus != 0, pct, nil
}

// DiskSpace implements sys.Native.
func (n *Native) DiskSpace(path string) (uint64, uint64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, 0, err
	}
	return total, free, nil
}

// Resolve implements sys.Native.
func (n *Native) Resolve(host string) ([]string, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []string{host}, nil
	}
	addrs, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, a := range addrs {
		if a.To4() != nil {
			out = append(out, a.String())
		}
	}
	return out, nil
}

// DNSQuery implements sys.Native: one A query sent straight to server:53.
func (n *Native) DNSQuery(server, host string, timeout time.Duration) (time.Duration, error) {
	conn, err := net.DialTimeout("udp", net.JoinHostPort(server, "53"), timeout)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	id := uint16(rand.Intn(65535))
	q := make([]byte, 12, 64)
	binary.BigEndian.PutUint16(q[0:], id)
	binary.BigEndian.PutUint16(q[2:], 0x0100) // recursion desired
	binary.BigEndian.PutUint16(q[4:], 1)      // one question
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		q = append(q, byte(len(label)))
		q = append(q, label...)
	}
	q = append(q, 0, 0, 1, 0, 1) // type A, class IN
	_ = conn.SetDeadline(time.Now().Add(timeout))
	start := time.Now()
	if _, err := conn.Write(q); err != nil {
		return 0, err
	}
	resp := make([]byte, 1500)
	for {
		nr, err := conn.Read(resp)
		if err != nil {
			return 0, err
		}
		if nr >= 12 && binary.BigEndian.Uint16(resp[0:]) == id {
			rcode := resp[3] & 0x0F
			if rcode != 0 && rcode != 3 {
				return 0, fmt.Errorf("resposta DNS com erro (rcode %d)", rcode)
			}
			return time.Since(start), nil
		}
	}
}

// MovePath implements sys.Native (rename, or copy+delete across volumes).
func (n *Native) MovePath(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyTree(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

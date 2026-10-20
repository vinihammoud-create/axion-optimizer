// Package sys defines the operating-system surface Axion touches.
//
// Every read or write the optimizer performs on the machine goes through the
// interfaces in this package. The real implementation lives in package winsys
// (Windows only); package mocksys provides an in-memory implementation used by
// unit tests and by the UI preview server on non-Windows hosts. Keeping this
// boundary explicit is what makes the tweak engine testable: backup, apply,
// verify and revert logic runs against the mock exactly as it runs on Windows.
package sys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound is returned when a registry key/value, service, scheme, etc. does not exist.
var ErrNotFound = errors.New("not found")

// ErrTimeout is returned by Runner.Run when RunOpts.Timeout expired and the
// process was stopped.
var ErrTimeout = errors.New("tempo esgotado")

// ErrUnsupported is returned when an operation is not available on this platform/build.
var ErrUnsupported = errors.New("unsupported on this system")

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

// RegKind mirrors the Windows registry value types Axion uses.
type RegKind int

const (
	RegNone RegKind = iota
	RegSZ
	RegExpandSZ
	RegBinary
	RegDWord
	RegMultiSZ
	RegQWord
)

func (k RegKind) String() string {
	switch k {
	case RegSZ:
		return "REG_SZ"
	case RegExpandSZ:
		return "REG_EXPAND_SZ"
	case RegBinary:
		return "REG_BINARY"
	case RegDWord:
		return "REG_DWORD"
	case RegMultiSZ:
		return "REG_MULTI_SZ"
	case RegQWord:
		return "REG_QWORD"
	}
	return "REG_NONE"
}

// ParseRegKind converts "REG_DWORD" (or "DWORD") into a RegKind.
func ParseRegKind(s string) RegKind {
	s = strings.ToUpper(strings.TrimPrefix(strings.ToUpper(s), "REG_"))
	switch s {
	case "SZ":
		return RegSZ
	case "EXPAND_SZ":
		return RegExpandSZ
	case "BINARY":
		return RegBinary
	case "DWORD":
		return RegDWord
	case "MULTI_SZ":
		return RegMultiSZ
	case "QWORD":
		return RegQWord
	}
	return RegNone
}

// RegValue is a typed registry value. Exists=false means the value is absent.
type RegValue struct {
	Exists bool     `json:"exists"`
	Kind   RegKind  `json:"kind"`
	Str    string   `json:"str,omitempty"`
	Strs   []string `json:"strs,omitempty"`
	Int    uint64   `json:"int,omitempty"`
	Bin    []byte   `json:"bin,omitempty"`
}

// DWord builds a REG_DWORD value.
func DWord(v uint32) RegValue { return RegValue{Exists: true, Kind: RegDWord, Int: uint64(v)} }

// QWord builds a REG_QWORD value.
func QWord(v uint64) RegValue { return RegValue{Exists: true, Kind: RegQWord, Int: v} }

// SZ builds a REG_SZ value.
func SZ(v string) RegValue { return RegValue{Exists: true, Kind: RegSZ, Str: v} }

// ExpandSZ builds a REG_EXPAND_SZ value.
func ExpandSZ(v string) RegValue { return RegValue{Exists: true, Kind: RegExpandSZ, Str: v} }

// MultiSZ builds a REG_MULTI_SZ value.
func MultiSZ(v ...string) RegValue {
	return RegValue{Exists: true, Kind: RegMultiSZ, Strs: append([]string(nil), v...)}
}

// Binary builds a REG_BINARY value.
func Binary(b []byte) RegValue {
	c := make([]byte, len(b))
	copy(c, b)
	return RegValue{Exists: true, Kind: RegBinary, Bin: c}
}

// Absent is the "value does not exist" marker.
func Absent() RegValue { return RegValue{} }

// Equal reports whether two values are the same type and data. Two absent
// values are equal. DWORD/QWORD with the same number compare equal to each
// other only when kinds match, because Windows code often reads a specific type.
func (v RegValue) Equal(o RegValue) bool {
	if !v.Exists || !o.Exists {
		return v.Exists == o.Exists
	}
	if v.Kind != o.Kind {
		return false
	}
	switch v.Kind {
	case RegSZ, RegExpandSZ:
		return v.Str == o.Str
	case RegDWord, RegQWord:
		return v.Int == o.Int
	case RegMultiSZ:
		if len(v.Strs) != len(o.Strs) {
			return false
		}
		for i := range v.Strs {
			if v.Strs[i] != o.Strs[i] {
				return false
			}
		}
		return true
	case RegBinary:
		if len(v.Bin) != len(o.Bin) {
			return false
		}
		for i := range v.Bin {
			if v.Bin[i] != o.Bin[i] {
				return false
			}
		}
		return true
	}
	return true
}

// Display renders the value for humans ("0x00000001 (1)", "\"text\"", "ausente").
func (v RegValue) Display() string {
	if !v.Exists {
		return "(ausente)"
	}
	switch v.Kind {
	case RegDWord:
		return fmt.Sprintf("0x%08X (%d)", uint32(v.Int), uint32(v.Int))
	case RegQWord:
		return fmt.Sprintf("0x%016X (%d)", v.Int, v.Int)
	case RegSZ, RegExpandSZ:
		return fmt.Sprintf("%q", v.Str)
	case RegMultiSZ:
		return fmt.Sprintf("%q", v.Strs)
	case RegBinary:
		var sb strings.Builder
		for i, b := range v.Bin {
			if i > 0 {
				sb.WriteByte(' ')
			}
			if i >= 24 {
				sb.WriteString("…")
				break
			}
			fmt.Fprintf(&sb, "%02X", b)
		}
		return sb.String()
	}
	return "?"
}

// Registry is the registry surface. Paths use the usual textual form, e.g.
// `HKLM\SOFTWARE\Microsoft\Windows` or `HKEY_CURRENT_USER\Control Panel\Mouse`.
// All access uses the native (64-bit) view.
type Registry interface {
	GetValue(path, name string) (RegValue, error) // absent value => RegValue{Exists:false}, nil
	SetValue(path, name string, v RegValue) error // creates missing keys
	DeleteValue(path, name string) error          // absent value is not an error
	KeyExists(path string) (bool, error)
	CreateKey(path string) (created bool, err error)
	DeleteKeyIfEmpty(path string) (deleted bool, err error)
	SubKeys(path string) ([]string, error)
	ValueNames(path string) ([]string, error)
}

// RegRoot identifies a hive.
type RegRoot int

const (
	RootUnknown RegRoot = iota
	RootHKLM
	RootHKCU
	RootHKU
	RootHKCR
)

// SplitRegPath returns the hive and the sub path (without leading backslash).
func SplitRegPath(p string) (RegRoot, string, error) {
	p = strings.TrimSpace(strings.ReplaceAll(p, "/", `\`))
	idx := strings.IndexByte(p, '\\')
	head, rest := p, ""
	if idx >= 0 {
		head, rest = p[:idx], p[idx+1:]
	}
	rest = strings.Trim(rest, `\`)
	switch strings.ToUpper(head) {
	case "HKLM", "HKEY_LOCAL_MACHINE":
		return RootHKLM, rest, nil
	case "HKCU", "HKEY_CURRENT_USER":
		return RootHKCU, rest, nil
	case "HKU", "HKEY_USERS":
		return RootHKU, rest, nil
	case "HKCR", "HKEY_CLASSES_ROOT":
		return RootHKCR, rest, nil
	}
	return RootUnknown, "", fmt.Errorf("caminho de registro inválido: %q", p)
}

// NormalizeRegPath produces a canonical `HKLM\...` form for comparisons and storage.
func NormalizeRegPath(p string) string {
	root, rest, err := SplitRegPath(p)
	if err != nil {
		return p
	}
	var h string
	switch root {
	case RootHKLM:
		h = "HKLM"
	case RootHKCU:
		h = "HKCU"
	case RootHKU:
		h = "HKU"
	case RootHKCR:
		h = "HKCR"
	}
	if rest == "" {
		return h
	}
	return h + `\` + rest
}

// ---------------------------------------------------------------------------
// Services
// ---------------------------------------------------------------------------

// ServiceStart is the start type of a Windows service.
type ServiceStart int

const (
	StartUnknown ServiceStart = iota
	StartBoot
	StartSystem
	StartAuto
	StartAutoDelayed
	StartManual
	StartDisabled
)

func (s ServiceStart) String() string {
	switch s {
	case StartBoot:
		return "Boot"
	case StartSystem:
		return "System"
	case StartAuto:
		return "Automático"
	case StartAutoDelayed:
		return "Automático (atraso)"
	case StartManual:
		return "Manual"
	case StartDisabled:
		return "Desativado"
	}
	return "Desconhecido"
}

// ServiceInfo describes a service.
type ServiceInfo struct {
	Name        string       `json:"name"`
	DisplayName string       `json:"displayName"`
	Exists      bool         `json:"exists"`
	Start       ServiceStart `json:"start"`
	Running     bool         `json:"running"`
}

// Services is the service-control surface.
type Services interface {
	Query(name string) (ServiceInfo, error) // missing service => Exists=false, nil
	SetStart(name string, start ServiceStart) error
	Stop(ctx context.Context, name string) error
	Start(name string) error
}

// ---------------------------------------------------------------------------
// Power management (powrprof)
// ---------------------------------------------------------------------------

// Well-known power scheme GUIDs.
const (
	SchemeBalanced         = "381b4222-f694-41f0-9685-ff5bb260df2e"
	SchemeHighPerformance  = "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c"
	SchemePowerSaver       = "a1841308-3541-4fab-bc81-f71556f20b4a"
	SchemeUltimateTemplate = "e9a42b02-d5df-448d-aa00-03f14749eb61"
)

// PowerScheme is a power plan.
type PowerScheme struct {
	GUID string `json:"guid"`
	Name string `json:"name"`
}

// Power exposes power plan reads/writes through the Power* APIs (language independent).
type Power interface {
	ActiveScheme() (string, error)
	SetActiveScheme(guid string) error
	Schemes() ([]PowerScheme, error)
	// ReadValue reads the AC (ac=true) or DC value index of a setting in a scheme.
	ReadValue(scheme, subgroup, setting string, ac bool) (uint32, error)
	WriteValue(scheme, subgroup, setting string, ac bool, value uint32) error
	DuplicateScheme(source string) (string, error)
	DeleteScheme(guid string) error
	// ActiveOverlay returns the Windows "Power mode" overlay applied on AC
	// ("" means the default "Balanced"/recommended mode).
	ActiveOverlay() (string, error)
	SetActiveOverlay(guid string) error
}

// Power mode overlays (Settings > System > Power > Power mode).
const (
	OverlayBestEfficiency  = "961cc777-2547-4f9d-8174-7d86181b8a7a"
	OverlayBalanced        = "00000000-0000-0000-0000-000000000000"
	OverlayBestPerformance = "ded574b5-45a0-4f42-8737-46345c09c238"
)

// ---------------------------------------------------------------------------
// External commands
// ---------------------------------------------------------------------------

// RunOpts configures a command execution.
type RunOpts struct {
	Timeout time.Duration
	Dir     string
	Env     []string
	Stdin   string
	// OnLine, when set, receives decoded output lines as they arrive (stdout and stderr).
	OnLine func(line string)
}

// Result is the outcome of an external command.
type Result struct {
	ExitCode int           `json:"exitCode"`
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	Duration time.Duration `json:"duration"`
}

// Runner starts built-in Windows tools that have no public API (bcdedit,
// sfc, DISM, chkdsk, netsh, winget...). Processes run hidden (no console
// window) and their output is decoded from the console code page.
// Axion never runs PowerShell or batch scripts.
type Runner interface {
	Run(ctx context.Context, exe string, args []string, opts RunOpts) (Result, error)
}

// ---------------------------------------------------------------------------
// WMI (COM, no PowerShell)
// ---------------------------------------------------------------------------

// WMIRow is one WMI object: property name -> value (string, int64, uint64,
// float64, bool, time.Time, []any or nil).
type WMIRow map[string]any

// Str returns a property as string.
func (r WMIRow) Str(k string) string {
	switch v := r[k].(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}

// Int returns a property as int64 (0 when absent or not numeric).
func (r WMIRow) Int(k string) int64 {
	switch v := r[k].(type) {
	case int64:
		return v
	case int32:
		return int64(v)
	case int:
		return int64(v)
	case uint64:
		return int64(v)
	case uint32:
		return int64(v)
	case uint16:
		return int64(v)
	case uint8:
		return int64(v)
	case int16:
		return int64(v)
	case float64:
		return int64(v)
	case bool:
		if v {
			return 1
		}
		return 0
	case string:
		var n int64
		_, err := fmt.Sscan(strings.TrimSpace(v), &n)
		if err == nil {
			return n
		}
	}
	return 0
}

// Uint returns a property as uint64.
func (r WMIRow) Uint(k string) uint64 {
	switch v := r[k].(type) {
	case uint64:
		return v
	case string:
		var n uint64
		if _, err := fmt.Sscan(strings.TrimSpace(v), &n); err == nil {
			return n
		}
		return 0
	}
	n := r.Int(k)
	if n < 0 {
		return 0
	}
	return uint64(n)
}

// Bool returns a property as bool.
func (r WMIRow) Bool(k string) bool {
	switch v := r[k].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	}
	return r.Int(k) != 0
}

// Time returns a property as time (WMI DATETIME strings are parsed).
func (r WMIRow) Time(k string) time.Time {
	switch v := r[k].(type) {
	case time.Time:
		return v
	case string:
		return ParseWMITime(v)
	}
	return time.Time{}
}

// Strings returns an array property as []string.
func (r WMIRow) Strings(k string) []string {
	switch v := r[k].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			out = append(out, fmt.Sprint(x))
		}
		return out
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	}
	return nil
}

// Ints returns an array property as []int64.
func (r WMIRow) Ints(k string) []int64 {
	arr, ok := r[k].([]any)
	if !ok {
		if n := r.Int(k); n != 0 {
			return []int64{n}
		}
		return nil
	}
	out := make([]int64, 0, len(arr))
	for _, x := range arr {
		out = append(out, WMIRow{"v": x}.Int("v"))
	}
	return out
}

// ParseWMITime parses the CIM DATETIME format yyyymmddHHMMSS.mmmmmmsUUU.
func ParseWMITime(s string) time.Time {
	if len(s) < 14 {
		return time.Time{}
	}
	t, err := time.ParseInLocation("20060102150405", s[:14], time.Local)
	if err != nil {
		return time.Time{}
	}
	if len(s) >= 25 {
		var off int
		if _, err := fmt.Sscan(s[22:25], &off); err == nil {
			sign := 1
			if s[21] == '-' {
				sign = -1
			}
			loc := time.FixedZone("", sign*off*60)
			t, _ = time.ParseInLocation("20060102150405", s[:14], loc)
		}
	}
	return t
}

// WMI executes queries and methods through the WMI COM API.
type WMI interface {
	// Query runs a WQL query in a namespace (e.g. `root\cimv2`).
	Query(namespace, wql string) ([]WMIRow, error)
	// Call invokes a method on an object path or class (e.g.
	// `Win32_NetworkAdapter.DeviceID="7"`, method "Disable") with positional
	// arguments and returns the method's return value.
	Call(namespace, objectPath, method string, args ...any) (any, error)
	// CallOut invokes a method (a static one when objectPath is a class name,
	// as in PS_MMAgent) with NAMED input parameters and returns what Call
	// cannot: the output parameters. The row holds "ReturnValue" plus one entry
	// per output parameter; an output that is an embedded object (an instance
	// returned by value) is a nested WMIRow. Call passes positional arguments
	// and returns only the return value, so it cannot read an out parameter.
	CallOut(namespace, objectPath, method string, in map[string]any) (WMIRow, error)
	// Put assigns property=value on every object returned by the WQL query and
	// saves it (SWbemObject.Put_). It returns how many objects were written.
	// The query must be "SELECT *": Put_ writes every property back, and a
	// partial instance (SELECT a, b) is refused by providers such as the WDM
	// one behind MSPower_DeviceEnable ("Objeto inválido", real report 0.3.2).
	Put(namespace, wql, property string, value any) (int, error)
}

// ---------------------------------------------------------------------------
// Task Scheduler (COM Schedule.Service)
// ---------------------------------------------------------------------------

// Trigger types (TASK_TRIGGER_TYPE2).
const (
	TriggerEvent   = 0
	TriggerTime    = 1
	TriggerDaily   = 2
	TriggerWeekly  = 3
	TriggerMonthly = 4
	TriggerIdle    = 6
	// TriggerRegistration runs the task as soon as it is registered (used to
	// start a program with the user's normal, non-elevated rights).
	TriggerRegistration = 7
	TriggerBoot         = 8
	TriggerLogon        = 9
)

// Task states (TASK_STATE).
const (
	TaskUnknown  = 0
	TaskDisabled = 1
	TaskQueued   = 2
	TaskReady    = 3
	TaskRunning  = 4
)

// TaskInfo describes a scheduled task.
type TaskInfo struct {
	Path     string   `json:"path"` // \Folder\Name
	Name     string   `json:"name"`
	Exists   bool     `json:"exists"`
	Enabled  bool     `json:"enabled"`
	State    int      `json:"state"`
	Author   string   `json:"author,omitempty"`
	Triggers []int    `json:"triggers,omitempty"`
	Actions  []string `json:"actions,omitempty"` // executable paths (+ arguments)
	Hidden   bool     `json:"hidden,omitempty"`
	// LastRun is zero when the task never ran; LastResult is the HRESULT of
	// the last run (0 = success).
	LastRun    time.Time `json:"lastRun,omitempty"`
	LastResult int       `json:"lastResult"`
}

// Tasks manages scheduled tasks.
type Tasks interface {
	Get(path string) (TaskInfo, error) // missing => Exists=false, nil
	SetEnabled(path string, enabled bool) error
	// List enumerates tasks under folder (recursive when requested).
	List(folder string, recursive bool) ([]TaskInfo, error)
	// Register creates/replaces an Axion task that runs exe with args.
	// trigger: TriggerLogon, TriggerDaily (at hh:mm) or TriggerWeekly (day+hh:mm).
	Register(path, description, exe, args string, trigger int, at string, weekday int, highest bool) error
	Delete(path string) error
}

// ---------------------------------------------------------------------------
// Event log (wevtapi)
// ---------------------------------------------------------------------------

// Event is one Windows event log record.
type Event struct {
	Channel  string            `json:"channel"`
	Provider string            `json:"provider"`
	ID       int               `json:"id"`
	Level    int               `json:"level"` // 1 critical, 2 error, 3 warning, 4 info
	Time     time.Time         `json:"time"`
	Message  string            `json:"message,omitempty"` // rendered data (best effort)
	Data     map[string]string `json:"data,omitempty"`
}

// ProcPower is one logical processor in CallNtPowerInformation
// (PROCESSOR_POWER_INFORMATION), in MHz.
type ProcPower struct {
	MaxMhz     int `json:"maxMhz"`
	CurrentMhz int `json:"currentMhz"`
	MhzLimit   int `json:"mhzLimit"`
}

// EventLog queries event channels with XPath.
type EventLog interface {
	// Query returns up to max events (newest first) matching the XPath query,
	// e.g. `*[System[Provider[@Name='Microsoft-Windows-WHEA-Logger']]]`.
	Query(channel, xpath string, max int) ([]Event, error)
}

// ---------------------------------------------------------------------------
// Native helpers (things with no command-line equivalent)
// ---------------------------------------------------------------------------

// DisplayMode describes one monitor's current and best supported refresh rate.
type DisplayMode struct {
	Device     string `json:"device"`  // \\.\DISPLAY1
	Monitor    string `json:"monitor"` // friendly monitor string
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	CurrentHz  int    `json:"currentHz"`
	MaxHzAtRes int    `json:"maxHzAtRes"` // highest refresh available at the current resolution
	MaxHzAny   int    `json:"maxHzAny"`   // highest refresh available at any resolution
	BitsPerPel int    `json:"bitsPerPel"`
	Primary    bool   `json:"primary"`
	// Internal: the built-in panel of a notebook or all-in-one (eDP/LVDS
	// output), from the display configuration Windows reports.
	Internal bool `json:"internal,omitempty"`
}

// MemoryLists is a snapshot of physical memory usage (bytes).
type MemoryLists struct {
	Total    uint64 `json:"total"`
	Avail    uint64 `json:"avail"`
	Free     uint64 `json:"free"`    // free + zeroed pages
	Standby  uint64 `json:"standby"` // sum of standby priorities
	Modified uint64 `json:"modified"`
	InUse    uint64 `json:"inUse"`
	Source   string `json:"source"` // "ntquery" or "globalmemorystatus"
}

// TimerResolution in 100-ns units (156250 = 15.625 ms, 5000 = 0.5 ms).
type TimerResolution struct {
	Coarsest uint32 `json:"coarsest"` // largest interval the system supports
	Finest   uint32 `json:"finest"`   // smallest interval the system supports
	Current  uint32 `json:"current"`
}

// SameProcessImage reports whether a process image path is the expected
// program: a full path (compared whole) or an executable name (compared with
// the image's file name), ignoring case.
func SameProcessImage(image, expect string) bool {
	if expect == "" || image == "" {
		return false
	}
	if strings.ContainsAny(expect, `\/`) {
		return strings.EqualFold(image, expect)
	}
	base := image
	if k := strings.LastIndexAny(base, `\/`); k >= 0 {
		base = base[k+1:]
	}
	return strings.EqualFold(base, expect)
}

// ProcessInfo is a snapshot of one process.
type ProcessInfo struct {
	PID        uint32 `json:"pid"`
	PPID       uint32 `json:"ppid"`
	Name       string `json:"name"`
	Path       string `json:"path,omitempty"`
	WorkingSet uint64 `json:"workingSet"`
	PrivateMem uint64 `json:"privateMem"`
	CPUTime    int64  `json:"cpuTime"` // 100-ns units (kernel+user)
	Priority   uint32 `json:"priority"`
	Threads    uint32 `json:"threads"`
	Session    uint32 `json:"session"`
}

// PingReply is one ICMP echo result.
type PingReply struct {
	OK      bool          `json:"ok"`
	From    string        `json:"from"`
	RTT     time.Duration `json:"rtt"`
	Status  uint32        `json:"status"` // IP_STATUS (0 = success)
	TTLHop  bool          `json:"ttlHop"` // reply was "TTL expired in transit"
	TooBig  bool          `json:"tooBig"` // fragmentation needed (DF set)
	Timeout bool          `json:"timeout"`
}

// Native groups OS calls that have no stable command-line equivalent.
type Native interface {
	IsElevated() bool
	WindowsBuild() (major, minor, build, ubr int)
	DisplayModes() ([]DisplayMode, error)
	MemoryLists() (MemoryLists, error)
	PurgeStandbyList(lowPriorityOnly bool) error
	TimerResolution() (TimerResolution, error)
	// SetTimerResolution requests (set=true) or releases a resolution in 100-ns units.
	SetTimerResolution(res uint32, set bool) (uint32, error)
	Processes() ([]ProcessInfo, error)
	// KillProcess ends a process. When expect is not empty the process must
	// still be that program (a full path, or an executable name), checked on
	// the handle used to end it, so a PID reused meanwhile is never touched.
	KillProcess(pid uint32, expect string) error
	SetPriorityClass(pid uint32, class uint32) error
	SuspendProcess(pid uint32) error
	ResumeProcess(pid uint32) error
	// Ping sends one ICMP echo (IPv4) with the given TTL, payload size and DF
	// flag. RTT is measured with the high-resolution counter (sub-millisecond).
	Ping(ip string, ttl int, size int, dontFragment bool, timeout time.Duration) (PingReply, error)
	// Monotonic is a high-resolution monotonic clock (QueryPerformanceCounter
	// on Windows). Go's own monotonic clock on Windows advances only at the
	// system timer tick (up to 15.6 ms), too coarse to time network round trips.
	Monotonic() time.Duration
	// SetMouseParams applies [threshold1, threshold2, speed] live (SPI_SETMOUSE) and persists it.
	SetMouseParams(threshold1, threshold2, speed int) error
	// BroadcastSettingChange notifies Explorer/apps that environment/policies changed.
	BroadcastSettingChange()
	OpenURL(url string) error
	// FileVersion returns the version resource of an executable ("" if none).
	FileVersion(path string) string
	CPUTimes() (idle, kernel, user uint64, err error)

	// SetDisplayMode changes a display's refresh rate at its current resolution
	// (ChangeDisplaySettingsEx with CDS_UPDATEREGISTRY).
	SetDisplayMode(device string, width, height, hz int) error
	// CreateRestorePoint creates a System Restore point (SRSetRestorePointW).
	CreateRestorePoint(description string) error
	// RecycleBin returns size and item count of all recycle bins.
	RecycleBin() (bytes uint64, items uint64, err error)
	EmptyRecycleBin() error
	// Adapters lists network adapters (GetAdaptersAddresses).
	Adapters() ([]NetAdapter, error)
	// DNSConfig reads the static DNS servers of an interface (by GUID,
	// "{...}"); ErrNotFound when the adapter does not exist.
	DNSConfig(interfaceGUID string) (DNSConfig, error)
	// SetDNSConfig sets the static DNS servers of both IP stacks; an empty
	// list puts that stack back to automatic.
	SetDNSConfig(interfaceGUID string, v4, v6 []string) error
	// PowerStatus reports battery presence and AC state.
	PowerStatus() (hasBattery, onAC bool, percent int, err error)
	// DiskSpace returns total and free bytes of the volume holding path.
	DiskSpace(path string) (total, free uint64, err error)
	// Resolve looks up IPv4 addresses for a host name.
	Resolve(host string) ([]string, error)
	// DNSQuery sends one A query for host directly to server (UDP 53) and
	// returns the round-trip time (used by the DNS benchmark).
	DNSQuery(server, host string, timeout time.Duration) (time.Duration, error)
	// GameRunning returns the PIDs of running processes with the given exe name.
	PIDsByName(name string) []uint32
	// MoveToQuarantine moves a file or folder (for the uninstaller leftovers).
	MovePath(src, dst string) error
	// FirmwareType returns "UEFI", "BIOS" or "" (GetFirmwareType).
	FirmwareType() string
	// PickFile shows the native Open dialog; returns "" when cancelled.
	PickFile(title string, filterName string, pattern string) (string, error)

	// Paths returns the well-known folders of this PC and user.
	Paths() Paths

	// VerifySignature checks the Authenticode signature of a file
	// (WinVerifyTrust, whole certificate chain and revocation of the leaf).
	VerifySignature(path string) (Signature, error)
	// StartProgram launches an executable without waiting for it (used for a
	// vendor installer the user asked to run after it was verified).
	StartProgram(path string, args string) error
	// RunVisible starts a program with its normal window (an uninstaller) and
	// waits for it to exit or for ctx to end (the program keeps running then).
	RunVisible(ctx context.Context, path string, args string) (exitCode int, err error)
	// ProtectDir restricts a folder to SYSTEM and administrators (quarantine).
	ProtectDir(path string) error
	// Reinherit makes a folder or file that was moved back to its place take
	// the inheritable permissions of its current parent again (a move keeps
	// the permissions inherited where it was), keeping its explicit entries
	// and, if inheritance was disabled on it, leaving it that way.
	Reinherit(path string) error

	// NVMeHealth reads the SMART / Health Information log of an NVMe disk
	// (\\.\PhysicalDriveN, IOCTL_STORAGE_QUERY_PROPERTY protocol query).
	NVMeHealth(diskNumber int) (NVMeHealth, error)

	// GPUScheduling reports hardware-accelerated GPU scheduling (HAGS) as
	// the graphics kernel sees it (D3DKMTQueryAdapterInfo, WDDM 2.7 caps):
	// the registry value alone does not say whether it is on, because
	// drivers may turn it on by default.
	GPUScheduling() (GPUScheduling, error)

	// InputDevices lists the present keyboards and mice with the USB port
	// details reported by the hub driver (cfgmgr32 + USB hub IOCTLs).
	InputDevices() ([]InputDevice, error)
	// CaptureInput opens a focused test window and records when each raw
	// input report arrives and, when requested, when each USB transfer
	// completes in the host controller driver (ETW, administrator only).
	CaptureInput(ctx context.Context, opts CaptureOpts) (InputCapture, error)
	// CaptureDPC traces the kernel's deferred procedure calls and interrupt
	// service routines for the given time (the NT Kernel Logger, administrator
	// only) and returns what each kernel module spent in them. It only reads.
	CaptureDPC(ctx context.Context, d time.Duration) (DPCCapture, error)
	// CPUFreqPct is the average frequency of the logical processors as a
	// percentage of their maximum (the base frequency, so turbo goes above 100
	// and a processor held back by heat or a power limit falls below it),
	// since the previous call. The first call only starts the measurement and
	// returns an error. It reads a Windows performance counter by its English
	// name, so it does not depend on the language of Windows.
	CPUFreqPct() (float64, error)
	// ProcessorPower reads CallNtPowerInformation(ProcessorInformation): for
	// each logical processor, the maximum specified frequency, the current one
	// and the limit Windows puts on it for heat ("the maximum specified
	// processor clock frequency multiplied by the current processor thermal
	// throttle limit", Microsoft). It only reads and needs no rights.
	ProcessorPower() ([]ProcPower, error)
	// KernelModules lists the file names of the drivers loaded in the kernel
	// now (NtQuerySystemInformation, read only). Without administrator rights
	// Windows hides the list; that comes back as an error, not as an empty
	// list.
	KernelModules() ([]string, error)
	// NVDRSProfiles reads NVIDIA driver profiles (NVDRSBase, NVDRSGlobal or
	// NVDRSAppRef(exe)) in ONE DRS session: the given settings
	// (NvAPI_DRS_GetSetting) and, when withUser is set, every setting stored
	// in the profile with a user value (NvAPI_DRS_EnumSettings). The result
	// has one profile per ref, in order; a profile that does not exist is
	// Found=false with no error. No NVIDIA driver (or no DRS in it) is
	// ErrNoNVAPI. It only reads.
	NVDRSProfiles(refs []string, ids []uint32, withUser bool) ([]NVDRSProfile, error)
	// NVDRSSet writes a DWORD user value into a profile and saves the database
	// (NvAPI_DRS_SetSetting + NvAPI_DRS_SaveSettings; needs administrator).
	NVDRSSet(ref string, id, value uint32) error
	// NVDRSRestoreDefault puts a setting back to the driver's value for that
	// profile (NvAPI_DRS_RestoreProfileDefaultSetting) and saves.
	NVDRSRestoreDefault(ref string, id uint32) error
	// NVDRSDelete removes the user value of a setting from a profile
	// (NvAPI_DRS_DeleteProfileSetting: "deletes a setting or sets it back to
	// predefined value") and saves; a setting that is not there is not an error.
	NVDRSDelete(ref string, id uint32) error
	// NVVRR is what the NVIDIA driver says about variable refresh on a display
	// (\\.\DISPLAY1): NvAPI_DISP_GetDisplayIdByDisplayName + NvAPI_Disp_GetVRRInfo
	// (driver R525 or newer).
	NVVRR(device string) (NVVRRInfo, error)
}

// DPCHistBoundsUS are the upper edges, in microseconds, of the duration
// buckets of a DPC/ISR capture. A routine longer than the last edge falls in
// the final, open bucket, so a histogram has len(DPCHistBoundsUS)+1 counts.
var DPCHistBoundsUS = [...]float64{1, 2, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

// DPC kinds.
const (
	KindDPC = "dpc"
	KindISR = "isr"
)

// DPCStat is what the routines of one kind ("dpc" or "isr") of one kernel
// module cost during a capture.
type DPCStat struct {
	Module  string   `json:"module"` // ndis.sys, nvlddmkm.sys...; "" when the address matches no loaded module
	Kind    string   `json:"kind"`
	Count   uint64   `json:"count"`
	TotalUS float64  `json:"totalUs"`
	MaxUS   float64  `json:"maxUs"`
	Hist    []uint64 `json:"hist"` // len(DPCHistBoundsUS)+1
}

// DPCCapture is the raw outcome of CaptureDPC.
type DPCCapture struct {
	Duration time.Duration `json:"duration"`
	CPUs     int           `json:"cpus"` // logical processors, to turn time into a share of the machine
	Stats    []DPCStat     `json:"stats"`
	// Events is how many DPC and ISR entries were read; Discarded the ones that
	// could not be used (an impossible duration or address), a sign that the
	// events were not decoded as expected.
	Events      uint64 `json:"events"`
	Discarded   uint64 `json:"discarded"`
	LostEvents  uint32 `json:"lostEvents"`  // EVENT_TRACE_PROPERTIES.EventsLost
	LostBuffers uint32 `json:"lostBuffers"` // real-time buffers dropped
	// NoModules: the list of loaded drivers was not available, so the routines
	// could not be assigned to a driver (every Module is empty).
	NoModules bool `json:"noModules,omitempty"`
	Cancelled bool `json:"cancelled"`
}

// GPUScheduling is the HAGS state over all adapters.
type GPUScheduling struct {
	Supported        bool `json:"supported"`        // some adapter's driver supports it
	Enabled          bool `json:"enabled"`          // it is running on a supporting adapter
	EnabledByDefault bool `json:"enabledByDefault"` // the driver turns it on when the setting is absent
}

// ---------------------------------------------------------------------------
// Input devices (USB/HID inventory and polling capture)
// ---------------------------------------------------------------------------

// USB bus speeds as reported by the hub driver (USB_DEVICE_SPEED).
const (
	USBSpeedUnknown = -1
	USBSpeedLow     = 0 // 1.5 Mbit/s
	USBSpeedFull    = 1 // 12 Mbit/s
	USBSpeedHigh    = 2 // 480 Mbit/s
	USBSpeedSuper   = 3 // 5 Gbit/s or more
)

// USBEndpoint is one endpoint of the device's active configuration.
type USBEndpoint struct {
	Interface  int    `json:"interface"` // bInterfaceNumber (-1 when unknown)
	Class      int    `json:"class"`     // bInterfaceClass (3 = HID, -1 unknown)
	Protocol   int    `json:"protocol"`  // bInterfaceProtocol (boot: 1 keyboard, 2 mouse)
	Address    uint8  `json:"address"`   // bEndpointAddress (bit 7 = IN)
	Attributes uint8  `json:"attributes"`
	MaxPacket  uint16 `json:"maxPacket"`
	Interval   uint8  `json:"interval"` // bInterval exactly as the device reports it
}

// IsInterruptIn reports whether the endpoint is an interrupt IN endpoint.
func (e USBEndpoint) IsInterruptIn() bool { return e.Address&0x80 != 0 && e.Attributes&0x03 == 0x03 }

// InputDevice is one keyboard or mouse collection as seen by Plug and Play,
// with the details of the USB port it hangs from when it is a USB device.
type InputDevice struct {
	InstanceID   string        `json:"instanceId"`             // HID\VID_046D&PID_C539&MI_01&COL01\...
	Kind         string        `json:"kind"`                   // mouse | keyboard
	Name         string        `json:"name"`                   // "Mouse compatível com HID"
	Product      string        `json:"product,omitempty"`      // product string reported by the USB device
	Manufacturer string        `json:"manufacturer,omitempty"` // driver provider / vendor
	Chain        []string      `json:"chain"`                  // parent instance IDs, nearest first
	USBNode      string        `json:"usbNode,omitempty"`      // USB device plugged into the hub port
	USBInterface string        `json:"usbInterface,omitempty"` // interface node of a composite device (…&MI_01\…)
	Hub          string        `json:"hub,omitempty"`
	HubIsRoot    bool          `json:"hubIsRoot"`
	HubDepth     int           `json:"hubDepth"` // external hubs between the device and the root hub
	Port         int           `json:"port"`
	Speed        int           `json:"speed"` // USBSpeed*
	BcdUSB       uint16        `json:"bcdUsb"`
	Endpoints    []USBEndpoint `json:"endpoints,omitempty"`
	Controller   string        `json:"controller,omitempty"`   // USB host controller name
	ControllerID string        `json:"controllerId,omitempty"` // PCI\VEN_1022&DEV_15B6...
	Err          string        `json:"err,omitempty"`          // why USB details are missing
}

// CaptureOpts configures an input capture.
type CaptureOpts struct {
	Duration time.Duration
	Title    string // text drawn in the test window
	Message  string
	USBTrace bool // also trace USB transfer completions (ETW)
}

// RawInputSeries holds the arrival time of every motion report of one device.
type RawInputSeries struct {
	Device string  `json:"device"` // raw input device name (\\?\HID#VID_...)
	Kind   string  `json:"kind"`   // mouse | keyboard
	Stamps []int64 `json:"stamps"` // QueryPerformanceCounter ticks
	Events int     `json:"events"` // every report, including ones without motion
}

// USBPipeSeries holds the completion time of every transfer of one pipe.
type USBPipeSeries struct {
	Device uint64  `json:"device"` // UCX USB device handle
	Pipe   uint64  `json:"pipe"`
	Stamps []int64 `json:"stamps"` // QueryPerformanceCounter ticks
}

// USBTraceDevice maps a UCX device handle to the device named by the hub
// driver rundown.
type USBTraceDevice struct {
	Handle uint64 `json:"handle"`
	Port   int    `json:"port"`
	Desc   string `json:"desc"`
	Path   string `json:"path"` // device interface path (contains VID/PID)
}

// InputCapture is the raw outcome of CaptureInput.
type InputCapture struct {
	Freq        int64            `json:"freq"` // QueryPerformanceFrequency
	Duration    time.Duration    `json:"duration"`
	RawInput    []RawInputSeries `json:"rawInput"`
	USB         []USBPipeSeries  `json:"usb"`
	USBDevices  []USBTraceDevice `json:"usbDevices"`
	USBTraced   bool             `json:"usbTraced"`
	TraceError  string           `json:"traceError,omitempty"`
	LostEvents  uint32           `json:"lostEvents"`  // EVENT_TRACE_PROPERTIES.EventsLost
	LostBuffers uint32           `json:"lostBuffers"` // real-time buffers dropped (whole chunks of events)
	FocusLost   bool             `json:"focusLost"`
	Cancelled   bool             `json:"cancelled"`
}

// NetAdapter is one network interface.
type NetAdapter struct {
	Name        string   `json:"name"`        // friendly name
	Description string   `json:"description"` // driver description
	GUID        string   `json:"guid"`        // {....}
	IfIndex     int      `json:"ifIndex"`
	IfType      int      `json:"ifType"` // 6 ethernet, 71 wifi, 24 loopback, 131 tunnel
	Up          bool     `json:"up"`
	SpeedBps    uint64   `json:"speedBps"`
	MTU         int      `json:"mtu"`
	MAC         string   `json:"mac"`
	Gateways    []string `json:"gateways"`
	DNS         []string `json:"dns"`
	IPv4        []string `json:"ipv4"`
	DHCP        bool     `json:"dhcp"`
	Physical    bool     `json:"physical"`
}

// DNSConfig is the DNS server configuration of one interface as set by the
// user (or Axion): empty lists mean automatic (DHCP for IPv4, DHCPv6 or the
// router advertisement for IPv6).
type DNSConfig struct {
	V4  []string `json:"v4,omitempty"`  // static IPv4 servers, in order
	V6  []string `json:"v6,omitempty"`  // static IPv6 servers, in order
	DoH bool     `json:"doh,omitempty"` // DNS over HTTPS set on this adapter (read only)
}

// ---------------------------------------------------------------------------
// Platform bundle
// ---------------------------------------------------------------------------

// Platform aggregates every OS surface. Construct with winsys.New() or mocksys.New().
type Platform struct {
	Name    string // "windows" or "mock"
	Reg     Registry
	Svc     Services
	Power   Power
	Run     Runner
	Native  Native
	WMI     WMI
	Tasks   Tasks
	Events  EventLog
	Updates Updates
	Net     NetProbe
	DataDir string // %ProgramData%\Axion (or a temp dir in mock)
	IsMock  bool
	// Key authenticates Axion's own records (history journal, quarantine
	// manifests) with HMAC-SHA256. On Windows it lives in a file only
	// administrators can read, so a record forged by a program without
	// administrator rights is never trusted.
	Key []byte
}

// ---------------------------------------------------------------------------
// Network probe (internet side of the Rede page)
// ---------------------------------------------------------------------------

// LoadDir is the direction of a link saturation.
type LoadDir int

// Load directions.
const (
	LoadDownload LoadDir = iota
	LoadUpload
)

// LoadStats is the outcome of one saturation phase.
type LoadStats struct {
	Bytes   int64         `json:"bytes"`
	Elapsed time.Duration `json:"elapsed"`
	Streams int           `json:"streams"`
	Server  string        `json:"server"`
}

// NetProbe groups the measurements that talk to services on the internet.
// Each call happens only when the user starts a network test.
type NetProbe interface {
	// SDRConfig returns Valve's relay list (Steam Datagram Relay) for an app
	// (Counter-Strike 2 = 730), as published by the Steam Web API.
	SDRConfig(ctx context.Context, appID int) ([]byte, error)
	// Load saturates the link in one direction for about d with parallel
	// connections to a public speed-test service. onBytes is called as data
	// moves (from several goroutines). An error means the phase did not keep
	// the link full for the whole time (nothing moved, or the server refused
	// part way); the stats still count what moved.
	Load(ctx context.Context, dir LoadDir, streams int, d time.Duration, onBytes func(n int64)) (LoadStats, error)
	// DNSQuery sends one query for the A record of name to a DNS server
	// (IPv4 or IPv6, UDP port 53) and returns the round-trip time and the
	// response code (0 = NOERROR).
	DNSQuery(ctx context.Context, server, name string, timeout time.Duration) (time.Duration, int, error)
}

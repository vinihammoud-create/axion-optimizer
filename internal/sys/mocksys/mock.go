// Package mocksys is an in-memory implementation of the sys interfaces.
//
// It is used by unit tests (to exercise apply → verify → revert without a real
// Windows machine) and by the preview server on non-Windows hosts. It seeds a
// realistic Windows 11 default state so the diagnostics and tweak states look
// like a fresh install.
package mocksys

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"axion/internal/sys"
)

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

type regKey struct {
	name   string                  // original-case name of this key segment
	values map[string]sys.RegValue // lower(name) -> value (value keeps nothing about name case)
	vnames map[string]string       // lower(name) -> original name
	subs   map[string]*regKey      // lower(name) -> key
}

func newKey(name string) *regKey {
	return &regKey{name: name, values: map[string]sys.RegValue{}, vnames: map[string]string{}, subs: map[string]*regKey{}}
}

// Registry is a case-insensitive in-memory registry.
type Registry struct {
	mu    sync.Mutex
	roots map[sys.RegRoot]*regKey
	// FailWrites makes every write return an error for the given normalized path prefix.
	FailWrites map[string]bool
	// Writes counts successful SetValue/DeleteValue calls.
	Writes int
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{roots: map[sys.RegRoot]*regKey{
		sys.RootHKLM: newKey("HKLM"), sys.RootHKCU: newKey("HKCU"),
		sys.RootHKU: newKey("HKU"), sys.RootHKCR: newKey("HKCR"),
	}, FailWrites: map[string]bool{}}
}

func splitSegs(rest string) []string {
	if rest == "" {
		return nil
	}
	parts := strings.Split(rest, `\`)
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (r *Registry) find(path string, create bool) (*regKey, bool, error) {
	root, rest, err := sys.SplitRegPath(path)
	if err != nil {
		return nil, false, err
	}
	k := r.roots[root]
	created := false
	for _, seg := range splitSegs(rest) {
		low := strings.ToLower(seg)
		next, ok := k.subs[low]
		if !ok {
			if !create {
				return nil, false, nil
			}
			next = newKey(seg)
			k.subs[low] = next
			created = true
		}
		k = next
	}
	return k, created, nil
}

func (r *Registry) failing(path string) bool {
	n := strings.ToLower(sys.NormalizeRegPath(path))
	for p := range r.FailWrites {
		if strings.HasPrefix(n, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// GetValue implements sys.Registry.
func (r *Registry) GetValue(path, name string) (sys.RegValue, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k, _, err := r.find(path, false)
	if err != nil {
		return sys.RegValue{}, err
	}
	if k == nil {
		return sys.Absent(), nil
	}
	v, ok := k.values[strings.ToLower(name)]
	if !ok {
		return sys.Absent(), nil
	}
	return clone(v), nil
}

func clone(v sys.RegValue) sys.RegValue {
	c := v
	if v.Strs != nil {
		c.Strs = append([]string(nil), v.Strs...)
	}
	if v.Bin != nil {
		c.Bin = append([]byte(nil), v.Bin...)
	}
	return c
}

// SetValue implements sys.Registry.
func (r *Registry) SetValue(path, name string, v sys.RegValue) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failing(path) {
		return fmt.Errorf("acesso negado (simulado): %s", path)
	}
	if !v.Exists {
		return errors.New("SetValue com valor ausente")
	}
	k, _, err := r.find(path, true)
	if err != nil {
		return err
	}
	low := strings.ToLower(name)
	k.values[low] = clone(v)
	k.vnames[low] = name
	r.Writes++
	return nil
}

// DeleteValue implements sys.Registry.
func (r *Registry) DeleteValue(path, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failing(path) {
		return fmt.Errorf("acesso negado (simulado): %s", path)
	}
	k, _, err := r.find(path, false)
	if err != nil || k == nil {
		return err
	}
	low := strings.ToLower(name)
	delete(k.values, low)
	delete(k.vnames, low)
	r.Writes++
	return nil
}

// KeyExists implements sys.Registry.
func (r *Registry) KeyExists(path string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k, _, err := r.find(path, false)
	return k != nil, err
}

// CreateKey implements sys.Registry.
func (r *Registry) CreateKey(path string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failing(path) {
		return false, fmt.Errorf("acesso negado (simulado): %s", path)
	}
	_, created, err := r.find(path, true)
	return created, err
}

// DeleteKeyIfEmpty implements sys.Registry.
func (r *Registry) DeleteKeyIfEmpty(path string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	root, rest, err := sys.SplitRegPath(path)
	if err != nil {
		return false, err
	}
	segs := splitSegs(rest)
	if len(segs) == 0 {
		return false, nil
	}
	parent := r.roots[root]
	for _, seg := range segs[:len(segs)-1] {
		next, ok := parent.subs[strings.ToLower(seg)]
		if !ok {
			return false, nil
		}
		parent = next
	}
	last := strings.ToLower(segs[len(segs)-1])
	k, ok := parent.subs[last]
	if !ok {
		return false, nil
	}
	if len(k.values) > 0 || len(k.subs) > 0 {
		return false, nil
	}
	delete(parent.subs, last)
	return true, nil
}

// SubKeys implements sys.Registry.
func (r *Registry) SubKeys(path string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k, _, err := r.find(path, false)
	if err != nil || k == nil {
		return nil, err
	}
	var out []string
	for _, s := range k.subs {
		out = append(out, s.name)
	}
	sort.Strings(out)
	return out, nil
}

// ValueNames implements sys.Registry.
func (r *Registry) ValueNames(path string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k, _, err := r.find(path, false)
	if err != nil || k == nil {
		return nil, err
	}
	var out []string
	for low := range k.values {
		out = append(out, k.vnames[low])
	}
	sort.Strings(out)
	return out, nil
}

// ---------------------------------------------------------------------------
// Services
// ---------------------------------------------------------------------------

// Services is an in-memory service manager.
type Services struct {
	mu  sync.Mutex
	svc map[string]*sys.ServiceInfo
	reg *Registry // when set, services also get their registry key, as on Windows
}

// NewServices creates an empty service table.
func NewServices() *Services { return &Services{svc: map[string]*sys.ServiceInfo{}} }

// Add registers a service.
func (s *Services) Add(name, display string, start sys.ServiceStart, running bool) {
	s.mu.Lock()
	s.svc[strings.ToLower(name)] = &sys.ServiceInfo{Name: name, DisplayName: display, Exists: true, Start: start, Running: running}
	reg := s.reg
	s.mu.Unlock()
	if reg != nil {
		_, _ = reg.CreateKey(`HKLM\SYSTEM\CurrentControlSet\Services\` + name)
	}
}

// Query implements sys.Services.
func (s *Services) Query(name string) (sys.ServiceInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.svc[strings.ToLower(name)]; ok {
		return *v, nil
	}
	return sys.ServiceInfo{Name: name}, nil
}

// SetStart implements sys.Services.
func (s *Services) SetStart(name string, start sys.ServiceStart) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.svc[strings.ToLower(name)]
	if !ok {
		return sys.ErrNotFound
	}
	v.Start = start
	return nil
}

// Stop implements sys.Services.
func (s *Services) Stop(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.svc[strings.ToLower(name)]; ok {
		v.Running = false
		return nil
	}
	return sys.ErrNotFound
}

// Start implements sys.Services.
func (s *Services) Start(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.svc[strings.ToLower(name)]
	if !ok {
		return sys.ErrNotFound
	}
	if v.Start == sys.StartDisabled {
		return errors.New("serviço desativado não pode iniciar")
	}
	v.Running = true
	return nil
}

// ---------------------------------------------------------------------------
// Power
// ---------------------------------------------------------------------------

// Power is an in-memory power plan store.
type Power struct {
	mu      sync.Mutex
	active  string
	schemes map[string]string            // guid -> name
	values  map[string]map[string]uint32 // guid -> "sub/setting/ac|dc" -> value
	seq     int
	overlay string
}

// NewPower seeds the three standard Windows schemes.
func NewPower() *Power {
	p := &Power{schemes: map[string]string{
		sys.SchemeBalanced:        "Equilibrado",
		sys.SchemeHighPerformance: "Alto desempenho",
		sys.SchemePowerSaver:      "Economia de energia",
	}, values: map[string]map[string]uint32{}, active: sys.SchemeBalanced}
	for g := range p.schemes {
		p.values[g] = map[string]uint32{}
	}
	return p
}

func pkey(sub, setting string, ac bool) string {
	m := "dc"
	if ac {
		m = "ac"
	}
	return strings.ToLower(sub + "/" + setting + "/" + m)
}

// Seed sets a value for every scheme (used to model Windows defaults).
func (p *Power) Seed(sub, setting string, ac, dc uint32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for g := range p.schemes {
		p.values[g][pkey(sub, setting, true)] = ac
		p.values[g][pkey(sub, setting, false)] = dc
	}
}

// ActiveScheme implements sys.Power.
func (p *Power) ActiveScheme() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.active, nil
}

// SetActiveScheme implements sys.Power.
func (p *Power) SetActiveScheme(guid string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	guid = strings.ToLower(guid)
	if _, ok := p.schemes[guid]; !ok {
		return sys.ErrNotFound
	}
	p.active = guid
	return nil
}

// AddScheme registers a power plan with a fixed GUID (tests: plans that other
// programs install).
func (p *Power) AddScheme(guid, name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	guid = strings.ToLower(guid)
	p.schemes[guid] = name
	if _, ok := p.values[guid]; !ok {
		p.values[guid] = map[string]uint32{}
	}
}

// Schemes implements sys.Power.
func (p *Power) Schemes() ([]sys.PowerScheme, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []sys.PowerScheme
	for g, n := range p.schemes {
		out = append(out, sys.PowerScheme{GUID: g, Name: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GUID < out[j].GUID })
	return out, nil
}

// ReadValue implements sys.Power.
func (p *Power) ReadValue(scheme, sub, setting string, ac bool) (uint32, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	vals, ok := p.values[strings.ToLower(scheme)]
	if !ok {
		return 0, sys.ErrNotFound
	}
	v, ok := vals[pkey(sub, setting, ac)]
	if !ok {
		return 0, sys.ErrNotFound
	}
	return v, nil
}

// WriteValue implements sys.Power.
func (p *Power) WriteValue(scheme, sub, setting string, ac bool, value uint32) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	vals, ok := p.values[strings.ToLower(scheme)]
	if !ok {
		return sys.ErrNotFound
	}
	vals[pkey(sub, setting, ac)] = value
	return nil
}

// DuplicateScheme implements sys.Power.
func (p *Power) DuplicateScheme(source string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	source = strings.ToLower(source)
	name := "Cópia"
	if source == sys.SchemeUltimateTemplate {
		name = "Desempenho Máximo"
	} else if n, ok := p.schemes[source]; ok {
		name = n + " (cópia)"
	} else {
		return "", sys.ErrNotFound
	}
	p.seq++
	g := fmt.Sprintf("00000000-0000-4000-8000-%012d", p.seq)
	p.schemes[g] = name
	nv := map[string]uint32{}
	if src, ok := p.values[source]; ok {
		for k, v := range src {
			nv[k] = v
		}
	} else if src, ok := p.values[sys.SchemeHighPerformance]; ok {
		for k, v := range src {
			nv[k] = v
		}
	}
	p.values[g] = nv
	return g, nil
}

// ActiveOverlay implements sys.Power.
func (p *Power) ActiveOverlay() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.overlay, nil
}

// SetActiveOverlay implements sys.Power.
func (p *Power) SetActiveOverlay(g string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	g = strings.ToLower(strings.Trim(g, "{} "))
	if g == sys.OverlayBalanced {
		g = ""
	}
	p.overlay = g
	return nil
}

// DeleteScheme implements sys.Power.
func (p *Power) DeleteScheme(guid string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	guid = strings.ToLower(guid)
	if guid == p.active {
		return errors.New("não é possível excluir o plano ativo")
	}
	delete(p.schemes, guid)
	delete(p.values, guid)
	return nil
}

// ---------------------------------------------------------------------------
// Runner
// ---------------------------------------------------------------------------

// Call records one external command.
type Call struct {
	Exe  string
	Args []string
	PS   string
}

// Runner records calls and returns scripted responses.
type Runner struct {
	mu        sync.Mutex
	Calls     []Call
	Responses map[string]sys.Result // key: exe name lower, or "ps:<marker>"
	Handler   func(exe string, args []string, ps string) (sys.Result, bool)
	// Hold, when set, may return a channel that a call waits on before it
	// answers: the call ends early with the context error when the context
	// is cancelled first (like the real runner). Tests use it to keep a
	// long tool "running" so a job can be cancelled.
	Hold func(exe string, args []string) <-chan struct{}
}

// NewRunner creates a recording runner.
func NewRunner() *Runner { return &Runner{Responses: map[string]sys.Result{}} }

// Recorded returns a copy of the calls made so far.
func (r *Runner) Recorded() []Call {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Call(nil), r.Calls...)
}

// Run implements sys.Runner.
func (r *Runner) Run(ctx context.Context, exe string, args []string, opts sys.RunOpts) (sys.Result, error) {
	r.mu.Lock()
	r.Calls = append(r.Calls, Call{Exe: exe, Args: append([]string(nil), args...)})
	h, hold := r.Handler, r.Hold
	r.mu.Unlock()
	if hold != nil {
		if ch := hold(exe, args); ch != nil {
			select {
			case <-ch:
			case <-ctx.Done():
				return sys.Result{ExitCode: -1}, ctx.Err()
			}
		}
	}
	if h != nil {
		if res, ok := h(exe, args, ""); ok {
			emit(opts, res)
			return res, nil
		}
	}
	r.mu.Lock()
	res, ok := r.Responses[strings.ToLower(filepath.Base(exe))]
	r.mu.Unlock()
	if !ok {
		res = sys.Result{ExitCode: 0}
	}
	emit(opts, res)
	return res, nil
}

func emit(opts sys.RunOpts, res sys.Result) {
	if opts.OnLine == nil {
		return
	}
	for _, l := range strings.Split(strings.ReplaceAll(res.Stdout, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			opts.OnLine(l)
		}
	}
}

// ---------------------------------------------------------------------------
// Native
// ---------------------------------------------------------------------------

// Native is a fake native layer with a plausible gaming desktop.
type Native struct {
	mu        sync.Mutex
	Elevated  bool
	Build     [4]int
	Displays  []sys.DisplayMode
	Mem       sys.MemoryLists
	Timer     sys.TimerResolution
	Procs     []sys.ProcessInfo
	Purges    int
	Mouse     [3]int
	OpenedURL []string
	pingSeq   int

	RestorePoints []string
	FreeBytes     uint64 // extra free space on C: (component cleanup)
	// RevocationOffline makes signature checks fail as if the certificate
	// revocation servers were unreachable.
	RevocationOffline bool
	RecycleBytes      uint64
	RecycleItems      uint64
	NetAdapters       []sys.NetAdapter
	// DoH marks, by lower-case GUID, an adapter with DNS over HTTPS set by the
	// user (read only for Axion); DNSWrites counts the SetDNSConfig calls that
	// wrote something.
	DoH         map[string]bool
	DNSWrites   int
	BestIfIndex int // route lookup result (0 = first connected adapter)
	HasBattery  bool
	OnBattery   bool // running on the battery (needs HasBattery)
	// BatteryDesignMWh and BatteryFullMWh (and BatteryCycles) are what the
	// simulated power report says; zero = the report cannot be made.
	BatteryDesignMWh int
	BatteryFullMWh   int
	BatteryCycles    int

	// Started lists programs launched with StartProgram; Visible those run
	// with RunVisible (RunHook simulates what they do).
	Started []string
	Visible []string
	// Reinherited lists paths passed to Reinherit.
	Reinherited []string
	RunHook     func(path, args string) int
	start       time.Time
	// Net is the simulated network behind Ping (shared with the NetProbe).
	Net   *NetSim
	paths sys.Paths
	reg   *Registry // for state the real API derives from the registry

	// NVMe holds the health log per disk number (missing = not NVMe / error).
	NVMe map[int]sys.NVMeHealth

	// Inputs overrides the simulated keyboards/mice (nil = defaults).
	Inputs []sys.InputDevice
	// CaptureFn overrides CaptureInput (nil = synthetic 1000 Hz mouse).
	CaptureFn func(ctx context.Context, opts sys.CaptureOpts) (sys.InputCapture, error)
	Captures  int
	// DPCFn overrides CaptureDPC (nil = a plausible desktop with one slow
	// graphics DPC); DPCCaptures counts the calls.
	DPCFn       func(ctx context.Context, d time.Duration) (sys.DPCCapture, error)
	DPCCaptures int
	// CPUFreq is what CPUFreqPct reports (0 = 112, a turbo running a game);
	// CPUFreqErr makes it fail, like a PC without the counter.
	CPUFreq    float64
	CPUFreqErr error
	// ProcPowers is what ProcessorPower reports (nil = 16 processors at
	// 4200 MHz with no thermal limit); ProcPowersErr makes it fail.
	ProcPowers    []sys.ProcPower
	ProcPowersErr error
	// Modules is what KernelModules reports (nil = a few ordinary drivers);
	// ModulesErr makes it fail, like a run without administrator rights.
	Modules    []string
	ModulesErr error
	// DRS is the simulated NVIDIA profile database (NVAPI DRS); it has its
	// own lock.
	DRS *DRSSim
}

// NewNative creates the fake native layer.
func NewNative() *Native {
	return &Native{
		Elevated: true,
		DRS:      defaultDRS(),
		Build:    [4]int{10, 0, 26100, 4061},
		Displays: []sys.DisplayMode{{Device: `\\.\DISPLAY1`, Monitor: "Monitor Gamer 27\" (simulado)", Width: 1920, Height: 1080, CurrentHz: 60, MaxHzAtRes: 240, MaxHzAny: 240, BitsPerPel: 32, Primary: true}},
		Mem:      sys.MemoryLists{Total: 32 << 30, Avail: 21 << 30, Free: 3 << 30, Standby: 17 << 30, Modified: 300 << 20, InUse: 11 << 30, Source: "mock"},
		Timer:    sys.TimerResolution{Coarsest: 156250, Finest: 5000, Current: 10000},
		// A worn DRAM-less 480 GB drive, like the WD Green SN350 of the real report.
		NVMe: map[int]sys.NVMeHealth{0: {TempC: 46, SparePct: 100, SpareThreshold: 10, PercentUsed: 81,
			DataReadBytes: 61_000_000 * 512_000, DataWrittenBytes: 94_921_875 * 512_000, PowerCycles: 1_850, PowerOnHours: 9_120,
			UnsafeShutdowns: 412, ErrorLogEntries: 12}},
		Procs: []sys.ProcessInfo{
			{PID: 4, Name: "System", WorkingSet: 2 << 20},
			{PID: 880, Name: "svchost.exe", WorkingSet: 60 << 20, PrivateMem: 48 << 20, CPUTime: 30_000_000, Path: `C:\Windows\System32\svchost.exe`},
			{PID: 3990, Name: "MsMpEng.exe", WorkingSet: 240 << 20, PrivateMem: 210 << 20, CPUTime: 80_000_000},
			{PID: 1200, Name: "explorer.exe", WorkingSet: 180 << 20, PrivateMem: 120 << 20, CPUTime: 90_000_000, Session: 1, Path: `C:\Windows\explorer.exe`},
			{PID: 2211, Name: "Discord.exe", WorkingSet: 420 << 20, PrivateMem: 380 << 20, CPUTime: 120_000_000, Session: 1, Path: `C:\Users\Gamer\AppData\Local\Discord\app-1.0.9212\Discord.exe`},
			{PID: 2250, Name: "chrome.exe", WorkingSet: 950 << 20, PrivateMem: 610 << 20, CPUTime: 400_000_000, Session: 1, Path: `C:\Program Files\Google\Chrome\Application\chrome.exe`},
			{PID: 2264, Name: "chrome.exe", WorkingSet: 380 << 20, PrivateMem: 290 << 20, CPUTime: 90_000_000, Session: 1},
			{PID: 2290, Name: "chrome.exe", WorkingSet: 260 << 20, PrivateMem: 210 << 20, CPUTime: 40_000_000, Session: 1},
			{PID: 3001, Name: "steam.exe", WorkingSet: 250 << 20, PrivateMem: 190 << 20, CPUTime: 60_000_000, Session: 1, Path: `C:\Program Files (x86)\Steam\steam.exe`},
			{PID: 3100, Name: "iCUE.exe", WorkingSet: 310 << 20, PrivateMem: 260 << 20, CPUTime: 150_000_000, Session: 1, Path: `C:\Program Files\Corsair\CORSAIR iCUE 5 Software\iCUE.exe`},
			{PID: 3202, Name: "OneDrive.exe", WorkingSet: 110 << 20, PrivateMem: 80 << 20, CPUTime: 20_000_000, Session: 1},
		},
		start: time.Now(),
		Net:   NewNetSim(),
	}
}

// cpuWeight is the simulated CPU share (1.0 = one full core) per program.
var cpuWeight = map[string]float64{"chrome.exe": 0.9, "iCUE.exe": 0.35, "Discord.exe": 0.2, "MsMpEng.exe": 0.1, "explorer.exe": 0.03}

// AddProc puts a process in the list (with the lock: a goroutine that looks
// at the list may be running).
func (n *Native) AddProc(p sys.ProcessInfo) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Procs = append(n.Procs, p)
}

// RemoveProcs takes out every process with the name and says how many left.
func (n *Native) RemoveProcs(name string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	kept, gone := make([]sys.ProcessInfo, 0, len(n.Procs)), 0
	for _, p := range n.Procs {
		if strings.EqualFold(p.Name, name) {
			gone++
			continue
		}
		kept = append(kept, p)
	}
	n.Procs = kept
	return gone
}

func (n *Native) IsElevated() bool { return n.Elevated }
func (n *Native) WindowsBuild() (int, int, int, int) {
	return n.Build[0], n.Build[1], n.Build[2], n.Build[3]
}
func (n *Native) DisplayModes() ([]sys.DisplayMode, error) {
	return append([]sys.DisplayMode(nil), n.Displays...), nil
}
func (n *Native) MemoryLists() (sys.MemoryLists, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.Mem, nil
}
func (n *Native) PurgeStandbyList(lowOnly bool) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Purges++
	freed := n.Mem.Standby
	if lowOnly {
		freed /= 3
	}
	n.Mem.Standby -= freed
	n.Mem.Free += freed
	return nil
}
func (n *Native) TimerResolution() (sys.TimerResolution, error) { return n.Timer, nil }
func (n *Native) SetTimerResolution(res uint32, set bool) (uint32, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if set {
		n.Timer.Current = res
	} else {
		n.Timer.Current = 156250
	}
	return n.Timer.Current, nil
}
func (n *Native) Processes() ([]sys.ProcessInfo, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := append([]sys.ProcessInfo(nil), n.Procs...)
	elapsed := float64(time.Since(n.start) / 100) // 100-ns units
	for i := range out {
		out[i].CPUTime += int64(elapsed * cpuWeight[out[i].Name])
	}
	return out, nil
}
func (n *Native) KillProcess(pid uint32, expect string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	for i, p := range n.Procs {
		if p.PID == pid {
			img := p.Path
			if img == "" {
				img = p.Name
			}
			if expect != "" && !sys.SameProcessImage(img, expect) {
				return fmt.Errorf("o processo %d não é mais %s", pid, expect)
			}
			n.Procs = append(n.Procs[:i], n.Procs[i+1:]...)
			return nil
		}
	}
	return sys.ErrNotFound
}
func (n *Native) SetPriorityClass(pid uint32, class uint32) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	for i := range n.Procs {
		if n.Procs[i].PID == pid {
			n.Procs[i].Priority = class
			return nil
		}
	}
	return sys.ErrNotFound
}
func (n *Native) SuspendProcess(uint32) error { return nil }
func (n *Native) ResumeProcess(uint32) error  { return nil }

// Ping simulates a stable connection with a little jitter; TTL hops model a
// small route. Payloads above 1472 bytes with DF set report "too big".
func (n *Native) SetMouseParams(a, b, c int) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Mouse = [3]int{a, b, c}
	return nil
}
func (n *Native) BroadcastSettingChange() {}
func (n *Native) OpenURL(u string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.OpenedURL = append(n.OpenedURL, u)
	return nil
}
func (n *Native) FileVersion(string) string { return "" }
func (n *Native) CPUTimes() (uint64, uint64, uint64, error) {
	// 16 logical CPUs, ~12% busy (kernel time includes idle on Windows).
	t := uint64(time.Now().UnixNano()/100) * 16
	return t * 88 / 100, t * 60 / 100, t * 40 / 100, nil
}

// ---------------------------------------------------------------------------
// Bundle
// ---------------------------------------------------------------------------

// Mock bundles all fakes so tests can reach them directly.
type Mock struct {
	*sys.Platform
	R *Registry
	S *Services
	P *Power
	X *Runner
	N *Native
	W *WMI
	T *Tasks
	E *Events
	U *Updates
	// Simulated winget (0.8): installed packages (id -> version), the ones
	// with a newer version (id -> available) and ids whose install fails
	// with the given exit code.
	WingetInstalled map[string]string
	WingetUpgrades  map[string]string
	WingetFail      map[string]int
	// WingetDelay makes each install or upgrade take this long (to test
	// cancelling between items).
	WingetDelay time.Duration
	// BCD is the simulated {current} boot entry (element -> value).
	BCD map[string]string
	// PnpDeleted holds the driver packages removed with "pnputil
	// /delete-driver" (lower-case published name). Guarded by mu.
	PnpDeleted map[string]bool
	// PresentMon simulation: runs so far, an error to answer with, and
	// "the game was not running" (no CSV).
	PMRuns   int
	PMFail   string
	PMNoGame bool
	// PMHook, when set, is called at the start of every recording run (not
	// --terminate_existing_session) with its number; PMNeedsProc makes a run
	// with --terminate_on_proc_exit end at once, without a CSV, when the
	// process is not running (the game closed). Off by default: the manual
	// measurement tests run without the game in the process list. Read under mu.
	PMHook      func(run int, args []string)
	PMNeedsProc bool
	// PMBase, when set, gives the average frame time (ms) of the n-th
	// simulated capture (1 = first); nil keeps the slow improvement from one
	// capture to the next. PMSeed, when set, gives the random seed index of
	// the n-th capture (nil = n), so a test can make captures repeat the same
	// draws. Both are read under mu and are not machine state (not in World()).
	PMBase func(run int) float64
	PMSeed func(run int) int
	// Repair simulates DISM /RestoreHealth and sfc /scannow: RepairSFC is
	// "" (no damage), "repaired", "unrepaired", "reboot", "pending" or
	// "fail"; RepairDISM is the DISM exit code (0 by default); RepairPT
	// answers in Brazilian Portuguese, like a pt-BR Windows. Guarded by mu.
	RepairSFC  string
	RepairDISM int
	RepairPT   bool
	// watch: folders whose files enter World() (name -> SHA-256); empty keeps
	// the old behaviour. Guarded by mu.
	watch []string
	mu    sync.Mutex
}

// New creates a mock platform seeded with Windows 11 defaults. dataDir may be
// empty, in which case a temp dir is created.
func New(dataDir string) (*Mock, error) {
	if dataDir == "" {
		d, err := os.MkdirTemp("", "axion-mock-*")
		if err != nil {
			return nil, err
		}
		dataDir = d
	}
	m := &Mock{R: NewRegistry(), S: NewServices(), P: NewPower(), X: NewRunner(), N: NewNative(), W: NewWMI(), T: NewTasks(), E: &Events{}, U: NewUpdates(), BCD: map[string]string{}}
	key := sha256.Sum256([]byte("axion-mock-key:" + dataDir))
	m.Platform = &sys.Platform{Name: "mock", Reg: m.R, Svc: m.S, Power: m.P, Run: m.X, Native: m.N, WMI: m.W, Tasks: m.T, Events: m.E, Updates: m.U, Net: m.N.Net, DataDir: dataDir, IsMock: true, Key: key[:]}
	m.X.Handler = m.simulateTool
	m.N.reg = m.R
	m.S.reg = m.R
	m.N.paths = mockPaths(filepath.Join(dataDir, "mockfs"))
	seedFS(m.N.paths)
	SeedWindowsDefaults(m)
	seedPrograms(m)
	return m, nil
}

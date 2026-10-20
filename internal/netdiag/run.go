package netdiag

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Target is one destination measured at idle.
type Target struct {
	Name   string    `json:"name"`
	Role   string    `json:"role"` // lan, isp, internet, valve
	Addr   string    `json:"addr"`
	POP    string    `json:"pop,omitempty"`
	Idle   Stats     `json:"idle"`
	Series []float64 `json:"series,omitempty"` // downsampled RTTs for the chart (Lost = no reply)
}

// Target roles.
const (
	RoleLAN      = "lan"
	RoleISP      = "isp"
	RoleInternet = "internet"
	RoleValve    = "valve"
)

// LoadPhase is one direction of the bufferbloat test.
type LoadPhase struct {
	Dir      string  `json:"dir"` // download, upload
	Mbps     float64 `json:"mbps"`
	Bytes    int64   `json:"bytes"`
	Seconds  float64 `json:"seconds"`
	Streams  int     `json:"streams"`
	Internet Stats   `json:"internet"` // latency to the internet while loaded
	LAN      Stats   `json:"lan"`      // latency to the router while loaded
	AddedMs  float64 `json:"addedMs"`  // loaded median minus idle median (internet)
	// LoadedMs is the median under load with the pings that did not come
	// back in time counted as the timeout; NoReply: none came back.
	LoadedMs float64   `json:"loadedMs,omitempty"`
	NoReply  bool      `json:"noReply,omitempty"`
	Series   []float64 `json:"series,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// Bloat is the result of the bufferbloat test.
type Bloat struct {
	// Grade and AddedMs (the worst direction) exist only when every
	// direction was measured: the good direction alone says nothing about
	// the other one.
	Grade   string      `json:"grade"`
	AddedMs float64     `json:"addedMs"`
	Phases  []LoadPhase `json:"phases"`
	DataMB  float64     `json:"dataMB"`
	Server  string      `json:"server"`
	// Partial: some direction was measured and some failed.
	Partial bool `json:"partial,omitempty"`
}

// Measured reports whether the phase moved data and measured the latency
// under load (the added latency is then meaningful).
func (p *LoadPhase) Measured() bool {
	return p != nil && p.Error == "" && p.Internet.Sent > 0 && p.LoadedMs > 0
}

// Failure is the reason the load test is incomplete ("" when every
// direction was measured).
func (b *Bloat) Failure() string {
	if b == nil {
		return ""
	}
	var out []string
	for _, p := range b.Phases {
		if p.Error != "" {
			out = append(out, p.Dir+": "+p.Error)
		}
	}
	return strings.Join(out, "; ")
}

// Complete reports whether every direction was measured.
func (b *Bloat) Complete() bool {
	if b == nil || len(b.Phases) == 0 {
		return false
	}
	for i := range b.Phases {
		if !b.Phases[i].Measured() {
			return false
		}
	}
	return true
}

// Phase returns the phase of a direction, or nil.
func (b *Bloat) Phase(dir string) *LoadPhase {
	if b == nil {
		return nil
	}
	for i := range b.Phases {
		if b.Phases[i].Dir == dir {
			return &b.Phases[i]
		}
	}
	return nil
}

// DNSServer is the timing of one resolver.
type DNSServer struct {
	Addr     string  `json:"addr"`
	Name     string  `json:"name"`
	System   bool    `json:"system"` // configured in Windows
	Median   float64 `json:"median"`
	P90      float64 `json:"p90"`
	Failures int     `json:"failures"`
	Queries  int     `json:"queries"`
}

// AdapterInfo is the network adapter used by the test.
type AdapterInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Kind        string   `json:"kind"` // Ethernet, Wi-Fi
	LinkMbps    int64    `json:"linkMbps"`
	MTU         int      `json:"mtu"`
	Gateway     string   `json:"gateway"`
	DNS         []string `json:"dns"`
	EEE         string   `json:"eee,omitempty"`         // *EEE value when exposed
	IntMod      string   `json:"intMod,omitempty"`      // *InterruptModeration value when exposed
	SpeedDuplex string   `json:"speedDuplex,omitempty"` // *SpeedDuplex (0 = automatic)
	// Gigabit: the card can link at 1 Gb/s or more (a 10/100 card at
	// 100 Mb/s is at its maximum, not a cable problem).
	Gigabit bool `json:"gigabit,omitempty"`
	// Via names the adapter Windows routes the internet through when it is
	// not this one (a VPN or virtual adapter).
	Via string `json:"via,omitempty"`
	// WiFiSignal and WiFiBand are read once, at the start of the test, so
	// they describe that moment and not the present.
	WiFiSignal int    `json:"wifiSignal,omitempty"` // 0-100%
	WiFiBand   string `json:"wifiBand,omitempty"`   // "2.4 GHz" or "5 GHz"
	// GUID of the adapter (used to pick its block in the netsh output and in
	// the DNS item); it is not stored with the test.
	GUID string `json:"-"`
	// DNSRead: the DNS of the adapter could be read (Windows 10 2004 or
	// newer). Without it the DNS item is not offered, so no button leads to a
	// technical error. Not stored with the test.
	DNSRead bool `json:"-"`
	// DoH: DNS over HTTPS is set on this adapter (by the user); the DNS item
	// is not offered. DNSAuto: no static server in either protocol.
	DoH     bool `json:"doh,omitempty"`
	DNSAuto bool `json:"dnsAuto,omitempty"`
	// IPv6DNS: the adapter already has an IPv6 DNS server that is not
	// link-local, so IPv6 works for names there.
	IPv6DNS bool `json:"ipv6Dns,omitempty"`
}

// CS2Net is what the game's own settings say about the network.
type CS2Net struct {
	Found       bool   `json:"found"`
	BufferTicks int    `json:"bufferTicks"` // -1 = not set (default: none)
	Rate        int    `json:"rate"`        // -1 = not set (default: unrestricted)
	File        string `json:"file,omitempty"`
}

// Action links a finding to where it is fixed.
type Action struct {
	Kind   string   `json:"kind"` // page, url, tweaks, apply
	Label  string   `json:"label"`
	Page   string   `json:"page,omitempty"`
	URL    string   `json:"url,omitempty"`
	Tweaks []string `json:"tweaks,omitempty"`
	// Confirm is the text the user reads before an "apply" action (an
	// execution ID applied by /api/apply, not an item of the catalog).
	Confirm string `json:"confirm,omitempty"`
}

// Finding is a verdict of the network test.
type Finding struct {
	ID       string   `json:"id"`
	Area     string   `json:"area"`     // lan, isp, valve, bloat, dns, cs2, nic, path
	Severity string   `json:"severity"` // good, info, warning, critical
	Title    string   `json:"title"`
	Text     string   `json:"text"`
	Actions  []Action `json:"actions,omitempty"`
}

// Run is one complete network test.
type Run struct {
	ID       string       `json:"id"`
	At       time.Time    `json:"at"`
	Label    string       `json:"label"`
	Seconds  float64      `json:"seconds"`
	Adapter  AdapterInfo  `json:"adapter"`
	Path     Path         `json:"path"`
	Targets  []Target     `json:"targets"`
	Relays   []RelayProbe `json:"relays,omitempty"` // quick look at every POP
	Bloat    *Bloat       `json:"bloat,omitempty"`
	DNS      []DNSServer  `json:"dns,omitempty"`
	AIM      *AIM         `json:"aim,omitempty"`
	CS2      CS2Net       `json:"cs2"`
	Findings []Finding    `json:"findings"`
	Errors   []string     `json:"errors,omitempty"`
}

// Target returns the first target of a role, or nil.
func (r *Run) Target(role string) *Target {
	for i := range r.Targets {
		if r.Targets[i].Role == role {
			return &r.Targets[i]
		}
	}
	return nil
}

// BestValve returns the Valve relay target with the lowest median.
func (r *Run) BestValve() *Target {
	var best *Target
	for i := range r.Targets {
		t := &r.Targets[i]
		if t.Role != RoleValve || !t.Idle.OK() {
			continue
		}
		if best == nil || t.Idle.Median < best.Idle.Median {
			best = t
		}
	}
	return best
}

// ---------------------------------------------------------------------------
// History
// ---------------------------------------------------------------------------

// Summary is one line of the history.
type Summary struct {
	ID        string    `json:"id"`
	At        time.Time `json:"at"`
	Label     string    `json:"label"`
	Adapter   string    `json:"adapter"`
	LANMs     float64   `json:"lanMs"`
	LANP95    float64   `json:"lanP95"`
	NetMs     float64   `json:"netMs"`
	NetJitter float64   `json:"netJitter"`
	NetLoss   float64   `json:"netLoss"`
	ValvePOP  string    `json:"valvePop"`
	ValveMs   float64   `json:"valveMs"`
	ValveJit  float64   `json:"valveJitter"`
	ValveLoss float64   `json:"valveLoss"`
	Grade     string    `json:"grade"`
	AddedMs   float64   `json:"addedMs"`
	DownMbps  float64   `json:"downMbps"`
	UpMbps    float64   `json:"upMbps"`
	// Latency added under load in each direction (-1 = not measured).
	DownAddedMs float64 `json:"downAddedMs"`
	UpAddedMs   float64 `json:"upAddedMs"`
	// LoadError: why the load test is incomplete (then there is no grade).
	LoadError  string `json:"loadError,omitempty"`
	AIM        string `json:"aim"`
	Problems   int    `json:"problems"`
	HasLoadRun bool   `json:"hasLoad"`
}

// Summarize builds the history line of a run (-1 = not measured).
func Summarize(r *Run) Summary {
	s := Summary{ID: r.ID, At: r.At, Label: r.Label, Adapter: r.Adapter.Kind,
		LANMs: -1, LANP95: -1, NetMs: -1, NetJitter: -1, NetLoss: -1, ValveMs: -1, ValveJit: -1, ValveLoss: -1, AddedMs: -1, DownMbps: -1, UpMbps: -1,
		DownAddedMs: -1, UpAddedMs: -1}
	if t := r.Target(RoleLAN); t != nil && t.Idle.OK() {
		s.LANMs, s.LANP95 = t.Idle.Median, t.Idle.P95
	}
	if t := r.Target(RoleInternet); t != nil && t.Idle.OK() {
		s.NetMs, s.NetJitter, s.NetLoss = t.Idle.Median, t.Idle.Jitter, t.Idle.LossPct
	}
	if t := r.BestValve(); t != nil {
		s.ValvePOP, s.ValveMs, s.ValveJit, s.ValveLoss = t.Name, t.Idle.Median, t.Idle.Jitter, t.Idle.LossPct
	}
	if r.Bloat != nil {
		s.HasLoadRun = true
		// Without a grade no phase measured the latency under load.
		s.Grade = r.Bloat.Grade
		if s.Grade != "" {
			s.AddedMs = math.Max(r.Bloat.AddedMs, 0)
		}
		if p := r.Bloat.Phase("download"); p != nil && p.Error == "" {
			s.DownMbps = p.Mbps
		}
		if p := r.Bloat.Phase("upload"); p != nil && p.Error == "" {
			s.UpMbps = p.Mbps
		}
		if p := r.Bloat.Phase("download"); p.Measured() {
			s.DownAddedMs = math.Max(p.AddedMs, 0)
		}
		if p := r.Bloat.Phase("upload"); p.Measured() {
			s.UpAddedMs = math.Max(p.AddedMs, 0)
		}
		if s.Grade == "" {
			s.LoadError = r.Bloat.Failure()
		}
	}
	if r.AIM != nil {
		s.AIM = r.AIM.Class
	}
	for _, f := range r.Findings {
		if f.Severity == "warning" || f.Severity == "critical" {
			s.Problems++
		}
	}
	return s
}

var validID = regexp.MustCompile(`^[0-9a-f]{8,40}$`)

// MaxRuns is how many tests the history keeps.
const MaxRuns = 40

// Save writes a run and trims the history to MaxRuns.
func Save(dir string, r *Run) error {
	if !validID.MatchString(r.ID) {
		return errors.New("identificador inválido")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, r.ID+".json.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, r.ID+".json")); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	runs := List(dir)
	for i := MaxRuns; i < len(runs); i++ {
		_ = os.Remove(filepath.Join(dir, runs[i].ID+".json"))
	}
	return nil
}

// Load reads one run.
func Load(dir, id string) (*Run, error) {
	if !validID.MatchString(id) {
		return nil, errors.New("identificador inválido")
	}
	b, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return nil, err
	}
	var r Run
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("teste %s ilegível", id)
	}
	normalize(&r)
	return &r, nil
}

// normalize fixes tests saved by 0.6.0, which graded the load test from the
// direction that worked when the other one failed (an "A+" from the upload
// alone): the grade is dropped and the verdicts are made again.
func normalize(r *Run) {
	b := r.Bloat
	if b == nil || b.Grade == "" || b.Failure() == "" {
		return
	}
	b.Grade, b.AddedMs = "", 0
	for i := range b.Phases {
		if b.Phases[i].Measured() {
			b.Partial = true
		}
	}
	Analyze(r)
}

// List returns the history, newest first.
func List(dir string) []Summary {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Summary
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		r, err := Load(dir, strings.TrimSuffix(name, ".json"))
		if err != nil {
			continue
		}
		out = append(out, Summarize(r))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

// Delete removes a run.
func Delete(dir, id string) error {
	if !validID.MatchString(id) {
		return errors.New("identificador inválido")
	}
	return os.Remove(filepath.Join(dir, id+".json"))
}

// Relabel changes the label of a run.
func Relabel(dir, id, label string) (*Run, error) {
	r, err := Load(dir, id)
	if err != nil {
		return nil, err
	}
	r.Label = CleanLabel(label)
	return r, Save(dir, r)
}

// CleanLabel trims a label to something short and printable.
func CleanLabel(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if len([]rune(s)) > 40 {
		s = string([]rune(s)[:40])
	}
	return s
}

// ---------------------------------------------------------------------------
// Before x after
// ---------------------------------------------------------------------------

// CompareRow is one metric of a before x after comparison.
type CompareRow struct {
	Label   string  `json:"label"`
	Unit    string  `json:"unit"`
	A       float64 `json:"a"`
	B       float64 `json:"b"`
	Delta   float64 `json:"delta"`
	Verdict string  `json:"verdict"` // melhor, pior, igual, sem dados
}

// Compare lists the key metrics of two tests. A change counts only when it
// is larger than a practical minimum (network latency varies from one
// minute to the next).
func Compare(a, b Summary) []CompareRow {
	type m struct {
		label, unit   string
		a, b          float64
		lowerIsBetter bool
		abs, rel      float64 // minimum change: absolute and relative
	}
	ms := []m{
		{"Roteador (mediana)", "ms", a.LANMs, b.LANMs, true, 0.3, 0.2},
		{"Roteador (P95)", "ms", a.LANP95, b.LANP95, true, 0.5, 0.2},
		{"Internet (mediana)", "ms", a.NetMs, b.NetMs, true, 1, 0.1},
		{"Internet (jitter)", "ms", a.NetJitter, b.NetJitter, true, 0.5, 0.2},
		{"Internet (perda)", "%", a.NetLoss, b.NetLoss, true, 0.5, 0},
		{"Valve (mediana)", "ms", a.ValveMs, b.ValveMs, true, 1, 0.1},
		{"Valve (jitter)", "ms", a.ValveJit, b.ValveJit, true, 0.5, 0.2},
		{"Valve (perda)", "%", a.ValveLoss, b.ValveLoss, true, 0.5, 0},
		{"Atraso a mais sob carga", "ms", a.AddedMs, b.AddedMs, true, 3, 0.2},
		{"Atraso a mais no download", "ms", a.DownAddedMs, b.DownAddedMs, true, 3, 0.2},
		{"Atraso a mais no upload", "ms", a.UpAddedMs, b.UpAddedMs, true, 3, 0.2},
		{"Download", "Mb/s", a.DownMbps, b.DownMbps, false, 5, 0.05},
		{"Upload", "Mb/s", a.UpMbps, b.UpMbps, false, 5, 0.05},
	}
	var out []CompareRow
	for _, x := range ms {
		row := CompareRow{Label: x.label, Unit: x.unit, A: x.a, B: x.b}
		if x.a < 0 || x.b < 0 {
			row.Verdict = "sem dados"
			out = append(out, row)
			continue
		}
		row.Delta = math.Round((x.b-x.a)*100) / 100
		minChange := math.Max(x.abs, x.rel*math.Abs(x.a))
		switch {
		case math.Abs(row.Delta) < minChange:
			row.Verdict = "igual"
		case (row.Delta < 0) == x.lowerIsBetter:
			row.Verdict = "melhor"
		default:
			row.Verdict = "pior"
		}
		out = append(out, row)
	}
	return out
}

package mocksys

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"axion/internal/sys"
)

// ---------------------------------------------------------------------------
// WMI (table-driven)
// ---------------------------------------------------------------------------

// WMI answers queries from registered tables. A query "SELECT ... FROM Class
// WHERE Prop='v'" returns rows of Class filtered by simple equality clauses.
type WMI struct {
	mu     sync.Mutex
	Tables map[string][]sys.WMIRow // key: lower(namespace)|lower(class)
	Calls  []string
	// CallHandler handles method calls; nil returns 0.
	CallHandler func(namespace, path, method string, args []any) (any, error)
	// OutHandler answers CallOut for tests that need their own method; nil
	// uses the built-in simulation (PS_MMAgent, see mmagent.go).
	OutHandler func(namespace, path, method string, in map[string]any) (sys.WMIRow, error)
}

// NewWMI creates an empty WMI.
func NewWMI() *WMI { return &WMI{Tables: map[string][]sys.WMIRow{}} }

// Set registers rows for a class.
func (w *WMI) Set(namespace, class string, rows ...sys.WMIRow) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Tables[strings.ToLower(namespace)+"|"+strings.ToLower(class)] = rows
}

// SetAssoc registers the rows returned by
// "ASSOCIATORS OF {objectPath} WHERE ResultClass = resultClass". objectPath is
// unescaped: Class.Key=value.
func (w *WMI) SetAssoc(namespace, objectPath, resultClass string, rows ...sys.WMIRow) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Tables[assocKey(namespace, objectPath, resultClass)] = rows
}

func assocKey(namespace, objectPath, resultClass string) string {
	return "assoc|" + strings.ToLower(namespace) + "|" + objectPath + "|" + strings.ToLower(resultClass)
}

// associators answers ASSOCIATORS OF queries, validating the object path
// escaping the way WMI does (backslash before \ and " inside quoted keys).
func (w *WMI) associators(namespace, wql string) ([]sys.WMIRow, error) {
	start := strings.Index(wql, "{")
	end := strings.LastIndex(wql, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("WQL inválido: %s", wql)
	}
	path := wql[start+1 : end]
	eq := strings.Index(path, "=\"")
	if eq < 0 || !strings.HasSuffix(path, "\"") {
		return nil, fmt.Errorf("caminho de objeto inválido: %s", path)
	}
	quoted := path[eq+2 : len(path)-1]
	var raw strings.Builder
	for i := 0; i < len(quoted); i++ {
		c := quoted[i]
		if c == '\\' {
			if i+1 >= len(quoted) || (quoted[i+1] != '\\' && quoted[i+1] != '"') {
				return nil, fmt.Errorf("escape inválido no caminho: %s", path)
			}
			i++
			c = quoted[i]
		} else if c == '"' {
			return nil, fmt.Errorf("aspas sem escape no caminho: %s", path)
		}
		raw.WriteByte(c)
	}
	rest := wql[end+1:]
	up := strings.ToUpper(rest)
	k := strings.Index(up, "RESULTCLASS")
	if k < 0 {
		return nil, fmt.Errorf("ResultClass ausente: %s", wql)
	}
	cls := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest[k+len("RESULTCLASS"):]), "="))
	if f := strings.Fields(cls); len(f) > 0 {
		cls = f[0]
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Tables[assocKey(namespace, path[:eq+1]+raw.String(), cls)], nil
}

// Query implements sys.WMI.
func (w *WMI) Query(namespace, wql string) ([]sys.WMIRow, error) {
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(wql)), "ASSOCIATORS OF") {
		return w.associators(namespace, wql)
	}
	up := strings.ToUpper(wql)
	i := strings.Index(up, " FROM ")
	if i < 0 {
		return nil, fmt.Errorf("WQL inválido: %s", wql)
	}
	rest := strings.TrimSpace(wql[i+6:])
	class := rest
	where := ""
	if j := strings.Index(strings.ToUpper(rest), " WHERE "); j >= 0 {
		class = strings.TrimSpace(rest[:j])
		where = strings.TrimSpace(rest[j+7:])
	}
	w.mu.Lock()
	rows := w.Tables[strings.ToLower(namespace)+"|"+strings.ToLower(class)]
	w.mu.Unlock()
	if where == "" {
		return rows, nil
	}
	var out []sys.WMIRow
	for _, r := range rows {
		if matchWhere(r, where) {
			out = append(out, r)
		}
	}
	return out, nil
}

// matchWhere supports "A='x' AND B<>0" style conditions, and alternatives
// joined by OR (AND binds tighter, as in WQL).
func matchWhere(r sys.WMIRow, where string) bool {
	for _, alt := range splitWord(where, " OR ") {
		if matchAll(r, alt) {
			return true
		}
	}
	return false
}

func matchAll(r sys.WMIRow, where string) bool {
	for _, cond := range splitAnd(where) {
		cond = strings.TrimSpace(cond)
		op := "="
		k := strings.Index(cond, "<>")
		cut := 2
		if k >= 0 {
			op = "<>"
		} else if j := strings.Index(strings.ToUpper(cond), " LIKE "); j >= 0 {
			op, k, cut = "LIKE", j, len(" LIKE ")
		} else {
			k, cut = strings.Index(cond, "="), 1
		}
		if k < 0 {
			continue
		}
		name := strings.TrimSpace(cond[:k])
		val := strings.TrimSpace(cond[k+cut:])
		val = strings.Trim(val, "'\"")
		val = strings.ReplaceAll(val, `\\`, `\`)
		have := r.Str(name)
		if op == "LIKE" {
			// WQL LIKE with % wildcards at the ends (what Axion uses).
			if !likeMatch(have, val) {
				return false
			}
			continue
		}
		eq := strings.EqualFold(have, val)
		if op == "=" && !eq || op == "<>" && eq {
			return false
		}
	}
	return true
}

// hasEventID tells whether the XPath names this event ID whole ("EventID=41"
// does not match 4 or 410).
func hasEventID(xpath string, id int) bool {
	needle := fmt.Sprintf("EventID=%d", id)
	for i := 0; ; {
		j := strings.Index(xpath[i:], needle)
		if j < 0 {
			return false
		}
		k := i + j + len(needle)
		if k >= len(xpath) || xpath[k] < '0' || xpath[k] > '9' {
			return true
		}
		i = k
	}
}

// likeMatch implements the WQL LIKE patterns Axion uses: a literal with %
// as the wildcard, case-insensitive; without a wildcard it is an equality.
func likeMatch(have, pattern string) bool {
	have, pattern = strings.ToUpper(have), strings.ToUpper(pattern)
	if !strings.Contains(pattern, "%") {
		return have == pattern
	}
	parts := strings.Split(pattern, "%")
	if !strings.HasPrefix(have, parts[0]) {
		return false
	}
	have = have[len(parts[0]):]
	for i := 1; i < len(parts); i++ {
		p := parts[i]
		if p == "" {
			continue
		}
		j := strings.Index(have, p)
		if j < 0 || (i == len(parts)-1 && !strings.HasSuffix(have, p)) {
			return false
		}
		have = have[j+len(p):]
	}
	return true
}

func splitAnd(s string) []string { return splitWord(s, " AND ") }

// splitWord splits on a keyword, ignoring case.
func splitWord(s, word string) []string {
	var out []string
	up := strings.ToUpper(s)
	for {
		i := strings.Index(up, word)
		if i < 0 {
			return append(out, s)
		}
		out = append(out, s[:i])
		s, up = s[i+len(word):], up[i+len(word):]
	}
}

// Put implements sys.WMI: it updates the matching rows in place.
func (w *WMI) Put(namespace, wql, property string, value any) (int, error) {
	if !sys.IsSelectStar(wql) {
		// What Windows answers when a partial instance is written back.
		return 0, errors.New("Put_: Exceção. (Objeto inválido )")
	}
	rows, err := w.Query(namespace, wql)
	if err != nil {
		return 0, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, r := range rows {
		r[property] = value
	}
	w.Calls = append(w.Calls, "Put "+property)
	return len(rows), nil
}

// Call implements sys.WMI.
func (w *WMI) Call(namespace, path, method string, args ...any) (any, error) {
	w.mu.Lock()
	w.Calls = append(w.Calls, path+"."+method)
	h := w.CallHandler
	w.mu.Unlock()
	if h != nil {
		return h(namespace, path, method, args)
	}
	return int64(0), nil
}

// CallOut implements sys.WMI.
func (w *WMI) CallOut(namespace, path, method string, in map[string]any) (sys.WMIRow, error) {
	w.mu.Lock()
	w.Calls = append(w.Calls, path+"."+method)
	h := w.OutHandler
	w.mu.Unlock()
	if h != nil {
		return h(namespace, path, method, in)
	}
	if strings.EqualFold(namespace, MMAgentNamespace) && strings.EqualFold(path, sys.MMAgentClass) {
		return w.mmagentCall(method, in)
	}
	return nil, fmt.Errorf("método %s.%s não simulado", path, method)
}

// ---------------------------------------------------------------------------
// Task Scheduler
// ---------------------------------------------------------------------------

// Tasks is an in-memory task store.
type Tasks struct {
	mu    sync.Mutex
	tasks map[string]*sys.TaskInfo
	// OnRegister simulates what a registered task runs.
	OnRegister func(exe, args string, trigger int)
}

// NewTasks creates an empty store.
func NewTasks() *Tasks { return &Tasks{tasks: map[string]*sys.TaskInfo{}} }

// Add registers a task.
func (t *Tasks) Add(path string, enabled bool, triggers []int, actions ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	name := path[strings.LastIndex(path, `\`)+1:]
	st := sys.TaskReady
	if !enabled {
		st = sys.TaskDisabled
	}
	t.tasks[strings.ToLower(path)] = &sys.TaskInfo{Path: path, Name: name, Exists: true, Enabled: enabled, State: st, Triggers: triggers, Actions: actions}
}

// SetLastRun records when a task last ran and its result.
func (t *Tasks) SetLastRun(path string, at time.Time, result int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if v, ok := t.tasks[strings.ToLower(path)]; ok {
		v.LastRun, v.LastResult = at, result
	}
}

// Remove deletes a task (simulates a debloat script that removed it).
func (t *Tasks) Remove(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.tasks, strings.ToLower(path))
}

func (t *Tasks) Get(path string) (sys.TaskInfo, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if v, ok := t.tasks[strings.ToLower(path)]; ok {
		return *v, nil
	}
	return sys.TaskInfo{Path: path}, nil
}

func (t *Tasks) SetEnabled(path string, enabled bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	v, ok := t.tasks[strings.ToLower(path)]
	if !ok {
		return sys.ErrNotFound
	}
	v.Enabled = enabled
	if enabled {
		v.State = sys.TaskReady
	} else {
		v.State = sys.TaskDisabled
	}
	return nil
}

func (t *Tasks) List(folder string, recursive bool) ([]sys.TaskInfo, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	folder = strings.ToLower(strings.TrimSuffix(folder, `\`)) + `\`
	var out []sys.TaskInfo
	for k, v := range t.tasks {
		if !strings.HasPrefix(k, folder) {
			continue
		}
		if !recursive && strings.Contains(k[len(folder):], `\`) {
			continue
		}
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (t *Tasks) Register(path, desc, exe, args string, trigger int, at string, weekday int, highest bool) error {
	t.Add(path, true, []int{trigger}, strings.TrimSpace(exe+" "+args))
	t.mu.Lock()
	hook := t.OnRegister
	t.mu.Unlock()
	if hook != nil {
		hook(exe, args, trigger)
	}
	return nil
}

func (t *Tasks) Delete(path string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.tasks, strings.ToLower(path))
	return nil
}

// ---------------------------------------------------------------------------
// Event log
// ---------------------------------------------------------------------------

// Events is a static event store.
type Events struct {
	mu     sync.Mutex
	Events []sys.Event
}

// Query returns events of the channel whose provider or ID appear in the XPath.
func (e *Events) Query(channel, xpath string, max int) ([]sys.Event, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []sys.Event
	for _, ev := range e.Events {
		if !strings.EqualFold(ev.Channel, channel) {
			continue
		}
		if strings.Contains(xpath, "@Name=") && !strings.Contains(xpath, "'"+ev.Provider+"'") {
			continue
		}
		// A query by ID alone (no provider) binds every event of the channel.
		if !strings.Contains(xpath, "@Name=") && strings.Contains(xpath, "EventID=") && !hasEventID(xpath, ev.ID) {
			continue
		}
		// An EventID condition only binds the provider it is written after
		// ("Provider[@Name='X'] and EventID=41"); a provider listed alone
		// (WHEA-Logger) matches every ID.
		tail := ""
		if i := strings.Index(xpath, "'"+ev.Provider+"']"); i >= 0 {
			tail = xpath[i+len(ev.Provider)+3:]
		}
		bound := strings.HasPrefix(tail, " and EventID=") || strings.HasPrefix(tail, " and (EventID=")
		if bound && !hasEventID(xpath, ev.ID) {
			continue
		}
		out = append(out, ev)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Native extras
// ---------------------------------------------------------------------------

func (n *Native) SetDisplayMode(device string, w, h, hz int) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	for i := range n.Displays {
		d := &n.Displays[i]
		if strings.EqualFold(d.Device, device) {
			if hz > d.MaxHzAny {
				return errors.New("modo não suportado")
			}
			d.Width, d.Height, d.CurrentHz = w, h, hz
			return nil
		}
	}
	return sys.ErrNotFound
}

func (n *Native) CreateRestorePoint(desc string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.RestorePoints = append(n.RestorePoints, desc)
	return nil
}

func (n *Native) RecycleBin() (uint64, uint64, error) { return n.RecycleBytes, n.RecycleItems, nil }
func (n *Native) EmptyRecycleBin() error {
	n.RecycleBytes, n.RecycleItems = 0, 0
	return nil
}
func (n *Native) Adapters() ([]sys.NetAdapter, error) {
	return append([]sys.NetAdapter(nil), n.NetAdapters...), nil
}

// BestInterface mimics the Windows route lookup: the first connected
// adapter with a gateway (BestIfIndex overrides it in tests).
func (n *Native) BestInterface(ip string) (int, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.BestIfIndex != 0 {
		return n.BestIfIndex, nil
	}
	for _, a := range n.NetAdapters {
		if a.Up && len(a.Gateways) > 0 {
			return a.IfIndex, nil
		}
	}
	return 0, sys.ErrNotFound
}

const (
	mockTcpipIfaces  = `HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\`
	mockTcpip6Ifaces = `HKLM\SYSTEM\CurrentControlSet\Services\Tcpip6\Parameters\Interfaces\`
)

// adapterIndexLocked finds an adapter by GUID (n.mu held).
func (n *Native) adapterIndexLocked(guid string) int {
	for i := range n.NetAdapters {
		if strings.EqualFold(n.NetAdapters[i].GUID, guid) {
			return i
		}
	}
	return -1
}

// dnsKnown: the adapter is listed, or at least its static configuration is
// still in the registry (an adapter turned off in Network Connections).
// n.mu must not be held: the registry has its own lock.
func (n *Native) dnsKnown(guid string) bool {
	n.mu.Lock()
	i := n.adapterIndexLocked(guid)
	n.mu.Unlock()
	if i >= 0 {
		return true
	}
	if n.reg == nil {
		return false
	}
	ok, err := n.reg.KeyExists(mockTcpipIfaces + guid)
	return err == nil && ok
}

// splitList reads a list of DNS servers (commas, semicolons or spaces), the
// same way winsys does.
func splitList(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		if ip := net.ParseIP(strings.TrimSpace(f)); ip != nil {
			out = append(out, ip.String())
		}
	}
	return out
}

func sameList(a, b []string) bool {
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

// DNSConfig implements sys.Native over the simulated registry.
func (n *Native) DNSConfig(guid string) (sys.DNSConfig, error) {
	if !n.dnsKnown(guid) {
		return sys.DNSConfig{}, sys.ErrNotFound
	}
	n.mu.Lock()
	doh := n.DoH[strings.ToLower(guid)]
	n.mu.Unlock()
	// n.reg has its own lock: never call it holding n.mu.
	v4, _ := n.reg.GetValue(mockTcpipIfaces+guid, "NameServer")
	v6, _ := n.reg.GetValue(mockTcpip6Ifaces+guid, "NameServer")
	return sys.DNSConfig{V4: splitList(v4.Str), V6: splitList(v6.Str), DoH: doh}, nil
}

// SetDNSConfig implements sys.Native: only a stack whose list differs from
// the current one is written (DNSWrites counts those writes).
func (n *Native) SetDNSConfig(guid string, v4, v6 []string) error {
	if !n.dnsKnown(guid) {
		return sys.ErrNotFound
	}
	cur, _ := n.DNSConfig(guid)
	wrote := false
	if !sameList(cur.V4, v4) {
		if err := n.reg.SetValue(mockTcpipIfaces+guid, "NameServer", sys.SZ(strings.Join(v4, ","))); err != nil {
			return err
		}
		wrote = true
	}
	if !sameList(cur.V6, v6) {
		if err := n.reg.SetValue(mockTcpip6Ifaces+guid, "NameServer", sys.SZ(strings.Join(v6, ","))); err != nil {
			return err
		}
		wrote = true
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if wrote {
		n.DNSWrites++
	}
	// The effective list: the static servers, or the router's (DHCP).
	if i := n.adapterIndexLocked(guid); i >= 0 {
		eff := append(append([]string(nil), v6...), v4...)
		if len(eff) == 0 {
			eff = []string{"192.168.0.1"}
		}
		n.NetAdapters[i].DNS = eff
	}
	return nil
}
func (n *Native) PowerStatus() (bool, bool, int, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.OnBattery {
		return n.HasBattery, false, 58, nil
	}
	return n.HasBattery, true, 100, nil
}
func (n *Native) DiskSpace(string) (uint64, uint64, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	// Same C: as the seeded Win32_LogicalDisk row.
	return 479_000_000_000, 41_000_000_000 + n.FreeBytes, nil
}
func (n *Native) Resolve(host string) ([]string, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []string{host}, nil
	}
	return []string{"162.254.197.36"}, nil
}
func (n *Native) DNSQuery(server, host string, timeout time.Duration) (time.Duration, error) {
	base := map[string]time.Duration{"1.1.1.1": 9, "8.8.8.8": 14, "9.9.9.9": 22, "208.67.222.222": 18}
	if d, ok := base[server]; ok {
		return d * time.Millisecond, nil
	}
	return 25 * time.Millisecond, nil
}
func (n *Native) PIDsByName(name string) []uint32 {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []uint32
	for _, p := range n.Procs {
		if strings.EqualFold(p.Name, name) {
			out = append(out, p.PID)
		}
	}
	return out
}
func (n *Native) MovePath(src, dst string) error { return nil }

// FirmwareType implements sys.Native.
func (n *Native) FirmwareType() string { return "UEFI" }

// NVMeHealth implements sys.Native.
func (n *Native) NVMeHealth(disk int) (sys.NVMeHealth, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	h, ok := n.NVMe[disk]
	if !ok {
		return sys.NVMeHealth{}, fmt.Errorf("disco %d sem log NVMe", disk)
	}
	return h, nil
}

// PickFile implements sys.Native (the mock returns a fixed path).
func (n *Native) PickFile(title, filterName, pattern string) (string, error) {
	return `C:\Games\Example\Example-Win64-Shipping.exe`, nil
}

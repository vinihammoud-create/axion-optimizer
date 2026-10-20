package mocksys

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Dump returns every registry value as "HIVE\path|name" -> "KIND:display".
func (r *Registry) Dump() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]string{}
	var walk func(prefix string, k *regKey)
	walk = func(prefix string, k *regKey) {
		for low, v := range k.values {
			out[strings.ToLower(prefix)+"|"+low] = v.Kind.String() + ":" + v.Display()
		}
		for _, s := range k.subs {
			walk(prefix+`\`+s.name, s)
		}
	}
	names := map[int]string{1: "HKLM", 2: "HKCU", 3: "HKU", 4: "HKCR"}
	for root, k := range r.roots {
		walk(names[int(root)], k)
	}
	return out
}

// KeyCount returns the number of keys (used to detect leaked empty keys).
func (r *Registry) KeyCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	var walk func(k *regKey)
	walk = func(k *regKey) {
		n++
		for _, s := range k.subs {
			walk(s)
		}
	}
	for _, k := range r.roots {
		walk(k)
	}
	return n
}

// World captures every simulated resource for before/after comparisons.
func (m *Mock) World() map[string]string {
	w := m.R.Dump()
	m.S.mu.Lock()
	for k, v := range m.S.svc {
		w["svc|"+k] = fmt.Sprintf("%d/%v", v.Start, v.Running)
	}
	m.S.mu.Unlock()
	m.P.mu.Lock()
	w["power|active"] = m.P.active
	w["power|overlay"] = m.P.overlay
	for g, vals := range m.P.values {
		if _, ok := m.P.schemes[g]; !ok {
			continue
		}
		for k, v := range vals {
			w["power|"+g+"|"+k] = fmt.Sprint(v)
		}
	}
	m.P.mu.Unlock()
	m.mu.Lock()
	for k, v := range m.BCD {
		w["bcd|"+k] = v
	}
	m.mu.Unlock()
	m.T.mu.Lock()
	for k, v := range m.T.tasks {
		w["task|"+k] = fmt.Sprint(v.Enabled)
	}
	m.T.mu.Unlock()
	m.W.mu.Lock()
	for k, rows := range m.W.Tables {
		for i, r := range rows {
			keys := make([]string, 0, len(r))
			for rk := range r {
				keys = append(keys, rk)
			}
			sort.Strings(keys)
			var sb strings.Builder
			for _, rk := range keys {
				fmt.Fprintf(&sb, "%s=%v;", rk, r[rk])
			}
			w[fmt.Sprintf("wmi|%s|%d", k, i)] = sb.String()
		}
	}
	m.W.mu.Unlock()
	m.N.mu.Lock()
	for _, d := range m.N.Displays {
		w["display|"+d.Device] = fmt.Sprintf("%dx%d@%d", d.Width, d.Height, d.CurrentHz)
	}
	w["mouse"] = fmt.Sprint(m.N.Mouse)
	for _, a := range m.N.NetAdapters {
		w["dns|"+a.GUID] = strings.Join(a.DNS, ",")
	}
	m.N.mu.Unlock()
	// The NVIDIA profile database has its own lock (never nested with N.mu).
	m.N.DRS.dump(w,
		func(ref string, id uint32) string { return fmt.Sprintf("nvdrs|%s|%08x", ref, id) },
		func(v uint32, user bool) string {
			if user {
				return fmt.Sprintf("%d|user", v)
			}
			return fmt.Sprintf("%d|pre", v)
		})
	// Watched folders (own lock, never nested with the ones above).
	m.mu.Lock()
	dirs := append([]string(nil), m.watch...)
	m.mu.Unlock()
	for _, d := range dirs {
		ents, err := os.ReadDir(d)
		if err != nil {
			w["dir|"+strings.ToLower(filepath.ToSlash(d))] = "ausente"
			continue
		}
		for _, e := range ents {
			if !e.Type().IsRegular() {
				continue
			}
			p := filepath.Join(d, e.Name())
			if b, err := os.ReadFile(p); err == nil {
				sum := sha256.Sum256(b)
				w["file|"+strings.ToLower(filepath.ToSlash(p))] = hex.EncodeToString(sum[:])
			}
		}
	}
	return w
}

// DiffWorld lists keys whose values differ between two snapshots, ignoring
// keys that start with any of the ignore prefixes.
func DiffWorld(a, b map[string]string, ignore ...string) []string {
	skip := func(k string) bool {
		for _, p := range ignore {
			if strings.HasPrefix(k, strings.ToLower(p)) || strings.HasPrefix(k, p) {
				return true
			}
		}
		return false
	}
	var out []string
	for k, va := range a {
		if skip(k) {
			continue
		}
		if vb, ok := b[k]; !ok {
			out = append(out, fmt.Sprintf("removido: %s (era %s)", k, va))
		} else if va != vb {
			out = append(out, fmt.Sprintf("alterado: %s: %s -> %s", k, va, vb))
		}
	}
	for k, vb := range b {
		if skip(k) {
			continue
		}
		if _, ok := a[k]; !ok {
			out = append(out, fmt.Sprintf("criado: %s = %s", k, vb))
		}
	}
	sort.Strings(out)
	return out
}

// KeyCountUnder counts keys at or below path.
func (r *Registry) KeyCountUnder(path string) int {
	r.mu.Lock()
	k, _, err := r.find(path, false)
	r.mu.Unlock()
	if err != nil || k == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	var walk func(k *regKey)
	walk = func(k *regKey) {
		n++
		for _, s := range k.subs {
			walk(s)
		}
	}
	walk(k)
	return n
}

// Package programs lists installed programs, runs their own uninstallers,
// finds what they leave behind and moves those leftovers to a quarantine
// from which they can be restored.
package programs

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"axion/internal/sys"
)

// Scopes of an installation.
const (
	ScopeMachine   = "machine"
	ScopeMachine32 = "machine32"
	ScopeUser      = "user"
)

var uninstallRoots = []struct{ key, scope string }{
	{`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, ScopeMachine},
	{`HKLM\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`, ScopeMachine32},
	{`HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall`, ScopeUser},
}

// Program is one entry of Apps & Features.
type Program struct {
	ID          string    `json:"id"` // registry key of the entry
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Publisher   string    `json:"publisher"`
	InstallDate time.Time `json:"installDate"`
	SizeBytes   int64     `json:"sizeBytes"`
	Location    string    `json:"location"`
	Uninstall   string    `json:"uninstall"`
	Icon        string    `json:"icon,omitempty"` // DisplayIcon
	Scope       string    `json:"scope"`
	MSI         bool      `json:"msi"`
	ProductCode string    `json:"productCode,omitempty"`
	NoRemove    bool      `json:"noRemove"`
}

var reGUID = regexp.MustCompile(`(?i)\{[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\}`)

func regStr(r sys.Registry, key, name string) string {
	v, err := r.GetValue(key, name)
	if err != nil || !v.Exists {
		return ""
	}
	if v.Kind == sys.RegDWord || v.Kind == sys.RegQWord {
		return ""
	}
	return strings.TrimSpace(v.Str)
}

func regInt(r sys.Registry, key, name string) int64 {
	v, err := r.GetValue(key, name)
	if err != nil || !v.Exists || (v.Kind != sys.RegDWord && v.Kind != sys.RegQWord) {
		return 0
	}
	return int64(v.Int)
}

// List reads the programs Windows shows in Apps & Features, without system
// components and updates.
func List(r sys.Registry) []Program {
	var out []Program
	seen := map[string]bool{}
	for _, root := range uninstallRoots {
		subs, _ := r.SubKeys(root.key)
		for _, sub := range subs {
			key := root.key + `\` + sub
			name := regStr(r, key, "DisplayName")
			un := regStr(r, key, "UninstallString")
			if name == "" || un == "" || regInt(r, key, "SystemComponent") == 1 || regStr(r, key, "ParentKeyName") != "" {
				continue
			}
			switch strings.ToLower(regStr(r, key, "ReleaseType")) {
			case "update", "hotfix", "security update", "service pack":
				continue
			}
			p := Program{ID: key, Name: name, Version: regStr(r, key, "DisplayVersion"), Publisher: regStr(r, key, "Publisher"),
				Location: strings.Trim(regStr(r, key, "InstallLocation"), `"`), Uninstall: un, Icon: regStr(r, key, "DisplayIcon"), Scope: root.scope,
				NoRemove: regInt(r, key, "NoRemove") == 1}
			if d := regStr(r, key, "InstallDate"); len(d) == 8 {
				if t, err := time.Parse("20060102", d); err == nil {
					p.InstallDate = t
				}
			}
			if kb := regInt(r, key, "EstimatedSize"); kb > 0 {
				p.SizeBytes = kb << 10
			}
			lowUn := strings.ToLower(un)
			if regInt(r, key, "WindowsInstaller") == 1 || strings.Contains(lowUn, "msiexec") {
				p.MSI = true
				if g := reGUID.FindString(sub); g != "" {
					p.ProductCode = strings.ToUpper(g)
				} else if g := reGUID.FindString(un); g != "" {
					p.ProductCode = strings.ToUpper(g)
				}
			}
			dedup := strings.ToLower(p.Name + "|" + p.Version)
			if seen[dedup] {
				continue
			}
			seen[dedup] = true
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// Find returns the program with the given entry key.
func Find(list []Program, id string) *Program {
	for i := range list {
		if strings.EqualFold(list[i].ID, id) {
			return &list[i]
		}
	}
	return nil
}

// Command returns the program's own uninstaller: msiexec /x for Windows
// Installer packages, otherwise the registered UninstallString split into
// executable and arguments. Environment variables are expanded.
func Command(p Program, getenv func(string) string) (exe, args string) {
	if p.MSI && p.ProductCode != "" {
		return "msiexec.exe", "/x " + p.ProductCode
	}
	c := strings.TrimSpace(expand(p.Uninstall, getenv))
	if c == "" {
		return "", ""
	}
	if c[0] == '"' {
		if i := strings.IndexByte(c[1:], '"'); i >= 0 {
			return c[1 : i+1], strings.TrimSpace(c[i+2:])
		}
		return strings.Trim(c, `"`), ""
	}
	low := strings.ToLower(c)
	if i := strings.Index(low, ".exe"); i >= 0 {
		return c[:i+4], strings.TrimSpace(c[i+4:])
	}
	f := strings.Fields(c)
	return f[0], strings.TrimSpace(strings.TrimPrefix(c, f[0]))
}

func expand(s string, getenv func(string) string) string {
	if getenv == nil || !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '%')
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		j := strings.IndexByte(s[i+1:], '%')
		if j < 0 {
			b.WriteString(s)
			return b.String()
		}
		name := s[i+1 : i+1+j]
		b.WriteString(s[:i])
		if v := getenv(name); v != "" {
			b.WriteString(v)
		} else {
			b.WriteString("%" + name + "%")
		}
		s = s[i+2+j:]
	}
}

// RunsAsUser tells whether the uninstaller must run with the user's normal
// rights: per-user installations (anyone can write their registry entry) and
// any uninstaller outside the folders only administrators can change (the
// Windows folder, Program Files and ProgramData). An uninstaller in the
// user's profile or on a games drive (D:\Jogos...) can be swapped by a
// program without administrator rights, which would then run elevated
// through Axion; run as the user, an uninstaller that needs administrator
// rights asks for them itself (UAC). Bare names (msiexec.exe) are Windows'
// own programs, resolved in System32.
func RunsAsUser(p Program, exe string, paths sys.Paths) bool {
	if p.Scope == ScopeUser {
		return true
	}
	if !filepath.IsAbs(exe) {
		return false
	}
	if within(exe, paths.UserProfile) {
		return true
	}
	for _, dir := range []string{paths.WindowsDir, paths.ProgramFiles, paths.ProgramFilesX86, paths.ProgramData} {
		if within(exe, dir) {
			return false
		}
	}
	return true
}

// within reports whether path is inside (or equal to) dir.
func within(path, dir string) bool {
	if path == "" || dir == "" {
		return false
	}
	p := strings.ToLower(filepath.Clean(path))
	d := strings.ToLower(filepath.Clean(dir))
	return p == d || strings.HasPrefix(p, d+string(filepath.Separator))
}

// exists reports whether a path exists (without following a final link).
func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

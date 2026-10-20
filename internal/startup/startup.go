// Package startup lists what starts with Windows (Run keys, Startup folders
// and logon scheduled tasks) and turns entries on or off the way Task
// Manager does: through the StartupApproved values, without deleting the
// entry, so everything can be turned back on.
package startup

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"axion/internal/sys"
)

// Sources of startup entries.
const (
	SrcRunUser      = "run-user"
	SrcRunMachine   = "run-machine"
	SrcRunMachine32 = "run-machine32"
	SrcFolderUser   = "folder-user"
	SrcFolderAll    = "folder-common"
	SrcTask         = "task"
)

type source struct {
	id, label           string
	runKey, approvedKey string
}

var runSources = []source{
	{SrcRunUser, "Registro (usuário)", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`},
	{SrcRunMachine, "Registro (todos os usuários)", `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`},
	{SrcRunMachine32, "Registro (programas 32 bits)", `HKLM\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Run`, `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run32`},
}

const (
	approvedFolderUser = `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\StartupFolder`
	approvedFolderAll  = `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\StartupFolder`
)

// Item is one thing that starts with Windows.
type Item struct {
	ID          string `json:"id"` // source|name
	Name        string `json:"name"`
	Command     string `json:"command"`
	Exe         string `json:"exe"`
	Source      string `json:"source"`
	SourceLabel string `json:"sourceLabel"`
	Enabled     bool   `json:"enabled"`
	Running     bool   `json:"running"`
	Missing     bool   `json:"missing"` // the program no longer exists
	Category    string `json:"category"`
	Verdict     string `json:"verdict"` // desativar, manter, avaliar
	Advice      string `json:"advice"`
	// New is set by the app when the entry was not there at the last visit to
	// the page. It is a notice only: nothing is blocked or turned off by it.
	New bool `json:"new"`
	// ByAxion is set by the app when the state the entry is in now was put
	// there by Axion (an active Histórico entry): undoing it brings it back.
	ByAxion bool `json:"byAxion,omitempty"`
}

// Approved decodes a StartupApproved value: the low bit of the first byte
// set means "disabled" (Task Manager writes 02 for on, 03 for off).
func Approved(v sys.RegValue) (enabled bool) {
	if !v.Exists || len(v.Bin) == 0 {
		return true
	}
	return v.Bin[0]&1 == 0
}

// ApprovedValue builds the 12-byte value Task Manager writes: a flag DWORD
// and the FILETIME of the change (zero when enabling).
func ApprovedValue(enabled bool, now time.Time) sys.RegValue {
	b := make([]byte, 12)
	if enabled {
		b[0] = 0x02
	} else {
		b[0] = 0x03
		ft := uint64(now.UnixNano()/100) + 116444736000000000
		binary.LittleEndian.PutUint64(b[4:], ft)
	}
	return sys.Binary(b)
}

// ExePath extracts the program from a command line ("C:\x y\a.exe" -b,
// C:\x\a.exe /b, rundll32 x.dll,Entry) and expands %VARS%.
func ExePath(cmd string, getenv func(string) string) string {
	c := strings.TrimSpace(expand(cmd, getenv))
	if c == "" {
		return ""
	}
	if c[0] == '"' {
		if i := strings.IndexByte(c[1:], '"'); i >= 0 {
			return c[1 : i+1]
		}
		return strings.Trim(c, `"`)
	}
	low := strings.ToLower(c)
	for _, ext := range []string{".exe", ".bat", ".cmd", ".lnk", ".com"} {
		if i := strings.Index(low, ext); i >= 0 {
			return c[:i+len(ext)]
		}
	}
	if f := strings.Fields(c); len(f) > 0 {
		return f[0]
	}
	return c
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

// List reads every startup entry.
func List(p *sys.Platform, getenv func(string) string) []Item {
	var out []Item
	for _, s := range runSources {
		names, _ := p.Reg.ValueNames(s.runKey)
		for _, n := range names {
			v, err := p.Reg.GetValue(s.runKey, n)
			if err != nil || !v.Exists || n == "" {
				continue
			}
			ap, _ := p.Reg.GetValue(s.approvedKey, n)
			cmd := v.Str
			out = append(out, Item{ID: s.id + "|" + n, Name: n, Command: cmd, Exe: ExePath(cmd, getenv), Source: s.id, SourceLabel: s.label, Enabled: Approved(ap)})
		}
	}
	paths := p.Native.Paths()
	for _, f := range []struct {
		id, label, dir, approved string
	}{
		{SrcFolderUser, "Pasta Inicializar (usuário)", paths.StartupUser, approvedFolderUser},
		{SrcFolderAll, "Pasta Inicializar (todos os usuários)", paths.StartupCommon, approvedFolderAll},
	} {
		if f.dir == "" {
			continue
		}
		entries, _ := os.ReadDir(f.dir)
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || strings.EqualFold(name, "desktop.ini") {
				continue
			}
			ap, _ := p.Reg.GetValue(f.approved, name)
			full := filepath.Join(f.dir, name)
			out = append(out, Item{ID: f.id + "|" + name, Name: strings.TrimSuffix(name, filepath.Ext(name)), Command: full, Exe: full, Source: f.id, SourceLabel: f.label, Enabled: Approved(ap)})
		}
	}
	if tasks, err := p.Tasks.List(`\`, true); err == nil {
		for _, t := range tasks {
			if !t.Exists || strings.HasPrefix(strings.ToLower(t.Path), `\microsoft\`) || !hasTrigger(t.Triggers, sys.TriggerLogon) {
				continue
			}
			cmd := strings.Join(t.Actions, " ")
			out = append(out, Item{ID: SrcTask + "|" + t.Path, Name: t.Name, Command: cmd, Exe: ExePath(cmd, getenv), Source: SrcTask, SourceLabel: "Tarefa agendada (ao entrar)", Enabled: t.Enabled})
		}
	}
	running := map[string]bool{}
	if procs, err := p.Native.Processes(); err == nil {
		for _, pr := range procs {
			running[strings.ToLower(pr.Name)] = true
		}
	}
	for i := range out {
		it := &out[i]
		base := strings.ToLower(filepath.Base(strings.ReplaceAll(it.Exe, `\`, "/")))
		it.Running = base != "" && running[base]
		// "--processStart Discord.exe" style launchers start another program.
		if f := strings.Fields(strings.ToLower(it.Command)); len(f) > 0 {
			for k, x := range f {
				if x == "--processstart" && k+1 < len(f) && running[strings.Trim(f[k+1], `"`)] {
					it.Running = true
				}
			}
		}
		if filepath.IsAbs(it.Exe) && !strings.HasSuffix(strings.ToLower(it.Exe), ".lnk") {
			// Commands mix separators; normalize for the local file system.
			if !exists(p.Native, filepath.FromSlash(strings.ReplaceAll(it.Exe, `\`, "/"))) {
				it.Missing = true
			}
		}
		classify(it)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Enabled != out[j].Enabled {
			return out[i].Enabled
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// fileChecker is implemented by a platform whose programs do not live on the
// host's file system (the simulated machine lists Windows paths that have no
// counterpart on disk, so asking the disk would call every entry broken).
type fileChecker interface{ FileExists(path string) bool }

// exists reports whether the program at path is still there.
func exists(n sys.Native, path string) bool {
	if fc, ok := n.(fileChecker); ok {
		return fc.FileExists(path)
	}
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func hasTrigger(ts []int, want int) bool {
	for _, t := range ts {
		if t == want {
			return true
		}
	}
	return false
}

// Location says where the on/off state of an entry lives.
type Location struct {
	ID     string // "source|name"
	Name   string // name for the user (folder: no extension; task: last path segment)
	Task   string // task path (SrcTask only)
	Key    string // StartupApproved key (Registry and folders)
	Value  string // value name in Key
	RunKey string // Run key where the entry lives (Registry only)
}

// Locate says where an entry's on/off state lives, without touching the
// machine.
func Locate(id string) (Location, error) {
	src, name, ok := strings.Cut(id, "|")
	if !ok || name == "" {
		return Location{}, errBadID
	}
	l := Location{ID: id, Name: name}
	switch src {
	case SrcTask:
		l.Task = name
		l.Name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
		return l, nil
	case SrcFolderUser, SrcFolderAll:
		l.Key, l.Value = approvedFolderUser, name
		if src == SrcFolderAll {
			l.Key = approvedFolderAll
		}
		l.Name = strings.TrimSuffix(name, filepath.Ext(name))
	default:
		for _, s := range runSources {
			if s.id == src {
				l.Key, l.Value, l.RunKey = s.approvedKey, name, s.runKey
			}
		}
	}
	if l.Key == "" {
		return Location{}, errBadID
	}
	return l, nil
}

// SetEnabled turns an entry on or off (StartupApproved or task state). The
// app goes through the tweak engine instead (catalog.StartupProvider), which
// keeps the previous value and lets the Histórico undo it; this stays for
// tests and for callers that do not need the journal.
func SetEnabled(p *sys.Platform, id string, enabled bool, now time.Time) error {
	l, err := Locate(id)
	if err != nil {
		return err
	}
	if l.Task != "" {
		return p.Tasks.SetEnabled(l.Task, enabled)
	}
	if l.RunKey != "" {
		if v, err := p.Reg.GetValue(l.RunKey, l.Value); err != nil || !v.Exists {
			return errMissing
		}
	}
	return p.Reg.SetValue(l.Key, l.Value, ApprovedValue(enabled, now))
}

type startupErr string

func (e startupErr) Error() string { return string(e) }

const (
	errBadID   = startupErr("item de inicialização inválido")
	errMissing = startupErr("o item não existe mais; atualize a lista")
)

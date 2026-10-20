package programs

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"axion/internal/startup"
	"axion/internal/sys"
)

// Leftover kinds.
const (
	LeftDir      = "dir"
	LeftFile     = "file"
	LeftRegKey   = "regkey"
	LeftRunValue = "runvalue"
	LeftTask     = "task"
)

// Leftover is something a program left behind after its uninstaller ran.
type Leftover struct {
	ID      string `json:"id"` // kind|path
	Kind    string `json:"kind"`
	Path    string `json:"path"`            // folder, file, registry key or task path
	Value   string `json:"value,omitempty"` // run value name
	Label   string `json:"label"`
	Bytes   int64  `json:"bytes"`
	Default bool   `json:"default"`
	Why     string `json:"why"`
}

// Snapshot is what is known about a program before its uninstaller runs.
type Snapshot struct {
	Program Program `json:"program"`
}

var (
	reVersion = regexp.MustCompile(`(?i)\s*(\(?\s*(x64|x86|64-bit|32-bit|64 bits|32 bits|remove only)\s*\)?|\s+v?\d+(\.\d+){1,3}[a-z]?|\s+\(\d+(\.\d+)*\))\s*$`)
	// Names too generic to be tied to a single program.
	genericNames = map[string]bool{
		"microsoft": true, "windows": true, "common files": true, "classes": true, "policies": true, "wow6432node": true,
		"programs": true, "packages": true, "temp": true, "update": true, "updates": true, "tools": true, "data": true,
		"shared": true, "games": true, "cache": true, "system": true, "drivers": true, "logs": true, "nvidia corporation": true,
		"intel": true, "amd": true, "google": true, "mozilla": true, "adobe": true, "steam": true, "valve": true, "apps": true,
		"bin": true, "x64": true, "x86": true, "application": true, "launcher": true, "program": true, "app": true, "client": true,
		"common": true, "runtime": true, "redist": true, "setup": true, "installer": true, "uninstall": true,
	}
)

// cleanName removes version numbers and architecture tags from a name.
func cleanName(s string) string {
	s = strings.TrimSpace(s)
	for i := 0; i < 3; i++ {
		t := strings.TrimSpace(reVersion.ReplaceAllString(s, ""))
		if t == s || t == "" {
			break
		}
		s = t
	}
	return s
}

// installDirDefault decides whether the folder the program was installed
// into is offered already ticked. Installers that let the user pick an
// existing folder register it as-is (D:\Games, C:\Tools...), and after the
// uninstaller ran it may still hold other games, saves and files: the folder
// is only ticked when its name is the program's (or its maker's) and its size
// fits what the program said it used.
func installDirDefault(prog Program, dir string, size int64) (bool, string) {
	why := "pasta de instalação que o desinstalador deixou"
	base := strings.TrimRight(dir, `\/`)
	if i := strings.LastIndexAny(base, `\/`); i >= 0 {
		base = base[i+1:]
	}
	if !usableName(base) || !relatedName(base, prog) {
		return false, why + " (o nome da pasta não é o do programa: pode ter outros arquivos; confira o conteúdo)"
	}
	const small = 20 << 20
	switch {
	case prog.SizeBytes <= 0 && size > 50<<20:
		return false, why + " (o programa não informou o tamanho e a pasta é grande; confira o conteúdo)"
	case prog.SizeBytes > 0 && size > small && size > 4*prog.SizeBytes:
		return false, why + " (maior que o tamanho informado pelo programa; confira o conteúdo)"
	}
	return true, why
}

// relatedName reports whether a folder name is the program's own name or its
// maker's (either contains the other, ignoring case and spaces).
func relatedName(base string, prog Program) bool {
	norm := func(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), "")) }
	b := norm(base)
	if len(b) < 3 {
		return false
	}
	// Only the program's and its maker's names count: names() also holds
	// the install folder's own name, which would always match itself.
	own := []string{prog.Name, cleanName(prog.Name)}
	for _, n := range append(own, publisherNames(prog)...) {
		if !usableName(n) {
			continue
		}
		x := norm(n)
		if len(x) < 3 {
			continue
		}
		if strings.Contains(b, x) || (len(b) >= 4 && strings.Contains(x, b)) {
			return true
		}
	}
	return false
}

func usableName(s string) bool {
	l := strings.ToLower(strings.TrimSpace(s))
	return len([]rune(l)) >= 3 && !genericNames[l] && !strings.ContainsAny(l, `\/:*?"<>|`)
}

// names returns the candidate folder/key names of a program.
func names(p Program) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if usableName(s) && !seen[strings.ToLower(s)] {
			seen[strings.ToLower(s)] = true
			out = append(out, s)
		}
	}
	add(p.Name)
	add(cleanName(p.Name))
	if p.Location != "" {
		add(filepath.Base(filepath.Clean(p.Location)))
	}
	return out
}

// publisherNames returns the folder/key names of the publisher, or nothing
// when the publisher is too generic (Microsoft, Intel…) to separate one
// program from the others.
func publisherNames(p Program) []string {
	if !usableName(cleanCompany(p.Publisher)) {
		return nil
	}
	var out []string
	for _, s := range []string{p.Publisher, strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(p.Publisher), "."), ", Inc"), cleanCompany(p.Publisher)} {
		s = strings.TrimSpace(s)
		if s != "" && len([]rune(s)) >= 2 && !strings.ContainsAny(s, `\/:*?"<>|`) {
			dup := false
			for _, o := range out {
				if strings.EqualFold(o, s) {
					dup = true
				}
			}
			if !dup {
				out = append(out, s)
			}
		}
	}
	return out
}

var reCompany = regexp.MustCompile(`(?i)[,.]?\s+(inc|llc|ltd|limited|corp|corporation|co|gmbh|s\.?a|ltda)\.?$`)

func cleanCompany(s string) string {
	return strings.TrimSpace(reCompany.ReplaceAllString(strings.TrimSpace(s), ""))
}

// personalDirs are the user's own folders. They are never offered, and a
// program folder inside one of them is offered unticked.
func personalDirs(paths sys.Paths) []string {
	if paths.UserProfile == "" {
		return nil
	}
	var out []string
	for _, n := range []string{"Desktop", "Documents", "Downloads", "Pictures", "Music", "Videos", "Saved Games", "OneDrive", "Favorites", "Contacts", "Links", "Searches", "3D Objects", "AppData", filepath.Join("AppData", "LocalLow")} {
		out = append(out, filepath.Join(paths.UserProfile, n))
	}
	// Where Windows really keeps them (moved to another drive, OneDrive
	// backup "OneDrive - Empresa"...).
	for _, d := range paths.Personal {
		if d != "" {
			out = append(out, filepath.Clean(d))
		}
	}
	if paths.UserProfile != "" {
		if ents, err := os.ReadDir(paths.UserProfile); err == nil {
			for _, e := range ents {
				if e.IsDir() && strings.HasPrefix(strings.ToLower(e.Name()), "onedrive") {
					out = append(out, filepath.Join(paths.UserProfile, e.Name()))
				}
			}
		}
	}
	return out
}

// protectedDirs are folders never offered for removal (nor their parents).
func protectedDirs(paths sys.Paths) []string {
	var out []string
	add := func(p string) {
		if p != "" {
			out = append(out, filepath.Clean(p))
		}
	}
	for _, p := range []string{paths.WindowsDir, paths.ProgramFiles, paths.ProgramFilesX86, paths.ProgramData, paths.AppData, paths.LocalAppData, paths.UserProfile, paths.SystemDrive, paths.UserTemp, paths.WindowsTemp} {
		add(p)
	}
	for _, base := range []string{paths.ProgramFiles, paths.ProgramFilesX86} {
		if base != "" {
			add(filepath.Join(base, "Common Files"))
			add(filepath.Join(base, "WindowsApps"))
			add(filepath.Join(base, "Microsoft"))
		}
	}
	for _, base := range []string{paths.ProgramData, paths.AppData, paths.LocalAppData} {
		if base != "" {
			add(filepath.Join(base, "Microsoft"))
			add(filepath.Join(base, "Packages"))
			add(filepath.Join(base, "Programs"))
		}
	}
	out = append(out, personalDirs(paths)...)
	return out
}

// specificDir reports whether a folder is specific enough to belong to one
// program: an absolute local path at least two levels deep, not protected,
// not a parent of a protected folder or of another installed program, not
// inside Windows and not inside another user's profile.
func specificDir(dir string, paths sys.Paths, protected, others []string) bool {
	c := filepath.Clean(dir)
	slash := filepath.ToSlash(c)
	if !filepath.IsAbs(c) || strings.HasPrefix(slash, "//") || len(strings.Split(strings.Trim(slash, "/"), "/")) < 2 {
		return false
	}
	for _, p := range protected {
		if within(p, c) { // c is p or a parent of p
			return false
		}
	}
	for _, o := range others {
		if o != "" && within(o, c) {
			return false
		}
	}
	if within(c, paths.WindowsDir) {
		return false
	}
	if paths.UserProfile != "" {
		users := filepath.Dir(filepath.Clean(paths.UserProfile))
		if within(c, users) && !within(c, paths.UserProfile) {
			return false // another user's profile, or Public
		}
	}
	return true
}

// safeDir is specificDir for a folder that exists and is not a link.
func safeDir(dir string, paths sys.Paths, protected, others []string) bool {
	c := filepath.Clean(dir)
	if !specificDir(c, paths, protected, others) {
		return false
	}
	info, err := os.Lstat(c)
	if err != nil || !info.IsDir() || info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
		return false
	}
	return true
}

// dirSize adds the sizes of regular files (links are not followed).
func dirSize(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}

// programDirs lists the folders a program occupies: its InstallLocation and
// the folders of its uninstaller and icon, when they are its own (not the
// Windows folder, where msiexec and rundll32 live).
func programDirs(o Program, paths sys.Paths, getenv func(string) string) []string {
	var out []string
	add := func(d string) {
		d = strings.Trim(strings.TrimSpace(d), `"`)
		if d == "" || !filepath.IsAbs(d) {
			return
		}
		d = filepath.Clean(d)
		if paths.WindowsDir != "" && within(d, paths.WindowsDir) {
			return
		}
		out = append(out, d)
	}
	add(o.Location)
	if !o.MSI {
		if exe, _ := Command(o, getenv); exe != "" {
			add(filepath.Dir(exe))
		}
	}
	if o.Icon != "" {
		icon := expand(o.Icon, getenv)
		if i := strings.LastIndex(icon, ","); i > 0 && !strings.Contains(icon[i:], `\`) {
			icon = icon[:i] // "app.exe,0"
		}
		add(filepath.Dir(strings.Trim(icon, `"`)))
	}
	return out
}

// FindLeftovers looks for what the program left behind. remaining are the
// programs still installed (their folders are never offered).
func FindLeftovers(p *sys.Platform, snap Snapshot, remaining []Program, getenv func(string) string) []Leftover {
	prog := snap.Program
	paths := p.Native.Paths()
	protected := protectedDirs(paths)
	personal := personalDirs(paths)
	var others []string
	// Names used by programs that stay installed (other versions of the same
	// product, other products of the vendor): a folder or key with one of
	// those names is shared and never offered.
	taken := map[string]bool{}
	for _, o := range remaining {
		if strings.EqualFold(o.ID, prog.ID) {
			continue
		}
		// Where the program that stays lives: its registered folder, and
		// the folders of its uninstaller and icon (many entries have no
		// InstallLocation).
		others = append(others, programDirs(o, paths, getenv)...)
		for _, n := range append(names(o), publisherNames(o)...) {
			taken[strings.ToLower(n)] = true
		}
	}
	var own []string
	for _, n := range names(prog) {
		if !taken[strings.ToLower(n)] {
			own = append(own, n)
		}
	}
	// A folder or key found by the program's own name comes checked; one
	// found only by the name of its install folder (C:\Program Files\Vendor
	// for "Vendor Tool") may belong to other products of the same vendor and
	// comes unchecked.
	byName := map[string]bool{strings.ToLower(strings.TrimSpace(prog.Name)): true, strings.ToLower(strings.TrimSpace(cleanName(prog.Name))): true}
	var out []Leftover
	seen := map[string]bool{}
	addDir := func(dir, why string, def bool) {
		c := filepath.Clean(dir)
		key := strings.ToLower(c)
		if seen[key] || !safeDir(c, paths, protected, others) {
			return
		}
		seen[key] = true
		for _, pd := range personal {
			if within(c, pd) {
				def = false // may hold the user's own files
			}
		}
		out = append(out, Leftover{ID: LeftDir + "|" + c, Kind: LeftDir, Path: c, Label: c, Bytes: dirSize(c), Default: def, Why: why})
	}
	if prog.Location != "" && exists(prog.Location) && !taken[strings.ToLower(filepath.Base(filepath.Clean(prog.Location)))] {
		// (A location named like a product or vendor that stays installed,
		// e.g. "...\Razer" for one of several Razer programs, is shared.)
		def, why := installDirDefault(prog, prog.Location, dirSize(prog.Location))
		addDir(prog.Location, why, def)
	}
	for _, base := range []string{paths.ProgramFiles, paths.ProgramFilesX86, paths.ProgramData, paths.AppData, paths.LocalAppData} {
		if base == "" {
			continue
		}
		for _, n := range own {
			d := filepath.Join(base, n)
			if exists(d) {
				if byName[strings.ToLower(n)] {
					addDir(d, "pasta com o nome do programa", true)
				} else {
					addDir(d, "pasta com o nome da pasta de instalação (pode ser de outro produto do mesmo fabricante; confira antes de marcar)", false)
				}
			}
			for _, pub := range publisherNames(prog) {
				d := filepath.Join(base, pub, n)
				if exists(d) {
					addDir(d, "pasta do programa dentro da pasta do fabricante", true)
				}
			}
		}
	}

	// Registry: the orphan entry and the program's own settings keys.
	if ok, _ := p.Reg.KeyExists(prog.ID); ok {
		out = append(out, Leftover{ID: LeftRegKey + "|" + prog.ID, Kind: LeftRegKey, Path: prog.ID, Label: prog.ID, Default: true, Why: "entrada de desinstalação que sobrou (o programa aparece como instalado)"})
		seen[strings.ToLower(prog.ID)] = true
	}
	type regCand struct {
		key string
		def bool
	}
	var keys []regCand
	for _, root := range []string{`HKCU\Software`, `HKLM\SOFTWARE`, `HKLM\SOFTWARE\WOW6432Node`} {
		for _, n := range own {
			def := byName[strings.ToLower(n)]
			keys = append(keys, regCand{root + `\` + n, def})
			for _, pub := range publisherNames(prog) {
				keys = append(keys, regCand{root + `\` + pub + `\` + n, def})
			}
		}
	}
	for _, k := range keys {
		low := strings.ToLower(k.key)
		if seen[low] {
			continue
		}
		if ok, _ := p.Reg.KeyExists(k.key); ok {
			seen[low] = true
			why := "configurações do programa no Registro"
			if !k.def {
				why = "chave com o nome da pasta de instalação (pode ser de outro produto do mesmo fabricante; confira antes de marcar)"
			}
			out = append(out, Leftover{ID: LeftRegKey + "|" + k.key, Kind: LeftRegKey, Path: k.key, Label: k.key, Default: k.def, Why: why})
		}
	}

	// Startup entries and scheduled tasks that point into the old folder,
	// only when that folder is the program's own (not a shared or broad one).
	if prog.Location != "" && specificDir(prog.Location, paths, protected, others) {
		for _, it := range startup.List(p, getenv) {
			if it.Exe == "" || !within(it.Exe, prog.Location) {
				continue
			}
			src, name, _ := strings.Cut(it.ID, "|")
			switch src {
			case startup.SrcTask:
				if !it.Enabled {
					// Already off: disabling does nothing, and a restore
					// would switch it on (the prior state is not kept).
					continue
				}
				out = append(out, Leftover{ID: LeftTask + "|" + name, Kind: LeftTask, Path: name, Label: "Tarefa agendada " + name, Default: true, Why: "tarefa que abre um programa que não existe mais (será desativada)"})
			case startup.SrcRunUser, startup.SrcRunMachine, startup.SrcRunMachine32:
				key := runKeyOf(src)
				out = append(out, Leftover{ID: LeftRunValue + "|" + key + "|" + name, Kind: LeftRunValue, Path: key, Value: name, Label: "Inicialização: " + name, Default: true, Why: "entrada de inicialização do programa removido"})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}

func runKeyOf(src string) string {
	switch src {
	case startup.SrcRunUser:
		return `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	case startup.SrcRunMachine32:
		return `HKLM\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Run`
	}
	return `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
}

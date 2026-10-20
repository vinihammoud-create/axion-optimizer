// Package cleanup lists what can be deleted safely to free disk space,
// measures it before anything is removed and deletes only what the user
// chose. Only disposable data is covered (temporary files, error reports,
// crash dumps, the recycle bin); caches that make games or Windows faster
// (shaders, Prefetch) are deliberately left alone.
package cleanup

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"axion/internal/sys"
	"axion/internal/tweaks"
)

// Kinds of targets.
const (
	KindFiles   = "files"   // files under folders, filtered by pattern and age
	KindRecycle = "recycle" // the recycle bin (shell API)
	KindDISM    = "dism"    // component store cleanup (DISM)
	KindLink    = "link"    // handled by Windows Settings, only measured and linked
)

var (
	srcStorageSense = tweaks.Source{Title: "Microsoft Support — Liberar espaço em disco (Sensor de Armazenamento e arquivos temporários)", URL: "https://support.microsoft.com/windows/free-up-drive-space-in-windows-85529ccb-c365-490d-b548-831022bc9b32"}
	srcWinSxS       = tweaks.Source{Title: "Microsoft Learn — Limpar a pasta WinSxS (DISM /StartComponentCleanup)", URL: "https://learn.microsoft.com/windows-hardware/manufacture/desktop/clean-up-the-winsxs-folder"}
	srcDumps        = tweaks.Source{Title: "Microsoft Learn — Visão geral dos arquivos de despejo de memória do modo kernel", URL: "https://learn.microsoft.com/windows-hardware/drivers/debugger/kernel-mode-dump-files"}
	srcWER          = tweaks.Source{Title: "Microsoft Learn — Windows Error Reporting (relatórios de falha guardados no PC)", URL: "https://learn.microsoft.com/windows/win32/wer/windows-error-reporting"}
)

// Target is one kind of data the page offers to delete.
type Target struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	Detail   string        `json:"detail"`
	Kind     string        `json:"kind"`
	Dirs     []string      `json:"-"`
	Files    []string      `json:"-"`
	Patterns []string      `json:"-"` // lower-case globs matched against file names; empty = all
	MinAge   time.Duration `json:"-"` // files modified more recently are kept
	Recurse  bool          `json:"-"`
	Default  bool          `json:"default"` // pre-selected
	// Keep lists files (lower-case full paths) that must stay, e.g. those a
	// pending restart will still move into place.
	Keep    map[string]bool `json:"-"`
	Caution string          `json:"caution,omitempty"`
	Link    string          `json:"link,omitempty"` // ms-settings page for KindLink
	Sources []tweaks.Source `json:"sources"`
}

// Targets builds the list for this PC.
func Targets(p sys.Paths) []Target {
	var t []Target
	add := func(x Target) { t = append(t, x) }
	userTemp := TempDir(p, p.UserTemp)
	if userTemp == "" {
		userTemp = TempDir(p, join(p.LocalAppData, "Temp"))
	}
	add(Target{ID: "temp.user", Title: "Arquivos temporários do usuário", Kind: KindFiles, Recurse: true, MinAge: 24 * time.Hour, Default: true,
		Dirs:    nonEmpty(userTemp),
		Detail:  "Pasta TEMP da sua conta: sobras de instaladores, atualizações e programas. Arquivos em uso, criados ou modificados nas últimas 24 horas, e pastas recém-extraídas ficam.",
		Sources: []tweaks.Source{srcStorageSense}})
	add(Target{ID: "temp.windows", Title: "Arquivos temporários do Windows", Kind: KindFiles, Recurse: true, MinAge: 24 * time.Hour, Default: true,
		Dirs:    nonEmpty(TempDir(p, p.WindowsTemp)),
		Detail:  "Pasta Temp do Windows (instalações e serviços). Arquivos em uso ou das últimas 24 horas ficam.",
		Sources: []tweaks.Source{srcStorageSense}})
	add(Target{ID: "wer", Title: "Relatórios de erro do Windows", Kind: KindFiles, Recurse: true, MinAge: 24 * time.Hour, Default: true,
		Dirs: nonEmpty(join(p.ProgramData, `Microsoft\Windows\WER\ReportArchive`), join(p.ProgramData, `Microsoft\Windows\WER\ReportQueue`),
			join(p.LocalAppData, `Microsoft\Windows\WER\ReportArchive`), join(p.LocalAppData, `Microsoft\Windows\WER\ReportQueue`)),
		Detail:  "Cópias locais de relatórios de travamento já processados. O histórico de confiabilidade e o Visualizador de Eventos continuam com os registros.",
		Sources: []tweaks.Source{srcWER, srcStorageSense}})
	add(Target{ID: "dumps.apps", Title: "Dumps de programas que travaram", Kind: KindFiles, Recurse: false, MinAge: 7 * 24 * time.Hour, Patterns: []string{"*.dmp"},
		Dirs:    nonEmpty(join(p.LocalAppData, "CrashDumps")),
		Detail:  "Cópias da memória de programas no momento em que travaram (podem ter centenas de MB). Só servem para o suporte do programa analisar; mantidos os dos últimos 7 dias.",
		Sources: []tweaks.Source{srcWER}})
	add(Target{ID: "dumps.memory", Title: "Despejo completo de tela azul (MEMORY.DMP)", Kind: KindFiles,
		Files:   nonEmpty(join(p.WindowsDir, "MEMORY.DMP")),
		Detail:  "Cópia da memória gravada na última tela azul. Pode ocupar vários GB.",
		Caution: "Se você está investigando telas azuis recentes, guarde este arquivo até terminar.",
		Sources: []tweaks.Source{srcDumps}})
	add(Target{ID: "dumps.mini", Title: "Minidumps de tela azul", Kind: KindFiles, Patterns: []string{"*.dmp"},
		Dirs:    nonEmpty(join(p.WindowsDir, "Minidump")),
		Detail:  "Resumos pequenos de cada tela azul, usados para descobrir o driver culpado.",
		Caution: "Ocupam pouco e ajudam a diagnosticar travamentos; apague só depois de resolver o problema.",
		Sources: []tweaks.Source{srcDumps}})
	add(Target{ID: "recycle", Title: "Lixeira", Kind: KindRecycle,
		Detail:  "Arquivos que você já apagou, em todas as unidades.",
		Caution: "Esvaziar é definitivo: confira antes se não há nada que queira recuperar.",
		Sources: []tweaks.Source{srcStorageSense}})
	add(Target{ID: "winsxs", Title: "Componentes antigos do Windows Update (WinSxS)", Kind: KindDISM,
		Detail:  "Remove versões substituídas de componentes do Windows com a ferramenta oficial (DISM /StartComponentCleanup): a mesma limpeza da tarefa do Windows, mas sem esperar os 30 dias que ela aguarda. Pode levar de 5 a 20 minutos.",
		Caution: "Depois dela não dá para desinstalar as atualizações já substituídas nem voltar às versões anteriores dos componentes.",
		Sources: []tweaks.Source{srcWinSxS}})
	add(Target{ID: "windows.old", Title: "Instalação anterior do Windows (Windows.old)", Kind: KindLink, Recurse: true,
		Dirs:    nonEmpty(join(driveRoot(p.SystemDrive), "Windows.old")),
		Link:    "ms-settings:storagesense",
		Detail:  "Cópia da versão anterior do Windows, usada para voltar atrás depois de uma atualização grande. O próprio Windows a remove em alguns dias; para apagar antes, use Configurações > Armazenamento > Arquivos temporários.",
		Sources: []tweaks.Source{srcStorageSense}})
	return t
}

// TempDir accepts a temporary folder only when it really is one: a folder
// named Temp or Tmp that is not itself one of the user's or the system's
// main folders. GetTempPath (behind os.TempDir) falls back to the user
// profile and then to the Windows folder when TMP and TEMP are not set; a
// recursive cleanup there would delete documents and saves. It returns ""
// when dir is not acceptable.
func TempDir(p sys.Paths, dir string) string {
	if dir == "" {
		return ""
	}
	c := filepath.Clean(dir)
	if safeRoot(c) != nil {
		return ""
	}
	switch strings.ToLower(filepath.Base(c)) {
	case "temp", "tmp":
	default:
		return ""
	}
	for _, x := range protected(p) {
		if samePath(c, x) {
			return ""
		}
	}
	return c
}

// protected are folders that a cleanup must never use as its root.
func protected(p sys.Paths) []string {
	out := nonEmpty(p.WindowsDir, p.UserProfile, p.ProgramData, p.ProgramFiles, p.ProgramFilesX86, p.AppData, p.LocalAppData,
		p.StartupUser, p.StartupCommon)
	if p.SystemDrive != "" {
		out = append(out, driveRoot(p.SystemDrive))
	}
	if p.UserProfile != "" {
		out = append(out, filepath.Dir(filepath.Clean(p.UserProfile))) // C:\Users
	}
	return out
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// driveRoot turns "C:" into "C:\" (a bare drive letter means the current
// directory of that drive).
func driveRoot(d string) string {
	if strings.HasSuffix(d, ":") {
		return d + `\`
	}
	return d
}

func join(base, rel string) string {
	if base == "" || base == `\` {
		return ""
	}
	return filepath.Join(base, filepath.FromSlash(strings.ReplaceAll(rel, `\`, "/")))
}

func nonEmpty(xs ...string) []string {
	var out []string
	for _, x := range xs {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

// Measure is what a target would free right now.
type Measure struct {
	ID      string    `json:"id"`
	Bytes   int64     `json:"bytes"`
	Files   int       `json:"files"`
	Kept    int       `json:"kept"` // recent files that stay
	Newest  time.Time `json:"newest,omitempty"`
	Present bool      `json:"present"` // something exists at the location
	Partial bool      `json:"partial,omitempty"`
	Note    string    `json:"note,omitempty"`
}

// Result of deleting a target.
type Result struct {
	ID      string   `json:"id"`
	Freed   int64    `json:"freed"`
	Deleted int      `json:"deleted"`
	Skipped int      `json:"skipped"` // in use or protected
	Errors  []string `json:"errors,omitempty"`
}

func matches(t Target, name string) bool {
	if len(t.Patterns) == 0 {
		return true
	}
	n := strings.ToLower(name)
	for _, p := range t.Patterns {
		if ok, _ := filepath.Match(p, n); ok {
			return true
		}
	}
	return false
}

// safeRoot refuses roots that would be catastrophic if the configuration
// were wrong (a drive root, the Windows folder itself, a user profile).
func safeRoot(dir string) error {
	clean := filepath.Clean(dir)
	if clean == "" || clean == "." || clean == string(filepath.Separator) || filepath.VolumeName(clean)+string(filepath.Separator) == clean || len(clean) < 8 {
		return errors.New("pasta raiz recusada: " + dir)
	}
	return nil
}

// fileTime is when a file was last written or created, whichever is later.
// Installers and archive tools restore the modification time stored in the
// package, so a file extracted a minute ago can look years old by that time
// alone; its creation time is when it was put on the disk.
func fileTime(info fs.FileInfo) time.Time {
	t := info.ModTime()
	if c := createdAt(info); c.After(t) {
		t = c
	}
	return t
}

// walk visits the files of a target, never following links or junctions.
// Files that are too recent, inside a folder created or changed recently (an
// installer extracting now) or listed in Keep are counted as kept. fn
// returns whether the file was handled.
func walk(t Target, now time.Time, deadline time.Time, fn func(path string, info fs.FileInfo) bool) (kept int, partial bool) {
	recentDirs := map[string]bool{}
	recent := func(info fs.FileInfo) bool { return t.MinAge > 0 && now.Sub(fileTime(info)) < t.MinAge }
	visit := func(path string, info fs.FileInfo) {
		if isLink(path, info) || !info.Mode().IsRegular() {
			return
		}
		if !matches(t, info.Name()) {
			return
		}
		if recent(info) || recentDirs[filepath.Dir(path)] || t.Keep[strings.ToLower(filepath.Clean(path))] {
			kept++
			return
		}
		fn(path, info)
	}
	for _, f := range t.Files {
		if info, err := os.Lstat(f); err == nil {
			visit(f, info)
		}
	}
	for _, dir := range t.Dirs {
		if safeRoot(dir) != nil {
			continue
		}
		root, err := os.Lstat(dir)
		if err != nil || !root.IsDir() || isLink(dir, root) {
			continue
		}
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if !deadline.IsZero() && time.Now().After(deadline) {
				partial = true
				return filepath.SkipAll
			}
			if err != nil {
				return nil
			}
			info, ierr := d.Info()
			if ierr != nil {
				return nil
			}
			if isLink(p, info) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				if p != dir && !t.Recurse {
					return filepath.SkipDir
				}
				// The root itself changes all the time; below it, a folder
				// created or changed recently marks work in progress.
				if p != dir && (recent(info) || recentDirs[filepath.Dir(p)]) {
					recentDirs[p] = true
				}
				return nil
			}
			visit(p, info)
			return nil
		})
	}
	return kept, partial
}

// Scan measures a file target (within the time budget).
func Scan(t Target, now time.Time, budget time.Duration) Measure {
	m := Measure{ID: t.ID}
	for _, f := range t.Files {
		if _, err := os.Lstat(f); err == nil {
			m.Present = true
		}
	}
	for _, d := range t.Dirs {
		if _, err := os.Lstat(d); err == nil {
			m.Present = true
		}
	}
	var deadline time.Time
	if budget > 0 {
		deadline = time.Now().Add(budget)
	}
	kept, partial := walk(t, now, deadline, func(p string, info fs.FileInfo) bool {
		m.Bytes += info.Size()
		m.Files++
		if ft := fileTime(info); ft.After(m.Newest) {
			m.Newest = ft
		}
		return true
	})
	m.Kept, m.Partial = kept, partial
	return m
}

// Clean deletes a file target. Files that cannot be removed (in use) are
// counted as skipped; afterwards, sub-folders that were already older than
// the minimum age before the cleanup and ended up empty are removed (never
// the target folder itself). Every delete goes through a handle checked to
// be inside the target folder (see remover).
func Clean(t Target, now time.Time) Result {
	r := Result{ID: t.ID}
	rm := newRemover()
	var oldDirs []string
	if t.Recurse {
		for _, dir := range t.Dirs {
			oldDirs = append(oldDirs, oldSubdirs(dir, now, t.MinAge)...)
		}
	}
	rootFor := func(p string) string {
		for _, d := range t.Dirs {
			if rel, err := filepath.Rel(d, p); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
				return d
			}
		}
		return filepath.Dir(p) // an explicit file (MEMORY.DMP)
	}
	walk(t, now, time.Time{}, func(p string, info fs.FileInfo) bool {
		if err := rm.remove(rootFor(p), p); err != nil {
			r.Skipped++
			if len(r.Errors) < 5 && !errors.Is(err, fs.ErrNotExist) {
				r.Errors = append(r.Errors, filepath.Base(p)+": "+shortErr(err))
			}
			return false
		}
		r.Freed += info.Size()
		r.Deleted++
		return true
	})
	// Deepest first, so parents empty out before their turn.
	sort.Slice(oldDirs, func(i, j int) bool { return len(oldDirs[i]) > len(oldDirs[j]) })
	for _, d := range oldDirs {
		if empty(d) {
			_ = rm.remove(rootFor(d), d)
		}
	}
	return r
}

// empty reports whether a folder has no entries.
func empty(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	defer f.Close()
	names, _ := f.Readdirnames(1)
	return len(names) == 0
}

func shortErr(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	s := err.Error()
	if strings.Contains(strings.ToLower(s), "used by another process") || strings.Contains(s, "sendo usado") {
		return "em uso"
	}
	return s
}

// oldSubdirs lists the sub-folders of root last modified before minAge.
func oldSubdirs(root string, now time.Time, minAge time.Duration) []string {
	if safeRoot(root) != nil {
		return nil
	}
	if info, err := os.Lstat(root); err != nil || !info.IsDir() || isLink(root, info) {
		return nil
	}
	var dirs []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == root {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil || isLink(p, info) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() && now.Sub(fileTime(info)) >= minAge {
			dirs = append(dirs, p)
		}
		return nil
	})
	return dirs
}

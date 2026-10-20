// Package games finds installed games (Steam, Epic, Riot, Ubisoft, Battle.net,
// EA and Xbox) and their executables, so per-game profiles can target the
// right process, and reads Counter-Strike 2 settings files for the CS2 page.
package games

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"axion/internal/sys"
)

// FS abstracts file access so the scanner can be tested with fake trees.
type FS interface {
	ReadFile(path string) ([]byte, error)
	Stat(path string) (fs.FileInfo, error)
	ReadDir(path string) ([]fs.DirEntry, error)
}

// OSFS uses the real file system.
type OSFS struct{}

func (OSFS) ReadFile(p string) ([]byte, error)       { return os.ReadFile(p) }
func (OSFS) Stat(p string) (fs.FileInfo, error)      { return os.Stat(p) }
func (OSFS) ReadDir(p string) ([]fs.DirEntry, error) { return os.ReadDir(p) }

// Game is an installed game.
type Game struct {
	ID          string    `json:"id"` // launcher:appid
	Name        string    `json:"name"`
	Launcher    string    `json:"launcher"` // Steam, Epic, Riot, Ubisoft, Battle.net, EA, Xbox
	AppID       string    `json:"appId"`
	InstallDir  string    `json:"installDir"`
	Exe         string    `json:"exe"`       // full path of the game process ("" when unknown)
	ExeName     string    `json:"exeName"`   // cs2.exe
	ExeSource   string    `json:"exeSource"` // known | manifest | heuristic | manual
	Competitive bool      `json:"competitive"`
	AntiCheat   string    `json:"antiCheat,omitempty"`
	LastPlayed  time.Time `json:"lastPlayed,omitempty"`
	// SizeBytes and Updated come from the Steam manifest (SizeOnDisk,
	// LastUpdated); zero when the launcher does not say.
	SizeBytes uint64    `json:"sizeBytes,omitempty"`
	Updated   time.Time `json:"updated,omitempty"`
}

// Known maps launcher app IDs to the executable of the game process
// (relative to the install dir). The first existing candidate wins.
type knownGame struct {
	Exes        []string
	Competitive bool
	AntiCheat   string
}

var knownSteam = map[string]knownGame{
	"730":     {Exes: []string{`game\bin\win64\cs2.exe`}, Competitive: true, AntiCheat: "VAC"},
	"570":     {Exes: []string{`game\bin\win64\dota2.exe`}, Competitive: true, AntiCheat: "VAC"},
	"578080":  {Exes: []string{`TslGame\Binaries\Win64\TslGame.exe`}, Competitive: true, AntiCheat: "BattlEye/Wellbia"},
	"1172470": {Exes: []string{`r5apex_dx12.exe`, `r5apex.exe`}, Competitive: true, AntiCheat: "EasyAntiCheat"},
	"252490":  {Exes: []string{`RustClient.exe`}, Competitive: true, AntiCheat: "EasyAntiCheat"},
	"359550":  {Exes: []string{`RainbowSix.exe`, `RainbowSix_Vulkan.exe`}, Competitive: true, AntiCheat: "BattlEye"},
	"2357570": {Exes: []string{`Overwatch.exe`}, Competitive: true},
	"1938090": {Exes: []string{`cod.exe`}, Competitive: true, AntiCheat: "Ricochet"},
	"1085660": {Exes: []string{`destiny2.exe`}, Competitive: true, AntiCheat: "BattlEye"},
	"236390":  {Exes: []string{`win64\aces.exe`}, Competitive: true, AntiCheat: "EasyAntiCheat"},
	"440":     {Exes: []string{`tf_win64.exe`, `hl2.exe`}, Competitive: true, AntiCheat: "VAC"},
	"1245620": {Exes: []string{`Game\eldenring.exe`}},
	"271590":  {Exes: []string{`GTA5.exe`}},
	"3240220": {Exes: []string{`GTA5_Enhanced.exe`}},
	"1091500": {Exes: []string{`bin\x64\Cyberpunk2077.exe`}},
	"2767030": {Exes: []string{`MarvelGame\Marvel\Binaries\Win64\Marvel-Win64-Shipping.exe`}, Competitive: true},
	"2507950": {Exes: []string{`Delta Force\Binaries\Win64\DeltaForceClient-Win64-Shipping.exe`}, Competitive: true},
	"2073850": {Exes: []string{`Discovery.exe`}, Competitive: true, AntiCheat: "EasyAntiCheat"}, // THE FINALS
	"1966720": {Exes: []string{`Lethal Company.exe`}},
	"1086940": {Exes: []string{`bin\bg3.exe`, `bin\bg3_dx11.exe`}},
}

// Steam app IDs that are tools/runtimes, not games.
var steamTools = map[string]bool{"228980": true, "1070560": true, "1391110": true, "1628350": true, "250820": true, "1493710": true, "2180100": true}

// Scanner finds games.
type Scanner struct {
	Reg sys.Registry
	FS  FS
	Env func(string) string
}

// NewScanner builds a scanner over the real file system.
func NewScanner(reg sys.Registry) *Scanner {
	return &Scanner{Reg: reg, FS: OSFS{}, Env: os.Getenv}
}

func (s *Scanner) regStr(path, name string) string {
	if s.Reg == nil {
		return ""
	}
	v, err := s.Reg.GetValue(path, name)
	if err != nil || !v.Exists {
		return ""
	}
	return v.Str
}

func (s *Scanner) exists(p string) bool {
	if p == "" {
		return false
	}
	st, err := s.FS.Stat(p)
	return err == nil && !st.IsDir()
}

func (s *Scanner) dirExists(p string) bool {
	if p == "" {
		return false
	}
	st, err := s.FS.Stat(p)
	return err == nil && st.IsDir()
}

// winJoin joins Windows paths regardless of the host OS (tests run on Linux).
func winJoin(parts ...string) string {
	var out []string
	for i, p := range parts {
		p = strings.ReplaceAll(p, "/", `\`)
		if i > 0 {
			p = strings.Trim(p, `\`)
		} else {
			p = strings.TrimRight(p, `\`)
		}
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, `\`)
}

// Scan returns every game found, sorted by name.
func (s *Scanner) Scan() []Game {
	var all []Game
	all = append(all, s.steam()...)
	all = append(all, s.epic()...)
	all = append(all, s.riot()...)
	all = append(all, s.ubisoft()...)
	all = append(all, s.battlenet()...)
	all = append(all, s.ea()...)
	all = append(all, s.xbox()...)
	seen := map[string]bool{}
	var out []Game
	for _, g := range all {
		k := strings.ToLower(g.Exe)
		if k == "" {
			k = g.ID
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, g)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Competitive != out[j].Competitive {
			return out[i].Competitive
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// ---------------------------------------------------------------------------
// Steam
// ---------------------------------------------------------------------------

// SteamRoot returns the Steam installation folder.
func (s *Scanner) SteamRoot() string {
	for _, c := range []string{
		s.regStr(`HKCU\Software\Valve\Steam`, "SteamPath"),
		s.regStr(`HKLM\SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath"),
		s.regStr(`HKLM\SOFTWARE\Valve\Steam`, "InstallPath"),
	} {
		c = strings.ReplaceAll(strings.TrimSpace(c), "/", `\`)
		if c != "" && s.dirExists(c) {
			return c
		}
	}
	return ""
}

// SteamLibraries returns the library folders (including the Steam root).
func (s *Scanner) SteamLibraries() []string {
	root := s.SteamRoot()
	if root == "" {
		return nil
	}
	libs := []string{root}
	for _, p := range []string{winJoin(root, "steamapps", "libraryfolders.vdf"), winJoin(root, "config", "libraryfolders.vdf")} {
		b, err := s.FS.ReadFile(p)
		if err != nil {
			continue
		}
		v, err := ParseVDF(string(b))
		if err != nil {
			continue
		}
		lf := v.Sub("libraryfolders")
		if lf == nil {
			continue
		}
		keys := make([]string, 0, len(lf))
		for k := range lf {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch e := lf[k].(type) {
			case VDF:
				if p := e.Str("path"); p != "" {
					libs = append(libs, p)
				}
			case string: // old format: "1" "D:\\SteamLibrary"
				if strings.Contains(e, `\`) || strings.Contains(e, ":") {
					libs = append(libs, e)
				}
			}
		}
		break
	}
	seen := map[string]bool{}
	var out []string
	for _, l := range libs {
		l = strings.ReplaceAll(l, "/", `\`)
		if !seen[strings.ToLower(l)] {
			seen[strings.ToLower(l)] = true
			out = append(out, l)
		}
	}
	return out
}

var reManifest = regexp.MustCompile(`(?i)^appmanifest_(\d+)\.acf$`)

func (s *Scanner) steam() []Game {
	var out []Game
	for _, lib := range s.SteamLibraries() {
		apps := winJoin(lib, "steamapps")
		entries, err := s.FS.ReadDir(apps)
		if err != nil {
			continue
		}
		for _, e := range entries {
			m := reManifest.FindStringSubmatch(e.Name())
			if m == nil || steamTools[m[1]] {
				continue
			}
			b, err := s.FS.ReadFile(winJoin(apps, e.Name()))
			if err != nil {
				continue
			}
			v, err := ParseVDF(string(b))
			if err != nil {
				continue
			}
			st := v.Sub("AppState")
			if st == nil || st.Str("installdir") == "" {
				continue
			}
			name := st.Str("name")
			if strings.Contains(strings.ToLower(name), "redistributable") || strings.HasPrefix(strings.ToLower(name), "proton") || strings.HasPrefix(strings.ToLower(name), "steamworks") {
				continue
			}
			dir := winJoin(apps, "common", st.Str("installdir"))
			g := Game{ID: "steam:" + m[1], Name: name, Launcher: "Steam", AppID: m[1], InstallDir: dir}
			if lp := st.Str("LastPlayed"); lp != "" {
				if n := atoi64(lp); n > 0 {
					g.LastPlayed = time.Unix(n, 0)
				}
			}
			if n := atoi64(st.Str("LastUpdated")); n > 0 {
				g.Updated = time.Unix(n, 0)
			}
			if n := atoi64(st.Str("SizeOnDisk")); n > 0 {
				g.SizeBytes = uint64(n)
			}
			if k, ok := knownSteam[m[1]]; ok {
				g.Competitive, g.AntiCheat = k.Competitive, k.AntiCheat
				for _, rel := range k.Exes {
					if p := winJoin(dir, rel); s.exists(p) {
						g.Exe, g.ExeSource = p, "known"
						break
					}
				}
			}
			if g.Exe == "" {
				if p := s.guessExe(dir); p != "" {
					g.Exe, g.ExeSource = p, "heuristic"
				}
			}
			if g.Exe != "" {
				g.ExeName = baseName(g.Exe)
			}
			if !s.dirExists(dir) {
				continue
			}
			out = append(out, g)
		}
	}
	return out
}

func atoi64(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

func baseName(p string) string {
	p = strings.ReplaceAll(p, "/", `\`)
	if i := strings.LastIndex(p, `\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

var badExe = regexp.MustCompile(`(?i)(unins|uninstall|setup|install|redist|vc_?redist|dxsetup|crash|report|helper|launcher|easyanticheat|beservice|battleye|update|patch|config|settings|benchmark|editor|server|dotnet|ue4prereq|unitycrashhandler|cefprocess|webhelper|overlay)`)

// alnum lower-cases s and keeps only letters and digits.
func alnum(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// guessExe picks the most likely game executable (depth <= 4): the exe named
// like the game folder, the one next to UnityPlayer.dll (Unity games ship a
// small stub exe), a Shipping/Win64 build, else the largest one whose name is
// not a helper. Launcher and installer folders are skipped. A helper-looking
// word that is part of the game's own name ("Crash Bandicoot 4") does not
// exclude it.
func (s *Scanner) guessExe(dir string) string {
	type cand struct {
		path  string
		size  int64
		score int
		unity bool
	}
	gameName := alnum(baseName(strings.TrimRight(dir, `\/`)))
	var best *cand
	var walk func(d string, depth int)
	walk = func(d string, depth int) {
		if depth > 4 {
			return
		}
		entries, err := s.FS.ReadDir(d)
		if err != nil {
			return
		}
		unity := false
		for _, e := range entries {
			if !e.IsDir() && strings.EqualFold(e.Name(), "UnityPlayer.dll") {
				unity = true
			}
		}
		for _, e := range entries {
			p := winJoin(d, e.Name())
			if e.IsDir() {
				switch low := strings.ToLower(e.Name()); low {
				case "_commonredist", "redist", "directx", "support", "launcher", "installer", "installers", "prereq", "prerequisites", "_installer", "tools":
					continue
				case "engine":
					if depth > 0 {
						continue
					}
				}
				walk(p, depth+1)
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(strings.ToLower(name), ".exe") {
				continue
			}
			stem := alnum(strings.TrimSuffix(name, name[len(name)-4:]))
			ownName := gameName != "" && len(stem) >= 4 && (strings.HasPrefix(gameName, stem) || strings.HasPrefix(stem, gameName))
			if badExe.MatchString(name) && !ownName {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			sc := 0
			low := strings.ToLower(p)
			if strings.Contains(low, "shipping") {
				sc += 2
			}
			if strings.Contains(low, `win64`) || strings.Contains(low, `x64`) {
				sc++
			}
			if ownName {
				sc += 3
			}
			if unity && !strings.Contains(strings.ToLower(name), "crashhandler") {
				sc += 3
			}
			c := cand{path: p, size: info.Size(), score: sc, unity: unity}
			if best == nil || c.score > best.score || (c.score == best.score && c.size > best.size) {
				best = &c
			}
		}
	}
	walk(dir, 0)
	if best == nil || (best.size < 1<<20 && !best.unity && best.score < 3) {
		return ""
	}
	return best.path
}

// ---------------------------------------------------------------------------
// Epic Games
// ---------------------------------------------------------------------------

var knownEpic = map[string]knownGame{
	"fortnite": {Exes: []string{`FortniteGame\Binaries\Win64\FortniteClient-Win64-Shipping.exe`}, Competitive: true, AntiCheat: "EasyAntiCheat/BattlEye"},
	"sugar":    {Exes: []string{`TAGame\Binaries\Win64\RocketLeague.exe`}, Competitive: true, AntiCheat: "EasyAntiCheat"}, // Rocket League
	"valorant": {Exes: []string{`ShooterGame\Binaries\Win64\VALORANT-Win64-Shipping.exe`}, Competitive: true, AntiCheat: "Vanguard"},
}

type epicItem struct {
	DisplayName      string `json:"DisplayName"`
	InstallLocation  string `json:"InstallLocation"`
	LaunchExecutable string `json:"LaunchExecutable"`
	AppName          string `json:"AppName"`
	MainGameAppName  string `json:"MainGameAppName"`
	IsApplication    *bool  `json:"bIsApplication"`
	IsExecutable     *bool  `json:"bIsExecutable"`
}

func (s *Scanner) epic() []Game {
	pd := s.Env("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	dir := winJoin(pd, "Epic", "EpicGamesLauncher", "Data", "Manifests")
	entries, err := s.FS.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Game
	for _, e := range entries {
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".item") {
			continue
		}
		b, err := s.FS.ReadFile(winJoin(dir, e.Name()))
		if err != nil {
			continue
		}
		var it epicItem
		if json.Unmarshal(b, &it) != nil || it.InstallLocation == "" {
			continue
		}
		if it.IsApplication != nil && !*it.IsApplication {
			continue
		}
		if it.MainGameAppName != "" && !strings.EqualFold(it.MainGameAppName, it.AppName) {
			continue // DLC
		}
		g := Game{ID: "epic:" + strings.ToLower(it.AppName), Name: it.DisplayName, Launcher: "Epic", AppID: it.AppName, InstallDir: strings.ReplaceAll(it.InstallLocation, "/", `\`)}
		if k, ok := knownEpic[strings.ToLower(it.AppName)]; ok {
			g.Competitive, g.AntiCheat = k.Competitive, k.AntiCheat
			for _, rel := range k.Exes {
				if p := winJoin(g.InstallDir, rel); s.exists(p) {
					g.Exe, g.ExeSource = p, "known"
					break
				}
			}
		}
		if g.Exe == "" && it.LaunchExecutable != "" {
			if p := winJoin(g.InstallDir, it.LaunchExecutable); s.exists(p) {
				g.Exe, g.ExeSource = p, "manifest"
			}
		}
		if g.Exe != "" {
			g.ExeName = baseName(g.Exe)
		}
		if s.dirExists(g.InstallDir) {
			out = append(out, g)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Riot Games
// ---------------------------------------------------------------------------

var riotProducts = []struct {
	Meta, Name, Exe, AntiCheat string
}{
	{"valorant.live", "VALORANT", `ShooterGame\Binaries\Win64\VALORANT-Win64-Shipping.exe`, "Vanguard"},
	{"league_of_legends.live", "League of Legends", `Game\League of Legends.exe`, "Vanguard"},
}

var reInstallPath = regexp.MustCompile(`(?m)^\s*product_install_full_path:\s*"?([^"\r\n]+)"?`)

func (s *Scanner) riot() []Game {
	pd := s.Env("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	var out []Game
	for _, p := range riotProducts {
		f := winJoin(pd, "Riot Games", "Metadata", p.Meta, p.Meta+".product_settings.yaml")
		b, err := s.FS.ReadFile(f)
		if err != nil {
			continue
		}
		m := reInstallPath.FindStringSubmatch(string(b))
		if m == nil {
			continue
		}
		dir := strings.ReplaceAll(strings.TrimSpace(m[1]), "/", `\`)
		g := Game{ID: "riot:" + p.Meta, Name: p.Name, Launcher: "Riot", AppID: p.Meta, InstallDir: dir, Competitive: true, AntiCheat: p.AntiCheat}
		if exe := winJoin(dir, p.Exe); s.exists(exe) {
			g.Exe, g.ExeName, g.ExeSource = exe, baseName(exe), "known"
		}
		out = append(out, g)
	}
	return out
}

// ---------------------------------------------------------------------------
// Ubisoft Connect
// ---------------------------------------------------------------------------

func (s *Scanner) ubisoft() []Game {
	if s.Reg == nil {
		return nil
	}
	base := `HKLM\SOFTWARE\WOW6432Node\Ubisoft\Launcher\Installs`
	ids, err := s.Reg.SubKeys(base)
	if err != nil {
		return nil
	}
	var out []Game
	for _, id := range ids {
		dir := strings.ReplaceAll(s.regStr(base+`\`+id, "InstallDir"), "/", `\`)
		if dir == "" || !s.dirExists(dir) {
			continue
		}
		name := baseName(strings.TrimRight(dir, `\`))
		g := Game{ID: "ubisoft:" + id, Name: name, Launcher: "Ubisoft", AppID: id, InstallDir: dir}
		if strings.Contains(strings.ToLower(name), "rainbow six") {
			g.Competitive, g.AntiCheat = true, "BattlEye"
			if p := winJoin(dir, "RainbowSix.exe"); s.exists(p) {
				g.Exe, g.ExeSource = p, "known"
			}
		}
		if g.Exe == "" {
			if p := s.guessExe(dir); p != "" {
				g.Exe, g.ExeSource = p, "heuristic"
			}
		}
		if g.Exe != "" {
			g.ExeName = baseName(g.Exe)
		}
		out = append(out, g)
	}
	return out
}

// Manual builds a game entry for an executable chosen by the user.
func Manual(exe string) Game {
	exe = filepath.Clean(exe)
	name := strings.TrimSuffix(baseName(exe), filepath.Ext(exe))
	return Game{ID: "manual:" + strings.ToLower(baseName(exe)), Name: name, Launcher: "Manual", InstallDir: strings.TrimSuffix(exe, `\`+baseName(exe)), Exe: exe, ExeName: baseName(exe), ExeSource: "manual"}
}

package games

// Read-only readers for Battle.net, the EA app (older EA Games key) and the
// Xbox app / Microsoft Store (GDK games). Every one only reads the registry
// and files; nothing is written and no launcher is started.

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Battle.net
// ---------------------------------------------------------------------------

// The Battle.net app registers each game it installs in the usual Uninstall
// keys, with a command line that carries "--uid=<id>" (the same detection
// the Playnite Battle.net library uses). The folder is InstallLocation.
// product.db (C:\ProgramData\Battle.net\Agent) is a protobuf with no
// documented schema, so it is not read.
var uninstallRoots = []string{
	`HKLM\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`,
	`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`,
	`HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`,
}

type bnetGame struct {
	Name        string
	Exes        []string
	Competitive bool
	AntiCheat   string
}

// bnetGames maps the Battle.net product uid (lower case) to the game. Names
// follow the Battle.net product list; the executable is only used when the
// file exists, otherwise the folder is searched.
var bnetGames = map[string]bnetGame{
	"wow":        {Name: "World of Warcraft", Exes: []string{`_retail_\Wow.exe`, `Wow.exe`}},
	"diablo3":    {Name: "Diablo III", Exes: []string{`Diablo III64.exe`}},
	"s2":         {Name: "StarCraft II"},
	"s1":         {Name: "StarCraft"},
	"hs_beta":    {Name: "Hearthstone", Exes: []string{`Hearthstone.exe`}},
	"heroes":     {Name: "Heroes of the Storm"},
	"prometheus": {Name: "Overwatch 2", Exes: []string{`_retail_\Overwatch.exe`, `Overwatch.exe`}, Competitive: true},
	"viper":      {Name: "Call of Duty: Black Ops 4", Competitive: true},
	"odin":       {Name: "Call of Duty: Modern Warfare", Competitive: true},
	"zeus":       {Name: "Call of Duty: Black Ops Cold War", Competitive: true},
	"fore":       {Name: "Call of Duty: Vanguard", Competitive: true},
	"auks":       {Name: "Call of Duty: Modern Warfare II", Exes: []string{`cod.exe`}, Competitive: true, AntiCheat: "Ricochet"},
	"pinta":      {Name: "Call of Duty: Modern Warfare III", Exes: []string{`cod.exe`}, Competitive: true, AntiCheat: "Ricochet"},
	"w3":         {Name: "Warcraft III: Reforged"},
	"osi":        {Name: "Diablo II: Resurrected", Exes: []string{`D2R.exe`}},
	"fen":        {Name: "Diablo IV", Exes: []string{`Diablo IV.exe`}},
	"anbs":       {Name: "Diablo Immortal"},
	"wlby":       {Name: "Crash Bandicoot 4"},
	"gryphon":    {Name: "Warcraft Rumble"},
}

var reBnetUID = regexp.MustCompile(`(?i)--uid=([^\s"]+)`)

func (s *Scanner) battlenet() []Game {
	if s.Reg == nil {
		return nil
	}
	var out []Game
	for _, root := range uninstallRoots {
		subs, err := s.Reg.SubKeys(root)
		if err != nil {
			continue
		}
		sort.Strings(subs)
		for _, sub := range subs {
			key := root + `\` + sub
			us := s.regStr(key, "UninstallString")
			if !strings.Contains(strings.ToLower(us), "battle.net") {
				continue
			}
			m := reBnetUID.FindStringSubmatch(us)
			if m == nil {
				continue
			}
			uid := strings.ToLower(strings.Trim(m[1], `"'`))
			if uid == "battle.net" || uid == "agent" {
				continue // the client itself, not a game
			}
			dir := strings.ReplaceAll(strings.TrimSpace(s.regStr(key, "InstallLocation")), "/", `\`)
			dir = strings.TrimRight(dir, `\`)
			if dir == "" || !s.dirExists(dir) {
				continue
			}
			k, known := bnetGames[uid]
			name := k.Name
			if name == "" {
				name = strings.TrimSpace(s.regStr(key, "DisplayName"))
			}
			if name == "" {
				name = baseName(dir)
			}
			g := Game{ID: "battlenet:" + uid, Name: name, Launcher: "Battle.net", AppID: uid, InstallDir: dir}
			if known {
				g.Competitive, g.AntiCheat = k.Competitive, k.AntiCheat
				for _, rel := range k.Exes {
					if p := winJoin(dir, rel); s.exists(p) {
						g.Exe, g.ExeSource = p, "known"
						break
					}
				}
			}
			s.finishExe(&g)
			out = append(out, g)
		}
	}
	return out
}

// finishExe falls back to the folder search when no known executable was
// found, and fills ExeName.
func (s *Scanner) finishExe(g *Game) {
	if g.Exe == "" {
		if p := s.guessExe(g.InstallDir); p != "" {
			g.Exe, g.ExeSource = p, "heuristic"
		}
	}
	if g.Exe != "" {
		g.ExeName = baseName(g.Exe)
	}
}

// ---------------------------------------------------------------------------
// EA app (games registered under the "EA Games" key)
// ---------------------------------------------------------------------------

// EA games installed through Origin/EA app register their folder in
// HKLM\SOFTWARE\WOW6432Node\EA Games\<game>, value "Install Dir". Games that
// do not write this key are not found: add them with "Adicionar jogo".
const eaGamesKey = `HKLM\SOFTWARE\WOW6432Node\EA Games`

func (s *Scanner) ea() []Game {
	if s.Reg == nil {
		return nil
	}
	subs, err := s.Reg.SubKeys(eaGamesKey)
	if err != nil {
		return nil
	}
	sort.Strings(subs)
	var out []Game
	for _, sub := range subs {
		dir := strings.ReplaceAll(strings.TrimSpace(s.regStr(eaGamesKey+`\`+sub, "Install Dir")), "/", `\`)
		dir = strings.TrimRight(dir, `\`)
		if dir == "" || !s.dirExists(dir) {
			continue
		}
		g := Game{ID: "ea:" + alnum(sub), Name: sub, Launcher: "EA", AppID: sub, InstallDir: dir}
		s.finishExe(&g)
		out = append(out, g)
	}
	return out
}

// ---------------------------------------------------------------------------
// Xbox app / Microsoft Store (GDK games)
// ---------------------------------------------------------------------------

// Packages registered for the user are listed under this key; each has the
// value PackageRootFolder. GDK games installed by the Xbox app live in
// C:\XboxGames (or another drive's XboxGames folder), which Microsoft says
// needs no elevated permission to read, and carry a MicrosoftGame.config
// with the executable list. Games kept in C:\Program Files\WindowsApps are
// not readable without special permission and are left out.
const appModelPackages = `HKCU\Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages`

type msGameConfig struct {
	Executables []struct {
		Name      string `xml:"Name,attr"`
		IsDevOnly string `xml:"IsDevOnly,attr"`
	} `xml:"ExecutableList>Executable"`
	Visuals struct {
		Display string `xml:"DefaultDisplayName,attr"`
	} `xml:"ShellVisuals"`
}

func parseMSGameConfig(b []byte) (msGameConfig, bool) {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	d := xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	var c msGameConfig
	if err := d.Decode(&c); err != nil || len(c.Executables) == 0 {
		return c, false
	}
	return c, true
}

func (s *Scanner) xbox() []Game {
	if s.Reg == nil {
		return nil
	}
	pkgs, err := s.Reg.SubKeys(appModelPackages)
	if err != nil {
		return nil
	}
	sort.Strings(pkgs)
	var out []Game
	for _, pkg := range pkgs {
		root := strings.ReplaceAll(strings.TrimSpace(s.regStr(appModelPackages+`\`+pkg, "PackageRootFolder")), "/", `\`)
		root = strings.TrimRight(root, `\`)
		low := strings.ToLower(root)
		if root == "" || strings.Contains(low, `\windowsapps`) || strings.HasPrefix(low, `c:\windows\`) {
			continue
		}
		var cfg msGameConfig
		dir, found := "", false
		for _, d := range []string{root, winJoin(root, "Content")} {
			b, err := s.FS.ReadFile(winJoin(d, "MicrosoftGame.config"))
			if err != nil {
				continue
			}
			if c, ok := parseMSGameConfig(b); ok {
				cfg, dir, found = c, d, true
				break
			}
		}
		if !found {
			continue
		}
		fam := pkg
		if i := strings.Index(pkg, "_"); i > 0 {
			fam = pkg[:i]
		}
		name := strings.TrimSpace(cfg.Visuals.Display)
		if name == "" || strings.HasPrefix(strings.ToLower(name), "ms-resource:") {
			folder := dir
			if strings.EqualFold(baseName(folder), "Content") {
				folder = strings.TrimSuffix(folder, `\`+baseName(folder))
			}
			name = baseName(folder)
		}
		g := Game{ID: "xbox:" + strings.ToLower(fam), Name: name, Launcher: "Xbox", AppID: fam, InstallDir: dir}
		for _, e := range cfg.Executables {
			if strings.EqualFold(strings.TrimSpace(e.IsDevOnly), "true") || strings.TrimSpace(e.Name) == "" {
				continue
			}
			if p := winJoin(dir, e.Name); s.exists(p) {
				g.Exe, g.ExeSource = p, "manifest"
				break
			}
		}
		s.finishExe(&g)
		out = append(out, g)
	}
	return out
}

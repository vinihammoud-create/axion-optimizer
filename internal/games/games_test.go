package games

import (
	"io/fs"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

// memFS is a case-insensitive in-memory tree keyed by Windows paths.
type memFS struct {
	files map[string][]byte
	dirs  map[string]bool
	names map[string]string // normalized path -> original base name
}

func newMemFS() *memFS {
	return &memFS{files: map[string][]byte{}, dirs: map[string]bool{}, names: map[string]string{}}
}

func norm(p string) string {
	return strings.ToLower(strings.TrimRight(strings.ReplaceAll(p, "/", `\`), `\`))
}

func (m *memFS) add(path string, content string) {
	orig := strings.TrimRight(strings.ReplaceAll(path, "/", `\`), `\`)
	m.files[norm(path)] = []byte(content)
	for o := orig; o != ""; o = parentOf(o) {
		m.names[norm(o)] = baseName(o)
		if norm(o) != norm(path) {
			m.dirs[norm(o)] = true
		}
	}
}

func (m *memFS) addSized(path string, size int) { m.add(path, strings.Repeat("x", size)) }

func parentOf(p string) string {
	i := strings.LastIndex(p, `\`)
	if i < 0 {
		return ""
	}
	return p[:i]
}

type fi struct {
	name string
	size int64
	dir  bool
}

func (f fi) Name() string       { return f.name }
func (f fi) Size() int64        { return f.size }
func (f fi) Mode() fs.FileMode  { return 0o644 }
func (f fi) ModTime() time.Time { return time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC) }
func (f fi) IsDir() bool        { return f.dir }
func (f fi) Sys() any           { return nil }

type de struct{ fi }

func (d de) Type() fs.FileMode          { return 0 }
func (d de) Info() (fs.FileInfo, error) { return d.fi, nil }

func (m *memFS) ReadFile(p string) ([]byte, error) {
	if b, ok := m.files[norm(p)]; ok {
		return b, nil
	}
	return nil, os.ErrNotExist
}

func (m *memFS) Stat(p string) (fs.FileInfo, error) {
	n := norm(p)
	if b, ok := m.files[n]; ok {
		return fi{name: m.names[n], size: int64(len(b))}, nil
	}
	if m.dirs[n] {
		return fi{name: m.names[n], dir: true}, nil
	}
	return nil, os.ErrNotExist
}

func (m *memFS) ReadDir(p string) ([]fs.DirEntry, error) {
	n := norm(p)
	if !m.dirs[n] {
		return nil, os.ErrNotExist
	}
	seen := map[string]bool{}
	var out []fs.DirEntry
	for f, b := range m.files {
		if parentOf(f) == n && !seen[f] {
			seen[f] = true
			out = append(out, de{fi{name: m.names[f], size: int64(len(b))}})
		}
	}
	for d := range m.dirs {
		if parentOf(d) == n && !seen[d] {
			seen[d] = true
			out = append(out, de{fi{name: m.names[d], dir: true}})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

const libraryVDF = `"libraryfolders"
{
	"0"
	{
		"path"		"C:\\Program Files (x86)\\Steam"
		"label"		""
		"apps"
		{
			"228980"		"417232262"
		}
	}
	"1"
	{
		"path"		"D:\\SteamLibrary"
		"apps"
		{
			"730"		"36015398562"
		}
	}
}`

const cs2Manifest = `"AppState"
{
	"appid"		"730"
	"name"		"Counter-Strike 2"
	"installdir"		"Counter-Strike Global Offensive"
	"LastUpdated"		"1758700000"
	"SizeOnDisk"		"36015398562"
	"LastPlayed"		"1758800000"
}`

const cs2Video = `"video.cfg"
{
	"Version"		"15"
	"setting.defaultres"		"1280"
	"setting.defaultresheight"		"960"
	"setting.refreshrate_numerator"		"60"
	"setting.refreshrate_denominator"		"1"
	"setting.fullscreen"		"1"
	"setting.nowindowborder"		"1"
	"setting.mat_vsync"		"0"
	"setting.r_low_latency"		"1"
}`

const localConfig = `"UserLocalConfigStore"
{
	"Software"
	{
		"Valve"
		{
			"Steam"
			{
				"apps"
				{
					"730"
					{
						"LastPlayed"		"1758800000"
						"LaunchOptions"		"-novid -threads 8 +fps_max 0 -allow_third_party_software \"+exec autoexec\""
					}
				}
			}
		}
	}
}`

func fixture(t *testing.T) *Scanner {
	t.Helper()
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = m.R.SetValue(`HKCU\Software\Valve\Steam`, "SteamPath", sys.SZ("c:/program files (x86)/steam"))
	_ = m.R.SetValue(`HKCU\Software\Valve\Steam\ActiveProcess`, "ActiveUser", sys.DWord(123))
	_ = m.R.SetValue(`HKLM\SOFTWARE\WOW6432Node\Ubisoft\Launcher\Installs\635`, "InstallDir", sys.SZ("E:/Ubisoft/Tom Clancy's Rainbow Six Siege/"))
	f := newMemFS()
	steam := `C:\Program Files (x86)\Steam`
	f.add(steam+`\steam.exe`, "x")
	f.add(steam+`\steamapps\libraryfolders.vdf`, libraryVDF)
	f.add(steam+`\steamapps\appmanifest_228980.acf`, `"AppState" { "appid" "228980" "name" "Steamworks Common Redistributables" "installdir" "Steamworks Shared" }`)
	f.add(`D:\SteamLibrary\steamapps\appmanifest_730.acf`, cs2Manifest)
	f.addSized(`D:\SteamLibrary\steamapps\common\Counter-Strike Global Offensive\game\bin\win64\cs2.exe`, 10)
	f.add(`D:\SteamLibrary\steamapps\appmanifest_999.acf`, `"AppState" { "appid" "999" "name" "Some Indie" "installdir" "SomeIndie" }`)
	f.addSized(`D:\SteamLibrary\steamapps\common\SomeIndie\UnityCrashHandler64.exe`, 3<<20)
	f.addSized(`D:\SteamLibrary\steamapps\common\SomeIndie\SomeIndie.exe`, 2<<20)
	f.add(steam+`\userdata\123\730\local\cfg\cs2_video.txt`, cs2Video)
	f.add(steam+`\userdata\123\730\local\cfg\cs2_user_convars_0_slot0.vcfg`, "\"config\" { \"convars\" { \"cl_net_buffer_ticks\" \"1\" } }")
	f.add(steam+`\userdata\123\730\local\cfg\cs2_machine_convars.vcfg`, "\"config\" { \"convars\" { \"fps_max\" \"400\" \"cl_net_buffer_ticks\" \"2\" } }")
	f.add(steam+`\userdata\123\config\localconfig.vdf`, localConfig)
	f.add(`C:\ProgramData\Epic\EpicGamesLauncher\Data\Manifests\ABC.item`, `{"DisplayName":"Fortnite","InstallLocation":"C:\\Program Files\\Epic Games\\Fortnite","LaunchExecutable":"FortniteGame/Binaries/Win64/FortniteLauncher.exe","AppName":"Fortnite","MainGameAppName":"Fortnite","bIsApplication":true}`)
	f.addSized(`C:\Program Files\Epic Games\Fortnite\FortniteGame\Binaries\Win64\FortniteClient-Win64-Shipping.exe`, 10)
	f.add(`C:\ProgramData\Riot Games\Metadata\valorant.live\valorant.live.product_settings.yaml`, "product_install_full_path: \"C:/Riot Games/VALORANT/live\"\nproduct_install_root: \"C:/Riot Games\"\n")
	f.addSized(`C:\Riot Games\VALORANT\live\ShooterGame\Binaries\Win64\VALORANT-Win64-Shipping.exe`, 10)
	f.addSized(`E:\Ubisoft\Tom Clancy's Rainbow Six Siege\RainbowSix.exe`, 10)
	return &Scanner{Reg: m.R, FS: f, Env: func(k string) string {
		if k == "ProgramData" {
			return `C:\ProgramData`
		}
		return ""
	}}
}

func TestScanGames(t *testing.T) {
	s := fixture(t)
	gs := s.Scan()
	byID := map[string]Game{}
	for _, g := range gs {
		byID[g.ID] = g
	}
	cs2, ok := byID["steam:730"]
	if !ok || cs2.ExeName != "cs2.exe" || cs2.ExeSource != "known" || !cs2.Competitive || !strings.HasPrefix(cs2.Exe, `D:\SteamLibrary`) {
		t.Fatalf("CS2 não detectado corretamente: %+v", cs2)
	}
	if g := byID["steam:999"]; g.ExeName != "SomeIndie.exe" || g.ExeSource != "heuristic" {
		t.Errorf("heurística escolheu errado: %+v", g)
	}
	// Size and last update come from the manifest; a manifest without them
	// leaves them empty (nothing is guessed from the folder).
	if cs2.SizeBytes != 36015398562 || cs2.Updated.Unix() != 1758700000 || cs2.LastPlayed.Unix() != 1758800000 {
		t.Errorf("tamanho e datas do manifesto: %+v", cs2)
	}
	if g := byID["steam:999"]; g.SizeBytes != 0 || !g.Updated.IsZero() || !g.LastPlayed.IsZero() {
		t.Errorf("sem dados no manifesto: %+v", g)
	}
	if _, ok := byID["steam:228980"]; ok {
		t.Errorf("redistribuível listado como jogo")
	}
	if g := byID["epic:fortnite"]; g.ExeName != "FortniteClient-Win64-Shipping.exe" {
		t.Errorf("Fortnite: %+v", g)
	}
	if g := byID["riot:valorant.live"]; g.ExeName != "VALORANT-Win64-Shipping.exe" || g.AntiCheat != "Vanguard" {
		t.Errorf("VALORANT: %+v", g)
	}
	if g := byID["ubisoft:635"]; g.ExeName != "RainbowSix.exe" || g.Name != "Tom Clancy's Rainbow Six Siege" {
		t.Errorf("R6: %+v", g)
	}
	if !gs[0].Competitive {
		t.Errorf("jogos competitivos deveriam vir primeiro")
	}
}

func TestCS2Info(t *testing.T) {
	s := fixture(t)
	info := s.CS2(s.Scan())
	if info.ConvarInt("cl_net_buffer_ticks", -1) != 1 || info.ConvarInt("fps_max", -1) != 400 || len(info.ConvarFiles) != 2 {
		t.Errorf("convars do CS2 (o arquivo do usuário vale primeiro): %v %v", info.Convars, info.ConvarFiles)
	}
	if !info.Installed || info.Video == nil {
		t.Fatalf("CS2 sem vídeo: %+v", info)
	}
	v := info.Video
	if v.RefreshHz != 60 || v.Width != 1280 || v.Height != 960 || v.LowLatency != 1 || v.VSync != 0 || !v.Fullscreen {
		t.Errorf("cs2_video.txt mal interpretado: %+v", v)
	}
	if !strings.Contains(info.LaunchOptions, "-threads 8") || !strings.Contains(info.LaunchOptions, `"+exec autoexec"`) {
		t.Errorf("opções de inicialização: %q", info.LaunchOptions)
	}
	levels := map[string]string{}
	for _, n := range info.LaunchNotes {
		levels[n.Option] = n.Level
	}
	if levels["-threads"] != "warning" || levels["-allow_third_party_software"] != "warning" || levels["-novid"] != "info" {
		t.Errorf("notas: %+v", info.LaunchNotes)
	}
}

func TestVDFEdgeCases(t *testing.T) {
	v, err := ParseVDF("// comment\n\"a\" { \"b\" \"x\\\\y\" \"c\" [$WIN32] { \"d\" \"1\" } }")
	if err != nil {
		t.Fatal(err)
	}
	if v.Path("a").Str("b") != `x\y` || v.Path("a", "c").Str("d") != "1" {
		t.Fatalf("vdf: %+v", v)
	}
	if _, err := ParseVDF(`"a" { "b" `); err == nil {
		t.Fatal("vdf truncado deveria falhar")
	}
}

// Real cases from a report: the game named like a helper word, a Unity game
// with a small stub exe, and a launcher bigger than the game.
func TestGuessExeRealCases(t *testing.T) {
	m := newMemFS()
	m.addSized(`D:\SteamLibrary\steamapps\common\Crash Bandicoot 4\CrashBandicoot4.exe`, 2<<20)
	m.addSized(`D:\SteamLibrary\steamapps\common\Crash Bandicoot 4\UnityCrashHandler64.exe`, 1<<20)
	m.addSized(`D:\SteamLibrary\steamapps\common\Green Hell\GH.exe`, 650<<10)
	m.addSized(`D:\SteamLibrary\steamapps\common\Green Hell\UnityPlayer.dll`, 30<<20)
	m.addSized(`D:\SteamLibrary\steamapps\common\Green Hell\UnityCrashHandler64.exe`, 1<<20)
	m.addSized(`D:\SteamLibrary\steamapps\common\Cities Skylines II\Cities2.exe`, 3<<20)
	m.addSized(`D:\SteamLibrary\steamapps\common\Cities Skylines II\Launcher\dowser.exe`, 90<<20)
	s := &Scanner{FS: m}
	for dir, want := range map[string]string{
		`D:\SteamLibrary\steamapps\common\Crash Bandicoot 4`:  "CrashBandicoot4.exe",
		`D:\SteamLibrary\steamapps\common\Green Hell`:         "GH.exe",
		`D:\SteamLibrary\steamapps\common\Cities Skylines II`: "Cities2.exe",
	} {
		if got := baseName(s.guessExe(dir)); !strings.EqualFold(got, want) {
			t.Errorf("%s: %q, esperado %q", dir, got, want)
		}
	}
}

func TestParseConvars(t *testing.T) {
	src := "\"config\"\n{\n\t\"convars\"\n\t{\n\t\t\"cl_net_buffer_ticks\"\t\t\"2\"\n\t\t\"rate\"\t\t\"196608\"\n\t\t\"sensitivity\"\t\t\"1.25\"\n\t}\n}\n"
	cv, ok := ParseConvars(src)
	if !ok || cv["cl_net_buffer_ticks"] != "2" || cv["rate"] != "196608" {
		t.Fatalf("%v %v", ok, cv)
	}
	info := CS2Info{Convars: cv}
	if info.ConvarInt("CL_NET_BUFFER_TICKS", -1) != 2 || info.ConvarInt("sensitivity", -1) != 1 || info.ConvarInt("ausente", -1) != -1 {
		t.Errorf("ConvarInt: %v", info.Convars)
	}
	if _, ok := ParseConvars(`"outro" { "x" "1" }`); ok {
		t.Error("sem bloco config/convars")
	}
}

package games

import (
	"strings"
	"testing"

	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

const msGameConfigSample = "\xef\xbb\xbf" + `<?xml version="1.0" encoding="utf-8"?>
<Game configVersion="1">
  <Identity Name="Contoso.ExampleGame" Publisher="CN=Contoso" Version="1.0.0.0"/>
  <ExecutableList>
    <Executable Name="ExampleGame_Debug.exe" IsDevOnly="true"/>
    <Executable Name="Game\ExampleGame.exe" TargetDeviceFamily="PC" Id="Game" IsDevOnly="false"/>
  </ExecutableList>
  <ShellVisuals DefaultDisplayName="Example Game" PublisherDisplayName="Contoso"/>
</Game>`

func launcherFixture(t *testing.T) (*Scanner, *mocksys.Mock) {
	t.Helper()
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := newMemFS()
	un := `HKLM\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\`

	// Battle.net: Overwatch 2 (known exe), an unlisted game (folder search),
	// the client itself, a non-Battle.net program and a stale entry.
	_ = m.R.SetValue(un+`Overwatch`, "UninstallString", sys.SZ(`"C:\Program Files (x86)\Battle.net\Battle.net.exe" --uid=prometheus --lang=ptBR --action=uninstall`))
	_ = m.R.SetValue(un+`Overwatch`, "InstallLocation", sys.SZ(`D:\Games\Overwatch\`))
	_ = m.R.SetValue(un+`Overwatch`, "DisplayName", sys.SZ("Overwatch"))
	_ = m.R.SetValue(un+`Indie Blizz`, "UninstallString", sys.SZ(`"C:\Program Files (x86)\Battle.net\Battle.net.exe" --uid=zzgame --action=uninstall`))
	_ = m.R.SetValue(un+`Indie Blizz`, "InstallLocation", sys.SZ(`D:\Games\IndieBlizz`))
	_ = m.R.SetValue(un+`Indie Blizz`, "DisplayName", sys.SZ("Indie Blizz"))
	_ = m.R.SetValue(un+`Battle.net`, "UninstallString", sys.SZ(`"C:\Program Files (x86)\Battle.net\Battle.net Launcher.exe" --uid=battle.net --action=uninstall`))
	_ = m.R.SetValue(un+`Battle.net`, "InstallLocation", sys.SZ(`C:\Program Files (x86)\Battle.net`))
	_ = m.R.SetValue(un+`Other`, "UninstallString", sys.SZ(`C:\Other\unins000.exe --uid=prometheus`))
	_ = m.R.SetValue(un+`Other`, "InstallLocation", sys.SZ(`D:\Other`))
	_ = m.R.SetValue(un+`Gone`, "UninstallString", sys.SZ(`"C:\Program Files (x86)\Battle.net\Battle.net.exe" --uid=fen`))
	_ = m.R.SetValue(un+`Gone`, "InstallLocation", sys.SZ(`D:\Games\Removed`))
	f.addSized(`D:\Games\Overwatch\_retail_\Overwatch.exe`, 10)
	f.addSized(`D:\Games\IndieBlizz\IndieBlizz.exe`, 3<<20)
	f.addSized(`C:\Program Files (x86)\Battle.net\Battle.net Launcher.exe`, 3<<20)
	f.addSized(`D:\Other\other.exe`, 3<<20)

	// EA: one game with a valid folder, one with a stale folder.
	_ = m.R.SetValue(`HKLM\SOFTWARE\WOW6432Node\EA Games\Battlefield 1`, "Install Dir", sys.SZ(`E:\EA Games\Battlefield 1\`))
	_ = m.R.SetValue(`HKLM\SOFTWARE\WOW6432Node\EA Games\Old Game`, "Install Dir", sys.SZ(`E:\EA Games\Old Game\`))
	f.addSized(`E:\EA Games\Battlefield 1\bf1.exe`, 5<<20)

	// Xbox: a GDK game (config in Content), a package under WindowsApps, a
	// package with no config and a system app.
	pk := `HKCU\Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages\`
	gdk := `Contoso.ExampleGame_1.0.0.0_x64__abcdefghijklm`
	_ = m.R.SetValue(pk+gdk, "PackageRootFolder", sys.SZ(`D:\XboxGames\Example Game\Content`))
	_ = m.R.SetValue(pk+`Contoso.Store_1.0_x64__abcdefghijklm`, "PackageRootFolder", sys.SZ(`C:\Program Files\WindowsApps\Contoso.Store_1.0_x64__abcdefghijklm`))
	_ = m.R.SetValue(pk+`Contoso.NoConfig_1.0_x64__abcdefghijklm`, "PackageRootFolder", sys.SZ(`D:\XboxGames\NoConfig\Content`))
	_ = m.R.SetValue(pk+`Microsoft.XboxGameCallableUI_1000_neutral_neutral_cw5n1h2txyewy`, "PackageRootFolder", sys.SZ(`C:\Windows\SystemApps\Microsoft.XboxGameCallableUI_cw5n1h2txyewy`))
	f.add(`D:\XboxGames\Example Game\Content\MicrosoftGame.Config`, msGameConfigSample)
	f.addSized(`D:\XboxGames\Example Game\Content\Game\ExampleGame.exe`, 10)
	f.addSized(`D:\XboxGames\NoConfig\Content\NoConfig.exe`, 3<<20)
	f.add(`C:\Windows\SystemApps\Microsoft.XboxGameCallableUI_cw5n1h2txyewy\MicrosoftGame.config`, msGameConfigSample)

	return &Scanner{Reg: m.R, FS: f, Env: func(string) string { return "" }}, m
}

func TestScanBattleNet(t *testing.T) {
	s, _ := launcherFixture(t)
	byID := map[string]Game{}
	for _, g := range s.battlenet() {
		byID[g.ID] = g
	}
	if len(byID) != 2 {
		t.Fatalf("esperado 2 jogos da Battle.net (sem o cliente, sem o outro programa, sem a pasta ausente): %+v", byID)
	}
	ow := byID["battlenet:prometheus"]
	if ow.Name != "Overwatch 2" || ow.Launcher != "Battle.net" || ow.ExeName != "Overwatch.exe" || ow.ExeSource != "known" || !ow.Competitive || ow.InstallDir != `D:\Games\Overwatch` {
		t.Errorf("Overwatch 2: %+v", ow)
	}
	ib := byID["battlenet:zzgame"]
	if ib.Name != "Indie Blizz" || ib.ExeName != "IndieBlizz.exe" || ib.ExeSource != "heuristic" || ib.Competitive {
		t.Errorf("jogo fora da lista: %+v", ib)
	}
}

func TestScanEA(t *testing.T) {
	s, _ := launcherFixture(t)
	gs := s.ea()
	if len(gs) != 1 {
		t.Fatalf("só o jogo com pasta existente: %+v", gs)
	}
	g := gs[0]
	if g.Name != "Battlefield 1" || g.Launcher != "EA" || g.ExeName != "bf1.exe" || g.InstallDir != `E:\EA Games\Battlefield 1` || g.ID != "ea:battlefield1" {
		t.Errorf("Battlefield 1: %+v", g)
	}
}

func TestScanXbox(t *testing.T) {
	s, _ := launcherFixture(t)
	gs := s.xbox()
	if len(gs) != 1 {
		t.Fatalf("só o pacote com MicrosoftGame.config fora de WindowsApps e do sistema: %+v", gs)
	}
	g := gs[0]
	if g.Name != "Example Game" || g.Launcher != "Xbox" || g.AppID != "Contoso.ExampleGame" || g.ExeSource != "manifest" ||
		!strings.EqualFold(g.Exe, `D:\XboxGames\Example Game\Content\Game\ExampleGame.exe`) || g.ExeName != "ExampleGame.exe" {
		t.Errorf("jogo Xbox: %+v", g)
	}
}

func TestXboxNameFallsBackToFolder(t *testing.T) {
	c, ok := parseMSGameConfig([]byte(`<Game><ExecutableList><Executable Name="a.exe"/></ExecutableList><ShellVisuals DefaultDisplayName="ms-resource:Name"/></Game>`))
	if !ok || c.Visuals.Display != "ms-resource:Name" || len(c.Executables) != 1 {
		t.Fatalf("config: %+v %v", c, ok)
	}
	if _, ok := parseMSGameConfig([]byte(`<Game></Game>`)); ok {
		t.Error("sem executáveis não é jogo")
	}
	if _, ok := parseMSGameConfig([]byte(`não é xml`)); ok {
		t.Error("xml inválido")
	}
	s, m := launcherFixture(t)
	pk := `HKCU\Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages\Contoso.Res_1.0_x64__abcdefghijklm`
	_ = m.R.SetValue(pk, "PackageRootFolder", sys.SZ(`F:\XboxGames\Res Game\Content`))
	f := s.FS.(*memFS)
	f.add(`F:\XboxGames\Res Game\Content\MicrosoftGame.config`, `<Game><ExecutableList><Executable Name="res.exe"/></ExecutableList><ShellVisuals DefaultDisplayName="ms-resource:Name"/></Game>`)
	f.addSized(`F:\XboxGames\Res Game\Content\res.exe`, 10)
	var got *Game
	for _, g := range s.xbox() {
		if g.AppID == "Contoso.Res" {
			g := g
			got = &g
		}
	}
	if got == nil || got.Name != "Res Game" || got.ExeName != "res.exe" {
		t.Errorf("nome por pasta: %+v", got)
	}
}

// All three new launchers go through Scan, are listed together with the
// older ones and carry an executable name, so the per-game profiles (keyed
// by the executable) can be applied to them.
func TestScanIncludesNewLaunchers(t *testing.T) {
	s, _ := launcherFixture(t)
	launchers := map[string]int{}
	for _, g := range s.Scan() {
		launchers[g.Launcher]++
		if g.ExeName == "" {
			t.Errorf("%s sem executável: sem perfil possível", g.ID)
		}
	}
	if launchers["Battle.net"] != 2 || launchers["EA"] != 1 || launchers["Xbox"] != 1 {
		t.Errorf("launchers: %v", launchers)
	}
}

// With no launcher installed (the common case) nothing is listed and nothing
// fails, including a nil registry.
func TestNewLaunchersAbsent(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	s := &Scanner{Reg: m.R, FS: newMemFS(), Env: func(string) string { return "" }}
	if len(s.battlenet())+len(s.ea())+len(s.xbox()) != 0 {
		t.Error("não deveria achar nada")
	}
	s = &Scanner{FS: newMemFS(), Env: func(string) string { return "" }}
	if len(s.battlenet())+len(s.ea())+len(s.xbox()) != 0 {
		t.Error("registro nulo")
	}
}

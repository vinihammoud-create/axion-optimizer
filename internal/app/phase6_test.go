package app

import (
	"context"
	"strings"
	"testing"

	"axion/internal/games"
	"axion/internal/sys"
)

func TestOldDriverPackagesEndpoint(t *testing.T) {
	_, c := setupServer(t)
	var v OldPackagesView
	c.json("GET", "/api/drivers/old", nil, &v)
	if !v.Supported || len(v.Packages) != 1 || v.Packages[0].Provider != "NVIDIA" || v.Packages[0].Published != "oem120.inf" {
		t.Fatalf("pacotes antigos do simulador: %+v", v)
	}
}

func TestGamesEndpointListsUnused(t *testing.T) {
	_, c := setupServer(t)
	scan(t, c)
	var v struct {
		Unused     []games.Unused `json:"unused"`
		UnusedDays int            `json:"unusedDays"`
	}
	c.json("GET", "/api/games", nil, &v)
	if v.UnusedDays != games.UnusedDays {
		t.Errorf("limite: %d", v.UnusedDays)
	}
	if v.Unused == nil {
		t.Fatalf("a lista deve ser [] e não null")
	}
	// The simulator's library has a Steam game last played 400 days ago; the
	// games played recently (and the other launchers) must not be listed.
	if len(v.Unused) != 1 {
		t.Fatalf("o jogo antigo do simulador deve ser o único da lista: %+v", v.Unused)
	}
	u := v.Unused[0]
	if u.AppID != "1091500" || u.Launcher != "Steam" || u.Name != "Cyberpunk 2077" || u.Never || u.Days < 399 || u.SizeBytes != 24<<30 {
		t.Errorf("jogo sem uso: %+v", u)
	}
}

func TestOldPackagesDeleteJob(t *testing.T) {
	m, c := setupServer(t)
	var j Job
	c.json("POST", "/api/drivers/old/delete", OldPackagesDeleteRequest{Published: []string{"oem120.inf"}}, &j)
	if j.ID == "" {
		t.Fatalf("a rota deve devolver um job (a tela espera um id): %+v", j)
	}
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("job: %+v", j)
	}
	var got struct {
		Result OldPackagesDeleteResult `json:"result"`
	}
	c.json("GET", "/api/jobs/"+j.ID, nil, &got)
	if len(got.Result.Removed) != 1 || got.Result.Removed[0] != "oem120.inf" || len(got.Result.Failed) != 0 {
		t.Errorf("resultado: %+v", got.Result)
	}
	var v OldPackagesView
	c.json("GET", "/api/drivers/old", nil, &v)
	if !v.Supported || len(v.Packages) != 0 {
		t.Errorf("depois de excluir não deve sobrar pacote antigo: %+v", v)
	}
	for _, call := range m.X.Calls {
		for _, a := range call.Args {
			if la := strings.ToLower(a); la == "/uninstall" || la == "/force" {
				t.Errorf("o Axion passou %s ao pnputil: %v", a, call.Args)
			}
		}
	}
}

// Only what the user confirmed AND is still old today is deleted; a stale or
// forged list is refused as a whole, before anything is removed.
func TestOldPackagesDeleteRefusesWhatIsNotListed(t *testing.T) {
	m, c := setupServer(t)
	for _, req := range []OldPackagesDeleteRequest{
		{},
		{Published: []string{}},
		{Published: []string{"  ", ""}},
		{Published: []string{"oem999.inf"}}, // not in the store
		{Published: []string{"oem148.inf"}}, // in use by the GPU
		{Published: []string{"oem120.inf", "oem148.inf"}}, // one bad name refuses all
		{Published: []string{"oem1.inf /force"}},
		{Published: []string{`..\oem120.inf`}},
	} {
		if code, b := c.do("POST", "/api/drivers/old/delete", req, nil); code != 400 {
			t.Errorf("%+v deveria ser recusado: %d %s", req.Published, code, b)
		}
	}
	for _, call := range m.X.Calls {
		if len(call.Args) > 0 && call.Args[0] == "/delete-driver" {
			t.Errorf("nada deveria ter sido excluído: %v", call.Args)
		}
	}
}

func TestOldPackagesDeleteNeedsAdministrator(t *testing.T) {
	m, c := setupServer(t)
	m.N.Elevated = false
	if code, b := c.do("POST", "/api/drivers/old/delete", OldPackagesDeleteRequest{Published: []string{"oem120.inf"}}, nil); code != 400 || !strings.Contains(string(b), "administrador") {
		t.Errorf("sem administrador: %d %s", code, b)
	}
}

// The "Desinstalar" button of an unused game opens steam://uninstall/<appid>,
// and only that steam: link (Steam asks for its own confirmation).
func TestOpenSteamUninstallOnly(t *testing.T) {
	_, c := setupServer(t)
	if code, b := c.do("POST", "/api/open", map[string]string{"url": "steam://uninstall/730"}, nil); code != 200 {
		t.Errorf("steam://uninstall/730: %d %s", code, b)
	}
	for _, u := range []string{"steam://run/730", "steam://uninstall/", "steam://uninstall/730/x", "steam://uninstall/7a", "steam://install/730"} {
		if code, _ := c.do("POST", "/api/open", map[string]string{"url": u}, nil); code != 400 {
			t.Errorf("%s deveria ser recusado: %d", u, code)
		}
	}
}

// wlanReading calls netsh.exe, picks the block of the measured adapter and
// says nothing when the command fails (Windows 11 24H2 may refuse without the
// location permission).
func TestWlanReading(t *testing.T) {
	m, c := setupServer(t)
	out := "    GUID : aaaaaaaa-1111-2222-3333-444444444444\r\n    Banda : 2,4 GHz\r\n    Sinal : 61 %\r\n" +
		"    GUID : bbbbbbbb-5555-6666-7777-888888888888\r\n    Banda : 5 GHz\r\n    Sinal : 90%\r\n"
	code := 0
	var exes []string
	m.X.Handler = func(exe string, args []string, _ string) (sys.Result, bool) {
		if strings.Contains(strings.ToLower(strings.Join(args, " ")), "wlan show interfaces") {
			exes = append(exes, exe)
			return sys.Result{ExitCode: code, Stdout: out}, true
		}
		return sys.Result{}, false
	}
	sig, band := c.s.App.wlanReading(context.Background(), "{BBBBBBBB-5555-6666-7777-888888888888}")
	if sig != 90 || band != "5 GHz" {
		t.Errorf("leitura: %d %q", sig, band)
	}
	sig, band = c.s.App.wlanReading(context.Background(), "{aaaaaaaa-1111-2222-3333-444444444444}")
	if sig != 61 || band != "2.4 GHz" {
		t.Errorf("leitura pt-BR: %d %q", sig, band)
	}
	if len(exes) != 2 || exes[0] != "netsh.exe" {
		t.Errorf("deve chamar netsh.exe: %v", exes)
	}
	code = 1
	if sig, band = c.s.App.wlanReading(context.Background(), "{aaaaaaaa-1111-2222-3333-444444444444}"); sig != 0 || band != "" {
		t.Errorf("com erro do netsh não se diz nada: %d %q", sig, band)
	}
}

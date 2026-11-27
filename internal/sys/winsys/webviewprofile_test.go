package winsys

import (
	"strings"
	"testing"
)

// The window's browser runs de-elevated, as the signed-in account, so its
// profile must be in that account's Local AppData. In the protected data
// folder (ProgramData\Axion) the browser could not create it and the window
// did not open on a real PC.
func TestWebViewProfileDirIsWritableByTheSignedInAccount(t *testing.T) {
	p := confinePaths()
	got := WebViewProfileDir(p)
	if want := `C:\Users\Ana\AppData\Local\Axion\WebView2`; got != want {
		t.Fatalf("perfil da janela = %q, quero %q", got, want)
	}
	if pathWithin(got, p.ProgramData) {
		t.Fatalf("perfil da janela na pasta protegida, onde o navegador sem privilégio não grava: %s", got)
	}

	// Local AppData aimed outside the profile goes back to it first
	// (confineUserFolders), and the profile follows.
	p.LocalAppData = `C:\ProgramData\Axion`
	confineUserFolders(&p)
	if got := WebViewProfileDir(p); !strings.HasPrefix(got, `C:\Users\Ana\`) {
		t.Fatalf("Local AppData apontada para fora do perfil: %s", got)
	}

	// Unknown Local AppData: no path is made up.
	p.LocalAppData = ""
	if got := WebViewProfileDir(p); got != "" {
		t.Fatalf("sem Local AppData: %q", got)
	}
}

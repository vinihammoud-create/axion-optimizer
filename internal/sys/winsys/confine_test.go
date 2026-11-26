package winsys

import (
	"testing"

	"axion/internal/sys"
)

func confinePaths() sys.Paths {
	return sys.Paths{
		UserProfile: `C:\Users\Ana`, WindowsDir: `C:\Windows`, ProgramFiles: `C:\Program Files`,
		ProgramFilesX86: `C:\Program Files (x86)`, ProgramData: `C:\ProgramData`,
		AppData: `C:\Users\Ana\AppData\Roaming`, LocalAppData: `C:\Users\Ana\AppData\Local`,
		StartupUser: `C:\Users\Ana\AppData\Roaming\Microsoft\Windows\Start Menu\Programs\Startup`,
		UserTemp:    `C:\Users\Ana\AppData\Local\Temp`,
	}
}

// A per-user folder aimed at a place the account cannot write goes back to
// the profile: Axion is elevated and would act there with rights the user
// does not have.
func TestConfineUserFoldersRejectsSystemPlaces(t *testing.T) {
	for _, bad := range []string{
		`C:\Windows`, `C:\Windows\System32\config`, `C:\Program Files\Jogo`, `C:\ProgramData`,
		`C:\Users\Outro\AppData\Local`, `C:\Users`, `C:\`, `c:\WINDOWS\Temp`, `C:\Users\Ana\..\Outro\AppData\Local`, `relativo\Temp`,
	} {
		p := confinePaths()
		p.AppData, p.LocalAppData, p.StartupUser, p.UserTemp = bad, bad, bad, bad
		confineUserFolders(&p)
		want := confinePaths()
		if p.AppData != want.AppData || p.LocalAppData != want.LocalAppData || p.StartupUser != want.StartupUser || p.UserTemp != want.UserTemp {
			t.Errorf("%q deveria voltar ao padrão do perfil: %+v", bad, p)
		}
	}
}

// What is inside the profile, on another drive or on a network share is the
// user's own (folder redirection) and stays.
func TestConfineUserFoldersKeepsTheUsersOwnPlaces(t *testing.T) {
	for _, ok := range []string{
		`C:\Users\Ana\AppData\Local`, `c:\users\ana\Dados\Local`, `D:\Perfil\Ana\Local`, `\\servidor\perfis\ana\AppData`, `C:\Dados\Ana\Local`,
	} {
		p := confinePaths()
		p.AppData, p.LocalAppData, p.StartupUser, p.UserTemp = ok, ok, ok, ok
		confineUserFolders(&p)
		if p.AppData != ok || p.LocalAppData != ok || p.StartupUser != ok || p.UserTemp != ok {
			t.Errorf("%q é da própria conta e deveria ficar: %+v", ok, p)
		}
	}
	// Unknown profile: nothing to compare with, nothing changes.
	p := confinePaths()
	p.UserProfile, p.LocalAppData = "", `C:\Windows`
	confineUserFolders(&p)
	if p.LocalAppData != `C:\Windows` {
		t.Error("sem perfil conhecido a função não deve mexer")
	}
}

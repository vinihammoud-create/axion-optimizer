package catalog

import "testing"

// Windows' own programs never get a game profile, however the path to them
// is spelled.
func TestUnderDirNormalizesThePath(t *testing.T) {
	const win = `C:\Windows`
	inside := []string{
		`C:\Windows\System32\notepad.exe`,
		`c:\windows\system32\NOTEPAD.EXE`,
		`C:/Windows/System32/notepad.exe`,
		`C:\Jogos\..\Windows\System32\notepad.exe`,
		`C:\Windows\.\System32\notepad.exe`,
		`\\?\C:\Windows\System32\notepad.exe`,
		`\\.\C:\Windows\System32\notepad.exe`,
		`\??\C:\Windows\System32\notepad.exe`,
		`C:\Windows.\System32\notepad.exe`,
		`C:\Windows \System32\notepad.exe`,
		`C:\Windows\\System32\notepad.exe`,
		`C:\Windows`,
	}
	for _, p := range inside {
		if !UnderDir(p, win) {
			t.Errorf("%q deveria contar como dentro de %s", p, win)
		}
		if !UnderDir(p, win+`\`) {
			t.Errorf("%q deveria contar como dentro de %s\\", p, win)
		}
	}
	outside := []string{
		`C:\WindowsApps\jogo.exe`,
		`C:\Jogos\Windows\jogo.exe`,
		`D:\Windows\jogo.exe`,
		`C:\Windows\..\Jogos\jogo.exe`,
		`\\servidor\Windows\jogo.exe`,
		`\\?\UNC\servidor\Windows\jogo.exe`,
		``,
	}
	for _, p := range outside {
		if UnderDir(p, win) {
			t.Errorf("%q não está dentro de %s", p, win)
		}
	}
	// Without the folder's name the usual one is assumed: the check must
	// not pass because the name was missing.
	if !UnderDir(`C:\Windows\System32\notepad.exe`, "") {
		t.Error("pasta do Windows desconhecida não pode liberar o caminho")
	}
}

func TestInWindowsDirWithoutSystemRoot(t *testing.T) {
	t.Setenv("SystemRoot", "")
	if !inWindowsDir(`C:\Windows\System32\notepad.exe`) {
		t.Error("sem SystemRoot, a pasta padrão do Windows continua valendo")
	}
	if inWindowsDir(`C:\Jogos\jogo.exe`) || inWindowsDir("") {
		t.Error("caminho fora do Windows não pode ser recusado")
	}
	t.Setenv("SystemRoot", `D:\WinNT`)
	if !inWindowsDir(`\\?\D:\WinNT\System32\..\notepad.exe`) || inWindowsDir(`C:\Windows\System32\notepad.exe`) {
		t.Error("SystemRoot definido decide a pasta")
	}
}

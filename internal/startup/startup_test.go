package startup

import (
	"strings"
	"testing"
	"time"

	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

func TestExePath(t *testing.T) {
	env := func(k string) string {
		return map[string]string{"windir": `C:\Windows`, "ProgramFiles": `C:\Program Files`}[k]
	}
	for in, want := range map[string]string{
		`"C:\Users\P\AppData\Local\Discord\Update.exe" --processStart Discord.exe`: `C:\Users\P\AppData\Local\Discord\Update.exe`,
		`C:\Program Files\Corsair\iCUE.exe --autorun`:                              `C:\Program Files\Corsair\iCUE.exe`,
		`%windir%\system32\SecurityHealthSystray.exe`:                              `C:\Windows\system32\SecurityHealthSystray.exe`,
		`%ProgramFiles%\X\y.exe /s`:                                                `C:\Program Files\X\y.exe`,
		`%UNKNOWN%\a.exe`:                                                          `%UNKNOWN%\a.exe`,
		`rundll32.exe C:\x\y.dll,Start`:                                            `rundll32.exe`,
		`"C:\sem aspas finais.exe`:                                                 `C:\sem aspas finais.exe`,
	} {
		if got := ExePath(in, env); got != want {
			t.Errorf("ExePath(%q) = %q, esperado %q", in, got, want)
		}
	}
}

func TestApprovedValue(t *testing.T) {
	on, off := ApprovedValue(true, time.Now()), ApprovedValue(false, time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC))
	if !Approved(on) || Approved(off) || len(on.Bin) != 12 || on.Bin[0] != 2 || off.Bin[0] != 3 {
		t.Fatalf("on=%v off=%v", on.Bin, off.Bin)
	}
	if !Approved(sys.RegValue{}) || Approved(sys.Binary([]byte{7, 0, 0, 0})) || !Approved(sys.Binary([]byte{6, 0, 0, 0})) {
		t.Error("valores ausentes contam como ligados; bit baixo = desligado")
	}
}

func TestListAndToggle(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	items := List(m.Platform, func(string) string { return "" })
	by := map[string]Item{}
	for _, it := range items {
		by[it.ID] = it
	}
	check := func(id string, enabled bool, verdict string, running bool) {
		t.Helper()
		it, ok := by[id]
		if !ok {
			t.Fatalf("%s ausente em %v", id, keys(by))
		}
		if it.Enabled != enabled || it.Verdict != verdict || it.Running != running {
			t.Errorf("%s: ligado=%v veredito=%s rodando=%v (%s)", id, it.Enabled, it.Verdict, it.Running, it.Advice)
		}
	}
	check("run-user|Discord", true, Disable, true) // Update.exe --processStart Discord.exe
	check("run-user|Steam", true, Disable, true)
	check("run-user|OneDrive", true, Review, true)
	check("run-user|EpicGamesLauncher", false, Disable, false)
	check("run-machine|SecurityHealth", true, Keep, false)
	check("run-machine|iCUE", true, Review, true)
	check("run-machine32|Adobe ARM", true, Disable, false)
	check("folder-user|Spotify.lnk", true, Disable, false)
	check(`task|\GoogleUpdateTaskMachineUA`, true, Disable, false)
	if _, ok := by["folder-user|desktop.ini"]; ok {
		t.Error("desktop.ini não é item de inicialização")
	}
	if items[len(items)-1].Enabled {
		t.Error("desligados vão para o fim")
	}

	now := time.Now()
	for _, id := range []string{"run-user|Discord", "run-machine32|Adobe ARM", "folder-user|Spotify.lnk", `task|\GoogleUpdateTaskMachineUA`} {
		if err := SetEnabled(m.Platform, id, false, now); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	after := map[string]bool{}
	for _, it := range List(m.Platform, nil) {
		after[it.ID] = it.Enabled
	}
	for _, id := range []string{"run-user|Discord", "run-machine32|Adobe ARM", "folder-user|Spotify.lnk", `task|\GoogleUpdateTaskMachineUA`} {
		if after[id] {
			t.Errorf("%s continua ligado", id)
		}
	}
	// Run32 entries use the Run32 approved key, like Task Manager.
	if v, _ := m.R.GetValue(`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run32`, "Adobe ARM"); !v.Exists || v.Bin[0] != 3 {
		t.Errorf("Run32: %+v", v)
	}
	// The Run value itself is untouched.
	if v, _ := m.R.GetValue(`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "Discord"); !v.Exists {
		t.Error("a entrada do Run foi apagada")
	}
	if err := SetEnabled(m.Platform, "run-user|Discord", true, now); err != nil {
		t.Fatal(err)
	}
	if err := SetEnabled(m.Platform, "run-user|NaoExiste", false, now); err == nil {
		t.Error("item inexistente aceito")
	}
	if err := SetEnabled(m.Platform, "lixo", false, now); err == nil {
		t.Error("id inválido aceito")
	}
}

func TestLocate(t *testing.T) {
	const approved = `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\`
	l, err := Locate("run-user|Discord")
	if err != nil || l.Key != approved+"Run" || l.Value != "Discord" || l.RunKey != `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` || l.Name != "Discord" {
		t.Errorf("run-user: %+v %v", l, err)
	}
	l, err = Locate("run-machine32|Adobe ARM")
	if err != nil || !strings.HasSuffix(l.Key, `StartupApproved\Run32`) || !strings.HasPrefix(l.RunKey, `HKLM\SOFTWARE\WOW6432Node`) {
		t.Errorf("run-machine32: %+v %v", l, err)
	}
	l, err = Locate("folder-user|Spotify.lnk")
	if err != nil || l.Key != approvedFolderUser || l.Value != "Spotify.lnk" || l.Name != "Spotify" || l.RunKey != "" {
		t.Errorf("folder-user: %+v %v", l, err)
	}
	l, err = Locate("folder-common|Office.lnk")
	if err != nil || l.Key != approvedFolderAll {
		t.Errorf("folder-common: %+v %v", l, err)
	}
	l, err = Locate(`task|\GoogleUpdateTaskMachineUA`)
	if err != nil || l.Task != `\GoogleUpdateTaskMachineUA` || l.Name != "GoogleUpdateTaskMachineUA" || l.Key != "" {
		t.Errorf("task: %+v %v", l, err)
	}
	for _, bad := range []string{"lixo", "xyz|a", "run-user|", ""} {
		if _, err := Locate(bad); err == nil {
			t.Errorf("%q deveria ser recusado", bad)
		}
	}
}

func keys(m map[string]Item) string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return strings.Join(out, ", ")
}

func TestSeenRoundTripAndNewSince(t *testing.T) {
	items := []Item{{ID: "run-user|Discord"}, {ID: `task|\Foo`}}
	b, err := EncodeSeen(Keys(items), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	seen, ok := ParseSeen(b)
	if !ok || !seen["run-user|discord"] || !seen[`task|\foo`] {
		t.Fatalf("ida e volta falhou: %v %v", ok, seen)
	}
	// Case of the registry name does not make an entry new.
	cur := []Item{{ID: "RUN-USER|DISCORD"}, {ID: "run-user|Outro"}}
	got := NewSince(seen, true, cur)
	if len(got) != 1 || !got["run-user|outro"] {
		t.Errorf("só o Outro é novo: %v", got)
	}
	if n := NewSince(seen, false, cur); len(n) != 0 {
		t.Errorf("sem linha de base nada é novo: %v", n)
	}
	for _, bad := range []string{"", "x", "{", `{"version":2}`, `{"ids":["a"]}`} {
		if _, ok := ParseSeen([]byte(bad)); ok {
			t.Errorf("%q não deveria ser aceito", bad)
		}
	}
}

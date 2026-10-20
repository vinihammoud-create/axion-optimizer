package cleanup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"axion/internal/sys"
)

func write(t *testing.T, path string, size int, age time.Duration) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
	backdateCreation(t, path, when)
}

func age(t *testing.T, path string, d time.Duration) {
	t.Helper()
	when := time.Now().Add(-d)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
	backdateCreation(t, path, when)
}

func TestScanAndCleanTemp(t *testing.T) {
	base := t.TempDir()
	tmp := filepath.Join(base, "Users", "Gamer", "AppData", "Local", "Temp")
	outside := filepath.Join(base, "Important", "keep.txt")
	write(t, filepath.Join(tmp, "old.tmp"), 1000, 48*time.Hour)
	write(t, filepath.Join(tmp, "new.tmp"), 500, time.Hour)
	write(t, filepath.Join(tmp, "sub", "old2.log"), 300, 72*time.Hour)
	write(t, outside, 42, 90*24*time.Hour)
	ro := filepath.Join(tmp, "readonly.tmp")
	write(t, ro, 10, 30*time.Hour)
	_ = os.Chmod(ro, 0o444)
	age(t, filepath.Join(tmp, "sub"), 72*time.Hour)
	if err := os.MkdirAll(filepath.Join(tmp, "emptyold"), 0o755); err != nil {
		t.Fatal(err)
	}
	age(t, filepath.Join(tmp, "emptyold"), 50*time.Hour)
	// A link planted in TEMP that points elsewhere must never be followed.
	// Windows only lets an administrator (or Developer Mode) create one.
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(tmp, "link")); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
		t.Logf("sem privilégio para criar o link simbólico; o resto do teste segue sem ele: %v", err)
	}

	tg := Targets(sys.Paths{UserTemp: tmp})[0]
	if tg.ID != "temp.user" {
		t.Fatalf("primeiro alvo: %s", tg.ID)
	}
	m := Scan(tg, time.Now(), 0)
	if m.Files != 3 || m.Bytes != 1310 || m.Kept != 1 || !m.Present {
		t.Fatalf("medição: %+v", m)
	}
	r := Clean(tg, time.Now())
	if r.Deleted != 3 || r.Freed != 1310 || r.Skipped != 0 {
		t.Fatalf("limpeza: %+v", r)
	}
	for _, gone := range []string{"old.tmp", "readonly.tmp", "sub", "emptyold"} {
		if _, err := os.Lstat(filepath.Join(tmp, gone)); !os.IsNotExist(err) {
			t.Errorf("%s deveria ter sido removido", gone)
		}
	}
	for _, kept := range []string{filepath.Join(tmp, "new.tmp"), outside, tmp} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s deveria continuar: %v", kept, err)
		}
	}
}

func TestPatternsFilesAndRoots(t *testing.T) {
	base := t.TempDir()
	win := filepath.Join(base, "Windows")
	write(t, filepath.Join(win, "Minidump", "092026-01.dmp"), 100, 10*24*time.Hour)
	write(t, filepath.Join(win, "Minidump", "notes.txt"), 100, 10*24*time.Hour)
	write(t, filepath.Join(win, "Minidump", "deep", "x.dmp"), 100, 10*24*time.Hour)
	write(t, filepath.Join(win, "MEMORY.DMP"), 2048, 40*24*time.Hour)
	var mini, mem Target
	for _, x := range Targets(sys.Paths{WindowsDir: win}) {
		switch x.ID {
		case "dumps.mini":
			mini = x
		case "dumps.memory":
			mem = x
		}
	}
	if m := Scan(mini, time.Now(), 0); m.Files != 1 || m.Bytes != 100 {
		t.Errorf("minidumps (só *.dmp, sem subpastas): %+v", m)
	}
	if m := Scan(mem, time.Now(), 0); m.Files != 1 || m.Bytes != 2048 {
		t.Errorf("MEMORY.DMP: %+v", m)
	}
	if r := Clean(mem, time.Now()); r.Deleted != 1 {
		t.Errorf("MEMORY.DMP não apagado: %+v", r)
	}
	// Roots that would be catastrophic are refused.
	bad := Target{ID: "x", Kind: KindFiles, Recurse: true, Dirs: []string{string(filepath.Separator), filepath.VolumeName(base) + string(filepath.Separator)}}
	if m := Scan(bad, time.Now(), 0); m.Files != 0 {
		t.Fatalf("raiz aceita: %+v", m)
	}
	// Missing folders are simply not present.
	if m := Scan(Targets(sys.Paths{})[0], time.Now(), 0); m.Present || m.Files != 0 {
		t.Errorf("pasta vazia/desconhecida: %+v", m)
	}
}

// GetTempPath falls back to the user profile when TMP and TEMP are unset:
// that must never become the root of a recursive cleanup.
func TestTempFallbackToProfileIsRefused(t *testing.T) {
	base := t.TempDir()
	profile := filepath.Join(base, "Users", "Joao")
	doc := filepath.Join(profile, "Documents", "tese.docx")
	write(t, doc, 1000, 90*24*time.Hour)
	local := filepath.Join(profile, "AppData", "Local")
	old := filepath.Join(local, "Temp", "old.tmp")
	write(t, old, 10, 90*24*time.Hour)
	age(t, filepath.Join(local, "Temp"), 90*24*time.Hour)
	p := sys.Paths{UserTemp: profile, UserProfile: profile, LocalAppData: local, WindowsDir: filepath.Join(base, "Windows")}
	tg := Targets(p)[0]
	if len(tg.Dirs) != 1 || !samePath(tg.Dirs[0], filepath.Join(local, "Temp")) {
		t.Fatalf("a pasta do perfil não pode ser usada; esperado LocalAppData\\Temp: %v", tg.Dirs)
	}
	Clean(tg, time.Now())
	if _, err := os.Stat(doc); err != nil {
		t.Fatalf("documento do usuário apagado: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("o temporário antigo deveria ser apagado")
	}
	// Neither the Windows folder nor a non-Temp folder is accepted.
	for _, d := range []string{p.WindowsDir, profile, filepath.Join(base, "Dados")} {
		if TempDir(p, d) != "" {
			t.Errorf("%s aceito como pasta temporária", d)
		}
	}
}

// An installer extracting right now restores old modification times; its
// fresh folder keeps the payload.
func TestFreshExtractionIsKept(t *testing.T) {
	base := t.TempDir()
	tmp := filepath.Join(base, "AppData", "Local", "Temp")
	payload := filepath.Join(tmp, "is-ABC12.tmp", "setup_data.bin")
	write(t, payload, 5000, 3*365*24*time.Hour) // folder created now, file time from the archive
	tg := Targets(sys.Paths{UserTemp: tmp})[0]
	r := Clean(tg, time.Now())
	if _, err := os.Stat(payload); err != nil {
		t.Fatalf("instalação em andamento apagada (deleted=%d): %v", r.Deleted, err)
	}
}

// Files a pending restart will still move into place stay.
func TestKeepListIsHonoured(t *testing.T) {
	base := t.TempDir()
	tmp := filepath.Join(base, "Temp")
	f := filepath.Join(tmp, "driver_update.sys")
	write(t, f, 100, 10*24*time.Hour)
	tg := Targets(sys.Paths{UserTemp: tmp})[0]
	tg.Keep = map[string]bool{strings.ToLower(filepath.Clean(f)): true}
	if m := Scan(tg, time.Now(), 0); m.Files != 0 || m.Kept != 1 {
		t.Fatalf("medição: %+v", m)
	}
	Clean(tg, time.Now())
	if _, err := os.Stat(f); err != nil {
		t.Fatalf("arquivo com renomeação pendente apagado: %v", err)
	}
}

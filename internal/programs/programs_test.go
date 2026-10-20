package programs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

func TestListAndCommands(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	list := List(m.R)
	var names []string
	for _, p := range list {
		names = append(names, p.Name)
	}
	if strings.Join(names, "|") != "7-Zip 23.01 (x64 edition)|Contoso Game Booster 2.4|Spotify" {
		t.Fatalf("programas: %v", names)
	}
	zip := list[0]
	if !zip.MSI || zip.ProductCode != "{23170F69-40C1-2702-2301-000001000000}" {
		t.Fatalf("MSI: %+v", zip)
	}
	if exe, args := Command(zip, nil); exe != "msiexec.exe" || args != "/x {23170F69-40C1-2702-2301-000001000000}" {
		t.Errorf("MSI deve desinstalar com /x: %q %q", exe, args)
	}
	c := list[1]
	if c.SizeBytes != 104_000<<10 || c.InstallDate.Year() != 2026 || c.Scope != ScopeMachine {
		t.Errorf("Contoso: %+v", c)
	}
	exe, args := Command(c, nil)
	if !strings.HasSuffix(exe, "unins000.exe") || args != "" {
		t.Errorf("comando: %q %q", exe, args)
	}
	paths := m.N.Paths()
	if RunsAsUser(c, exe, paths) {
		t.Error("programa da máquina deveria desinstalar como administrador")
	}
	sp := list[2]
	spExe, spArgs := Command(sp, nil)
	if !RunsAsUser(sp, spExe, paths) || spArgs != "/uninstall" {
		t.Errorf("programa da conta do usuário deve desinstalar sem elevação: %q %q", spExe, spArgs)
	}
	// A machine entry whose uninstaller lives in the user's profile also runs unelevated.
	if !RunsAsUser(Program{Scope: ScopeMachine}, filepath.Join(paths.UserProfile, "x", "u.exe"), paths) {
		t.Error("desinstalador no perfil do usuário deveria rodar sem elevação")
	}
	// Outside the folders only administrators change (a games drive): the
	// file could be swapped by a program without rights, so no elevation.
	if !RunsAsUser(Program{Scope: ScopeMachine}, `Z:\Jogos\X\unins000.exe`, paths) {
		t.Error("desinstalador fora das pastas protegidas deveria rodar sem elevação")
	}
	if RunsAsUser(Program{Scope: ScopeMachine}, filepath.Join(paths.ProgramData, "Package Cache", "x", "setup.exe"), paths) || RunsAsUser(Program{Scope: ScopeMachine}, "msiexec.exe", paths) {
		t.Error("ProgramData e programas do Windows desinstalam como administrador")
	}
	for in, want := range map[string][2]string{
		`"C:\Program Files\X\uninstall.exe" /S`:  {`C:\Program Files\X\uninstall.exe`, "/S"},
		`C:\Program Files\X\uninst.exe --remove`: {`C:\Program Files\X\uninst.exe`, "--remove"},
		`%ProgramFiles%\Y\u.exe`:                 {`C:\PF\Y\u.exe`, ""},
		`rundll32.exe "C:\a b\x.dll",Uninstall`:  {`rundll32.exe`, `"C:\a b\x.dll",Uninstall`},
	} {
		e, a := Command(Program{Uninstall: in}, func(k string) string {
			if k == "ProgramFiles" {
				return `C:\PF`
			}
			return ""
		})
		if e != want[0] || a != want[1] {
			t.Errorf("Command(%q) = %q %q", in, e, a)
		}
	}
}

func TestLeftoversQuarantineRestore(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	paths := m.N.Paths()
	list := List(m.R)
	c := *Find(list, mocksys.ContosoKey)
	exe, args := Command(c, nil)
	if _, err := m.N.RunVisible(context.Background(), exe, args); err != nil {
		t.Fatal(err)
	}
	if ok, _ := m.R.KeyExists(mocksys.ContosoKey); ok {
		t.Fatal("o desinstalador simulado deveria remover a entrada")
	}
	left := FindLeftovers(m.Platform, Snapshot{Program: c}, List(m.R), nil)
	got := map[string]Leftover{}
	for _, l := range left {
		got[l.ID] = l
	}
	loc := filepath.Join(paths.ProgramFiles, "Contoso Game Booster")
	want := []string{
		"dir|" + loc,
		"dir|" + filepath.Join(paths.AppData, "Contoso Game Booster"),
		"dir|" + filepath.Join(paths.ProgramData, "Contoso", "Contoso Game Booster"),
		`regkey|HKCU\Software\Contoso\Contoso Game Booster`,
		`runvalue|HKCU\Software\Microsoft\Windows\CurrentVersion\Run|ContosoBooster`,
		`task|\Contoso\BoosterUpdate`,
	}
	for _, id := range want {
		if _, ok := got[id]; !ok {
			t.Errorf("sobra não encontrada: %s", id)
		}
	}
	if len(left) != len(want) {
		t.Errorf("sobras: %d, esperado %d: %v", len(left), len(want), left)
	}
	for _, l := range left {
		if strings.Contains(l.Path, "Other Product") || strings.HasSuffix(l.Path, filepath.Join("Contoso", "Shared")) || l.Path == filepath.Join(paths.ProgramData, "Contoso") {
			t.Errorf("item de outro programa oferecido: %s", l.Path)
		}
	}
	if got["dir|"+loc].Bytes != 12<<20 {
		t.Errorf("tamanho da pasta que sobrou: %d", got["dir|"+loc].Bytes)
	}

	root := filepath.Join(m.DataDir, "quarentena")
	b, err := Quarantine(m.Platform, root, c.Name, left, time.Now())
	if err != nil || len(b.Errors) != 0 || len(b.Items) != len(left) {
		t.Fatalf("quarentena: %v %+v", err, b)
	}
	if _, err := os.Stat(loc); !os.IsNotExist(err) {
		t.Error("pasta não foi para a quarentena")
	}
	if ok, _ := m.R.KeyExists(`HKCU\Software\Contoso\Contoso Game Booster`); ok {
		t.Error("chave do Registro não foi removida")
	}
	if ok, _ := m.R.KeyExists(`HKCU\Software\Contoso\Other Product`); !ok {
		t.Error("chave de outro produto do mesmo fabricante foi removida")
	}
	if tk, _ := m.T.Get(`\Contoso\BoosterUpdate`); tk.Enabled {
		t.Error("tarefa não foi desativada")
	}
	if bs := Batches(root, m.Key); len(bs) != 1 || bs[0].Bytes == 0 {
		t.Fatalf("lotes: %+v", bs)
	}
	for _, it := range Batches(root, m.Key)[0].Items {
		if it.Partial {
			t.Errorf("item movido por inteiro não pode ficar marcado como parcial: %+v", it)
		}
	}

	r, err := Restore(m.Platform, root, b.ID)
	if err != nil || len(r.Errors) != 0 || !r.Restored {
		t.Fatalf("restaurar: %v %+v", err, r)
	}
	if _, err := os.Stat(filepath.Join(loc, "logs", "booster.log")); err != nil {
		t.Error("arquivo não voltou")
	}
	// Restored folders take the permissions of their place again (a move
	// keeps the ones inherited from the quarantine).
	reinh := strings.Join(m.N.Reinherited, "|")
	for _, d := range []string{loc, filepath.Join(paths.AppData, "Contoso Game Booster")} {
		if !strings.Contains(reinh, d) {
			t.Errorf("permissões não reajustadas em %s (%v)", d, m.N.Reinherited)
		}
	}
	if v, _ := m.R.GetValue(`HKCU\Software\Contoso\Contoso Game Booster\Profiles\CS2`, "Priority"); v.Str != "High" {
		t.Errorf("subchave não voltou: %+v", v)
	}
	if v, _ := m.R.GetValue(`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "ContosoBooster"); !v.Exists {
		t.Error("entrada de inicialização não voltou")
	}
	if tk, _ := m.T.Get(`\Contoso\BoosterUpdate`); !tk.Enabled {
		t.Error("tarefa não foi reativada")
	}
	if len(Batches(root, m.Key)) != 0 {
		t.Error("lote restaurado deveria sair da quarentena")
	}
	if _, err := Restore(m.Platform, root, `..\..\x`); err == nil {
		t.Error("id de lote malicioso aceito")
	}
}

func TestPurgeOldBatches(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(m.DataDir, "quarentena")
	old := time.Now().Add(-Keep - time.Hour)
	if _, err := Quarantine(m.Platform, root, "Velho", nil, old); err != nil {
		t.Fatal(err)
	}
	if _, err := Quarantine(m.Platform, root, "Novo", nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if n := Purge(root, time.Now()); n != 1 || len(Batches(root, m.Key)) != 1 || Batches(root, m.Key)[0].Program != "Novo" {
		t.Fatalf("purge: %d %+v", n, Batches(root, m.Key))
	}
}

func TestCleanName(t *testing.T) {
	for in, want := range map[string]string{
		"Contoso Game Booster 2.4":  "Contoso Game Booster",
		"7-Zip 23.01 (x64 edition)": "7-Zip 23.01 (x64 edition)",
		"Discord (x64)":             "Discord",
		"Some Tool v1.2.3":          "Some Tool",
		"Notepad++ (64-bit x64)":    "Notepad++",
		"Game (1.0)":                "Game",
	} {
		if got := cleanName(in); got != want {
			t.Errorf("cleanName(%q) = %q, esperado %q", in, got, want)
		}
	}
	if usableName("Microsoft") || usableName("ab") || !usableName("Contoso") {
		t.Error("nomes genéricos/curtos")
	}
	_ = sys.Paths{}
}

// Other versions of the same product stay installed: the vendor key and
// folder named after the de-versioned name are shared and never offered.
func TestSharedNamesAreNotLeftovers(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	paths := m.N.Paths()
	u := `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\`
	for _, v := range []struct{ key, name, dir string }{
		{"Python312", "Python 3.12.1 (64-bit)", "Python312"},
		{"Python311", "Python 3.11.9 (64-bit)", "Python311"},
	} {
		_ = m.R.SetValue(u+v.key, "DisplayName", sys.SZ(v.name))
		_ = m.R.SetValue(u+v.key, "Publisher", sys.SZ("Python Software Foundation"))
		_ = m.R.SetValue(u+v.key, "UninstallString", sys.SZ(`"C:\x\uninst.exe"`))
		_ = m.R.SetValue(u+v.key, "InstallLocation", sys.SZ(filepath.Join(paths.ProgramFiles, v.dir)))
		_ = os.MkdirAll(filepath.Join(paths.ProgramFiles, v.dir), 0o755)
	}
	_ = m.R.SetValue(`HKLM\SOFTWARE\Python\PythonCore\3.11`, "SysVersion", sys.SZ("3.11"))
	_ = m.R.SetValue(`HKLM\SOFTWARE\Python\PythonCore\3.12`, "SysVersion", sys.SZ("3.12"))
	_ = os.MkdirAll(filepath.Join(paths.AppData, "Python", "Python311"), 0o755)
	p312 := *Find(List(m.R), u+"Python312")
	for _, n := range []string{"DisplayName", "Publisher", "UninstallString", "InstallLocation"} {
		_ = m.R.DeleteValue(u+"Python312", n)
	}
	_, _ = m.R.DeleteKeyIfEmpty(u + "Python312")
	for _, l := range FindLeftovers(m.Platform, Snapshot{Program: p312}, List(m.R), nil) {
		if strings.EqualFold(l.Path, `HKLM\SOFTWARE\Python`) || strings.EqualFold(l.Path, filepath.Join(paths.AppData, "Python")) {
			t.Errorf("pasta/chave compartilhada com o Python 3.11 oferecida: %s", l.Path)
		}
	}
}

// A program registered with a broad folder (all of Program Files) must not
// drag unrelated startup entries and tasks along.
func TestBroadLocationDoesNotMatchAutoruns(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	paths := m.N.Paths()
	prog := Program{ID: `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Sloppy`, Name: "Sloppy Tool", Location: paths.ProgramFiles}
	for _, l := range FindLeftovers(m.Platform, Snapshot{Program: prog}, List(m.R), nil) {
		if l.Kind == LeftRunValue || l.Kind == LeftTask || l.Kind == LeftDir {
			t.Errorf("item de outro programa oferecido por causa da pasta ampla: %+v", l)
		}
	}
	// A program folder inside the user's documents is offered unticked.
	doc := filepath.Join(paths.UserProfile, "Documents", "Sloppy Tool")
	_ = os.MkdirAll(doc, 0o755)
	prog.Location = doc
	var found bool
	for _, l := range FindLeftovers(m.Platform, Snapshot{Program: prog}, List(m.R), nil) {
		if l.Kind == LeftDir && l.Path == doc {
			found = true
			if l.Default {
				t.Error("pasta dentro de Documentos não pode vir marcada")
			}
		}
	}
	if !found {
		t.Error("pasta do programa em Documentos deveria ser oferecida (desmarcada)")
	}
}

// Deleting a key fails half-way: what was removed stays in the manifest and
// comes back on restore.
func TestQuarantinePartialRegistryKey(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const key = `HKCU\Software\Foo Tool`
	_ = m.R.SetValue(key+`\Settings`, "Theme", sys.SZ("dark"))
	_ = m.R.SetValue(key+`\A-Locked`, "X", sys.DWord(1))
	m.R.FailWrites[sys.NormalizeRegPath(key+`\A-Locked`)] = true
	root := filepath.Join(m.DataDir, "quarentena")
	b, err := Quarantine(m.Platform, root, "Foo Tool", []Leftover{{ID: LeftRegKey + "|" + key, Kind: LeftRegKey, Path: key, Label: key}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Errors) != 1 || len(b.Items) != 1 || !b.Items[0].Partial {
		t.Fatalf("remoção parcial deveria ficar no manifesto como parcial: %+v", b)
	}
	if v, _ := m.R.GetValue(key+`\Settings`, "Theme"); v.Exists {
		t.Fatal("cenário: a subchave desbloqueada deveria ter sido removida")
	}
	delete(m.R.FailWrites, sys.NormalizeRegPath(key+`\A-Locked`))
	r, err := Restore(m.Platform, root, b.ID)
	if err != nil || len(r.Errors) != 0 {
		t.Fatalf("restaurar: %v %+v", err, r)
	}
	if v, _ := m.R.GetValue(key+`\Settings`, "Theme"); v.Str != "dark" {
		t.Errorf("valor removido não voltou: %+v", v)
	}
	// A run value that cannot be deleted leaves nothing in the manifest.
	run := `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	_ = m.R.SetValue(run, "Stuck", sys.SZ(`"C:\x\stuck.exe"`))
	m.R.FailWrites[sys.NormalizeRegPath(run)] = true
	b, err = Quarantine(m.Platform, root, "Stuck", []Leftover{{Kind: LeftRunValue, Path: run, Value: "Stuck", Label: "Stuck"}}, time.Now().Add(time.Second))
	if err != nil || len(b.Items) != 0 || len(b.Errors) != 1 {
		t.Fatalf("valor intacto não deveria ficar no manifesto: %v %+v", err, b)
	}
}

// Restore runs elevated: a manifest pointing at places Quarantine never
// produces is refused item by item.
func TestRestoreRejectsForeignManifest(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	paths := m.N.Paths()
	root := filepath.Join(m.DataDir, "quarentena")
	b, err := Quarantine(m.Platform, root, "Falso", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	evilKey := `HKLM\SYSTEM\CurrentControlSet\Services\Evil`
	ifeo := `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options\sethc.exe`
	val := sys.SZ(`C:\evil.exe`)
	_ = os.MkdirAll(filepath.Join(root, b.ID, "data", "0"), 0o755)
	b.Items = []QItem{
		{Kind: LeftRegKey, Original: evilKey, Reg: &RegTree{Key: evilKey, Values: map[string]sys.RegValue{"ImagePath": val}}},
		{Kind: LeftRegKey, Original: ifeo, Reg: &RegTree{Key: ifeo, Values: map[string]sys.RegValue{"Debugger": val}}},
		{Kind: LeftRunValue, Original: `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon`, Value: "Shell", RegValue: &val},
		{Kind: LeftDir, Original: filepath.Join(paths.WindowsDir, "System32", "evil"), Stored: filepath.Join("data", "0")},
	}
	if err := saveBatch(root, b, m.Key); err != nil {
		t.Fatal(err)
	}
	r, err := Restore(m.Platform, root, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Errors) != 4 || r.Restored {
		t.Fatalf("manifesto estranho deveria ser recusado: %+v", r)
	}
	for _, k := range []string{evilKey, ifeo} {
		if ok, _ := m.R.KeyExists(k); ok {
			t.Errorf("chave gravada a partir de manifesto estranho: %s", k)
		}
	}
	if _, err := os.Stat(filepath.Join(paths.WindowsDir, "System32", "evil")); err == nil {
		t.Error("pasta restaurada dentro do Windows")
	}
	if !restorableKey(`HKLM\SOFTWARE\WOW6432Node\Contoso\Tool`) || !restorableKey(`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\X_is1`) ||
		restorableKey(`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\X\Sub`) || restorableKey(`HKCU\Software\Classes\exefile`) {
		t.Error("regra de chaves restauráveis")
	}
}

func TestMergeBack(t *testing.T) {
	dir := t.TempDir()
	stored, original := filepath.Join(dir, "q"), filepath.Join(dir, "orig")
	for p, c := range map[string]string{filepath.Join(stored, "a.txt"): "cópia", filepath.Join(stored, "b", "c.txt"): "c", filepath.Join(original, "a.txt"): "original"} {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(c), 0o644)
	}
	if err := mergeBack(stored, original); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(original, "a.txt")); string(b) != "original" {
		t.Errorf("arquivo existente foi sobrescrito: %s", b)
	}
	if b, _ := os.ReadFile(filepath.Join(original, "b", "c.txt")); string(b) != "c" {
		t.Error("arquivo que faltava não voltou")
	}
	if exists(stored) {
		t.Error("sobra da cópia não foi removida")
	}
}

// A batch written by anyone but this installation (no valid MAC) is listed
// as unverified and never restored.
func TestUnsignedBatchIsNotRestored(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(m.DataDir, "quarentena")
	b, err := Quarantine(m.Platform, root, "Contoso", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if bs := Batches(root, m.Key); len(bs) != 1 || !bs[0].Verified {
		t.Fatalf("lote da própria instalação deveria ser verificado: %+v", bs)
	}
	run := `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	v := sys.SZ(`C:\evil.exe`)
	b.Items = []QItem{{Kind: LeftRunValue, Original: run, Value: "Evil", RegValue: &v}}
	if err := saveBatch(root, b, []byte("outra chave qualquer")); err != nil {
		t.Fatal(err)
	}
	if bs := Batches(root, m.Key); len(bs) != 1 || bs[0].Verified {
		t.Fatalf("lote forjado aparece como verificado: %+v", bs)
	}
	if _, err := Restore(m.Platform, root, b.ID); err == nil {
		t.Fatal("lote sem assinatura válida foi restaurado")
	}
	if got, _ := m.R.GetValue(run, "Evil"); got.Exists {
		t.Fatal("entrada de inicialização gravada a partir de lote forjado")
	}
}

// A program installed in the vendor folder shared with another product of
// the same vendor (which registers no InstallLocation) must not have that
// folder offered.
func TestSharedVendorFolderIsNotOffered(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	paths := m.N.Paths()
	vendor := filepath.Join(paths.ProgramFilesX86, "Razer")
	for _, f := range []string{filepath.Join(vendor, "Synapse3", "Razer Synapse 3.exe"), filepath.Join(vendor, "Razer Cortex", "RazerCortex.exe")} {
		_ = os.MkdirAll(filepath.Dir(f), 0o755)
		_ = os.WriteFile(f, []byte("x"), 0o644)
	}
	u := `HKLM\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\`
	_ = m.R.SetValue(u+"Synapse3", "DisplayName", sys.SZ("Razer Synapse 3"))
	_ = m.R.SetValue(u+"Synapse3", "Publisher", sys.SZ("Razer Inc."))
	_ = m.R.SetValue(u+"Synapse3", "UninstallString", sys.SZ(`"`+filepath.Join(vendor, "Synapse3", "uninst.exe")+`"`))
	cortex := Program{ID: u + "RazerCortex", Name: "Razer Cortex", Publisher: "Razer Inc.", Location: vendor, Scope: ScopeMachine32}
	for _, l := range FindLeftovers(m.Platform, Snapshot{Program: cortex}, List(m.R), os.Getenv) {
		if l.Kind == LeftDir && (samePathT(l.Path, vendor) || strings.Contains(l.Path, "Synapse3")) {
			t.Errorf("pasta compartilhada oferecida: %s (padrão=%v)", l.Path, l.Default)
		}
	}
}

func samePathT(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }

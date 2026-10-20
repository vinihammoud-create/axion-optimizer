package mocksys

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"axion/internal/sys"
)

// Simulated installed programs, with a sloppy uninstaller (Contoso) that
// leaves folders, settings, a startup entry and a task behind.

// UninstallKey of the simulated Contoso program.
const ContosoKey = `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\ContosoBooster_is1`

func seedPrograms(m *Mock) {
	p := m.N.paths
	loc := filepath.Join(p.ProgramFiles, "Contoso Game Booster")
	for _, f := range []seedFile{
		{filepath.Join(loc, "app.exe"), 38 << 20, 200 * 24 * time.Hour},
		{filepath.Join(loc, "unins000.exe"), 3 << 20, 200 * 24 * time.Hour},
		{filepath.Join(loc, "logs", "booster.log"), 12 << 20, 3 * 24 * time.Hour},
		{filepath.Join(p.AppData, "Contoso Game Booster", "settings.json"), 4 << 10, 10 * 24 * time.Hour},
		{filepath.Join(p.ProgramData, "Contoso", "Contoso Game Booster", "cache.bin"), 64 << 20, 10 * 24 * time.Hour},
		{filepath.Join(p.ProgramData, "Contoso", "Shared", "license.dat"), 1 << 10, 10 * 24 * time.Hour},
		{filepath.Join(p.LocalAppData, "Programs", "Spotify", "Spotify.exe"), 90 << 20, 90 * 24 * time.Hour},
	} {
		_ = os.MkdirAll(filepath.Dir(f.path), 0o755)
		if fh, err := os.Create(f.path); err == nil {
			_ = fh.Truncate(f.size)
			_ = fh.Close()
		}
	}
	set := func(key, name string, v sys.RegValue) { _ = m.R.SetValue(key, name, v) }
	u := `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\`
	set(ContosoKey, "DisplayName", sys.SZ("Contoso Game Booster 2.4"))
	set(ContosoKey, "DisplayVersion", sys.SZ("2.4.1"))
	set(ContosoKey, "Publisher", sys.SZ("Contoso Ltd."))
	set(ContosoKey, "InstallLocation", sys.SZ(loc+string(filepath.Separator)))
	set(ContosoKey, "UninstallString", sys.SZ(`"`+filepath.Join(loc, "unins000.exe")+`"`))
	set(ContosoKey, "InstallDate", sys.SZ("20260301"))
	set(ContosoKey, "EstimatedSize", sys.DWord(104_000))
	set(`HKCU\Software\Contoso\Contoso Game Booster`, "Turbo", sys.DWord(1))
	set(`HKCU\Software\Contoso\Contoso Game Booster\Profiles\CS2`, "Priority", sys.SZ("High"))
	set(`HKCU\Software\Contoso\Other Product`, "x", sys.DWord(1)) // same publisher, different program: never offered
	set(`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "ContosoBooster", sys.SZ(`"`+filepath.Join(loc, "app.exe")+`" --tray`))
	m.T.Add(`\Contoso\BoosterUpdate`, true, []int{sys.TriggerLogon}, filepath.Join(loc, "app.exe")+" /update")

	g := u + `{23170F69-40C1-2702-2301-000001000000}`
	set(g, "DisplayName", sys.SZ("7-Zip 23.01 (x64 edition)"))
	set(g, "DisplayVersion", sys.SZ("23.01.00.0"))
	set(g, "Publisher", sys.SZ("Igor Pavlov"))
	set(g, "UninstallString", sys.ExpandSZ(`MsiExec.exe /I{23170F69-40C1-2702-2301-000001000000}`))
	set(g, "WindowsInstaller", sys.DWord(1))
	set(g, "EstimatedSize", sys.DWord(5_600))

	set(u+"KB5031234", "DisplayName", sys.SZ("Atualização de segurança"))
	set(u+"KB5031234", "UninstallString", sys.SZ("x"))
	set(u+"KB5031234", "ParentKeyName", sys.SZ("OperatingSystem"))
	set(u+"NVIDIA.Container", "DisplayName", sys.SZ("NVIDIA Container"))
	set(u+"NVIDIA.Container", "UninstallString", sys.SZ("x"))
	set(u+"NVIDIA.Container", "SystemComponent", sys.DWord(1))

	sp := `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\Spotify`
	set(sp, "DisplayName", sys.SZ("Spotify"))
	set(sp, "DisplayVersion", sys.SZ("1.2.60.564"))
	set(sp, "Publisher", sys.SZ("Spotify AB"))
	set(sp, "InstallLocation", sys.SZ(filepath.Join(p.LocalAppData, "Programs", "Spotify")))
	set(sp, "UninstallString", sys.SZ(`"`+filepath.Join(p.LocalAppData, "Programs", "Spotify", "Spotify.exe")+`" /uninstall`))
	set(sp, "EstimatedSize", sys.DWord(290_000))

	// The "uninstallers": Contoso removes its entry and executables only.
	m.N.RunHook = func(path, args string) int {
		if strings.HasSuffix(strings.ToLower(path), "unins000.exe") {
			_ = os.Remove(filepath.Join(loc, "app.exe"))
			_ = os.Remove(filepath.Join(loc, "unins000.exe"))
			for _, n := range []string{"DisplayName", "DisplayVersion", "Publisher", "InstallLocation", "UninstallString", "InstallDate", "EstimatedSize"} {
				_ = m.R.DeleteValue(ContosoKey, n)
			}
			_, _ = m.R.DeleteKeyIfEmpty(ContosoKey)
		}
		if strings.EqualFold(filepath.Base(path), "msiexec.exe") && strings.Contains(strings.ToUpper(args), "/X {23170F69") {
			for _, n := range []string{"DisplayName", "DisplayVersion", "Publisher", "UninstallString", "WindowsInstaller", "EstimatedSize"} {
				_ = m.R.DeleteValue(g, n)
			}
			_, _ = m.R.DeleteKeyIfEmpty(g)
		}
		return 0
	}
	// Per-user uninstallers run through a task with the user's rights.
	m.T.OnRegister = func(exe, args string, trigger int) {
		if trigger == sys.TriggerRegistration && strings.HasSuffix(strings.ToLower(exe), "spotify.exe") {
			for _, n := range []string{"DisplayName", "DisplayVersion", "Publisher", "InstallLocation", "UninstallString", "EstimatedSize"} {
				_ = m.R.DeleteValue(sp, n)
			}
			_, _ = m.R.DeleteKeyIfEmpty(sp)
			_ = os.RemoveAll(filepath.Join(p.LocalAppData, "Programs", "Spotify"))
		}
	}
}

// RunVisible implements sys.Native.
func (n *Native) RunVisible(ctx context.Context, path, args string) (int, error) {
	n.mu.Lock()
	n.Visible = append(n.Visible, strings.TrimSpace(path+" "+args))
	hook := n.RunHook
	n.mu.Unlock()
	if hook != nil {
		return hook(path, args), nil
	}
	return 0, nil
}

// ProtectDir implements sys.Native.
func (n *Native) ProtectDir(path string) error { return os.MkdirAll(path, 0o755) }

// Reinherit implements sys.Native (records the call).
func (n *Native) Reinherit(path string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Reinherited = append(n.Reinherited, path)
	return nil
}

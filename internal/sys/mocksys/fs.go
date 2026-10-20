package mocksys

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"axion/internal/sys"
)

// The mock keeps a small Windows-like folder tree inside its data folder so
// that cleanup, startup folders and the uninstaller work on real files.
// Large files are sparse (apparent size only).

// Paths implements sys.Native.
func (n *Native) Paths() sys.Paths {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.paths
}

func mockPaths(root string) sys.Paths {
	j := func(parts ...string) string { return filepath.Join(append([]string{root}, parts...)...) }
	user := j("Users", "Gamer")
	return sys.Paths{
		WindowsDir: j("Windows"), WindowsTemp: j("Windows", "Temp"), UserTemp: filepath.Join(user, "AppData", "Local", "Temp"),
		ProgramData: j("ProgramData"), ProgramFiles: j("Program Files"), ProgramFilesX86: j("Program Files (x86)"),
		AppData: filepath.Join(user, "AppData", "Roaming"), LocalAppData: filepath.Join(user, "AppData", "Local"),
		StartupUser:   filepath.Join(user, "AppData", "Roaming", "Microsoft", "Windows", "Start Menu", "Programs", "Startup"),
		StartupCommon: j("ProgramData", "Microsoft", "Windows", "Start Menu", "Programs", "StartUp"),
		SystemDrive:   root, UserProfile: user,
	}
}

type seedFile struct {
	path string
	size int64
	age  time.Duration
}

// seedFS creates the simulated files; errors are ignored (the mock still works).
func seedFS(p sys.Paths) {
	day := 24 * time.Hour
	files := []seedFile{
		{filepath.Join(p.UserTemp, "setup_4F2A.tmp"), 480 << 20, 12 * day},
		{filepath.Join(p.UserTemp, "chrome_installer.log"), 2 << 10, 30 * day},
		{filepath.Join(p.UserTemp, "7zS8C1E", "Setup.exe"), 95 << 20, 20 * day},
		{filepath.Join(p.UserTemp, "discord_update.log"), 64 << 10, 2 * time.Hour},
		{filepath.Join(p.WindowsTemp, "CAB_2384_2", "update.cab"), 2300 << 20, 25 * day},
		{filepath.Join(p.WindowsTemp, "MpCmdRun.log"), 180 << 10, 3 * day},
		{filepath.Join(p.WindowsDir, "MEMORY.DMP"), 2150 << 20, 58 * day},
		{filepath.Join(p.WindowsDir, "system32", "SecurityHealthSystray.exe"), 96 << 10, 90 * day},
		{filepath.Join(p.WindowsDir, "Minidump", "071526-10312-01.dmp"), 1200 << 10, 73 * day},
		{filepath.Join(p.ProgramData, "Microsoft", "Windows", "WER", "ReportArchive", "AppCrash_cs2.exe_4a1b", "Report.wer"), 24 << 10, 16 * day},
		{filepath.Join(p.LocalAppData, "CrashDumps", "Discord.exe.18244.dmp"), 310 << 20, 40 * day},
		{filepath.Join(p.LocalAppData, "NVIDIA", "DXCache", "d3d_0001.nvph"), 900 << 20, 5 * day}, // shader cache: never offered
		{filepath.Join(p.StartupUser, "Spotify.lnk"), 2 << 10, 90 * day},
		{filepath.Join(p.StartupUser, "desktop.ini"), 174, 400 * day},
		{filepath.Join(p.SystemDrive, "Windows.old", "Windows", "System32", "restante.bin"), 18 << 30, 6 * day},
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			continue
		}
		fh, err := os.Create(f.path)
		if err != nil {
			continue
		}
		markSparse(fh)
		if err := fh.Truncate(f.size); err != nil {
			_ = fh.Close()
			fmt.Fprintln(os.Stderr, "mocksys: arquivo simulado nao criado:", f.path, err)
			continue
		}
		_ = fh.Close()
		when := time.Now().Add(-f.age)
		_ = os.Chtimes(f.path, when, when)
		backdateCreation(f.path, when)
	}
	for _, d := range []string{p.ProgramFiles, p.ProgramFilesX86, p.StartupCommon} {
		_ = os.MkdirAll(d, 0o755)
	}
	seedOBS(p)
	// Folders get the age of what is inside them (they were created then).
	for _, d := range []string{filepath.Join(p.UserTemp, "7zS8C1E"), filepath.Join(p.WindowsTemp, "CAB_2384_2"),
		filepath.Join(p.ProgramData, "Microsoft", "Windows", "WER", "ReportArchive", "AppCrash_cs2.exe_4a1b")} {
		when := time.Now().Add(-16 * day)
		_ = os.Chtimes(d, when, when)
		backdateCreation(d, when)
	}
}

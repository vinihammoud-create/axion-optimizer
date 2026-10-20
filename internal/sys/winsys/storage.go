//go:build windows

package winsys

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"

	"axion/internal/sys"
)

// NVMeHealth implements sys.Native: Get Log Page 02h through the inbox
// NVMe driver (Microsoft Learn, "Working with NVMe drives"). Needs the
// administrator token Axion already runs with; nothing is written.
func (n *Native) NVMeHealth(diskNumber int) (sys.NVMeHealth, error) {
	path, err := windows.UTF16PtrFromString(fmt.Sprintf(`\\.\PhysicalDrive%d`, diskNumber))
	if err != nil {
		return sys.NVMeHealth{}, err
	}
	h, err := windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return sys.NVMeHealth{}, fmt.Errorf("abrir disco %d: %w", diskNumber, err)
	}
	defer windows.CloseHandle(h)
	// Namespace scope first (Microsoft's example), then the controller-wide
	// log for drives that only keep it per controller.
	var errs []string
	for _, adapter := range []bool{false, true} {
		buf := sys.NVMeHealthQueryScope(adapter)
		var got uint32
		err := windows.DeviceIoControl(h, sys.IOCTLStorageQueryProperty, &buf[0], uint32(len(buf)), &buf[0], uint32(len(buf)), &got, nil)
		if err == nil {
			if hl, perr := sys.ParseNVMeHealthResponse(buf, int(got)); perr == nil {
				return hl, nil
			} else {
				err = perr
			}
		}
		scope := "namespace"
		if adapter {
			scope = "controladora"
		}
		errs = append(errs, scope+" "+err.Error())
	}
	return sys.NVMeHealth{}, fmt.Errorf("log de saúde NVMe do disco %d: %s", diskNumber, strings.Join(errs, "; "))
}

// Paths implements sys.Native.
func (n *Native) Paths() sys.Paths {
	kf := func(id *windows.KNOWNFOLDERID) string {
		s, err := windows.KnownFolderPath(id, 0)
		if err != nil {
			return ""
		}
		return s
	}
	p := sys.Paths{
		ProgramData: kf(windows.FOLDERID_ProgramData), ProgramFiles: kf(windows.FOLDERID_ProgramFilesX64),
		ProgramFilesX86: kf(windows.FOLDERID_ProgramFilesX86), AppData: kf(windows.FOLDERID_RoamingAppData), LocalAppData: kf(windows.FOLDERID_LocalAppData),
		StartupUser: kf(windows.FOLDERID_Startup), StartupCommon: kf(windows.FOLDERID_CommonStartup), UserProfile: kf(windows.FOLDERID_Profile),
	}
	if p.UserProfile == "" {
		p.UserProfile = os.Getenv("USERPROFILE")
	}
	for _, id := range []*windows.KNOWNFOLDERID{windows.FOLDERID_Desktop, windows.FOLDERID_Documents, windows.FOLDERID_Downloads,
		windows.FOLDERID_Pictures, windows.FOLDERID_Music, windows.FOLDERID_Videos, windows.FOLDERID_SavedGames, windows.FOLDERID_SkyDrive} {
		if d := kf(id); d != "" {
			p.Personal = append(p.Personal, d)
		}
	}
	if od := os.Getenv("OneDrive"); od != "" {
		p.Personal = append(p.Personal, od)
	}
	// GetTempPath falls back to the user profile, then to the Windows folder,
	// when TMP and TEMP are not set: only a folder really named Temp/Tmp is
	// taken, otherwise the standard %LOCALAPPDATA%\Temp.
	p.UserTemp = os.TempDir()
	switch strings.ToLower(filepath.Base(filepath.Clean(p.UserTemp))) {
	case "temp", "tmp":
	default:
		p.UserTemp = ""
		if p.LocalAppData != "" {
			p.UserTemp = filepath.Join(p.LocalAppData, "Temp")
		}
	}
	if p.ProgramFiles == "" {
		p.ProgramFiles = kf(windows.FOLDERID_ProgramFiles)
	}
	if wd, err := windows.GetWindowsDirectory(); err == nil {
		p.WindowsDir = wd
		p.WindowsTemp = filepath.Join(wd, "Temp")
		p.SystemDrive = filepath.VolumeName(wd)
	}
	// Per-user folders belong to the console account (see user.go).
	userPaths(&p)
	return p
}

package sys

import (
	"path/filepath"
	"strings"
)

// TrustedEnv expands the environment variables found in registry commands
// (uninstall strings, Run entries) from the known-folder API instead of the
// process environment. Axion runs elevated but inherits the user's own
// variables (HKCU\Environment), which any program without administrator
// rights can set: expanding %ProgramFiles% or %windir% from them could make
// Axion run a planted program with its administrator rights. The per-user
// folders are those of the person signed in (see Paths.User). Any other
// variable expands to "" and the command is then treated as not found.
func TrustedEnv(p Paths) func(string) string {
	join := func(base, rest string) string {
		if base == "" {
			return ""
		}
		return filepath.Join(base, rest)
	}
	return func(name string) string {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "systemroot", "windir":
			return p.WindowsDir
		case "systemdrive":
			return p.SystemDrive
		case "programfiles", "programw6432":
			return p.ProgramFiles
		case "programfiles(x86)":
			return p.ProgramFilesX86
		case "commonprogramfiles", "commonprogramw6432":
			return join(p.ProgramFiles, "Common Files")
		case "commonprogramfiles(x86)":
			return join(p.ProgramFilesX86, "Common Files")
		case "programdata", "allusersprofile":
			return p.ProgramData
		case "userprofile":
			return p.UserProfile
		case "appdata":
			return p.AppData
		case "localappdata":
			return p.LocalAppData
		case "temp", "tmp":
			return p.UserTemp
		}
		return ""
	}
}

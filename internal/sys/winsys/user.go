//go:build windows

package winsys

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"axion/internal/sys"
)

// The person using the PC is the account signed in at the console. Axion
// runs elevated, and when UAC was answered with a DIFFERENT administrator
// account (a standard-user gamer on a family PC, for instance) the process
// token belongs to that administrator: HKEY_CURRENT_USER and the known
// folders then point at the administrator's profile, and every per-user
// setting (mouse acceleration, Game Bar, telemetry, startup entries...)
// would be read from and written to the wrong account while Axion reports
// success. consoleUser resolves the interactive account once; the registry
// layer maps HKCU to HKEY_USERS\<SID> and Paths derives the profile folders
// from that hive when the accounts differ.

var (
	wtsapi32                       = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSQuerySessionInformation = wtsapi32.NewProc("WTSQuerySessionInformationW")
)

const (
	wtsUserName   = 5 // WTS_INFO_CLASS WTSUserName
	wtsDomainName = 7 // WTS_INFO_CLASS WTSDomainName
)

// userContext describes the account Axion's per-user changes address.
type userContext struct {
	// Name is DOMAIN\user of the console account (or of the process account
	// when no one else could be resolved).
	Name string
	// SID is the console account's SID string; empty when the process
	// account is used.
	SID string
	// ProcessName is DOMAIN\user of the process token when it differs from
	// the console account (the administrator who answered UAC).
	ProcessName string
	// Differs is true when per-user work is redirected to HKU\<SID> and to
	// the console account's profile folders.
	Differs bool
	// Profile is the console account's profile folder when Differs.
	Profile string
}

var (
	consoleOnce sync.Once
	consoleCtx  userContext
)

// consoleUser returns the resolved interactive account (cached).
func consoleUser() userContext {
	consoleOnce.Do(func() { consoleCtx = resolveConsoleUser() })
	return consoleCtx
}

func accountName(sid *windows.SID) string {
	if sid == nil {
		return ""
	}
	account, domain, _, err := sid.LookupAccount("")
	if err != nil {
		return sid.String()
	}
	if domain == "" {
		return account
	}
	return domain + `\` + account
}

// wtsString reads one string of the console session (WTSQuerySessionInformation).
func wtsString(session uint32, class uint32) string {
	if procWTSQuerySessionInformation.Find() != nil {
		return ""
	}
	var buf *uint16
	var n uint32
	r, _, _ := procWTSQuerySessionInformation.Call(0 /* WTS_CURRENT_SERVER_HANDLE */, uintptr(session), uintptr(class), uintptr(unsafe.Pointer(&buf)), uintptr(unsafe.Pointer(&n)))
	if r == 0 || buf == nil {
		return ""
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(buf)))
	return windows.UTF16PtrToString(buf)
}

func resolveConsoleUser() userContext {
	var ctx userContext
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || tu == nil || tu.User.Sid == nil {
		return ctx
	}
	procSID := tu.User.Sid
	ctx.Name = accountName(procSID)

	// The session Axion itself runs in (the window the person uses), not
	// the physical console: over Remote Desktop they differ.
	var session uint32
	if windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session) != nil || session == 0 {
		session = windows.WTSGetActiveConsoleSessionId()
	}
	if session == 0xFFFFFFFF {
		return ctx
	}
	user := wtsString(session, wtsUserName)
	if user == "" {
		return ctx // no one signed in at the console (service, remote session)
	}
	domain := wtsString(session, wtsDomainName)
	account := user
	if domain != "" {
		account = domain + `\` + user
	}
	sid, _, _, err := windows.LookupSID("", account)
	if err != nil || sid == nil {
		log.Printf("usuário do console (%s): %v", account, err)
		return ctx
	}
	if sid.Equals(procSID) {
		return ctx
	}
	sidStr := sid.String()
	// The hive is loaded while the account is signed in; without it the
	// redirection would fail on every access, so keep the process account.
	k, err := registry.OpenKey(registry.USERS, sidStr, registry.QUERY_VALUE)
	if err != nil {
		log.Printf("hive do usuário do console (%s) indisponível: %v", account, err)
		return ctx
	}
	k.Close()
	profile := profilePath(sidStr)
	if profile == "" {
		// Without the profile folder the per-user folders cannot follow:
		// keep everything on the process account rather than half of it.
		log.Printf("perfil do usuário %s não encontrado: ajustes por usuário ficam na conta %s", account, ctx.Name)
		return ctx
	}
	ctx.ProcessName = ctx.Name
	ctx.Name = account
	ctx.SID = sidStr
	ctx.Differs = true
	ctx.Profile = profile
	log.Printf("Axion elevado como %s; ajustes por usuário direcionados ao usuário do console %s (%s)", ctx.ProcessName, ctx.Name, sidStr)
	return ctx
}

// profilePath is the console account's profile folder (ProfileList).
func profilePath(sid string) string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\`+sid, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return ""
	}
	defer k.Close()
	s, _, err := k.GetStringValue("ProfileImagePath")
	if err != nil {
		return ""
	}
	return expandForUser(s, "")
}

// expandForUser expands %USERPROFILE% (and the machine-wide variables) for
// another account, whose environment this process does not have.
func expandForUser(s, profile string) string {
	if profile != "" {
		for {
			i := strings.Index(strings.ToUpper(s), "%USERPROFILE%")
			if i < 0 {
				break
			}
			s = s[:i] + profile + s[i+len("%USERPROFILE%"):]
		}
	}
	if strings.Contains(s, "%") {
		if e, err := registry.ExpandString(s); err == nil {
			s = e
		}
	}
	return s
}

// userShellFolder reads one entry of the console account's User Shell
// Folders (where Windows really keeps Desktop, Documents, AppData...),
// falling back to the given default under the profile.
func userShellFolder(u userContext, name, def string) string {
	k, err := registry.OpenKey(registry.USERS, u.SID+`\Software\Microsoft\Windows\CurrentVersion\Explorer\User Shell Folders`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err == nil {
		defer k.Close()
		if s, _, err := k.GetStringValue(name); err == nil && s != "" {
			return filepath.Clean(expandForUser(s, u.Profile))
		}
	}
	if def == "" {
		return ""
	}
	return filepath.Join(u.Profile, def)
}

// userEnv reads one variable of the console account's own environment
// (HKU\<SID>\Environment), expanded for that account.
func userEnv(u userContext, name string) string {
	k, err := registry.OpenKey(registry.USERS, u.SID+`\Environment`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	s, _, err := k.GetStringValue(name)
	if err != nil || s == "" {
		return ""
	}
	return expandForUser(s, u.Profile)
}

// hkcuRoot returns the hive and subkey prefix that HKEY_CURRENT_USER stands
// for: the process account's hive, or HKEY_USERS\<SID> of the console
// account when Axion was elevated with another account.
func hkcuRoot() (registry.Key, string) {
	if u := consoleUser(); u.Differs && u.SID != "" {
		return registry.USERS, u.SID
	}
	return registry.CURRENT_USER, ""
}

// UserPaths fills the per-user members of Paths for the console account
// when it differs from the process account, so cleanup, startup entries
// and program detection look at the right profile.
func userPaths(p *sys.Paths) {
	u := consoleUser()
	p.User = u.Name
	if !u.Differs || u.Profile == "" {
		return
	}
	p.ElevatedAs = u.ProcessName
	p.UserProfile = u.Profile
	p.AppData = userShellFolder(u, "AppData", `AppData\Roaming`)
	p.LocalAppData = userShellFolder(u, "Local AppData", `AppData\Local`)
	p.StartupUser = userShellFolder(u, "Startup", `AppData\Roaming\Microsoft\Windows\Start Menu\Programs\Startup`)
	p.Personal = nil
	for _, f := range []struct{ name, def string }{
		{"Desktop", "Desktop"}, {"Personal", "Documents"}, {"{374DE290-123F-4565-9164-39C4925E467B}", "Downloads"},
		{"My Pictures", "Pictures"}, {"My Music", "Music"}, {"My Video", "Videos"},
		{"{4C5C32FF-BB9D-43B0-B5B4-2D72E54EAAA4}", `Saved Games`},
	} {
		if d := userShellFolder(u, f.name, f.def); d != "" {
			p.Personal = append(p.Personal, d)
		}
	}
	if k, err := registry.OpenKey(registry.USERS, u.SID+`\Software\Microsoft\OneDrive`, registry.QUERY_VALUE); err == nil {
		if s, _, err := k.GetStringValue("UserFolder"); err == nil && s != "" {
			p.Personal = append(p.Personal, s)
		}
		k.Close()
	}
	// The account's own TEMP (its environment), else the standard folder.
	tmp := userEnv(u, "TEMP")
	if tmp == "" {
		tmp = userEnv(u, "TMP")
	}
	switch strings.ToLower(filepath.Base(filepath.Clean(tmp))) {
	case "temp", "tmp":
		p.UserTemp = filepath.Clean(tmp)
	default:
		p.UserTemp = filepath.Join(p.LocalAppData, "Temp")
	}
	if fi, err := os.Stat(p.UserProfile); err != nil || !fi.IsDir() {
		log.Printf("pasta de perfil do usuário do console não encontrada: %s", p.UserProfile)
	}
}

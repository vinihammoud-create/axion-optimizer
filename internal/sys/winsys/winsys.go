//go:build windows

package winsys

import (
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"

	"axion/internal/netprobe"
	"axion/internal/sys"
)

// New builds the Windows platform. dataDir defaults to %ProgramData%\Axion.
func New(dataDir string) (*sys.Platform, error) {
	if dataDir == "" {
		base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0) // not %ProgramData% (user-settable)
		if err != nil || base == "" {
			base = `C:\ProgramData`
		}
		dataDir = filepath.Join(base, "Axion")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	key, err := RecordKey(dataDir)
	if err != nil {
		return nil, err
	}
	com := &COM{}
	reg := Registry{}
	w := &WMI{COM: com, Timeout: 30 * time.Second}
	n := &Native{WMI: w, Reg: reg}
	EnablePrivileges("SeProfileSingleProcessPrivilege", "SeIncreaseQuotaPrivilege", "SeShutdownPrivilege", "SeBackupPrivilege", "SeRestorePrivilege")
	return &sys.Platform{
		Name:    "windows",
		Reg:     reg,
		Svc:     Services{},
		Power:   Power{Reg: reg},
		Run:     Runner{},
		Native:  n,
		WMI:     w,
		Tasks:   &Tasks{COM: com},
		Events:  EventLog{},
		Updates: &WindowsUpdate{COM: &COM{}},
		Net:     netprobe.New(n.Monotonic),
		DataDir: dataDir,
		Key:     key,
	}, nil
}

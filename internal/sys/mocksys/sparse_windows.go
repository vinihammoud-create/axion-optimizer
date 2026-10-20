//go:build windows

package mocksys

import (
	"os"
	"syscall"
)

// fsctlSetSparse is FSCTL_SET_SPARSE.
const fsctlSetSparse = 0x000900C4

// markSparse makes the file sparse on NTFS, so a seeded file of many GB does
// not need that much free space on the test disk. Best effort.
func markSparse(f *os.File) {
	var n uint32
	_ = syscall.DeviceIoControl(syscall.Handle(f.Fd()), fsctlSetSparse, nil, 0, nil, 0, &n, nil)
}

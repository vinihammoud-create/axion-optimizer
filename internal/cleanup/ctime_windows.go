//go:build windows

package cleanup

import (
	"io/fs"
	"syscall"
	"time"
)

// createdAt returns the file's creation time (NTFS keeps it apart from the
// modification time).
func createdAt(info fs.FileInfo) time.Time {
	if d, ok := info.Sys().(*syscall.Win32FileAttributeData); ok && d != nil {
		return time.Unix(0, d.CreationTime.Nanoseconds())
	}
	return time.Time{}
}

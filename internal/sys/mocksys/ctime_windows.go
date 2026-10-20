//go:build windows

package mocksys

import (
	"syscall"
	"time"
)

// backdateCreation moves a file's or folder's creation time back to when.
// NTFS keeps it apart from the modification time, os.Chtimes cannot change
// it, and the cleanup treats anything created in the last day as fresh.
func backdateCreation(path string, when time.Time) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	h, err := syscall.CreateFile(p, syscall.FILE_WRITE_ATTRIBUTES, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return
	}
	defer syscall.CloseHandle(h)
	ft := syscall.NsecToFiletime(when.UnixNano())
	_ = syscall.SetFileTime(h, &ft, &ft, &ft)
}

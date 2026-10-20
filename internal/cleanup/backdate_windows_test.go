//go:build windows

package cleanup

import (
	"syscall"
	"testing"
	"time"
)

// backdateCreation moves the creation time back too: the cleanup keeps
// anything created in the last day, and os.Chtimes cannot change it.
func backdateCreation(t *testing.T, path string, when time.Time) {
	t.Helper()
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := syscall.CreateFile(p, syscall.FILE_WRITE_ATTRIBUTES, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(h)
	ft := syscall.NsecToFiletime(when.UnixNano())
	if err := syscall.SetFileTime(h, &ft, &ft, &ft); err != nil {
		t.Fatal(err)
	}
}

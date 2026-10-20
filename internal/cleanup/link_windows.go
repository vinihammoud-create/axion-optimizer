//go:build windows

package cleanup

import (
	"io/fs"
	"syscall"
)

// isLink reports symbolic links, junctions and other reparse points (cloud
// placeholders included): they are never followed nor deleted.
func isLink(_ string, info fs.FileInfo) bool {
	if info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
		return true
	}
	if a, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		return a.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
	}
	return false
}

//go:build !windows

package cleanup

import (
	"io/fs"
	"time"
)

// createdAt is unknown outside Windows (the inode change time is not a
// creation time); only the modification time is used.
func createdAt(fs.FileInfo) time.Time { return time.Time{} }

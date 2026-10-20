//go:build !windows

package mocksys

import "os"

// markSparse is a no-op where Truncate already creates sparse files.
func markSparse(*os.File) {}

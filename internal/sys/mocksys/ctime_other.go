//go:build !windows

package mocksys

import "time"

// backdateCreation is a no-op where the creation time follows os.Chtimes.
func backdateCreation(string, time.Time) {}

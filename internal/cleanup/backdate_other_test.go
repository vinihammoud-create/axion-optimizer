//go:build !windows

package cleanup

import (
	"testing"
	"time"
)

// backdateCreation is a no-op where the creation time follows os.Chtimes.
func backdateCreation(*testing.T, string, time.Time) {}

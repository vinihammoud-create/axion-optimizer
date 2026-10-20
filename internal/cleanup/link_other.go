//go:build !windows

package cleanup

import "io/fs"

// isLink reports symbolic links, which are never followed nor deleted.
func isLink(_ string, info fs.FileInfo) bool {
	return info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0
}

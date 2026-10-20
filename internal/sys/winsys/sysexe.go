//go:build windows

package winsys

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// systemExe resolves a bare program name ("fsutil.exe", "msiexec.exe") to
// the copy in the System32 folder, or else in the Windows folder, and never
// through PATH: Axion runs as administrator, and a folder that comes earlier
// in PATH and that a program without administrator rights can write to
// (the per-user WindowsApps folder is one by default) could hold a planted
// "fsutil.exe". Absolute paths are returned as they are; other relative
// paths are refused.
func systemExe(name string) (string, error) {
	name = strings.TrimSpace(name)
	if filepath.IsAbs(name) {
		return name, nil
	}
	if name == "" || strings.ContainsAny(name, `\/:`) {
		return "", fmt.Errorf("caminho de programa não aceito: %q", name)
	}
	var dirs []string
	if d, err := windows.GetSystemDirectory(); err == nil {
		dirs = append(dirs, d)
	}
	if d, err := windows.GetWindowsDirectory(); err == nil {
		dirs = append(dirs, d)
	}
	cands := []string{name}
	if !strings.HasSuffix(strings.ToLower(name), ".exe") {
		cands = append(cands, name+".exe")
	}
	for _, d := range dirs {
		for _, c := range cands {
			p := filepath.Join(d, c)
			if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("%s não foi encontrado na pasta do Windows", name)
}

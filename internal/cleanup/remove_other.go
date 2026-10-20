//go:build !windows

package cleanup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type remover struct{}

func newRemover() *remover { return &remover{} }

var errOutside = errors.New("o caminho aponta para fora da pasta limpa; ignorado")

// remove deletes path (a file or an empty folder) if it is inside root.
func (r *remover) remove(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return errOutside
	}
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() && info.Mode()&0o200 == 0 {
		_ = os.Chmod(path, 0o666)
	}
	return os.Remove(path)
}

//go:build windows

package cleanup

import (
	"errors"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// remover deletes files and empty folders through a handle, after checking
// where the handle really points. The listing and the delete happen at
// different moments; a program running as the user could swap a folder of
// its TEMP for a junction in between, and an elevated delete by path would
// then remove a system file. Opening the item (without following a final
// link), reading its real path from the handle and deleting through that
// same handle leaves no such window.
type remover struct {
	roots map[string]string // root -> its real path (no "\\?\" prefix)
}

func newRemover() *remover { return &remover{roots: map[string]string{}} }

var errOutside = errors.New("o caminho aponta para fora da pasta limpa (link trocado durante a limpeza); ignorado")

func finalPath(h windows.Handle) (string, error) {
	buf := make([]uint16, 512)
	for {
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
		if err != nil {
			return "", err
		}
		if int(n) < len(buf) {
			s := windows.UTF16ToString(buf[:n])
			s = strings.TrimPrefix(s, `\\?\UNC\`)
			return strings.TrimPrefix(s, `\\?\`), nil
		}
		buf = make([]uint16, n+1)
	}
}

func openNoFollow(path string, access uint32) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	return windows.CreateFile(p, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
}

func (r *remover) rootOf(root string) (string, error) {
	if f, ok := r.roots[root]; ok {
		return f, nil
	}
	h, err := openNoFollow(root, windows.FILE_READ_ATTRIBUTES)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	f, err := finalPath(h)
	if err != nil {
		return "", err
	}
	r.roots[root] = f
	return f, nil
}

// fileDispositionInfoEx flags (Windows 10 1809+ on NTFS).
const (
	fileDispositionDelete         = 0x1
	fileDispositionPosixSemantics = 0x2
	fileDispositionIgnoreReadOnly = 0x10
	fileDispositionInfoClass      = 4 // FileDispositionInfo
	fileDispositionInfoExClass    = 21
)

// remove deletes path (a file or an empty folder) if it is inside root.
func (r *remover) remove(root, path string) error {
	rootReal, err := r.rootOf(root)
	if err != nil {
		return err
	}
	h, err := openNoFollow(path, windows.DELETE|windows.FILE_READ_ATTRIBUTES)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	real, err := finalPath(h)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(strings.ToLower(real), strings.ToLower(strings.TrimRight(rootReal, `\`))+`\`) {
		return errOutside
	}
	flags := uint32(fileDispositionDelete | fileDispositionPosixSemantics | fileDispositionIgnoreReadOnly)
	err = windows.SetFileInformationByHandle(h, fileDispositionInfoExClass, (*byte)(unsafe.Pointer(&flags)), uint32(unsafe.Sizeof(flags)))
	if err == nil {
		return nil
	}
	// File systems without the extended form (FAT, older builds): the
	// classic delete-on-close, which refuses read-only files.
	del := byte(1)
	return windows.SetFileInformationByHandle(h, fileDispositionInfoClass, &del, 1)
}

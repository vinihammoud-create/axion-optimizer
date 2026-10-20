//go:build windows

package winsys

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// dataDirSDDL protects Axion's data folder: SYSTEM and elevated
// administrators have full control; the OWNER RIGHTS entry removes the
// implicit WRITE_DAC of a file's owner. Without it, a program running in the
// user's non-elevated session (same user SID as the owner of files created by
// the elevated Axion) could edit journal.json and make Axion "restore"
// registry values of its choosing with administrator rights, or swap a
// downloaded installer before Axion launches it.
const dataDirSDDL = "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1200a9;;;OW)"

// auditMarker marks a data folder whose contents were already checked for
// things other accounts put there before it was protected (done once).
const auditMarker = ".protegida"

// asideTag is appended to what is moved out of the way; Axion never reads
// those names again.
const asideTag = ".nao-confiavel-"

func notFound(err error) bool {
	return errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)
}

// trustedOwner accepts SYSTEM, the Administrators group and the account
// running Axion (elevated). Anything owned by another account was put there
// by someone else.
func trustedOwner(sid *windows.SID) bool {
	if sid == nil {
		return false
	}
	if sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) || sid.IsWellKnown(windows.WinLocalSystemSid) {
		return true
	}
	if tu, err := windows.GetCurrentProcessToken().GetTokenUser(); err == nil && tu.User.Sid != nil {
		return sid.Equals(tu.User.Sid)
	}
	return false
}

// adminOwned reports files owned by the Administrators group or SYSTEM.
// A program running without administrator rights cannot create such a file
// (in its token the Administrators group is deny-only), so this proves that
// an elevated process wrote it.
func adminOwned(path string) bool {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	owner, _, err := sd.Owner()
	return err == nil && owner != nil && (owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) || owner.IsWellKnown(windows.WinLocalSystemSid))
}

// createAdminFile creates a new file owned by the Administrators group, with
// the folder's (protected) inherited permissions.
func createAdminFile(path string, data []byte) error {
	sd, err := windows.SecurityDescriptorFromString("O:BA")
	if err != nil {
		return err
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE, 0, &sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return fmt.Errorf("criar %s: %w", path, err)
	}
	var n uint32
	if len(data) > 0 {
		err = windows.WriteFile(h, data, &n, nil)
	}
	if cerr := windows.CloseHandle(h); err == nil {
		err = cerr
	}
	if err == nil && int(n) != len(data) {
		err = errors.New("gravação incompleta")
	}
	if err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("gravar %s: %w", path, err)
	}
	return nil
}

// RecordKey loads (or creates) the key that authenticates Axion's own
// records. It is a file owned by the Administrators group inside the
// protected folder: programs without administrator rights can neither read
// it nor plant one. A key file with any other owner was not created by Axion
// and is replaced (records signed with it stop being trusted).
func RecordKey(dir string) ([]byte, error) {
	path := filepath.Join(dir, ".chave")
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode().IsRegular() && adminOwned(path) {
			if b, err := os.ReadFile(path); err == nil && len(b) == 32 {
				return b, nil
			}
		}
		if err := moveAside(path); err != nil {
			return nil, fmt.Errorf("chave dos registros do Axion: %w", err)
		}
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := createAdminFile(path, key); err != nil {
		return nil, fmt.Errorf("chave dos registros do Axion: %w", err)
	}
	return key, nil
}

// openDirNoFollow opens the folder entry itself, never the target of a link
// put in its place, with the rights to read and change its permissions and
// owner.
func openDirNoFollow(dir string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return windows.InvalidHandle, err
	}
	return windows.CreateFile(p, windows.READ_CONTROL|windows.WRITE_DAC|windows.WRITE_OWNER|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
}

// createProtectedDir creates the folder already protected and owned by the
// Administrators group, so there is no moment in which it is open to others.
func createProtectedDir(dir string) error {
	sd, err := windows.SecurityDescriptorFromString("O:BA" + dataDirSDDL)
	if err != nil {
		return err
	}
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	return windows.CreateDirectory(p, &sa)
}

func moveAside(path string) error {
	return os.Rename(path, path+asideTag+time.Now().Format("20060102-150405.000"))
}

type dirState int

const (
	dirMissing   dirState = iota
	dirNotFolder          // a link, junction or file where the folder should be
	dirUntrusted          // a folder created by another account
	dirProtected
)

// inspectAndProtect checks the folder through a handle that does not follow
// links and, when it is a real folder created by an administrator, applies
// the protected permissions through that same handle: a junction swapped in
// at the last moment cannot redirect the change.
func inspectAndProtect(dir string, dacl *windows.ACL) (dirState, error) {
	h, err := openDirNoFollow(dir)
	switch {
	case err != nil && notFound(err):
		return dirMissing, nil
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		// Administrators can always manage a folder Axion created.
		return dirUntrusted, nil
	case err != nil:
		return 0, fmt.Errorf("abrir %s: %w", dir, err)
	}
	defer windows.CloseHandle(h)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return 0, fmt.Errorf("ler %s: %w", dir, err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return dirNotFolder, nil
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return 0, fmt.Errorf("ler o dono de %s: %w", dir, err)
	}
	if owner, _, err := sd.Owner(); err != nil || !trustedOwner(owner) {
		return dirUntrusted, nil
	}
	// The folder is also handed to the Administrators group, so any
	// administrator account recognizes it later.
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return 0, err
	}
	if err := windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, admins, nil, dacl, nil); err != nil {
		return 0, fmt.Errorf("proteger %s: %w", dir, err)
	}
	return dirProtected, nil
}

// PrepareDataDir creates (or repairs) the data folder so that only SYSTEM
// and administrators can change anything in it. A link or file planted where
// the folder should be is removed and a folder created by another account is
// moved aside, then the check starts over; if the folder keeps changing the
// preparation fails and Axion must not use it. warn reports problems with
// items inside the folder that do not make it unsafe.
func PrepareDataDir(dir string) (warn error, err error) {
	sd, err := windows.SecurityDescriptorFromString(dataDirSDDL)
	if err != nil {
		return nil, err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 6; attempt++ {
		st, err := inspectAndProtect(dir, dacl)
		if err != nil {
			return nil, err
		}
		switch st {
		case dirProtected:
			return hardenContents(dir), nil
		case dirMissing:
			if err := createProtectedDir(dir); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
				return nil, fmt.Errorf("criar %s: %w", dir, err)
			}
		case dirNotFolder:
			if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("%s não é uma pasta comum e não pôde ser removido: %w", dir, err)
			}
		case dirUntrusted:
			if err := moveAside(dir); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("%s foi criada por outra conta e não pôde ser afastada: %w", dir, err)
			}
		}
	}
	return nil, fmt.Errorf("%s mudou várias vezes durante a preparação; por segurança o Axion não vai usá-la", dir)
}

// HardenDir protects a folder inside the (already protected) data folder,
// e.g. the quarantine, and resets what is inside it.
func HardenDir(dir string) error {
	sd, err := windows.SecurityDescriptorFromString(dataDirSDDL)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	st, err := inspectAndProtect(dir, dacl)
	if err != nil {
		return err
	}
	if st != dirProtected {
		return fmt.Errorf("%s não é uma pasta confiável", dir)
	}
	return resetContents(dir, false)
}

// hardenContents removes links inside the data folder and resets every
// item to the inherited (protected) permissions. The first time the folder
// is protected, what another account put there is moved aside, and a marker
// owned by the Administrators group records that this was done.
func hardenContents(dir string) error {
	marker := filepath.Join(dir, auditMarker)
	audited := trustedFile(marker)
	err := resetContents(dir, !audited)
	if !audited && err == nil {
		err = writeMarker(marker)
	}
	return err
}

// emptyACL returns a DACL with no entries (not a NULL DACL, which would
// grant everyone full access). windows.ACLFromEntries(nil, nil) must not be
// used for this: with no entries SetEntriesInAcl returns a NULL ACL and the
// wrapper dereferences it.
func emptyACL() (*windows.ACL, error) {
	sd, err := windows.SecurityDescriptorFromString("D:")
	if err != nil {
		return nil, err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return nil, err
	}
	if acl == nil {
		return nil, errors.New("DACL vazia indisponível")
	}
	return acl, nil
}

func resetContents(dir string, audit bool) error {
	empty, err := emptyACL()
	if err != nil {
		return err
	}
	var firstErr error
	note := func(e error) {
		if e != nil && firstErr == nil {
			firstErr = e
		}
	}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == dir {
			return nil
		}
		if strings.Contains(d.Name(), asideTag) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() && quarantineData(p) {
			// Programs' folders moved to the quarantine keep what they had
			// (permissions, links, compressed or cloud files) so a restore
			// puts them back exactly; Axion only ever moves them.
			return filepath.SkipDir
		}
		if d.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 || reparseAttr(p) {
			note(os.Remove(p))
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if audit && !ownedByTrusted(p) {
			// Fail closed: an item whose owner cannot even be read (a
			// hostile ACL) is moved aside too.
			note(moveAside(p))
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// Empty explicit DACL + UNPROTECTED: only the inherited entries remain.
		if e := windows.SetNamedSecurityInfo(p, windows.SE_FILE_OBJECT,
			windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION, nil, nil, empty, nil); e != nil {
			note(fmt.Errorf("proteger %s: %w", p, e))
		}
		return nil
	})
	return firstErr
}

// quarantineData reports a batch's data folder (...\quarentena\<lote>\data).
func quarantineData(p string) bool {
	return strings.EqualFold(filepath.Base(p), "data") && strings.EqualFold(filepath.Base(filepath.Dir(filepath.Dir(p))), "quarentena")
}

// reparseAttr is the lenient check used while walking: an item that cannot
// be inspected is left alone (it is not deleted on a transient error).
func reparseAttr(path string) bool {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attrs, err := windows.GetFileAttributes(p)
	return err == nil && attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

func ownedByTrusted(path string) bool {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	owner, _, err := sd.Owner()
	return err == nil && trustedOwner(owner)
}

// trustedFile: a regular file created by an elevated process (see adminOwned).
func trustedFile(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode().IsRegular() && adminOwned(path)
}

// writeMarker creates the audit marker owned by the Administrators group
// (with the folder's inherited permissions).
func writeMarker(path string) error {
	if _, err := os.Lstat(path); err == nil {
		if err := moveAside(path); err != nil {
			return err
		}
	}
	return createAdminFile(path, nil)
}

// Reinherit implements sys.Native. It works through a handle that does not
// follow links (like inspectAndProtect): a junction swapped in for the
// restored folder cannot redirect the change. Setting the DACL with the
// "unprotected" flag makes Windows merge the parent's inheritable entries
// and propagate them to everything below, replacing the ones inherited
// from the quarantine folder; explicit entries stay.
func (n *Native) Reinherit(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(p, windows.READ_CONTROL|windows.WRITE_DAC|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return fmt.Errorf("abrir %s: %w", path, err)
	}
	defer windows.CloseHandle(h)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("%s virou um link; permissões não alteradas", path)
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("ler permissões de %s: %w", path, err)
	}
	control, _, err := sd.Control()
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	explicit, err := explicitOnly(dacl)
	if err != nil {
		return err
	}
	flags := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION)
	if control&windows.SE_DACL_PROTECTED != 0 {
		flags |= windows.PROTECTED_DACL_SECURITY_INFORMATION
	} else {
		flags |= windows.UNPROTECTED_DACL_SECURITY_INFORMATION
	}
	if err := windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, flags, nil, nil, explicit, nil); err != nil {
		return fmt.Errorf("reaplicar permissões herdadas em %s: %w", path, err)
	}
	return nil
}

// explicitOnly copies an ACL keeping only the entries that were not
// inherited (their order, canonical in the original, is kept).
func explicitOnly(acl *windows.ACL) (*windows.ACL, error) {
	if acl == nil {
		return emptyACL()
	}
	var aces [][]byte
	size := 8 // ACL header
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			return nil, err
		}
		if ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
			continue
		}
		raw := unsafe.Slice((*byte)(unsafe.Pointer(ace)), int(ace.Header.AceSize))
		aces = append(aces, append([]byte(nil), raw...))
		size += int(ace.Header.AceSize)
	}
	if len(aces) == 0 {
		return emptyACL()
	}
	buf := make([]uint32, (size+3)/4) // 4-byte aligned
	b := unsafe.Slice((*byte)(unsafe.Pointer(&buf[0])), size)
	b[0] = *(*byte)(unsafe.Pointer(acl)) // same revision as the original
	b[2], b[3] = byte(size), byte(size>>8)
	b[4], b[5] = byte(len(aces)), byte(len(aces)>>8)
	off := 8
	for _, a := range aces {
		copy(b[off:], a)
		off += len(a)
	}
	return (*windows.ACL)(unsafe.Pointer(&buf[0])), nil
}

//go:build windows

// Package winsys implements the sys interfaces on Windows using native APIs
// (Registry, Service Control Manager, powrprof, WMI/COM, Task Scheduler COM,
// wevtapi, iphlpapi, user32, ntdll). No PowerShell or batch is used.
package winsys

import (
	"errors"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"axion/internal/sys"
)

// Registry implements sys.Registry on the native (64-bit) view.
type Registry struct{}

// rootKey maps a hive to its predefined key. HKCU stands for the console
// account's hive (see user.go): when Axion was elevated with another
// account, it becomes HKEY_USERS\<SID> and the SID is returned as the prefix
// to put in front of the subkey.
func rootKey(r sys.RegRoot) (registry.Key, string, error) {
	switch r {
	case sys.RootHKLM:
		return registry.LOCAL_MACHINE, "", nil
	case sys.RootHKCU:
		k, prefix := hkcuRoot()
		return k, prefix, nil
	case sys.RootHKU:
		return registry.USERS, "", nil
	case sys.RootHKCR:
		return registry.CLASSES_ROOT, "", nil
	}
	return 0, "", errors.New("hive desconhecida")
}

// splitPath resolves a registry path into the predefined key to open and
// the subkey under it (already redirected for HKCU).
func splitPath(path string) (registry.Key, string, error) {
	root, sub, err := sys.SplitRegPath(path)
	if err != nil {
		return 0, "", err
	}
	rk, prefix, err := rootKey(root)
	if err != nil {
		return 0, "", err
	}
	if prefix != "" {
		if sub == "" {
			sub = prefix
		} else {
			sub = prefix + `\` + sub
		}
	}
	return rk, sub, nil
}

func open(path string, access uint32) (registry.Key, error) {
	rk, sub, err := splitPath(path)
	if err != nil {
		return 0, err
	}
	if sub == "" {
		return rk, nil
	}
	return registry.OpenKey(rk, sub, access|registry.WOW64_64KEY)
}

func isNotExist(err error) bool {
	return errors.Is(err, registry.ErrNotExist) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)
}

func utf16BytesToString(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	u := unsafe.Slice((*uint16)(unsafe.Pointer(&b[0])), len(b)/2)
	for i, c := range u {
		if c == 0 {
			u = u[:i]
			break
		}
	}
	return string(utf16.Decode(u))
}

// multiSZ splits REG_MULTI_SZ data. Empty strings in the middle are kept
// (PendingFileRenameOperations uses them for "delete" and a value copied
// for quarantine must come back identical); only the terminating empty
// strings at the end are dropped.
func multiSZ(b []byte) []string {
	return splitMultiSZ(unsafe.Slice((*uint16)(unsafe.Pointer(unsafe.SliceData(b))), len(b)/2))
}

// GetValue implements sys.Registry.
func (Registry) GetValue(path, name string) (sys.RegValue, error) {
	k, err := open(path, registry.QUERY_VALUE)
	if err != nil {
		if isNotExist(err) {
			return sys.Absent(), nil
		}
		return sys.RegValue{}, err
	}
	defer k.Close()
	n, typ, err := k.GetValue(name, nil)
	if err != nil && !errors.Is(err, registry.ErrShortBuffer) {
		if isNotExist(err) {
			return sys.Absent(), nil
		}
		return sys.RegValue{}, err
	}
	buf := make([]byte, n+2)
	n, typ, err = k.GetValue(name, buf)
	if err != nil {
		if isNotExist(err) {
			return sys.Absent(), nil
		}
		return sys.RegValue{}, err
	}
	buf = buf[:n]
	v := sys.RegValue{Exists: true}
	switch typ {
	case registry.SZ:
		v.Kind, v.Str = sys.RegSZ, utf16BytesToString(buf)
	case registry.EXPAND_SZ:
		v.Kind, v.Str = sys.RegExpandSZ, utf16BytesToString(buf)
	case registry.MULTI_SZ:
		v.Kind, v.Strs = sys.RegMultiSZ, multiSZ(buf)
	case registry.DWORD:
		v.Kind = sys.RegDWord
		if len(buf) >= 4 {
			v.Int = uint64(uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24)
		}
	case registry.QWORD:
		v.Kind = sys.RegQWord
		for i := 0; i < 8 && i < len(buf); i++ {
			v.Int |= uint64(buf[i]) << (8 * i)
		}
	default:
		v.Kind, v.Bin = sys.RegBinary, append([]byte(nil), buf...)
	}
	return v, nil
}

// SetValue implements sys.Registry.
func (Registry) SetValue(path, name string, v sys.RegValue) error {
	rk, sub, err := splitPath(path)
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(rk, sub, registry.SET_VALUE|registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return err
	}
	defer k.Close()
	switch v.Kind {
	case sys.RegDWord:
		return k.SetDWordValue(name, uint32(v.Int))
	case sys.RegQWord:
		return k.SetQWordValue(name, v.Int)
	case sys.RegSZ:
		return k.SetStringValue(name, v.Str)
	case sys.RegExpandSZ:
		return k.SetExpandStringValue(name, v.Str)
	case sys.RegMultiSZ:
		return k.SetStringsValue(name, v.Strs)
	case sys.RegBinary:
		return k.SetBinaryValue(name, v.Bin)
	}
	return errors.New("tipo de valor não suportado")
}

// DeleteValue implements sys.Registry.
func (Registry) DeleteValue(path, name string) error {
	k, err := open(path, registry.SET_VALUE)
	if err != nil {
		if isNotExist(err) {
			return nil
		}
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(name); err != nil && !isNotExist(err) {
		return err
	}
	return nil
}

// KeyExists implements sys.Registry.
func (Registry) KeyExists(path string) (bool, error) {
	k, err := open(path, registry.QUERY_VALUE)
	if err != nil {
		if isNotExist(err) {
			return false, nil
		}
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return true, nil
		}
		return false, err
	}
	k.Close()
	return true, nil
}

// CreateKey implements sys.Registry.
func (Registry) CreateKey(path string) (bool, error) {
	rk, sub, err := splitPath(path)
	if err != nil {
		return false, err
	}
	k, existed, err := registry.CreateKey(rk, sub, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return false, err
	}
	k.Close()
	return !existed, nil
}

// DeleteKeyIfEmpty implements sys.Registry.
func (Registry) DeleteKeyIfEmpty(path string) (bool, error) {
	k, err := open(path, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		if isNotExist(err) {
			return false, nil
		}
		return false, err
	}
	st, err := k.Stat()
	k.Close()
	if err != nil {
		return false, err
	}
	if st.SubKeyCount > 0 || st.ValueCount > 0 {
		return false, nil
	}
	i := strings.LastIndex(path, `\`)
	if i < 0 {
		return false, nil
	}
	parent, err := open(path[:i], registry.ENUMERATE_SUB_KEYS|registry.SET_VALUE)
	if err != nil {
		return false, err
	}
	defer parent.Close()
	if err := registry.DeleteKey(parent, path[i+1:]); err != nil {
		if isNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// SubKeys implements sys.Registry.
func (Registry) SubKeys(path string) ([]string, error) {
	k, err := open(path, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		if isNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer k.Close()
	return k.ReadSubKeyNames(-1)
}

// ValueNames implements sys.Registry.
func (Registry) ValueNames(path string) ([]string, error) {
	k, err := open(path, registry.QUERY_VALUE)
	if err != nil {
		if isNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer k.Close()
	return k.ReadValueNames(-1)
}

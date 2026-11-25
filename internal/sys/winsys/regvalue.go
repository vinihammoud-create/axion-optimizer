package winsys

import (
	"encoding/binary"
	"unicode/utf16"
	"unsafe"

	"axion/internal/sys"
)

// Registry value types, as numbered by Windows.
const (
	regTypeSZ       = 1
	regTypeExpandSZ = 2
	regTypeBinary   = 3
	regTypeDWord    = 4
	regTypeMultiSZ  = 7
	regTypeQWord    = 11
)

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

// stringBytes is the REG_SZ data Windows tools write for s: UTF-16LE plus one
// terminating NUL.
func stringBytes(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 0, 2*len(u)+2)
	for _, c := range u {
		b = binary.LittleEndian.AppendUint16(b, c)
	}
	return append(b, 0, 0)
}

// decodeRegValue turns the raw type and data of a registry value into a
// sys.RegValue that writes back the same type and the same bytes.
func decodeRegValue(typ uint32, buf []byte) sys.RegValue {
	v := sys.RegValue{Exists: true}
	switch typ {
	case regTypeSZ, regTypeExpandSZ:
		v.Kind = sys.RegSZ
		if typ == regTypeExpandSZ {
			v.Kind = sys.RegExpandSZ
		}
		v.Str = utf16BytesToString(buf)
		if string(stringBytes(v.Str)) != string(buf) {
			v.Bin = append([]byte(nil), buf...)
		}
	case regTypeMultiSZ:
		v.Kind, v.Strs = sys.RegMultiSZ, multiSZ(buf)
	case regTypeDWord:
		v.Kind = sys.RegDWord
		if len(buf) >= 4 {
			v.Int = uint64(uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24)
		}
	case regTypeQWord:
		v.Kind = sys.RegQWord
		for i := 0; i < 8 && i < len(buf); i++ {
			v.Int |= uint64(buf[i]) << (8 * i)
		}
	case regTypeBinary:
		v.Kind, v.Bin = sys.RegBinary, append([]byte(nil), buf...)
	default:
		v.Kind, v.Type, v.Bin = sys.RegNone, typ, append([]byte(nil), buf...)
	}
	return v
}

// rawRegValue returns the exact type and bytes to write for the values that
// the typed setters cannot reproduce: types without their own Kind, and
// strings whose stored bytes are more than Str. ok is false for the rest.
func rawRegValue(v sys.RegValue) (typ uint32, data []byte, ok bool) {
	switch v.Kind {
	case sys.RegNone:
		return v.Type, v.Bin, true
	case sys.RegSZ, sys.RegExpandSZ:
		if v.Bin == nil {
			return 0, nil, false
		}
		typ = regTypeSZ
		if v.Kind == sys.RegExpandSZ {
			typ = regTypeExpandSZ
		}
		return typ, v.Bin, true
	}
	return 0, nil, false
}

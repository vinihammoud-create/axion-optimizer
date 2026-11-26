package winsys

import (
	"bytes"
	"testing"

	"axion/internal/sys"
)

// Whatever decodeRegValue reads, rawRegValue (or, for the common types, the
// typed setters' data) must write back with the same type and bytes.
func TestRawTypesRoundTrip(t *testing.T) {
	for _, typ := range []uint32{0, 5, 6, 8, 9, 10, 0x7fff0001} {
		data := []byte{0x01, 0x00, 0xFF, 0x00, 0x80}
		v := decodeRegValue(typ, data)
		if v.Kind != sys.RegNone || v.Type != typ {
			t.Fatalf("tipo %d: lido como %v/%d", typ, v.Kind, v.Type)
		}
		gt, gd, ok := rawRegValue(v)
		if !ok || gt != typ || !bytes.Equal(gd, data) {
			t.Errorf("tipo %d: regravaria tipo %d, %x (ok=%v)", typ, gt, gd, ok)
		}
	}
}

func TestNonCanonicalStringRoundTrip(t *testing.T) {
	canonical := stringBytes("abc")
	if v := decodeRegValue(regTypeSZ, canonical); v.Bin != nil || v.Str != "abc" {
		t.Errorf("REG_SZ comum não deveria guardar bytes crus: %+v", v)
	}
	if _, _, ok := rawRegValue(decodeRegValue(regTypeSZ, canonical)); ok {
		t.Error("REG_SZ comum deve ir pelo gravador de texto")
	}
	cases := []struct {
		name string
		typ  uint32
		data []byte
	}{
		{"dados depois do NUL", regTypeSZ, append(append([]byte(nil), canonical...), 'x', 0, 0, 0)},
		{"sem terminador", regTypeExpandSZ, canonical[:len(canonical)-2]},
		{"tamanho ímpar", regTypeSZ, append(append([]byte(nil), canonical...), 0x41)},
	}
	for _, c := range cases {
		v := decodeRegValue(c.typ, c.data)
		gt, gd, ok := rawRegValue(v)
		if !ok || gt != c.typ || !bytes.Equal(gd, c.data) {
			t.Errorf("%s: regravaria tipo %d, %x (ok=%v), esperado tipo %d, %x", c.name, gt, gd, ok, c.typ, c.data)
		}
	}
}

func TestCommonTypesKeepTheirKinds(t *testing.T) {
	cases := []struct {
		typ  uint32
		data []byte
		kind sys.RegKind
	}{
		{regTypeSZ, stringBytes("x"), sys.RegSZ},
		{regTypeExpandSZ, stringBytes("%x%"), sys.RegExpandSZ},
		{regTypeBinary, []byte{1, 2}, sys.RegBinary},
		{regTypeDWord, []byte{1, 0, 0, 0}, sys.RegDWord},
		{regTypeQWord, []byte{1, 0, 0, 0, 0, 0, 0, 0}, sys.RegQWord},
	}
	for _, c := range cases {
		if v := decodeRegValue(c.typ, c.data); v.Kind != c.kind {
			t.Errorf("tipo %d: Kind %v, esperado %v", c.typ, v.Kind, c.kind)
		}
	}
}

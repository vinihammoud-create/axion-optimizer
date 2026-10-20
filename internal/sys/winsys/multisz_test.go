package winsys

import (
	"reflect"
	"testing"
	"unicode/utf16"
)

func TestSplitMultiSZ(t *testing.T) {
	enc := func(s string) []uint16 { return utf16.Encode([]rune(s)) }
	cases := []struct {
		in   string
		want []string
	}{
		{"a\x00b\x00\x00", []string{"a", "b"}},
		// PendingFileRenameOperations: source, "" (delete), source, destination.
		{`\??\C:\t\x.tmp` + "\x00\x00" + `\??\C:\t\y.dll` + "\x00" + `!\??\C:\W\y.dll` + "\x00\x00", []string{`\??\C:\t\x.tmp`, "", `\??\C:\t\y.dll`, `!\??\C:\W\y.dll`}},
		{"\x00", nil},
		{"", nil},
		{"sem-terminador", []string{"sem-terminador"}},
	}
	for _, c := range cases {
		if got := splitMultiSZ(enc(c.in)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: %q, esperado %q", c.in, got, c.want)
		}
	}
}

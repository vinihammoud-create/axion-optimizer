package programs_test

import (
	"encoding/json"
	"testing"

	"axion/internal/programs"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

// A key saved in the quarantine manifest (JSON on disk) and written back
// keeps rare value types and strings with data after the first NUL.
func TestRegTreeKeepsRawTypesThroughManifest(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const key = `HKCU\Software\AxionTreeTest`
	want := map[string]sys.RegValue{
		"none":   {Exists: true, Kind: sys.RegNone, Bin: []byte{0xDE, 0xAD, 0x00, 0xBE}},
		"link":   {Exists: true, Kind: sys.RegNone, Type: 6, Bin: []byte{1, 0, 2, 0}},
		"strNUL": {Exists: true, Kind: sys.RegSZ, Str: "a", Bin: []byte{'a', 0, 0, 0, 'b', 0, 0, 0}},
	}
	for n, v := range want {
		if err := m.R.SetValue(key, n, v); err != nil {
			t.Fatal(err)
		}
	}
	tree, err := programs.ReadTree(m.R, key)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	var back programs.RegTree
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if err := programs.DeleteTree(m.R, tree); err != nil {
		t.Fatal(err)
	}
	if err := programs.WriteTree(m.R, &back); err != nil {
		t.Fatal(err)
	}
	for n, w := range want {
		got, _ := m.R.GetValue(key, n)
		if !got.Equal(w) {
			t.Errorf("%s: %+v, esperado %+v", n, got, w)
		}
	}
}

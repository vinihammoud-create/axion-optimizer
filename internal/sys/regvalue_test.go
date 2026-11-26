package sys

import "testing"

func TestRegValueEqualRawBytes(t *testing.T) {
	a := RegValue{Exists: true, Kind: RegNone, Bin: []byte{1, 2, 3}}
	same := RegValue{Exists: true, Kind: RegNone, Bin: []byte{1, 2, 3}}
	other := RegValue{Exists: true, Kind: RegNone, Bin: []byte{1, 2, 4}}
	if !a.Equal(same) {
		t.Error("REG_NONE com os mesmos bytes deveria ser igual")
	}
	if a.Equal(other) {
		t.Error("REG_NONE com bytes diferentes não pode ser igual")
	}
}

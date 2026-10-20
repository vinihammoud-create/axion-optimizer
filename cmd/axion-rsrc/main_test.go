package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// The object must be what the Go linker accepts and what Windows reads: one
// .rsrc section, one ADDR32NB relocation per data entry, the root directory
// sorted by type and every data entry pointing inside the section.
func TestCOFFLayout(t *testing.T) {
	ico, err := buildICO("../../build/axion-mark.png")
	if err != nil {
		t.Fatal(err)
	}
	if len(ico.images) != len(icoSizes) {
		t.Fatalf("ícone com %d tamanhos", len(ico.images))
	}
	if n := binary.LittleEndian.Uint16(ico.file[4:]); int(n) != len(icoSizes) {
		t.Fatalf("ICONDIR diz %d imagens", n)
	}
	v, err := parseVersion("1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	vi := versionInfo(v, "1.2.3")
	i := bytes.Index(vi, []byte{0xBD, 0x04, 0xEF, 0xFE})
	if i < 0 || binary.LittleEndian.Uint32(vi[i+8:]) != 1<<16|2 || binary.LittleEndian.Uint32(vi[i+12:]) != 3<<16 {
		t.Fatal("VS_FIXEDFILEINFO sem a versão 1.2.3")
	}
	if int(binary.LittleEndian.Uint16(vi)) != len(vi) {
		t.Fatal("wLength do VS_VERSION_INFO não confere")
	}

	obj := buildCOFF(ico, []byte("<assembly/>"), vi)
	if m := binary.LittleEndian.Uint16(obj); m != 0x8664 {
		t.Fatalf("máquina %#x", m)
	}
	if n := binary.LittleEndian.Uint16(obj[2:]); n != 1 {
		t.Fatalf("%d seções", n)
	}
	sh := obj[20:60]
	if !bytes.HasPrefix(sh, []byte(".rsrc\x00")) {
		t.Fatalf("seção %q", sh[:8])
	}
	size := binary.LittleEndian.Uint32(sh[16:])
	raw := binary.LittleEndian.Uint32(sh[20:])
	relOff := binary.LittleEndian.Uint32(sh[24:])
	nrel := int(binary.LittleEndian.Uint16(sh[32:]))
	want := len(icoSizes) + 3 // icons + group + version + manifest
	if nrel != want {
		t.Fatalf("%d realocações, esperado %d", nrel, want)
	}
	sec := obj[raw : raw+size]
	// Root directory: four types in ascending order.
	if n := binary.LittleEndian.Uint16(sec[14:]); n != 4 {
		t.Fatalf("raiz com %d tipos", n)
	}
	prev := uint32(0)
	for k := 0; k < 4; k++ {
		id := binary.LittleEndian.Uint32(sec[16+8*k:])
		if id <= prev {
			t.Fatalf("tipos fora de ordem: %d depois de %d", id, prev)
		}
		prev = id
	}
	for r := 0; r < nrel; r++ {
		e := binary.LittleEndian.Uint32(obj[relOff+uint32(10*r):])
		if typ := binary.LittleEndian.Uint16(obj[relOff+uint32(10*r)+8:]); typ != 3 {
			t.Fatalf("realocação do tipo %d", typ)
		}
		off := binary.LittleEndian.Uint32(sec[e:])
		ln := binary.LittleEndian.Uint32(sec[e+4:])
		if off+ln > size {
			t.Fatalf("entrada %d aponta para fora da seção", r)
		}
	}
}

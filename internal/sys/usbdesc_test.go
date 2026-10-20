package sys

import (
	"encoding/binary"
	"testing"
)

// A two-interface gaming mouse: boot mouse on interface 0, vendor HID
// collection on interface 1 with an alternate setting that must be ignored.
var mouseConfig = []byte{
	9, 0x02, 59, 0, 2, 1, 0, 0xA0, 49, // configuration
	9, 0x04, 0, 0, 1, 3, 1, 2, 0, // interface 0: HID, boot, mouse
	9, 0x21, 0x11, 0x01, 0, 1, 0x22, 67, 0, // HID descriptor
	7, 0x05, 0x81, 0x03, 8, 0, 1, // EP 1 IN interrupt, 8 bytes, bInterval 1
	9, 0x04, 1, 0, 1, 3, 0, 0, 0, // interface 1: HID
	9, 0x21, 0x11, 0x01, 0, 1, 0x22, 23, 0,
	7, 0x05, 0x82, 0x03, 20, 0, 8, // EP 2 IN interrupt, bInterval 8
	9, 0x04, 1, 1, 1, 3, 0, 0, 0, // interface 1 alt 1 (ignored)
	7, 0x05, 0x83, 0x03, 64, 0, 1,
}

func TestParseConfigDescriptor(t *testing.T) {
	eps := ParseConfigDescriptor(mouseConfig)
	if len(eps) != 2 {
		t.Fatalf("esperava 2 endpoints, obtido %d: %+v", len(eps), eps)
	}
	if e := eps[0]; e.Interface != 0 || e.Class != 3 || e.Protocol != 2 || e.Address != 0x81 || e.Interval != 1 || e.MaxPacket != 8 || !e.IsInterruptIn() {
		t.Errorf("endpoint 0: %+v", e)
	}
	if e := eps[1]; e.Interface != 1 || e.Protocol != 0 || e.Interval != 8 || e.MaxPacket != 20 {
		t.Errorf("endpoint 1: %+v", e)
	}
	// Truncated input must not panic.
	for i := range mouseConfig {
		_ = ParseConfigDescriptor(mouseConfig[:i])
	}
}

func TestParseNodeConnection(t *testing.T) {
	b := make([]byte, NodeConnInfoExSize+2*PipeInfoSize)
	le := binary.LittleEndian
	le.PutUint32(b[0:], 3) // ConnectionIndex
	dd := b[4:22]
	dd[0], dd[1] = 18, 1
	le.PutUint16(dd[2:], 0x0200)
	le.PutUint16(dd[8:], 0x046D)
	le.PutUint16(dd[10:], 0xC547)
	b[22] = 1 // configuration
	b[23] = 1 // full speed
	b[24] = 0
	le.PutUint16(b[25:], 7)
	le.PutUint32(b[27:], 2) // open pipes
	le.PutUint32(b[31:], 1) // connected
	p := b[NodeConnInfoExSize:]
	copy(p, []byte{7, 5, 0x81, 3, 8, 0, 1})
	copy(p[PipeInfoSize:], []byte{7, 5, 0x82, 3, 20, 0, 1})
	c, ok := ParseNodeConnection(b, len(b))
	if !ok {
		t.Fatal("não decodificou")
	}
	if c.VendorID != 0x046D || c.ProductID != 0xC547 || c.Speed != USBSpeedFull || c.BcdUSB != 0x0200 || c.Status != 1 || c.Address != 7 {
		t.Errorf("conexão: %+v", c)
	}
	if len(c.OpenPipes) != 2 || c.OpenPipes[1].Address != 0x82 || c.OpenPipes[1].MaxPacket != 20 || c.OpenPipes[0].Interval != 1 {
		t.Errorf("pipes: %+v", c.OpenPipes)
	}
	if _, ok := ParseNodeConnection(b, 20); ok {
		t.Error("buffer curto deveria falhar")
	}
	// Claims more pipes than the buffer holds: decode only what is there.
	le.PutUint32(b[27:], 9)
	if c, _ := ParseNodeConnection(b, len(b)); len(c.OpenPipes) != 2 {
		t.Errorf("pipes truncados: %d", len(c.OpenPipes))
	}
}

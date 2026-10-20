package periph

import (
	"testing"

	"axion/internal/sys"
)

func TestParseHIDVendor(t *testing.T) {
	cases := []struct {
		in       string
		vid, pid uint16
		ok       bool
	}{
		{`HID\VEN_06CB&COL01\5&2D74AD6E&0&0000`, 0x06CB, 0, true},
		{`HID\VEN_04F3&DEV_3282&Col02\5&1&0`, 0x04F3, 0x3282, true},
		{`USB\VID_046D&PID_C547\5&1`, 0, 0, false},
		{`HID\CONVERTEDDEVICE&COL01\5&1BD8A768&0&0000`, 0, 0, false},
		{`ACPI\DLLK0BF8\4&29651D20&0`, 0, 0, false},
	}
	for _, c := range cases {
		v, p, ok := ParseHIDVendor(c.in)
		if v != c.vid || p != c.pid || ok != c.ok {
			t.Errorf("%s: %04X:%04X %v", c.in, v, p, ok)
		}
	}
}

// The touchpad of the first notebook report was listed as "Microsoft" (the
// HID driver's provider).
func TestTouchpadNamedByVendor(t *testing.T) {
	devs := Group([]sys.InputDevice{
		{InstanceID: `HID\VEN_06CB&COL01\5&2D74AD6E&0&0000`, Kind: "mouse", Name: "Mouse compatível com HID", Manufacturer: "Microsoft", Chain: []string{`ACPI\SYNA30D2\4&1`}},
		{InstanceID: `HID\VID_045E&PID_0823&MI_01&COL01\8&1&0&0000`, Kind: "mouse", Name: "HID-compliant mouse", Manufacturer: "Microsoft", USBNode: `USB\VID_045E&PID_0823\5&2`, Chain: []string{`USB\VID_045E&PID_0823\5&2`}},
	})
	names := map[string]bool{}
	for _, d := range devs {
		names[d.Name] = true
	}
	if !names["Touchpad Synaptics"] {
		t.Errorf("touchpad: %v", names)
	}
	if !names["Microsoft"] {
		t.Errorf("mouse da própria Microsoft (VID 045E) mantém o nome do fabricante: %v", names)
	}
}

// 0.6.3: product strings with trailing spaces ("USB Gaming Keyboard ").
func TestNameTrimmed(t *testing.T) {
	out := Group([]sys.InputDevice{{InstanceID: `HID\VID_1A2C&PID_930B&MI_00\7&1&0&0000`, Kind: "keyboard", Name: "Dispositivo de teclado HID", Product: "USB Gaming Keyboard ",
		USBNode: `USB\VID_1A2C&PID_930B\6&1`, Chain: []string{`USB\VID_1A2C&PID_930B\6&1`}, Speed: sys.USBSpeedLow}})
	if len(out) != 1 || out[0].Name != "USB Gaming Keyboard" {
		t.Fatalf("nome: %+v", out)
	}
}

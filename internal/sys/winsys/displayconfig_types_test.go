package winsys

import (
	"testing"
	"unsafe"
)

// The sizes Windows expects (wingdi.h); a wrong layout would make the calls
// fail or read garbage.
func TestDisplayConfigLayout(t *testing.T) {
	for name, got := range map[string]uintptr{
		"DISPLAYCONFIG_PATH_INFO":          unsafe.Sizeof(dcPathInfo{}),
		"DISPLAYCONFIG_MODE_INFO":          unsafe.Sizeof(dcModeInfo{}),
		"DISPLAYCONFIG_DEVICE_INFO_HEADER": unsafe.Sizeof(dcHeader{}),
		"DISPLAYCONFIG_SOURCE_DEVICE_NAME": unsafe.Sizeof(dcSourceName{}),
		"DISPLAYCONFIG_TARGET_DEVICE_NAME": unsafe.Sizeof(dcTargetName{}),
	} {
		want := map[string]uintptr{"DISPLAYCONFIG_PATH_INFO": 72, "DISPLAYCONFIG_MODE_INFO": 64, "DISPLAYCONFIG_DEVICE_INFO_HEADER": 20,
			"DISPLAYCONFIG_SOURCE_DEVICE_NAME": 84, "DISPLAYCONFIG_TARGET_DEVICE_NAME": 420}[name]
		if got != want {
			t.Errorf("%s: %d bytes, Windows espera %d", name, got, want)
		}
	}
	if unsafe.Offsetof(dcPathInfo{}.TargetAdapter) != 20 || unsafe.Offsetof(dcPathInfo{}.OutputTechnology) != 36 || unsafe.Offsetof(dcPathInfo{}.Flags) != 68 {
		t.Errorf("deslocamentos de DISPLAYCONFIG_PATH_INFO errados")
	}
	if unsafe.Offsetof(dcTargetName{}.MonitorFriendlyDeviceName) != 36 || unsafe.Offsetof(dcTargetName{}.OutputTechnology) != 24 {
		t.Errorf("deslocamentos de DISPLAYCONFIG_TARGET_DEVICE_NAME errados")
	}
	if !internalOutput(0x80000000) || !internalOutput(11) || internalOutput(5) || internalOutput(10) {
		t.Errorf("classificação da saída interna")
	}
}

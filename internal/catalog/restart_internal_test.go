package catalog

import (
	"context"
	"strings"
	"testing"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

// restartAdapters must never leave the adapter disabled silently: when
// re-enabling fails it retries and reports the adapter by name.
func TestRestartAdaptersReportsDisabledAdapter(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &facts.Facts{Adapters: []facts.Adapter{{Name: "Ethernet", Kind: "Ethernet", Up: true, ClassKey: "0001",
		PnPDeviceID: `PCI\VEN_10EC&DEV_8125&SUBSYS_E0001458&REV_05\01000000684CE00000`}}}
	enabled := true
	enables := 0
	m.W.CallHandler = func(ns, path, method string, args []any) (any, error) {
		switch method {
		case "Disable":
			enabled = false
		case "Enable":
			enables++
		}
		return int64(0), nil
	}
	setRow := func() {
		m.W.Set(`root\cimv2`, "Win32_NetworkAdapter", sys.WMIRow{"PNPDeviceID": f.Adapters[0].PnPDeviceID, "DeviceID": "7", "NetConnectionID": "Ethernet", "NetEnabled": enabled})
	}
	setRow()
	c := &tweaks.Ctx{Context: context.Background(), P: m.Platform, F: f}
	// Enable "succeeds" but the adapter stays off: retried, then reported.
	m.W.CallHandler = func(ns, path, method string, args []any) (any, error) {
		if method == "Disable" {
			enabled = false
			setRow()
		}
		if method == "Enable" {
			enables++
		}
		return int64(0), nil
	}
	err = restartAdapters(true)(c, false)
	if err == nil || !strings.Contains(err.Error(), "não voltou a ser ativado") {
		t.Fatalf("expected a disabled-adapter error, got %v", err)
	}
	if enables < 2 {
		t.Fatalf("Enable should be retried, got %d attempt(s)", enables)
	}
	// Normal case: Enable brings it back.
	m.W.CallHandler = func(ns, path, method string, args []any) (any, error) {
		switch method {
		case "Disable":
			enabled = false
		case "Enable":
			enabled = true
		}
		setRow()
		return int64(0), nil
	}
	if err := restartAdapters(true)(c, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

package periph

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

func TestNominalHzMatchesMicrosoftTables(t *testing.T) {
	cases := []struct {
		speed, interval int
		want            float64
	}{
		{sys.USBSpeedLow, 10, 125}, {sys.USBSpeedLow, 0, 125}, {sys.USBSpeedLow, 15, 125},
		{sys.USBSpeedLow, 16, 62.5}, {sys.USBSpeedLow, 36, 31.25},
		{sys.USBSpeedFull, 1, 1000}, {sys.USBSpeedFull, 2, 500}, {sys.USBSpeedFull, 3, 500},
		{sys.USBSpeedFull, 4, 250}, {sys.USBSpeedFull, 8, 125}, {sys.USBSpeedFull, 10, 125},
		{sys.USBSpeedFull, 16, 62.5}, {sys.USBSpeedFull, 255, 31.25}, {sys.USBSpeedFull, 0, 0},
		{sys.USBSpeedHigh, 1, 8000}, {sys.USBSpeedHigh, 2, 4000}, {sys.USBSpeedHigh, 3, 2000},
		{sys.USBSpeedHigh, 4, 1000}, {sys.USBSpeedHigh, 6, 250}, {sys.USBSpeedHigh, 9, 250},
		{sys.USBSpeedUnknown, 1, 0},
	}
	for _, c := range cases {
		if got := NominalHz(c.speed, c.interval); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("NominalHz(%d, %d) = %v, want %v", c.speed, c.interval, got, c.want)
		}
	}
}

func TestTransportAndIDs(t *testing.T) {
	cases := []struct {
		inst  string
		chain []string
		want  string
	}{
		{`HID\VID_046D&PID_C547&MI_01&COL01\8&1`, []string{`USB\VID_046D&PID_C547&MI_01\7&1`, `USB\VID_046D&PID_C547\5&1`}, TransportUSB},
		{`HID\{00001812-0000-1000-8000-00805f9b34fb}_Dev_VID&02046d_PID&b023_REV&0006\9&1`, []string{`BTHLEDEVICE\{00001812-0000-1000-8000-00805f9b34fb}_Dev_VID&02046d_PID&b023\8&1`}, TransportBLE},
		{`HID\{00001124-0000-1000-8000-00805f9b34fb}_VID&0002046d_PID&b01a&Col01\9&1`, []string{`BTHENUM\{00001124-0000-1000-8000-00805f9b34fb}_VID&0002046d_PID&b01a\8&1`}, TransportBT},
		{`ACPI\PNP0303\4&1`, nil, TransportPS2},
		{`HID\ELAN0001&Col02\5&1`, []string{`ACPI\ELAN0001\1`}, TransportInternal},
		{`HID\VID_0000&PID_0000\1`, []string{`ROOT\RDP_MOU\0000`}, TransportVirtual},
	}
	for _, c := range cases {
		if got := TransportOf(c.inst, c.chain); got != c.want {
			t.Errorf("TransportOf(%s) = %s, want %s", c.inst, got, c.want)
		}
	}
	if v, p, ok := ParseVIDPID(`USB\VID_046D&PID_C547\5&2f`); !ok || v != 0x046D || p != 0xC547 {
		t.Errorf("USB VID/PID: %04x %04x %v", v, p, ok)
	}
	if v, p, ok := ParseVIDPID(`BTHLEDEVICE\{00001812-0000-1000-8000-00805f9b34fb}_Dev_VID&02046d_PID&b023`); !ok || v != 0x046D || p != 0xB023 {
		t.Errorf("BLE VID/PID: %04x %04x %v", v, p, ok)
	}
	if v, p, ok := ParseVIDPID(`HID\{00001124-0000-1000-8000-00805f9b34fb}_VID&0002046d_PID&b01a&Col01`); !ok || v != 0x046D || p != 0xB01A {
		t.Errorf("BT VID/PID: %04x %04x %v", v, p, ok)
	}
	if InterfaceNumber(`USB\VID_046D&PID_C547&MI_01\7&1`) != 1 || InterfaceNumber(`USB\VID_046D&PID_C547\5`) != -1 {
		t.Errorf("InterfaceNumber")
	}
	got := RawNameToInstance(`\\?\HID#VID_046D&PID_C547&MI_01&Col01#8&1a2b3c4d&0&0000#{378de44c-56ef-11d1-bc8c-00a0c91405dd}`)
	if got != `HID\VID_046D&PID_C547&MI_01&COL01\8&1A2B3C4D&0&0000` {
		t.Errorf("RawNameToInstance = %s", got)
	}
}

func TestGroupMergesCollections(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	raw, _ := m.N.InputDevices()
	devs := Group(raw)
	if len(devs) != 2 {
		t.Fatalf("esperava 2 dispositivos físicos, obtido %d: %+v", len(devs), devs)
	}
	recv, kbd := devs[0], devs[1]
	if recv.Primary != "mouse" || !recv.Receiver || len(recv.Collections) != 2 || recv.NominalHz != 1000 || recv.Name != "USB Receiver" {
		t.Errorf("receptor: %+v", recv)
	}
	if !recv.Has("keyboard") || !recv.Has("mouse") {
		t.Errorf("receptor deveria expor mouse e teclado: %v", recv.Kinds)
	}
	if kbd.Primary != "keyboard" || kbd.Speed != sys.USBSpeedLow || kbd.BInterval != 10 || kbd.NominalHz != 125 || kbd.HubDepth != 1 {
		t.Errorf("teclado: %+v", kbd)
	}
	if kbd.VID != 0x1A2C || kbd.PID != 0x2124 {
		t.Errorf("VID/PID do teclado: %04x:%04x", kbd.VID, kbd.PID)
	}
}

func TestKeyboardWithMacroMouseKeepsKeyboardRate(t *testing.T) {
	ep := func(iface, proto int, addr, interval uint8) sys.USBEndpoint {
		return sys.USBEndpoint{Interface: iface, Class: 3, Protocol: proto, Address: addr, Attributes: 3, Interval: interval}
	}
	node := `USB\VID_1234&PID_0001\1`
	eps := []sys.USBEndpoint{ep(0, 1, 0x81, 8), ep(1, 0, 0x82, 1)}
	devs := Group([]sys.InputDevice{
		{InstanceID: `HID\VID_1234&PID_0001&MI_01&COL02\1`, Kind: "mouse", USBNode: node, USBInterface: `USB\VID_1234&PID_0001&MI_01\1`, Chain: []string{node}, Speed: sys.USBSpeedFull, Endpoints: eps},
		{InstanceID: `HID\VID_1234&PID_0001&MI_00\1`, Kind: "keyboard", USBNode: node, USBInterface: `USB\VID_1234&PID_0001&MI_00\1`, Chain: []string{node}, Speed: sys.USBSpeedFull, Endpoints: eps},
	})
	if len(devs) != 1 || devs[0].Primary != "keyboard" || devs[0].BInterval != 8 || devs[0].NominalHz != 125 {
		t.Fatalf("teclado com coleção de mouse: %+v", devs)
	}
}

// Real case (report 0.3.1): a Low-Speed "USB Gaming Keyboard" whose second
// interface declares the boot-mouse protocol but whose HID collections are
// only keyboard/consumer controls was shown as a mouse.
func TestGamingKeyboardWithBootMouseInterface(t *testing.T) {
	ep := func(iface, proto int, addr, interval uint8) sys.USBEndpoint {
		return sys.USBEndpoint{Interface: iface, Class: 3, Protocol: proto, Address: addr, Attributes: 3, Interval: interval}
	}
	node := `USB\VID_1A2C&PID_930B\8&f2cb6fa&0&6`
	eps := []sys.USBEndpoint{ep(0, 1, 0x81, 10), ep(1, 2, 0x82, 10)}
	devs := Group([]sys.InputDevice{
		{InstanceID: `HID\VID_1A2C&PID_930B&MI_00\A&32574F56&0&0000`, Kind: "keyboard", Product: "USB Gaming Keyboard ", USBNode: node, USBInterface: `USB\VID_1A2C&PID_930B&MI_00\9&1`, Chain: []string{node}, Speed: sys.USBSpeedLow, Endpoints: eps},
		{InstanceID: `HID\VID_1A2C&PID_930B&MI_01&COL04\A&E801194&0&0003`, Kind: "keyboard", Product: "USB Gaming Keyboard ", USBNode: node, USBInterface: `USB\VID_1A2C&PID_930B&MI_01\9&2`, Chain: []string{node}, Speed: sys.USBSpeedLow, Endpoints: eps},
	})
	if len(devs) != 1 || devs[0].Primary != "keyboard" || devs[0].NominalHz != 125 {
		t.Fatalf("teclado com interface de mouse virou mouse: %+v", devs)
	}
}

func series(n int, periodTicks int64, jitter func(i int) int64) []int64 {
	out := make([]int64, n)
	for i := range out {
		out[i] = 1_000_000 + int64(i)*periodTicks + jitter(i)
	}
	return out
}

func TestAnalyzeSteady1000Hz(t *testing.T) {
	st := Analyze(series(3000, 10_000, func(i int) int64 { return int64(i%3) - 1 }), 10_000_000, 1000)
	if st == nil {
		t.Fatal("sem estatística")
	}
	if st.Snapped != 1000 || math.Abs(st.RateHz-1000) > 5 || st.OnTimePct < 99 || st.Pauses != 0 {
		t.Errorf("1000 Hz estável: %+v", st)
	}
}

func TestAnalyzeCoalescedAndPauses(t *testing.T) {
	var ts []int64
	t0 := int64(0)
	for i := 0; i < 4000; i++ {
		t0 += 1250 // 8000 Hz at 10 MHz
		if i == 2000 {
			t0 += 5_000_000 // 0.5 s pause: user stopped moving
		}
		ts = append(ts, t0)
		if i%10 == 0 {
			ts = append(ts, t0) // delivered together
		}
	}
	st := Analyze(ts, 10_000_000, 8000)
	if st == nil || st.Snapped != 8000 {
		t.Fatalf("8000 Hz: %+v", st)
	}
	if st.Pauses != 0 {
		// 0.5 s is above the 250 ms "mouse left still" cutoff, so it is
		// dropped rather than counted as a pause.
		t.Errorf("pausa longa deveria ser descartada: %+v", st)
	}
	if st.Coalesced < 300 {
		t.Errorf("agrupados: %d", st.Coalesced)
	}
	// A 20 ms hiccup counts as a pause.
	ts2 := series(2000, 10_000, func(i int) int64 {
		if i >= 1000 {
			return 200_000
		}
		return 0
	})
	st2 := Analyze(ts2, 10_000_000, 1000)
	if st2.Pauses != 1 || math.Abs(st2.PauseMS-21) > 0.5 {
		t.Errorf("pausa de 20 ms: %+v", st2)
	}
}

func TestSnap(t *testing.T) {
	for hz, want := range map[float64]int{998: 1000, 7900: 8000, 480: 500, 125.3: 125, 700: 0, 0: 0, 3100: 0} {
		if got := Snap(hz); got != want {
			t.Errorf("Snap(%v) = %d, want %d", hz, got, want)
		}
	}
}

func TestSummarizeMockCapture(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	raw, _ := m.N.InputDevices()
	devs := Group(raw)
	cap, err := m.N.CaptureInput(context.Background(), sys.CaptureOpts{Duration: 3 * time.Second, USBTrace: true})
	if err != nil {
		t.Fatal(err)
	}
	res := Summarize(devs, cap)
	if len(res.Devices) != 1 {
		t.Fatalf("esperava 1 dispositivo medido: %+v", res.Devices)
	}
	d := res.Devices[0]
	if d.Name != "USB Receiver" || d.Source != "usb" || d.Snapped != 1000 || d.Verdict != VerdictOK {
		t.Errorf("resultado: %+v", d)
	}
	if d.USB == nil || d.Raw == nil || d.Lag == nil {
		t.Fatalf("faltam séries: usb=%v raw=%v lag=%v", d.USB, d.Raw, d.Lag)
	}
	if d.Lag.MedianUS < 5 || d.Lag.MedianUS > 80 {
		t.Errorf("atraso USB→app fora do esperado: %+v", d.Lag)
	}
	if !strings.Contains(d.Matched, "VID/PID") {
		t.Errorf("associação: %s", d.Matched)
	}

	// Without the USB trace the verdict comes from raw input.
	cap2, _ := m.N.CaptureInput(context.Background(), sys.CaptureOpts{Duration: 2 * time.Second})
	res2 := Summarize(devs, cap2)
	if len(res2.Devices) != 1 || res2.Devices[0].Source != "raw" || res2.Devices[0].Snapped != 1000 {
		t.Errorf("só raw input: %+v", res2.Devices)
	}
	if len(res2.Notes) == 0 || !strings.Contains(res2.Notes[0], "ETW") {
		t.Errorf("deveria explicar a falta do rastreamento USB: %v", res2.Notes)
	}
}

func TestSummarizeLowRate(t *testing.T) {
	m, _ := mocksys.New(t.TempDir())
	raw, _ := m.N.InputDevices()
	devs := Group(raw)
	st := series(1500, 20_000, func(int) int64 { return 0 }) // 500 Hz on a 1000 Hz port
	cap := sys.InputCapture{Freq: 10_000_000, Duration: 3 * time.Second, RawInput: []sys.RawInputSeries{{
		Device: `\\?\HID#VID_046D&PID_C547&MI_01&Col01#8&1a2b3c4d&0&0000#{378de44c-56ef-11d1-bc8c-00a0c91405dd}`, Kind: "mouse", Stamps: st,
	}}}
	res := Summarize(devs, cap)
	if len(res.Devices) != 1 || res.Devices[0].Verdict != VerdictLow || res.Devices[0].Snapped != 500 {
		t.Fatalf("taxa baixa: %+v", res.Devices)
	}
}

func TestBluetoothDevicesStayApart(t *testing.T) {
	bt := `BTHLEDEVICE\{00001812-0000-1000-8000-00805f9b34fb}_Dev_VID&02046d_PID&b023_REV&0006_d4a8\8&1`
	radio := `USB\VID_8087&PID_0029\5&1`
	devs := Group([]sys.InputDevice{
		{InstanceID: `HID\{00001812-0000-1000-8000-00805f9b34fb}_Dev_VID&02046d_PID&b023_REV&0006_d4a8&Col01\9&1`, Kind: "mouse", Chain: []string{bt, `BTH\MS_BTHLE\7&1`, radio, `USB\ROOT_HUB30\4&1`}},
		{InstanceID: `HID\{00001812-0000-1000-8000-00805f9b34fb}_Dev_VID&02046d_PID&b023_REV&0006_d4a8&Col02\9&1`, Kind: "keyboard", Chain: []string{bt, `BTH\MS_BTHLE\7&1`, radio, `USB\ROOT_HUB30\4&1`}},
		{InstanceID: `HID\{00001124-0000-1000-8000-00805f9b34fb}_VID&0002045e_PID&0800&Col01\9&2`, Kind: "keyboard", Chain: []string{`BTHENUM\{00001124-0000-1000-8000-00805f9b34fb}_VID&0002045e_PID&0800\8&2`, radio}},
	})
	if len(devs) != 2 {
		t.Fatalf("dois aparelhos Bluetooth diferentes, obtido %d: %+v", len(devs), devs)
	}
	if devs[0].Transport != TransportBLE || devs[0].Primary != "mouse" || devs[0].VID != 0x046D || devs[0].PID != 0xB023 || devs[0].NominalHz != 0 {
		t.Errorf("mouse BLE: %+v", devs[0])
	}
	if devs[1].Transport != TransportBT || devs[1].VID != 0x045E {
		t.Errorf("teclado Bluetooth: %+v", devs[1])
	}
}

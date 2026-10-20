package mocksys

import (
	"context"
	"math/rand"
	"time"

	"axion/internal/sys"
)

// Mock input devices: a Logitech wireless receiver plugged straight into the
// CPU's USB controller and a generic low-speed keyboard behind a hub, which
// is the kind of setup the diagnostic has to explain.
const (
	MockMouseHID    = `HID\VID_046D&PID_C547&MI_01&COL01\8&1A2B3C4D&0&0000`
	MockMouseIface  = `USB\VID_046D&PID_C547&MI_01\7&2F5E1C3&0&0001`
	MockMouseUSB    = `USB\VID_046D&PID_C547\5&2F5E1C3&0&3`
	MockRecvKbdHID  = `HID\VID_046D&PID_C547&MI_00\8&3B4C5D6E&0&0000`
	MockRecvKbdIfc  = `USB\VID_046D&PID_C547&MI_00\7&2F5E1C3&0&0000`
	MockKbdHID      = `HID\VID_1A2C&PID_2124&MI_00\8&11AA22BB&0&0000`
	MockKbdIface    = `USB\VID_1A2C&PID_2124&MI_00\7&7C8D9E0F&0&0000`
	MockKbdUSB      = `USB\VID_1A2C&PID_2124\6&7C8D9E0F&0&2`
	MockExtHub      = `USB\VID_05E3&PID_0610\5&2F5E1C3&0&4`
	MockRootHub     = `USB\ROOT_HUB30\4&1D2E3F40&0&0`
	MockController  = `PCI\VEN_1022&DEV_15B6&SUBSYS_88771043&REV_00\4&2A3B4C5D&0&0341`
	mockControllerN = "Controlador de Host eXtensível AMD USB 3.10 - 1.10 (Microsoft)"
)

func defaultInputDevices() []sys.InputDevice {
	hid := func(iface, proto int, addr uint8, interval uint8) sys.USBEndpoint {
		return sys.USBEndpoint{Interface: iface, Class: 3, Protocol: proto, Address: addr, Attributes: 3, MaxPacket: 8, Interval: interval}
	}
	recvEPs := []sys.USBEndpoint{hid(0, 1, 0x81, 1), hid(1, 2, 0x82, 1), hid(2, 0, 0x83, 1)}
	kbdEPs := []sys.USBEndpoint{hid(0, 1, 0x81, 10), hid(1, 0, 0x82, 10)}
	recv := func(id, kind, name, iface string) sys.InputDevice {
		return sys.InputDevice{
			InstanceID: id, Kind: kind, Name: name, Product: "USB Receiver", Manufacturer: "Logitech",
			Chain:   []string{iface, MockMouseUSB, MockRootHub, MockController, `ACPI\PNP0A08\0`},
			USBNode: MockMouseUSB, USBInterface: iface, Hub: MockRootHub, HubIsRoot: true, HubDepth: 0, Port: 3,
			Speed: sys.USBSpeedFull, BcdUSB: 0x0200, Endpoints: recvEPs,
			Controller: mockControllerN, ControllerID: MockController,
		}
	}
	return []sys.InputDevice{
		recv(MockMouseHID, "mouse", "Mouse compatível com HID", MockMouseIface),
		recv(MockRecvKbdHID, "keyboard", "Dispositivo de teclado HID", MockRecvKbdIfc),
		{
			InstanceID: MockKbdHID, Kind: "keyboard", Name: "Dispositivo de teclado HID", Product: "USB Keyboard", Manufacturer: "(Teclados padrão)",
			Chain:   []string{MockKbdIface, MockKbdUSB, MockExtHub, MockRootHub, MockController, `ACPI\PNP0A08\0`},
			USBNode: MockKbdUSB, USBInterface: MockKbdIface, Hub: MockExtHub, HubIsRoot: false, HubDepth: 1, Port: 2,
			Speed: sys.USBSpeedLow, BcdUSB: 0x0110, Endpoints: kbdEPs,
			Controller: mockControllerN, ControllerID: MockController,
		},
	}
}

// InputDevices implements sys.Native.
func (n *Native) InputDevices() ([]sys.InputDevice, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.Inputs == nil {
		n.Inputs = defaultInputDevices()
	}
	out := make([]sys.InputDevice, len(n.Inputs))
	copy(out, n.Inputs)
	return out, nil
}

// CaptureInput implements sys.Native with a synthetic capture: the receiver
// reports at 1000 Hz with realistic delivery jitter (a few coalesced
// messages and one short stall) and the USB trace shows the steady poll.
func (n *Native) CaptureInput(ctx context.Context, opts sys.CaptureOpts) (sys.InputCapture, error) {
	n.mu.Lock()
	fn := n.CaptureFn
	n.Captures++
	n.mu.Unlock()
	if fn != nil {
		return fn(ctx, opts)
	}
	d := opts.Duration
	if d <= 0 {
		d = 5 * time.Second
	}
	const freq = 10_000_000 // 100 ns ticks, like QPC on Windows 10+
	rng := rand.New(rand.NewSource(42))
	res := sys.InputCapture{Freq: freq, Duration: d}

	// Short pause so the preview shows the progress while "capturing".
	select {
	case <-ctx.Done():
		res.Cancelled = true
		return res, nil
	case <-time.After(300 * time.Millisecond):
	}

	moving := d - 400*time.Millisecond
	count := int(moving / time.Millisecond)
	var raw []int64
	var usb []int64
	t := int64(2_000_000)
	for i := 0; i < count; i++ {
		poll := t + int64(i)*10_000 + int64(rng.Intn(21)-10) // ±1 µs host jitter
		usb = append(usb, poll)
		deliver := poll + 120 + int64(rng.Intn(600)) // 12 µs + up to 60 µs
		if i%97 == 0 && i > 0 {
			deliver = raw[len(raw)-1] // coalesced with the previous message
		}
		if i == count/2 {
			deliver += 30_000 // a 3 ms delivery hiccup
		}
		if len(raw) > 0 && deliver < raw[len(raw)-1] {
			deliver = raw[len(raw)-1]
		}
		raw = append(raw, deliver)
	}
	res.RawInput = []sys.RawInputSeries{{
		Device: `\\?\HID#VID_046D&PID_C547&MI_01&Col01#8&1a2b3c4d&0&0000#{378de44c-56ef-11d1-bc8c-00a0c91405dd}`,
		Kind:   "mouse", Stamps: raw, Events: len(raw) + 12,
	}}
	if opts.USBTrace {
		res.USBTraced = true
		res.USB = []sys.USBPipeSeries{
			{Device: 0xFFFFA00100001000, Pipe: 0xFFFFA00100002000, Stamps: usb},
			{Device: 0xFFFFA00100003000, Pipe: 0xFFFFA00100004000, Stamps: []int64{3_000_000, 3_800_000, 11_000_000}},
		}
		res.USBDevices = []sys.USBTraceDevice{
			{Handle: 0xFFFFA00100001000, Port: 3, Desc: "USB Composite Device", Path: `\\?\USB#VID_046D&PID_C547#5&2f5e1c3&0&3#{a5dcbf10-6530-11d2-901f-00c04fb951ed}`},
			{Handle: 0xFFFFA00100003000, Port: 2, Desc: "USB Composite Device", Path: `\\?\USB#VID_1A2C&PID_2124#6&7c8d9e0f&0&2#{a5dcbf10-6530-11d2-901f-00c04fb951ed}`},
		}
	}
	return res, nil
}

// seedInputDevices registers the WMI power-management instances and the
// registry values the peripheral step reads.
func seedInputDevices(m *Mock) {
	// Power Management tab state (MSPower_DeviceEnable): the generic
	// keyboard's vendor enabled selective suspend; the hubs allow it too.
	m.W.Set(`root\WMI`, "MSPower_DeviceEnable",
		sys.WMIRow{"InstanceName": MockKbdUSB + "_0", "Enable": true, "Active": true},
		sys.WMIRow{"InstanceName": MockExtHub + "_0", "Enable": true, "Active": true},
		sys.WMIRow{"InstanceName": MockRootHub + "_0", "Enable": true, "Active": true},
	)
	enum := `HKLM\SYSTEM\CurrentControlSet\Enum\`
	set := func(p, n string, v sys.RegValue) { _ = m.R.SetValue(p, n, v) }
	set(enum+MockKbdUSB+`\Device Parameters`, "SelectiveSuspendEnabled", sys.Binary([]byte{1}))
	set(enum+MockMouseUSB+`\Device Parameters`, "SymbolicName", sys.SZ(`\??\USB#VID_046D&PID_C547#5&2f5e1c3&0&3#{a5dcbf10-6530-11d2-901f-00c04fb951ed}`))
	// Input driver queues left at 20 by an old "input lag" guide.
	set(`HKLM\SYSTEM\CurrentControlSet\Services\mouclass\Parameters`, "MouseDataQueueSize", sys.DWord(20))
	set(`HKLM\SYSTEM\CurrentControlSet\Services\kbdclass\Parameters`, "KeyboardDataQueueSize", sys.DWord(100))
	set(`HKCU\Control Panel\Accessibility\Keyboard Response`, "Flags", sys.SZ("126"))
	set(`HKCU\Control Panel\Accessibility\Keyboard Response`, "DelayBeforeAcceptance", sys.SZ("1000"))
	set(`HKCU\Control Panel\Accessibility\Keyboard Response`, "BounceTime", sys.SZ("0"))
	set(`HKCU\Control Panel\Accessibility\ToggleKeys`, "Flags", sys.SZ("62"))
}

// GPUScheduling implements sys.Native: supported, enabled when HwSchMode = 2.
func (n *Native) GPUScheduling() (sys.GPUScheduling, error) {
	g := sys.GPUScheduling{Supported: true}
	if n.reg != nil {
		if v, err := n.reg.GetValue(`HKLM\SYSTEM\CurrentControlSet\Control\GraphicsDrivers`, "HwSchMode"); err == nil && v.Exists {
			g.Enabled = v.Int == 2
		}
	}
	return g, nil
}

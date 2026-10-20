// Package periph turns the raw keyboard/mouse inventory into physical
// devices, computes the polling rate the USB port allows and analyses
// polling captures (raw input arrival and USB transfer completion times).
//
// Sources for the rules used here:
//   - Polling period per bus speed and bInterval: Microsoft Learn,
//     USB_ENDPOINT_DESCRIPTOR (tables for low, full and high speed).
//   - A mouse reports on every poll only while it moves; a still mouse
//     answers NAK and nothing completes, so captures need continuous motion.
package periph

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"axion/internal/facts"
	"axion/internal/sys"
)

// PeriodMS returns the polling period Windows uses for an interrupt endpoint
// of a device at the given speed, or 0 when unknown.
func PeriodMS(speed, bInterval int) float64 {
	switch speed {
	case sys.USBSpeedLow:
		switch {
		case bInterval <= 15:
			return 8
		case bInterval <= 35:
			return 16
		default:
			return 32
		}
	case sys.USBSpeedFull:
		switch {
		case bInterval <= 0:
			return 0
		case bInterval == 1:
			return 1
		case bInterval <= 3:
			return 2
		case bInterval <= 7:
			return 4
		case bInterval <= 15:
			return 8
		case bInterval <= 31:
			return 16
		default:
			return 32
		}
	case sys.USBSpeedHigh, sys.USBSpeedSuper:
		if bInterval <= 0 {
			return 0
		}
		exp := bInterval - 1
		if exp > 5 { // Windows caps the period at 32 microframes
			exp = 5
		}
		return 0.125 * float64(int(1)<<exp)
	}
	return 0
}

// NominalHz is the highest polling rate the port schedules (0 = unknown).
func NominalHz(speed, bInterval int) float64 {
	p := PeriodMS(speed, bInterval)
	if p <= 0 {
		return 0
	}
	return 1000 / p
}

// SpeedLabel names a USB bus speed.
func SpeedLabel(speed int) string {
	switch speed {
	case sys.USBSpeedLow:
		return "USB Low-Speed (1,5 Mbit/s)"
	case sys.USBSpeedFull:
		return "USB Full-Speed (12 Mbit/s)"
	case sys.USBSpeedHigh:
		return "USB High-Speed (480 Mbit/s)"
	case sys.USBSpeedSuper:
		return "USB SuperSpeed"
	}
	return "velocidade USB desconhecida"
}

// Transport values.
const (
	TransportUSB      = "usb"
	TransportBT       = "bluetooth"
	TransportBLE      = "bluetooth-le"
	TransportPS2      = "ps2"
	TransportInternal = "internal"
	TransportVirtual  = "virtual"
	TransportUnknown  = "unknown"
)

// TransportLabel names a transport for the UI.
func TransportLabel(t string) string {
	switch t {
	case TransportUSB:
		return "USB"
	case TransportBT:
		return "Bluetooth"
	case TransportBLE:
		return "Bluetooth LE"
	case TransportPS2:
		return "PS/2"
	case TransportInternal:
		return "Interno (I2C/ACPI)"
	case TransportVirtual:
		return "Virtual (acesso remoto, software)"
	}
	return "Desconhecido"
}

const (
	uuidHIDOverGATT = "{00001812-0000-1000-8000-00805F9B34FB}"
	uuidBTHID       = "{00001124-0000-1000-8000-00805F9B34FB}"
)

// TransportOf classifies how an input collection reaches the PC from its
// instance ID and parent chain.
func TransportOf(instanceID string, chain []string) string {
	nodes := append([]string{instanceID}, chain...)
	for _, n := range nodes {
		up := strings.ToUpper(n)
		switch {
		case strings.HasPrefix(up, `BTHLEDEVICE\`) || strings.Contains(up, uuidHIDOverGATT):
			return TransportBLE
		case strings.HasPrefix(up, `BTHENUM\`) || strings.Contains(up, uuidBTHID):
			return TransportBT
		case strings.HasPrefix(up, `USB\`):
			return TransportUSB
		case strings.HasPrefix(up, `ACPI\`):
			for _, id := range []string{"PNP0303", "PNP030B", "PNP0F03", "PNP0F13"} {
				if strings.Contains(up, id) {
					return TransportPS2
				}
			}
			return TransportInternal
		case strings.HasPrefix(up, `ROOT\`), strings.HasPrefix(up, `SWD\`), strings.HasPrefix(up, `TERMINPUT_BUS\`):
			return TransportVirtual
		}
	}
	return TransportUnknown
}

var (
	reUSBID = regexp.MustCompile(`(?i)VID_([0-9A-F]{4})&PID_([0-9A-F]{4})`)
	reBTID  = regexp.MustCompile(`(?i)VID&[0-9A-F]*?([0-9A-F]{4})_PID&([0-9A-F]{4})`)
	reMI    = regexp.MustCompile(`(?i)&MI_([0-9A-F]{2})`)
)

// ParseVIDPID extracts the vendor and product IDs from a USB or Bluetooth
// instance ID or device path.
func ParseVIDPID(s string) (vid, pid uint16, ok bool) {
	m := reUSBID.FindStringSubmatch(s)
	if m == nil {
		m = reBTID.FindStringSubmatch(s)
	}
	if m == nil {
		return 0, 0, false
	}
	v, err1 := strconv.ParseUint(m[1], 16, 16)
	p, err2 := strconv.ParseUint(m[2], 16, 16)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return uint16(v), uint16(p), true
}

var reHIDVen = regexp.MustCompile(`(?i)^HID[\\#]VEN_([0-9A-F]{4})(?:&DEV_([0-9A-F]{4}))?`)

// ParseHIDVendor reads the vendor of a HID device enumerated over I2C or
// ACPI, which Windows names "HID\VEN_06CB&DEV_CE26&Col01" (or without DEV):
// notebook touchpads and keyboards.
func ParseHIDVendor(instanceID string) (vid, pid uint16, ok bool) {
	m := reHIDVen.FindStringSubmatch(strings.TrimSpace(instanceID))
	if m == nil {
		return 0, 0, false
	}
	v, err := strconv.ParseUint(m[1], 16, 16)
	if err != nil || v == 0 {
		return 0, 0, false
	}
	if m[2] != "" {
		if p, err := strconv.ParseUint(m[2], 16, 16); err == nil {
			pid = uint16(p)
		}
	}
	return uint16(v), pid, true
}

// hidVendors names the vendors of keyboards, mice and touchpads by their
// USB/HID vendor ID.
var hidVendors = map[uint16]string{
	0x045E: "Microsoft", 0x046D: "Logitech", 0x1532: "Razer", 0x1038: "SteelSeries", 0x1B1C: "Corsair",
	0x0951: "HyperX", 0x03F0: "HP", 0x3434: "Keychron", 0x0B05: "ASUS", 0x1E7D: "ROCCAT", 0x093A: "PixArt",
	0x06CB: "Synaptics", 0x04F3: "Elan", 0x044E: "Alps", 0x17EF: "Lenovo", 0x413C: "Dell", 0x04F2: "Chicony",
	0x05AC: "Apple", 0x0458: "Genius", 0x2516: "Cooler Master", 0x1050: "Yubico",
}

// touchpadVendors make touchpads (and pointing sticks) of notebooks.
var touchpadVendors = map[uint16]bool{0x06CB: true, 0x04F3: true, 0x044E: true, 0x093A: true}

// VendorName names a vendor ID ("" when unknown).
func VendorName(vid uint16) string { return hidVendors[vid] }

// InterfaceNumber returns the xx of "&MI_xx" or -1.
func InterfaceNumber(instanceID string) int {
	m := reMI.FindStringSubmatch(instanceID)
	if m == nil {
		return -1
	}
	n, err := strconv.ParseUint(m[1], 16, 8)
	if err != nil {
		return -1
	}
	return int(n)
}

// PickInterval returns the bInterval of the interrupt IN endpoint that feeds
// the collection: the endpoint of the collection's own USB interface when it
// is known, otherwise the fastest HID interrupt IN endpoint.
func PickInterval(d sys.InputDevice) int {
	best := 0
	consider := func(e sys.USBEndpoint) {
		if !e.IsInterruptIn() || e.Interval == 0 {
			return
		}
		if best == 0 || int(e.Interval) < best {
			best = int(e.Interval)
		}
	}
	mi := InterfaceNumber(d.USBInterface)
	if mi < 0 {
		mi = InterfaceNumber(d.InstanceID)
	}
	if mi >= 0 {
		for _, e := range d.Endpoints {
			if e.Interface == mi {
				consider(e)
			}
		}
		if best > 0 {
			return best
		}
	}
	for _, e := range d.Endpoints {
		if e.Class == 3 {
			consider(e)
		}
	}
	if best > 0 {
		return best
	}
	for _, e := range d.Endpoints {
		consider(e)
	}
	return best
}

var receiverWords = []string{"receiver", "receptor", "dongle", "unifying", "lightspeed", "hyperspeed", "slipstream", "2.4g", "2,4g", "wireless"}

// IsReceiver guesses whether a USB device is a wireless receiver.
func IsReceiver(names ...string) bool {
	for _, n := range names {
		low := strings.ToLower(n)
		for _, w := range receiverWords {
			if strings.Contains(low, w) {
				return true
			}
		}
	}
	return false
}

var genericNames = []string{"usb composite device", "dispositivo composto usb", "usb input device", "dispositivo de entrada usb", "hid-compliant", "compatível com hid", "dispositivo de teclado hid", "dispositivo de teclado", "hid keyboard device"}

func generic(name string) bool {
	low := strings.ToLower(strings.TrimSpace(name))
	if low == "" {
		return true
	}
	for _, g := range genericNames {
		if strings.Contains(low, g) {
			return true
		}
	}
	return false
}

// RawNameToInstance converts a raw input device name
// (\\?\HID#VID_046D&PID_C547&MI_01&Col01#8&1a2b&0&0000#{guid}) into the
// upper-case device instance ID it belongs to.
func RawNameToInstance(name string) string {
	s := strings.TrimSpace(name)
	for _, p := range []string{`\\?\`, `\??\`, `\\.\`} {
		s = strings.TrimPrefix(s, p)
	}
	if i := strings.LastIndex(s, "#{"); i >= 0 {
		s = s[:i]
	}
	return strings.ToUpper(strings.ReplaceAll(s, "#", `\`))
}

// Group merges the keyboard/mouse collections into physical devices.
func Group(devs []sys.InputDevice) []facts.InputDev {
	var out []facts.InputDev
	index := map[string]int{}
	var protoMouse, protoKbd []bool
	var biMouse, biKbd []int
	for _, d := range devs {
		transport := TransportOf(d.InstanceID, d.Chain)
		key := d.USBNode
		if key == "" {
			key = btNode(d)
		}
		if key == "" {
			key = d.InstanceID
		}
		ukey := strings.ToUpper(key)
		i, ok := index[ukey]
		if !ok {
			vid, pid, _ := ParseVIDPID(key)
			if vid == 0 {
				vid, pid, _ = ParseVIDPID(d.InstanceID)
			}
			if vid == 0 {
				vid, pid, _ = ParseHIDVendor(d.InstanceID)
			}
			out = append(out, facts.InputDev{
				Key: key, Transport: transport, VID: vid, PID: pid,
				USBNode: d.USBNode, Speed: sys.USBSpeedUnknown, SelSuspend: -1,
			})
			protoMouse = append(protoMouse, false)
			protoKbd = append(protoKbd, false)
			biMouse = append(biMouse, 0)
			biKbd = append(biKbd, 0)
			i = len(out) - 1
			index[ukey] = i
		}
		g := &out[i]
		g.Collections = append(g.Collections, d.InstanceID)
		if !contains(g.Kinds, d.Kind) {
			g.Kinds = append(g.Kinds, d.Kind)
		}
		if d.USBInterface != "" && !containsFold(g.Interfaces, d.USBInterface) {
			g.Interfaces = append(g.Interfaces, d.USBInterface)
		}
		if d.USBNode != "" && g.Speed == sys.USBSpeedUnknown {
			g.Speed, g.BcdUSB, g.Port, g.HubDepth, g.HubIsRoot = d.Speed, d.BcdUSB, d.Port, d.HubDepth, d.HubIsRoot
			g.Controller, g.ControllerID = d.Controller, d.ControllerID
		}
		if g.Err == "" {
			g.Err = d.Err
		}
		for _, e := range d.Endpoints {
			if e.IsInterruptIn() && e.Class == 3 {
				protoMouse[i] = protoMouse[i] || e.Protocol == 2
				protoKbd[i] = protoKbd[i] || e.Protocol == 1
			}
		}
		// Interval of the collection's own interface, kept per kind: the
		// device's primary role decides which one is shown.
		if bi := PickInterval(d); bi > 0 {
			slot := &biKbd[i]
			if d.Kind == "mouse" {
				slot = &biMouse[i]
			}
			if *slot == 0 || bi < *slot {
				*slot = bi
			}
		}
		names := []string{d.Product, d.Name}
		if g.Name == "" || generic(g.Name) {
			for _, n := range names {
				if !generic(n) {
					g.Name = n
					break
				}
			}
		}
		if !g.Receiver {
			g.Receiver = IsReceiver(d.Product)
		}
		// The manufacturer Windows lists is often the driver's ("Microsoft"
		// for the in-box HID driver), not the device's.
		if g.Name == "" && d.Manufacturer != "" && !strings.HasPrefix(d.Manufacturer, "(") &&
			(!strings.EqualFold(strings.TrimSpace(d.Manufacturer), "Microsoft") || g.VID == 0x045E) {
			g.Name = d.Manufacturer
		}
	}
	for i := range out {
		g := &out[i]
		// Gaming keyboards often expose an extra boot-mouse interface (macros)
		// and gaming mice a keyboard one: the product name and the HID
		// collections Windows created say more than the boot protocol.
		name := strings.ToLower(g.Name)
		switch {
		case strings.Contains(name, "keyboard") || strings.Contains(name, "teclado"):
			g.Primary = "keyboard"
		case strings.Contains(name, "mouse"):
			g.Primary = "mouse"
		case g.Has("mouse") && !g.Has("keyboard"):
			g.Primary = "mouse"
		case g.Has("keyboard") && !g.Has("mouse"):
			g.Primary = "keyboard"
		case protoMouse[i] && !protoKbd[i]:
			g.Primary = "mouse"
		case protoKbd[i] && !protoMouse[i]:
			g.Primary = "keyboard"
		case g.Has("mouse"):
			g.Primary = "mouse"
		default:
			g.Primary = "keyboard"
		}
		g.BInterval = biKbd[i]
		if g.Primary == "mouse" || g.BInterval == 0 {
			if biMouse[i] > 0 {
				g.BInterval = biMouse[i]
			}
		}
		if g.Name == "" {
			kind := "Teclado"
			if g.Primary == "mouse" {
				kind = "Mouse"
				if g.Transport == TransportInternal && touchpadVendors[g.VID] {
					kind = "Touchpad"
				}
			}
			g.Name = kind
			if v := VendorName(g.VID); v != "" {
				g.Name = kind + " " + v
			}
		}
		if g.Transport == TransportUSB && g.Speed != sys.USBSpeedUnknown {
			g.NominalHz = NominalHz(g.Speed, g.BInterval)
		}
		// Product strings come with trailing spaces ("USB Gaming Keyboard ").
		g.Name = strings.TrimSpace(g.Name)
		sort.Strings(g.Kinds)
	}
	sort.SliceStable(out, func(a, b int) bool {
		ra, rb := rank(out[a]), rank(out[b])
		if ra != rb {
			return ra < rb
		}
		return strings.ToLower(out[a].Name) < strings.ToLower(out[b].Name)
	})
	return out
}

func rank(d facts.InputDev) int {
	r := 0
	if d.Primary != "mouse" {
		r += 2
	}
	if d.Transport == TransportVirtual {
		r += 4
	}
	return r
}

// btNode returns the Bluetooth device node of a collection, if any.
func btNode(d sys.InputDevice) string {
	for _, n := range d.Chain {
		up := strings.ToUpper(n)
		if strings.HasPrefix(up, `BTHLEDEVICE\`) || strings.HasPrefix(up, `BTHENUM\`) {
			return n
		}
	}
	return ""
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

// Find returns the physical device that owns an input collection.
func Find(devs []facts.InputDev, instance string) *facts.InputDev {
	for i := range devs {
		for _, c := range devs[i].Collections {
			if strings.EqualFold(c, instance) {
				return &devs[i]
			}
		}
	}
	return nil
}

// FormatHz renders a rate like "1000 Hz" or "62,5 Hz".
func FormatHz(hz float64) string {
	if hz <= 0 {
		return "?"
	}
	if math.Abs(hz-math.Round(hz)) < 0.05 {
		return strconv.Itoa(int(math.Round(hz))) + " Hz"
	}
	return strings.Replace(strconv.FormatFloat(hz, 'f', 1, 64), ".", ",", 1) + " Hz"
}

// FormatMS renders a period in milliseconds with a decimal comma.
func FormatMS(ms float64) string {
	prec := 1
	if ms < 1 {
		prec = 3
	}
	s := strconv.FormatFloat(ms, 'f', prec, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return strings.Replace(s, ".", ",", 1) + " ms"
}

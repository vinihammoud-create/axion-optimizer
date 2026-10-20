package drivers

import (
	"sort"
	"strings"
	"time"

	"axion/internal/sys"
)

// Installed is the driver in use by one device (Win32_PnPSignedDriver).
type Installed struct {
	Name         string    `json:"name"`
	Class        string    `json:"class"`
	Provider     string    `json:"provider"`
	Manufacturer string    `json:"manufacturer"`
	Version      string    `json:"version"`
	Date         time.Time `json:"date"`
	HardwareID   string    `json:"hardwareId"`
	DeviceID     string    `json:"deviceId"`
	Inf          string    `json:"inf"`
	Signer       string    `json:"signer"`
	Signed       bool      `json:"signed"`
	Generic      bool      `json:"generic"` // driver that ships with Windows (provider Microsoft)
}

// InventoryWQL is the query the inventory is built from.
const InventoryWQL = "SELECT DeviceName, DeviceClass, DriverProviderName, Manufacturer, DriverVersion, DriverDate, HardWareID, DeviceID, InfName, IsSigned, Signer FROM Win32_PnPSignedDriver"

// FromWMI converts the WMI rows, skipping entries without a driver.
func FromWMI(rows []sys.WMIRow) []Installed {
	var out []Installed
	for _, r := range rows {
		d := Installed{
			Name: strings.TrimSpace(r.Str("DeviceName")), Class: strings.ToUpper(strings.TrimSpace(r.Str("DeviceClass"))),
			Provider: strings.TrimSpace(r.Str("DriverProviderName")), Manufacturer: strings.TrimSpace(r.Str("Manufacturer")),
			Version: strings.TrimSpace(r.Str("DriverVersion")), Date: r.Time("DriverDate"),
			HardwareID: strings.TrimSpace(r.Str("HardWareID")), DeviceID: strings.TrimSpace(r.Str("DeviceID")),
			Inf: strings.TrimSpace(r.Str("InfName")), Signer: strings.TrimSpace(r.Str("Signer")), Signed: r.Bool("IsSigned"),
		}
		if d.Name == "" || d.DeviceID == "" {
			continue
		}
		d.Generic = strings.EqualFold(d.Provider, "Microsoft")
		out = append(out, d)
	}
	return out
}

// Component kinds, in the order the page shows them.
var componentOrder = []struct{ Kind, Label string }{
	{"gpu", "Placa de vídeo"},
	{"chipset", "Chipset e placa-mãe"},
	{"net", "Rede"},
	{"audio", "Áudio"},
	{"storage", "Armazenamento"},
	{"usb", "Controladores USB"},
	{"bluetooth", "Bluetooth"},
	{"input", "Mouse, teclado e controles"},
}

// ComponentLabel returns the label of a component kind.
func ComponentLabel(kind string) string {
	for _, c := range componentOrder {
		if c.Kind == kind {
			return c.Label
		}
	}
	return "Outros"
}

func physical(id string) bool {
	up := strings.ToUpper(id)
	return strings.HasPrefix(up, `PCI\`) || strings.HasPrefix(up, `USB\`) || strings.HasPrefix(up, `HDAUDIO\`) || strings.HasPrefix(up, `ACPI\`)
}

func vendorProvider(p string) bool {
	p = strings.ToLower(p)
	return strings.Contains(p, "amd") || strings.Contains(p, "advanced micro") || strings.Contains(p, "intel")
}

// KindOf places a device in a component, or "" when it is not shown.
func KindOf(d Installed) string {
	id := strings.ToUpper(d.DeviceID)
	switch d.Class {
	case "DISPLAY":
		return "gpu"
	case "NET":
		if strings.HasPrefix(id, `PCI\`) || strings.HasPrefix(id, `USB\`) {
			return "net"
		}
	case "MEDIA":
		if strings.HasPrefix(id, `HDAUDIO\`) || strings.HasPrefix(id, `USB\`) || strings.HasPrefix(id, `PCI\`) {
			return "audio"
		}
	case "SYSTEM":
		if physical(id) && !d.Generic && vendorProvider(d.Provider+" "+d.Manufacturer) {
			return "chipset"
		}
	case "HDC", "SCSIADAPTER":
		if physical(id) {
			return "storage"
		}
	case "USB":
		if strings.HasPrefix(id, `PCI\`) {
			return "usb"
		}
	case "BLUETOOTH":
		if strings.HasPrefix(id, `USB\`) || strings.HasPrefix(id, `PCI\`) {
			return "bluetooth"
		}
	case "HIDCLASS", "MOUSE", "KEYBOARD":
		if !d.Generic {
			return "input"
		}
	}
	return ""
}

// Component groups the devices of one kind.
type Component struct {
	Kind    string      `json:"kind"`
	Label   string      `json:"label"`
	Devices []Installed `json:"devices"`
}

// Group builds the components shown on the page, with duplicates (the same
// driver on several identical devices) collapsed.
func Group(all []Installed) []Component {
	by := map[string][]Installed{}
	seen := map[string]bool{}
	for _, d := range all {
		k := KindOf(d)
		if k == "" {
			continue
		}
		key := k + "|" + strings.ToLower(d.Name) + "|" + d.Version
		if seen[key] {
			continue
		}
		seen[key] = true
		by[k] = append(by[k], d)
	}
	var out []Component
	for _, c := range componentOrder {
		devs := by[c.Kind]
		if len(devs) == 0 {
			continue
		}
		sort.SliceStable(devs, func(i, j int) bool {
			if devs[i].Generic != devs[j].Generic {
				return !devs[i].Generic
			}
			return devs[i].Name < devs[j].Name
		})
		out = append(out, Component{Kind: c.Kind, Label: c.Label, Devices: devs})
	}
	return out
}

// Match finds the installed device a Windows Update offer is for: the same
// matched hardware ID, else a device instance whose ID starts with it
// (PCI\VEN_10EC&DEV_8125 matches PCI\VEN_10EC&DEV_8125&SUBSYS_…\01…).
func Match(all []Installed, hwid string) *Installed {
	h := strings.ToUpper(strings.TrimSpace(hwid))
	if h == "" {
		return nil
	}
	for i := range all {
		if strings.EqualFold(all[i].HardwareID, h) {
			return &all[i]
		}
	}
	for i := range all {
		id := strings.ToUpper(all[i].DeviceID)
		if strings.HasPrefix(id, h) && (len(id) == len(h) || id[len(h)] == '&' || id[len(h)] == '\\') {
			return &all[i]
		}
	}
	return nil
}

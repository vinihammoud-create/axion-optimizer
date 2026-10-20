package facts

import (
	"path/filepath"
	"sort"
	"strings"
)

// StepVulnDrivers is the collection step that looks for known vulnerable
// kernel drivers left on the PC.
const StepVulnDrivers = "Drivers de kernel vulneráveis"

// VulnDriverDef is a kernel driver that public catalogs list as vulnerable
// (it gives any program running as administrator access to physical memory,
// I/O ports or MSRs). The list is fixed and short on purpose: it is the
// Axion's criterion, not a complete catalog.
type VulnDriverDef struct {
	ID      string   // stable key, used by the rule to pick the sources
	Display string   // how the driver is called in the text
	Files   []string // file names, lower case
	Service []string // service names, lower case
	// Origin says what usually ships it ("costuma vir com"): never an
	// accusation, only where to look in the programs list.
	Origin string
}

// VulnDriverDefs is the list of drivers the Axion recognizes.
var VulnDriverDefs = []VulnDriverDef{
	{ID: "winring0", Display: "WinRing0 (WinRing0x64.sys)", Files: []string{"winring0x64.sys", "winring0.sys"}, Service: []string{"winring0_1_2_0", "winring0"},
		Origin: "versões antigas de monitores de hardware e utilitários de placa, como Libre Hardware Monitor, Open Hardware Monitor, HWiNFO, MSI Afterburner, OpenRGB, CapFrameX e ZenTimings"},
	{ID: "amifldrv", Display: "amifldrv64.sys (American Megatrends)", Files: []string{"amifldrv64.sys", "amifldrv.sys"}, Service: []string{"amifldrv64", "amifldrv"},
		Origin: "utilitários da American Megatrends que leem ou gravam a BIOS, como o SCEWIN, e alguns programas de atualização de BIOS"},
	{ID: "amigendrv", Display: "amigendrv64.sys (American Megatrends)", Files: []string{"amigendrv64.sys"}, Service: []string{"amigendrv64"},
		Origin: "utilitários da American Megatrends que leem ou gravam a BIOS, como o SCEWIN, e alguns programas de atualização de BIOS"},
	{ID: "rwdrv", Display: "rwdrv.sys (RWEverything)", Files: []string{"rwdrv.sys"}, Service: []string{"rwdrv"},
		Origin: "o RWEverything (RW-Everything), usado para mexer em registradores de hardware"},
}

// VulnDriver is one known vulnerable driver found on this PC.
type VulnDriver struct {
	ID      string `json:"id"`
	Display string `json:"display"`
	Origin  string `json:"origin"`
	// Loaded: the driver is in the kernel right now (seen in the module list).
	Loaded bool `json:"loaded"`
	// Registered: a kernel-driver service with that name or file exists.
	Registered bool `json:"registered"`
	// State and Start are the service state and start mode when registered.
	State string `json:"state,omitempty"`
	Start string `json:"start,omitempty"`
	// Path is the file of the registered service, when it has one.
	Path string `json:"path,omitempty"`
}

// VulnDriverScan is what the search read. Read is false when neither the list
// of loaded drivers nor the list of driver services could be read: the rule
// then says nothing (an empty list is not "clean").
type VulnDriverScan struct {
	Read  bool         `json:"read"`
	Found []VulnDriver `json:"found,omitempty"`
}

// DriverService is a kernel-driver service as the Service Control Manager
// lists it (Win32_SystemDriver).
type DriverService struct {
	Name  string
	Path  string
	State string
	Start string
}

// MatchVulnDrivers compares the loaded kernel modules (file names) and the
// driver services with VulnDriverDefs. It is pure so it can be tested without
// a PC: a nil list means "not read" and finds nothing from that source.
func MatchVulnDrivers(modules []string, services []DriverService) []VulnDriver {
	loaded := map[string]bool{}
	for _, m := range modules {
		loaded[strings.ToLower(strings.TrimSpace(m))] = true
	}
	var out []VulnDriver
	for _, d := range VulnDriverDefs {
		v := VulnDriver{ID: d.ID, Display: d.Display, Origin: d.Origin}
		for _, f := range d.Files {
			if loaded[f] {
				v.Loaded = true
			}
		}
		for _, s := range services {
			name := strings.ToLower(strings.TrimSpace(s.Name))
			base := strings.ToLower(driverBase(s.Path))
			if !contains(d.Service, name) && !contains(d.Files, base) {
				continue
			}
			v.Registered = true
			v.State, v.Start, v.Path = s.State, s.Start, strings.TrimSpace(s.Path)
			break
		}
		if v.Loaded || v.Registered {
			out = append(out, v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Loaded && !out[j].Loaded })
	return out
}

func contains(list []string, s string) bool {
	if s == "" {
		return false
	}
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// driverBase returns the file name of a service image path, which can come as
// \??\C:\dir\x.sys, "C:\dir\x.sys" or with arguments.
func driverBase(p string) string {
	p = strings.Trim(strings.TrimSpace(p), `"`)
	if i := strings.Index(strings.ToLower(p), ".sys"); i >= 0 {
		p = p[:i+4]
	}
	p = strings.ReplaceAll(p, `\`, "/")
	return filepath.Base(p)
}

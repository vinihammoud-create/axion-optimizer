package drivers

import (
	"strings"
	"testing"
	"time"

	"axion/internal/sys"
)

// Shapes captured from NVIDIA's services on 2026-09-26 (trimmed).
const productsXML = `<?xml version="1.0" encoding="utf-8"?>
<LookupValueSearch><LookupValues>
<LookupValue ParentID="120"><Name>GeForce RTX 3060 Ti</Name><Value>934</Value></LookupValue>
<LookupValue ParentID="120"><Name>GeForce RTX 3060</Name><Value>942</Value></LookupValue>
<LookupValue ParentID="127"><Name>GeForce RTX 4060</Name><Value>1023</Value></LookupValue>
<LookupValue ParentID="129"><Name>GeForce RTX 4060 Laptop GPU</Name><Value>1034</Value></LookupValue>
<LookupValue ParentID="999"><Name>   </Name><Value>1</Value></LookupValue>
</LookupValues></LookupValueSearch>`

const lookupJSON = `{"Success":"3","IDS":[
{"downloadInfo":{"Success":"1","ID":"279803","Name":"GeForce%20Game%20Ready%20Driver","Version":"617.14","IsBeta":"0","IsWHQL":"1","IsCRD":"0","ReleaseDateTime":"Tue Sep 22, 2026","DetailsURL":"https://www.nvidia.com/en-us/drivers/details/279803/","DownloadURL":"https://us.download.nvidia.com/Windows/617.14/617.14-desktop-win10-win11-64bit-international-dch-whql.exe","DownloadURLFileSize":"990.85 MB"}},
{"downloadInfo":{"ID":"279700","Name":"NVIDIA%20Studio%20Driver","Version":"616.92","IsBeta":"0","IsWHQL":"0","IsCRD":"1","ReleaseDateTime":"Wed Sep 9, 2026","DownloadURL":"https://us.download.nvidia.com/Windows/616.92/616.92-desktop-win10-win11-64bit-international-nsd-dch-whql.exe"}},
{"downloadInfo":{"ID":"279699","Name":"GeForce%20Game%20Ready%20Driver","Version":"616.92","IsBeta":"0","IsWHQL":"1","IsCRD":"0","ReleaseDateTime":"Wed Sep 9, 2026","DownloadURL":"https://us.download.nvidia.com/Windows/616.92/616.92-desktop-win10-win11-64bit-international-dch-whql.exe"}}
]}`

func TestNvidiaLookup(t *testing.T) {
	products, err := ParseProducts([]byte(productsXML))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]int{"NVIDIA GeForce RTX 3060": 942, "NVIDIA GeForce RTX 3060 Ti": 934, "NVIDIA  GeForce RTX 4060 Laptop GPU": 1034} {
		if id, ok := ProductID(products, name); !ok || id != want {
			t.Errorf("%s → %d %v, esperado %d", name, id, ok, want)
		}
	}
	if _, ok := ProductID(products, "NVIDIA GeForce RTX 3070"); ok {
		t.Error("produto inexistente encontrado")
	}
	q := LookupQuery(NvidiaLookupURL, 942, NvidiaOSID(true))
	for _, part := range []string{"func=DriverManualLookup", "pfid=942", "osID=135", "dch=1"} {
		if !strings.Contains(q, part) {
			t.Errorf("consulta sem %s: %s", part, q)
		}
	}
	list, err := ParseLookup([]byte(lookupJSON))
	if err != nil || len(list) != 3 {
		t.Fatalf("%v %d", err, len(list))
	}
	d := PickGameReady(list)
	if d == nil || d.Version != "617.14" || d.Name != "GeForce Game Ready Driver" || !d.WHQL || d.Studio || d.SizeLabel != "990.85 MB" ||
		!d.Released.Equal(time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("%+v", d)
	}
	if !list[1].Studio {
		t.Error("Studio não reconhecido")
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{{"617.14", "616.92", 1}, {"566.36", "566.36", 0}, {"560.94", "566.03", -1}, {"32.0.16.1714", "32.0.15.6094", 1}} {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("%s vs %s = %d", c.a, c.b, got)
		}
	}
}

func TestDownloadTarget(t *testing.T) {
	name, err := DownloadTarget("https://us.download.nvidia.com/Windows/617.14/617.14-desktop-win10-win11-64bit-international-dch-whql.exe")
	if err != nil || name != "617.14-desktop-win10-win11-64bit-international-dch-whql.exe" {
		t.Fatalf("%q %v", name, err)
	}
	for _, bad := range []string{
		"http://us.download.nvidia.com/Windows/617.14/x.exe",       // not https
		"https://download.nvidia.com.evil.io/x.exe",                // look-alike host
		"https://us.download.nvidia.com/Windows/617.14/x.zip",      // not an installer
		"https://user@us.download.nvidia.com/Windows/617.14/x.exe", // credentials
		"https://us.download.nvidia.com/Windows/617.14/..%5Cx.exe", // escaped separator
	} {
		if _, err := DownloadTarget(bad); err == nil {
			t.Errorf("aceito: %s", bad)
		}
	}
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func sampleInstalled() []Installed {
	return FromWMI([]sys.WMIRow{
		{"DeviceName": "NVIDIA GeForce RTX 3060", "DeviceClass": "DISPLAY", "DriverProviderName": "NVIDIA", "DriverVersion": "32.0.15.6636", "DriverDate": "20250115000000.******+000",
			"HardWareID": `PCI\VEN_10DE&DEV_2504&SUBSYS_397D1462&REV_A1`, "DeviceID": `PCI\VEN_10DE&DEV_2504&SUBSYS_397D1462&REV_A1\4&1E2A9C0&0&0019`, "IsSigned": true},
		{"DeviceName": "Realtek PCIe 2.5GbE Family Controller", "DeviceClass": "NET", "DriverProviderName": "Realtek", "DriverVersion": "10.68.815.2023", "DriverDate": "20230815000000.******+000",
			"HardWareID": `PCI\VEN_10EC&DEV_8125&SUBSYS_E0001458&REV_05`, "DeviceID": `PCI\VEN_10EC&DEV_8125&SUBSYS_E0001458&REV_05\01000000684CE00000`, "IsSigned": true},
		{"DeviceName": "WAN Miniport (IP)", "DeviceClass": "NET", "DriverProviderName": "Microsoft", "DeviceID": `SWD\MSRRAS\MS_NDISWANIP`},
		{"DeviceName": "SMBus", "DeviceClass": "SYSTEM", "DriverProviderName": "Microsoft", "DriverVersion": "10.0.26100.1", "DriverDate": "20060621000000.******+000",
			"HardWareID": `PCI\VEN_1022&DEV_790B&SUBSYS_50011458&REV_71`, "DeviceID": `PCI\VEN_1022&DEV_790B&SUBSYS_50011458&REV_71\3&11583659&0&A0`},
		{"DeviceName": "AMD GPIO Controller", "DeviceClass": "SYSTEM", "DriverProviderName": "Advanced Micro Devices, Inc", "DriverVersion": "2.2.0.130", "DriverDate": "20230601000000.******+000",
			"HardWareID": `ACPI\AMDI0030`, "DeviceID": `ACPI\AMDI0030\0`},
		{"DeviceName": "Standard NVM Express Controller", "DeviceClass": "SCSIADAPTER", "DriverProviderName": "Microsoft", "DriverVersion": "10.0.26100.1", "DeviceID": `PCI\VEN_15B7&DEV_5019\4&AB&0&0009`},
		{"DeviceName": "", "DeviceClass": "SYSTEM", "DeviceID": `ROOT\X\0000`},
	})
}

func TestGroupComponents(t *testing.T) {
	cs := Group(sampleInstalled())
	var kinds []string
	for _, c := range cs {
		kinds = append(kinds, c.Kind)
	}
	if strings.Join(kinds, ",") != "gpu,chipset,net,storage" {
		t.Fatalf("componentes: %v", kinds)
	}
	if cs[1].Devices[0].Name != "AMD GPIO Controller" || len(cs[1].Devices) != 1 {
		t.Errorf("chipset: %+v", cs[1].Devices) // the Microsoft SMBus driver is not a vendor chipset driver
	}
	if cs[2].Devices[0].Name != "Realtek PCIe 2.5GbE Family Controller" || len(cs[2].Devices) != 1 {
		t.Errorf("rede: %+v", cs[2].Devices)
	}
	if !cs[3].Devices[0].Generic {
		t.Error("driver da Microsoft deveria ser genérico")
	}
}

func TestEvaluateOffers(t *testing.T) {
	inst := sampleInstalled()
	offers := Evaluate([]sys.DriverUpdate{
		{UpdateID: "a", Title: "NVIDIA - Display - 32.0.15.6094", Class: "Display", HardwareID: `PCI\VEN_10DE&DEV_2504&SUBSYS_397D1462`, VerDate: day(2024, 8, 14), BrowseOnly: true},
		{UpdateID: "b", Title: "Realtek Semiconductor Corp. - Net - 1125.21.903.2025", Class: "Net", HardwareID: `PCI\VEN_10EC&DEV_8125&SUBSYS_E0001458&REV_05`, VerDate: day(2025, 9, 3), BrowseOnly: true},
		{UpdateID: "c", Title: "Advanced Micro Devices, Inc. - System - 7.6.2.123", Class: "System", HardwareID: `PCI\VEN_1022&DEV_790B&SUBSYS_50011458`, VerDate: day(2025, 6, 10)},
		{UpdateID: "d", Title: "Gigabyte - Firmware - 1.0.0.6", Class: "Firmware", HardwareID: `UEFI\RES_{AAAA}`, VerDate: day(2026, 1, 15), Model: "System Firmware"},
		{UpdateID: "e", Title: "Intel Corporation - Bluetooth - 23.100.0.3", Class: "Bluetooth", HardwareID: `USB\VID_8087&PID_0032`, VerDate: day(2025, 2, 20), Hidden: true},
	}, inst)
	by := map[string]Offer{}
	for _, o := range offers {
		by[o.UpdateID] = o
	}
	check := func(id, verdict string, sel bool) {
		t.Helper()
		if o := by[id]; o.Verdict != verdict || o.Select != sel {
			t.Errorf("%s: veredito %s seleção %v (%v)", id, o.Verdict, o.Select, o.Reasons)
		}
	}
	check("a", VerdictNo, false)    // older than the installed NVIDIA driver
	check("b", VerdictYes, true)    // newer Realtek driver
	check("c", VerdictYes, true)    // vendor driver over Windows' generic SMBus
	check("d", VerdictMaybe, false) // firmware
	check("e", VerdictYes, false)   // hidden: never pre-selected
	if o := by["b"]; o.Version != "1125.21.903.2025" || o.Device != "Realtek PCIe 2.5GbE Family Controller" || o.InstalledVersion != "10.68.815.2023" || o.Component != "net" {
		t.Errorf("%+v", o)
	}
	if o := by["c"]; !o.InstalledGeneric || !strings.Contains(o.Reasons[0], "genérico") || o.Component != "chipset" {
		t.Errorf("%+v", o)
	}
	if o := by["d"]; o.Device != "System Firmware" {
		t.Errorf("sem dispositivo correspondente deveria usar o modelo: %+v", o)
	}
	if offers[len(offers)-1].UpdateID != "e" {
		t.Error("ocultas vão para o fim")
	}
}

// After an install the old search result still lists the driver: it must
// read as the driver in use, not as a newer one to install.
func TestEvaluateOfferAlreadyInstalled(t *testing.T) {
	inst := sampleInstalled()
	offers := Evaluate([]sys.DriverUpdate{
		{UpdateID: "b", Title: "Realtek Semiconductor Corp. - Net - 10.68.815.2023", Class: "Net", HardwareID: `PCI\VEN_10EC&DEV_8125&SUBSYS_E0001458&REV_05`, VerDate: day(2025, 9, 3), BrowseOnly: true},
		{UpdateID: "x", Title: "Realtek Semiconductor Corp. - Net - 1125.21.903.2025", Class: "Net", HardwareID: `PCI\VEN_10EC&DEV_8125&SUBSYS_E0001458&REV_05`, VerDate: day(2025, 9, 3), BrowseOnly: true},
	}, inst)
	if len(offers) != 2 || offers[0].UpdateID != "x" {
		t.Fatalf("a oferta já instalada vai para o fim: %+v", offers)
	}
	if o := offers[1]; o.Verdict != VerdictInstalled || o.Select || len(o.Reasons) != 1 {
		t.Errorf("já instalado: %+v", o)
	}
}

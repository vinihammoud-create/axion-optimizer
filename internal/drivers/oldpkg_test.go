package drivers

import (
	"context"
	"strings"
	"testing"
	"time"

	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

// The shape is the real output of "pnputil /enum-drivers /devices /format xml"
// (Windows 11 10.0.26300), shortened: the two display packages of a notebook
// and two other packages with the same INF name.
const realPnpUtil = `<?xml version="1.0" encoding="utf-8"?>
<PnpUtil Version="10.0.26300" Command="/enum-drivers /devices /format xml">
    <Driver DriverName="oem172.inf">
        <OriginalName>iigd_dch.inf</OriginalName>
        <ProviderName>Intel Corporation</ProviderName>
        <ClassName>Display</ClassName>
        <ClassGuid>{4d36e968-e325-11ce-bfc1-08002be10318}</ClassGuid>
        <DriverVersion>06/17/2026 32.0.101.7088</DriverVersion>
        <SignerName>Microsoft Windows Hardware Compatibility Publisher</SignerName>
        <CatalogAttributes>
            <Attribute>Universal</Attribute>
        </CatalogAttributes>
        <WhcpVersion>Unknown</WhcpVersion>
        <Devices>
            <Device InstanceId="PCI\VEN_8086&amp;DEV_A78B&amp;SUBSYS_0BF81028&amp;REV_04\3&amp;11583659&amp;0&amp;10">
                <DeviceDescription>Intel(R) UHD Graphics</DeviceDescription>
                <Status>Started</Status>
            </Device>
        </Devices>
        <CatalogFile>igdlh.cat</CatalogFile>
        <Files>
            <File Name="igdkmdn64.sys"></File>
        </Files>
    </Driver>
    <Driver DriverName="oem148.inf">
        <OriginalName>nvdmi.inf</OriginalName>
        <ProviderName>NVIDIA</ProviderName>
        <ClassName>Display</ClassName>
        <ClassGuid>{4d36e968-e325-11ce-bfc1-08002be10318}</ClassGuid>
        <DriverVersion>09/17/2026 32.0.16.1714</DriverVersion>
        <Devices>
            <Device InstanceId="PCI\VEN_10DE&amp;DEV_25EC\4&amp;2a788c0f&amp;0&amp;0008">
                <DeviceDescription>NVIDIA GeForce RTX 3050 6GB Laptop GPU</DeviceDescription>
                <Status>Started</Status>
            </Device>
        </Devices>
    </Driver>
</PnpUtil>`

func TestParsePnpUtilRealShape(t *testing.T) {
	pkgs, err := ParsePnpUtil(realPnpUtil)
	if err != nil || len(pkgs) != 2 {
		t.Fatalf("%v %+v", err, pkgs)
	}
	nv := pkgs[1]
	if nv.Published != "oem148.inf" || nv.Original != "nvdmi.inf" || nv.Provider != "NVIDIA" || nv.Class != "Display" || nv.ClassGUID != DisplayClassGUID ||
		nv.Version != "32.0.16.1714" || nv.Date.Format("2006-01-02") != "2026-09-17" || nv.Devices != 1 || nv.Started != 1 {
		t.Errorf("NVIDIA: %+v", nv)
	}
	if pkgs[0].Version != "32.0.101.7088" || pkgs[0].Date.Month() != 6 {
		t.Errorf("Intel: %+v", pkgs[0])
	}
	// Nothing old on this notebook: one package per display adapter.
	if old := OldDisplayPackages(pkgs); len(old) != 0 {
		t.Errorf("pacotes antigos que não existem: %+v", old)
	}
}

func TestParsePnpUtilToleratesBadBytesAndGarbage(t *testing.T) {
	// A device name read through another code page leaves a byte that is not UTF-8.
	bad := strings.Replace(realPnpUtil, "Intel(R) UHD Graphics", "Controlador padr\xe3o", 1)
	if pkgs, err := ParsePnpUtil(bad); err != nil || len(pkgs) != 2 {
		t.Errorf("bytes inválidos: %v %d", err, len(pkgs))
	}
	for _, s := range []string{"", "not xml at all", "<PnpUtil></PnpUtil>", "<PnpUtil><Driver></Driver></PnpUtil>"} {
		if pkgs, err := ParsePnpUtil(s); err == nil && len(pkgs) != 0 {
			t.Errorf("%q: %+v", s, pkgs)
		}
	}
	// An unreadable date leaves the date empty; the version still counts.
	odd := `<PnpUtil><Driver DriverName="oem1.inf"><OriginalName>a.inf</OriginalName><DriverVersion>31/12/2026 1.2.3</DriverVersion></Driver></PnpUtil>`
	if pkgs, _ := ParsePnpUtil(odd); len(pkgs) != 1 || !pkgs[0].Date.IsZero() || pkgs[0].Version != "1.2.3" {
		t.Errorf("data ilegível: %+v", pkgs)
	}
}

func disp(pub, orig, prov, ver, date string, devices, started int) Package {
	p := Package{Published: pub, Original: orig, Provider: prov, Class: "Display", ClassGUID: DisplayClassGUID, Version: ver, Devices: devices, Started: started}
	if t, err := time.Parse("01/02/2006", date); err == nil {
		p.Date = t
	}
	return p
}

func TestOldDisplayPackages(t *testing.T) {
	pkgs := []Package{
		disp("oem10.inf", "nv_dispi.inf", "NVIDIA", "31.0.15.3623", "11/02/2023", 0, 0),
		disp("oem148.inf", "nv_dispi.inf", "NVIDIA", "32.0.15.9174", "09/17/2026", 1, 1), // the one running
		disp("oem20.inf", "nv_dispi.inf", "NVIDIA", "32.0.15.6094", "01/20/2025", 0, 0),
		disp("oem30.inf", "nv_dispi.inf", "NVIDIA", "32.0.15.7000", "03/01/2025", 1, 0), // installed on a device that is not running
		disp("oem40.inf", "nv_dispi.inf", "NVIDIA", "9.9.9.9", "01/01/2020", 0, 0),      // 9 < 32 as numbers
		disp("oem172.inf", "iigd_dch.inf", "Intel Corporation", "32.0.101.7088", "06/17/2026", 1, 1),
		// Another provider with the same INF name is another group.
		disp("oem50.inf", "nv_dispi.inf", "Outra Marca", "1.0.0.0", "01/01/2024", 0, 0),
		// Not display adapters: the same INF name does not mean an older version.
		{Published: "oem198.inf", Original: "firmware.inf", Provider: "Dell, Inc.", Class: "Firmware", ClassGUID: "{f2e7dd72-6468-4e36-b6f1-6488f42c1b52}", Version: "92.41.11.41"},
		{Published: "oem117.inf", Original: "firmware.inf", Provider: "Dell, Inc.", Class: "Firmware", ClassGUID: "{f2e7dd72-6468-4e36-b6f1-6488f42c1b52}", Version: "0.1.34.0"},
	}
	old := OldDisplayPackages(pkgs)
	got := map[string]string{}
	for _, o := range old {
		got[o.Published] = o.Newest
	}
	// The newest stays; the one on a running device stays; the rest are old.
	if len(got) != 3 || got["oem10.inf"] != "32.0.15.9174" || got["oem20.inf"] != "32.0.15.9174" || got["oem40.inf"] != "32.0.15.9174" {
		t.Errorf("antigos: %+v", old)
	}
	for _, keep := range []string{"oem148.inf", "oem30.inf", "oem172.inf", "oem50.inf", "oem198.inf", "oem117.inf"} {
		if _, ok := got[keep]; ok {
			t.Errorf("%s não deveria ser listado", keep)
		}
	}
	// The order is stable (by group, then newest to oldest).
	again := OldDisplayPackages(pkgs)
	for i := range old {
		if old[i].Published != again[i].Published {
			t.Errorf("ordem instável: %v %v", old, again)
		}
	}
}

// Same version and date: the later installation (higher oemNN) is the newer.
func TestOldPackagesTieBreakers(t *testing.T) {
	a := disp("oem71.inf", "nvppc.inf", "NVIDIA", "1.52.831.832", "11/18/2025", 0, 0)
	b := disp("oem143.inf", "nvppc.inf", "NVIDIA", "1.52.831.832", "11/18/2025", 0, 0)
	old := OldDisplayPackages([]Package{a, b})
	if len(old) != 1 || old[0].Published != "oem71.inf" {
		t.Errorf("empate: %+v", old)
	}
	// Same version, different dates: the later date wins.
	c := disp("oem90.inf", "x.inf", "P", "2.0", "01/01/2025", 0, 0)
	d := disp("oem80.inf", "x.inf", "P", "2.0", "06/01/2025", 0, 0)
	if old := OldDisplayPackages([]Package{c, d}); len(old) != 1 || old[0].Published != "oem90.inf" {
		t.Errorf("datas: %+v", old)
	}
	// One package alone is never old.
	if old := OldDisplayPackages([]Package{a}); len(old) != 0 {
		t.Errorf("sozinho: %+v", old)
	}
}

func TestCmpVersion(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{{"32.0.9", "32.0.16", -1}, {"32.0.16", "32.0.9", 1}, {"1.2", "1.2.0", 0}, {"10", "9", 1}, {"", "1", -1}, {"2.0.0.1", "2.0.0.0", 1}} {
		if got := cmpVersion(tc.a, tc.b); got != tc.want {
			t.Errorf("%q vs %q: %d, esperava %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestListPackagesFromTheMock(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pkgs, ok := ListPackages(context.Background(), m.Platform)
	if !ok || len(pkgs) < 3 {
		t.Fatalf("o simulador deveria listar pacotes: %v %+v", ok, pkgs)
	}
	if old := OldDisplayPackages(pkgs); len(old) != 1 || old[0].Provider != "NVIDIA" {
		t.Errorf("o simulador tem um pacote antigo de vídeo: %+v", old)
	}
	// A Windows that does not know /format: nothing is said.
	m.X.Handler = func(exe string, args []string, _ string) (sys.Result, bool) {
		if strings.Contains(strings.ToLower(exe), "pnputil") {
			return sys.Result{ExitCode: 1, Stdout: "Formato inválido."}, true
		}
		return sys.Result{}, false
	}
	if pkgs, ok := ListPackages(context.Background(), m.Platform); ok || pkgs != nil {
		t.Errorf("sem /format: %v %+v", ok, pkgs)
	}
}

// The delete never passes /uninstall or /force, so pnputil itself refuses a
// package that a device uses; and the simulator, like the real tool, removes a
// package nobody uses.
func TestRemovePackageNeverUninstalls(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := RemovePackage(ctx, m.Platform, "oem120.inf"); err != nil {
		t.Fatalf("pacote sem uso deveria sair: %v", err)
	}
	if err := RemovePackage(ctx, m.Platform, "oem148.inf"); err == nil || !strings.Contains(err.Error(), "presently installed") {
		t.Errorf("pacote em uso deveria ser recusado pelo pnputil: %v", err)
	}
	var deletes int
	for _, c := range m.X.Calls {
		for _, a := range c.Args {
			if la := strings.ToLower(a); la == "/uninstall" || la == "/force" {
				t.Errorf("RemovePackage passou %s: %v", a, c.Args)
			}
		}
		if len(c.Args) > 0 && c.Args[0] == "/delete-driver" {
			deletes++
		}
	}
	if deletes != 2 {
		t.Errorf("chamadas ao /delete-driver: %d, esperava 2", deletes)
	}
	pkgs, ok := ListPackages(ctx, m.Platform)
	if !ok {
		t.Fatal("ListPackages")
	}
	for _, p := range pkgs {
		if p.Published == "oem120.inf" {
			t.Error("o pacote excluído continua listado")
		}
	}
	if len(pkgs) != 2 {
		t.Errorf("sobraram %d pacotes, esperava 2 (os que estão em uso)", len(pkgs))
	}
}

func TestRemovePackageRefusesBadNames(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "oem.inf", "oem1.inf /force", `..\oem1.inf`, "oem1.inf\" /uninstall", "nvdmi.inf", "oem12", "oem 1.inf", "oem1.INF.bak"} {
		before := len(m.X.Calls)
		if err := RemovePackage(context.Background(), m.Platform, name); err == nil {
			t.Errorf("%q deveria ser recusado", name)
		}
		if len(m.X.Calls) != before {
			t.Errorf("%q: o pnputil não deveria ter sido chamado", name)
		}
	}
	// Both spellings of a real published name are accepted.
	for _, name := range []string{"oem120.inf", "OEM120.INF"} {
		m2, _ := mocksys.New(t.TempDir())
		if err := RemovePackage(context.Background(), m2.Platform, name); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
}

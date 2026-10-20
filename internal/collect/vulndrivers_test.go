package collect_test

import (
	"errors"
	"testing"

	"axion/internal/facts"
	"axion/internal/sys"
)

func vulnIDs(f *facts.Facts) map[string]facts.VulnDriver {
	out := map[string]facts.VulnDriver{}
	for _, d := range f.VulnDrivers.Found {
		out[d.ID] = d
	}
	return out
}

func TestVulnDriversCleanMachine(t *testing.T) {
	_, collectNow := collectMock(t)
	f := collectNow()
	if !f.VulnDrivers.Read || len(f.VulnDrivers.Found) != 0 {
		t.Errorf("simulador limpo: read=%v achados=%+v", f.VulnDrivers.Read, f.VulnDrivers.Found)
	}
	if !f.Read(facts.StepVulnDrivers) {
		t.Error("etapa marcada como não lida")
	}
}

func TestVulnDriversLoadedAndRegistered(t *testing.T) {
	m, collectNow := collectMock(t)
	m.N.Modules = []string{"ntoskrnl.exe", "ndis.sys", "WinRing0x64.sys"}
	m.W.Set(`root\cimv2`, "Win32_SystemDriver",
		sys.WMIRow{"Name": "Ndis", "PathName": `C:\Windows\system32\drivers\ndis.sys`, "State": "Running", "StartMode": "Boot"},
		sys.WMIRow{"Name": "WinRing0_1_2_0", "PathName": `\??\C:\Program Files\Monitor\WinRing0x64.sys`, "State": "Running", "StartMode": "Manual"},
		sys.WMIRow{"Name": "rwdrv", "PathName": `C:\Tools\rwdrv.sys`, "State": "Stopped", "StartMode": "Manual"})
	got := vulnIDs(collectNow())
	w, ok := got["winring0"]
	if !ok || !w.Loaded || !w.Registered || w.State != "Running" || w.Start != "Manual" {
		t.Errorf("winring0: %+v", w)
	}
	r, ok := got["rwdrv"]
	if !ok || r.Loaded || !r.Registered || r.State != "Stopped" {
		t.Errorf("rwdrv: %+v", r)
	}
	if len(got) != 2 {
		t.Errorf("achados a mais: %+v", got)
	}
}

// Without administrator rights the module list is an error: the services still
// answer, and nothing is said about "loaded".
func TestVulnDriversWithoutModuleList(t *testing.T) {
	m, collectNow := collectMock(t)
	m.N.ModulesErr = errors.New("lista indisponível")
	m.W.Set(`root\cimv2`, "Win32_SystemDriver", sys.WMIRow{"Name": "amifldrv64", "PathName": `C:\Windows\Temp\amifldrv64.sys`, "State": "Stopped", "StartMode": "Manual"})
	f := collectNow()
	d, ok := vulnIDs(f)["amifldrv"]
	if !f.VulnDrivers.Read || !ok || d.Loaded || !d.Registered {
		t.Errorf("read=%v achado=%+v", f.VulnDrivers.Read, d)
	}
}

// Zero data: neither source read, so the search is not "read" and finds nothing.
func TestVulnDriversNothingRead(t *testing.T) {
	m, collectNow := collectMock(t)
	m.N.ModulesErr = errors.New("sem administrador")
	m.W.Set(`root\cimv2`, "Win32_SystemDriver")
	f := collectNow()
	if f.VulnDrivers.Read || len(f.VulnDrivers.Found) != 0 {
		t.Errorf("sem dados deveria ficar não lido: %+v", f.VulnDrivers)
	}
	// An empty (not failed) module list is also "not read", never "clean".
	m.N.ModulesErr = nil
	m.N.Modules = []string{}
	if f := collectNow(); f.VulnDrivers.Read {
		t.Errorf("lista vazia de módulos e de serviços não é leitura: %+v", f.VulnDrivers)
	}
}

package collect_test

import (
	"context"
	"strings"
	"testing"

	"axion/internal/collect"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

// 0.6.3: the PCI devices are read with their Device Manager names (only the
// present ones), the Device Guard scenarios enabled in the registry are
// listed, and an anti-cheat whose service is not registered is still found
// by its entry in the installed programs (FACEIT and the Gamers Club on the
// desktop report).
func TestCollectPCIScenariosAndAntiCheatPrograms(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	u := `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\`
	_ = m.R.SetValue(u+`FACEIT`, "DisplayName", sys.SZ("FACEIT"))
	_ = m.R.SetValue(u+`FACEIT`, "UninstallString", sys.SZ(`"C:\Program Files\FACEIT AC\uninstall.exe"`))
	_ = m.R.SetValue(u+`FACEIT`, "Publisher", sys.SZ("FACEIT Ltd"))
	_ = m.R.SetValue(u+`{GC-1}`, "DisplayName", sys.SZ("Gamersclub Anti Cheat"))
	_ = m.R.SetValue(u+`{GC-1}`, "UninstallString", sys.SZ(`C:\Program Files\Gamers Club\unins000.exe`))
	_ = m.R.SetValue(u+`{GC-1}`, "Publisher", sys.SZ("Gamers Club Ltda"))
	_ = m.R.SetValue(u+`Riot Vanguard`, "DisplayName", sys.SZ("Riot Vanguard"))
	_ = m.R.SetValue(u+`Riot Vanguard`, "UninstallString", sys.SZ(`C:\Program Files\Riot Vanguard\uninstall.exe`))
	f := collect.Collect(context.Background(), m.Platform, nil)
	if len(f.PnPProblems) != 0 {
		t.Errorf("as linhas PCI do mock não são dispositivos com problema: %+v", f.PnPProblems)
	}

	names := map[string]bool{}
	for _, d := range f.PCIDevices {
		names[d.Name] = true
		if !strings.HasPrefix(strings.ToUpper(d.PNPID), `PCI\`) {
			t.Errorf("não é PCI: %+v", d)
		}
	}
	if !names["NVIDIA High Definition Audio"] || !names["AMD PCI Express Root Port"] || names["Placa antiga removida"] || names["USB Receiver"] {
		t.Errorf("dispositivos PCI: %v", names)
	}
	if n := f.PCIDeviceName("VEN_10DE&DEV_228E"); n != "NVIDIA High Definition Audio" {
		t.Errorf("nome pelo ID: %q", n)
	}
	if got := strings.Join(f.Security.Scenarios, ","); got != "HypervisorEnforcedCodeIntegrity" {
		t.Errorf("cenários ligados: %q", got)
	}
	if got := strings.Join(f.Security.ScenarioNames(), ","); !strings.Contains(got, "Integridade de Memória") {
		t.Errorf("nomes dos cenários: %q", got)
	}
	acs := strings.Join(f.Gaming.AntiCheats, ",")
	if !strings.Contains(acs, "FACEIT") || !strings.Contains(acs, "Gamers Club") || !strings.Contains(acs, "Vanguard") || strings.Count(acs, "FACEIT") != 1 {
		t.Errorf("anti-cheats pelos programas instalados: %q", acs)
	}
	// The service entry alone still counts, once.
	_, _ = m.R.CreateKey(`HKLM\SYSTEM\CurrentControlSet\Services\FACEITService`)
	if f := collect.Collect(context.Background(), m.Platform, nil); strings.Count(strings.Join(f.Gaming.AntiCheats, ","), "FACEIT") != 1 {
		t.Errorf("FACEIT contado duas vezes: %v", f.Gaming.AntiCheats)
	}
}

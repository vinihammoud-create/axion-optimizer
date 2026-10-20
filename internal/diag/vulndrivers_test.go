package diag_test

import (
	"context"
	"strings"
	"testing"

	"axion/internal/collect"
	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

func vulnFinding(f *facts.Facts) *diag.Finding {
	return find(diag.Build(f, nil), "sec.driver.vulneravel")
}

func TestVulnDriverFindingFromMock(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := collect.Collect(context.Background(), m.Platform, nil)
	if vulnFinding(f) != nil {
		t.Fatal("simulador limpo não deveria gerar o achado")
	}
	m.N.Modules = []string{"ntoskrnl.exe", "WinRing0x64.sys"}
	m.W.Set(`root\cimv2`, "Win32_SystemDriver", sys.WMIRow{"Name": "WinRing0_1_2_0", "PathName": `\??\C:\Apps\Monitor\WinRing0x64.sys`, "State": "Running", "StartMode": "Manual"})
	x := vulnFinding(collect.Collect(context.Background(), m.Platform, nil))
	if x == nil {
		t.Fatal("achado ausente")
	}
	if x.Severity != diag.SevWarning || x.Area != diag.AreaSeguranca {
		t.Errorf("severidade/área: %s %s", x.Severity, x.Area)
	}
	if len(x.Actions) != 1 || x.Actions[0].Kind != "page" || x.Actions[0].Page != "programas" {
		t.Errorf("a única ação é abrir Programas: %+v", x.Actions)
	}
	for _, want := range []string{"WinRing0", "carregado no kernel agora", "WinRing0x64.sys"} {
		if !strings.Contains(x.Evidence, want) {
			t.Errorf("evidência sem %q: %s", want, x.Evidence)
		}
	}
	for _, want := range []string{"costuma vir com", "Programas", "critério do Axion", "não carrega, não para nem apaga"} {
		if !strings.Contains(x.Explanation, want) {
			t.Errorf("explicação sem %q", want)
		}
	}
	if len(x.Sources) < 2 {
		t.Errorf("fontes: %+v", x.Sources)
	}
}

func TestVulnDriverFindingSilent(t *testing.T) {
	found := []facts.VulnDriver{{ID: "rwdrv", Display: "rwdrv.sys (RWEverything)", Origin: "o RWEverything", Loaded: true}}
	// Not read (zero data) or step set aside: silent even with data present.
	f := base()
	f.VulnDrivers = facts.VulnDriverScan{Read: false, Found: found}
	if vulnFinding(f) != nil {
		t.Error("leitura não concluída deveria ficar em silêncio")
	}
	f.VulnDrivers.Read = true
	f.Unread = []string{facts.StepVulnDrivers}
	if vulnFinding(f) != nil {
		t.Error("etapa não lida deveria ficar em silêncio")
	}
	f.Unread = nil
	f.VulnDrivers.Found = nil
	if vulnFinding(f) != nil {
		t.Error("sem driver deveria ficar em silêncio")
	}
	f.VulnDrivers.Found = found
	if x := vulnFinding(f); x == nil || len(x.Sources) != 1 {
		t.Errorf("rwdrv: %+v", x)
	}
	// Several drivers make one finding.
	f.VulnDrivers.Found = append(found, facts.VulnDriver{ID: "amifldrv", Display: "amifldrv64.sys (American Megatrends)", Origin: "o SCEWIN", Registered: true, State: "Stopped", Start: "Manual"})
	n := 0
	for _, x := range diag.Build(f, nil).Findings {
		if x.ID == "sec.driver.vulneravel" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d achados, esperado 1", n)
	}
}

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

// nvdrsReport sets the simulated NVIDIA profile database up, collects and
// builds the diagnostic.
func nvdrsReport(t *testing.T, setup func(m *mocksys.Mock)) *diag.Report {
	t.Helper()
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	setup(m)
	f := collect.Collect(context.Background(), m.Platform, nil)
	if len(f.Errors) > 0 {
		t.Fatalf("erros na coleta: %v", f.Errors)
	}
	return diag.Build(f, nil)
}

func withVRR(m *mocksys.Mock) {
	m.N.DRS.VRR[`\\.\DISPLAY1`] = sys.NVVRRInfo{Device: `\\.\DISPLAY1`, Read: true, Possible: true}
}

func TestNVDRSVSyncFindings(t *testing.T) {
	// Forced on, driver says no VRR: a warning with the fix.
	r := nvdrsReport(t, func(m *mocksys.Mock) { m.N.DRS.SetUser(sys.NVDRSBase, sys.NVDRSVSyncMode, sys.NVVSyncForceOn) })
	x := find(r, "gpu.nvdrs.vsync")
	if x == nil || x.Severity != diag.SevWarning || len(x.Actions) != 1 || x.Actions[0].Tweaks[0] != "fix.nvdrs.vsync" {
		t.Fatalf("V-Sync forçado sem VRR: %+v", x)
	}
	if !strings.Contains(x.Title, "sem G-SYNC/VRR ativo") || !strings.Contains(x.Evidence, "não informa VRR disponível") || !strings.Contains(x.Evidence, "Base Profile") {
		t.Errorf("texto: %s | %s", x.Title, x.Evidence)
	}
	if find(r, "gpu.nvdrs.vrr") != nil {
		t.Error("sem VRR não há o achado de VRR")
	}
	// Same value with a VRR monitor: no V-Sync finding, the VRR guide shows.
	r = nvdrsReport(t, func(m *mocksys.Mock) {
		m.N.DRS.SetUser(sys.NVDRSBase, sys.NVDRSVSyncMode, sys.NVVSyncForceOn)
		withVRR(m)
	})
	if find(r, "gpu.nvdrs.vsync") != nil {
		t.Error("com G-SYNC, V-Sync ligado no driver é válido")
	}
	if x := find(r, "gpu.nvdrs.vrr"); x == nil || x.Severity != diag.SevGood || !strings.Contains(x.Evidence, "possível, mas desligado") || !strings.Contains(x.Evidence, "ligado (forçado)") {
		t.Errorf("guia de VRR: %+v", x)
	}
	// VRR unknown (notebook with the panel on the integrated GPU): info only.
	r = nvdrsReport(t, func(m *mocksys.Mock) {
		m.N.DRS.SetUser(sys.NVDRSBase, sys.NVDRSVSyncMode, sys.NVVSyncForceOn)
		m.N.DRS.VRR = map[string]sys.NVVRRInfo{}
	})
	if x := find(r, "gpu.nvdrs.vsync"); x == nil || x.Severity != diag.SevInfo || strings.Contains(x.Evidence, "não informa VRR disponível") {
		t.Errorf("VRR desconhecido: %+v", x)
	}
	// An optimizer "revert" value: unknown to the driver, warning even with VRR.
	r = nvdrsReport(t, func(m *mocksys.Mock) { m.N.DRS.SetUser(sys.NVDRSBase, sys.NVDRSVSyncMode, 0x60925619); withVRR(m) })
	if x := find(r, "gpu.nvdrs.vsync"); x == nil || x.Severity != diag.SevWarning || !strings.Contains(x.Title, "valor que o driver não conhece") || !strings.Contains(x.Evidence, "fora da lista") {
		t.Errorf("valor inválido: %+v", x)
	}
	// Forced off is the user's choice: nothing.
	r = nvdrsReport(t, func(m *mocksys.Mock) { m.N.DRS.SetUser(sys.NVDRSBase, sys.NVDRSVSyncMode, sys.NVVSyncForceOff) })
	if find(r, "gpu.nvdrs.vsync") != nil {
		t.Error("V-Sync desligado não é achado")
	}
}

func TestNVDRSFRLFindings(t *testing.T) {
	limit := func(v uint32, vrr bool) func(m *mocksys.Mock) {
		return func(m *mocksys.Mock) {
			m.N.Displays[0].CurrentHz = 240
			m.N.DRS.SetUser("app:cs2.exe", sys.NVDRSFrameRateLimit, v)
			if vrr {
				withVRR(m)
			}
		}
	}
	r := nvdrsReport(t, limit(120, false))
	x := find(r, "gpu.nvdrs.frl.cs2.exe")
	if x == nil || x.Severity != diag.SevWarning || len(x.Actions) != 1 || x.Actions[0].Tweaks[0] != "fix.nvdrs.frl" {
		t.Fatalf("limite abaixo da taxa sem VRR: %+v", x)
	}
	if !strings.Contains(x.Evidence, "120 FPS no perfil do jogo") || !strings.Contains(x.Evidence, "240 Hz") {
		t.Errorf("evidência: %s", x.Evidence)
	}
	if x := find(nvdrsReport(t, limit(237, true)), "gpu.nvdrs.frl.cs2.exe"); x == nil || x.Severity != diag.SevGood || len(x.Actions) != 0 {
		t.Errorf("237 em 240 Hz com G-SYNC é o recomendado: %+v", x)
	}
	if x := find(nvdrsReport(t, limit(239, true)), "gpu.nvdrs.frl.cs2.exe"); x == nil || x.Severity != diag.SevInfo || len(x.Actions) != 0 || !strings.Contains(x.Title, "perto demais") {
		t.Errorf("239 em 240 Hz: %+v", x)
	}
	if x := find(nvdrsReport(t, limit(120, true)), "gpu.nvdrs.frl.cs2.exe"); x == nil || x.Severity != diag.SevInfo || len(x.Actions) != 1 || !strings.Contains(x.Title, "bem abaixo") {
		t.Errorf("120 em 240 Hz com G-SYNC: %+v", x)
	}
	// The current rate decides, not the highest available (60 Hz now).
	r = nvdrsReport(t, func(m *mocksys.Mock) { m.N.DRS.SetUser("app:cs2.exe", sys.NVDRSFrameRateLimit, 237); withVRR(m) })
	if x := find(r, "gpu.nvdrs.frl.cs2.exe"); x == nil || x.Severity != diag.SevInfo || !strings.Contains(x.Title, "perto demais") || !strings.Contains(x.Evidence, "60 Hz (suporta 240 Hz") {
		t.Errorf("237 com o monitor a 60 Hz: %+v", x)
	}
	// VRR unknown: info, no action, no "sem VRR" claim.
	r = nvdrsReport(t, func(m *mocksys.Mock) {
		m.N.Displays[0].CurrentHz = 240
		m.N.DRS.SetUser("app:cs2.exe", sys.NVDRSFrameRateLimit, 120)
		m.N.DRS.VRR = map[string]sys.NVVRRInfo{}
	})
	if x := find(r, "gpu.nvdrs.frl.cs2.exe"); x == nil || x.Severity != diag.SevInfo || len(x.Actions) != 0 || strings.Contains(x.Explanation, "Sem VRR,") {
		t.Errorf("VRR desconhecido: %+v", x)
	}
	// A limit inherited from the global profile is reported per game.
	r = nvdrsReport(t, func(m *mocksys.Mock) {
		m.N.Displays[0].CurrentHz = 240
		m.N.DRS.SetUser(sys.NVDRSBase, sys.NVDRSFrameRateLimit, 100)
	})
	if x := find(r, "gpu.nvdrs.frl.cs2.exe"); x == nil || !strings.Contains(x.Evidence, "no perfil global") {
		t.Errorf("limite herdado do global: %+v", x)
	}
	// Rule shared with the catalog.
	if !facts.NVFRLFixable(120, 240, false, true) || facts.NVFRLFixable(237, 240, true, true) || facts.NVFRLFixable(120, 240, false, false) || !facts.NVFRLFixable(120, 240, true, true) || facts.NVFRLFixable(300, 240, false, true) {
		t.Error("NVFRLFixable")
	}
}

func TestNVDRSShaderAndResidueFindings(t *testing.T) {
	r := nvdrsReport(t, func(m *mocksys.Mock) { m.N.DRS.SetUser(sys.NVDRSBase, sys.NVDRSShaderCache, 0) })
	if x := find(r, "gpu.nvdrs.shadercache"); x == nil || x.Severity != diag.SevWarning || x.Actions[0].Tweaks[0] != "fix.nvdrs.shadercache" {
		t.Errorf("cache de shader: %+v", x)
	}
	if find(nvdrsReport(t, func(*mocksys.Mock) {}), "gpu.nvdrs.shadercache") != nil {
		t.Error("mock limpo não tem o achado")
	}
	pack := func(m *mocksys.Mock) {
		d := m.N.DRS
		d.SetUser(sys.NVDRSBase, sys.NVDRSPowerMode, sys.NVPowerPreferMax)
		d.SetUser(sys.NVDRSBase, sys.NVDRSTextureQuality, 0x14)
		d.SetUser(sys.NVDRSBase, sys.NVDRSOGLThreads, 1)
		d.SetUser(sys.NVDRSBase, sys.NVDRSUltraLowLatency, 1)
		d.SetUser(sys.NVDRSBase, sys.NVDRSOGLTripleBuffer, 0) // equal to the default: not counted
	}
	x := find(nvdrsReport(t, pack), "res.nvdrs")
	if x == nil || x.Severity != diag.SevInfo || !strings.HasPrefix(x.Title, "4 ajustes") || x.Actions[0].Tweaks[0] != "fix.nvdrs.residue" {
		t.Fatalf("resíduo: %+v", x)
	}
	for _, frag := range []string{"Modo de gerenciamento de energia = Preferir desempenho máximo (padrão do driver: Ideal (padrão))", "Filtragem de textura - qualidade = Alto desempenho", "Otimização em threads = ligado", "Baixa latência Ultra = ligado", "5 ajustes do perfil global têm valor de usuário"} {
		if !strings.Contains(x.Evidence, frag) {
			t.Errorf("evidência sem %q: %s", frag, x.Evidence)
		}
	}
	if strings.Contains(x.Evidence, "(0x") {
		t.Errorf("valor numérico na evidência: %s", x.Evidence)
	}
	r = nvdrsReport(t, func(m *mocksys.Mock) {
		m.N.DRS.SetUser(sys.NVDRSBase, sys.NVDRSPowerMode, sys.NVPowerPreferMax)
		m.N.DRS.SetUser(sys.NVDRSBase, sys.NVDRSOGLThreads, 1)
	})
	if find(r, "res.nvdrs") != nil {
		t.Error("2 ajustes ficam abaixo do limite de 3")
	}
	// Without NVAPI nothing is said about the driver profile.
	r = nvdrsReport(t, func(m *mocksys.Mock) { m.N.DRS.Absent = true })
	for _, x := range r.Findings {
		if strings.HasPrefix(x.ID, "gpu.nvdrs.") || x.ID == "res.nvdrs" {
			t.Errorf("achado sem leitura do DRS: %s", x.ID)
		}
	}
}

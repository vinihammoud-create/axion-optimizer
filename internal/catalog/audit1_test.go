package catalog_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"axion/internal/collect"
	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// Priority 1 findings of the catalog audits (docs/audit/CATALOGO-1.md and
// CATALOGO-2.md): a hardware key offered as safe and one-click, a tweak that
// names the discrete GPU and lands on the integrated one, and three places
// where a read that failed turned into a normal state or a recommendation.

// failingInput answers InputDevices with an error, as the real machine does
// when the device enumeration is denied.
type failingInput struct {
	sys.Native
}

func (failingInput) InputDevices() ([]sys.InputDevice, error) {
	return nil, errors.New("acesso negado (simulado)")
}

// failingRegValue answers with an error for one registry value name.
type failingRegValue struct {
	sys.Registry
	name string
}

func (r failingRegValue) GetValue(path, name string) (sys.RegValue, error) {
	if name == r.name {
		return sys.RegValue{}, errors.New("acesso negado (simulado)")
	}
	return r.Registry.GetValue(path, name)
}

// CATALOGO-2 F01: input.usb.power writes SelectiveSuspendEnabled in the
// device's own hardware key (Enum\<nó>\Device Parameters), which the review
// rules keep out of the safe risk and of the one-click tier; and no source
// measures the gain in game, so the evidence is not high.
func TestUSBPowerRiskAndEvidence(t *testing.T) {
	_, _, e := setup(t)
	tw := e.Catalog.Get("input.usb.power")
	if tw == nil {
		t.Fatal("input.usb.power não existe")
	}
	if tw.OneClick || tw.Risk == tweaks.RiskSafe {
		t.Errorf("grava na chave de hardware do dispositivo: risco %s, 1 clique=%v", tw.Risk, tw.OneClick)
	}
	if tw.Evidence == tweaks.EvHigh {
		t.Error("evidência alta pede medição ou ganho documentado; as fontes só descrevem o mecanismo")
	}
}

// CATALOGO-2 F02: peripherals that could not be read are not "no mouse or
// keyboard can be suspended".
func TestUSBPowerUnreadPeripherals(t *testing.T) {
	m, _, e := setup(t)
	p := *m.Platform
	p.Native = failingInput{Native: m.Native}
	f := collect.Collect(context.Background(), &p, nil)
	if f.Peripherals.Err == "" {
		t.Fatal("a leitura dos periféricos deveria ter falhado")
	}
	ok, why := e.Catalog.Get("input.usb.power").IsApplicable(f)
	if ok {
		t.Fatal("sem a lista de dispositivos o ajuste não tem o que fazer")
	}
	if !strings.Contains(why, "não lid") {
		t.Errorf("leitura que falhou virou estado normal: %q", why)
	}
}

// CATALOGO-1 F30: gpu.msi.mode fala em GPU dedicada; num PC só com vídeo
// integrado, PrimaryGPU devolve a integrada e o ajuste gravaria na chave PCI
// dela.
func TestMSIModeOnlyOnDiscreteGPU(t *testing.T) {
	_, f, e := setup(t)
	tw := e.Catalog.Get("gpu.msi.mode")
	if tw == nil {
		t.Fatal("gpu.msi.mode não existe")
	}
	if ok, why := tw.IsApplicable(f); !ok {
		t.Fatalf("o PC simulado tem GPU dedicada PCI: deveria se aplicar (%s)", why)
	}
	f.GPUs = []facts.GPUInfo{{
		Name: "AMD Radeon(TM) Graphics", Vendor: "AMD", Integrated: true,
		PNPDeviceID: `PCI\VEN_1002&DEV_164E&SUBSYS_88771043&REV_C1\4&1A2B3C4D&0&0041`,
	}}
	if ok, _ := tw.IsApplicable(f); ok {
		t.Error("num PC só com vídeo integrado o ajuste da \"GPU dedicada\" não pode se aplicar")
	}
}

// CATALOGO-2 F22: PagingFiles that could not be read is not "already managed
// by Windows".
func TestPagefileUnreadableIsNotManaged(t *testing.T) {
	m, _, e := setup(t)
	p := *m.Platform
	p.Reg = failingRegValue{Registry: m.R, name: "PagingFiles"}
	f := collect.Collect(context.Background(), &p, nil)
	if f.Residue.PageFile != "unknown" {
		t.Fatalf("PagingFiles ilegível: PageFile = %q", f.Residue.PageFile)
	}
	ok, why := e.Catalog.Get("fix.pagefile").IsApplicable(f)
	if ok {
		t.Fatal("sem ler a configuração não dá para dizer que ela está errada")
	}
	if !strings.Contains(why, "não lid") {
		t.Errorf("leitura que falhou virou estado normal: %q", why)
	}
}

// CATALOGO-2 F26: with the gaming step unread, fix.bcd.hypervisor must not be
// recommended — a read that failed is not a reason to change the boot.
func TestHypervisorUnreadGamingIsNeutral(t *testing.T) {
	m, _, e := setup(t)
	m.BCD["hypervisorlaunchtype"] = "Off"
	f := collect.CollectWith(context.Background(), m.Platform, nil,
		collect.Options{Skip: map[string]bool{facts.StepGaming: true}})
	if f.Read(facts.StepGaming) {
		t.Fatal("a etapa de jogos deveria constar como não lida")
	}
	tw := e.Catalog.Get("fix.bcd.hypervisor")
	if ok, why := tw.IsApplicable(f); !ok {
		t.Fatalf("o hipervisor está Off: deveria se aplicar (%s)", why)
	}
	if r := tw.Recommendation(f); r.Level != tweaks.RecNeutral {
		t.Errorf("leitura que falhou virou recomendação: %+v", r)
	}
}

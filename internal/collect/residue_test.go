package collect_test

import (
	"context"
	"errors"
	"testing"

	"axion/internal/collect"
	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

func collectMock(t *testing.T) (*mocksys.Mock, func() *facts.Facts) {
	t.Helper()
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return m, func() *facts.Facts { return collect.Collect(context.Background(), m.Platform, nil) }
}

// NetworkThrottlingIndex: Windows ships 10; "competitive" optimizer packs
// write 0xFFFFFFFF. An absent value must never count as a leftover.
func TestResidueNetworkThrottling(t *testing.T) {
	m, collectNow := collectMock(t)
	if r := collectNow().Residue; r.NetworkThrottling != 10 || r.NetworkThrottlingOff() {
		t.Fatalf("padrão do simulador: %d, off=%v", r.NetworkThrottling, r.NetworkThrottlingOff())
	}
	_ = m.R.SetValue(facts.KeySystemProfile, "NetworkThrottlingIndex", sys.DWord(0xFFFFFFFF))
	if r := collectNow().Residue; r.NetworkThrottling != 0xFFFFFFFF || !r.NetworkThrottlingOff() {
		t.Errorf("0xFFFFFFFF: %d, off=%v", r.NetworkThrottling, r.NetworkThrottlingOff())
	}
	_ = m.R.SetValue(facts.KeySystemProfile, "NetworkThrottlingIndex", sys.DWord(20))
	if r := collectNow().Residue; r.NetworkThrottling != 20 || r.NetworkThrottlingOff() {
		t.Errorf("valor 20 é um ajuste válido do MMCSS, não resíduo: %d, off=%v", r.NetworkThrottling, r.NetworkThrottlingOff())
	}
	_ = m.R.DeleteValue(facts.KeySystemProfile, "NetworkThrottlingIndex")
	if r := collectNow().Residue; r.NetworkThrottling != -1 || r.NetworkThrottlingOff() {
		t.Errorf("valor ausente: %d, off=%v", r.NetworkThrottling, r.NetworkThrottlingOff())
	}
}

// Services that debloat packs turn off and the Axion now recognizes.
func TestResidueServices(t *testing.T) {
	m, collectNow := collectMock(t)
	for _, name := range []string{"XblGameSave", "TabletInputService"} {
		if r := collectNow().Residue; hasDisabled(r.DisabledServices, name) {
			t.Fatalf("%s não deveria estar na lista com o serviço no padrão", name)
		}
	}
	m.S.Add("XblGameSave", "Xbox Live Game Save", sys.StartDisabled, false)
	m.S.Add("TabletInputService", "Touch Keyboard and Handwriting Panel Service", sys.StartDisabled, false)
	r := collectNow().Residue
	for _, name := range []string{"XblGameSave", "TabletInputService"} {
		d, ok := findDisabled(r.DisabledServices, name)
		if !ok {
			t.Errorf("%s desativado deveria ser detectado", name)
			continue
		}
		if d.Group != "apps" || d.Default != sys.StartManual {
			t.Errorf("%s: grupo %q, padrão %v", name, d.Group, d.Default)
		}
	}
	// Manual is the shipped state: a service set back to it is not a leftover.
	_ = m.S.SetStart("XblGameSave", sys.StartManual)
	if hasDisabled(collectNow().Residue.DisabledServices, "XblGameSave") {
		t.Error("serviço em Manual não é resíduo")
	}
}

func findDisabled(list []facts.DisabledService, name string) (facts.DisabledService, bool) {
	for _, d := range list {
		if d.Name == name {
			return d, true
		}
	}
	return facts.DisabledService{}, false
}

func hasDisabled(list []facts.DisabledService, name string) bool {
	_, ok := findDisabled(list, name)
	return ok
}

// failingReg answers with an error for the values named in fail, as the real
// registry does when a key denies access.
type failingReg struct {
	sys.Registry
	fail map[string]bool
}

func (r failingReg) GetValue(path, name string) (sys.RegValue, error) {
	if r.fail[name] {
		return sys.RegValue{}, errors.New("acesso negado (simulado)")
	}
	return r.Registry.GetValue(path, name)
}

func collectWithFailingReads(t *testing.T, names ...string) *facts.Facts {
	t.Helper()
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fail := map[string]bool{}
	for _, n := range names {
		fail[n] = true
	}
	p := *m.Platform
	p.Reg = failingReg{Registry: m.R, fail: fail}
	return collect.Collect(context.Background(), &p, nil)
}

// A PagingFiles value that could not be read is unknown, never "none".
func TestPagingFilesUnreadableIsUnknown(t *testing.T) {
	if r := collectWithFailingReads(t, "PagingFiles").Residue; r.PageFile != "unknown" {
		t.Errorf("PagingFiles ilegível: PageFile = %q, esperado \"unknown\"", r.PageFile)
	}
	_, collectNow := collectMock(t)
	if r := collectNow().Residue; r.PageFile == "unknown" || r.PageFile == "none" {
		t.Errorf("leitura normal não pode virar %q", r.PageFile)
	}
}

// A MouseSpeed that could not be read is not "acceleration on".
func TestMouseSpeedUnreadableIsNotRead(t *testing.T) {
	if g := collectWithFailingReads(t, "MouseSpeed").Gaming; g.MouseAccelRead || g.MouseAccelOn {
		t.Errorf("MouseSpeed ilegível: lido=%v ligada=%v", g.MouseAccelRead, g.MouseAccelOn)
	}
	_, collectNow := collectMock(t)
	if g := collectNow().Gaming; !g.MouseAccelRead || !g.MouseAccelOn {
		t.Errorf("leitura normal (MouseSpeed=1): lido=%v ligada=%v", g.MouseAccelRead, g.MouseAccelOn)
	}
}

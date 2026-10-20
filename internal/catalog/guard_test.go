package catalog_test

import (
	"regexp"
	"strings"
	"testing"

	"axion/internal/catalog"
	"axion/internal/tweaks"
)

// These tests keep the catalog honest about its own rules. They exist because
// a third-party change once added items that the "O que não fazemos" page
// already rejected, touched a protected system process and put interrupt
// routing in the one-click tier, and every existing test still passed.
//
// They only catch what is already catalogued: they cannot judge whether a new
// claim is true. Anything that comes from outside (an assistant, a pull
// request) still needs its sources opened and read.

var ifeoProcess = regexp.MustCompile(`image file execution options\\([^\\|]+)\\`)

// excludedHit reports the "O que não fazemos" entry whose protected target
// appears in an operation target.
func excludedHit(target string) (string, bool) {
	for _, ex := range catalog.Excluded() {
		for _, frag := range ex.Targets {
			if strings.Contains(target, strings.ToLower(frag)) {
				return ex.Name, true
			}
		}
	}
	return "", false
}

// systemProcessHit reports a protected Windows process whose Image File
// Execution Options an operation would write.
func systemProcessHit(target string) (string, bool) {
	if m := ifeoProcess.FindStringSubmatch(target); m != nil && catalog.SystemExe(m[1]) {
		return m[1], true
	}
	return "", false
}

func TestCatalogHonoursExclusions(t *testing.T) {
	_, f, _ := setup(t)
	for _, tw := range catalog.All() {
		if tw.Category == "correcoes" {
			continue // the fixes exist to undo exactly these changes
		}
		for _, op := range tw.Ops(f) {
			if name, bad := excludedHit(op.Target()); bad {
				t.Errorf("%s grava %s, que a página \"O que não fazemos\" recusa (%s)", tw.ID, op.Target(), name)
			}
		}
	}
}

func TestNoTweakTouchesSystemProcesses(t *testing.T) {
	_, f, _ := setup(t)
	for _, tw := range catalog.All() {
		for _, op := range tw.Ops(f) {
			if exe, bad := systemProcessHit(op.Target()); bad {
				t.Errorf("%s altera o IFEO de %s, um processo do Windows que o Axion não toca (%s)", tw.ID, exe, op.Target())
			}
		}
	}
	// A per-game profile must refuse a system process, whatever the feature.
	for _, exe := range []string{"csrss.exe", "dwm.exe", "audiodg.exe", "explorer.exe", "svchost.exe", "lsass.exe"} {
		for _, feature := range catalog.ProfileFeatures {
			tw := catalog.ProfileProvider(catalog.ProfileID(exe, feature), f)
			if tw == nil {
				continue
			}
			if ok, _ := tw.Applicable(f); ok {
				t.Errorf("o perfil %s de %s é aplicável, mas é um processo do sistema", feature, exe)
			}
		}
	}
}

// Interrupt routing (MSI, affinity policy) lives in the device's own hardware
// key: a device that handles it badly loses its connection or does not start.
// The documented mechanism has no published latency gain, so these items are
// manual, never recommended in one click.
func TestInterruptRoutingIsNeverOneClick(t *testing.T) {
	_, f, _ := setup(t)
	seen := 0
	for _, tw := range catalog.All() {
		for _, op := range tw.Ops(f) {
			if !strings.Contains(op.Target(), `\interrupt management\`) {
				continue
			}
			seen++
			if tw.OneClick || tw.Risk == tweaks.RiskSafe || tw.Evidence == tweaks.EvHigh {
				t.Errorf("%s mexe em Interrupt Management (%s): precisa ser manual, risco acima de seguro e evidência abaixo de alta", tw.ID, op.Target())
			}
		}
	}
	if seen == 0 {
		t.Skip("a máquina simulada não tem GPU/placa de rede PCI com itens de interrupção")
	}
}

// The guards must actually bite: a hit on each rule is detected.
func TestGuardsDetectKnownViolations(t *testing.T) {
	for _, target := range []string{
		`registry-merge:hkcu\software\microsoft\directx\usergpupreferences|directxuserglobalsettings|maxshadercachesizeinmb`,
		`reg:hklm\software\microsoft\windows nt\currentversion\image file execution options\game.exe\perfoptions|affinitymask`,
		`reg:hklm\system\currentcontrolset\control\class\{4d36e972-e325-11ce-bfc1-08002be10318}\0001|*packetcoalescing`,
		`reg:hkcu\software\microsoft\windows nt\currentversion\image file execution options\cs2.exe\perfoptions|cpupriorityclass`,
		// Added with the residue review (18/10/2026).
		`reg:hklm\system\currentcontrolset\enum\pci\ven_10de&dev_25ec\4&1\device parameters\interrupt management\affinity policy|devicepriority`,
		`reg:hklm\software\microsoft\windows nt\currentversion\image file execution options\cs2.exe\perfoptions|iopriority`,
		`reg:hklm\software\microsoft\windows nt\currentversion\multimedia\systemprofile|networkthrottlingindex`,
		`reg:hklm\software\microsoft\windows nt\currentversion\multimedia\systemprofile\tasks\games|scheduling category`,
		`reg:hklm\software\microsoft\windows nt\currentversion\multimedia\systemprofile\tasks\games|priority`,
		`mmagent:memorycompression`,
		// NVIDIA profile pack in the global profile (0.9).
		`nvdrs:base|1057eb71`,
		`nvdrs:global|10835000`,
	} {
		if _, bad := excludedHit(target); !bad {
			t.Errorf("o guarda não pegou %s", target)
		}
	}
	if _, bad := excludedHit(`reg:hklm\software\microsoft\windows nt\currentversion\image file execution options\cs2.exe\perfoptions|cpupriorityclass`); bad {
		t.Error("a prioridade por jogo em HKLM é a forma correta e não pode ser barrada")
	}
	if _, bad := excludedHit(`nvdrs:app:cs2.exe|1057eb71`); bad {
		t.Error("o modo de energia no perfil de um jogo é a forma permitida e não pode ser barrado")
	}
	if exe, bad := systemProcessHit(`reg:hklm\software\microsoft\windows nt\currentversion\image file execution options\audiodg.exe\perfoptions|cpupriorityclass`); !bad || exe != "audiodg.exe" {
		t.Error("o guarda não pegou o IFEO do audiodg.exe")
	}
	if _, bad := systemProcessHit(`reg:hklm\software\microsoft\windows nt\currentversion\image file execution options\cs2.exe\perfoptions|cpupriorityclass`); bad {
		t.Error("cs2.exe não é um processo do sistema")
	}
}

package catalog_test

import (
	"strings"
	"testing"

	"axion/internal/facts"
	"axion/internal/tweaks"
)

// Priority 2 findings of the catalog audits (docs/audit/CATALOGO-1.md and
// CATALOGO-2.md): sources that do not say what the text claims. Where no
// readable source sustains the claim, the evidence goes down to low, and a
// low-evidence item never enters the one-click set.

func TestUnsourcedClaimsAreLowEvidenceAndOutOfOneClick(t *testing.T) {
	_, _, e := setup(t)
	for _, id := range []string{
		"adv.vbs.off", "adv.vmp.off", // CATALOGO-1 F01: the Microsoft Support page is gone
		"privacy.telemetry.tasks", // CATALOGO-1 F18: no source names the six tasks
		"services.sysmain.hdd",    // CATALOGO-1 F21: the source only describes the service
		"net.nic.power",           // CATALOGO-2 F05: the source does not cite PnPCapabilities = 24
		"fix.power.throttling",    // CATALOGO-2 F29: PowerThrottlingOff is not documented
	} {
		tw := e.Catalog.Get(id)
		if tw == nil {
			t.Fatalf("%s não está no catálogo", id)
		}
		if tw.Evidence != tweaks.EvLow {
			t.Errorf("%s: evidência %v sem fonte que diga o que o texto afirma", id, tw.Evidence)
		}
		if tw.Recommendable() {
			t.Errorf("%s: evidência baixa não pode entrar no 1 clique", id)
		}
	}
}

// CATALOGO-1 F21: a system disk that was not read, or whose type Windows did
// not report, is not "SSD".
func TestSysMainHDDNeedsTheDiskType(t *testing.T) {
	_, _, e := setup(t)
	tw := e.Catalog.Get("services.sysmain.hdd")
	cases := map[string]struct {
		disks []facts.Disk
		ok    bool
		why   string
	}{
		"disco não lido":     {nil, false, "não lido"},
		"tipo não informado": {[]facts.Disk{{IsSystem: true, MediaType: "Unspecified"}}, false, "não identificado"},
		"SSD":                {[]facts.Disk{{IsSystem: true, MediaType: "SSD"}}, false, "SSD"},
		"disco rígido (HDD)": {[]facts.Disk{{IsSystem: true, MediaType: "HDD"}}, true, ""},
	}
	for name, c := range cases {
		ok, why := tw.IsApplicable(&facts.Facts{Disks: c.disks})
		if ok != c.ok || !strings.Contains(why, c.why) {
			t.Errorf("%s: aplicável=%v (%q), esperado %v com %q", name, ok, why, c.ok, c.why)
		}
	}
	if r := tw.Recommend(&facts.Facts{Disks: []facts.Disk{{IsSystem: true, MediaType: "HDD"}}}); r.Level == tweaks.RecYes {
		t.Errorf("sem medição o SysMain em HDD não pode vir recomendado: %+v", r)
	}
}

// CATALOGO-2 F03: with the pointer acceleration not seen on, the item is not
// recommended (and says nothing it cannot keep).
func TestMouseAccelNotRecommendedWhenNotOn(t *testing.T) {
	_, _, e := setup(t)
	tw := e.Catalog.Get("input.mouse.accel")
	r := tw.Recommend(&facts.Facts{})
	if r.Level == tweaks.RecYes {
		t.Errorf("aceleração não vista ligada não pode virar recomendação: %+v", r)
	}
	if strings.Contains(r.Reason, "garante") {
		t.Errorf("texto com palavra proibida pela régua: %q", r.Reason)
	}
	on := &facts.Facts{}
	on.Gaming.MouseAccelOn = true
	if r := tw.Recommend(on); r.Level != tweaks.RecYes {
		t.Errorf("com a aceleração ligada o item continua recomendado: %+v", r)
	}
}

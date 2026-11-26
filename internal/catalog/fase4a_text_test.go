package catalog_test

import (
	"strings"
	"testing"

	"axion/internal/catalog"
	"axion/internal/collect"
	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

func tweakByID(t *testing.T, id string) *tweaks.Tweak {
	t.Helper()
	for _, tw := range catalog.All() {
		if tw.ID == id {
			return tw
		}
	}
	t.Fatalf("otimização %s não existe", id)
	return nil
}

// G-015 a / CATALOGO-1 F25: the Policy CSP pages of these policies list Pro,
// Enterprise and Education, not Home. The text says so, and on Windows Home
// the recommendation says it may have no effect (same level as on Pro).
func TestPolicyTweaksWarnAboutHome(t *testing.T) {
	home, pro := &facts.Facts{}, &facts.Facts{}
	home.OS.Edition, home.OS.Caption = "Core", "Microsoft Windows 11 Home"
	pro.OS.Edition, pro.OS.Caption = "Professional", "Microsoft Windows 11 Pro"
	for _, id := range []string{"privacy.activity", "privacy.websearch", "privacy.widgets", "update.do.p2p", "update.drivers.exclude"} {
		tw := tweakByID(t, id)
		if !strings.Contains(tw.Detail, "não a Home") {
			t.Errorf("%s: o texto deveria avisar sobre o Windows Home: %s", id, tw.Detail)
		}
		rh, rp := tw.Recommendation(home), tw.Recommendation(pro)
		if !strings.Contains(rh.Reason, "Windows Home") || strings.Contains(rp.Reason, "Windows Home") {
			t.Errorf("%s: recomendação no Home %q, no Pro %q", id, rh.Reason, rp.Reason)
		}
		if rh.Level != rp.Level {
			t.Errorf("%s: o nível não muda com a edição (%v x %v)", id, rh.Level, rp.Level)
		}
	}
}

// G-015 c / CATALOGO-2 F23: fix.services.apps puts each service back to its
// Windows default; the text cannot say "Manual" for one whose default is not.
func TestServicesAppsTextNamesNonManualDefaults(t *testing.T) {
	tw := tweakByID(t, "fix.services.apps")
	for _, s := range collect.CriticalServices {
		if s.Group == "apps" && s.Default != sys.StartManual && !strings.Contains(tw.Summary, s.Display) {
			t.Errorf("%s volta a %v, não a Manual, e o resumo não diz: %s", s.Name, s.Default, tw.Summary)
		}
	}
}

// G-015 b / CATALOGO-2 F06/F07: no desktop text on a notebook and no
// promise of microseconds.
func TestNicTweaksNotebookText(t *testing.T) {
	nb := &facts.Facts{}
	nb.Power.IsLaptop = true
	if r := tweakByID(t, "net.nic.eee").Recommendation(nb); strings.Contains(r.Reason, "desktop") || !strings.Contains(r.Reason, "bateria") {
		t.Errorf("net.nic.eee em notebook: %q", r.Reason)
	}
	for _, id := range []string{"net.nic.eee", "net.nic.intmod"} {
		tw := tweakByID(t, id)
		text := tw.Summary + " " + tw.Detail + " " + tw.Recommendation(nb).Reason
		if strings.Contains(strings.ToLower(text), "microssegundo") {
			t.Errorf("%s promete microssegundos: %s", id, text)
		}
	}
}

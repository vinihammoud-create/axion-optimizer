package catalog_test

import (
	"context"
	"strings"
	"testing"

	"axion/internal/collect"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

// managementCases are the four states of a PC for "Desbloquear o Windows
// Update": only a PC known not to be managed gets the recommendation; on a
// domain or MDM PC the policies may be the organization's, and a read that
// failed is not "unmanaged".
var managementCases = []struct {
	name   string
	set    func(m *mocksys.Mock)
	level  tweaks.RecLevel
	reason []string
}{
	{"sem gerenciamento", func(*mocksys.Mock) {}, tweaks.RecYes, []string{"não faz parte de domínio nem está inscrito em MDM"}},
	{"domínio", func(m *mocksys.Mock) { m.SetDomain("corp.exemplo.com.br") }, tweaks.RecNo, []string{"faz parte de um domínio", "fale com o TI"}},
	{"MDM", func(m *mocksys.Mock) { m.SetMDM(true) }, tweaks.RecNo, []string{"gerenciamento de dispositivos (MDM)", "fale com o TI"}},
	{"leitura falhou", func(m *mocksys.Mock) { m.FailManagementRead() }, tweaks.RecNo, []string{"não foi possível saber se o PC é gerenciado", "TI"}},
}

func TestUpdatePoliciesManagedPC(t *testing.T) {
	for _, tc := range managementCases {
		t.Run(tc.name, func(t *testing.T) {
			m, _, e := setup(t)
			tc.set(m)
			f := collect.Collect(context.Background(), m.Platform, nil)
			tw := e.Catalog.Get("fix.update.policies")
			if ok, why := tw.IsApplicable(f); !ok {
				t.Fatalf("o PC simulado tem bloqueio do Windows Update: %s", why)
			}
			rec := tw.Recommendation(f)
			if rec.Level != tc.level {
				t.Errorf("recomendação %s, esperado %s (%q)", rec.Level, tc.level, rec.Reason)
			}
			for _, s := range tc.reason {
				if !strings.Contains(rec.Reason, s) {
					t.Errorf("motivo sem %q: %q", s, rec.Reason)
				}
			}
			// The one-click plans take only RecYes items (or RecNeutral as
			// optional): a RecNo item stays out of them.
			inOneClick := tw.Recommendable() && rec.Level == tweaks.RecYes
			if inOneClick != (tc.level == tweaks.RecYes) {
				t.Errorf("1 clique = %v", inOneClick)
			}
			// The item still applies and undoes exactly when the person
			// chooses it on its own page.
			roundTrip(t, m, f, e, tw)
		})
	}
}

// The item says, in its own text, why a work or school PC is different.
func TestUpdatePoliciesTextExplainsManagement(t *testing.T) {
	_, _, e := setup(t)
	tw := e.Catalog.Get("fix.update.policies")
	for _, s := range []string{"domínio", "MDM", "fale com o TI", "não foi possível ler se ele é gerenciado", "fora do 1 clique"} {
		if !strings.Contains(tw.Detail, s) {
			t.Errorf("texto sem %q: %s", s, tw.Detail)
		}
	}
	urls := map[string]bool{}
	for _, s := range tw.Sources {
		urls[s.URL] = true
	}
	for _, u := range []string{
		"https://learn.microsoft.com/windows/win32/cimwin32prov/win32-computersystem",
		"https://learn.microsoft.com/windows/win32/api/mdmregistration/nf-mdmregistration-isdeviceregisteredwithmanagement",
	} {
		if !urls[u] {
			t.Errorf("fonte que fundamenta a leitura faltando: %s", u)
		}
	}
}

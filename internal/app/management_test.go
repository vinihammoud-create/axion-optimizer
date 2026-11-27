package app

import (
	"testing"

	"axion/internal/sys/mocksys"
)

// "Desbloquear o Windows Update" enters the one-click plans only on a PC
// known not to be managed: on a domain or MDM PC, or when that could not be
// read, it stays out of both tiers (it is still on its own page).
func TestUpdatePoliciesOneClickOnlyWhenUnmanaged(t *testing.T) {
	const id = "fix.update.policies"
	for _, tc := range []struct {
		name string
		set  func(m *mocksys.Mock)
		in   bool
	}{
		{"sem gerenciamento", func(*mocksys.Mock) {}, true},
		{"domínio", func(m *mocksys.Mock) { m.SetDomain("corp.exemplo.com.br") }, false},
		{"MDM", func(m *mocksys.Mock) { m.SetMDM(true) }, false},
		{"leitura falhou", func(m *mocksys.Mock) { m.FailManagementRead() }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, c := setupServer(t)
			tc.set(m)
			var j Job
			c.json("POST", "/api/scan", map[string]any{}, &j)
			if j = c.wait(j.ID); j.Status != JobDone {
				t.Fatalf("análise falhou: %+v", j)
			}
			for _, tier := range []string{"recomendado", "completo"} {
				var p Plan
				c.json("GET", "/api/plan?tier="+tier, nil, &p)
				found := false
				for _, it := range p.Items {
					if it.ID == id {
						found = true
					}
				}
				if found != tc.in {
					t.Errorf("plano %s: %s presente = %v, esperado %v", tier, id, found, tc.in)
				}
			}
		})
	}
}

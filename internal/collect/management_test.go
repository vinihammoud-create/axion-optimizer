package collect_test

import (
	"context"
	"testing"

	"axion/internal/collect"
	"axion/internal/facts"
	"axion/internal/sys/mocksys"
)

// Domain and MDM are read with explicit flags: a read that failed (or a
// NULL PartOfDomain, "status unknown" for Microsoft) is not "not managed".
func TestManagementRead(t *testing.T) {
	cases := []struct {
		name      string
		set       func(m *mocksys.Mock)
		want      facts.ManagementInfo
		managed   bool
		unmanaged bool
	}{
		{"sem gerenciamento", func(*mocksys.Mock) {},
			facts.ManagementInfo{DomainRead: true, MDMRead: true}, false, true},
		{"domínio", func(m *mocksys.Mock) { m.SetDomain("corp.exemplo.com.br") },
			facts.ManagementInfo{DomainRead: true, Domain: true, MDMRead: true}, true, false},
		{"MDM", func(m *mocksys.Mock) { m.SetMDM(true) },
			facts.ManagementInfo{DomainRead: true, MDMRead: true, MDM: true}, true, false},
		{"leitura falhou", func(m *mocksys.Mock) { m.FailManagementRead() },
			facts.ManagementInfo{}, false, false},
		{"PartOfDomain NULL", func(m *mocksys.Mock) { m.SetDomainUnknown() },
			facts.ManagementInfo{MDMRead: true}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := mocksys.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			tc.set(m)
			f := collect.Collect(context.Background(), m.Platform, nil)
			if f.Management != tc.want {
				t.Errorf("gerenciamento lido: %+v, esperado %+v", f.Management, tc.want)
			}
			if f.Management.Managed() != tc.managed || f.Management.KnownUnmanaged() != tc.unmanaged {
				t.Errorf("gerenciado=%v, sabidamente sem gerenciamento=%v", f.Management.Managed(), f.Management.KnownUnmanaged())
			}
		})
	}
}

package winsys

import (
	"errors"
	"strings"
	"testing"

	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

// failingDNSReg fails the reads of one registry key (and what is under it).
type failingDNSReg struct {
	sys.Registry
	under string
}

var errDNSRead = errors.New("acesso negado (simulado)")

func (r failingDNSReg) hit(path string) bool {
	return strings.HasPrefix(strings.ToLower(path), strings.ToLower(r.under))
}

func (r failingDNSReg) GetValue(path, name string) (sys.RegValue, error) {
	if r.hit(path) {
		return sys.RegValue{}, errDNSRead
	}
	return r.Registry.GetValue(path, name)
}

func (r failingDNSReg) KeyExists(path string) (bool, error) {
	if r.hit(path) {
		return false, errDNSRead
	}
	return r.Registry.KeyExists(path)
}

func (r failingDNSReg) SubKeys(path string) ([]string, error) {
	if r.hit(path) {
		return nil, errDNSRead
	}
	return r.Registry.SubKeys(path)
}

const dnsTestGUID = "{11111111-2222-3333-4444-555555555555}"

func dnsTestReg(t *testing.T) *mocksys.Mock {
	t.Helper()
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = m.R.SetValue(keyTcpipIfaces+dnsTestGUID, "NameServer", sys.SZ("1.1.1.1,1.0.0.1"))
	_ = m.R.SetValue(keyTcpip6Ifaces+dnsTestGUID, "NameServer", sys.SZ("2606:4700:4700::1111"))
	return m
}

func TestDNSRegistryViewReadsStaticLists(t *testing.T) {
	m := dnsTestReg(t)
	c := sys.DNSConfig{V4: []string{"1.1.1.1", "1.0.0.1"}}
	if err := dnsRegistryView(m.R, dnsTestGUID, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.V4) != 2 || len(c.V6) != 1 || c.V6[0] != "2606:4700:4700::1111" || c.DoH {
		t.Errorf("configuração lida: %+v", c)
	}
	// DHCP: the API hands back the router's servers, the registry has none.
	_ = m.R.SetValue(keyTcpipIfaces+dnsTestGUID, "NameServer", sys.SZ(""))
	c = sys.DNSConfig{V4: []string{"192.168.0.1"}}
	if err := dnsRegistryView(m.R, dnsTestGUID, &c); err != nil || c.V4 != nil {
		t.Errorf("DNS automático não pode virar lista estática: %+v %v", c, err)
	}
	// A DNS over HTTPS server registered for the adapter.
	if _, err := m.R.CreateKey(keyDohIfaces(dnsTestGUID) + `\Doh\1.1.1.1`); err != nil {
		t.Fatal(err)
	}
	c = sys.DNSConfig{}
	if err := dnsRegistryView(m.R, dnsTestGUID, &c); err != nil || !c.DoH {
		t.Errorf("DNS sobre HTTPS configurado não foi visto: %+v %v", c, err)
	}
}

// A read that fails is an error, never "no static IPv6" or "no DNS over
// HTTPS": the snapshot built from this is what the undo puts back.
func TestDNSRegistryViewFailsWhenAReadFails(t *testing.T) {
	m := dnsTestReg(t)
	for name, under := range map[string]string{
		"IPv4 estático":   keyTcpipIfaces + dnsTestGUID,
		"IPv6 estático":   keyTcpip6Ifaces + dnsTestGUID,
		"DNS sobre HTTPS": keyDohIfaces(dnsTestGUID),
	} {
		c := sys.DNSConfig{V4: []string{"1.1.1.1"}}
		err := dnsRegistryView(failingDNSReg{Registry: m.R, under: under}, dnsTestGUID, &c)
		if !errors.Is(err, errDNSRead) {
			t.Errorf("%s: leitura que falhou deveria ser erro, obtido %v (%+v)", name, err, c)
		}
	}
}

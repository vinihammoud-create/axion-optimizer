//go:build windows

package winsys

import (
	"flag"
	"reflect"
	"testing"
)

// realDNS turns on the read-only check against the real machine: it reads the
// DNS of every adapter through GetInterfaceDnsSettings and compares it with
// the registry. It writes nothing. Run it by hand:
//
//	go test ./internal/sys/winsys -run TestDNSConfigReadsEveryAdapter -real-dns -v
var realDNS = flag.Bool("real-dns", false, "lê o DNS de cada adaptador pela API real e confere com o registro (somente leitura)")

func TestDNSConfigReadsEveryAdapter(t *testing.T) {
	if !*realDNS {
		t.Skip("verificação com o Windows real: informe -real-dns")
	}
	n := &Native{Reg: Registry{}}
	if procGetInterfaceDnsSettings.Find() != nil {
		t.Skip("GetInterfaceDnsSettings não existe neste Windows")
	}
	list, err := n.Adapters()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range list {
		if !a.Up {
			continue
		}
		cfg, err := n.DNSConfig(a.GUID)
		if err != nil {
			t.Errorf("%s (%s): %v", a.Name, a.GUID, err)
			continue
		}
		v4, _ := n.Reg.GetValue(keyTcpipIfaces+a.GUID, "NameServer")
		v6, _ := n.Reg.GetValue(keyTcpip6Ifaces+a.GUID, "NameServer")
		t.Logf("%s dhcp=%v efetivo=%v | API v4=%v v6=%v doh=%v | registro v4=%q v6=%q", a.Name, a.DHCP, a.DNS, cfg.V4, cfg.V6, cfg.DoH, v4.Str, v6.Str)
		if want := splitServers(v4.Str); !reflect.DeepEqual(cfg.V4, want) && !(len(cfg.V4) == 0 && len(want) == 0) {
			t.Errorf("%s: a API deu %v e o registro diz %v", a.Name, cfg.V4, want)
		}
	}
}

package app

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"axion/internal/catalog"
	"axion/internal/netdiag"
	"axion/internal/sys/mocksys"
)

func dnsScan(t *testing.T, c *client) {
	t.Helper()
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	c.s.App.netTimingOverride = &netTiming{Idle: 300 * time.Millisecond, Load: 500 * time.Millisecond, Interval: 10 * time.Millisecond, LoadPing: 20 * time.Millisecond, Streams: [2]int{2, 2}}
}

func dnsNetTest(t *testing.T, c *client) *netdiag.Run {
	t.Helper()
	var j Job
	c.json("POST", "/api/net/test", map[string]any{"label": "antes", "load": false, "dns": true}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("teste de rede: %+v", j)
	}
	var v NetView
	c.json("GET", "/api/net", nil, &v)
	if v.Last == nil {
		t.Fatal("sem teste salvo")
	}
	return v.Last
}

func dnsFinding(r *netdiag.Run, id string) *netdiag.Finding {
	for i := range r.Findings {
		if r.Findings[i].ID == id {
			return &r.Findings[i]
		}
	}
	return nil
}

// The network test finds the DNS failing, offers the change, the change is
// applied through /api/apply and undone through /api/revert, exactly.
func TestDNSFixFromNetTest(t *testing.T) {
	m, c := setupServer(t)
	a := c.s.App
	dnsScan(t, c)
	m.N.Net.DNSDown = map[string]bool{"192.168.0.1": true}
	r := dnsNetTest(t, c)
	f := dnsFinding(r, "dns.fail")
	if f == nil || len(f.Actions) != 1 || f.Actions[0].Kind != "apply" {
		t.Fatalf("dns.fail com a ação: %+v", f)
	}
	// The simulated adapter has no IPv6 DNS: the item writes IPv4 only.
	id := "dns:" + mocksys.EthernetGUID + ":cloudflare:v4"
	if len(f.Actions[0].Tweaks) != 1 || f.Actions[0].Tweaks[0] != id {
		t.Fatalf("ID oferecido: %v", f.Actions[0].Tweaks)
	}
	if !strings.Contains(f.Actions[0].Confirm, "não muda o ping") || !strings.Contains(f.Actions[0].Confirm, "automático") {
		t.Errorf("confirmação: %s", f.Actions[0].Confirm)
	}
	var v NetView
	c.json("GET", "/api/net", nil, &v)
	if v.Adapter == nil || !v.Adapter.DNSAuto || v.Adapter.DoH {
		t.Errorf("adaptador: %+v", v.Adapter)
	}

	before := m.World()
	var j Job
	c.json("POST", "/api/apply", ApplyRequest{IDs: []string{id}, Label: "DNS"}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("aplicar: %+v", j)
	}
	cfg, _ := m.N.DNSConfig(mocksys.EthernetGUID)
	if len(cfg.V4) == 0 || cfg.V4[0] != "1.1.1.1" || len(cfg.V6) != 0 {
		t.Fatalf("DNS depois de aplicar: %+v", cfg)
	}
	ent := a.Engine.J.ActiveFor(id)
	if ent == nil || !strings.HasPrefix(ent.TweakTitle, "DNS da Cloudflare") {
		t.Fatalf("entrada no histórico: %+v", ent)
	}
	c.json("POST", "/api/revert", RevertRequest{IDs: []string{id}}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("desfazer: %+v", j)
	}
	if d := mocksys.DiffWorld(before, m.World()); len(d) > 0 {
		t.Errorf("o estado deveria voltar exato: %v", d)
	}
}

// With an IPv6 DNS already on the adapter, both stacks are written.
func TestDNSFixWithIPv6(t *testing.T) {
	m, c := setupServer(t)
	dnsScan(t, c)
	const v6 = "2804:14d:1:0:181:213:132:4"
	m.N.NetAdapters[0].DNS = []string{v6, "192.168.0.1"}
	m.N.Net.DNSDown = map[string]bool{v6: true, "192.168.0.1": true}
	r := dnsNetTest(t, c)
	f := dnsFinding(r, "dns.fail")
	if f == nil || len(f.Actions) != 1 || f.Actions[0].Tweaks[0] != "dns:"+mocksys.EthernetGUID+":cloudflare" {
		t.Fatalf("com IPv6 o ID não tem :v4: %+v", f)
	}
	id := f.Actions[0].Tweaks[0]
	var j Job
	c.json("POST", "/api/apply", ApplyRequest{IDs: []string{id}, Label: "DNS"}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("aplicar: %+v", j)
	}
	cfg, _ := m.N.DNSConfig(mocksys.EthernetGUID)
	if !reflect.DeepEqual(cfg.V6, []string{"2606:4700:4700::1111", "2606:4700:4700::1001"}) {
		t.Errorf("IPv6 gravado: %+v", cfg)
	}
}

// DNS over HTTPS set by the user: no action, and the apply is refused.
func TestDNSFixRefusedWithDoH(t *testing.T) {
	m, c := setupServer(t)
	dnsScan(t, c)
	m.N.DoH = map[string]bool{strings.ToLower(mocksys.EthernetGUID): true}
	m.N.Net.DNSDown = map[string]bool{"192.168.0.1": true}
	r := dnsNetTest(t, c)
	f := dnsFinding(r, "dns.fail")
	if f == nil || len(f.Actions) != 0 || !strings.Contains(f.Text, "DNS over HTTPS") {
		t.Fatalf("DoH: %+v", f)
	}
	code, _ := c.do("POST", "/api/apply", ApplyRequest{IDs: []string{"dns:" + mocksys.EthernetGUID + ":cloudflare:v4"}, Label: "DNS"}, nil)
	if code != http.StatusBadRequest {
		t.Errorf("o servidor deveria recusar um ID não oferecido: %d", code)
	}
}

// The DNS change is only applied when the latest test offered it.
func TestDNSApplyNotOffered(t *testing.T) {
	m, c := setupServer(t)
	dnsScan(t, c)
	id := "dns:" + mocksys.EthernetGUID + ":google"
	// No network test at all.
	code, body := c.do("POST", "/api/apply", ApplyRequest{IDs: []string{id}, Label: "DNS"}, nil)
	if code != http.StatusBadRequest || !strings.Contains(string(body), "só é oferecida") {
		t.Fatalf("sem teste: %d %s", code, body)
	}
	// A test where the DNS is fine (dns.ok has no action).
	r := dnsNetTest(t, c)
	if f := dnsFinding(r, "dns.ok"); f == nil || len(f.Actions) != 0 {
		t.Fatalf("dns.ok: %+v", f)
	}
	code, _ = c.do("POST", "/api/apply", ApplyRequest{IDs: []string{"dns:" + mocksys.EthernetGUID + ":cloudflare:v4"}, Label: "DNS"}, nil)
	if code != http.StatusBadRequest {
		t.Errorf("com dns.ok: %d", code)
	}
	if cfg, _ := m.N.DNSConfig(mocksys.EthernetGUID); len(cfg.V4) != 0 || m.N.DNSWrites != 0 {
		t.Errorf("nada deveria ter sido gravado: %+v", cfg)
	}
	// Other execution IDs are not gated by the DNS rule.
	if !catalog.IsDNSID(id) || catalog.IsDNSID("activehours:20-2") {
		t.Error("IsDNSID")
	}
}

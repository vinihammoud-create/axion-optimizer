package catalog_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"axion/internal/catalog"
	"axion/internal/facts"
	"axion/internal/netdiag"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

// DNS of an adapter (0.9): applied and undone by the engine, exact.

func setupDNS(t *testing.T) (*mocksys.Mock, *facts.Facts, *tweaks.Engine) {
	t.Helper()
	m, f, e := setup(t)
	e.Providers = append(e.Providers, catalog.DNSProvider)
	return m, f, e
}

func flushes(m *mocksys.Mock) int {
	n := 0
	for _, c := range m.X.Recorded() {
		if c.Exe == "ipconfig.exe" && len(c.Args) == 1 && c.Args[0] == "/flushdns" {
			n++
		}
	}
	return n
}

func TestDNSProviderRoundTrip(t *testing.T) {
	m, f, e := setupDNS(t)
	id := netdiag.DNSTweakID(mocksys.EthernetGUID, "cloudflare", false)
	tw := e.Resolve(id, f)
	if tw == nil {
		t.Fatal("o provedor não montou o item de DNS")
	}
	if tw.OneClick || tw.Risk != tweaks.RiskModerate || tw.Evidence != tweaks.EvMedium || tw.MinBuild != 19041 || tw.Group != "dns:"+strings.ToLower(mocksys.EthernetGUID) {
		t.Errorf("metadados: %+v", tw)
	}
	if !strings.Contains(tw.Detail, "ping durante a partida não muda") || !strings.Contains(tw.Title, "da Cloudflare") {
		t.Errorf("texto: %s | %s", tw.Title, tw.Detail)
	}
	roundTrip(t, m, f, e, tw)
	if flushes(m) != 2 {
		t.Errorf("ipconfig /flushdns deveria rodar ao aplicar e ao desfazer: %d", flushes(m))
	}
}

func TestDNSAppliesBothStacks(t *testing.T) {
	m, f, e := setupDNS(t)
	ctx := context.Background()
	if r := e.Apply(ctx, f, []string{netdiag.DNSTweakID(mocksys.EthernetGUID, "cloudflare", false)}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	cfg, err := m.N.DNSConfig(mocksys.EthernetGUID)
	if err != nil || !reflect.DeepEqual(cfg.V4, []string{"1.1.1.1", "1.0.0.1"}) || !reflect.DeepEqual(cfg.V6, []string{"2606:4700:4700::1111", "2606:4700:4700::1001"}) {
		t.Fatalf("DNS gravado: %+v %v", cfg, err)
	}
	if flushes(m) != 1 {
		t.Errorf("flushdns ao aplicar: %d", flushes(m))
	}
	// IPv4 only: the IPv6 list stays as it was (empty).
	m2, f2, e2 := setupDNS(t)
	id := netdiag.DNSTweakID(mocksys.EthernetGUID, "quad9", true)
	tw := e2.Resolve(id, f2)
	if !strings.Contains(tw.Detail, "Quad9 bloqueia domínios") || !strings.Contains(tw.Detail, "IPv6 não é gravado") {
		t.Errorf("o texto do Quad9 em IPv4: %s", tw.Detail)
	}
	if r := e2.Apply(ctx, f2, []string{id}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar v4: %+v", r[0])
	}
	cfg, _ = m2.N.DNSConfig(mocksys.EthernetGUID)
	if !reflect.DeepEqual(cfg.V4, []string{"9.9.9.9", "149.112.112.112"}) || len(cfg.V6) != 0 {
		t.Fatalf("DNS só IPv4: %+v", cfg)
	}
	// And the same item on a clean machine round-trips exactly.
	m3, f3, e3 := setupDNS(t)
	roundTrip(t, m3, f3, e3, e3.Resolve(id, f3))
}

// A static list from before is restored exactly (order included).
func TestDNSRestoresStaticList(t *testing.T) {
	m, f, e := setupDNS(t)
	ctx := context.Background()
	if err := m.N.SetDNSConfig(mocksys.EthernetGUID, []string{"192.168.0.53", "192.168.0.54"}, []string{"fd00::53"}); err != nil {
		t.Fatal(err)
	}
	before := m.World()
	id := netdiag.DNSTweakID(mocksys.EthernetGUID, "google", false)
	if r := e.Apply(ctx, f, []string{id}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	if r := e.RevertTweaks(ctx, f, []string{id}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer: %+v", r[0])
	}
	cfg, _ := m.N.DNSConfig(mocksys.EthernetGUID)
	if !reflect.DeepEqual(cfg.V4, []string{"192.168.0.53", "192.168.0.54"}) || !reflect.DeepEqual(cfg.V6, []string{"fd00::53"}) {
		t.Errorf("a lista anterior deveria voltar: %+v", cfg)
	}
	if d := mocksys.DiffWorld(before, m.World()); len(d) > 0 {
		t.Errorf("o estado não voltou: %v", d)
	}
}

// Choosing another resolver reverts the first one (same group) and a full
// undo brings back the state from before the first.
func TestDNSGroupSwitch(t *testing.T) {
	m, f, e := setupDNS(t)
	ctx := context.Background()
	before := m.World()
	for _, key := range []string{"cloudflare", "quad9"} {
		if r := e.Apply(ctx, f, []string{netdiag.DNSTweakID(mocksys.EthernetGUID, key, false)}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
			t.Fatalf("%s: %+v", key, r[0])
		}
	}
	if n := len(e.J.Active()); n != 1 {
		t.Fatalf("só a última escolha deveria estar ativa: %d", n)
	}
	cfg, _ := m.N.DNSConfig(mocksys.EthernetGUID)
	if len(cfg.V4) == 0 || cfg.V4[0] != "9.9.9.9" {
		t.Fatalf("DNS: %+v", cfg)
	}
	e.RevertAll(ctx, f, tweaks.RevertOpts{})
	if d := mocksys.DiffWorld(before, m.World()); len(d) > 0 {
		t.Errorf("o estado original não voltou: %v", d)
	}
}

func TestDNSRefusesDoH(t *testing.T) {
	m, f, e := setupDNS(t)
	m.N.DoH = map[string]bool{strings.ToLower(mocksys.EthernetGUID): true}
	before := m.World()
	r := e.Apply(context.Background(), f, []string{netdiag.DNSTweakID(mocksys.EthernetGUID, "cloudflare", false)}, tweaks.ApplyOpts{})
	if r[0].Outcome != tweaks.OutFailed || !strings.Contains(r[0].Message, "DNS over HTTPS") {
		t.Fatalf("DoH deveria ser recusado: %+v", r[0])
	}
	if d := mocksys.DiffWorld(before, m.World()); len(d) > 0 {
		t.Errorf("nada deveria mudar: %v", d)
	}
	// The refusal and its rollback never touch the adapter.
	if m.N.DNSWrites != 0 {
		t.Errorf("SetDNSConfig gravou %d vez(es) apesar da recusa", m.N.DNSWrites)
	}
}

func TestDNSRefusesPolicy(t *testing.T) {
	m, f, e := setupDNS(t)
	_ = m.R.SetValue(catalog.KeyDNSPolicy, "NameServer", sys.SZ("10.0.0.1"))
	before := m.World()
	r := e.Apply(context.Background(), f, []string{netdiag.DNSTweakID(mocksys.EthernetGUID, "google", false)}, tweaks.ApplyOpts{})
	if r[0].Outcome != tweaks.OutFailed || !strings.Contains(r[0].Message, "política de grupo") {
		t.Fatalf("a política deveria recusar: %+v", r[0])
	}
	if d := mocksys.DiffWorld(before, m.World()); len(d) > 0 {
		t.Errorf("nada deveria mudar: %v", d)
	}
	if m.N.DNSWrites != 0 {
		t.Errorf("SetDNSConfig gravou %d vez(es)", m.N.DNSWrites)
	}
}

// An adapter that left the system (its static configuration too) cannot be
// restored: the undo says nothing is left, without failing.
func TestDNSAdapterRemoved(t *testing.T) {
	m, f, e := setupDNS(t)
	ctx := context.Background()
	id := netdiag.DNSTweakID(mocksys.EthernetGUID, "cloudflare", false)
	if r := e.Apply(ctx, f, []string{id}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	m.N.NetAdapters = m.N.NetAdapters[1:]
	for _, root := range []string{`HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\`, `HKLM\SYSTEM\CurrentControlSet\Services\Tcpip6\Parameters\Interfaces\`} {
		_ = m.R.DeleteValue(root+mocksys.EthernetGUID, "NameServer")
		_, _ = m.R.DeleteKeyIfEmpty(root + mocksys.EthernetGUID)
	}
	if r := e.RevertTweaks(ctx, f, []string{id}, tweaks.RevertOpts{Force: true}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer com o adaptador removido: %+v", r[0])
	}
}

// An adapter turned off in Network Connections leaves the adapter list but
// keeps its static configuration: the undo must still restore it.
func TestDNSAdapterDisabledStillRestored(t *testing.T) {
	m, f, e := setupDNS(t)
	ctx := context.Background()
	id := netdiag.DNSTweakID(mocksys.EthernetGUID, "cloudflare", false)
	if r := e.Apply(ctx, f, []string{id}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	m.N.NetAdapters = m.N.NetAdapters[1:]
	if r := e.RevertTweaks(ctx, f, []string{id}, tweaks.RevertOpts{Force: true}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer: %+v", r[0])
	}
	cfg, err := m.N.DNSConfig(mocksys.EthernetGUID)
	if err != nil || len(cfg.V4) != 0 || len(cfg.V6) != 0 {
		t.Errorf("o DNS anterior (automático) deveria voltar: %+v %v", cfg, err)
	}
}

func TestDNSUnknownAdapter(t *testing.T) {
	_, f, e := setupDNS(t)
	r := e.Apply(context.Background(), f, []string{netdiag.DNSTweakID("{00000000-0000-0000-0000-000000000000}", "google", false)}, tweaks.ApplyOpts{})
	if r[0].Outcome != tweaks.OutFailed || !strings.Contains(r[0].Message, "não foi encontrado") {
		t.Fatalf("adaptador inexistente: %+v", r[0])
	}
}

func TestDNSOpsAreRebuildable(t *testing.T) {
	_, f, _ := setupDNS(t)
	for _, v4 := range []bool{false, true} {
		for _, k := range []string{"cloudflare", "google", "quad9"} {
			id := netdiag.DNSTweakID(mocksys.EthernetGUID, k, v4)
			tw := catalog.DNSProvider(id, f)
			if tw == nil {
				t.Fatalf("o provedor não montou %s", id)
			}
			for _, op := range tw.Ops(f) {
				fo, ok := op.(*tweaks.FuncOp)
				if !ok || len(fo.SpecJSON()) == 0 {
					t.Fatalf("%s: operação sem Spec", id)
				}
				rb := tweaks.Reconstruct(&tweaks.OpRecord{Kind: op.Kind(), Target: op.Target(), Spec: fo.SpecJSON()})
				if rb == nil || rb.Target() != op.Target() || op.Target() != "dns:"+strings.ToLower(mocksys.EthernetGUID) {
					t.Errorf("%s: não pode ser reconstruída (%s)", id, op.Target())
				}
			}
		}
	}
	if catalog.IsDNSID("dns:lixo") || !catalog.IsDNSID(netdiag.DNSTweakID(mocksys.EthernetGUID, "google", false)) {
		t.Error("IsDNSID")
	}
}

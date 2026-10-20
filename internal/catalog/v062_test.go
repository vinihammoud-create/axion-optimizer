package catalog_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"axion/internal/catalog"
	"axion/internal/collect"
	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

const ethKey = mocksys.NetClass + `\0001`

// "Placas de rede no padrão do driver": only the restorable properties set
// to something else are written back to the driver's default, the adapter
// is restarted, and the undo returns every value.
func TestNICDefaultsRoundTrip(t *testing.T) {
	m, _, e := setup(t)
	_ = m.R.SetValue(ethKey+`\Ndi\Params\*FlowControl\enum`, "3", sys.SZ("Rx & Tx Enabled"))
	_ = m.R.SetValue(ethKey, "*FlowControl", sys.SZ("0"))         // restorable
	_ = m.R.SetValue(ethKey, "*EEE", sys.SZ("0"))                 // net.nic.eee owns it
	_ = m.R.SetValue(ethKey, "*InterruptModeration", sys.SZ("0")) // net.nic.intmod owns it
	f := collect.Collect(context.Background(), m.Platform, nil)
	tw := e.Catalog.Get("fix.nic.defaults")
	if ok, why := tw.IsApplicable(f); !ok {
		t.Fatalf("deveria se aplicar: %s", why)
	}
	ops := tw.Ops(f)
	if len(ops) != 1 || !strings.HasSuffix(ops[0].Target(), "|*flowcontrol") {
		t.Fatalf("operações: %d %v", len(ops), ops)
	}
	if tw.Recommendation(f).Level != tweaks.RecNeutral {
		t.Errorf("restaurar o padrão é opcional: %+v", tw.Recommendation(f))
	}
	var calls []string
	m.W.CallHandler = func(ns, path, method string, args []any) (any, error) {
		calls = append(calls, method)
		return int64(0), nil
	}
	if r := e.Apply(context.Background(), f, []string{"fix.nic.defaults"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	if v, _ := m.R.GetValue(ethKey, "*FlowControl"); v.Str != "3" {
		t.Errorf("*FlowControl = %q, esperado o padrão 3", v.Str)
	}
	if v, _ := m.R.GetValue(ethKey, "*EEE"); v.Str != "0" {
		t.Errorf("*EEE não é deste item: %q", v.Str)
	}
	// (The simulated adapter never reports itself disabled, so only the
	// Disable half of the restart shows here; the full cycle is covered by
	// TestRestartAdaptersReportsDisabledAdapter.)
	if strings.Join(calls, ",") != "Disable" {
		t.Errorf("reinício da placa: %v", calls)
	}
	f2 := collect.Collect(context.Background(), m.Platform, nil)
	if st := e.StateOf(context.Background(), f2, tw); st.Status != "on" || !st.ByAxion {
		t.Fatalf("depois de aplicar: %+v", st)
	}
	if r := e.RevertTweaks(context.Background(), f2, []string{"fix.nic.defaults"}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer: %+v", r[0])
	}
	if v, _ := m.R.GetValue(ethKey, "*FlowControl"); v.Str != "0" {
		t.Errorf("desfazer deveria voltar *FlowControl = 0: %q", v.Str)
	}
	if strings.Join(calls, ",") != "Disable,Disable" {
		t.Errorf("o desfazer também reinicia a placa alterada: %v", calls)
	}
}

// The first notebook report: an unplugged Realtek with the speed fixed at
// 1 Gb/s and EEE settings Axion did not see because only connected adapters
// were read.
func TestUnpluggedAdapterSettingsAreSeen(t *testing.T) {
	_, _, e := setup(t)
	f := &facts.Facts{Adapters: []facts.Adapter{
		{Name: "Ethernet", Kind: "Ethernet", Up: false, ClassKey: ethKey, Keywords: map[string]facts.NicKeyword{
			"*SpeedDuplex":         {Default: "0", Current: "6", Set: true, Values: []string{"0", "1", "2", "3", "4", "6"}},
			"*EEE":                 {Default: "1", Current: "0", Set: true, Values: []string{"0", "1"}},
			"*InterruptModeration": {Default: "1", Current: "1", Values: []string{"0", "1"}},
			"*FlowControl":         {Default: "3", Current: "0", Set: true, Values: []string{"0", "3"}},
		}},
		{Name: "Wi-Fi", Kind: "Wi-Fi", Up: true, HasGateway: true, ClassKey: mocksys.NetClass + `\0002`},
	}}
	for id, want := range map[string]bool{"fix.nic.speed": true, "net.nic.eee": true, "net.nic.intmod": true, "fix.nic.defaults": true} {
		if ok, why := e.Catalog.Get(id).IsApplicable(f); ok != want {
			t.Errorf("%s: aplicável=%v (%s)", id, ok, why)
		}
	}
	if r := e.Catalog.Get("fix.nic.speed").Recommendation(f); r.Level != tweaks.RecNeutral {
		t.Errorf("1 Gb/s fixo é opcional: %+v", r)
	}
	f.Adapters[0].Keywords["*SpeedDuplex"] = facts.NicKeyword{Default: "0", Current: "4", Set: true}
	if r := e.Catalog.Get("fix.nic.speed").Recommendation(f); r.Level != tweaks.RecYes {
		t.Errorf("100 Mb/s fixo é recomendado corrigir: %+v", r)
	}
	// No two items own the same property.
	owner := map[string]string{}
	for _, id := range []string{"fix.nic.speed", "net.nic.eee", "net.nic.intmod", "fix.nic.defaults", "net.nic.power"} {
		for _, op := range e.Catalog.Get(id).Ops(f) {
			if prev, ok := owner[op.Target()]; ok {
				t.Errorf("%s e %s alteram %s", prev, id, op.Target())
			}
			owner[op.Target()] = id
		}
	}
}

// Without the list of changed targets, the restart keeps the old behaviour
// (connected adapters); with it, only the adapters whose key changed, and an
// adapter the user disabled is left alone.
func TestRestartOnlyTouchedAdapters(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	wifiKey := mocksys.NetClass + `\0002`
	f := &facts.Facts{Adapters: []facts.Adapter{
		{Name: "Ethernet", Kind: "Ethernet", Up: false, ClassKey: ethKey, PnPDeviceID: `PCI\VEN_10EC&DEV_8168\A`},
		{Name: "Wi-Fi", Kind: "Wi-Fi", Up: true, ClassKey: wifiKey, PnPDeviceID: `PCI\VEN_8086&DEV_7A70\B`},
	}}
	enabled := map[string]bool{"7": true, "9": true}
	rows := func() {
		m.W.Set(`root\cimv2`, "Win32_NetworkAdapter",
			sys.WMIRow{"PNPDeviceID": `PCI\VEN_10EC&DEV_8168\A`, "DeviceID": "7", "NetConnectionID": "Ethernet", "NetEnabled": enabled["7"]},
			sys.WMIRow{"PNPDeviceID": `PCI\VEN_8086&DEV_7A70\B`, "DeviceID": "9", "NetConnectionID": "Wi-Fi", "NetEnabled": enabled["9"]})
	}
	rows()
	var paths []string
	m.W.CallHandler = func(ns, path, method string, args []any) (any, error) {
		paths = append(paths, path+"."+method)
		id := "7"
		if strings.Contains(path, `"9"`) {
			id = "9"
		}
		enabled[id] = method == "Enable"
		rows()
		return int64(0), nil
	}
	post := e2ePost(t)
	c := &tweaks.Ctx{Context: context.Background(), P: m.Platform, F: f, Touched: []string{tweaks.Reg(ethKey, "*FlowControl", sys.SZ("3")).Target()}}
	if err := post(c, false); err != nil {
		t.Fatalf("reinício: %v", err)
	}
	if len(paths) != 2 || !strings.Contains(paths[0], `"7"`) {
		t.Fatalf("só a placa com cabo foi alterada: %v", paths)
	}
	// The user disabled the Ethernet adapter: not restarted (enabling it
	// would undo the user's choice).
	paths = nil
	enabled["7"] = false
	rows()
	if err := post(c, false); err != nil || len(paths) != 0 {
		t.Fatalf("placa desativada pelo usuário não deve ser reiniciada: %v %v", paths, err)
	}
	// Old callers (no list): the connected adapters.
	paths = nil
	c.Touched = nil
	if err := post(c, false); err != nil || len(paths) != 2 || !strings.Contains(paths[0], `"9"`) {
		t.Fatalf("sem lista: só as conectadas: %v %v", paths, err)
	}
}

// e2ePost is the Post of the network defaults item.
func e2ePost(t *testing.T) func(*tweaks.Ctx, bool) error {
	t.Helper()
	for _, tw := range catalog.All() {
		if tw.ID == "fix.nic.defaults" {
			return tw.Post
		}
	}
	t.Fatal("fix.nic.defaults não existe")
	return nil
}

// AllowTelemetry 0 on Home/Pro is treated as 1 by Windows: it is already the
// minimum and must not be "raised" to 1 (the first notebook report showed
// 0 → 1 in the history).
func TestTelemetryZeroIsMinimum(t *testing.T) {
	m, f, e := setup(t)
	_ = m.R.SetValue(`HKLM\SOFTWARE\Policies\Microsoft\Windows\DataCollection`, "AllowTelemetry", sys.DWord(0))
	_ = m.R.SetValue(`HKLM\SOFTWARE\Policies\Microsoft\Windows\DataCollection`, "DoNotShowFeedbackNotifications", sys.DWord(1))
	if r := e.Apply(context.Background(), f, []string{"privacy.telemetry"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutAlready {
		t.Fatalf("0 já é o mínimo: %+v", r[0])
	}
	if v, _ := m.R.GetValue(`HKLM\SOFTWARE\Policies\Microsoft\Windows\DataCollection`, "AllowTelemetry"); v.Int != 0 {
		t.Errorf("AllowTelemetry mudou para %d", v.Int)
	}
}

// The VBS switch says why Windows keeps VBS running.
func TestVBSOffEffect(t *testing.T) {
	_, _, e := setup(t)
	tw := e.Catalog.Get("adv.vbs.off")
	f := &facts.Facts{}
	f.Security.VBSRunning, f.Security.VBSPolicy = true, "on"
	ok, why := tw.Effect(f)
	if ok || !strings.Contains(why, "política de grupo") {
		t.Errorf("com política: %v %q", ok, why)
	}
	f.Security = facts.SecurityInfo{}
	if ok, _ := tw.Effect(f); !ok {
		t.Errorf("VBS parado: em efeito")
	}
	f.Unread = []string{facts.StepSecurity}
	f.Security.VBSRunning = true
	if ok, _ := tw.Effect(f); !ok {
		t.Errorf("segurança não lida: não dá para afirmar que não vale")
	}
}

// An "Ultimate Performance" copy made by another tool is used instead of
// adding another copy, and the undo does not delete it (it is not Axion's).
func TestUltimateReusesExistingCopy(t *testing.T) {
	m, _, e := setup(t)
	ext, err := m.P.DuplicateScheme(sys.SchemeUltimateTemplate)
	if err != nil {
		t.Fatal(err)
	}
	f := collect.Collect(context.Background(), m.Platform, nil)
	before, _ := m.P.Schemes()
	if r := e.Apply(context.Background(), f, []string{"power.plan.ultimate"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	after, _ := m.P.Schemes()
	if act, _ := m.P.ActiveScheme(); !strings.EqualFold(act, ext) || len(after) != len(before) {
		t.Fatalf("deveria ativar a cópia existente %s sem criar outra: ativo %s, planos %d → %d", ext, act, len(before), len(after))
	}
	f = collect.Collect(context.Background(), m.Platform, nil)
	if r := e.RevertTweaks(context.Background(), f, []string{"power.plan.ultimate"}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer: %+v", r[0])
	}
	final, _ := m.P.Schemes()
	found := false
	for _, s := range final {
		if strings.EqualFold(s.GUID, ext) {
			found = true
		}
	}
	if act, _ := m.P.ActiveScheme(); !found || strings.EqualFold(act, ext) {
		t.Fatalf("a cópia de outro programa deveria continuar existindo e sair de ativa: %v / %s", final, act)
	}
	_ = time.Second
}

func TestSlowSpeedReasonOnUnpluggedPort(t *testing.T) {
	_, _, e := setup(t)
	f := &facts.Facts{Adapters: []facts.Adapter{{Name: "Ethernet", Kind: "Ethernet", Up: false, ClassKey: ethKey,
		Keywords: map[string]facts.NicKeyword{"*SpeedDuplex": {Default: "0", Current: "4", Set: true}}}}}
	r := e.Catalog.Get("fix.nic.speed").Recommendation(f)
	if r.Level != tweaks.RecYes || !strings.Contains(r.Reason, "desconectada") {
		t.Errorf("placa desconectada fixada em 100 Mb/s: %+v", r)
	}
}

package facts

import (
	"strings"
	"testing"
)

// "Restore driver defaults" leaves out what Axion handles elsewhere, the
// network identity and what only matters with the PC asleep.
func TestNicRestorable(t *testing.T) {
	for kw, want := range map[string]bool{
		"*FlowControl": true, "*LsoV2IPv4": true, "RoamAggressiveness": true, "MIMOPowerSaveMode": true, "*PacketCoalescing": true,
		"*EEE": false, "EnableGreenEthernet": false, "GigaLite": false, "PowerSavingMode": false, "*InterruptModeration": false, "*SpeedDuplex": false,
		"NetworkAddress": false, "VlanID": false, "*JumboPacket": false,
		"*WakeOnMagicPacket": false, "*WakeOnPattern": false, "WakeOnLink": false, "S5WakeOnLan": false, "WolShutdownLinkSpeed": false,
		"*PMARPOffload": false, "*PMNSOffload": false, "*PMWiFiRekeyOffload": false, "*DeviceSleepOnDisconnect": false, "*SelectiveSuspend": false, "": false,
	} {
		if got := NicRestorable(kw); got != want {
			t.Errorf("NicRestorable(%q) = %v, esperado %v", kw, got, want)
		}
	}
}

// The real Intel AX201 of the first notebook report: only the restorable
// properties set to something other than the default are listed, with the
// driver's own labels and a Portuguese name when known.
func TestNonDefaults(t *testing.T) {
	a := Adapter{Description: "Intel(R) Wi-Fi 6 AX201 160MHz", Keywords: map[string]NicKeyword{
		"RoamAggressiveness":       {Default: "2", Current: "0", Set: true, Values: []string{"0", "1", "2", "3", "4"}, Labels: map[string]string{"0": "1. Lowest", "2": "3. Medium"}},
		"ThroughputBoosterEnabled": {Default: "0", Current: "1", Set: true, Name: "Throughput Booster"},
		"EnableChLoad4ApSelection": {Default: "0", Current: "1", Set: true, Name: "Channel Load"},
		"*PMARPOffload":            {Default: "1", Current: "0", Set: true},
		"*PacketCoalescing":        {Default: "1", Current: "1", Set: true}, // set, but the default
		"MIMOPowerSaveMode":        {Default: "0", Current: "0"},            // absent: the default
		"UnknownThing":             {Default: "5", Current: "7", Set: true, Name: "@netwtw.inf,%x%"},
	}}
	got := a.NonDefaults()
	var kws []string
	for _, c := range got {
		kws = append(kws, c.Keyword)
	}
	if strings.Join(kws, ",") != "EnableChLoad4ApSelection,RoamAggressiveness,ThroughputBoosterEnabled,UnknownThing" {
		t.Fatalf("opções listadas: %v", kws)
	}
	r := got[1]
	if r.Name != "Agressividade de roaming" || r.Current != "1. Lowest" || r.Default != "3. Medium" || r.Note == "" {
		t.Errorf("roaming: %+v", r)
	}
	if got[0].Name != "Carga do canal na escolha do ponto de acesso" || got[3].Name == "" {
		t.Errorf("nomes: %+v / %+v", got[0], got[3])
	}
	if (NicKeyword{Labels: map[string]string{"1": " "}}).Label("1") != "1" {
		t.Errorf("rótulo vazio deveria mostrar o valor")
	}
}

func TestCPUSignatureAndRaptorDie(t *testing.T) {
	c := CPUInfo{Name: "13th Gen Intel(R) Core(TM) i7-13700K", Vendor: "Intel", Family: 6, Model: 0xB7, Stepping: 1, Microcode: 0x129}
	if c.Signature() != "06-B7-01" || !c.RaptorDie() {
		t.Errorf("i7-13700K 06-B7-01: %s %v", c.Signature(), c.RaptorDie())
	}
	// Same name on the other die (C0/H0, 06-BF): not affected, whatever the
	// microcode numbering says.
	c.Model, c.Microcode = 0xBF, 0x3A
	if c.RaptorDie() {
		t.Errorf("06-BF não é o die afetado")
	}
	// Signature unknown: the microcode numbering decides, as before.
	c.Family, c.Model, c.Microcode = 0, 0, 0x12B
	if c.Signature() != "" || !c.RaptorDie() {
		t.Errorf("sem assinatura, microcode 0x1xx: %v", c.RaptorDie())
	}
	// Notebook HX parts are not desktop parts (Intel: mobile not affected).
	hx := CPUInfo{Name: "13th Gen Intel(R) Core(TM) i5-13450HX", Vendor: "Intel", Family: 6, Model: 0xB7, Microcode: 0x129}
	if hx.RaptorDie() {
		t.Errorf("HX não deveria entrar no alerta de Vmin")
	}
	// Microcode unread: never assumed.
	if (CPUInfo{Name: "Intel(R) Core(TM) i9-14900K", Vendor: "Intel", Family: 6, Model: 0xB7}).RaptorDie() {
		t.Errorf("microcode desconhecido não pode virar alerta")
	}
}

func TestLinkAtFullSpeed(t *testing.T) {
	if (NvidiaInfo{PCIeGenCur: 1, PCIeGenMax: 4}).LinkAtFullSpeed() || (NvidiaInfo{PCIeGenMax: 4}).LinkAtFullSpeed() {
		t.Error("link em economia ou não lido não é velocidade máxima")
	}
	if !(NvidiaInfo{PCIeGenCur: 4, PCIeGenMax: 4}).LinkAtFullSpeed() {
		t.Error("geração 4 de 4 é velocidade máxima")
	}
}

func TestVBSHolders(t *testing.T) {
	s := SecurityInfo{VBSRunning: true, VBSPolicy: "on", VBSLocked: true, ServicesRunning: []int{1, 99}}
	h := strings.Join(s.VBSHolders(), " | ")
	for _, want := range []string{"política de grupo", "bloqueio UEFI", "Credential Guard"} {
		if !strings.Contains(h, want) {
			t.Errorf("faltou %q em %q", want, h)
		}
	}
	if got := (SecurityInfo{VBSRunning: true}).VBSHolders(); len(got) != 1 || !strings.Contains(got[0], "Windows Hello") {
		t.Errorf("sem causa conhecida: %v", got)
	}
}

func TestPhysicalAdaptersAndLaptop(t *testing.T) {
	f := &Facts{Adapters: []Adapter{
		{Name: "Ethernet", Kind: "Ethernet", ClassKey: `HKLM\x\0001`},
		{Name: "Wi-Fi", Kind: "Wi-Fi", ClassKey: `HKLM\x\0000`, Up: true},
		{Name: "Bluetooth", Kind: "Other", ClassKey: `HKLM\x\0003`},
		{Name: "sem chave", Kind: "Ethernet"},
	}}
	if got := f.PhysicalAdapters(); len(got) != 2 || got[0].Name != "Ethernet" || got[1].Name != "Wi-Fi" {
		t.Errorf("adaptadores físicos: %+v", got)
	}
	if f.IsLaptop() {
		t.Error("sem bateria nem chassi portátil não é notebook")
	}
	f.Board.ChassisTypes = []int64{10}
	if !f.IsLaptop() {
		t.Error("chassi 10 (notebook) é notebook")
	}
}

// Review of 0.6.2: what only matters with the PC asleep or off stays out, and
// a default the driver does not accept itself is never written or listed.
func TestNicRestorableReview(t *testing.T) {
	for _, kw := range []string{"EnablePME", "*PMEEnable", "ReduceSpeedOnPowerDown"} {
		if NicRestorable(kw) {
			t.Errorf("%s deveria ficar de fora", kw)
		}
	}
	a := Adapter{Keywords: map[string]NicKeyword{
		"*FlowControl": {Default: "3", Current: "0", Set: true, Values: []string{"0", "1"}}, // default outside the list
		"*RSS":         {Default: "1", Current: "0", Set: true, Values: []string{"0", "1"}},
	}}
	if got := a.NonDefaults(); len(got) != 1 || got[0].Keyword != "*RSS" {
		t.Errorf("padrão fora da lista do driver não entra: %+v", got)
	}
}

// 0.6.3: the option is shown by the name the driver prints in Device
// Manager, with Axion's translation after it, so the user finds it on screen.
func TestNicChangeLabel(t *testing.T) {
	for _, c := range []struct {
		ch   NicChange
		want string
	}{
		{NicChange{Keyword: "*PacketCoalescing", Name: "Agrupamento de pacotes", DriverName: "União de pacote"}, "União de pacote / Agrupamento de pacotes"},
		{NicChange{Keyword: "*FlowControl", Name: "Controle de fluxo (802.3x)", DriverName: "Controle de fluxo (802.3x)"}, "Controle de fluxo (802.3x)"},
		{NicChange{Keyword: "UnknownThing", Name: "UnknownThing", DriverName: "Throughput Booster"}, "Throughput Booster"},
		{NicChange{Keyword: "X", Name: "Opção X", DriverName: ""}, "Opção X"},
		{NicChange{Keyword: "*RSS", Name: "RSS", DriverName: "*RSS"}, "RSS"},
	} {
		if got := c.ch.Label(); got != c.want {
			t.Errorf("%+v: %q, esperado %q", c.ch, got, c.want)
		}
	}
	a := Adapter{Keywords: map[string]NicKeyword{"*PacketCoalescing": {Default: "1", Current: "0", Set: true, Name: " União de pacote ", Values: []string{"0", "1"}}}}
	if got := a.NonDefaults(); len(got) != 1 || got[0].DriverName != "União de pacote" {
		t.Errorf("nome do driver guardado: %+v", got)
	}
}

// 0.6.3: VBS holders name the enabled Device Guard scenarios and never
// guess about a restart (the caller knows when the change was made).
func TestVBSHoldersScenarios(t *testing.T) {
	s := SecurityInfo{VBSRunning: true, Scenarios: []string{"SecureBiometrics", "Foo"}}
	h := strings.Join(s.VBSHolders(), " | ")
	if !strings.Contains(h, "Windows Hello (biometria)") || !strings.Contains(h, "Foo") || strings.Contains(h, "reiniciar") {
		t.Errorf("cenários: %q", h)
	}
	if h := strings.Join((SecurityInfo{VBSRunning: true}).VBSHolders(), " | "); !strings.Contains(h, "Opções de entrada") || strings.Contains(h, "reiniciar") {
		t.Errorf("sem causa: %q", h)
	}
	f := &Facts{PCIDevices: []PCIDevice{{Name: " NVIDIA High Definition Audio ", PNPID: `PCI\VEN_10DE&DEV_2291&SUBSYS_0BF81028&REV_A1\4&2A788C0F&0&0108`}}}
	if f.PCIDeviceName("ven_10de&dev_2291") != "NVIDIA High Definition Audio" || f.PCIDeviceName("VEN_10DE&DEV_0000") != "" || (*Facts)(nil).PCIDeviceName("x") != "" {
		t.Errorf("nome PCI pelo ID")
	}
}

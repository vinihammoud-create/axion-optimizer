package diag_test

import (
	"strings"
	"testing"
	"time"

	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// PCIe width: in a notebook the lanes are the manufacturer's design (the
// first notebook report told a Dell G15 to "use the upper slot"); on a
// desktop only a reading with the link at full speed is conclusive.
func TestPCIeWidthFinding(t *testing.T) {
	f := base()
	f.Nvidia = facts.NvidiaInfo{SMI: true, PCIeWidth: 8, PCIeWidthMax: 16, PCIeGenCur: 1, PCIeGenMax: 4, PState: "P8", UtilPct: 0}
	x := find(diag.Build(f, nil), "gpu.pcie.width")
	if x == nil || x.Severity != diag.SevInfo || !strings.Contains(x.Title, "repouso") || !strings.Contains(x.Evidence, "geração 1 de 4") {
		t.Fatalf("desktop em repouso: %+v", x)
	}
	f.Nvidia.PCIeGenCur, f.Nvidia.PState, f.Nvidia.UtilPct = 4, "P0", 97
	if x := find(diag.Build(f, nil), "gpu.pcie.width"); x == nil || x.Severity != diag.SevWarning || !strings.Contains(x.Explanation, "slot") {
		t.Fatalf("desktop com o link na geração máxima: %+v", x)
	}
	f.Board.PCSystemType = 2 // notebook
	x = find(diag.Build(f, nil), "gpu.pcie.width")
	if x == nil || x.Severity != diag.SevGood || strings.Contains(x.Explanation, "slot superior") {
		t.Fatalf("notebook: %+v", x)
	}
	// Report 0.6.1 (no generation read): still a notebook note, no slot advice.
	f.Nvidia = facts.NvidiaInfo{SMI: true, PCIeWidth: 8, PCIeWidthMax: 16, PCIeGenMax: 4, UtilPct: -1}
	if x := find(diag.Build(f, nil), "gpu.pcie.width"); x == nil || x.Severity != diag.SevGood {
		t.Fatalf("notebook sem geração lida: %+v", x)
	}
	f.Nvidia.PCIeWidth = 16
	if x := find(diag.Build(f, nil), "gpu.pcie.width"); x != nil {
		t.Fatalf("x16 de x16: %+v", x)
	}
}

func whea(id, level int, at time.Time, data map[string]string) sys.Event {
	return sys.Event{Provider: "Microsoft-Windows-WHEA-Logger", ID: id, Level: level, Time: at, Data: data}
}

// Corrected PCIe errors logged only while the PC starts or wakes up (links
// training) are not treated like errors during use, and the device is named.
// A machine check right after a resume still is (review of 0.6.2).
func TestWHEAAtBootOnly(t *testing.T) {
	boot := time.Now().Add(-3 * time.Hour)
	pcie := map[string]string{"ErrorSource": "4", "PrimaryDeviceName": `PCI\VEN_8086&DEV_A73E&SUBSYS_0BF81028&REV_01`, "ClassCode": "0x60400", "VendorId": "0x8086", "DeviceId": "0xa73e"}
	evs := []sys.Event{
		{Provider: "Microsoft-Windows-Kernel-General", ID: 12, Level: 4, Time: boot},
		whea(17, 3, boot.Add(20*time.Second), pcie),
		whea(17, 3, boot.Add(40*time.Second), pcie),
		whea(46, 4, boot.Add(2*time.Hour), nil), // informational record: not an error
	}
	f := base()
	f.Board.PCSystemType = 2
	x := whFinding(t, evs, f)
	if x.Severity != diag.SevInfo || !strings.HasPrefix(x.Title, "2 ") || !strings.Contains(x.Title, "ao ligar") {
		t.Fatalf("só na inicialização: %+v", x)
	}
	// 0.6.3: the device is named as Device Manager names it, never by the
	// class code of the record (logged in a format that does not match the
	// PCI specification).
	f.PCIDevices = []facts.PCIDevice{{Name: "Intel(R) PCI Express Root Port #9 - 7A38", PNPID: `PCI\VEN_8086&DEV_A73E&SUBSYS_0BF81028&REV_01\3&11583659&0&E0`}}
	x = whFinding(t, evs, f)
	for _, want := range []string{"PCI Express 2", "Intel(R) PCI Express Root Port #9 - 7A38 (VEN_8086&DEV_A73E): 2", "2 de 2 erros PCIe corrigidos até 5 minutos"} {
		if !strings.Contains(x.Evidence, want) {
			t.Errorf("evidência sem %q: %s", want, x.Evidence)
		}
	}
	if strings.Contains(x.Explanation, "XMP") || len(x.Actions) != 0 {
		t.Errorf("na inicialização não há o que ajustar: %+v", x)
	}
	// Corrected machine checks 4 minutes after each wake-up: a warning.
	cpu := []sys.Event{{Provider: "Microsoft-Windows-Kernel-Power", ID: 507, Level: 4, Time: boot}, whea(19, 3, boot.Add(4*time.Minute), map[string]string{"MciStat": "0x9c00"})}
	if x := whFinding(t, cpu, f); x.Severity != diag.SevWarning || !strings.Contains(x.Explanation, "Processador") {
		t.Errorf("machine check ao acordar: %+v", x)
	}
	// A fatal machine check plus boot-only PCIe: no PCIe advice nor the ASPM test.
	mixed := append(evs, whea(18, 2, boot.Add(time.Hour), map[string]string{"MciStat": "0xbea0000000000108"}))
	x = whFinding(t, mixed, f)
	if x.Severity != diag.SevCritical || strings.Contains(x.Explanation, "PCI Express:") || len(x.Actions) != 0 || !strings.Contains(x.Explanation, "Processador") {
		t.Errorf("fatal de processador com PCIe só na inicialização: %+v", x)
	}
}

// Errors during use: by kind, with platform-aware advice (no XMP/PBO for a
// notebook) and the ASPM test for PCIe errors. The GPU is named.
func TestWHEADuringUse(t *testing.T) {
	boot := time.Now().Add(-10 * time.Hour)
	gpu := map[string]string{"PrimaryDeviceName": `PCI\VEN_8086&DEV_A70D`, "SecondaryDeviceName": `PCI\VEN_10DE&DEV_25EC&SUBSYS_0BF81028`, "ClassCode": "0x60400"}
	evs := []sys.Event{
		{Provider: "Microsoft-Windows-Kernel-Boot", ID: 27, Level: 4, Time: boot},
		whea(17, 3, boot.Add(3*time.Hour), gpu),
		whea(19, 3, boot.Add(4*time.Hour), map[string]string{"MciStat": "0x9c0000000000011c"}),
	}
	f := base()
	f.GPUs = []facts.GPUInfo{{Name: "NVIDIA GeForce RTX 3050 6GB Laptop GPU", Vendor: "NVIDIA", PNPDeviceID: `PCI\VEN_10DE&DEV_25EC&SUBSYS_0BF81028&REV_A1\4&2A788C0F&0&0008`}}
	f.Board.PCSystemType = 2
	x := whFinding(t, evs, f)
	if x.Severity != diag.SevWarning || !strings.Contains(x.Evidence, "→ NVIDIA GeForce RTX 3050 6GB Laptop GPU (VEN_10DE&DEV_25EC)") || !strings.Contains(x.Evidence, "0 de 1 erros PCIe corrigidos") {
		t.Fatalf("durante o uso: %+v", x)
	}
	// Without a PnP name the vendor still names the port; the class code never does.
	if !strings.Contains(x.Evidence, "dispositivo PCIe Intel (VEN_8086&DEV_A70D)") || strings.Contains(x.Evidence, "porta PCIe") {
		t.Errorf("nome sem PnP: %s", x.Evidence)
	}
	if strings.Contains(x.Explanation, "XMP") || !strings.Contains(x.Explanation, "ThrottleStop") || len(x.Actions) != 1 || x.Actions[0].Tweaks[0] != "power.pcie.aspm" {
		t.Errorf("conselho de notebook: %+v", x)
	}
	f.Board.PCSystemType = 1 // desktop
	if x := whFinding(t, evs, f); !strings.Contains(x.Explanation, "XMP/EXPO") || !strings.Contains(x.Explanation, "riser") {
		t.Errorf("conselho de desktop: %s", x.Explanation)
	}
	// A fatal record (the machine check that crashed the PC).
	evs = append(evs, whea(18, 2, boot.Add(5*time.Hour), map[string]string{"MciStat": "0xbea0000000000108"}))
	if x := whFinding(t, evs, f); x.Severity != diag.SevCritical || !strings.HasPrefix(x.Title, "1 erro de hardware grave") {
		t.Errorf("erro fatal: %+v", x)
	}
	// Without boot information, as 0.6.1: every corrected error is a warning.
	if x := whFinding(t, []sys.Event{whea(19, 3, boot, nil)}, nil); x.Severity != diag.SevWarning || strings.Contains(x.Evidence, "minutos") {
		t.Errorf("sem horário de boot: %+v", x)
	}
}

func whFinding(t *testing.T, evs []sys.Event, f *facts.Facts) diag.Finding {
	t.Helper()
	for _, x := range diag.StabilityFor(evs, f) {
		if x.ID == "stab.whea" {
			return x
		}
	}
	t.Fatalf("sem achado WHEA para %d eventos", len(evs))
	return diag.Finding{}
}

// VBS without Memory Integrity, and a "VBS off" in the registry that Windows
// does not honour (the first notebook report).
func TestVBSFinding(t *testing.T) {
	f := base()
	f.Security = facts.SecurityInfo{VBSRunning: true, VBSLocked: true, HypervisorFeatures: []string{"Plataforma de Máquina Virtual (WSL2, Android)"}}
	x := find(diag.Build(f, nil), "sec.vbs")
	if x == nil || x.Title != "VBS ligado (Integridade de Memória desligada)" || !strings.Contains(x.Evidence, "também é iniciado por") || len(x.Actions) != 1 {
		t.Fatalf("VBS sem HVCI: %+v", x)
	}
	st := map[string]tweaks.State{"adv.vbs.off": {ID: "adv.vbs.off", Applicable: true, Status: "on", Effect: "ignored", EffectNote: "x"}}
	x = find(diag.Build(f, st), "sec.vbs")
	if x == nil || !strings.Contains(x.Title, "continua ligado") || !strings.Contains(x.Explanation, "bloqueio UEFI") || len(x.Actions) != 0 {
		t.Fatalf("VBS ignorando o registro: %+v", x)
	}
	st["adv.vbs.off"] = tweaks.State{ID: "adv.vbs.off", Applicable: true, Status: "on", Effect: "reboot"}
	if x = find(diag.Build(f, st), "sec.vbs"); x == nil || !strings.Contains(x.Title, "reinício pendente") {
		t.Fatalf("reinício pendente: %+v", x)
	}
	f.Security.HVCIRunning, f.Security.ServicesRunning = true, []int{2}
	if x = find(diag.Build(f, nil), "sec.vbs"); x == nil || x.Title != "VBS e Integridade de Memória ligados" || !strings.Contains(x.Evidence, "Integridade de Memória (HVCI)") {
		t.Fatalf("VBS com HVCI: %+v", x)
	}
}

// Network settings are read on every adapter: the unplugged Realtek of the
// first notebook report had the speed fixed and 13 changed properties.
func TestNetworkFindingsOnAllAdapters(t *testing.T) {
	f := base()
	f.Adapters = []facts.Adapter{
		{Description: "Realtek PCIe GbE Family Controller", Kind: "Ethernet", Up: false, ClassKey: `HKLM\x\0001`, Keywords: map[string]facts.NicKeyword{
			"*SpeedDuplex":   {Default: "0", Current: "6", Set: true, Labels: map[string]string{"6": "1.0 Gbps Full Duplex"}},
			"*FlowControl":   {Default: "3", Current: "0", Set: true, Name: "Flow Control", Labels: map[string]string{"0": "Disabled", "3": "Rx & Tx Enabled"}},
			"*EEE":           {Default: "1", Current: "0", Set: true},
			"*WakeOnPattern": {Default: "1", Current: "0", Set: true},
		}},
		{Description: "Intel(R) Wi-Fi 6 AX201 160MHz", Kind: "Wi-Fi", Up: true, HasGateway: true, LinkSpeedMbps: 866, ClassKey: `HKLM\x\0000`, Keywords: map[string]facts.NicKeyword{
			"ThroughputBoosterEnabled": {Default: "0", Current: "1", Set: true, Labels: map[string]string{"0": "Disabled", "1": "Enabled"}},
		}},
	}
	r := diag.Build(f, nil)
	sp := find(r, "net.speed")
	if sp == nil || sp.Severity != diag.SevInfo || !strings.Contains(sp.Evidence, "(desconectada)") || !strings.Contains(sp.Evidence, "1.0 Gbps Full Duplex") {
		t.Fatalf("velocidade fixa na placa desconectada: %+v", sp)
	}
	nd := find(r, "net.nic.defaults")
	if nd == nil || !strings.HasPrefix(nd.Title, "2 ") || nd.Severity != diag.SevInfo {
		t.Fatalf("opções fora do padrão: %+v", nd)
	}
	for _, want := range []string{"Flow Control / Controle de fluxo (802.3x) = Disabled (padrão: Rx & Tx Enabled)", "Throughput Booster = Enabled (padrão: Disabled)", "Realtek PCIe GbE Family Controller (desconectada)"} {
		if !strings.Contains(nd.Evidence, want) {
			t.Errorf("evidência sem %q: %s", want, nd.Evidence)
		}
	}
	if strings.Contains(nd.Evidence, "EEE") || strings.Contains(nd.Evidence, "Wake") || !strings.Contains(nd.Explanation, "Intel avisa") {
		t.Errorf("EEE e Wake-on-LAN ficam de fora; nota do Throughput Booster: %+v", nd)
	}
	if find(r, "net.wifi") == nil || find(r, "net.eee") != nil {
		t.Errorf("Wi-Fi ativo: achado de Wi-Fi sim, EEE da placa desconectada não")
	}
}

// The Vmin alert follows the CPUID signature when Windows reports it.
func TestVminUsesSignature(t *testing.T) {
	f := base()
	f.CPU = facts.CPUInfo{Name: "13th Gen Intel(R) Core(TM) i7-13700K", Vendor: "Intel", Family: 6, Model: 0xB7, Stepping: 1, Microcode: 0x129}
	if x := find(diag.Build(f, nil), "cpu.intel.vmin"); x == nil || x.Severity != diag.SevCritical || !strings.Contains(x.Evidence, "CPUID 06-B7-01") {
		t.Fatalf("06-B7-01 com 0x129: %+v", x)
	}
	f.CPU.Model = 0xBF
	if x := find(diag.Build(f, nil), "cpu.intel.vmin"); x != nil {
		t.Fatalf("06-BF não é afetado: %+v", x)
	}
	f.CPU = facts.CPUInfo{Name: "13th Gen Intel(R) Core(TM) i5-13450HX", Vendor: "Intel", Family: 6, Model: 0xB7, Microcode: 0x129}
	if x := find(diag.Build(f, nil), "cpu.intel.vmin"); x != nil {
		t.Fatalf("processador de notebook (HX) não entra: %+v", x)
	}
}

// FACEIT's rule (support FAQ): from 14/10/2026 Windows 10 must be 22H2 with
// the Extended Security Updates on; Windows 11 from October 2027.
func TestWindows10FACEITText(t *testing.T) {
	f := base()
	f.OS = facts.OSInfo{Caption: "Microsoft Windows 10 Pro", Edition: "Professional", Build: 19044}
	f.Gaming.AntiCheats = []string{"FACEIT"}
	x := find(diag.Build(f, nil), "os.win10")
	if x == nil || x.Severity != diag.SevCritical || !strings.Contains(x.Explanation, "22H2") || !strings.Contains(x.Explanation, "não é a versão 22H2") || strings.Contains(x.Explanation, "exigir Windows 11 a partir de 14/10/2026") {
		t.Fatalf("Windows 10 21H2 com FACEIT: %+v", x)
	}
	f.OS.Build = 19045
	if x = find(diag.Build(f, nil), "os.win10"); x == nil || !strings.Contains(x.Explanation, "inscrição no ESU") {
		t.Fatalf("Windows 10 22H2 com FACEIT: %+v", x)
	}
}

// Review of 0.6.2: FACEIT installed with the IOMMU reported off is not
// "requirements met".
func TestFACEITWithIOMMUOff(t *testing.T) {
	f := base()
	f.Gaming.AntiCheats = []string{"FACEIT"}
	f.Security = facts.SecurityInfo{TPMReady: true, TPMVersion: "2.0", SecureBoot: "on", VBSRunning: true, IOMMU: "off"}
	r := diag.Build(f, nil)
	if x := find(r, "sec.anticheat.iommu"); x == nil || x.Severity != diag.SevWarning {
		t.Fatalf("IOMMU desligado com FACEIT: %+v", x)
	}
	if x := find(r, "sec.anticheat"); x != nil && x.ID == "sec.anticheat" && x.Severity == diag.SevGood {
		t.Errorf("não pode dizer 'atendidos' com o IOMMU desligado: %+v", x)
	}
	f.Security.IOMMU = "on"
	r = diag.Build(f, nil)
	if x := find(r, "sec.anticheat.iommu"); x != nil {
		t.Errorf("IOMMU ligado: %+v", x)
	}
	if x := find(r, "sec.anticheat"); x == nil || x.Severity != diag.SevGood || !strings.Contains(x.Evidence, "IOMMU ligado") {
		t.Errorf("requisitos atendidos com IOMMU: %+v", x)
	}
}

// Review of 0.6.2: a speed fixed at 100 Mb/s on an unplugged port is not
// "limiting your internet" today; on the connected one it is.
func TestSlowSpeedOnUnpluggedPort(t *testing.T) {
	f := base()
	f.Adapters = []facts.Adapter{{Description: "Realtek PCIe GbE Family Controller", Kind: "Ethernet", Up: false, ClassKey: `HKLM\x\0001`,
		Keywords: map[string]facts.NicKeyword{"*SpeedDuplex": {Default: "0", Current: "4", Set: true}}}}
	if x := find(diag.Build(f, nil), "net.speed"); x == nil || x.Severity != diag.SevInfo || !strings.Contains(x.Explanation, "quando o cabo for usado") {
		t.Fatalf("porta desconectada: %+v", x)
	}
	f.Adapters[0].Up = true
	if x := find(diag.Build(f, nil), "net.speed"); x == nil || x.Severity != diag.SevWarning {
		t.Fatalf("porta conectada: %+v", x)
	}
}

func TestPCIeTitleWithoutGeneration(t *testing.T) {
	f := base()
	f.Nvidia = facts.NvidiaInfo{SMI: true, PCIeWidth: 8, PCIeWidthMax: 16, PCIeGenMax: 4, UtilPct: -1}
	if x := find(diag.Build(f, nil), "gpu.pcie.width"); x == nil || !strings.Contains(x.Title, "sem confirmar") || x.Severity != diag.SevInfo {
		t.Fatalf("geração não lida: %+v", x)
	}
}

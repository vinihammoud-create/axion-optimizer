package diag_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"axion/internal/catalog"
	"axion/internal/collect"
	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

func mockReport(t *testing.T) (*facts.Facts, *diag.Report, map[string]tweaks.State) {
	t.Helper()
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := collect.Collect(context.Background(), m.Platform, nil)
	cat, err := tweaks.NewCatalog(catalog.All())
	if err != nil {
		t.Fatal(err)
	}
	j, err := tweaks.OpenJournal(m.DataDir, m.Key)
	if err != nil {
		t.Fatal(err)
	}
	e := tweaks.NewEngine(m.Platform, cat, j)
	st := map[string]tweaks.State{}
	for _, s := range e.States(context.Background(), f) {
		st[s.ID] = s
	}
	return f, diag.Build(f, st), st
}

func find(r *diag.Report, prefix string) *diag.Finding {
	for i := range r.Findings {
		if strings.HasPrefix(r.Findings[i].ID, prefix) {
			return &r.Findings[i]
		}
	}
	return nil
}

func TestMockDiagnostic(t *testing.T) {
	_, r, _ := mockReport(t)
	want := map[string]string{
		"display.refresh.": diag.SevCritical, // 240 Hz monitor left at 60 Hz
		"ram.xmp":          diag.SevWarning,  // KF560C36 (6000) running at 4800: an opportunity, not a failure
		"disk.wear.":       diag.SevWarning,  // 81 % wear
		"boot.hpet":        diag.SevWarning,  // useplatformclock left by an old script
		"game.dvr":         diag.SevWarning,  // background recording
		"gpu.driver.old":   diag.SevWarning,  // driver from 01/2025
		"input.accel":      diag.SevInfo,
		"net.eee":          diag.SevInfo,
		"gpu.rebar":        diag.SevInfo,
		"sec.anticheat":    diag.SevGood, // FACEIT with TPM + Secure Boot + VBS
		"cpu.x3d.single":   diag.SevGood,
		// Residue left by other optimizers.
		"res.offload":  diag.SevInfo,
		"res.pagefile": diag.SevInfo, // fixed 4096 MB
		"res.wu":       diag.SevWarning,
		"res.svc.apps": diag.SevInfo,
		"res.spectre":  diag.SevInfo,
		// Peripherals: generic low-speed keyboard behind a hub, with the
		// vendor's selective suspend on, and a shrunk mouse queue.
		"per.lowspeed.1A2C-2124": diag.SevWarning,
		"per.hub.1A2C-2124":      diag.SevInfo,
		"per.suspend.1A2C-2124":  diag.SevWarning,
		"res.inputqueue":         diag.SevWarning,
	}
	for id, sev := range want {
		x := find(r, id)
		if x == nil {
			t.Errorf("achado %q ausente", id)
			continue
		}
		if x.Severity != sev {
			t.Errorf("%s: severidade %s, esperado %s", id, x.Severity, sev)
		}
		if x.Title == "" || x.Evidence == "" {
			t.Errorf("%s: título/evidência vazios", id)
		}
	}
	for _, id := range []string{"ram.single", "ram.samechannel", "cpu.x3d.dual", "disk.hdd", "sec.vbs", "os.win10", "net.wifi", "gpu.hags", "res.dse", "res.uac", "res.svc.system", "res.tdr", "axion.drift",
		"per.rate.046D-C547", "per.hub.046D-C547", "per.suspend.046D-C547", "per.lowspeed.046D-C547", "input.filterkeys", "res.threaddpc", "res.bootlimits"} {
		if x := find(r, id); x != nil {
			t.Errorf("achado indevido %s: %s", id, x.Title)
		}
	}
	// Ordering: critical first, good last.
	last := -1
	rank := map[string]int{diag.SevCritical: 0, diag.SevWarning: 1, diag.SevInfo: 2, diag.SevGood: 3}
	for _, x := range r.Findings {
		if rank[x.Severity] < last {
			t.Fatalf("achados fora de ordem de severidade")
		}
		last = rank[x.Severity]
	}
	if r.Overall < 12 || r.Overall >= 75 || len(r.Scores) != 6 {
		t.Errorf("nota geral inesperada %d (%d áreas)", r.Overall, len(r.Scores))
	}
	// Every tweak action must reference a real catalog tweak.
	cat, _ := tweaks.NewCatalog(catalog.All())
	for _, x := range r.Findings {
		for _, a := range x.Actions {
			for _, id := range a.Tweaks {
				if cat.Get(id) == nil {
					t.Errorf("%s: ação aponta para otimização inexistente %s", x.ID, id)
				}
			}
		}
	}
}

func TestStabilityFindings(t *testing.T) {
	now := time.Now()
	evs := []sys.Event{
		{Provider: "Microsoft-Windows-WHEA-Logger", ID: 19, Time: now.Add(-time.Hour)},
		{Provider: "Microsoft-Windows-WHEA-Logger", ID: 17, Time: now.Add(-2 * time.Hour)},
		{Provider: "Microsoft-Windows-Kernel-Power", ID: 41, Time: now.Add(-24 * time.Hour)},
		{Provider: "Microsoft-Windows-Kernel-Power", ID: 42, Time: now}, // sleep: ignored
		{Provider: "Microsoft-Windows-WER-SystemErrorReporting", ID: 1001, Time: now},
		{Provider: "Display", ID: 4101, Time: now},
	}
	got := map[string]diag.Finding{}
	for _, x := range diag.Stability(evs) {
		got[x.ID] = x
	}
	if x, ok := got["stab.whea"]; !ok || !strings.HasPrefix(x.Title, "2 ") {
		t.Errorf("WHEA: %+v", x)
	}
	if x, ok := got["stab.kp41"]; !ok || !strings.HasPrefix(x.Title, "1 ") {
		t.Errorf("Kernel-Power 41: %+v", x)
	}
	if x, ok := got["stab.bsod"]; !ok || x.Severity != diag.SevCritical {
		t.Errorf("BSOD: %+v", x)
	}
	if _, ok := got["stab.tdr"]; !ok {
		t.Errorf("TDR ausente")
	}
	if len(diag.Stability(nil)) != 0 {
		t.Errorf("sem eventos não deveria gerar achados")
	}
}

func TestRatedSpeed(t *testing.T) {
	cases := map[string]int{
		"KF560C36-16":         6000,
		"KF436C17BBK2/16":     3600,
		"F5-6000J3038F16G":    6000,
		"F4-3200C16-8GVKB":    3200,
		"CMK32GX5M2B5600C36":  5600,
		"CMK16GX4M2B3200C16":  3200,
		"BL8G32C16U4B.M8FE1":  3200,
		"CP16G60C36U5B.M8D1":  6000,
		"FF3D516G6000HC38ABK": 6000,
		"M378A1K43DB2-CTD":    0, // Samsung JEDEC module: unknown rated speed
		"":                    0,
	}
	for part, want := range cases {
		if got := diag.RatedSpeed(part); got != want {
			t.Errorf("RatedSpeed(%q) = %d, esperado %d", part, got, want)
		}
	}
}

// Fixture-based rules: very different PCs.
func base() *facts.Facts {
	return &facts.Facts{
		OS:  facts.OSInfo{Caption: "Microsoft Windows 11 Pro", Build: 26300, IsWin11: true, SystemDrive: "C:"},
		CPU: facts.CPUInfo{Name: "AMD Ryzen 9 7950X3D 16-Core Processor", Vendor: "AMD", Cores: 16, Threads: 32, IsX3D: true, IsDualCCDX3D: true},
		RAM: facts.RAMInfo{TotalBytes: 32 << 30, Type: "DDR5", ConfiguredMTs: 6000, Modules: []facts.MemModule{
			{CapacityBytes: 16 << 30, PartNumber: "F5-6000J3038F16G", Bank: "P0 CHANNEL A", Locator: "DIMM 1"},
			{CapacityBytes: 16 << 30, PartNumber: "F5-6000J3038F16G", Bank: "P0 CHANNEL B", Locator: "DIMM 1"},
		}},
		Gaming: facts.GamingInfo{GameModeOn: true, GameBarInstalled: true, X3DServiceState: "em execução"},
		Power:  facts.PowerInfo{ActiveScheme: sys.SchemeBalanced},
	}
}

func TestDualCCDX3D(t *testing.T) {
	f := base()
	r := diag.Build(f, nil)
	if x := find(r, "cpu.x3d.dual"); x == nil || x.Severity != diag.SevGood {
		t.Fatalf("7950X3D configurado corretamente deveria ser 'good': %+v", x)
	}
	f.Gaming.GameBarInstalled = false
	f.Power.ActiveScheme = sys.SchemeHighPerformance
	r = diag.Build(f, nil)
	x := find(r, "cpu.x3d.dual")
	if x == nil || x.Severity != diag.SevCritical || !strings.Contains(x.Evidence, "Game Bar") || !strings.Contains(x.Evidence, "Equilibrado") {
		t.Fatalf("7950X3D sem Game Bar/plano Equilibrado deveria ser crítico: %+v", x)
	}
	if find(r, "ram.xmp") != nil {
		t.Errorf("kit a 6000 com perfil ativo não deveria gerar ram.xmp")
	}
}

func TestLowEndMachine(t *testing.T) {
	f := &facts.Facts{
		OS:  facts.OSInfo{Caption: "Microsoft Windows 10 Home", Build: 19045, SystemDrive: "C:"},
		CPU: facts.CPUInfo{Name: "Intel(R) Core(TM) i5-12400F", Vendor: "Intel", Cores: 6, Threads: 12},
		RAM: facts.RAMInfo{TotalBytes: 8 << 30, Type: "DDR4", ConfiguredMTs: 2666, Modules: []facts.MemModule{
			{CapacityBytes: 8 << 30, PartNumber: "M378A1K43DB2-CTD", Bank: "BANK 0", Locator: "ChannelA-DIMM0"},
		}},
		Disks:    []facts.Disk{{Model: "ST1000DM010", MediaType: "HDD", BusType: "SATA", Health: "Healthy", IsSystem: true, WearPct: -1, TempC: -1}},
		Volumes:  []facts.Volume{{Letter: "C:", SizeBytes: 1000 << 30, FreeBytes: 9 << 30}},
		Gaming:   facts.GamingInfo{GameModeOn: false, AntiCheats: []string{"FACEIT"}},
		Adapters: []facts.Adapter{{Description: "Realtek PCIe GbE Family Controller", Kind: "Ethernet", LinkSpeedMbps: 100, Up: true, HasGateway: true}},
		// Read and absent (an unread TPM is "unknown", not "missing").
		Security: facts.SecurityInfo{TPMRead: true, SecureBoot: "off"},
	}
	r := diag.Build(f, nil)
	for id, sev := range map[string]string{
		"disk.hdd":      diag.SevCritical,
		"os.win10":      diag.SevCritical, // FACEIT requires Windows 11 from 14/10/2026
		"ram.single":    diag.SevWarning,
		"ram.low":       diag.SevWarning,
		"disk.free":     diag.SevWarning,
		"game.mode":     diag.SevWarning,
		"net.link100":   diag.SevWarning,
		"sec.anticheat": diag.SevCritical, // no TPM/Secure Boot/VBS
	} {
		x := find(r, id)
		if x == nil {
			t.Errorf("achado %s ausente", id)
			continue
		}
		if x.Severity != sev {
			t.Errorf("%s: severidade %s, esperado %s", id, x.Severity, sev)
		}
	}
	if r.Overall >= 55 {
		t.Errorf("PC com HD, 8 GB e Windows 10 deveria ter nota baixa, obtido %d", r.Overall)
	}
	// A Samsung JEDEC module (M378...) has no profile to turn on.
	if x := find(r, "ram.xmp"); x != nil {
		t.Errorf("módulo JEDEC sem perfil não deveria gerar ram.xmp: %+v", x)
	}
	// An unknown part at the standard speed only gets a hint.
	f.RAM.Modules[0].PartNumber = "XYZ-GAMER-3200"
	if x := find(diag.Build(f, nil), "ram.xmp"); x == nil || x.Severity != diag.SevInfo {
		t.Errorf("módulo desconhecido a 2666: esperado aviso informativo, obtido %+v", x)
	}
	// Notebooks have no profile to enable.
	f.Power.IsLaptop = true
	if x := find(diag.Build(f, nil), "ram.xmp"); x != nil {
		t.Errorf("notebook não deveria gerar ram.xmp: %+v", x)
	}
	if r.Grade != "Precisa de atenção" {
		t.Errorf("classificação %q", r.Grade)
	}
}

func TestCleanMachineScoresHigh(t *testing.T) {
	f := base()
	f.CPU = facts.CPUInfo{Name: "AMD Ryzen 7 9800X3D 8-Core Processor", Vendor: "AMD", Cores: 8, Threads: 16, IsX3D: true}
	f.Disks = []facts.Disk{{Model: "Samsung SSD 990 PRO 2TB", MediaType: "SSD", BusType: "NVMe", Health: "Healthy", WearPct: 2, TempC: 41, IsSystem: true}}
	f.Volumes = []facts.Volume{{Letter: "C:", SizeBytes: 2000 << 30, FreeBytes: 1200 << 30}}
	f.Adapters = []facts.Adapter{{Description: "Intel(R) Ethernet Controller I226-V", Kind: "Ethernet", LinkSpeedMbps: 2500, Up: true, HasGateway: true}}
	r := diag.Build(f, nil)
	for _, x := range r.Findings {
		if x.Severity == diag.SevCritical || x.Severity == diag.SevWarning {
			t.Errorf("PC limpo com achado %s (%s): %s", x.ID, x.Severity, x.Title)
		}
	}
	if r.Overall < 90 || r.Grade != "Excelente" {
		t.Errorf("PC limpo deveria ser Excelente, obtido %d %s", r.Overall, r.Grade)
	}
}

func TestMemChannel(t *testing.T) {
	cases := []struct{ bank, loc, want string }{
		{"P0 CHANNEL A", "DIMM 1", "A"},
		{"P0 CHANNEL B", "DIMM 1", "B"},
		{"BANK 0", "ChannelA-DIMM0", "A"},
		{"BANK 2", "ChannelB-DIMM0", "B"},
		{"BANK 0", "Controller0-ChannelA-DIMM1", "C0-A"},
		{"BANK 0", "Controller1-ChannelA-DIMM1", "C1-A"},
		{"BANK 1", "DIMM_A2", "A"},
		{"BANK 3", "DIMM_B2", "B"},
		{"", "A1", "A"},
		{"P0_Node0_Channel1_Dimm0", "DIMM 0", "1"},
		{"BANK 0", "DIMM 0", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := diag.MemChannel(facts.MemModule{Bank: c.bank, Locator: c.loc}); got != c.want {
			t.Errorf("MemChannel(%q, %q) = %q, esperado %q", c.bank, c.loc, got, c.want)
		}
	}
	// Two sticks in the same channel.
	f := base()
	f.RAM.Modules[1].Bank = "P0 CHANNEL A"
	f.RAM.Modules[1].Locator = "DIMM 2"
	if x := find(diag.Build(f, nil), "ram.samechannel"); x == nil {
		t.Fatalf("pentes no mesmo canal não detectados")
	}
}

func TestResidueFindings(t *testing.T) {
	f := base()
	f.Gaming.AntiCheats = []string{"Vanguard"}
	f.Boot = facts.BootInfo{Readable: true, TestSigning: "Yes", NX: "AlwaysOff"}
	f.Residue = facts.Residue{Readable: true, UACOff: true, Tcp1323Opts: 0, TdrLevel: 0, PageFile: "none", NetworkThrottling: 0xFFFFFFFF, MMAgentRead: true, MemoryCompression: false, HostsEntries: 120, HostsSample: []string{"vortex.data.microsoft.com"},
		NVMeOverrides:    []string{"735209102", "1853569164", "156965516"},
		DisabledServices: []facts.DisabledService{{Name: "wuauserv", Display: "Windows Update", Impact: "sem atualizações", Group: "system", Default: sys.StartManual}},
		UpdateBlocks:     []facts.RegRef{{Path: `HKLM\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate\AU`, Name: "NoAutoUpdate", Why: "atualizações automáticas desativadas por política"}}}
	r := diag.Build(f, nil)
	for id, sev := range map[string]string{
		"res.dse":             diag.SevCritical, // anti-cheat installed
		"res.dep":             diag.SevWarning,
		"res.uac":             diag.SevWarning,
		"res.tcp1323":         diag.SevWarning,
		"res.tdr":             diag.SevWarning,
		"res.pagefile":        diag.SevWarning,
		"res.wu":              diag.SevWarning,
		"res.svc.system":      diag.SevWarning,
		"res.hosts":           diag.SevInfo,
		"res.nvme.override":   diag.SevWarning,
		"res.net.throttling":  diag.SevInfo,
		"res.mem.compression": diag.SevInfo,
	} {
		x := find(r, id)
		if x == nil {
			t.Errorf("achado %s ausente", id)
			continue
		}
		if x.Severity != sev {
			t.Errorf("%s: severidade %s, esperado %s", id, x.Severity, sev)
		}
	}
	// The fix is attached to the finding.
	if x := find(r, "res.mem.compression"); x == nil || len(x.Actions) != 1 || len(x.Actions[0].Tweaks) != 1 || x.Actions[0].Tweaks[0] != "fix.mem.compression" {
		t.Errorf("res.mem.compression sem a correção: %+v", x)
	}
	// Compression on, or a state that could not be read, produces no finding.
	for _, rd := range []bool{true, false} {
		g := f
		g.Residue.MMAgentRead, g.Residue.MemoryCompression = rd, rd
		if x := find(diag.Build(g, nil), "res.mem.compression"); x != nil {
			t.Errorf("MMAgentRead=%v ligada=%v não deveria gerar achado", rd, rd)
		}
	}
	g := f
	g.Residue.MMAgentRead, g.Residue.MemoryCompression = false, false
	if x := find(diag.Build(g, nil), "res.mem.compression"); x != nil {
		t.Error("estado não lido nunca é achado")
	}
	// Defaults produce nothing.
	f.Boot = facts.BootInfo{Readable: true, NX: "OptIn"}
	f.Residue = facts.Residue{Readable: true, Tcp1323Opts: -1, TdrLevel: -1, PageFile: "auto", NetworkThrottling: 10}
	for _, x := range diag.Build(f, nil).Findings {
		if strings.HasPrefix(x.ID, "res.") {
			t.Errorf("padrão do Windows gerou achado %s", x.ID)
		}
	}
}

func TestDriftFinding(t *testing.T) {
	f := base()
	st := map[string]tweaks.State{
		"game.mode":  {ID: "game.mode", ByAxion: true, Drifted: true, Status: "off", Applicable: true},
		"ui.fileext": {ID: "ui.fileext", ByAxion: true, Status: "on", Applicable: true},
	}
	x := find(diag.Build(f, st), "axion.drift")
	if x == nil || len(x.Actions) != 1 || len(x.Actions[0].Tweaks) != 1 || x.Actions[0].Tweaks[0] != "game.mode" {
		t.Fatalf("drift: %+v", x)
	}
}

func TestFirmwareRules(t *testing.T) {
	f := base()
	f.CPU = facts.CPUInfo{Name: "13th Gen Intel(R) Core(TM) i9-13900K", Vendor: "Intel", Cores: 24, Threads: 32, Microcode: 0x129}
	if x := find(diag.Build(f, nil), "cpu.intel.vmin"); x == nil || x.Severity != diag.SevCritical {
		t.Fatalf("0x129 deveria ser crítico: %+v", x)
	}
	f.CPU.Microcode = 0x12B
	if x := find(diag.Build(f, nil), "cpu.intel.vmin"); x == nil || x.Severity != diag.SevInfo {
		t.Fatalf("0x12B deveria ser informativo: %+v", x)
	}
	f.CPU.Microcode = 0x12F
	if x := find(diag.Build(f, nil), "cpu.intel.vmin"); x != nil {
		t.Fatalf("0x12F não deveria gerar achado")
	}
	f.CPU.Microcode = 0 // unknown: say nothing
	if x := find(diag.Build(f, nil), "cpu.intel.vmin"); x != nil {
		t.Fatalf("microcode desconhecido não deveria gerar achado")
	}
	g := base()
	g.CPU = facts.CPUInfo{Name: "AMD Ryzen 7 7800X3D 8-Core Processor", Vendor: "AMD", Cores: 8, Threads: 16, IsX3D: true}
	g.Board.BIOSDate = time.Date(2023, 2, 10, 0, 0, 0, 0, time.UTC)
	if x := find(diag.Build(g, nil), "cpu.am5.soc"); x == nil || x.Severity != diag.SevCritical {
		t.Fatalf("BIOS AM5 antiga com X3D deveria ser crítica: %+v", x)
	}
	g.Board.BIOSDate = time.Date(2025, 3, 12, 0, 0, 0, 0, time.UTC)
	if x := find(diag.Build(g, nil), "cpu.am5.soc"); x != nil {
		t.Fatalf("BIOS atual não deveria gerar achado")
	}
}

// Real report (v0.1.0): "disabledynamictick No" is what the undo of a tick
// tweak leaves behind. It behaves like the default and must not be flagged.
func TestBootNoValuesAreDefault(t *testing.T) {
	f := base()
	f.Boot = facts.BootInfo{Readable: true, NX: "OptIn", DisableDynamicTick: "No", UsePlatformTick: "No", UsePlatformClock: "No"}
	r := diag.Build(f, nil)
	for _, id := range []string{"boot.tick", "boot.hpet"} {
		if x := find(r, id); x != nil {
			t.Errorf("%s com valor \"No\": %+v", id, x)
		}
	}
	f.Boot = facts.BootInfo{Readable: true, NX: "OptIn", DisableDynamicTick: "Yes", UsePlatformTick: "No", UsePlatformClock: "Yes"}
	r = diag.Build(f, nil)
	x := find(r, "boot.tick")
	if x == nil || !strings.Contains(x.Evidence, "disabledynamictick = Yes") || strings.Contains(x.Evidence, "useplatformtick") {
		t.Fatalf("boot.tick: %+v", x)
	}
	if find(r, "boot.hpet") == nil {
		t.Fatal("boot.hpet ausente com useplatformclock = Yes")
	}
}

func TestNVMeHealthFindings(t *testing.T) {
	f := base()
	h := &sys.NVMeHealth{CriticalWarning: sys.NVMeWarnReliability | sys.NVMeWarnTemperature, TempC: 78, SparePct: 100, SpareThreshold: 10, PercentUsed: 104,
		DataWrittenBytes: 63_400_000_000_000, PowerOnHours: 14_210, PowerCycles: 2_000, UnsafeShutdowns: 480, MediaErrors: 7, WarnTempMinutes: 95}
	f.Disks = []facts.Disk{{Model: "WD Green SN350 480GB", BusType: "NVMe", MediaType: "SSD", Health: "Healthy", Number: 0, IsSystem: true,
		WearPct: 104, TempC: 78, PowerOnHours: 14_210, NVMe: h, HealthSource: "nvme"}}
	r := diag.Build(f, nil)
	for id, sev := range map[string]string{
		"disk.wear.0":        diag.SevCritical,
		"disk.nvme.alert.0":  diag.SevCritical,
		"disk.nvme.temp.0":   diag.SevWarning,
		"disk.nvme.unsafe.0": diag.SevInfo,
	} {
		x := find(r, id)
		if x == nil || x.Severity != sev {
			t.Errorf("%s: %+v", id, x)
		}
	}
	if x := find(r, "disk.wear.0"); x != nil && (!strings.Contains(x.Title, "além da vida útil") || !strings.Contains(x.Evidence, "63,4 TB") || !strings.Contains(x.Evidence, "14.210")) {
		t.Errorf("desgaste: %+v", x)
	}
	// The reliability alert already covers media errors; the generic
	// temperature finding is replaced by the drive's own flag.
	for _, id := range []string{"disk.nvme.media.0", "disk.temp.0"} {
		if x := find(r, id); x != nil {
			t.Errorf("%s duplicado: %+v", id, x)
		}
	}

	// Media errors without a critical flag, few unsafe shutdowns: one warning.
	h2 := *h
	h2.CriticalWarning, h2.UnsafeShutdowns, h2.PercentUsed = 0, 30, 20
	f.Disks[0].NVMe, f.Disks[0].WearPct, f.Disks[0].TempC = &h2, 20, 40
	r = diag.Build(f, nil)
	if x := find(r, "disk.nvme.media.0"); x == nil || x.Severity != diag.SevWarning {
		t.Errorf("erros de mídia: %+v", x)
	}
	for _, id := range []string{"disk.nvme.alert.0", "disk.nvme.temp.0", "disk.nvme.unsafe.0", "disk.wear.0"} {
		if x := find(r, id); x != nil {
			t.Errorf("%s inesperado: %+v", id, x)
		}
	}
}

func TestDuplicatePowerPlans(t *testing.T) {
	f := base()
	f.Power.ActiveName = "Alto desempenho"
	f.Power.Schemes = []sys.PowerScheme{{GUID: "a", Name: "Desempenho Máximo"}, {GUID: "b", Name: "ChrisTitus - Ultimate Power Plan"}, {GUID: "c", Name: "ChrisTitus - Ultimate Power Plan"},
		{GUID: "d", Name: "Equilibrado"}, {GUID: "e", Name: "chrisTitus - ultimate power plan "}, {GUID: "f", Name: "Alto desempenho"}, {GUID: "g", Name: "Desempenho Máximo"}}
	x := find(diag.Build(f, nil), "power.plans.dup")
	if x == nil || !strings.Contains(x.Evidence, "2× Desempenho Máximo, 3× ChrisTitus - Ultimate Power Plan.") {
		t.Fatalf("%+v", x)
	}
	f.Power.Schemes = f.Power.Schemes[:2]
	if x := find(diag.Build(f, nil), "power.plans.dup"); x != nil {
		t.Fatalf("sem duplicatas: %+v", x)
	}
}

func TestHealthUnknownNamesDriver(t *testing.T) {
	f := base()
	f.Disks = []facts.Disk{{Model: "NVMe WDC WDS480G2G0C-", BusType: "NVMe", MediaType: "SSD", Health: "Healthy", Number: 0, IsSystem: true, WearPct: -1}}
	f.Errors = []string{"saúde NVMe do disco 0: log de saúde NVMe do disco 0: namespace Incorrect function.; controladora Incorrect function."}
	f.Controllers = []facts.StorageController{
		{Name: "AMD-RAID Bottom Device", Driver: "rcbottom", PNP: `PCI\VEN_1022&DEV_43F6\3&11583659&0&43`},
		{Name: "Microsoft Storage Spaces Controller", Driver: "spaceport", PNP: `ROOT\SPACEPORT\0000`},
	}
	x := find(diag.Build(f, nil), "disk.health.unknown.0")
	if x == nil || !strings.Contains(x.Evidence, "AMD-RAID Bottom Device (rcbottom)") || strings.Contains(x.Evidence, "spaceport") || !strings.Contains(x.Evidence, "controladora Incorrect function") {
		t.Fatalf("%+v", x)
	}
	// The real case (report 0.4.2): a Phison miniport on a WD SN350.
	f.Controllers = []facts.StorageController{{Name: "Phison NVMe 1.2 Storport Miniport", Driver: "nvme", PNP: `PCI\VEN_15B7&DEV_5019&SUBSYS_501915B7&REV_01\4&218BD16B&0&000A`}}
	x = find(diag.Build(f, nil), "disk.health.unknown.0")
	if x == nil || !strings.Contains(x.Evidence, "Phison NVMe 1.2 Storport Miniport (nvme)") || !strings.Contains(x.Explanation, "controlador NVM Express padrão") {
		t.Fatalf("driver Phison: %+v", x)
	}
	f.Controllers = []facts.StorageController{{Name: "Controlador NVM Express padrão", Driver: "stornvme", PNP: `PCI\VEN_15B7&DEV_5019\4&1`}}
	x = find(diag.Build(f, nil), "disk.health.unknown.0")
	if x == nil || !strings.Contains(x.Evidence, "Controlador NVM Express padrão (stornvme), o driver padrão do Windows") {
		t.Fatalf("%+v", x)
	}
}

func TestTrimFindings(t *testing.T) {
	now := time.Now()
	mk := func() *facts.Facts {
		f := base()
		f.Disks = []facts.Disk{{Model: "WD Green SN350 480GB", BusType: "NVMe", MediaType: "SSD", Health: "Healthy", Number: 0, IsSystem: true, WearPct: 3},
			{Model: "Kingston XS1000", BusType: "USB", MediaType: "Unspecified", Health: "Healthy", Number: 1, WearPct: -1}}
		f.Volumes = []facts.Volume{{Letter: "C:", FS: "NTFS", SizeBytes: 480 << 30, FreeBytes: 200 << 30, Disk: 0, SSD: true},
			{Letter: "D:", FS: "NTFS", SizeBytes: 1000 << 30, FreeBytes: 500 << 30, Disk: 1}}
		f.Trim = facts.Trim{Readable: true, NTFS: 0, ReFS: -1, TaskRead: true, TaskExists: true, TaskOn: true,
			LastRetrim: map[string]time.Time{"C:": now.Add(-10 * 24 * time.Hour), "D:": now.Add(-90 * 24 * time.Hour)}}
		return f
	}
	ids := func(f *facts.Facts) []string {
		var out []string
		for _, x := range diag.Build(f, nil).Findings {
			if strings.HasPrefix(x.ID, "ssd.") {
				out = append(out, x.ID)
			}
		}
		return out
	}
	// Healthy PC; D: is not an SSD, so its old retrim is not reported.
	if got := ids(mk()); len(got) != 0 {
		t.Fatalf("PC em ordem não deveria ter achados de SSD: %v", got)
	}
	f := mk()
	f.Trim.NTFS = 1
	f.Trim.TaskOn = false
	r := diag.Build(f, nil)
	if x := find(r, "ssd.trim.off"); x == nil || x.Severity != diag.SevWarning || len(x.Actions) == 0 || x.Actions[0].Tweaks[0] != "fix.trim.on" {
		t.Errorf("TRIM desligado: %+v", x)
	}
	if x := find(r, "ssd.optimize.off"); x == nil || x.Severity != diag.SevWarning {
		t.Errorf("tarefa desativada: %+v", x)
	}
	f = mk()
	f.Trim.TaskExists, f.Trim.TaskOn = false, false
	if got := ids(f); len(got) != 1 || got[0] != "ssd.optimize.missing" {
		t.Errorf("tarefa removida: %v", got)
	}
	// Stale retrim on the SSD: only when the log shows an old one.
	f = mk()
	f.Trim.LastRetrim["C:"] = now.Add(-60 * 24 * time.Hour)
	if x := find(diag.Build(f, nil), "ssd.retrim.old.C"); x == nil || x.Severity != diag.SevInfo || !strings.Contains(x.Title, "60 dias") {
		t.Errorf("retrim atrasado: %+v", x)
	}
	delete(f.Trim.LastRetrim, "C:")
	if got := ids(f); len(got) != 0 {
		t.Errorf("sem eventos não há como afirmar atraso: %v", got)
	}
	// No SSD: nothing to say about TRIM.
	f = mk()
	f.Disks = f.Disks[1:]
	f.Trim.NTFS = 1
	if got := ids(f); len(got) != 0 {
		t.Errorf("sem SSD: %v", got)
	}
}

// A card left on Windows' basic driver keeps its vendor in the PCI ID and
// has a translated name: it must still be reported as having no driver.
func TestGPUWithoutDriver(t *testing.T) {
	f := base()
	f.GPUs = []facts.GPUInfo{{Name: "Adaptador de Vídeo Básico da Microsoft", Vendor: "NVIDIA", PNPDeviceID: `PCI\VEN_10DE&DEV_2504&SUBSYS_00000000`,
		InfFile: "display.inf", BasicDriver: true, DriverDate: time.Date(2006, 6, 21, 0, 0, 0, 0, time.UTC)}}
	r := diag.Build(f, nil)
	if x := find(r, "gpu.nodriver"); x == nil || x.Severity != diag.SevCritical {
		t.Fatalf("GPU com driver básico: %+v", x)
	}
	if x := find(r, "gpu.driver.old"); x != nil {
		t.Errorf("o driver básico (2006) não é 'driver antigo': %+v", x)
	}
	// Virtual adapters (Parsec, Hyper-V) have nothing to update.
	f.GPUs = []facts.GPUInfo{{Name: "Parsec Virtual Display Adapter", Vendor: "Other", PNPDeviceID: `ROOT\DISPLAY\0000`, DriverDate: time.Now().AddDate(-5, 0, 0)}}
	if x := find(diag.Build(f, nil), "gpu.driver.old"); x != nil {
		t.Errorf("adaptador virtual não deveria ter aviso de driver: %+v", x)
	}
}

// 59 Hz vs 60 Hz is rounding of 59.94 Hz, not a monitor left at a low rate.
func TestRefreshRounding(t *testing.T) {
	f := base()
	f.Displays = []sys.DisplayMode{{Device: `\\.\DISPLAY1`, Monitor: "AOC", Width: 1920, Height: 1080, CurrentHz: 59, MaxHzAtRes: 60}}
	if x := find(diag.Build(f, nil), "display.refresh."+`\\.\DISPLAY1`); x != nil {
		t.Errorf("59 x 60 Hz é arredondamento: %+v", x)
	}
	f.Displays[0].CurrentHz, f.Displays[0].MaxHzAtRes = 60, 240
	if x := find(diag.Build(f, nil), "display.refresh."+`\\.\DISPLAY1`); x == nil || x.Severity != diag.SevCritical {
		t.Errorf("240 Hz a 60 Hz deveria ser importante: %+v", x)
	}
	f.Displays[0].CurrentHz, f.Displays[0].MaxHzAtRes = 144, 165
	if x := find(diag.Build(f, nil), "display.refresh."+`\\.\DISPLAY1`); x == nil || x.Severity != diag.SevWarning {
		t.Errorf("144 x 165 Hz deveria ser ponto de atenção: %+v", x)
	}
}

// One important problem keeps the score out of "Bom".
func TestCriticalCapsScore(t *testing.T) {
	r := diag.Score([]diag.Finding{{ID: "x", Area: diag.AreaEstab, Severity: diag.SevCritical, Title: "BSOD"}})
	if r.Overall > 74 || r.Grade == "Bom" || r.Grade == "Excelente" {
		t.Fatalf("nota com problema importante: %d (%s)", r.Overall, r.Grade)
	}
}

// An i5-13400F on the Alder Lake die (microcode 0x35) is not affected by
// the 13th/14th generation instability.
func TestIntelVminOnlyOnRaptorDie(t *testing.T) {
	f := base()
	f.CPU = facts.CPUInfo{Name: "13th Gen Intel(R) Core(TM) i5-13400F", Vendor: "Intel", Cores: 10, Threads: 16, Microcode: 0x35}
	if x := find(diag.Build(f, nil), "cpu.intel.vmin"); x != nil {
		t.Fatalf("i5-13400F C0 (0x35) não é afetado: %+v", x)
	}
	f.CPU = facts.CPUInfo{Name: "13th Gen Intel(R) Core(TM) i7-13700K", Vendor: "Intel", Cores: 16, Threads: 24, Microcode: 0x119}
	if x := find(diag.Build(f, nil), "cpu.intel.vmin"); x == nil || x.Severity != diag.SevCritical {
		t.Fatalf("i7-13700K com 0x119 deveria ser importante: %+v", x)
	}
}

// A step or part set aside after a crash must not read as "off/absent":
// no false critical findings, and the anti-cheat safety block stays.
func TestUnreadStepsDoNotCreateFindings(t *testing.T) {
	for _, skip := range []string{facts.StepSecurity, facts.StepGaming, facts.PartX3D, facts.StepPower} {
		t.Run(skip, func(t *testing.T) {
			m, err := mocksys.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			f := collect.CollectWith(context.Background(), m.Platform, nil, collect.Options{Skip: map[string]bool{skip: true}})
			if f.Read(skip) {
				t.Fatalf("%s should be listed as unread: %v", skip, f.Unread)
			}
			r := diag.Build(f, map[string]tweaks.State{})
			for _, x := range r.Findings {
				if x.Severity == diag.SevCritical && (strings.HasPrefix(x.ID, "sec.anticheat") || strings.HasPrefix(x.ID, "cpu.x3d")) {
					t.Errorf("false critical finding with %q unread: %s — %s", skip, x.ID, x.Evidence)
				}
				if x.ID == "game.mode" && skip == facts.StepGaming {
					t.Errorf("Game Mode reported off although it was not read")
				}
			}
			if skip == facts.StepGaming && !f.RequiresVBS() {
				t.Error("with the anti-cheats unknown, VBS must be treated as required")
			}
		})
	}
}

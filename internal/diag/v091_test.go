package diag_test

import (
	"testing"
	"time"

	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/sys"
)

// 0.9.1: what a healthy notebook must not be told is wrong.
func TestNotebookFalsePositives(t *testing.T) {
	base := func() *facts.Facts {
		return &facts.Facts{
			OS:       facts.OSInfo{Caption: "Microsoft Windows 11 Home", Build: 26100, IsWin11: true, SystemDrive: "C:"},
			CPU:      facts.CPUInfo{Name: "13th Gen Intel(R) Core(TM) i5-13450HX", Vendor: "Intel", Cores: 10, Threads: 16},
			Power:    facts.PowerInfo{IsLaptop: true, OnBattery: true, BatteryPercent: -1},
			RAM:      facts.RAMInfo{TotalBytes: 16 << 30, Type: "LPDDR5", Modules: []facts.MemModule{{CapacityBytes: 16 << 30, Locator: "Controller0-ChannelA"}}},
			Displays: []sys.DisplayMode{{Device: `\\.\DISPLAY1`, Monitor: "Tela do notebook", Width: 1920, Height: 1080, CurrentHz: 60, MaxHzAtRes: 144, MaxHzAny: 144, Internal: true}},
		}
	}
	f := base()
	r := diag.Build(f, nil)
	if x := find(r, "display.refresh."); x == nil || x.Severity != diag.SevInfo {
		t.Errorf("tela interna a 60 Hz na bateria deveria ser só sugestão: %+v", x)
	}
	if find(r, "ram.single") != nil {
		t.Error("memória soldada (LPDDR) não é \"um pente só\"")
	}
	if x := find(r, "power.battery"); x == nil || x.Evidence != "O notebook está na bateria." {
		t.Errorf("bateria sem porcentagem conhecida: %+v", x)
	}
	f.Power.OnBattery = false
	if x := find(diag.Build(f, nil), "display.refresh."); x == nil || x.Severity != diag.SevWarning {
		t.Errorf("tela interna a 60 Hz na tomada: aviso, não problema importante: %+v", x)
	}

	// The TPM query timed out: unknown, not "requirements not met".
	f = base()
	f.Gaming.AntiCheats = []string{"FACEIT"}
	f.Security = facts.SecurityInfo{TPMRead: false, SecureBoot: "unknown", VBSRunning: true}
	if x := find(diag.Build(f, nil), "sec.anticheat"); x != nil && x.Severity == diag.SevCritical {
		t.Errorf("TPM não lido virou requisito não atendido: %+v", x)
	}
	f.Security = facts.SecurityInfo{TPMRead: true, SecureBoot: "off", VBSRunning: true}
	if x := find(diag.Build(f, nil), "sec.anticheat"); x == nil || x.Severity != diag.SevCritical {
		t.Errorf("TPM ausente e Secure Boot desligado com FACEIT: %+v", x)
	}

	// Defender off because another antivirus is registered.
	f = base()
	f.Security.DefenderRealtime = "off"
	f.Health.Defender.ThirdParty = "Antivírus do fabricante"
	if find(diag.Build(f, nil), "sec.defender") != nil {
		t.Error("Defender desligado com outro antivírus não é problema")
	}
}

// A blue screen also writes a Kernel-Power 41 (with its stop code): that one
// is the blue screen, not an unexpected power loss.
func TestKP41AfterBlueScreen(t *testing.T) {
	now := time.Now()
	evs := []sys.Event{
		{Channel: "System", Provider: "Microsoft-Windows-Kernel-Power", ID: 41, Time: now.Add(-time.Hour), Data: map[string]string{"BugcheckCode": "278"}},
		{Channel: "System", Provider: "Microsoft-Windows-WER-SystemErrorReporting", ID: 1001, Time: now.Add(-time.Hour), Data: map[string]string{"param1": "0x00000116"}},
	}
	fs := diag.StabilityFor(evs, &facts.Facts{})
	for _, x := range fs {
		if x.ID == "stab.kp41" {
			t.Fatalf("desligamento contado em dobro depois da tela azul: %+v", x)
		}
	}
	evs = append(evs, sys.Event{Channel: "System", Provider: "Microsoft-Windows-Kernel-Power", ID: 41, Time: now, Data: map[string]string{"BugcheckCode": "0"}})
	found := false
	for _, x := range diag.StabilityFor(evs, &facts.Facts{Power: facts.PowerInfo{IsLaptop: true}}) {
		if x.ID == "stab.kp41" {
			found = true
			if x.Title != "1 desligamento inesperado" {
				t.Errorf("título: %q", x.Title)
			}
		}
	}
	if !found {
		t.Fatal("desligamento sem tela azul deveria aparecer")
	}
}

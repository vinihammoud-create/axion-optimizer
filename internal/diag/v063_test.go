package diag_test

import (
	"strings"
	"testing"
	"time"

	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/netdiag"
	"axion/internal/perf"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

func findExact(r *diag.Report, id string) *diag.Finding {
	for i := range r.Findings {
		if r.Findings[i].ID == id {
			return &r.Findings[i]
		}
	}
	return nil
}

func whFindingWith(t *testing.T, evs []sys.Event, f *facts.Facts, st map[string]tweaks.State) diag.Finding {
	t.Helper()
	for _, x := range diag.StabilityWith(evs, f, st) {
		if x.ID == "stab.whea" {
			return x
		}
	}
	t.Fatalf("sem achado WHEA para %d eventos", len(evs))
	return diag.Finding{}
}

// Review of 0.6.2 on the G15: 169 corrected PCIe errors, 154 at boot, no
// blue screen, no driver reset and no unexpected shutdown in the period.
// Corrected errors have no impact by themselves (PCIe AER): a note, not a
// warning, and the ASPM test is not offered when the plan already runs
// without PCIe power saving on AC.
func TestWHEASeverityByImpact(t *testing.T) {
	boot := time.Now().Add(-20 * 24 * time.Hour)
	gpu := map[string]string{"PrimaryDeviceName": `PCI\VEN_10DE&DEV_25EC&SUBSYS_0BF81028&REV_A1`, "ClassCode": "0x18000"}
	audio := map[string]string{"PrimaryDeviceName": `PCI\VEN_10DE&DEV_2291&SUBSYS_0BF81028&REV_A1`, "ClassCode": "0x20300"}
	evs := []sys.Event{{Provider: "Microsoft-Windows-Kernel-General", ID: 12, Level: 4, Time: boot}}
	for i := 0; i < 6; i++ {
		evs = append(evs, whea(17, 3, boot.Add(time.Duration(i)*20*time.Second), gpu))
	}
	// During use, spread over days: no burst.
	for i := 1; i <= 4; i++ {
		evs = append(evs, whea(17, 3, boot.Add(time.Duration(i)*26*time.Hour), gpu))
	}
	evs = append(evs, whea(17, 3, boot.Add(5*26*time.Hour), audio))
	f := base()
	f.Board.PCSystemType = 2
	f.PCIDevices = []facts.PCIDevice{
		{Name: "NVIDIA GeForce RTX 3050 6GB Laptop GPU", PNPID: `PCI\VEN_10DE&DEV_25EC&SUBSYS_0BF81028&REV_A1\4&2A788C0F&0&0008`},
		{Name: "NVIDIA High Definition Audio", PNPID: `PCI\VEN_10DE&DEV_2291&SUBSYS_0BF81028&REV_A1\4&2A788C0F&0&0108`},
	}
	x := whFinding(t, evs, f)
	if x.Severity != diag.SevInfo || !strings.Contains(x.Title, "sem sintoma") {
		t.Fatalf("só corrigidos, sem sintoma: %+v", x)
	}
	// The GPU's HDMI audio is named as Device Manager names it, never as a
	// "network card" from the class code of the record.
	for _, want := range []string{"NVIDIA High Definition Audio (VEN_10DE&DEV_2291): 1", "6 de 11 erros PCIe corrigidos", "Nenhum desligamento inesperado"} {
		if !strings.Contains(x.Evidence, want) {
			t.Errorf("evidência sem %q: %s", want, x.Evidence)
		}
	}
	if strings.Contains(x.Evidence, "placa de rede") {
		t.Errorf("áudio HDMI como placa de rede: %s", x.Evidence)
	}
	if len(x.Actions) != 1 || x.Actions[0].Tweaks[0] != "power.pcie.aspm" {
		t.Errorf("sem o estado do ASPM o teste é oferecido: %+v", x.Actions)
	}
	// ASPM already off on AC (the G15, not applied by Axion): no test to
	// offer, and the errors went on with it, so ASPM is ruled out.
	st := map[string]tweaks.State{"power.pcie.aspm": {ID: "power.pcie.aspm", Applicable: true, Status: "on"}}
	x = whFindingWith(t, evs, f, st)
	if len(x.Actions) != 0 || !strings.Contains(x.Explanation, "o ASPM não é a causa") {
		t.Errorf("ASPM já aplicado: %+v", x)
	}
	// Applied by Axion a minute ago, after the last error: nothing is ruled
	// out yet; the text only says what to watch for.
	justNow := time.Now().Add(-time.Minute)
	st["power.pcie.aspm"] = tweaks.State{ID: "power.pcie.aspm", Applicable: true, Status: "on", ByAxion: true, AppliedAt: &justNow}
	x = whFindingWith(t, evs, f, st)
	if len(x.Actions) != 0 || strings.Contains(x.Explanation, "e os erros continuam") || !strings.Contains(x.Explanation, "se os erros continuarem") {
		t.Errorf("ASPM aplicado há pouco: %+v", x)
	}
	// Applied by Axion a month ago, with errors logged since: ruled out.
	longAgo := boot.Add(-24 * time.Hour)
	st["power.pcie.aspm"] = tweaks.State{ID: "power.pcie.aspm", Applicable: true, Status: "on", ByAxion: true, AppliedAt: &longAgo}
	if x := whFindingWith(t, evs, f, st); !strings.Contains(x.Explanation, "o ASPM não é a causa") {
		t.Errorf("ASPM aplicado antes dos erros: %+v", x)
	}
	st["power.pcie.aspm"] = tweaks.State{ID: "power.pcie.aspm", Applicable: true, Status: "on"}
	// A blue screen in the same period: the same errors become a warning,
	// still without the ASPM action when the plan already runs without it.
	withBSOD := append(append([]sys.Event(nil), evs...), sys.Event{Provider: "Microsoft-Windows-WER-SystemErrorReporting", ID: 1001, Level: 2, Time: boot.Add(3 * 24 * time.Hour)})
	if x := whFindingWith(t, withBSOD, f, st); x.Severity != diag.SevWarning || !strings.Contains(x.Title, "com sintomas") || !strings.Contains(x.Evidence, "No mesmo período") || len(x.Actions) != 0 {
		t.Errorf("com tela azul no período: %+v", x)
	}
	if x := whFinding(t, withBSOD, f); len(x.Actions) != 1 {
		t.Errorf("com tela azul e sem o estado do ASPM o teste é oferecido: %+v", x.Actions)
	}
	// On a desktop the notebook sentence does not appear.
	desk := base()
	desk.PCIDevices = f.PCIDevices
	if x := whFinding(t, evs, desk); strings.Contains(x.Explanation, "notebook") {
		t.Errorf("frase de notebook no desktop: %s", x.Explanation)
	}
	// A burst: 10 corrected errors in 5 minutes with the PC in use.
	burst := append([]sys.Event(nil), evs...)
	for i := 0; i < 10; i++ {
		burst = append(burst, whea(17, 3, boot.Add(10*24*time.Hour+time.Duration(i)*20*time.Second), gpu))
	}
	if x := whFindingWith(t, burst, f, st); x.Severity != diag.SevWarning || !strings.Contains(x.Title, "em rajada") || !strings.Contains(x.Evidence, "Rajada") {
		t.Errorf("rajada: %+v", x)
	}
	// A corrected machine check during use stays a warning on its own.
	mce := append(append([]sys.Event(nil), evs...), whea(19, 3, boot.Add(2*24*time.Hour), map[string]string{"MciStat": "0x9c00"}))
	if x := whFindingWith(t, mce, f, st); x.Severity != diag.SevWarning || strings.Contains(x.Title, "rajada") || strings.Contains(x.Title, "sintomas") {
		t.Errorf("machine check corrigido durante o uso: %+v", x)
	}
	// The exported rows carry what the classification used.
	rows := diag.WHEARows(evs, f)
	if len(rows) != 11 || !rows[0].Time.After(rows[1].Time) || rows[0].Device != "VEN_10DE&DEV_2291" || rows[0].NearBoot || rows[len(rows)-1].NearBoot != true || rows[0].Fatal {
		t.Errorf("linhas exportadas: %d, primeira %+v, última %+v", len(rows), rows[0], rows[len(rows)-1])
	}
}

// Measurements temper and add findings: with the game limited by the GPU in
// most frames (the desktop report: 64,5%), the memory profile stops costing
// points; the last complete network test brings the queue under load and
// the route to Valve into the network area.
func TestMeasuredFindings(t *testing.T) {
	f := base()
	f.RAM = facts.RAMInfo{TotalBytes: 32 << 30, Type: "DDR5", ConfiguredMTs: 4800, RatedMTs: 4800,
		Modules: []facts.MemModule{{CapacityBytes: 16 << 30, Manufacturer: "Kingston", PartNumber: "KF556C40-16", ConfiguredMTs: 4800, SpeedMTs: 4800}}}
	if x := find(diag.Build(f, nil), "ram.xmp"); x == nil || x.Severity != diag.SevWarning {
		t.Fatalf("sem medição o XMP é aviso: %+v", x)
	}
	m := &diag.Measured{Perf: &perf.Summary{GPUBoundPct: 64.5, CPUBoundPct: 28.2, Limiter: "gpu", AvgFPS: 370.6, Low1FPS: 173.9}, PerfAt: time.Now().Add(-6 * 24 * time.Hour), PerfGame: "Counter-Strike 2"}
	rep := diag.BuildWith(f, nil, m)
	x := find(rep, "ram.xmp")
	if x == nil || x.Severity != diag.SevInfo || !strings.Contains(x.Explanation, "64,5%") || !strings.Contains(x.Explanation, "Counter-Strike 2") {
		t.Fatalf("XMP com o jogo limitado pela GPU: %+v", x)
	}
	// An old capture no longer speaks for the PC.
	m.PerfAt = time.Now().Add(-90 * 24 * time.Hour)
	if x := find(diag.BuildWith(f, nil, m), "ram.xmp"); x == nil || x.Severity != diag.SevWarning {
		t.Errorf("medição antiga: %+v", x)
	}
	// A CPU-bound capture changes nothing.
	m.PerfAt, m.Perf.GPUBoundPct, m.Perf.Limiter = time.Now(), 20, "cpu"
	if x := find(diag.BuildWith(f, nil, m), "ram.xmp"); x == nil || x.Severity != diag.SevWarning {
		t.Errorf("limitado pela CPU: %+v", x)
	}
	// Network: the desktop's test of 28/09 (grade C, +125 ms, all from the upload).
	net := &netdiag.Summary{At: time.Now().Add(-3 * 24 * time.Hour), Label: "QoS 700/100", Grade: "C", AddedMs: 125.22, DownAddedMs: 21.82, UpAddedMs: 125.22, DownMbps: 485.7, UpMbps: 24.7,
		ValvePOP: "São Paulo", ValveMs: 18.78, ValveJit: 6.44, ValveLoss: 0, HasLoadRun: true}
	rep = diag.BuildWith(f, nil, &diag.Measured{Net: net})
	b := find(rep, "net.bloat")
	if b == nil || b.Severity != diag.SevWarning || !strings.Contains(b.Title, "nota C") || !strings.Contains(b.Evidence, "vem do upload") || !strings.Contains(b.Evidence, "QoS 700/100") {
		t.Fatalf("bufferbloat: %+v", b)
	}
	v := find(rep, "net.valve")
	if v == nil || v.Severity != diag.SevInfo || !strings.Contains(v.Title, "jitter de 6,4 ms") || strings.Contains(v.Title, "perda") {
		t.Fatalf("rota Valve (só jitter): %+v", v)
	}
	net.ValveLoss = 2.5
	if v := find(diag.BuildWith(f, nil, &diag.Measured{Net: net}), "net.valve"); v == nil || v.Severity != diag.SevWarning || !strings.Contains(v.Title, "perda de 2,5%") {
		t.Fatalf("rota Valve com perda: %+v", v)
	}
	net.ValveLoss = 0
	for _, sc := range rep.Scores {
		if sc.Area == diag.AreaRede && sc.Score >= 90 {
			t.Errorf("a nota de rede deve refletir o teste: %+v", sc)
		}
	}
	// Grade A and a clean route: a good mark, nothing else.
	net.Grade, net.AddedMs, net.ValveJit = "A", 12, 2.1
	rep = diag.BuildWith(f, nil, &diag.Measured{Net: net})
	if b := find(rep, "net.bloat"); b == nil || b.Severity != diag.SevGood {
		t.Errorf("nota A: %+v", b)
	}
	if find(rep, "net.valve") != nil {
		t.Errorf("rota limpa não gera achado")
	}
	// An incomplete test (no grade) says nothing about the queue.
	net.Grade = ""
	if find(diag.BuildWith(f, nil, &diag.Measured{Net: net}), "net.bloat") != nil {
		t.Errorf("teste incompleto não tem nota")
	}
}

// Gamers Club (Anti-Cheat 2.0): Secure Boot required of everyone, TPM and
// VBS still in test; the FACEIT rule stays the FACEIT rule.
func TestGamersClubRule(t *testing.T) {
	f := base()
	f.Gaming.AntiCheats = []string{"Gamers Club"}
	f.Security = facts.SecurityInfo{SecureBoot: "on", TPMReady: true, TPMVersion: "2.0"}
	rep := diag.Build(f, nil)
	if x := find(rep, "sec.anticheat.gc"); x == nil || x.Severity != diag.SevGood {
		t.Fatalf("GC com Secure Boot: %+v", x)
	}
	if findExact(rep, "sec.anticheat") != nil || findExact(rep, "sec.anticheat.vbs") != nil {
		t.Errorf("a regra do FACEIT não vale para a GC: %+v", rep.Findings)
	}
	f.Security.SecureBoot = "off"
	rep = diag.Build(f, nil)
	if x := find(rep, "sec.anticheat.gc"); x == nil || x.Severity != diag.SevCritical {
		t.Fatalf("GC sem Secure Boot: %+v", x)
	}
	if find(rep, "sec.secureboot") != nil {
		t.Errorf("o aviso genérico de Secure Boot não se repete")
	}
	// VBS off is not a GC requirement yet: no VBS warning, and the VBS note
	// keeps its own wording.
	f.Security.SecureBoot, f.Security.VBSRunning = "on", false
	if x := find(diag.Build(f, nil), "sec.anticheat.vbs"); x != nil {
		t.Errorf("VBS não é exigência da GC: %+v", x)
	}
	// A legacy-BIOS install gets the MBR2GPT caution before "turn the CSM off".
	f.Security.SecureBoot, f.OS.Firmware = "unsupported", "BIOS"
	if x := find(diag.Build(f, nil), "sec.anticheat.gc"); x == nil || x.Severity != diag.SevCritical || !strings.Contains(x.Explanation, "MBR2GPT") {
		t.Errorf("GC em BIOS antigo: %+v", x)
	}
	// With FACEIT also installed the FACEIT rule owns Secure Boot; the GC
	// rule stays silent instead of repeating it.
	f.Gaming.AntiCheats = []string{"Gamers Club", "FACEIT"}
	f.OS.Firmware = "UEFI"
	rep = diag.Build(f, nil)
	if find(rep, "sec.anticheat.gc") != nil || findExact(rep, "sec.anticheat") == nil {
		t.Errorf("GC com FACEIT: %+v", rep.Findings)
	}
}

// The VBS note after a restart names the Device Guard scenarios still
// enabled, and only says "waits for a restart" when the effect is "reboot".
func TestVBSScenariosInNote(t *testing.T) {
	f := base()
	f.Security = facts.SecurityInfo{VBSRunning: true, Scenarios: []string{"SecureBiometrics"}}
	applied := time.Now().Add(-48 * time.Hour)
	st := map[string]tweaks.State{"adv.vbs.off": {ID: "adv.vbs.off", Applicable: true, Status: "on", Effect: "ignored", EffectNote: "x", ByAxion: true, AppliedAt: &applied}}
	x := find(diag.Build(f, st), "sec.vbs")
	if x == nil || !strings.Contains(x.Explanation, "Windows Hello (biometria)") || !strings.Contains(x.Explanation, "já reiniciou") || strings.Contains(x.Explanation, "depende de reiniciar") {
		t.Fatalf("cenário ligado, aplicado pelo Axion: %+v", x)
	}
	f.Security.Scenarios = nil
	x = find(diag.Build(f, st), "sec.vbs")
	if x == nil || !strings.Contains(x.Explanation, "Opções de entrada") || strings.Contains(x.Explanation, "depende de reiniciar") {
		t.Fatalf("sem cenário: %+v", x)
	}
	// Values written by someone else: nothing is known about a restart, so
	// the note asks for one before concluding Windows ignores them.
	st["adv.vbs.off"] = tweaks.State{ID: "adv.vbs.off", Applicable: true, Status: "on", Effect: "ignored"}
	x = find(diag.Build(f, st), "sec.vbs")
	if x == nil || strings.Contains(x.Explanation, "já reiniciou") || !strings.Contains(x.Explanation, "Reinicie o PC antes") || !strings.Contains(x.Explanation, "não foram gravados pelo Axion") {
		t.Fatalf("valores de outro programa: %+v", x)
	}
}

// Windows 11 26H2 (build 26300) has its dates; the next version does not yet.
func TestWin11End26H2(t *testing.T) {
	f := base()
	f.OS.IsWin11, f.OS.Build, f.OS.Caption, f.OS.Edition = true, 26300, "Microsoft Windows 11 Pro", "Professional"
	if x := find(diag.Build(f, nil), "os.win11.eos"); x != nil {
		t.Fatalf("26H2 em suporte: %+v", x)
	}
	for _, b := range []int{26220, 26350} { // Insider builds of 25H2 and 26H2: in support
		f.OS.Build = b
		if x := find(diag.Build(f, nil), "os.win11.eos"); x != nil {
			t.Errorf("build %d: %+v", b, x)
		}
	}
	// 24H2 Home/Pro (26100, and 26120, an Insider build of the same series):
	// support ends 13/10/2026, so the expectation depends on today's date.
	for _, b := range []int{26100, 26120} {
		f.OS.Build = b
		x := find(diag.Build(f, nil), "os.win11.eos")
		if time.Now().After(time.Date(2026, time.October, 13, 23, 59, 0, 0, time.UTC)) {
			if x == nil || !strings.Contains(x.Title, "24H2") {
				t.Fatalf("build %d, 24H2 fora do suporte: %+v", b, x)
			}
		} else if x != nil {
			t.Fatalf("build %d, 24H2 ainda em suporte: %+v", b, x)
		}
	}
}

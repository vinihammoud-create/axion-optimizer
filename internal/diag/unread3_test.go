package diag_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"axion/internal/collect"
	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/perf"
	"axion/internal/profile"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

// Phase 3 of the review: a reading that failed never becomes a green state,
// an alarm or a verdict.

var errSimulated = errors.New("falha simulada")

type failingSvcQuery struct {
	sys.Services
	name string
}

func (s failingSvcQuery) Query(name string) (sys.ServiceInfo, error) {
	if strings.EqualFold(name, s.name) {
		return sys.ServiceInfo{}, errSimulated
	}
	return s.Services.Query(name)
}

type failingSubKeys struct {
	sys.Registry
	part string
}

func (r failingSubKeys) SubKeys(path string) ([]string, error) {
	if strings.Contains(path, r.part) {
		return nil, errSimulated
	}
	return r.Registry.SubKeys(path)
}

type failingActiveScheme struct{ sys.Power }

func (failingActiveScheme) ActiveScheme() (string, error) { return "", errSimulated }

// dualX3DMock is the simulated PC with a Ryzen 9 7950X3D, its chipset service
// running, the Game Bar installed, Game Mode on and the Balanced plan.
func dualX3DMock(t *testing.T) *mocksys.Mock {
	t.Helper()
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m.W.Set(`root\cimv2`, "Win32_Processor", sys.WMIRow{"Name": "AMD Ryzen 9 7950X3D 16-Core Processor", "Manufacturer": "AuthenticAMD", "NumberOfCores": int64(16), "NumberOfLogicalProcessors": int64(32), "MaxClockSpeed": int64(4201), "VirtualizationFirmwareEnabled": false})
	m.S.Add("amd3dvcacheSvc", "AMD 3D V-Cache Performance Optimizer Service", sys.StartAuto, true)
	return m
}

// G-002: the chipset service, the Game Bar package list or the active plan
// failing to answer is not a critical problem, and the good finding does not
// say that what was not read is in place.
func TestX3DDualUnreadIsNotCritical(t *testing.T) {
	m := dualX3DMock(t)
	if x := find(diag.Build(collect.Collect(context.Background(), m.Platform, nil), nil), "cpu.x3d.dual"); x == nil || x.Severity != diag.SevGood || !strings.Contains(x.Title, "configurado corretamente") {
		t.Fatalf("tudo lido e certo: %+v", x)
	}
	cases := []struct {
		name    string
		patch   func(p *sys.Platform)
		claimed string // what the good finding must not claim
	}{
		{"serviço do chipset", func(p *sys.Platform) { p.Svc = failingSvcQuery{Services: m.S, name: "amd3dvcacheSvc"} }, "Driver de chipset"},
		{"pacotes da Game Bar", func(p *sys.Platform) { p.Reg = failingSubKeys{Registry: m.R, part: `AppModel\Repository\Packages`} }, "Game Bar e"},
		{"plano de energia ativo", func(p *sys.Platform) { p.Power = failingActiveScheme{Power: m.P} }, "Equilibrado"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := *m.Platform
			c.patch(&p)
			x := find(diag.Build(collect.Collect(context.Background(), &p, nil), nil), "cpu.x3d.dual")
			if x == nil {
				t.Fatal("sem o achado cpu.x3d.dual")
			}
			if x.Severity == diag.SevCritical {
				t.Fatalf("leitura que falhou virou problema importante: %s", x.Evidence)
			}
			if strings.Contains(x.Evidence, c.claimed) || strings.Contains(x.Title, "configurado corretamente") {
				t.Errorf("o achado afirma o que não foi lido: %s / %s", x.Title, x.Evidence)
			}
			if !strings.Contains(x.Evidence, "Não foi possível verificar") {
				t.Errorf("a evidência deveria dizer o que não foi lido: %s", x.Evidence)
			}
		})
	}
	// A real problem still counts next to what was not read.
	p := *m.Platform
	p.Power = failingActiveScheme{Power: m.P}
	_ = m.R.SetValue(`HKCU\Software\Microsoft\GameBar`, "AutoGameModeEnabled", sys.DWord(0))
	if x := find(diag.Build(collect.Collect(context.Background(), &p, nil), nil), "cpu.x3d.dual"); x == nil || x.Severity != diag.SevCritical || !strings.Contains(x.Evidence, "Modo de Jogo desligado") {
		t.Errorf("Modo de Jogo desligado continua crítico: %+v", x)
	}
}

// G-010 (server side): the Gamers Club finding says "not read" for a TPM or
// a VBS state that was not read, never "not found" or "off".
func TestGamersClubEvidenceSaysNotRead(t *testing.T) {
	f := base()
	f.Gaming.AntiCheats = []string{"Gamers Club"}
	f.Security = facts.SecurityInfo{SecureBoot: "on"}
	x := find(diag.Build(f, nil), "sec.anticheat.gc")
	if x == nil || x.Severity != diag.SevGood {
		t.Fatalf("Secure Boot ligado: %+v", x)
	}
	if strings.Contains(x.Evidence, "não encontrado") || strings.Contains(x.Evidence, "desligado") || !strings.Contains(x.Evidence, "TPM não lido") || !strings.Contains(x.Evidence, "VBS não lido") {
		t.Errorf("TPM e VBS não lidos: %s", x.Evidence)
	}
	f.Security = facts.SecurityInfo{SecureBoot: "on", TPMRead: true, DeviceGuardRead: true}
	if x := find(diag.Build(f, nil), "sec.anticheat.gc"); x == nil || !strings.Contains(x.Evidence, "TPM não encontrado") || !strings.Contains(x.Evidence, "VBS desligado") {
		t.Errorf("lidos e ausentes: %+v", x)
	}
}

// G-011: a failing read of the active hours is not "not set in the
// registry": no finding at all.
func TestActiveHoursUnreadIsSilent(t *testing.T) {
	if x := find(buildWithFailingRead(t, "NoSuchValue"), "os.activehours"); x == nil {
		t.Fatal("leitura normal deveria gerar os.activehours")
	}
	for _, name := range []string{"ActiveHoursStart", "ActiveHoursEnd", "SmartActiveHoursState", "SetActiveHours"} {
		if x := find(buildWithFailingRead(t, name), "os.activehours"); x != nil {
			t.Errorf("%s ilegível gerou os.activehours: %s — %s", name, x.Title, x.Evidence)
		}
	}
}

// A-006: without the boot and resume times, corrected PCIe errors away from
// the last boot are not "during use" (no burst, no warning) and the evidence
// says the times were not read.
func TestWHEABootsUnreadAreNotInUse(t *testing.T) {
	at := time.Now().Add(-26 * time.Hour)
	pcie := map[string]string{"ErrorSource": "4", "PrimaryDeviceName": `PCI\VEN_8086&DEV_A73E&SUBSYS_0BF81028&REV_01`, "ClassCode": "0x60400", "VendorId": "0x8086", "DeviceId": "0xa73e"}
	var evs []sys.Event
	for i := 0; i < 12; i++ {
		evs = append(evs, sys.Event{Channel: "System", Provider: "Microsoft-Windows-WHEA-Logger", ID: 17, Level: 3, Time: at.Add(time.Duration(i) * 10 * time.Second), Data: pcie})
	}
	full := whFindingWith(t, evs, &facts.Facts{}, nil)
	if full.Severity != diag.SevWarning || !strings.Contains(full.Evidence, "Rajada") {
		t.Fatalf("com os horários lidos (nenhum boot perto) é rajada em uso: %+v", full)
	}
	x := whFindingWith(t, evs, &facts.Facts{Unread: []string{diag.PartBoots}}, nil)
	if x.Severity != diag.SevInfo {
		t.Errorf("horários de boot não lidos: severidade %s", x.Severity)
	}
	if strings.Contains(x.Evidence, "Rajada") || strings.Contains(x.Evidence, "em uso.") || strings.Contains(x.Title, "rajada") || strings.Contains(x.Title, "só ao ligar") {
		t.Errorf("afirma quando os erros ocorreram: %s / %s", x.Title, x.Evidence)
	}
	if !strings.Contains(x.Evidence, "horários de inicialização e de retomada não puderam ser lidos") {
		t.Errorf("a evidência deveria dizer que os horários não foram lidos: %s", x.Evidence)
	}
}

// G-004: with too few classified frames (Limiter empty) the shares say
// nothing: the memory profile item is not tempered.
func TestMeasuredNeedsTheLimiterVerdict(t *testing.T) {
	f := base()
	f.RAM = facts.RAMInfo{TotalBytes: 32 << 30, Type: "DDR5", ConfiguredMTs: 4800, RatedMTs: 4800,
		Modules: []facts.MemModule{{CapacityBytes: 16 << 30, Manufacturer: "Kingston", PartNumber: "KF556C40-16", ConfiguredMTs: 4800, SpeedMTs: 4800}}}
	m := &diag.Measured{Perf: &perf.Summary{Frames: 1000, Limiter: "", GPUBoundPct: 100, ClassifiedFrac: 0.002}, PerfAt: time.Now(), PerfGame: "Counter-Strike 2"}
	if x := find(diag.BuildWith(f, nil, m), "ram.xmp"); x == nil || x.Severity != diag.SevWarning || strings.Contains(x.Explanation, "limitou") {
		t.Errorf("sem veredito do limitador a GPU não pode ter limitado: %+v", x)
	}
}

// cpuText: cores and threads not read are said so, not "0 threads".
func TestCPUTextNotRead(t *testing.T) {
	for _, x := range diag.Capacity(&facts.Facts{CPU: facts.CPUInfo{Name: "AMD Ryzen 5 5600"}}, "") {
		if x.ID == "cap.cpu" {
			if strings.Contains(x.Evidence, "0 threads") || !strings.Contains(x.Evidence, "não lidos") {
				t.Errorf("processador não lido: %q", x.Evidence)
			}
			return
		}
	}
	t.Fatal("sem cap.cpu")
}

// Item 14: with parts of the collection not read the report lists them,
// the grade is not "Excelente" and the summary says the analysis is
// incomplete instead of "nothing found"; also after Finalize re-scores.
func TestReportSaysAnalysisIncomplete(t *testing.T) {
	clean := func() *facts.Facts {
		f := base()
		f.CPU = facts.CPUInfo{Name: "AMD Ryzen 7 9800X3D 8-Core Processor", Vendor: "AMD", Cores: 8, Threads: 16, IsX3D: true}
		f.Disks = []facts.Disk{{Model: "Samsung SSD 990 PRO 2TB", MediaType: "SSD", BusType: "NVMe", Health: "Healthy", WearPct: 2, TempC: 41, IsSystem: true}}
		f.Volumes = []facts.Volume{{Letter: "C:", SizeBytes: 2000 << 30, FreeBytes: 1200 << 30}}
		f.Adapters = []facts.Adapter{{Description: "Intel(R) Ethernet Controller I226-V", Kind: "Ethernet", LinkSpeedMbps: 2500, Up: true, HasGateway: true}}
		return f
	}
	if r := diag.Build(clean(), nil); r.Grade != "Excelente" || len(r.Unread) != 0 || !strings.HasPrefix(r.Summary, "Nenhum problema importante") {
		t.Fatalf("tudo lido: %d %s %q %v", r.Overall, r.Grade, r.Summary, r.Unread)
	}
	f := clean()
	f.Unread = []string{facts.StepSecurity, diag.PartWHEA, facts.StepSecurity}
	for name, r := range map[string]*diag.Report{
		"Build":    diag.Build(f, nil),
		"Finalize": diag.Finalize(diag.Build(f, nil), profile.Work, nil),
	} {
		if len(r.Unread) != 2 || r.Unread[0] != facts.StepSecurity || r.Unread[1] != diag.PartWHEA {
			t.Errorf("%s: partes não lidas: %v", name, r.Unread)
		}
		if r.Grade == "Excelente" {
			t.Errorf("%s: análise incompleta com nota %d rotulada Excelente", name, r.Overall)
		}
		if strings.Contains(r.Summary, "Nenhum problema") || !strings.Contains(r.Summary, "Análise incompleta") || !strings.Contains(r.Summary, "2 partes da coleta não foram lidas") {
			t.Errorf("%s: resumo %q", name, r.Summary)
		}
	}
}

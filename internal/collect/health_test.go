package collect_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"axion/internal/collect"
	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

func collectHealth(t *testing.T) (*mocksys.Mock, *facts.Facts) {
	t.Helper()
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return m, collect.Collect(context.Background(), m.Platform, nil)
}

func TestHealthFromTheMock(t *testing.T) {
	_, f := collectHealth(t)
	h := f.Health
	d := h.Defender
	if !d.Read || !d.Enabled || !d.RealTime || !d.Tamper || d.Mode != "Normal" || d.SigAgeDays != 1 || d.SigUpdated.IsZero() || d.ThirdParty != "" {
		t.Errorf("Defender: %+v", d)
	}
	if !d.ExclusionsRead || len(d.ExclPaths) != 1 || d.ExclPaths[0] != `D:\Jogos\Steam` || len(d.ExclExts) != 0 {
		t.Errorf("exclusões: %+v", d)
	}
	// The recovery partition has no letter: only C: and D: are read.
	if !h.DirtyRead || len(h.Dirty) != 0 {
		t.Errorf("volumes: %+v", h)
	}
	u := h.Updates
	days := time.Since(u.Last).Hours() / 24
	if !u.Read || u.LastKB != "5121794" || days < 11.9 || days > 12.1 {
		t.Errorf("atualizações: %+v (%.1f dias)", u, days)
	}
	// The Defender definitions (KB2267602), 20 hours old, are not "the update".
	if u.FailKB != "5119001" || u.FailCode != "0x800f0820" || !u.Fail.Before(u.Last) {
		t.Errorf("falha antiga: %+v", u)
	}
	b := h.Boot
	if !b.Read || len(b.Samples) != 5 || b.Samples[0].MainMs != 14200 || b.Samples[0].TotalMs != 25200 || b.Samples[0].Degraded || len(b.Slow) != 0 {
		t.Errorf("boot: %+v", b)
	}
	if !b.Samples[0].At.After(b.Samples[4].At) {
		t.Errorf("os boots deveriam vir do mais novo para o mais antigo: %+v", b.Samples)
	}
	// A desktop never reads a battery.
	if h.Battery.Read {
		t.Errorf("bateria num desktop: %+v", h.Battery)
	}
	if !f.Read(facts.StepHealth) {
		t.Errorf("etapa não lida: %v", f.Unread)
	}
}

func TestHealthBatteryWearFromThePowerReport(t *testing.T) {
	m, _ := collectHealth(t)
	m.N.HasBattery = true
	m.N.BatteryDesignMWh, m.N.BatteryFullMWh, m.N.BatteryCycles = 54880, 42362, 32
	// A desktop with a USB UPS shows a battery too, and it is not a notebook
	// battery: the firmware's form factor decides.
	if f := collect.Collect(context.Background(), m.Platform, nil); f.Power.IsLaptop || f.Health.Battery.Read {
		t.Errorf("nobreak de um desktop lido como bateria: %+v", f.Health.Battery)
	}
	m.W.Set(`root\cimv2`, "Win32_ComputerSystem", sys.WMIRow{"Manufacturer": "Dell Inc.", "Model": "G15 5530", "PCSystemType": int64(2), "HypervisorPresent": true})
	f := collect.Collect(context.Background(), m.Platform, nil)
	b := f.Health.Battery
	if !b.Read || b.DesignMWh != 54880 || b.FullMWh != 42362 || b.Cycles != 32 || b.WearPct != 23 || b.Name != "MOCK BATTERY" {
		t.Errorf("bateria: %+v", b)
	}
	if _, err := os.Stat(filepath.Join(m.Platform.DataDir, "battery-report.xml")); !os.IsNotExist(err) {
		t.Errorf("o relatório temporário ficou no disco: %v", err)
	}
	// The report cannot be made: not read, not "no wear".
	m.N.BatteryDesignMWh = 0
	f = collect.Collect(context.Background(), m.Platform, nil)
	if f.Health.Battery.Read || f.Health.Battery.WearPct != 0 {
		t.Errorf("sem relatório: %+v", f.Health.Battery)
	}
}

// The report as Windows writes it (namespace, one battery, the system block
// around it), shortened.
const realBatteryReport = `<?xml version="1.0" encoding="utf-8"?>
<?xml-stylesheet type='text/xsl' href='C:\battery-stylesheet.xsl'?>
<BatteryReport xmlns="http://schemas.microsoft.com/battery/2012">
  <ReportInformation><ReportVersion>1.1</ReportVersion></ReportInformation>
  <SystemInformation><PlatformRole>Mobile</PlatformRole></SystemInformation>
  <Batteries>
    <Battery>
      <Id>DELL P8P1P3B</Id>
      <Manufacturer>SMP</Manufacturer>
      <SerialNumber>1025</SerialNumber>
      <ManufactureDate></ManufactureDate>
      <Chemistry>LiP</Chemistry>
      <LongTerm>1</LongTerm>
      <RelativeCapacity>0</RelativeCapacity>
      <DesignCapacity>54880</DesignCapacity>
      <FullChargeCapacity>42362</FullChargeCapacity>
      <CycleCount>32</CycleCount>
    </Battery>
  </Batteries>
  <RuntimeEstimates><DesignCapacity><Capacity>54880</Capacity></DesignCapacity></RuntimeEstimates>
</BatteryReport>`

func TestParseBatteryReport(t *testing.T) {
	b, ok := collect.ParseBatteryReport([]byte("\xef\xbb\xbf" + realBatteryReport)) // Windows writes a BOM
	if !ok || !b.Read || b.Name != "DELL P8P1P3B" || b.DesignMWh != 54880 || b.FullMWh != 42362 || b.Cycles != 32 || b.WearPct != 23 {
		t.Errorf("relatório real: %+v ok=%v", b, ok)
	}
	two := `<BatteryReport xmlns="http://schemas.microsoft.com/battery/2012"><Batteries>
	<Battery><Id>A</Id><DesignCapacity>40000</DesignCapacity><FullChargeCapacity>30000</FullChargeCapacity><CycleCount>10</CycleCount></Battery>
	<Battery><Id>B</Id><DesignCapacity>10000</DesignCapacity><FullChargeCapacity>10000</FullChargeCapacity><CycleCount>0</CycleCount></Battery></Batteries></BatteryReport>`
	if b, ok := collect.ParseBatteryReport([]byte(two)); !ok || b.DesignMWh != 50000 || b.FullMWh != 40000 || b.WearPct != 20 || b.Cycles != 10 || b.Name != "A + B" {
		t.Errorf("duas baterias: %+v", b)
	}
	// A battery that holds more than its label is not "negative wear".
	over := `<BatteryReport><Batteries><Battery><DesignCapacity>40000</DesignCapacity><FullChargeCapacity>41000</FullChargeCapacity></Battery></Batteries></BatteryReport>`
	if b, ok := collect.ParseBatteryReport([]byte(over)); !ok || b.WearPct != 0 || b.Cycles != -1 {
		t.Errorf("capacidade acima da de fábrica: %+v", b)
	}
	for name, doc := range map[string]string{
		"relativa":        `<BatteryReport><Batteries><Battery><RelativeCapacity>1</RelativeCapacity><DesignCapacity>100</DesignCapacity><FullChargeCapacity>80</FullChargeCapacity></Battery></Batteries></BatteryReport>`,
		"sem projeto":     `<BatteryReport><Batteries><Battery><DesignCapacity>0</DesignCapacity><FullChargeCapacity>80</FullChargeCapacity></Battery></Batteries></BatteryReport>`,
		"sem baterias":    `<BatteryReport><Batteries></Batteries></BatteryReport>`,
		"não é um XML":    `não é um relatório`,
		"carga vazia":     `<BatteryReport><Batteries><Battery><DesignCapacity>100</DesignCapacity><FullChargeCapacity>0</FullChargeCapacity></Battery></Batteries></BatteryReport>`,
		"documento vazio": ``,
	} {
		if b, ok := collect.ParseBatteryReport([]byte(doc)); ok || b.Read {
			t.Errorf("%s não diz nada sobre o desgaste: %+v", name, b)
		}
	}
}

func TestHealthDefenderDirtyAndOtherAntivirus(t *testing.T) {
	m, _ := collectHealth(t)
	m.W.Set(`root\Microsoft\Windows\Defender`, "MSFT_MpComputerStatus", sys.WMIRow{
		"AntivirusEnabled": true, "RealTimeProtectionEnabled": false, "IsTamperProtected": false,
		"AMRunningMode": "Passive Mode", "AntivirusSignatureAge": int64(9)})
	m.W.Set(`root\SecurityCenter2`, "AntiVirusProduct", sys.WMIRow{"displayName": "Windows Defender"}, sys.WMIRow{"displayName": "Avast Free Antivirus"})
	m.W.Set(`root\cimv2`, "Win32_Volume",
		sys.WMIRow{"DriveLetter": "c:", "DriveType": int64(3), "DirtyBitSet": true},
		sys.WMIRow{"DriveLetter": "E:", "DriveType": int64(5), "DirtyBitSet": true}, // optical drive: not a fixed volume
		sys.WMIRow{"DriveLetter": "D:", "DriveType": int64(3), "DirtyBitSet": true})
	d := collect.Collect(context.Background(), m.Platform, nil).Health
	if d.Defender.Mode != "Passive Mode" || d.Defender.SigAgeDays != 9 || d.Defender.RealTime || d.Defender.Tamper || d.Defender.ThirdParty != "Avast Free Antivirus" {
		t.Errorf("Defender: %+v", d.Defender)
	}
	if !d.DirtyRead || len(d.Dirty) != 2 || d.Dirty[0] != "C:" || d.Dirty[1] != "D:" {
		t.Errorf("volumes sujos: %+v", d)
	}
}

// Windows answers a non-administrator "N/A: Must be an administrator to view
// exclusions": that text is not an exclusion.
func TestHealthExclusionsHiddenFromNonAdmin(t *testing.T) {
	m, _ := collectHealth(t)
	const na = "N/A: Must be an administrator to view exclusions"
	m.W.Set(`root\Microsoft\Windows\Defender`, "MSFT_MpPreference", sys.WMIRow{
		"ExclusionPath": []any{na}, "ExclusionExtension": []any{na}, "ExclusionProcess": []any{na}})
	d := collect.Collect(context.Background(), m.Platform, nil).Health.Defender
	if d.ExclusionsRead || len(d.ExclPaths) != 0 || len(d.ExclExts) != 0 || len(d.ExclProcs) != 0 {
		t.Errorf("a mensagem do Windows virou exclusão: %+v", d)
	}
	// Defender not answering at all: nothing is read, nothing invented.
	m.W.Set(`root\Microsoft\Windows\Defender`, "MSFT_MpComputerStatus")
	m.W.Set(`root\Microsoft\Windows\Defender`, "MSFT_MpPreference")
	d = collect.Collect(context.Background(), m.Platform, nil).Health.Defender
	if d.Read || d.ExclusionsRead || d.SigAgeDays != -1 {
		t.Errorf("sem resposta do Defender: %+v", d)
	}
}

func TestHealthUpdatesFailingAndOnlyQualityUpdatesCount(t *testing.T) {
	m, _ := collectHealth(t)
	wu := func(id int, ago time.Duration, data map[string]string) sys.Event {
		return sys.Event{Channel: "System", Provider: "Microsoft-Windows-WindowsUpdateClient", ID: id, Level: 4, Time: time.Now().Add(-ago), Data: data}
	}
	m.E.Events = append(m.E.Events,
		wu(20, 24*time.Hour, map[string]string{"updateTitle": "2026-10 Atualização de Segurança (KB5129195) (26200.9457)", "errorCode": "0x80073701"}),
		wu(19, time.Hour, map[string]string{"updateTitle": "Ferramenta de Remoção de Software Mal-Intencionado do Windows x64 - KB890830"}),
		wu(19, time.Hour, map[string]string{"updateTitle": "9N0DX20HK701-Microsoft.WindowsTerminal"}),
		wu(19, time.Hour, map[string]string{"updateTitle": "Atualização da plataforma do Microsoft Defender Antivirus - KB4052623 (Versão 4.18.26080.4)"}),
		wu(20, time.Hour, map[string]string{"updateTitle": "Atualização de inteligência de segurança - KB2267602", "errorCode": "0x80070643"}))
	u := collect.Collect(context.Background(), m.Platform, nil).Health.Updates
	if u.LastKB != "5121794" {
		t.Errorf("a última atualização de qualidade deveria seguir a de 12 dias: %+v", u)
	}
	if u.FailKB != "5129195" || u.FailCode != "0x80073701" || !u.Fail.After(u.Last) {
		t.Errorf("a falha de ontem deveria ser a mais recente (a das definições não conta): %+v", u)
	}
}

func TestHealthBootSlowPrograms(t *testing.T) {
	m, _ := collectHealth(t)
	ev := func(name string, delay int, bootsAgo int) sys.Event {
		at := time.Now().Add(-time.Duration(bootsAgo)*26*time.Hour + 5*time.Minute)
		return sys.Event{Channel: "Microsoft-Windows-Diagnostics-Performance/Operational", Provider: "Microsoft-Windows-Diagnostics-Performance", ID: 101, Level: 3, Time: at,
			Data: map[string]string{"Name": name, "DegradationTime": fmt.Sprint(delay), "StartTime": fmt.Sprintf("boot-%d", bootsAgo)}}
	}
	for i := 1; i <= 4; i++ { // slow in 4 boots
		m.E.Events = append(m.E.Events, ev("OCControl.Service.exe", 5000+i*500, i))
	}
	for i := 1; i <= 3; i++ { // slow in 3 boots, less
		m.E.Events = append(m.E.Events, ev("Updater.exe", 5000, i))
	}
	for i := 1; i <= 2; i++ { // only 2 boots: not enough
		m.E.Events = append(m.E.Events, ev("Once.exe", 9000, i))
	}
	for i := 1; i <= 5; i++ { // every boot but under 5 s
		m.E.Events = append(m.E.Events, ev("Quick.exe", 4999, i))
	}
	// Two programs slowed the same boot: it counts once for each.
	m.E.Events = append(m.E.Events, ev("OCControl.Service.exe", 6000, 1))
	// A boot older than the five read.
	m.E.Events = append(m.E.Events, ev("Old.exe", 8000, 40), ev("Old.exe", 8000, 41), ev("Old.exe", 8000, 42))
	b := collect.Collect(context.Background(), m.Platform, nil).Health.Boot
	if len(b.Slow) != 2 {
		t.Fatalf("programas lentos: %+v", b.Slow)
	}
	if s := b.Slow[0]; s.Name != "OCControl.Service.exe" || s.Boots != 4 || s.WorstMs != 7000 {
		t.Errorf("primeiro: %+v", s)
	}
	if s := b.Slow[1]; s.Name != "Updater.exe" || s.Boots != 3 || s.WorstMs != 5000 {
		t.Errorf("segundo: %+v", s)
	}
}

func TestHealthBootDegradationAndGarbage(t *testing.T) {
	m, _ := collectHealth(t)
	m.E.Events = append(m.E.Events,
		sys.Event{Channel: "Microsoft-Windows-Diagnostics-Performance/Operational", Provider: "Microsoft-Windows-Diagnostics-Performance", ID: 100, Time: time.Now().Add(-time.Hour),
			Data: map[string]string{"BootTime": "90000", "MainPathBootTime": "61000", "BootIsDegradation": "true"}},
		// Records without usable numbers are skipped.
		sys.Event{Channel: "Microsoft-Windows-Diagnostics-Performance/Operational", Provider: "Microsoft-Windows-Diagnostics-Performance", ID: 100, Time: time.Now().Add(-30 * time.Minute),
			Data: map[string]string{"BootTime": "", "MainPathBootTime": "abc"}})
	b := collect.Collect(context.Background(), m.Platform, nil).Health.Boot
	if len(b.Samples) != 6 || b.Samples[0].MainMs != 61000 || !b.Samples[0].Degraded {
		t.Errorf("boots: %+v", b.Samples)
	}
}

// One group that fails does not take the others with it.
func TestHealthGroupsAreIndependent(t *testing.T) {
	m, _ := collectHealth(t)
	m.W.Set(`root\cimv2`, "Win32_Volume") // no volumes answer
	f := collect.Collect(context.Background(), m.Platform, nil)
	if f.Health.DirtyRead || !f.Health.Defender.Read || !f.Health.Updates.Read || !f.Health.Boot.Read {
		t.Errorf("grupos: %+v", f.Health)
	}
}

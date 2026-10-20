package mocksys

import (
	"fmt"
	"os"
	"time"

	"axion/internal/sys"
)

// seedHealth gives the simulated machine a healthy Windows: Defender up to
// date, no dirty volume, a quality update a few days old and quick boots.
// Tests change the rows and the events to make it sick.
func seedHealth(m *Mock) {
	w := m.W
	w.Set(`root\Microsoft\Windows\Defender`, "MSFT_MpComputerStatus", sys.WMIRow{
		"AntivirusEnabled": true, "RealTimeProtectionEnabled": true, "IsTamperProtected": true,
		"AMRunningMode": "Normal", "AntivirusSignatureAge": int64(1),
		"AntivirusSignatureLastUpdated": time.Now().Add(-20 * time.Hour)})
	w.Set(`root\Microsoft\Windows\Defender`, "MSFT_MpPreference", sys.WMIRow{
		"ExclusionPath": []any{`D:\Jogos\Steam`}, "ExclusionExtension": []any{}, "ExclusionProcess": []any{}})
	w.Set(`root\SecurityCenter2`, "AntiVirusProduct", sys.WMIRow{"displayName": "Windows Defender"})
	w.Set(`root\cimv2`, "Win32_Volume",
		sys.WMIRow{"DriveLetter": "C:", "DriveType": int64(3), "DirtyBitSet": false},
		sys.WMIRow{"DriveLetter": "D:", "DriveType": int64(3), "DirtyBitSet": false},
		sys.WMIRow{"DriveLetter": "", "DriveType": int64(3), "DirtyBitSet": false}) // the recovery partition has no letter

	now := time.Now()
	wu := func(id int, ago time.Duration, data map[string]string) sys.Event {
		return sys.Event{Channel: "System", Provider: "Microsoft-Windows-WindowsUpdateClient", ID: id, Level: 4, Time: now.Add(-ago), Data: data}
	}
	m.E.Events = append(m.E.Events,
		// Defender definitions arrive daily and are not "the Windows update".
		wu(19, 20*time.Hour, map[string]string{"updateTitle": "Atualização de inteligência de segurança para Microsoft Defender Antivirus - KB2267602 (versão 1.459.565.0)"}),
		wu(19, 12*24*time.Hour, map[string]string{"updateTitle": "2026-09 Atualização Cumulativa para Windows 11 (KB5121794) (26100.9539)"}),
		// A failure older than the success that followed it says nothing today.
		wu(20, 30*24*time.Hour, map[string]string{"updateTitle": "2026-08 Atualização Cumulativa para Windows 11 (KB5119001)", "errorCode": "0x800f0820"}))
	for i, ms := range []int{14200, 13100, 12200, 13800, 15100} {
		at := now.Add(-time.Duration(i+1) * 26 * time.Hour)
		m.E.Events = append(m.E.Events, sys.Event{Channel: diagPerfChannel, Provider: "Microsoft-Windows-Diagnostics-Performance", ID: 100, Level: 4, Time: at,
			Data: map[string]string{"BootTime": fmt.Sprint(ms + 11000), "MainPathBootTime": fmt.Sprint(ms), "BootIsDegradation": "false",
				"BootStartTime": at.UTC().Format(time.RFC3339Nano)}})
	}
}

const diagPerfChannel = "Microsoft-Windows-Diagnostics-Performance/Operational"

// batteryReport answers `powercfg /batteryreport /xml /output <file>` the way
// Windows does: the report of the simulated battery, or a failure without one.
func (m *Mock) batteryReport(out string) sys.Result {
	m.N.mu.Lock()
	has, design, full, cycles := m.N.HasBattery, m.N.BatteryDesignMWh, m.N.BatteryFullMWh, m.N.BatteryCycles
	m.N.mu.Unlock()
	if !has || design <= 0 || out == "" {
		return sys.Result{ExitCode: 1, Stdout: "Não foi possível gerar o relatório: nenhuma bateria encontrada."}
	}
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<BatteryReport xmlns="http://schemas.microsoft.com/battery/2012">
  <Batteries>
    <Battery>
      <Id>MOCK BATTERY</Id>
      <Manufacturer>Axion</Manufacturer>
      <Chemistry>LiP</Chemistry>
      <RelativeCapacity>0</RelativeCapacity>
      <DesignCapacity>%d</DesignCapacity>
      <FullChargeCapacity>%d</FullChargeCapacity>
      <CycleCount>%d</CycleCount>
    </Battery>
  </Batteries>
</BatteryReport>
`, design, full, cycles)
	if err := os.WriteFile(out, []byte(xml), 0o644); err != nil {
		return sys.Result{ExitCode: 1, Stdout: err.Error()}
	}
	return sys.Result{Stdout: "Relatório de duração da bateria salvo."}
}

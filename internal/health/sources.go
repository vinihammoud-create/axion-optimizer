package health

import "axion/internal/tweaks"

// The pages below were opened and read when the checks were written.
var (
	SrcDefenderUpdates   = tweaks.Source{Title: "Microsoft Learn — Atualizações de inteligência de segurança (KB2267602) e de plataforma (KB4052623) do Microsoft Defender Antivirus", URL: "https://learn.microsoft.com/defender-endpoint/microsoft-defender-antivirus-updates"}
	SrcTamper            = tweaks.Source{Title: "Microsoft Learn — Proteção contra violação (tamper protection): o que ela impede que mudem", URL: "https://learn.microsoft.com/defender-endpoint/tamper-protection-overview"}
	SrcExclusionMistakes = tweaks.Source{Title: "Microsoft Learn — Exclusões a evitar no Microsoft Defender Antivirus (pastas, tipos de arquivo e processos)", URL: "https://learn.microsoft.com/defender-endpoint/defender-endpoint-exclusions-common-mistakes"}
	SrcWUTroubleshoot    = tweaks.Source{Title: "Microsoft Learn — Solução de problemas do Windows Update (começa pela solução de problemas integrada)", URL: "https://learn.microsoft.com/troubleshoot/windows-client/installing-updates-features-roles/windows-update-issues-troubleshooting"}
	SrcFsutilDirty       = tweaks.Source{Title: "Microsoft Learn — fsutil dirty: o bit de volume sujo e a verificação de disco no próximo boot", URL: "https://learn.microsoft.com/windows-server/administration/windows-commands/fsutil-dirty"}
	SrcBatteryReport     = tweaks.Source{Title: "Microsoft Learn — powercfg /batteryreport (relatório de bateria, também em XML)", URL: "https://learn.microsoft.com/windows-hardware/design/device-experiences/powercfg-command-line-options"}
	SrcBootPerf          = tweaks.Source{Title: "Event Log Explorer — Desempenho de boot do Windows: os campos BootTime e MainPathBootTime do evento 100 (a Microsoft não os documenta)", URL: "https://eventlogxp.com/blog/windows-boot-performance-diagnostics-1/"}
)

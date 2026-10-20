package catalog

import (
	"strings"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

func advancedTweaks() []*tweaks.Tweak {
	return []*tweaks.Tweak{
		{
			ID: "adv.vbs.off", Category: "avancado", Title: "Integridade de Memória (HVCI) e VBS desligadas",
			Summary: "Desliga a virtualização de segurança do Windows. A Microsoft diz que, em alguns cenários e configurações, esses recursos podem afetar o desempenho em jogos.",
			Detail: "HKLM\\SYSTEM\\CurrentControlSet\\Control\\DeviceGuard\\Scenarios\\HypervisorEnforcedCodeIntegrity Enabled=0 e DeviceGuard EnableVirtualizationBasedSecurity=0. " +
				"A Microsoft recomenda religar ao terminar de jogar. Uma política de grupo, o bloqueio UEFI (lock) ou outro recurso que use o VBS fazem o Windows ignorar a mudança: depois de reiniciar, o Axion confere se o VBS parou de verdade e, se não parou, diz o motivo. " +
				"BLOQUEADO quando FACEIT ou Riot Vanguard estão instalados: o FACEIT exige VBS (para o IOMMU) de parte dos jogadores, em etapas, e pode pedir a Integridade de Memória; o Vanguard pode exigir a Integridade de Memória. " +
				"A partir de outubro de 2026 o Windows 11 passa a ligar a Integridade de Memória sozinho em PCs compatíveis, por atualização de qualidade; segundo a cobertura do anúncio, quem a desligou de propósito não é alterado. Se ela voltar a ligar, o Axion avisa que o ajuste foi desfeito.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcVMP, srcMemIntegrity, srcMemIntExpand, srcMemIntNews, srcFaceitReq, srcFaceitFAQ, srcFaceitKnown, srcVanguardReq},
			Risk: tweaks.RiskAdvanced, RiskNote: "reduz a proteção contra drivers maliciosos e exploits de kernel",
			Reboot: tweaks.RebootRestart, Tags: []string{"fps", "seguranca"}, OneClick: false,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Security.VBSRunning && !f.Security.HVCIRunning && !f.Security.VBSConfigured {
					return false, "VBS e Integridade de Memória já estão desligados"
				}
				return true, ""
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if f.RequiresVBS() {
					return tweaks.Rec(tweaks.RecBlocked, "você tem "+joinAC(f)+" instalado, que pode exigir VBS/Integridade de Memória: desligar pode impedir de jogar")
				}
				if f.HasAntiCheat("Gamers Club") {
					return tweaks.Rec(tweaks.RecNo, "a Gamers Club anunciou TPM 2.0 e VBS em teste no anti-cheat 2.0 (março de 2026): desligar pode impedir de jogar quando a exigência valer")
				}
				return tweaks.Rec(tweaks.RecNeutral, "pode ajudar em alguns jogos e configurações, com perda de segurança; a Microsoft sugere religar depois de jogar")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{
					tweaks.Reg(`HKLM\SYSTEM\CurrentControlSet\Control\DeviceGuard\Scenarios\HypervisorEnforcedCodeIntegrity`, "Enabled", sys.DWord(0)),
					tweaks.Reg(`HKLM\SYSTEM\CurrentControlSet\Control\DeviceGuard`, "EnableVirtualizationBasedSecurity", sys.DWord(0)),
				}
			},
			// The registry values only ask: Windows reports what is running.
			Effect: func(f *facts.Facts) (bool, string) {
				if !f.Read(facts.StepSecurity) {
					return true, ""
				}
				s := f.Security
				if !s.VBSRunning && !s.HVCIRunning {
					return true, ""
				}
				return false, "o VBS continua em execução: " + strings.Join(s.VBSHolders(), "; ")
			},
		},
		{
			ID: "adv.vmp.off", Category: "avancado", Title: "Virtual Machine Platform desligado",
			Summary:  "Desativa o componente de virtualização usado pelo WSL2 e pelo Android no Windows. A Microsoft diz que, em alguns cenários, ele pode afetar o desempenho em jogos, junto com a Integridade de Memória.",
			Detail:   "DISM /online /disable-feature /featurename:VirtualMachinePlatform. Não desligue se você usa WSL2, Windows Sandbox, Docker Desktop ou emuladores que dependem do Hyper-V. A Microsoft sugere religar depois de jogar se você usa esses recursos.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcVMP},
			Risk: tweaks.RiskModerate, Reboot: tweaks.RebootRestart, Tags: []string{"fps"}, OneClick: false,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Security.VMPEnabled {
					return false, "o Virtual Machine Platform não está ativado"
				}
				return true, ""
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if f.RequiresVBS() {
					return tweaks.Rec(tweaks.RecNeutral, "não afeta o anti-cheat, mas desligue só se não usa WSL2/emuladores")
				}
				return tweaks.Rec(tweaks.RecNeutral, "desligue se você não usa WSL2, Android no Windows ou Docker")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{featureOp("VirtualMachinePlatform", "Virtual Machine Platform")}
			},
		},
		{
			ID: "adv.timer.global", Category: "avancado", Title: "Resolução de timer global (Windows 11)",
			Summary: "Faz um pedido de timer de alta resolução (ex.: 0,5 ms da Sessão de Jogo) valer para todos os processos, como era antes do Windows 10 2004.",
			Detail: "GlobalTimerResolutionRequests=1 em HKLM\\SYSTEM\\CurrentControlSet\\Control\\Session Manager\\kernel (lido pelo kernel do Windows 11 e Server 2022+). " +
				"Sozinho não muda nada: só tem efeito quando algum processo pede resolução maior. As medições públicas de ganho em frametime/latência são inconsistentes; aumenta o consumo de energia.",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcTimer, srcTimerDawson},
			Risk: tweaks.RiskModerate, Reboot: tweaks.RebootRestart, Tags: []string{"latencia"}, OneClick: false, MinBuild: 22000,
			Recommend: neutral("experimental: compare no seu jogo (FPS médio e 1% low) antes de manter"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\kernel`, "GlobalTimerResolutionRequests", sys.DWord(1))}
			},
		},
		{
			ID: "adv.mmcss", Category: "avancado", Title: "MMCSS: menos reserva para tarefas de fundo",
			Summary: "Reduz de 20% para 10% a fatia de CPU que o agendador multimídia reserva para tarefas de baixa prioridade.",
			Detail: "SystemResponsiveness=10 em HKLM\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion\\Multimedia\\SystemProfile. " +
				"Só afeta threads registradas no MMCSS (áudio e poucos jogos). Valores abaixo de 10 (como 0, usado por muitos otimizadores) são convertidos para 20 pelo Windows, segundo a documentação. " +
				"O NetworkThrottlingIndex, que muitos otimizadores mudam junto, fica no padrão: a única medição encontrada viu a latência de DPC da rede piorar (veja \"O que não fazemos\"; se outro programa o desligou, a correção volta ao padrão).",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcMMCSS, srcDjdNetwork},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"latencia"},
			Recommend: neutral("efeito pequeno e restrito a threads multimídia"),
			Ops: func(*facts.Facts) []tweaks.Op {
				k := `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Multimedia\SystemProfile`
				return []tweaks.Op{tweaks.Reg(k, "SystemResponsiveness", sys.DWord(10))}
			},
		},
		{
			ID: "adv.spectre", Category: "avancado", Title: "Mitigações Spectre/Meltdown desligadas (CPUs Intel antigas)",
			Summary: "Remove as correções de segurança de execução especulativa, que custam mais desempenho em CPUs Intel de 4ª geração (Haswell) ou mais antigas.",
			Detail: "FeatureSettingsOverride=3 e FeatureSettingsOverrideMask=3 em HKLM\\SYSTEM\\CurrentControlSet\\Control\\Session Manager\\Memory Management (KB4073119). " +
				"A Microsoft mediu quedas de um dígito em CPUs Skylake (6ª geração) ou mais novas e quedas maiores em Haswell ou anteriores. Deixa o sistema vulnerável a ataques de canal lateral (inclusive via navegador).",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcSpectre, srcSpectrePerf},
			Risk: tweaks.RiskAdvanced, RiskNote: "vulnerabilidade de segurança real; só considere em PCs dedicados a jogo com CPUs antigas",
			Reboot: tweaks.RebootRestart, Tags: []string{"fps", "seguranca"}, OneClick: false, Group: "spectre",
			Applicable: func(f *facts.Facts) (bool, string) {
				if f.CPU.Vendor != "Intel" {
					return false, "sem ganho relevante fora de CPUs Intel antigas"
				}
				return true, ""
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if f.CPU.IntelGen > 0 && f.CPU.IntelGen <= 4 {
					return tweaks.Rec(tweaks.RecNeutral, "sua CPU Intel é de 4ª geração ou anterior, onde o custo das mitigações é maior")
				}
				return tweaks.Rec(tweaks.RecNo, "em CPUs Intel de 6ª geração ou mais novas a Microsoft mediu perdas de um dígito; o risco continua")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				k := `HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Memory Management`
				return []tweaks.Op{
					tweaks.Reg(k, "FeatureSettingsOverride", sys.DWord(3)),
					tweaks.Reg(k, "FeatureSettingsOverrideMask", sys.DWord(3)),
				}
			},
		},
	}
}

func joinAC(f *facts.Facts) string {
	var out []string
	for _, a := range f.Gaming.AntiCheats {
		if a == "FACEIT" || a == "Vanguard" {
			out = append(out, a)
		}
	}
	switch len(out) {
	case 0:
		return "um anti-cheat que exige VBS"
	case 1:
		return out[0]
	}
	return out[0] + " e " + out[1]
}

var (
	srcTimer       = tweaks.Source{Title: "Microsoft Learn — timeBeginPeriod (mudanças no Windows 10 2004 e Windows 11)", URL: "https://learn.microsoft.com/windows/win32/api/timeapi/nf-timeapi-timebeginperiod"}
	srcTimerDawson = tweaks.Source{Title: "Bruce Dawson — Windows Timer Resolution: The Great Rule Change", URL: "https://randomascii.wordpress.com/2020/10/04/windows-timer-resolution-the-great-rule-change/"}
)

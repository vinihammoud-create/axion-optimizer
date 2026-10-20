// Package catalog contains every optimization Axion offers, with its
// mechanism, evidence grade, sources and machine-specific recommendation rules.
//
// Rules of the catalog (enforced by tests):
//   - every tweak has at least one source, a detail text and an evidence grade;
//   - every operation is reversible from its captured snapshot;
//   - no two tweaks touch the same resource unless they share a Group
//     (mutually exclusive choices such as power plans).
package catalog

import (
	"strings"

	"axion/internal/facts"
	"axion/internal/tweaks"
)

// ---------------------------------------------------------------------------
// Sources (primary documentation first)
// ---------------------------------------------------------------------------

var (
	srcGameMode       = tweaks.Source{Title: "Xbox Support — Usar o Modo de Jogo no PC (impede instalação de drivers pelo Windows Update e avisos de reinício; taxa de quadros mais estável, conforme o jogo e o PC)", URL: "https://support.xbox.com/en-US/help/games-apps/game-setup-and-play/use-game-mode-gaming-on-pc"}
	srcGameOpt11      = tweaks.Source{Title: "Microsoft Support — Opções para otimizar o desempenho de jogos no Windows 11 (página em inglês)", URL: "https://support.microsoft.com/en-us/windows/options-to-optimize-gaming-performance-in-windows-11-a255f612-2949-4373-a566-ff6f3f474613"}
	srcWindowedOpt    = tweaks.Source{Title: "Microsoft Support — Otimizações para jogos em janela no Windows 11", URL: "https://support.microsoft.com/windows/optimizations-for-windowed-games-in-windows-11-3f006843-2c7e-4ed0-9a5e-f9389e535952"}
	srcFlipModel      = tweaks.Source{Title: "Microsoft Learn — Para melhor desempenho, use o flip model do DXGI", URL: "https://learn.microsoft.com/windows/win32/direct3ddxgi/for-best-performance--use-dxgi-flip-model"}
	srcHAGS           = tweaks.Source{Title: "DirectX Developer Blog — Hardware Accelerated GPU Scheduling", URL: "https://devblogs.microsoft.com/directx/hardware-accelerated-gpu-scheduling/"}
	srcNvidiaFG       = tweaks.Source{Title: "Nixxes Support — Como usar o DLSS Frame Generation (exige o agendamento de GPU acelerado por hardware)", URL: "https://support.nixxes.com/hc/en-us/articles/48432027293971-How-to-use-DLSS-Frame-Generation"}
	srcMMCSS          = tweaks.Source{Title: "Microsoft Learn — Multimedia Class Scheduler Service", URL: "https://learn.microsoft.com/windows/win32/procthread/multimedia-class-scheduler-service"}
	srcPowerOpts      = tweaks.Source{Title: "Microsoft Learn — Opções de gerenciamento de energia do processador", URL: "https://learn.microsoft.com/windows-hardware/customize/power-settings/configure-processor-power-management-options"}
	srcCoreParking    = tweaks.Source{Title: "Microsoft Learn — CPMinCores: mínimo de processadores lógicos fora do estado estacionado (100% desliga o core parking)", URL: "https://learn.microsoft.com/windows-hardware/customize/power-settings/options-for-core-parking-cpmincores"}
	srcUSBSuspend     = tweaks.Source{Title: "Microsoft Learn — USB selective suspend", URL: "https://learn.microsoft.com/windows-hardware/drivers/usbcon/usb-selective-suspend"}
	srcASPM           = tweaks.Source{Title: "Microsoft Learn — PCI Express: Link State Power Management", URL: "https://learn.microsoft.com/windows-hardware/customize/power-settings/pci-express-settings-link-state-power-management"}
	srcX3D            = tweaks.Source{Title: "TechPowerUp — Ryzen 9 7950X3D: requisitos de software (chipset, Game Bar, plano Equilibrado)", URL: "https://www.techpowerup.com/review/amd-ryzen-9-7950x3d/5.html"}
	srcX3DGuide       = tweaks.Source{Title: "VRChat Wiki — Guia X3D (amd3dvcacheSvc, Game Bar, plano Equilibrado)", URL: "https://wiki.vrchat.com/wiki/Guides:AMD_X3D_Series_Processors"}
	srcPowerThrottle  = tweaks.Source{Title: "Microsoft Learn — Power throttling / EcoQoS", URL: "https://learn.microsoft.com/windows/win32/procthread/quality-of-service"}
	srcHibernate      = tweaks.Source{Title: "Microsoft Learn — powercfg /hibernate", URL: "https://learn.microsoft.com/windows-hardware/design/device-experiences/powercfg-command-line-options"}
	srcMouseAccel     = tweaks.Source{Title: "Microsoft Learn — SystemParametersInfo SPI_SETMOUSE (limiares e aceleração)", URL: "https://learn.microsoft.com/windows/win32/api/winuser/nf-winuser-systemparametersinfow"}
	srcRawInput       = tweaks.Source{Title: "Microsoft Learn — Movimento de mouse de alta resolução: o Windows aplica aceleração (balística) ao ponteiro, o WM_INPUT (raw input) não", URL: "https://learn.microsoft.com/windows/win32/dxtecharts/taking-advantage-of-high-dpi-mouse-movement"}
	srcStickyKeys     = tweaks.Source{Title: "Microsoft Learn — STICKYKEYS/FILTERKEYS/TOGGLEKEYS (flag HOTKEYACTIVE)", URL: "https://learn.microsoft.com/windows/win32/api/winuser/ns-winuser-stickykeys"}
	srcNicPower       = tweaks.Source{Title: "Microsoft Learn — Set-NetAdapterPowerManagement", URL: "https://learn.microsoft.com/powershell/module/netadapter/set-netadapterpowermanagement"}
	srcNdisKeywords   = tweaks.Source{Title: "Microsoft Learn — Palavras-chave INF padronizadas de gerenciamento de energia (*EEE)", URL: "https://learn.microsoft.com/windows-hardware/drivers/network/standardized-inf-keywords-for-power-management"}
	srcIntMod         = tweaks.Source{Title: "Microsoft Learn — Interrupt Moderation", URL: "https://learn.microsoft.com/windows-hardware/drivers/network/interrupt-moderation"}
	srcNagle          = tweaks.Source{Title: "Microsoft Learn — TcpAckFrequency e comportamento de ACK atrasado", URL: "https://learn.microsoft.com/troubleshoot/windows-server/networking/registry-entry-control-tcp-acknowledgment-behavior"}
	srcTCPSettings    = tweaks.Source{Title: "Microsoft Learn — Set-NetTCPSetting (autotuning, ECN, timestamps)", URL: "https://learn.microsoft.com/powershell/module/nettcpip/set-nettcpsetting"}
	srcRSS            = tweaks.Source{Title: "Microsoft Learn — Receive Side Scaling", URL: "https://learn.microsoft.com/windows-hardware/drivers/network/introduction-to-receive-side-scaling"}
	srcMPO            = tweaks.Source{Title: "NVIDIA Support 5157 — Apps piscam/travam ao redimensionar (desativar MPO)", URL: "https://nvidia.custhelp.com/app/answers/detail/a_id/5157"}
	srcAdsSettings    = tweaks.Source{Title: "Microsoft Support — Configurações gerais de privacidade (ID de anúncio, conteúdo sugerido em Configurações, acompanhamento de apps iniciados)", URL: "https://support.microsoft.com/windows/general-privacy-settings-in-windows-7c7f6a09-cebd-5589-c376-7f505e5bf65a"}
	srcSearchPolicy   = tweaks.Source{Title: "Microsoft Learn — Policy CSP ADMX_WindowsExplorer: DisableSearchBoxSuggestions (documentada para a caixa de pesquisa do Explorador de Arquivos)", URL: "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-admx-windowsexplorer"}
	srcWidgets        = tweaks.Source{Title: "Microsoft Learn — Policy CSP: NewsAndInterests/AllowNewsAndInterests", URL: "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-newsandinterests"}
	srcTelemetry      = tweaks.Source{Title: "Microsoft Learn — Configurar dados de diagnóstico do Windows (AllowTelemetry)", URL: "https://learn.microsoft.com/windows/privacy/configure-windows-diagnostic-data-in-your-organization"}
	srcTelemetryCSP   = tweaks.Source{Title: "Microsoft Learn — Policy CSP System/AllowTelemetry: o valor 0 fora do Enterprise/Education equivale a 1", URL: "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-system"}
	srcActivity       = tweaks.Source{Title: "Microsoft Learn — Policy CSP: Privacy/PublishUserActivities", URL: "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-privacy"}
	srcWUDrivers      = tweaks.Source{Title: "Microsoft Learn — Policy CSP: Update/ExcludeWUDriversInQualityUpdate", URL: "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-update"}
	srcDO             = tweaks.Source{Title: "Microsoft Learn — Referência do Delivery Optimization (DownloadMode)", URL: "https://learn.microsoft.com/windows/deployment/do/waas-delivery-optimization-reference"}
	srcMenuDelay      = tweaks.Source{Title: "Microsoft Learn — SPI_SETMENUSHOWDELAY", URL: "https://learn.microsoft.com/windows/win32/api/winuser/nf-winuser-systemparametersinfow"}
	srcFileExt        = tweaks.Source{Title: "Microsoft Support — Extensões de nome de arquivo comuns no Windows", URL: "https://support.microsoft.com/windows/common-file-name-extensions-in-windows-da4a4430-8e76-89c5-59f7-1cdbbc75cb01"}
	srcMemIntegrity   = tweaks.Source{Title: "Microsoft Learn — Habilitar a Integridade de Memória (HVCI)", URL: "https://learn.microsoft.com/windows/security/hardware-security/enable-virtualization-based-protection-of-code-integrity"}
	srcMemIntExpand   = tweaks.Source{Title: "Microsoft Windows IT Pro Blog — Expanding memory integrity protection across Windows devices (31/08/2026)", URL: "https://techcommunity.microsoft.com/blog/windows-itpro-blog/expanding-memory-integrity-protection-across-windows-devices/4551984"}
	srcMemIntNews     = tweaks.Source{Title: "Tom's Hardware — Integridade de Memória ligada automaticamente a partir de outubro (escolhas anteriores mantidas)", URL: "https://www.tomshardware.com/software/windows/microsoft-will-expand-windows-11-memory-integrity-feature-to-more-pcs-starting-in-october-security-feature-reduces-gaming-performance-on-some-systems"}
	srcFaceitReq      = tweaks.Source{Title: "FACEIT — TPM 2.0, Secure Boot, IOMMU e VBS", URL: "https://www.faceit.com/en/news/faceit-rollout-of-tpm-secure-boot-iommu-and-vbs"}
	srcFaceitFAQ      = tweaks.Source{Title: "FACEIT — FAQ de requisitos de segurança do Windows", URL: "https://support.faceit.com/hc/en-us/articles/23117181142556-Windows-Security-Requirements-FAQ"}
	srcVanguardReq    = tweaks.Source{Title: "Riot Games — Requisitos de segurança do Vanguard", URL: "https://support.riotgames.com/en-us/riot/performance/vanguard-security-requirements"}
	srcQPC            = tweaks.Source{Title: "Microsoft Learn — Obter carimbos de data/hora de alta resolução (TSC x HPET)", URL: "https://learn.microsoft.com/windows/win32/sysinfo/acquiring-high-resolution-time-stamps"}
	srcPrioSepDoc     = tweaks.Source{Title: "Microsoft Learn — Win32PrioritySeparation (padrão 0x2; \"Serviços em segundo plano\" grava 0x18: primeiro plano e segundo plano com o mesmo tempo)", URL: "https://learn.microsoft.com/previous-versions/cc976120(v=technet.10)"}
	srcBCDEdit        = tweaks.Source{Title: "Microsoft Learn — BCDEdit /set", URL: "https://learn.microsoft.com/windows-hardware/drivers/devtest/bcdedit--set"}
	srcSpectre        = tweaks.Source{Title: "Microsoft — KB4073119: mitigações de canal lateral (FeatureSettingsOverride)", URL: "https://support.microsoft.com/topic/windows-client-guidance-for-it-pros-to-protect-against-silicon-based-microarchitectural-and-speculative-execution-side-channel-vulnerabilities-35820a8a-ae13-1299-88cc-357f104f5b11"}
	srcVMP            = tweaks.Source{Title: "Microsoft Support — Integridade de Memória e Plataforma de Máquina Virtual em jogos (\"em alguns cenários e configurações\"; página em inglês)", URL: "https://support.microsoft.com/en-us/windows/options-to-optimize-gaming-performance-in-windows-11-a255f612-2949-4373-a566-ff6f3f474613"}
	srcSchedTasks     = tweaks.Source{Title: "Microsoft Learn — Disable-ScheduledTask", URL: "https://learn.microsoft.com/powershell/module/scheduledtasks/disable-scheduledtask"}
	srcServices       = tweaks.Source{Title: "Microsoft Learn — Diretrizes para desabilitar serviços do sistema (Windows Server; usada só para a descrição de cada serviço)", URL: "https://learn.microsoft.com/windows-server/security/windows-services/security-guidelines-for-disabling-system-services-in-windows-server"}
	srcTaskbarEndTask = tweaks.Source{Title: "Microsoft — Windows 11: Finalizar tarefa na barra de tarefas (Configurações > Sistema > Avançado; antes em Para desenvolvedores)", URL: "https://learn.microsoft.com/windows/advanced-settings/"}
	srcSpeedDuplex    = tweaks.Source{Title: "Microsoft Learn — Palavras-chave de enumeração: *SpeedDuplex (0 = negociação automática, o padrão)", URL: "https://learn.microsoft.com/windows-hardware/drivers/network/enumeration-keywords"}
	srcAutoneg        = tweaks.Source{Title: "Wikipedia — Autonegociação (obrigatória no 1000BASE-T)", URL: "https://en.wikipedia.org/wiki/Autonegotiation"}
	srcNicReset       = tweaks.Source{Title: "Microsoft Learn — Reset-NetAdapterAdvancedProperty (volta as propriedades avançadas ao padrão)", URL: "https://learn.microsoft.com/powershell/module/netadapter/reset-netadapteradvancedproperty"}
	srcNicParams      = tweaks.Source{Title: "Microsoft Learn — Parâmetros da página Avançado (Ndi\\Params: o valor padrão de cada opção)", URL: "https://learn.microsoft.com/windows-hardware/drivers/network/specifying-configuration-parameters-for-the-advanced-properties-page"}
	srcIntelWifiAdv   = tweaks.Source{Title: "Intel — Configurações avançadas dos adaptadores Wi-Fi (artigo 000005585)", URL: "https://www.intel.com/content/www/us/en/support/articles/000005585/wireless/legacy-intel-wireless-products.html"}
	srcIntModTuning   = tweaks.Source{Title: "Microsoft Learn — Ajuste de desempenho de adaptadores de rede (moderação de interrupção: menos CPU x mais latência; 64 KB de janela com o autoajuste desligado)", URL: "https://learn.microsoft.com/windows-server/networking/technologies/network-subsystem/net-sub-performance-tuning-nics"}
	srcDjdNetwork     = tweaks.Source{Title: "djdallmann/GamingPCSetup — pesquisa de rede (NetworkThrottlingIndex desligado aumenta a latência de DPC do NDIS)", URL: "https://github.com/djdallmann/GamingPCSetup/blob/master/CONTENT/RESEARCH/NETWORK/README.md"}
	srcPowerStates    = tweaks.Source{Title: "Microsoft Learn — Estados de energia do sistema (hibernação, arquivo hiberfil e Inicialização Rápida)", URL: "https://learn.microsoft.com/windows/win32/power/system-power-states"}
	srcPowerSlider    = tweaks.Source{Title: "Microsoft Learn — Personalizar o controle de modo de energia (sobreposições: Melhor desempenho ded574b5...)", URL: "https://learn.microsoft.com/windows-hardware/customize/desktop/customize-power-slider"}
	srcFaceitKnown    = tweaks.Source{Title: "FACEIT — Problemas conhecidos dos requisitos do anti-cheat (Integridade de Memória pode ser exigida de alguns jogadores)", URL: "https://support.faceit.com/hc/en-us/articles/22851956652956-Known-issues-with-Anti-Cheat-Requirements"}
	srcActivityPriv   = tweaks.Source{Title: "Microsoft Support — Histórico de atividades do Windows e sua privacidade (guardado no PC; envio à Microsoft descontinuado no Windows 11)", URL: "https://support.microsoft.com/en-us/windows/privacy/windows-activity-history-and-your-privacy"}
	srcSpectrePerf    = tweaks.Source{Title: "Microsoft Security Blog (09/01/2018) — Impacto das mitigações de Spectre/Meltdown: queda maior em CPUs Haswell (4ª geração) ou mais antigas", URL: "https://www.microsoft.com/en-us/security/blog/2018/01/09/understanding-the-performance-impact-of-spectre-and-meltdown-mitigations-on-windows-systems/"}
	srcHIDSuspend     = tweaks.Source{Title: "Microsoft Learn — Selective suspend for HID over USB devices (desligado por padrão para evitar atraso ao retomar)", URL: "https://learn.microsoft.com/windows-hardware/drivers/hid/selective-suspend-for-hid-over-usb-devices"}
	srcFilterKeys     = tweaks.Source{Title: "Microsoft Learn — FILTERKEYS (FKF_FILTERKEYSON, tempo de aceitação e repetição)", URL: "https://learn.microsoft.com/windows/win32/api/winuser/ns-winuser-filterkeys"}
	srcInputQueue     = tweaks.Source{Title: "Microsoft KB Q102989 — KeyboardDataQueueSize / MouseDataQueueSize: eventos guardados pelo driver (padrão 100)", URL: "https://jeffpar.github.io/kbarchive/kb/102/Q102989/"}
	srcInputQueueTest = tweaks.Source{Title: "djdallmann/GamingPCSetup — pesquisa sobre MouseDataQueueSize (buffer em memória não paginada)", URL: "https://github.com/djdallmann/GamingPCSetup/blob/master/CONTENT/RESEARCH/WINDRIVERS/README.md"}
	srcThreadedDPC    = tweaks.Source{Title: "Microsoft Learn — Introduction to Threaded DPCs (ThreadDpcEnable)", URL: "https://learn.microsoft.com/windows-hardware/drivers/kernel/introduction-to-threaded-dpcs"}
	srcGameDVRBG      = tweaks.Source{Title: "Xbox Support — Ajustar as configurações de captura (a gravação em segundo plano pode afetar o desempenho)", URL: "https://support.xbox.com/en-US/help/games-apps/game-setup-and-play/adjust-capture-settings-windows-10"}
	srcMSI            = tweaks.Source{Title: "Microsoft Learn — Introdução às interrupções sinalizadas por mensagem (MSI/MSI-X)", URL: "https://learn.microsoft.com/windows-hardware/drivers/kernel/introduction-to-message-signaled-interrupts"}
)

// ---------------------------------------------------------------------------
// Machine predicates
// ---------------------------------------------------------------------------

func win11(f *facts.Facts) bool { return f.OS.IsWin11 || f.OS.Build >= 22000 }

func hasGPU(f *facts.Facts, vendor string) bool { return f.HasGPUVendor(vendor) }

func nvidiaFGCapable(f *facts.Facts) bool {
	for _, g := range f.GPUs {
		n := strings.ToUpper(g.Name)
		if g.Vendor == "NVIDIA" && (strings.Contains(n, "RTX 40") || strings.Contains(n, "RTX 50") || strings.Contains(n, "RTX 60")) {
			return true
		}
	}
	return false
}

func discreteGPU(f *facts.Facts) bool {
	for _, g := range f.GPUs {
		if !g.Integrated && (g.Vendor == "NVIDIA" || g.Vendor == "AMD" || g.Vendor == "Intel") {
			return true
		}
	}
	return false
}

func ethernetActive(f *facts.Facts) bool {
	a := f.ActiveAdapter()
	return a != nil && a.Kind == "Ethernet"
}

func always(*facts.Facts) (bool, string) { return true, "" }

func yes(reason string) func(*facts.Facts) tweaks.Recommendation {
	return func(*facts.Facts) tweaks.Recommendation { return tweaks.Rec(tweaks.RecYes, reason) }
}

func neutral(reason string) func(*facts.Facts) tweaks.Recommendation {
	return func(*facts.Facts) tweaks.Recommendation { return tweaks.Rec(tweaks.RecNeutral, reason) }
}

// All returns the full catalog in display order.
func All() []*tweaks.Tweak {
	var out []*tweaks.Tweak
	out = append(out, fixTweaks()...)
	out = append(out, nvdrsFixTweaks()...)
	out = append(out, gameTweaks()...)
	out = append(out, powerTweaks()...)
	out = append(out, inputTweaks()...)
	out = append(out, networkTweaks()...)
	out = append(out, gpuTweaks()...)
	out = append(out, privacyTweaks()...)
	out = append(out, updateTweaks()...)
	out = append(out, uiTweaks()...)
	out = append(out, serviceTweaks()...)
	out = append(out, advancedTweaks()...)
	return out
}

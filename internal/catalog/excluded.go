package catalog

import "axion/internal/tweaks"

var (
	srcRegCleaners   = tweaks.Source{Title: "Microsoft Support — Política de suporte para utilitários de limpeza de Registro (\"a Microsoft não oferece suporte ao uso de limpadores de Registro\")", URL: "https://support.microsoft.com/en-us/topic/microsoft-support-policy-for-the-use-of-registry-cleaning-utilities-0485f4df-9520-3691-2461-7b0fd54e8b3a"}
	srcQoSMyth       = tweaks.Source{Title: "Microsoft KB316666 (arquivo) — \"essas afirmações estão incorretas\": o QoS não reserva 20% da banda para si", URL: "https://ftp.zx.net.nz/pub/archive/ftp.microsoft.com/MISC/KB/en-us/316/666.HTM"}
	srcEdgeWV2       = tweaks.Source{Title: "Microsoft Learn — Distribuição do WebView2 (o Runtime é separado do navegador Edge)", URL: "https://learn.microsoft.com/microsoft-edge/webview2/concepts/distribution"}
	srcEdgeBoost     = tweaks.Source{Title: "Microsoft Support — Aumento de desempenho na inicialização do Edge (mantém processos mínimos com o navegador fechado)", URL: "https://support.microsoft.com/en-us/topic/get-help-with-startup-boost-ebef73ed-5c72-462f-8726-512782c5e442"}
	srcNVMeHot       = tweaks.Source{Title: "HotHardware (24/03/2026) — A Microsoft bloqueia o ajuste do NVMe nativo nas builds Insider (e pode bloquear nas estáveis)", URL: "https://hothardware.com/news/microsoft-kills-nvme-registry-trick-but-theres-a-workaround"}
	srcVisualFXQ     = tweaks.Source{Title: "Microsoft Q&A (arquivo MSDN/TechNet) — VisualFXSetting sozinho não aplica o conjunto de efeitos", URL: "https://learn.microsoft.com/en-us/archive/msdn-technet-forums/39d6e2c3-0c5a-4fe1-98af-9a881d619187"}
	srcPrioSep       = tweaks.Source{Title: "Blur Busters — tabela de valores do Win32PrioritySeparation (comunidade)", URL: "https://forums.blurbusters.com/viewtopic.php?t=8535"}
	srcMaxFrameLat   = tweaks.Source{Title: "Microsoft Learn — IDXGIDevice1::SetMaximumFrameLatency (definido pelo jogo, não pelo Registro)", URL: "https://learn.microsoft.com/windows/win32/api/dxgi/nf-dxgi-idxgidevice1-setmaximumframelatency"}
	srcRwDrv         = tweaks.Source{Title: "LOLDrivers — rwdrv.sys (RWEverything): driver vulnerável, acesso a memória física, MSR e PCI", URL: "https://www.loldrivers.io/drivers/fbdd993b-47b1-4448-8c41-24c310802398/"}
	srcIMODGuide     = tweaks.Source{Title: "PC-Optimization-Hub — guia de IMOD do xHCI (exige RWEverything)", URL: "https://github.com/BoringBoredom/PC-Optimization-Hub/blob/main/content/xhci%20imod/xhci%20imod.md"}
	srcHidusbfRM     = tweaks.Source{Title: "hidusbf (README) — versões que alteram o polling exigem desligar a Integridade de Memória", URL: "https://github.com/LordOfMice/hidusbf/blob/master/README.md"}
	srcPrioSepMS     = tweaks.Source{Title: "Microsoft Learn — Win32PrioritySeparation (campos de bits AABBCC; padrão 0x2)", URL: "https://learn.microsoft.com/previous-versions/cc976120(v=technet.10)"}
	srcPagingExec    = tweaks.Source{Title: "Microsoft Learn — DisablePagingExecutive (padrão 0; 1 é útil para depurar drivers)", URL: "https://learn.microsoft.com/previous-versions/windows/it-pro/windows-server-2003/cc757875(v=ws.10)"}
	srcKbdDelay      = tweaks.Source{Title: "Microsoft Learn — SystemParametersInfo: SPI_SETKEYBOARDDELAY (0 ≈ 250 ms … 3 ≈ 1 s) e SPI_SETKEYBOARDSPEED", URL: "https://learn.microsoft.com/windows/win32/api/winuser/nf-winuser-systemparametersinfow"}
	srcWERDoc        = tweaks.Source{Title: "Microsoft Learn — Windows Error Reporting", URL: "https://learn.microsoft.com/windows/win32/wer/windows-error-reporting"}
	srcFreeSpace     = tweaks.Source{Title: "Microsoft Support — Liberar espaço em disco (Limpeza do Windows Update pelo Sensor de Armazenamento)", URL: "https://support.microsoft.com/windows/free-up-drive-space-in-windows-85529ccb-c365-490d-b548-831022bc9b32"}
	srcShaderBlog    = tweaks.Source{Title: "Microsoft DirectX Blog — Advanced Shader Delivery: compilar shaders de novo causa carregamento longo e travadas", URL: "https://devblogs.microsoft.com/directx/introducing-advanced-shader-delivery/"}
	srcChromeCache   = tweaks.Source{Title: "Google (Ajuda da Conta) — Limpar cache e cookies: \"alguns sites podem parecer mais lentos\"", URL: "https://support.google.com/accounts/answer/32050"}
	srcGitVenom      = tweaks.Source{Title: "Kaspersky Securelist (24/02/2025) — GitVenom: projetos falsos no GitHub com README gerado por IA e commits automáticos", URL: "https://securelist.com/gitvenom-campaign/115694/"}
	srcTrendFakeGH   = tweaks.Source{Title: "Trend Micro (11/03/2025) — Repositórios falsos no GitHub (cheats, cracks) distribuem SmartLoader e Lumma Stealer", URL: "https://www.trendmicro.com/en_us/research/25/c/ai-assisted-fake-github-repositories.html"}
	srcMSGitHubMal   = tweaks.Source{Title: "Microsoft Security Blog (06/03/2025) — Malvertising leva a ladrões de dados hospedados no GitHub", URL: "https://www.microsoft.com/en-us/security/blog/2025/03/06/malvertising-campaign-leads-to-info-stealers-hosted-on-github/"}
	srcSDRValve      = tweaks.Source{Title: "Valve — Steam Datagram Relay: o tráfego passa pelos relays e pela rede da Valve; os IPs nunca são revelados", URL: "https://partner.steamgames.com/doc/features/multiplayer/steamdatagramrelay"}
	srcCS2Interp     = tweaks.Source{Title: "Valve — Notas do CS2 de 7/2/2024: \"cl_interp can no longer be changed directly\" (use o buffer contra perda de pacotes)", URL: "https://store.steampowered.com/news/posts/?feed=steam_community_announcements&appids=730&enddate=1708400000"}
	srcRFC1034       = tweaks.Source{Title: "RFC 1034 — Nomes de domínio: o DNS traduz nomes em endereços", URL: "https://www.rfc-editor.org/rfc/rfc1034"}
	srcPacketCoal    = tweaks.Source{Title: "Microsoft Learn — Overview of Packet Coalescing: só pacotes de broadcast e multicast são agrupados, para reduzir interrupções e consumo", URL: "https://learn.microsoft.com/en-us/windows-hardware/drivers/network/overview-of-packet-coalescing"}
	srcIRQPriority   = tweaks.Source{Title: "Microsoft Learn — IRQ_PRIORITY (wdm.h): Undefined, Low, Normal, High, nessa ordem", URL: "https://learn.microsoft.com/windows-hardware/drivers/ddi/wdm/ne-wdm-_irq_priority"}
	srcCS2SnapTap    = tweaks.Source{Title: "Dust2.us (19/08/2024) — Atualização do CS2: null binds, Snap Tap e jump binds passam a ser ignorados ou punidos; contra-strafe assistido por hardware é detectado nos servidores oficiais", URL: "https://www.dust2.us/news/51933/cs2-8-19-update-null-binds-snap-tap-and-jump-binds-banned-in-latest-update"}
	srcAmiFlDrv      = tweaks.Source{Title: "LOLDrivers — amifldrv64.sys: driver vulnerável (verificado), listado para obter privilégio de kernel", URL: "https://www.loldrivers.io/drivers/34fa6ba4-dc7c-4fd6-b947-8a0bb8ebd031/"}
	srcSuspendThread = tweaks.Source{Title: "Microsoft Learn — SuspendThread: feita principalmente para depuradores; suspender uma thread que segura um mutex ou seção crítica pode causar deadlock", URL: "https://learn.microsoft.com/windows/win32/api/processthreadsapi/nf-processthreadsapi-suspendthread"}
	srcCoreTopology  = tweaks.Source{Title: "Microsoft Learn — PROCESSOR_RELATIONSHIP: cada núcleo informa se tem mais de um processador lógico (LTP_PC_SMT) e sua classe de eficiência", URL: "https://learn.microsoft.com/windows/win32/api/winnt/ns-winnt-processor_relationship"}
	srcI913900K      = tweaks.Source{Title: "Intel — Core i9-13900K: 24 núcleos (8 P-cores e 16 E-cores) e 32 threads", URL: "https://www.intel.com/content/www/us/en/products/sku/230496/intel-core-i913900k-processor-36m-cache-up-to-5-80-ghz/specifications.html"}
	srcIoPrioHint    = tweaks.Source{Title: "Microsoft Learn — IO_PRIORITY_HINT (wdm.h): IoPriorityHigh e IoPriorityCritical são reservados para uso do sistema", URL: "https://learn.microsoft.com/windows-hardware/drivers/ddi/wdm/ne-wdm-_io_priority_hint"}
	srcIoHintInfo    = tweaks.Source{Title: "Microsoft Learn — FILE_IO_PRIORITY_HINT_INFO: se a prioridade é honrada depende do driver", URL: "https://learn.microsoft.com/windows/win32/api/winbase/ns-winbase-file_io_priority_hint_info"}
	srcD3DKMTPrio    = tweaks.Source{Title: "Microsoft Learn — D3DKMTSetProcessSchedulingPriorityClass (interface de driver; a página só descreve o mecanismo)", URL: "https://learn.microsoft.com/windows-hardware/drivers/ddi/d3dkmthk/nf-d3dkmthk-d3dkmtsetprocessschedulingpriorityclass"}
	srcGPUPrio       = tweaks.Source{Title: "Microsoft Learn — IDXGIDevice::SetGPUThreadPriority: usada de forma inadequada pode prejudicar a velocidade de renderização; meça o impacto", URL: "https://learn.microsoft.com/windows/win32/api/dxgi/nf-dxgi-idxgidevice-setgputhreadpriority"}
	srcStandbyMem    = tweaks.Source{Title: "Microsoft Learn — Memory Footprint: a memória em espera é cache de dados e arquivos e já conta como disponível", URL: "https://learn.microsoft.com/windows-hardware/test/assessments/results-for-the-memory-footprint-assessment"}
	srcWSTrimBench   = tweaks.Source{Title: "Pavise (desenvolvedor) — teste informal: reler 256 MiB depois de \"limpar\", 0,92 ms → 50,74 ms (65 mil page faults)", URL: "https://github.com/dulaiduwang003/Pavise-Game/blob/pavise2x/tools/OptimizationRiskBench/RESULTS.md"}
	// NVIDIA driver profiles (shared with catalog/nvdrs.go).
	srcNvSettingsH    = tweaks.Source{Title: "NVIDIA — NVAPI SDK, NvApiDriverSettings.h: IDs e valores padrão dos ajustes do driver (V-Sync padrão = usar a configuração do aplicativo; cache de shader padrão = ligado; modo de energia padrão = ideal)", URL: "https://github.com/NVIDIA/nvapi/blob/main/NvApiDriverSettings.h"}
	srcFPSReviewPower = tweaks.Source{Title: "TheFPSReview (04/12/2019) — Modos de energia do driver NVIDIA comparados (RTX 2080 SUPER e 2060 SUPER): experiência igual nos três modos; \"Preferir desempenho máximo\" eleva o consumo em repouso", URL: "https://www.thefpsreview.com/2019/12/04/nvidia-geforce-driver-power-mode-settings-compared/11/"}
)

// Excluded lists what other optimizers do that Axion deliberately does not,
// with the reason. It is shown in the app ("O que não fazemos e por quê") and
// exported in the evidence matrix.
func Excluded() []tweaks.Excluded {
	return []tweaks.Excluded{
		{Name: "Win32PrioritySeparation = 0x26 (38)", SeenIn: []string{"optimizerDuck", "Adaptive PC Optimizer", "Turbo Gamer V13"},
			Mechanism: `HKLM\SYSTEM\CurrentControlSet\Control\PriorityControl`, Verdict: "sem efeito",
			Reason: "0x26 = quantum curto, variável e boost 3:1 do primeiro plano, que é exatamente o comportamento do valor padrão (2) no Windows cliente. Já valores em que o primeiro plano perde a vantagem, com os bits do meio em 10 (intervalo fixo, como o 0x18 da opção \"Serviços em segundo plano\" e o 0x1A de alguns otimizadores) ou os bits finais em 00, o Axion detecta e oferece voltar ao padrão.", Sources: []tweaks.Source{srcPrioSepMS, srcPrioSep}},
		{Name: "Win32PrioritySeparation = 0x1A (26)", SeenIn: []string{"otimizadores comerciais (nível equilibrado)"},
			Mechanism: `HKLM\SYSTEM\CurrentControlSet\Control\PriorityControl`, Verdict: "pode piorar",
			Reason: "0x1A = 01 10 10: intervalo longo e FIXO. Pela documentação da Microsoft, com intervalo fixo o primeiro e o segundo plano recebem o mesmo tempo, e a razão 3:1 dos dois bits finais só vale com intervalo variável. Ou seja, o valor tira a vantagem do primeiro plano em vez de dar prioridade ao jogo. O Axion detecta e oferece voltar ao padrão (Correções).", Sources: []tweaks.Source{srcPrioSepMS, srcPrioSep}},
		{Name: "Win32PrioritySeparation = 0x16 (22)", SeenIn: []string{"Tutu's Optimizer"},
			Mechanism: `HKLM\SYSTEM\CurrentControlSet\Control\PriorityControl`, Verdict: "sem evidência",
			Reason: "0x16 = quantum longo, variável e boost 3:1. O primeiro plano já recebe o boost 3:1 no padrão; a única mudança é o tamanho da fatia de tempo, que só pesa quando threads de mesma prioridade disputam o mesmo núcleo. Não há medição pública de ganho em jogos, e o valor não dá \"prioridade máxima\" ao jogo.", Sources: []tweaks.Source{srcPrioSepMS, srcPrioSep}},
		{Name: "SystemResponsiveness = 0", SeenIn: []string{"Pc-Optimizer (Ping_Boost)", "pc-optimizer (AlkesDEV)", "PrecisionGamingOptimizer", "otimizadores comerciais"},
			Mechanism: `Multimedia\SystemProfile`, Verdict: "sem efeito",
			Reason: "A documentação da Microsoft diz que valores abaixo de 10 são convertidos para 20 (o padrão). O Axion oferece 10, o mínimo real, como opção avançada.", Sources: []tweaks.Source{srcMMCSS}},
		{Name: "MMCSS \"GPU Priority\" = 8 e \"SFIO Priority\" = High", Targets: []string{"|gpu priority", "|sfio priority"}, SeenIn: []string{"optimizerDuck", "Adaptive PC Optimizer", "Ping_Boost", "otimizadores comerciais"},
			Mechanism: `Multimedia\SystemProfile\Tasks\Games`, Verdict: "sem efeito",
			Reason: "A documentação do MMCSS descreve os dois valores como \"não usados\". Além disso, só afetam threads que se registram no MMCSS.", Sources: []tweaks.Source{srcMMCSS}},
		{Name: "MaxFrameLatency no Registro (Direct3D)", Targets: []string{"|maxframelatency"}, SeenIn: []string{"Adaptive PC Optimizer"},
			Mechanism: `HKCU/HKLM\Software\Microsoft\Direct3D\MaxFrameLatency`, Verdict: "sem evidência",
			Reason: "A fila de quadros é definida pelo próprio jogo via API (SetMaximumFrameLatency) ou pelo driver (Reflex/Anti-Lag/Modo de baixa latência). Não há documentação de que o DirectX leia esse valor de Registro, nem medição de efeito.", Sources: []tweaks.Source{srcMaxFrameLat}},
		{Name: "DirectXShaderCacheSize / \"cache de shader de 10 GB\" via Registro", Targets: []string{"|directxshadercachesize", "|maxshadercachesizeinmb", "|shadercachesize"}, SeenIn: []string{"Adaptive PC Optimizer"},
			Mechanism: `HKCU\Software\Microsoft\DirectX\UserGpuPreferences`, Verdict: "sem efeito",
			Reason: "Não é um valor documentado. O tamanho do cache de shaders é controlado pelo driver (ex.: Painel NVIDIA > Tamanho do cache de shader)."},
		{Name: "LargeSystemCache = 0, MouseDataQueueSize = 100, KeyboardDataQueueSize = 100", SeenIn: []string{"Adaptive PC Optimizer"},
			Mechanism: "Memory Management / mouclass / kbdclass", Verdict: "sem efeito",
			Reason: "São os valores padrão do Windows. Gravá-los não muda nada."},
		{Name: "Diminuir MouseDataQueueSize / KeyboardDataQueueSize (16, 20, 50…)", SeenIn: []string{"guias de \"input lag\"", "vários .reg"},
			Mechanism: `HKLM\SYSTEM\CurrentControlSet\Services\mouclass|kbdclass\Parameters`, Verdict: "mito / pode piorar",
			Reason: "É o tamanho do buffer onde o driver guarda eventos até o sistema lê-los, não um atraso: os eventos saem assim que são lidos. Um buffer menor só aumenta a chance de descartar cliques e movimentos quando o sistema atrasa. O Axion detecta e oferece voltar ao padrão.", Sources: []tweaks.Source{srcInputQueue, srcInputQueueTest}},
		{Name: "ThreadDpcEnable = 0 (\"desligar DPCs em thread\")", SeenIn: []string{"guias de latência"},
			Mechanism: `HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\kernel`, Verdict: "sem evidência / pode piorar",
			Reason: "Pela documentação da Microsoft, o trabalho que os drivers marcaram para rodar em thread passa a rodar como DPC comum, que interrompe todas as threads até terminar. Não há medição pública de ganho. O Axion detecta e oferece voltar ao padrão.", Sources: []tweaks.Source{srcThreadedDPC}},
		{Name: "Desligar a moderação de interrupções do USB (IMOD do xHCI) com RWEverything", SeenIn: []string{"PC-Optimization-Hub (guia)", "guias de \"input lag\""},
			Mechanism: "escrita direta em registradores MMIO do controlador via RwDrv.sys", Verdict: "risco alto",
			Reason: "Exige carregar o RwDrv.sys, listado como driver vulnerável (lê e escreve memória física, MSR e PCI) e incompatível com a Integridade de Memória; anti-cheats bloqueiam ou sinalizam drivers desse tipo. A mudança some a cada reinício, um valor errado pode travar o controlador e não há medição pública consistente de ganho.", Sources: []tweaks.Source{srcRwDrv, srcIMODGuide}},
		{Name: "\"Overclock\" do polling por driver de filtro (hidusbf)", SeenIn: []string{"guias de mouse/teclado"},
			Mechanism: "driver de filtro que altera o bInterval informado pelo dispositivo", Verdict: "incompatível com anti-cheat",
			Reason: "As versões que alteram o intervalo exigem desligar a Integridade de Memória (HVCI), que o FACEIT pode exigir ligada de alguns jogadores e o Vanguard pode pedir. Além disso, forçam o dispositivo a um ritmo diferente do que ele declarou; em dispositivos Low-Speed o próprio autor avisa que pode não funcionar. Para mais Hz, use um periférico que ofereça a taxa oficialmente.", Sources: []tweaks.Source{srcHidusbfRM, srcFaceitKnown, srcVanguardReq}},
		{Name: "Desligar a economia de energia de todos os dispositivos do PC (varredura do registro Enum)", SeenIn: []string{"Atlas (DisablePowerSaving)", "scripts .ps1 de \"latência\""},
			Mechanism: "AllowIdleIrpInD3, EnhancedPowerManagementEnabled, SelectiveSuspendEnabled… em todo HKLM\\SYSTEM\\CurrentControlSet\\Enum + MSPower_DeviceEnable em todos", Verdict: "amplo demais",
			Reason: "Altera centenas de dispositivos (Wi-Fi, Bluetooth, armazenamento, hubs), costuma vir junto com a desativação de dispositivos ACPI de gerenciamento e do EcoQoS, e não guarda o estado exato para desfazer. O Axion só age em mouse e teclado, só onde a suspensão está ligada, com reversão exata.", Sources: []tweaks.Source{srcHIDSuspend}},
		{Name: "\"Sincronizar\" o PC com o tick do servidor (PLL / \"anti-lag elétrico\")", SeenIn: []string{"electricity-anti-lag-ultimate (Hermes Engine)"},
			Mechanism: "processo externo com timers e afinidade própria", Verdict: "sem base técnica",
			Reason: "O ritmo de rede e a interpolação são tratados pelo próprio jogo (no CS2, pelo sistema de sub-tick); um processo externo não enxerga nem ajusta esse relógio. A ferramenta analisada ainda altera o mínimo de núcleos ativos do plano de energia (CPMINCORES) sem restaurar o valor original."},
		{Name: "\"Canais de IRQ\" / IRQ8Priority e prioridades de IRQ no Registro", Targets: []string{"|irq8priority", "|irq16priority"}, SeenIn: []string{"GameBoost-Pro (descrição)", "guias antigos"},
			Mechanism: `HKLM\SYSTEM\CurrentControlSet\Control\PriorityControl → IRQ8Priority, IRQ16Priority…`, Verdict: "mito",
			Reason: "Valores da era Windows 9x que o kernel NT não lê. Interrupções modernas usam APIC e MSI, e a afinidade é definida pela política do driver."},
		{Name: "netsh winsock reset / netsh int ip reset como \"limpeza\"", SeenIn: []string{"PCVitalBoost", "Tutu's Optimizer"},
			Mechanism: "netsh", Verdict: "prejudicial fora de reparo",
			Reason: "São comandos de reparo: apagam a configuração da pilha de rede (IP fixo, provedores de camada, VPNs) e exigem reiniciar. Não reduzem ping; use apenas para consertar uma conexão quebrada."},
		{Name: "Limites do msconfig (Número de processadores / Memória máxima) para \"acelerar o boot\"", SeenIn: []string{"guias antigos de boot"},
			Mechanism: "bcdedit numproc / truncatememory / removememory", Verdict: "sem efeito ou prejudicial",
			Reason: "O Windows já usa todos os núcleos e toda a RAM. As opções só servem para testes e, abaixo do hardware real, tiram núcleos ou memória do sistema. O Axion detecta e oferece remover.", Sources: []tweaks.Source{srcBCDEdit}},
		{Name: "Prioridade alta para csrss.exe", Targets: []string{`image file execution options\csrss.exe`}, SeenIn: []string{"Adaptive PC Optimizer"},
			Mechanism: "IFEO csrss.exe PerfOptions", Verdict: "risco sem ganho",
			Reason: "O csrss é um processo protegido e crítico. Não há evidência de ganho, e mexer nele pode causar instabilidade."},
		{Name: "Perfis de jogo em HKCU\\...\\Image File Execution Options", Targets: []string{`reg:hkcu\software\microsoft\windows nt\currentversion\image file execution options`}, SeenIn: []string{"PrecisionGamingOptimizer"},
			Mechanism: `HKCU\Software\Microsoft\Windows NT\CurrentVersion\Image File Execution Options`, Verdict: "sem efeito",
			Reason: "O Windows só lê IFEO em HKLM. O Axion grava a prioridade por jogo no local correto.", Sources: []tweaks.Source{srcIFEO}},
		{Name: "NonBestEffortLimit = 0 (\"liberar os 20% de banda do QoS\")", Targets: []string{"|nonbesteffortlimit"}, SeenIn: []string{"PrecisionGamingOptimizer", "Pc-Optimizer", "Ping_Boost", "Tutu's Optimizer"},
			Mechanism: `HKLM\SOFTWARE\Policies\Microsoft\Windows\Psched`, Verdict: "mito",
			Reason: "Mito da época do Windows XP, desmentido pela própria Microsoft: essa reserva só vale para aplicativos que pedem QoS explicitamente. Sem eles, 100% da banda já fica disponível.", Sources: []tweaks.Source{srcQoSMyth}},
		{Name: "Pacotes de \"Ping Boost\" (TcpNumConnections, MaxUserPort, IRPStackSize, SizReqBuf, DefaultTTL, GlobalMaxTcpWindowSize…)", Targets: []string{"|tcpnumconnections", "|maxuserport", "|irpstacksize", "|sizreqbuf", "|defaultttl", "|globalmaxtcpwindowsize"}, SeenIn: []string{"Pc-Optimizer (Ping_Boost.bat)"},
			Mechanism: `HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters e LanmanServer`, Verdict: "sem efeito / obsoleto",
			Reason: "São parâmetros de servidor da era XP. O Windows moderno ignora a maioria ou os ajusta sozinho, e nenhum reduz a latência até o servidor do jogo."},
		{Name: "netsh: autotuninglevel=disabled, chimney, netdma, dca, congestionprovider=ctcp", SeenIn: []string{"Ping_Boost", "Turbo Gamer V13", "SG TCP Optimizer (perfis antigos)"},
			Mechanism: "netsh int tcp set global", Verdict: "prejudicial (autotuning) / obsoleto",
			Reason: "Desligar o autotuning limita a janela TCP a 64 KB e deixa downloads lentos, sem baixar o ping. Chimney, NetDMA e DCA foram removidos nas versões atuais do Windows. O CTCP é um controle de congestionamento que pode ajudar a vazão em conexões de alta latência, mas não muda o ping; o padrão atual do Windows já é adequado. O Axion oferece a correção do autotuning para voltar ao padrão.", Sources: []tweaks.Source{srcTCPSettings, srcIntModTuning}},
		{Name: "MTU fixo (1452/1472) no adaptador", SeenIn: []string{"Ping_Boost"},
			Mechanism: "netsh interface ipv4 set subinterface mtu=", Verdict: "sem efeito no ping",
			Reason: "Pacotes de jogo têm poucas centenas de bytes, muito abaixo de qualquer MTU, então o valor não muda o ping. Um MTU menor só aumenta um pouco a sobrecarga em downloads; problemas (fragmentação, sites que não carregam) surgem quando o MTU fica maior que o do caminho. O Axion só mede o MTU do caminho, como diagnóstico."},
		{Name: "QoS/DSCP por jogo na internet", SeenIn: []string{"otimizadores comerciais", "LigeirinhoTW (anúncio)"},
			Mechanism: "Policy-based QoS (DSCP 46)", Verdict: "sem efeito na maioria das redes",
			Reason: "Provedores zeram ou ignoram a marcação DSCP. Só teria efeito dentro de uma rede local com roteador configurado para priorizar, o que é raro em casa."},
		{Name: "\"Liberar RAM\" esvaziando a memória dos processos (EmptyWorkingSet, \"otimizador de RAM\")", SeenIn: []string{"MemCleaner", "Mem Reduct", "WinMemoryCleaner", "TUFFSJ (trim)", "PCVitalBoost", "Tutu's Optimizer", "otimizadores comerciais (limpeza de RAM automática)"},
			Mechanism: "EmptyWorkingSet / MemoryEmptyWorkingSets", Verdict: "prejudicial",
			Reason: "O mecanismo é conhecido: o que foi tirado da memória dos programas precisa ser recarregado logo em seguida (page faults), o que gera engasgos. Num teste informal de um desenvolvedor, reler 256 MiB depois de \"limpar\" passou de 0,92 ms para 50,74 ms. O número de RAM livre sobe, mas o desempenho piora. Quando isso é feito automaticamente durante a partida (há otimizadores que esvaziam todos os processos, inclusive o do jogo), o custo cai justamente no jogo. O Axion não esvazia a memória de programas.", Sources: []tweaks.Source{srcWSTrimBench}},
		{Name: "Apagar Prefetch", SeenIn: []string{"Turbo Gamer V13", "Ping_Boost", "optimizerDuck (opção)", "Tutu's Optimizer"},
			Mechanism: `C:\Windows\Prefetch`, Verdict: "prejudicial",
			Reason: "O Prefetch acelera a abertura de programas e o Windows o gerencia sozinho. Apagar só deixa os próximos carregamentos mais lentos."},
		{Name: "Limpar todos os logs de eventos", SeenIn: []string{"WindowsCleanerUtility", "Tutu's Optimizer"},
			Mechanism: "wevtutil cl", Verdict: "prejudicial",
			Reason: "Não ganha desempenho e apaga o histórico usado para diagnosticar travamentos, telas azuis e erros de hardware (WHEA)."},
		{Name: "bcdedit disabledynamictick / useplatformtick / useplatformclock", SeenIn: []string{"Terabyte Tweaker", "otimizadores comerciais (versões antigas)", "vários .bat"},
			Mechanism: "BCD {current}", Verdict: "prejudicial / sem evidência",
			Reason: "O useplatformclock força o HPET, uma fonte de tempo mais lenta. Os outros dois aumentam o consumo sem ganho medido. O Axion detecta e oferece remover.", Sources: []tweaks.Source{srcQPC, srcBCDEdit}},
		{Name: "PowerThrottlingOff = 1", SeenIn: []string{"optimizerDuck", "Pc-Optimizer", "TUFFSJ"},
			Mechanism: `HKLM\SYSTEM\CurrentControlSet\Control\Power\PowerThrottling`, Verdict: "pode piorar",
			Reason: "O EcoQoS reduz a energia de processos em segundo plano, o que deixa mais margem para o jogo. Desligá-lo mantém o segundo plano em alto consumo e, em Intel híbrido, atrapalha o envio de tarefas para os E-cores. O Axion detecta o valor e oferece voltar ao padrão (Correções).", Sources: []tweaks.Source{srcPowerThrottle}},
		{Name: "Chaves ocultas de driver (NVIDIA DisableDynamicPstate, AMD ULPS/PowerGating)", Targets: []string{"|disabledynamicpstate", "|enableulps"}, SeenIn: []string{"optimizerDuck"},
			Mechanism: `HKLM\SYSTEM\...\Control\Class\{4d36e968-…}\000N`, Verdict: "risco sem ganho",
			Reason: "Não são documentadas pelos fabricantes. Travam a GPU em clocks altos (mais calor e ruído) e podem causar instabilidade sem ganho medido. O modo \"desempenho máximo\" do driver, por jogo, faz o mesmo de forma suportada."},
		{Name: "Desativar serviços do Xbox", SeenIn: []string{"Turbo Gamer V13", "Tutu's Optimizer", "vários .bat", "otimizadores comerciais (nível equilibrado)"},
			Mechanism: "XblAuthManager, XblGameSave, XboxNetApiSvc, XboxGipSvc", Verdict: "prejudicial",
			Reason: "Quebra Game Pass, salvamento na nuvem, controles Xbox e a Game Bar, que os Ryzen X3D de 2 CCDs usam para detectar jogos. Esses serviços já ficam em Manual e não rodam sem necessidade. O Axion detecta quando estão desativados e oferece reativar (Correções)."},
		{Name: "Desativar o Spooler de impressão", SeenIn: []string{"Turbo Gamer V13", "Tutu's Optimizer", "otimizadores comerciais (nível agressivo)"},
			Mechanism: "Spooler", Verdict: "sem ganho de desempenho",
			Reason: "Impede de imprimir e não acelera nada. (A Microsoft já sugeriu desligá-lo como medida temporária de segurança contra a falha PrintNightmare, em 2021 — por segurança, não por desempenho; a falha foi corrigida por atualização.)"},
		{Name: "Remover o Microsoft Edge", SeenIn: []string{"RyTuneX (opção)", "vários debloaters"},
			Mechanism: "RemoveEdge.ps1", Verdict: "desnecessário / risco",
			Reason: "O WebView2 Runtime é separado do navegador, mas alguns scripts de remoção apagam os dois, quebrando Widgets, Teams, launchers de jogos e o próprio Axion. Remover é desnecessário: o Edge só fica em segundo plano pelo \"Aumento de desempenho na inicialização\" e por extensões em segundo plano, que se desligam em edge://settings/system.", Sources: []tweaks.Source{srcEdgeWV2, srcEdgeBoost}},
		{Name: "Desativar Windows Defender / SmartScreen", Targets: []string{"|disableantispyware", "|disablerealtimemonitoring"}, SeenIn: []string{"vários debloaters"},
			Mechanism: "políticas DisableAntiSpyware / SmartScreen", Verdict: "risco de segurança",
			Reason: "O ganho é mínimo diante do risco. Para jogos, uma exclusão de pasta no Defender é a alternativa segura."},
		{Name: "Limpadores de Registro", SeenIn: []string{"ZTweaks", "vários"},
			Mechanism: "remoção de chaves \"órfãs\"", Verdict: "risco sem ganho",
			Reason: "A Microsoft não suporta esses utilitários: não há ganho de desempenho e podem deixar o sistema sem inicializar.", Sources: []tweaks.Source{srcRegCleaners}},
		{Name: "Arquivo de paginação com tamanho fixo \"otimizado\"", SeenIn: []string{"WindowsOptimizerPro"},
			Mechanism: "Win32_PageFileSetting", Verdict: "prejudicial",
			Reason: "Um tamanho fixo mal escolhido causa \"memória insuficiente\" e fechamento de jogos. O gerenciamento automático do Windows é o recomendado."},
		{Name: "Programas de resolução de timer sempre abertos (TimerResolution.exe, SetTimerResolutionService)", SeenIn: []string{"Pc-Optimizer", "Terabyte Tweaker", "TUFFSJ"},
			Mechanism: "NtSetTimerResolution", Verdict: "sem efeito desde o Win10 2004",
			Reason: "Desde o Windows 10 2004 a resolução é por processo, e no Windows 11 também depende da janela estar visível. O jogo já pede o que precisa. O Axion oferece o modo global como opção experimental, com medição.", Sources: []tweaks.Source{srcTimer, srcTimerDawson}},
		{Name: "\"Otimização de rota\" local (como ExitLag/WTFast) sem servidores próprios", SeenIn: []string{"anúncios de otimizadores"},
			Mechanism: "rotas estáticas / DNS", Verdict: "impossível localmente",
			Reason: "Mudar a rota exige servidores intermediários (relays). No CS2 oficial o tráfego já entra na rede da Valve pelo relay mais próximo (SDR). Um GPN com servidores próprios só ajuda quando a rota da operadora até esse relay é ruim: a página Rede e ping mede o relay mais próximo e mostra se é o seu caso.", Sources: []tweaks.Source{srcSDRValve}},
		{Name: "Trocar o DNS para \"diminuir o ping\"", SeenIn: []string{"guias de otimização", "Ping_Boost"},
			Mechanism: "servidor DNS do adaptador", Verdict: "sem efeito no ping",
			Reason: "O DNS só traduz um nome (como api.steampowered.com) em endereço antes de conectar; durante a partida o jogo fala direto com o endereço. Um DNS mais rápido abre sites e faz o login mais depressa, não muda o ping. A página Rede e ping compara o seu DNS com os públicos e só oferece a troca quando o seu falha ou demora (para abrir sites e o login), com desfazer exato.", Sources: []tweaks.Source{srcRFC1034}},
		{Name: "cl_interp e scripts de interpolação no CS2", SeenIn: []string{"configs e guias antigos do CS:GO"},
			Mechanism: "comandos de console / autoexec", Verdict: "sem efeito (travado pela Valve)",
			Reason: "A Valve bloqueou a mudança direta do cl_interp em fevereiro de 2024, porque valores altos causavam rubber-banding; a interpolação agora segue a opção \"Buffering to smooth over packet loss\". A página Rede e ping diz qual valor dela combina com a perda e a oscilação medidas.", Sources: []tweaks.Source{srcCS2Interp}},
		{Name: "Abrir portas, UPnP ou \"NAT aberto\" para o CS2", SeenIn: []string{"guias de roteador", "fóruns"},
			Mechanism: "redirecionamento de portas no roteador", Verdict: "sem efeito no CS2",
			Reason: "No matchmaking oficial todos se conectam aos relays da Valve (SDR), sem conexões de entrada até o seu PC. Abrir portas só importa para hospedar servidores ou jogos que usam P2P direto, e deixa o PC mais exposto.", Sources: []tweaks.Source{srcSDRValve}},
		{Name: "Consolidar serviços (SvcHostSplitThresholdInKB)", Targets: []string{"|svchostsplitthresholdinkb"}, SeenIn: []string{"WinUtil", "optimizerDuck", "RyTuneX", "Tutu's Optimizer"},
			Mechanism: `HKLM\SYSTEM\CurrentControlSet\Control`, Verdict: "sem ganho relevante",
			Reason: "Economiza pouca RAM em PCs com 16 GB ou mais e faz uma falha derrubar vários serviços de uma vez, porque passam a dividir o mesmo processo."},
		{Name: "Tempos de desligamento (AutoEndTasks, WaitToKillAppTimeout, HungAppTimeout)", SeenIn: []string{"Ping_Boost", "RyTuneX", "Tutu's Optimizer"},
			Mechanism: `HKCU\Control Panel\Desktop`, Verdict: "risco sem ganho em jogo",
			Reason: "Só acelera o desligamento, e o AutoEndTasks fecha programas sem salvar."},
		{Name: "Forçar o driver NVMe nativo do Windows Server no Windows 11 (FeatureManagement\\Overrides 735209102, 1853569164, 156965516, 3244671118, 1409234060; ViVeTool 60786016/48433719)", SeenIn: []string{"Tutu's Optimizer", "guias de 2026"},
			Mechanism: `HKLM\SYSTEM\CurrentControlSet\Policies\Microsoft\FeatureManagement\Overrides`, Verdict: "não suportado",
			Reason: "O nvmedisk.sys só é suportado no Windows Server 2025. Em março de 2026 as builds Insider do Windows 11 deixaram de aceitar esses valores (o bloqueio pode chegar às versões estáveis), e quem usou relatou Samsung Magician e WD Dashboard sem reconhecer o SSD, pedidos da chave de recuperação do BitLocker e risco de o Windows não iniciar. É um driver só de NVMe: a promessa de acelerar SSDs SATA não tem base. O Axion detecta os valores e oferece remover.", Sources: []tweaks.Source{srcNVMeServer, srcNVMeBlocked, srcNVMeHot}},
		{Name: "Desligar o TRIM ou a otimização de unidades \"para proteger o SSD\"", SeenIn: []string{"guias de \"otimização de SSD\"", "scripts de debloat"},
			Mechanism: "fsutil behavior set disabledeletenotify 1 / tarefa \\Microsoft\\Windows\\Defrag\\ScheduledDefrag desativada", Verdict: "prejudicial",
			Reason:  "Parte de um mito da época dos primeiros SSDs. Sem TRIM o SSD perde desempenho de gravação com o tempo e se desgasta mais. A tarefa de otimização não desfragmenta o SSD toda semana: faz o retrim uma vez por mês e, segundo a equipe de armazenamento do Windows (2014), só desfragmenta um SSD quando há instantâneos de volume (Restauração do Sistema), porque a cópia-na-gravação fica lenta num volume muito fragmentado. O Axion detecta as duas coisas e oferece religar.",
			Sources: []tweaks.Source{srcFsutilTrim, srcDefragCmd, srcSSDStory}},
		{Name: "Apagar o cache de shaders (DirectX, NVIDIA, AMD)", SeenIn: []string{"RyTuneX", "Marcão Boost", "WindowsOptimizerPro"},
			Mechanism: `%LOCALAPPDATA%\D3DSCache, NVIDIA\DXCache e GLCache, AMD\DxCache`, Verdict: "prejudicial",
			Reason: "O cache guarda os shaders já compilados para a sua placa. Apagar força cada jogo a compilar tudo de novo: carregamentos longos e travadas nas primeiras partidas. Apagar só faz sentido para resolver um problema específico, como um cache corrompido.", Sources: []tweaks.Source{srcShaderBlog}},
		{Name: "\"Limpar\" o cache dos navegadores como otimização", SeenIn: []string{"RyTuneX", "PCVitalBoost", "WindowsOptimizerPro"},
			Mechanism: "pastas Cache/Code Cache do Chrome, Edge e Firefox", Verdict: "sem ganho",
			Reason: "Não deixa o PC nem os jogos mais rápidos: o navegador baixa tudo de novo e as páginas abrem mais devagar, como a própria ajuda do Google avisa. Para um site com problema, use a opção do próprio navegador.", Sources: []tweaks.Source{srcChromeCache}},
		{Name: "Apagar a pasta SoftwareDistribution (downloads do Windows Update)", SeenIn: []string{"Tutu's Optimizer"},
			Mechanism: `C:\Windows\SoftwareDistribution\Download`, Verdict: "risco sem ganho",
			Reason: "Apagar com o Windows Update em andamento pode interromper uma instalação e forçar o download de novo. Para liberar esse espaço existe a Limpeza do Windows Update em Configurações > Armazenamento > Arquivos temporários (a página Limpeza do Axion leva até lá).", Sources: []tweaks.Source{srcFreeSpace}},
		{Name: "KeyboardDelay = 0 e KeyboardSpeed = 31 para \"resposta instantânea\"", SeenIn: []string{"Tutu's Optimizer"},
			Mechanism: `HKCU\Control Panel\Keyboard`, Verdict: "mito (não é atraso de entrada)",
			Reason: "Os dois valores controlam só a repetição automática de uma tecla segurada: 0 é o atraso mais curto antes de repetir (cerca de 250 ms) e 31 a repetição mais rápida (cerca de 30 por segundo, já o padrão do Windows). O primeiro toque é enviado na hora em qualquer configuração, e os jogos leem tecla apertada e solta, não a repetição. É preferência de digitação, ajustável no Painel de Controle.", Sources: []tweaks.Source{srcKbdDelay}},
		{Name: "Desativar o Relatório de Erros do Windows (WerSvc)", SeenIn: []string{"Tutu's Optimizer", "vários debloaters"},
			Mechanism: "serviço WerSvc desativado", Verdict: "prejudicial",
			Reason: "O serviço só trabalha quando um programa trava ou o Windows tem um problema, então não custa nada durante o jogo. Desativado, os relatórios de falha deixam de ser gerados, o que dificulta descobrir a causa de fechamentos e travamentos. O Axion detecta e oferece reativar.", Sources: []tweaks.Source{srcWERDoc}},
		{Name: "DisablePagingExecutive = 1 e LargeSystemCache = 1 (\"cache do kernel na RAM\")", Targets: []string{"|disablepagingexecutive"}, SeenIn: []string{"Tutu's Optimizer", "otimizadores comerciais"},
			Mechanism: `HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Memory Management`, Verdict: "sem ganho medido",
			Reason: "A Microsoft documenta o DisablePagingExecutive para depuração de drivers e cita ganho em máquinas com muita memória por evitar reler código do disco; como o Windows só tira esse código da RAM quando precisa de memória, com RAM sobrando a diferença some e não há medição pública em jogos. Não é prejudicial, mas fica de fora por falta de ganho. Já o LargeSystemCache = 1 é a configuração de servidor de arquivos: favorece o cache de arquivos em vez da memória dos programas. O Axion detecta o LargeSystemCache e oferece voltar ao padrão.", Sources: []tweaks.Source{srcPagingExec, srcLargeCache}},
		{Name: "Desligar o Modo de Jogo junto com a Game Bar/gravação", SeenIn: []string{"Tutu's Optimizer (opção de Game DVR)"},
			Mechanism: "AutoGameModeEnabled = 0 e AllowAutoGameMode = 0", Verdict: "prejudicial",
			Reason: "Desligar a gravação não exige desligar o Modo de Jogo. O Modo de Jogo prioriza o jogo e segura instalações de driver e avisos de reinício do Windows Update durante a partida, e nos Ryzen X3D de 2 CCDs o driver da AMD usa essa detecção de jogo. O Axion desliga só a gravação e mantém o Modo de Jogo.", Sources: []tweaks.Source{srcGameMode, srcX3D}},
		{Name: "NetworkThrottlingIndex = FFFFFFFF (\"desligar a limitação de rede do MMCSS\")", Targets: []string{"|networkthrottlingindex"}, SeenIn: []string{"optimizerDuck", "Ping_Boost", "vários .reg", "otimizadores comerciais (pacotes \"competitivos\")"},
			Mechanism: `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Multimedia\SystemProfile`, Verdict: "pode piorar",
			Reason: "O valor não é descrito na documentação do MMCSS. A única medição encontrada (pesquisa de rede do djdallmann) viu a latência de DPC do NDIS aumentar de forma notável com ele desligado e recomenda manter o padrão (10). Não há medição pública de ping menor. O Axion detecta o valor e oferece voltar ao padrão (Correções).", Sources: []tweaks.Source{srcDjdNetwork, srcMMCSS}},
		{Name: "\"Efeitos visuais: melhor desempenho\" só pelo Registro (VisualFXSetting = 2)", SeenIn: []string{"vários .reg", "versões anteriores do Axion", "otimizadores comerciais"},
			Mechanism: `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\VisualEffects`, Verdict: "sem efeito",
			Reason: "Esse valor só marca a opção na janela; sozinho não aplica o conjunto de efeitos (que também desliga a suavização de fontes). Não muda FPS em tela cheia. Se quiser, use Configurações > Sistema > Sobre > Configurações avançadas do sistema > Desempenho.", Sources: []tweaks.Source{srcVisualFXQ}},
		{Name: "\"Versões Pro\" de otimizadores e atualizadores de driver em repositórios-isca do GitHub", SeenIn: []string{"repositórios de \"Driver Booster Pro\", \"Ashampoo\" e \"PC Optimizer\" sem código (analisados em 09/2026)"},
			Mechanism: "comando do PowerShell que baixa e executa um script da internet, arquivo compactado com senha, commits automáticos por bot, download fora do GitHub", Verdict: "não executar (malware provável)",
			Reason: "Nenhum tem código-fonte. O README manda colar no PowerShell um comando que baixa e executa na hora um script da internet, ou baixar um arquivo com senha, que o antivírus não consegue examinar. Commits automáticos a cada poucas horas e README gerado por IA imitam um projeto ativo, o mesmo padrão de campanhas documentadas que espalharam ladrões de senhas e de carteiras pelo GitHub. O Axion não baixa nem executa script nenhum: o que ele instala (drivers e os programas da página Instalar programas) passa pelo instalador assinado da NVIDIA ou pelo winget da Microsoft, que confere o hash de cada instalador com o catálogo, e só depois da sua confirmação.", Sources: []tweaks.Source{srcGitVenom, srcTrendFakeGH, srcMSGitHubMal}},
		{Name: "Prioridade alta para o audiodg.exe (e outros processos do sistema) via IFEO", Targets: []string{`image file execution options\audiodg.exe`}, SeenIn: []string{"guias de mouse de alto polling", "sugestões de assistentes de IA"},
			Mechanism: "IFEO audiodg.exe PerfOptions → CpuPriorityClass", Verdict: "risco sem ganho",
			Reason: "O audiodg.exe é o motor de áudio do Windows, um processo do sistema da mesma classe do csrss.exe, que o Axion não altera. O motor de áudio já usa o MMCSS para priorizar as suas threads de tempo real, e não há medição pública mostrando que subir a prioridade do processo evite estalos com mouses de 2000 a 8000 Hz. Se o áudio estala com um mouse rápido, o caminho é medir (a latência de DPC mostra qual driver atrasa) e olhar o driver de áudio ou o hub USB."},
		{Name: "Afinidade de núcleos fixa por IFEO (AffinityMask, \"só P-cores\" ou \"só o CCD do V-Cache\")", Targets: []string{"|affinitymask"}, SeenIn: []string{"sugestões de assistentes de IA", "guias de CPU híbrida"},
			Mechanism: `IFEO <jogo>.exe\PerfOptions → AffinityMask`, Verdict: "sem evidência / risco",
			Reason: "Não encontrei documentação nem medição de que o Windows leia um AffinityMask em PerfOptions (a afinidade documentada vive no próprio executável ou é definida por processo, com SetProcessAffinityMask), então o ajuste pode simplesmente não fazer nada. Quando faz efeito por outro meio, tira do agendador uma decisão que ele toma melhor: no Windows 11 o Thread Director prefere os P-cores para o primeiro plano, e nos Ryzen X3D de 2 CCDs o driver da AMD e a Game Bar escolhem o CCD do V-Cache (o Axion confere se esse serviço está ativo). Uma máscara errada, e a contagem de núcleos varia por modelo, deixa o jogo sem núcleos suficientes.", Sources: []tweaks.Source{srcX3D}},
		{Name: "Desligar o Packet Coalescing (*PacketCoalescing) para \"menos jitter\"", Targets: []string{"|*packetcoalescing"}, SeenIn: []string{"sugestões de assistentes de IA", "guias de placa de rede"},
			Mechanism: "propriedade avançada *PacketCoalescing da placa de rede", Verdict: "sem efeito no jogo",
			Reason: "O coalescimento de pacotes do NDIS só junta pacotes de broadcast e multicast (como anúncios de nome e descoberta de dispositivos) para a placa acordar a CPU menos vezes; um pacote que não combina com o filtro, como o tráfego unicast do jogo, gera a interrupção na hora. Desligá-lo não muda o ping nem o jitter do jogo: só aumenta o trabalho da CPU e o consumo. O Axion lista a opção quando ela está fora do padrão e oferece voltar ao padrão do driver (Rede, correção das placas de rede).", Sources: []tweaks.Source{srcPacketCoal}},
		{Name: "SOCD, Snap Tap e \"null binds\" por software no CS2", SeenIn: []string{"otimizadores comerciais para CS2"},
			Mechanism: "programa que intercepta o teclado e solta sozinho a tecla de direção oposta", Verdict: "expulsão da partida (CS2)",
			Reason: "Desde a atualização de 19/08/2024 a Valve ignora ou pune null binds e jump binds e detecta o contra-strafe assistido por hardware (o Snap Tap), expulsando o jogador da partida nos servidores oficiais. Um programa que solta a tecla oposta por você faz esse tipo de automação. Outros jogos e ligas têm regras próprias. O Axion não gera nem solta teclas por você; só lê o mouse durante a medição de taxa de polling.", Sources: []tweaks.Source{srcCS2SnapTap}},
		{Name: "Binds de várias ações (null bind, jump-throw), Snap Tap/SOCD por script e autoexec com convars protegidas no CS2", Targets: []string{"cs2_user_keys", "autoexec.cfg", "cl_allow_multi_input_binds"}, SeenIn: []string{"configs e autoexec compartilhados de CS2", "teclados com SOCD (Snap Tap)"},
			Mechanism: "binds em cs2_user_keys_0_slot0.vcfg ou autoexec.cfg, a convar protegida cl_allow_multi_input_binds, recurso do teclado", Verdict: "proibido pela Valve",
			Reason: "Desde 19/8/2024 a Valve não permite automação por script ou hardware que substitua o counter-strafe e o movimento: binds com mais de uma ação de movimento ou ataque são ignorados e só voltam com uma convar protegida contra cheat, e o counter-strafe assistido por hardware é detectado. O Axion não grava binds, autoexec nem as convars da conta; o preset do CS2 muda só opções do menu Vídeo no cs2_video.txt, com o jogo fechado.", Sources: []tweaks.Source{srcCS2SideStep, srcCS2Notes0819}},
		{Name: "Alterar a BIOS pelo Windows (AMI SCEWIN com amifldrv64.sys e amigendrv64.sys)", SeenIn: []string{"otimizadores comerciais (ajuste de BIOS)"},
			Mechanism: "SCEWIN_64.exe grava a NVRAM da BIOS por um driver de kernel", Verdict: "risco alto",
			Reason: "Exige carregar o amifldrv64.sys, que o LOLDrivers lista como driver vulnerável (verificado) usado para obter privilégio de kernel, o mesmo tipo de risco do RwDrv.sys. O Axion não carrega drivers desse tipo nem grava a BIOS: o guia de BIOS mostra onde fica cada opção (XMP/EXPO, Resizable BAR, virtualização) na tela da própria placa.", Sources: []tweaks.Source{srcAmiFlDrv, srcRwDrv}},
		{Name: "Suspender ou rebaixar programas durante o jogo (navegador, Spotify, Discord, OneDrive…)", SeenIn: []string{"otimizadores comerciais (modo de jogo)"},
			Mechanism: "NtSuspendProcess/NtResumeProcess ou prioridade baixa nos processos em segundo plano", Verdict: "risco sem ganho medido",
			Reason: "Suspender congela todas as threads do programa onde estiverem. A Microsoft avisa que a suspensão de threads é feita principalmente para depuradores e que suspender uma thread que segura um mutex ou uma seção crítica pode travar quem depende dela. Música, chamadas de voz, downloads e sincronizações param, e se o otimizador fechar antes de retomar, os programas ficam congelados. Rebaixar também é desnecessário: no padrão do Windows o programa em primeiro plano já recebe mais tempo de processador, e o Windows já põe janelas fora de foco e serviços em níveis de QoS mais baixos. O Axion não suspende nem rebaixa processos.", Sources: []tweaks.Source{srcSuspendThread, srcPrioSepMS, srcPowerThrottle}},
		{Name: "Afinidade \"só núcleos físicos\" escolhendo os processadores lógicos pares", SeenIn: []string{"otimizadores comerciais (\"só núcleos físicos\")"},
			Mechanism: "SetProcessAffinityMask e CPU Sets no processo do jogo, com máscara calculada pelo número de núcleos", Verdict: "errado em CPUs híbridas e X3D",
			Reason: "A ideia supõe que todo núcleo tem dois processadores lógicos lado a lado. Nos Intel híbridos os E-cores não têm Hyper-Threading (o i9-13900K tem 24 núcleos e 32 threads), então uma máscara de índices pares não corresponde a um processador por núcleo. Nos Ryzen X3D de 2 CCDs ela pega os dois CCDs e desfaz a escolha do CCD com 3D V-Cache que o driver da AMD e a Game Bar fazem. A topologia real vem do Windows, núcleo por núcleo (GetLogicalProcessorInformationEx). O Axion não fixa afinidade de jogos.", Sources: []tweaks.Source{srcCoreTopology, srcI913900K, srcX3D}},
		{Name: "\"Escudo\" contra o EcoQoS no processo do jogo (ProcessPowerThrottling)", SeenIn: []string{"otimizadores comerciais (boost de jogo)"},
			Mechanism: "SetProcessInformation(ProcessPowerThrottling) com ControlMask = 1 e StateMask = 0 (HighQoS explícito)", Verdict: "sem efeito no jogo em foco",
			Reason: "Pela Microsoft, a janela em primeiro plano e em foco já recebe o nível High de QoS, o mesmo de quem marca HighQoS explicitamente. Os níveis mais baixos valem para janelas visíveis sem foco (Medium), minimizadas ou encobertas (Low) e, na bateria, para o app em foco depois de um tempo sem uso. A marcação só muda algo nesses casos, por exemplo com o foco em outro programa num segundo monitor, e não há medição pública de ganho. O Axion não marca processos.", Sources: []tweaks.Source{srcPowerThrottle}},
		{Name: "Prioridade de E/S alta para o jogo (IFEO IoPriority, ProcessIoPriority)", Targets: []string{"|iopriority"}, SeenIn: []string{"otimizadores comerciais (IFEO do jogo)", "otimizadores comerciais (boost de jogo)"},
			Mechanism: `IFEO <jogo>.exe\PerfOptions → IoPriority; NtSetInformationProcess(ProcessIoPriority)`, Verdict: "sem evidência",
			Reason: "A Microsoft documenta as dicas de prioridade de E/S (muito baixa, baixa, normal, alta e crítica, nessa ordem) e diz que alta e crítica são reservadas para uso do sistema; se a dica é honrada depende do driver de armazenamento. O IoPriority do IFEO não é documentado. Se seguir a mesma ordem, o 3 que alguns otimizadores enviam (ProcessIoPriority) ou gravam no IFEO corresponde à prioridade alta reservada ao sistema, e o 2 é o normal. Não há medição pública de carregamento mais rápido ou de menos travadas. Nos perfis por jogo o Axion grava a prioridade de CPU (CpuPriorityClass), não a de E/S.", Sources: []tweaks.Source{srcIoPrioHint, srcIoHintInfo}},
		{Name: "Prioridade de agendamento da GPU por processo (D3DKMTSetProcessSchedulingPriorityClass)", SeenIn: []string{"otimizadores comerciais"},
			Mechanism: "gdi32!D3DKMTSetProcessSchedulingPriorityClass no processo do jogo", Verdict: "sem evidência",
			Reason: "É uma interface de driver gráfico, e a Microsoft não publica medição de ganho em jogos. Na função equivalente para aplicativos (SetGPUThreadPriority), avisa que a prioridade usada de forma inadequada pode prejudicar a velocidade de renderização e pede para medir o impacto no aplicativo e no sistema. O Axion não altera a prioridade da GPU.", Sources: []tweaks.Source{srcD3DKMTPrio, srcGPUPrio}},
		{Name: "Esvaziar a lista de espera (standby list) da memória", SeenIn: []string{"otimizadores comerciais (limpeza de RAM)"},
			Mechanism: "NtSetSystemInformation (lista de memória: purgar a lista em espera)", Verdict: "sem ganho",
			Reason: "A memória em espera é cache de dados e arquivos que não estão em uso ativo, e a Microsoft a conta como memória disponível, entregue a qualquer programa que precisar. Esvaziá-la não aumenta a memória disponível, só joga fora o cache: o próximo acesso a esses arquivos (texturas, mapas, o próprio jogo) volta a ler do disco. A função usada não é documentada pela Microsoft. O Axion lê o tamanho das listas de memória, mas não as esvazia.", Sources: []tweaks.Source{srcStandbyMem}},
		{Name: "MMCSS \"Scheduling Category\" = High e \"Priority\" = 6 na tarefa Games", Targets: []string{`\tasks\games|scheduling category`, `\tasks\games|priority`}, SeenIn: []string{"otimizadores comerciais"},
			Mechanism: `Multimedia\SystemProfile\Tasks\Games`, Verdict: "sem evidência",
			Reason: "Pela documentação do MMCSS, a categoria High (prioridades 23 a 26) foi feita para tarefas de áudio profissional e, nela, o Priority é sempre tratado como 2, então o 6 não muda nada. Só afeta threads que o próprio jogo registra na tarefa Games, e não há medição pública de ganho.", Sources: []tweaks.Source{srcMMCSS}},
		{Name: "Desligar a compressão de memória (Disable-MMAgent -MemoryCompression) como \"otimização\"", Targets: []string{"mmagent:memorycompression"}, SeenIn: []string{"otimizadores comerciais"},
			Mechanism: "WMI PS_MMAgent.Disable com MemoryCompression (o que o cmdlet Disable-MMAgent chama)", Verdict: "sem evidência",
			Reason: "A página do Disable-MMAgent lista o parâmetro, mas não descreve o efeito nem promete ganho. A única fonte encontrada sobre o efeito (resposta de consultor independente no Microsoft Q&A) diz que, sem a compressão, o arquivo de paginação pode ser mais usado e a lentidão aparece se a RAM encher; o que se ganha é ver menos memória em uso e um pouco menos de CPU. Não há medição pública de FPS ou de latência. Por isso o Axion não desliga a compressão. Se outro programa a desligou, o Axion lê o estado pelo WMI, sem PowerShell, e oferece religar (Correções); quem desligou de propósito pode manter.", Sources: []tweaks.Source{srcMMAgentEnable, srcMemCompQA}},
		{Name: "Prioridade de interrupção (DevicePriority) junto com o MSI, como \"prioridade alta\"", Targets: []string{"|devicepriority"}, SeenIn: []string{"sugestões de assistentes de IA", "guias de MSI"},
			Mechanism: `Enum\<PCI>\Device Parameters\Interrupt Management\Affinity Policy → DevicePriority`, Verdict: "sem fonte / valor invertido",
			Reason: "Nessa chave a Microsoft documenta só DevicePolicy e AssignmentSetOverride; o valor DevicePriority não tem documentação. Se o Windows o lesse como a enumeração IRQ_PRIORITY (Undefined, Low, Normal, High, nessa ordem), o valor 1 seria IrqPriorityLow, o oposto de \"prioridade alta\". Por isso os itens de MSI do Axion gravam só o MSISupported e não prometem prioridade de interrupção.", Sources: []tweaks.Source{srcIRQPriority}},
		{Name: "Pacote de ajustes no perfil global do driver NVIDIA com nvidiaProfileInspector + .nip (desempenho máximo, filtragem em alto desempenho, otimização em threads, pré-renderizados 1, Ultra Low Latency, V-Sync forçado)",
			Targets: []string{"nvdrs:base|1057eb71", "nvdrs:global|1057eb71", "nvdrs:base|00ce2691", "nvdrs:global|00ce2691", "nvdrs:base|20c1221e", "nvdrs:global|20c1221e",
				"nvdrs:base|10835000", "nvdrs:global|10835000", "nvdrs:base|007ba09e", "nvdrs:global|007ba09e"},
			SeenIn:    []string{"pacotes de otimização (nvidiaProfileInspector)"},
			Mechanism: "nvidiaProfileInspector.exe importando um .nip temporário no Base Profile", Verdict: "global demais / desfazer inexato",
			Reason: "Força para todos os programas valores sem medição pública de ganho que o Axion tenha encontrado; \"Preferir desempenho máximo\" no global eleva o consumo em repouso, e a medição pública do TheFPSReview viu a experiência igual nos três modos. " +
				"Há pacotes que gravam até 9 ajustes no Base Profile (energia máxima, baixa latência Ultra, otimização em threads…) e cujo desfazer não volta ao padrão: grava no V-Sync 0x60925619, valor que não existe no driver (o padrão é 0x60925292), e o modo de energia Adaptável em vez do padrão Ideal. " +
				"O Axion lê o perfil pelo NVAPI, muda um ajuste por vez guardando o valor anterior, oferece o modo de energia máximo só no perfil de um jogo e detecta esses valores para voltar ao padrão do driver.",
			Sources: []tweaks.Source{srcNvSettingsH, srcFPSReviewPower}},
	}
}

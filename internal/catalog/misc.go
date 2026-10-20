package catalog

import (
	"strings"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

var srcWPBT = tweaks.Source{Title: "WinUtil — Windows Platform Binary Table (DisableWpbtExecution)", URL: "https://winutil.christitus.com/code-reference/tweaks/essential-tweaks/wpbt/"}
var srcWPBTInfo = tweaks.Source{Title: "persistence-info — WPBT: como o smss.exe executa wpbbin.exe a partir do UEFI", URL: "https://persistence-info.github.io/Data/wpbbin.html"}
var srcEclypsium = tweaks.Source{Title: "Eclypsium — atualizador escondido em firmwares Gigabyte (WPBT)", URL: "https://eclypsium.com/blog/supply-chain-risk-from-gigabyte-app-center-backdoor/"}

func gpuTweaks() []*tweaks.Tweak {
	return []*tweaks.Tweak{
		{
			ID: "gpu.mpo.off", Category: "gpu", Title: "Correção: desativar Multiplane Overlay (MPO)",
			Summary:  "Solução publicada pela NVIDIA para apps que piscam ou travam ao redimensionar janelas; relatos também a ligam a engasgos em jogos em janela com mais de um monitor.",
			Detail:   "OverlayTestMode = 5 em HKLM\\SOFTWARE\\Microsoft\\Windows\\Dwm (o mesmo .reg publicado pela NVIDIA). Só use se você tem esses sintomas: o MPO também traz benefícios de consumo e latência em modo janela quando funciona bem. Se o valor ainda funciona nas versões 24H2/25H2 é contestado. Requer reinício.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcMPO},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"estabilidade"}, OneClick: false,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !discreteGPU(f) {
					return false, "requer GPU dedicada"
				}
				return true, ""
			},
			// A fix for a symptom: never part of a one-click tier.
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if len(f.Displays) > 1 {
					return tweaks.Rec(tweaks.RecNo, "só se houver tela piscando ou travadas no desktop (comum com mais de um monitor); sem sintoma, o MPO ajuda")
				}
				return tweaks.Rec(tweaks.RecNo, "só se houver tela piscando ou travadas no desktop ou em jogos em janela; sem sintoma, o MPO ajuda")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(`HKLM\SOFTWARE\Microsoft\Windows\Dwm`, "OverlayTestMode", sys.DWord(5))}
			},
		},
		{
			ID: "gpu.msi.mode", Category: "gpu", Title: "Modo MSI (Message Signaled Interrupts) na GPU dedicada",
			Summary: "Força interrupções por mensagem (MSI) na GPU. Só vale se o driver ainda usa o modo herdado; a maioria das placas atuais já usa MSI de fábrica. Opção avançada, fora do 1 clique.",
			Detail: "MSISupported=1 na chave de hardware PCI da placa de vídeo (HKLM\\SYSTEM\\CurrentControlSet\\Enum\\PCI\\...\\Device Parameters\\Interrupt Management\\MessageSignaledInterruptProperties). O Axion não grava prioridade de interrupção: o valor DevicePriority não está documentado pela Microsoft. " +
				"O MSI é um recurso documentado do Windows, e os drivers atuais da NVIDIA e da AMD já o declaram no instalador, então na maioria dos PCs o valor já está em vigor e nada muda. " +
				"A Microsoft descreve o mecanismo, mas não publica medição de ganho de latência ao forçá-lo: o benefício é uma hipótese a medir (LatencyMon ou PresentMon, antes e depois). " +
				"Se a placa não tratar bem o MSI, o efeito é tela preta ou travamento na inicialização; nesse caso, desfaça pelo Histórico (em modo seguro, se necessário).",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcMSI},
			Risk: tweaks.RiskModerate, Reboot: tweaks.RebootRestart, Tags: []string{"latencia"},
			Applicable: func(f *facts.Facts) (bool, string) {
				g := f.PrimaryGPU()
				if g == nil || g.Virtual() || g.PNPDeviceID == "" {
					return false, "nenhuma GPU dedicada com identificador PCI encontrada"
				}
				if !strings.HasPrefix(strings.ToUpper(g.PNPDeviceID), "PCI\\") {
					return false, "o dispositivo não está conectado ao barramento PCI"
				}
				return true, ""
			},
			Recommend: neutral("só para testar com medição; sem sintoma de latência de DPC, não há motivo para mexer"),
			Ops: func(f *facts.Facts) []tweaks.Op {
				g := f.PrimaryGPU()
				if g == nil || g.PNPDeviceID == "" {
					return nil
				}
				base := `HKLM\SYSTEM\CurrentControlSet\Enum\` + g.PNPDeviceID + `\Device Parameters\Interrupt Management`
				msiKey := base + `\MessageSignaledInterruptProperties`
				return []tweaks.Op{
					tweaks.Reg(msiKey, "MSISupported", sys.DWord(1)),
				}
			},
		},
	}
}

func privacyTweaks() []*tweaks.Tweak {
	return []*tweaks.Tweak{
		{
			ID: "privacy.ads", Category: "privacidade", Title: "Anúncios, sugestões e apps promovidos desligados",
			Summary: "Desliga o ID de publicidade, as sugestões no Iniciar e em Configurações, as dicas do Windows e a instalação automática de apps promovidos.",
			Detail: "HKCU\\...\\AdvertisingInfo Enabled=0; ContentDeliveryManager: SilentInstalledAppsEnabled, SystemPaneSuggestionsEnabled, SubscribedContent-338388/338389/353694/353696Enabled = 0; Privacy\\TailoredExperiencesWithDiagnosticDataEnabled=0; UserProfileEngagement\\ScoobeSystemSettingEnabled=0 (tela \"vamos terminar de configurar\"). " +
				"O ID de anúncio e as sugestões em Configurações são opções documentadas pela Microsoft; os valores do ContentDeliveryManager não são documentados e podem mudar entre versões.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcAdsSettings},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"privacidade", "limpeza"}, OneClick: true,
			Recommend: yes("remove propaganda e instalações silenciosas sem nenhuma contrapartida"),
			Ops: func(*facts.Facts) []tweaks.Op {
				cdm := `HKCU\Software\Microsoft\Windows\CurrentVersion\ContentDeliveryManager`
				return []tweaks.Op{
					tweaks.Reg(`HKCU\Software\Microsoft\Windows\CurrentVersion\AdvertisingInfo`, "Enabled", sys.DWord(0)),
					tweaks.Reg(cdm, "SilentInstalledAppsEnabled", sys.DWord(0)),
					tweaks.Reg(cdm, "SystemPaneSuggestionsEnabled", sys.DWord(0)),
					tweaks.Reg(cdm, "SubscribedContent-338388Enabled", sys.DWord(0)),
					tweaks.Reg(cdm, "SubscribedContent-338389Enabled", sys.DWord(0)),
					tweaks.Reg(cdm, "SubscribedContent-353694Enabled", sys.DWord(0)),
					tweaks.Reg(cdm, "SubscribedContent-353696Enabled", sys.DWord(0)),
					tweaks.Reg(`HKCU\Software\Microsoft\Windows\CurrentVersion\Privacy`, "TailoredExperiencesWithDiagnosticDataEnabled", sys.DWord(0)),
					tweaks.Reg(`HKCU\Software\Microsoft\Windows\CurrentVersion\UserProfileEngagement`, "ScoobeSystemSettingEnabled", sys.DWord(0)),
				}
			},
		},
		{
			ID: "privacy.telemetry", Category: "privacidade", Title: "Telemetria no nível mínimo",
			Summary:  "Reduz os dados de diagnóstico enviados à Microsoft ao mínimo que sua edição do Windows permite e desliga os pedidos de feedback.",
			Detail:   "Política AllowTelemetry (HKLM\\SOFTWARE\\Policies\\Microsoft\\Windows\\DataCollection) = 1 \"Obrigatório\" no Home/Pro e DoNotShowFeedbackNotifications=1. O nível 0 só existe no Enterprise/Education; nas outras edições a Microsoft documenta que 0 equivale a 1, então um 0 já gravado conta como no mínimo e não é trocado. O Windows Update continua funcionando normalmente.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcTelemetry, srcTelemetryCSP},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"privacidade"}, OneClick: true,
			Recommend: yes("configuração suportada pela Microsoft, sem efeito colateral"),
			Ops: func(f *facts.Facts) []tweaks.Op {
				level := uint32(1)
				ed := strings.ToLower(f.OS.Edition + " " + f.OS.Caption)
				if strings.Contains(ed, "enterprise") || strings.Contains(ed, "education") {
					level = 0
				}
				dc := `HKLM\SOFTWARE\Policies\Microsoft\Windows\DataCollection`
				// Outside Enterprise/Education, 0 is treated as 1 (Microsoft):
				// both are the minimum, so a 0 already there is left alone.
				var alt []sys.RegValue
				if level == 1 {
					alt = append(alt, sys.DWord(0))
				}
				return []tweaks.Op{
					tweaks.Reg(dc, "AllowTelemetry", sys.DWord(level), alt...),
					tweaks.Reg(dc, "DoNotShowFeedbackNotifications", sys.DWord(1)),
				}
			},
		},
		{
			ID: "privacy.telemetry.tasks", Category: "privacidade", Title: "Tarefas de coleta de compatibilidade desligadas",
			Summary: "Desativa tarefas agendadas de coleta de dados de compatibilidade e de experiência do usuário (Microsoft Compatibility Appraiser e afins), que rodam em segundo plano.",
			Detail: "Agendador de Tarefas: \\Microsoft\\Windows\\Application Experience\\Microsoft Compatibility Appraiser, \\ProgramDataUpdater; \\Customer Experience Improvement Program\\Consolidator, \\UsbCeip; \\Feedback\\Siuf\\DmClient, \\DmClientOnScenarioDownload. Tarefas inexistentes na sua versão são ignoradas. " +
				"O ganho é de privacidade; não há medição publicada de efeito em FPS. O Appraiser também informa a compatibilidade do PC para atualizações do Windows.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcSchedTasks, srcTelemetry},
			Risk: tweaks.RiskModerate, Reboot: tweaks.RebootNone, Tags: []string{"privacidade"}, OneClick: false,
			RiskNote:  "O Microsoft Compatibility Appraiser é o que avalia se o PC é compatível com a próxima versão do Windows: com ele desligado essa avaliação não roda, e a oferta de uma atualização de versão pode atrasar ou não aparecer. Por isso o item fica fora do 1 clique.",
			Recommend: yes("menos coleta em segundo plano, sem efeito nos jogos"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tasksOp("telemetry", []string{
					`\Microsoft\Windows\Application Experience\Microsoft Compatibility Appraiser`,
					`\Microsoft\Windows\Application Experience\ProgramDataUpdater`,
					`\Microsoft\Windows\Customer Experience Improvement Program\Consolidator`,
					`\Microsoft\Windows\Customer Experience Improvement Program\UsbCeip`,
					`\Microsoft\Windows\Feedback\Siuf\DmClient`,
					`\Microsoft\Windows\Feedback\Siuf\DmClientOnScenarioDownload`,
				})}
			},
		},
		{
			ID: "privacy.diagtrack", Category: "privacidade", Title: "Serviço de telemetria (DiagTrack) desligado",
			Summary:  "Desativa o serviço \"Experiências do Usuário Conectado e Telemetria\", que envia os dados de diagnóstico.",
			Detail:   "Serviço DiagTrack → Desativado (e parado). Com ele desligado alguns recursos de diagnóstico e o Programa Windows Insider deixam de funcionar; atualizações não são afetadas.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcTelemetry, srcServices},
			Risk: tweaks.RiskModerate, Reboot: tweaks.RebootNone, Tags: []string{"privacidade"}, OneClick: false,
			Recommend: neutral("ganho de privacidade; efeito de desempenho pequeno"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Svc("DiagTrack", sys.StartDisabled, true)}
			},
		},
		{
			ID: "privacy.activity", Category: "privacidade", Title: "Histórico de atividades desligado",
			Summary:  "Impede que o Windows guarde no PC o histórico dos apps e arquivos que você usa.",
			Detail:   "Políticas em HKLM\\SOFTWARE\\Policies\\Microsoft\\Windows\\System: EnableActivityFeed=0, PublishUserActivities=0, UploadUserActivities=0. No Windows 11 o envio desse histórico à Microsoft já foi descontinuado; ele fica guardado só no PC.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcActivity, srcActivityPriv},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"privacidade"}, OneClick: true,
			Recommend: yes("sem contrapartida para jogos"),
			Ops: func(*facts.Facts) []tweaks.Op {
				k := `HKLM\SOFTWARE\Policies\Microsoft\Windows\System`
				return []tweaks.Op{
					tweaks.Reg(k, "EnableActivityFeed", sys.DWord(0)),
					tweaks.Reg(k, "PublishUserActivities", sys.DWord(0)),
					tweaks.Reg(k, "UploadUserActivities", sys.DWord(0)),
				}
			},
		},
		{
			ID: "privacy.websearch", Category: "privacidade", Title: "Pesquisa na web (Bing) fora do Iniciar",
			Summary: "A pesquisa do Iniciar deixa de mostrar resultados e sugestões do Bing, ficando com apps, arquivos e configurações do PC.",
			Detail: "Política DisableSearchBoxSuggestions=1 em HKCU\\Software\\Policies\\Microsoft\\Windows\\Explorer. Aplica ao reiniciar o Explorer ou ao entrar novamente. " +
				"A Microsoft documenta esta política só para as sugestões da caixa de pesquisa do Explorador de Arquivos; a remoção dos resultados do Bing no Iniciar é um efeito observado, não documentado, e pode mudar em atualizações.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcSearchPolicy},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootSignOut, Tags: []string{"privacidade", "interface"}, OneClick: true,
			Recommend: yes("o que você digita no Iniciar deixa de ir para o Bing"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(`HKCU\Software\Policies\Microsoft\Windows\Explorer`, "DisableSearchBoxSuggestions", sys.DWord(1))}
			},
		},
		{
			ID: "privacy.widgets", Category: "privacidade", Title: "Widgets e Notícias desligados",
			Summary:  "Remove o painel de Widgets/Notícias da barra de tarefas e os processos que ele mantém em segundo plano.",
			Detail:   "Política AllowNewsAndInterests=0 em HKLM\\SOFTWARE\\Policies\\Microsoft\\Dsh. O painel usa processos WebView2 que ficam carregados na memória.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcWidgets},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootSignOut, Tags: []string{"privacidade", "ram"}, OneClick: true,
			Recommend: yes("libera memória e remove notícias em segundo plano"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(`HKLM\SOFTWARE\Policies\Microsoft\Dsh`, "AllowNewsAndInterests", sys.DWord(0))}
			},
		},
		{
			ID: "security.wpbt", Category: "privacidade", Title: "Bloquear programas instalados pelo firmware (WPBT)",
			Summary: "Impede que a BIOS/UEFI da placa-mãe injete e execute um instalador no Windows a cada boot (ex.: centrais de apps de fabricantes).",
			Detail: "DisableWpbtExecution=1 em HKLM\\SYSTEM\\CurrentControlSet\\Control\\Session Manager (valor não documentado oficialmente pela Microsoft; o mecanismo WPBT é). O smss.exe deixa de gravar e executar o wpbbin.exe fornecido pela tabela WPBT do firmware. " +
				"Em 2023 a Eclypsium mostrou que firmwares Gigabyte usavam esse mecanismo para um atualizador com falha de segurança. Softwares antirroubo embutidos no firmware (comuns em notebooks corporativos, raros em desktops) param de se reinstalar.",
			// The WPBT mechanism is documented, the value that turns it off is
			// not: low evidence (out of the 1-click plans, offered as an option).
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcWPBT, srcWPBTInfo, srcEclypsium},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"seguranca", "limpeza"}, OneClick: true,
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if f.Power.IsLaptop {
					return tweaks.Rec(tweaks.RecNeutral, "em notebooks o WPBT também reinstala o antirroubo do fabricante (Computrace/Absolute); desligue só se não usa")
				}
				m := strings.ToLower(f.Board.Manufacturer + " " + f.Board.SystemMaker)
				if strings.Contains(m, "gigabyte") || strings.Contains(m, "asus") || strings.Contains(m, "micro-star") || strings.Contains(m, "msi") || strings.Contains(m, "asrock") {
					return tweaks.Rec(tweaks.RecYes, "sua placa ("+f.Board.Manufacturer+") é de um fabricante que usa instaladores via firmware")
				}
				return tweaks.Rec(tweaks.RecYes, "impede reinstalação de software do fabricante sem consentimento")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager`, "DisableWpbtExecution", sys.DWord(1))}
			},
		},
	}
}

func updateTweaks() []*tweaks.Tweak {
	return []*tweaks.Tweak{
		{
			ID: "update.do.p2p", Category: "update", Title: "Atualizações sem compartilhamento P2P",
			Summary: "O Windows deixa de trocar partes de atualizações com outros PCs e baixa só dos servidores da Microsoft.",
			Detail: "Política DODownloadMode = 0 (\"HTTP Only\") em HKLM\\SOFTWARE\\Policies\\Microsoft\\Windows\\DeliveryOptimization. Por padrão o Windows só compartilha com PCs da mesma rede local (modo 1), sem usar o upload da sua internet; " +
				"o ajuste só economiza banda de upload se \"Dispositivos na internet\" estava ligado (modo 3).",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcDO},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"rede"}, OneClick: true,
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				switch f.OS.DOMode {
				case 3:
					return tweaks.Rec(tweaks.RecYes, "o compartilhamento com PCs da internet está ligado ("+f.OS.DOSource+"): o upload de atualizações pode disputar a sua banda durante a partida")
				case 0, 99, 100:
					return tweaks.Rec(tweaks.RecNeutral, "o compartilhamento já está desligado")
				}
				return tweaks.Rec(tweaks.RecNeutral, "o Windows só compartilha na rede local (padrão), sem usar a banda da internet")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(`HKLM\SOFTWARE\Policies\Microsoft\Windows\DeliveryOptimization`, "DODownloadMode", sys.DWord(0))}
			},
		},
		{
			ID: "update.drivers.exclude", Category: "update", Title: "Windows Update sem drivers",
			Summary:  "Impede que o Windows Update troque drivers automaticamente (por exemplo, substituir o driver de vídeo que você instalou por uma versão mais antiga).",
			Detail:   "Política ExcludeWUDriversInQualityUpdate=1 em HKLM\\SOFTWARE\\Policies\\Microsoft\\Windows\\WindowsUpdate. Com isso você passa a ser responsável por atualizar drivers de chipset, rede e áudio pelo site do fabricante.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcWUDrivers},
			Risk: tweaks.RiskModerate, Reboot: tweaks.RebootNone, Tags: []string{"estabilidade"}, OneClick: false,
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if discreteGPU(f) {
					return tweaks.Rec(tweaks.RecNeutral, "útil se você instala o driver da GPU manualmente; exige atualizar os outros drivers por conta própria")
				}
				return tweaks.Rec(tweaks.RecNo, "sem GPU dedicada o Windows Update é a forma mais simples de manter drivers atualizados")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(`HKLM\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate`, "ExcludeWUDriversInQualityUpdate", sys.DWord(1))}
			},
		},
	}
}

func uiTweaks() []*tweaks.Tweak {
	return []*tweaks.Tweak{
		{
			ID: "ui.endtask", Category: "interface", Title: "\"Finalizar tarefa\" na barra de tarefas",
			Summary:  "Adiciona \"Finalizar tarefa\" ao clicar com o botão direito em um app na barra de tarefas: encerra um jogo travado sem abrir o Gerenciador de Tarefas.",
			Detail:   "TaskbarEndTask=1 em HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Explorer\\Advanced\\TaskbarDeveloperSettings (Windows 11 23H2 ou mais novo).",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcTaskbarEndTask},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"interface"}, OneClick: true, MinBuild: 22631,
			Recommend: yes("atalho oficial para matar jogos travados"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(`HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced\TaskbarDeveloperSettings`, "TaskbarEndTask", sys.DWord(1))}
			},
		},
		{
			ID: "ui.fileext", Category: "interface", Title: "Mostrar extensões de arquivo",
			Summary:  "Mostra .exe, .pdf, .zip no nome dos arquivos, deixando evidente um \"cheat.pdf.exe\" malicioso.",
			Detail:   "HideFileExt=0 em HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Explorer\\Advanced (a mesma opção de Explorador de Arquivos > Exibir > Extensões de nomes de arquivos). Mostrar a extensão ajuda a perceber arquivos com extensão dupla, como \"foto.jpg.exe\".",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcFileExt},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"seguranca"}, OneClick: true,
			Recommend: yes("proteção simples contra arquivos disfarçados"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(`HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced`, "HideFileExt", sys.DWord(0))}
			},
		},
		{
			ID: "ui.menudelay", Category: "interface", Title: "Menus sem atraso de abertura",
			Summary:  "Submenus do Windows abrem imediatamente ao passar o mouse, em vez de esperar 400 ms.",
			Detail:   "MenuShowDelay=\"0\" em HKCU\\Control Panel\\Desktop (padrão \"400\"). Só afeta menus clássicos do Windows; não muda FPS nem latência em jogo.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcMenuDelay},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootSignOut, Tags: []string{"interface"}, OneClick: true,
			Recommend: neutral("preferência de interface; não afeta jogos"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(`HKCU\Control Panel\Desktop`, "MenuShowDelay", sys.SZ("0"))}
			},
		},
	}
}

func serviceTweaks() []*tweaks.Tweak {
	return []*tweaks.Tweak{
		{
			ID: "services.optional", Category: "servicos", Title: "Serviços dispensáveis em Manual",
			Summary: "Passa para Manual serviços que ficam em execução sem necessidade num PC doméstico (mapas offline, rastreamento de links, fax, demonstração de loja).",
			Detail: "MapsBroker, TrkWks, Fax → Manual; RetailDemo → Desativado. Em Manual o Windows ainda inicia o serviço se algum programa precisar. Ganho pequeno (alguns MB de RAM e menos atividade em segundo plano). " +
				"Não mexemos em serviços do Xbox (quebra Game Pass/controle), Spooler (impressão), IP Helper (chat de voz Xbox) nem Diagnóstico.",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcServices},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"ram"}, OneClick: true,
			Recommend: neutral("ganho pequeno e seguro"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{
					tweaks.Svc("MapsBroker", sys.StartManual, false),
					tweaks.Svc("TrkWks", sys.StartManual, false),
					tweaks.Svc("Fax", sys.StartManual, false),
					tweaks.Svc("RetailDemo", sys.StartDisabled, true),
				}
			},
		},
		{
			ID: "services.sysmain.hdd", Category: "servicos", Title: "SysMain (Superfetch) desligado em HDD",
			Summary:  "Em PCs com Windows num disco rígido mecânico, o SysMain pode deixar o disco em 100% por minutos após o boot.",
			Detail:   "Serviço SysMain → Desativado. Em SSD o SysMain é benéfico (pré-carrega apps que você usa) e fica recomendado manter ligado.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcServices},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"disco"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if d := f.SystemDisk(); d == nil || !strings.EqualFold(d.MediaType, "HDD") {
					return false, "o Windows está em SSD: mantenha o SysMain ligado"
				}
				return true, ""
			},
			Recommend: yes("o Windows está em HDD, onde o SysMain costuma causar disco em 100%"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Svc("SysMain", sys.StartDisabled, true)}
			},
		},
	}
}

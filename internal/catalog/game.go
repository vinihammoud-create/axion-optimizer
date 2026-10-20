package catalog

import (
	"fmt"
	"strings"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

const (
	keyGameBar      = `HKCU\Software\Microsoft\GameBar`
	keyGameDVR      = `HKCU\Software\Microsoft\Windows\CurrentVersion\GameDVR`
	keyGameConfig   = `HKCU\System\GameConfigStore`
	keyGraphicsDrv  = `HKLM\SYSTEM\CurrentControlSet\Control\GraphicsDrivers`
	keyUserGpuPrefs = `HKCU\Software\Microsoft\DirectX\UserGpuPreferences`
	valDXGlobal     = "DirectXUserGlobalSettings"
	keyAudioPrefs   = `HKCU\Software\Microsoft\Multimedia\Audio`
)

// The two Microsoft pages describe the Communications tab and its options;
// the thread reports a known limit of the setting. No source documents the
// registry value 3 itself: it was observed by Axion on Windows 11.
var (
	srcDuckingDefault = tweaks.Source{Title: "Microsoft Learn — Experiência padrão de ducking (aba Comunicações do painel de Som: padrão de 80%, silenciar ou não fazer nada)", URL: "https://learn.microsoft.com/windows/win32/coreaudio/stream-attenuation"}
	srcDuckingOff     = tweaks.Source{Title: "Microsoft Learn — Desativar o ducking padrão (a opção do painel de Som; a transição dura 1 segundo e jogos podem não querer a chamada interferindo no áudio)", URL: "https://learn.microsoft.com/windows/win32/coreaudio/disabling-the-ducking-experience"}
	srcDuckingValue   = tweaks.Source{Title: "Microsoft Q&A — UserDuckingPreference: depois de reiniciar, a tela continuava em \"Não fazer nada\" mas o efeito sumiu; só voltou ao alternar a opção manualmente (Windows 10 LTSC 1809)", URL: "https://learn.microsoft.com/answers/questions/286017/audio-ducking-stream-attenuation-setting-doesnt-ta"}
)

func gameTweaks() []*tweaks.Tweak {
	return []*tweaks.Tweak{
		{
			ID: "game.mode", Category: "jogo", Title: "Modo de Jogo ativado",
			Summary: "Mantém o Modo de Jogo do Windows ligado: ele prioriza o jogo em primeiro plano e impede que o Windows Update instale drivers ou mostre avisos de reinício durante a partida.",
			Detail: "Valor AutoGameModeEnabled em HKCU\\Software\\Microsoft\\GameBar. Quando ausente, o Windows 10 (1703+) e o 11 consideram o Modo de Jogo ligado. " +
				"Nos Ryzen X3D de 2 CCDs ele é obrigatório: o driver da AMD usa a detecção de jogo da Game Bar/Modo de Jogo para estacionar o CCD sem 3D V-Cache.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcGameMode, srcX3D},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"fps", "estabilidade"}, OneClick: true,
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if f.CPU.IsDualCCDX3D {
					return tweaks.Rec(tweaks.RecYes, "obrigatório para o agendamento correto do seu "+f.CPU.Name)
				}
				return tweaks.Rec(tweaks.RecYes, "recurso oficial do Windows sem contrapartida")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(keyGameBar, "AutoGameModeEnabled", sys.DWord(1), sys.Absent())}
			},
		},
		{
			ID: "game.dvr.history", Category: "jogo", Title: "Gravação em segundo plano desligada",
			Summary: "Desliga o \"Registrar o que aconteceu\" da Xbox Game Bar, que grava continuamente em segundo plano e usa o codificador de vídeo da GPU o tempo todo.",
			Detail: "HistoricalCaptureEnabled = 0 em HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\GameDVR. A própria tela de Configurações do Windows avisa que gravar em segundo plano pode afetar o desempenho dos jogos. " +
				"A captura manual (Win+Alt+R) continua funcionando.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcGameDVRBG},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestartApp, Tags: []string{"fps"}, OneClick: true,
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if f.Gaming.HistoricalRecord {
					return tweaks.Rec(tweaks.RecYes, "a gravação em segundo plano está ligada neste PC")
				}
				return tweaks.Rec(tweaks.RecYes, "garante que a gravação contínua fique desligada")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(keyGameDVR, "HistoricalCaptureEnabled", sys.DWord(0), sys.Absent())}
			},
		},
		{
			ID: "game.dvr.capture", Category: "jogo", Title: "Captura da Game Bar desligada",
			Summary: "Desliga o recurso de captura de clipes da Xbox Game Bar (Win+Alt+R). O overlay e o Modo de Jogo continuam funcionando.",
			Detail: "AppCaptureEnabled = 0 (HKCU\\...\\GameDVR) e GameDVR_Enabled = 0 (HKCU\\System\\GameConfigStore). Com a gravação desligada, o custo de desempenho medido é pequeno; " +
				"faça isso se você não grava clipes pela Game Bar (OBS/ShadowPlay não são afetados).",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcGameDVRBG},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestartApp, Tags: []string{"fps"}, OneClick: false,
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if f.CPU.IsDualCCDX3D {
					return tweaks.Rec(tweaks.RecNeutral, "mantido opcional: no seu X3D de 2 CCDs a Game Bar participa da detecção de jogos")
				}
				return tweaks.Rec(tweaks.RecNeutral, "desligue se não usa a Game Bar para gravar clipes")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{
					tweaks.Reg(keyGameDVR, "AppCaptureEnabled", sys.DWord(0)),
					tweaks.Reg(keyGameConfig, "GameDVR_Enabled", sys.DWord(0)),
				}
			},
		},
		{
			ID: "game.ducking", Category: "jogo", Title: "Chamadas não abaixam o volume do jogo",
			Summary: "Impede o Windows de reduzir o volume do jogo e de outros sons quando um programa de chamada, como o Discord, usa o dispositivo de comunicação.",
			Detail: "UserDuckingPreference = 3 em HKCU\\Software\\Microsoft\\Multimedia\\Audio. O valor 3 é o que o Windows grava quando se escolhe \"Não fazer nada\" na aba Comunicações do painel de Som (valor conhecido do painel de Som, não medido pelo Axion; a Microsoft não documenta o valor). Por padrão o Windows reduz o volume dos outros sons em 80% durante uma chamada, com uma transição de 1 segundo. " +
				"É uma preferência de uso e não de desempenho: quem fala no Discord e sente o jogo \"sumir\" no começo da fala ganha; quem usa chamadas de trabalho e quer ouvir só a pessoa deve manter o padrão. " +
				"Limite conhecido: num relato à Microsoft (Windows 10 LTSC 1809), depois de reiniciar a aba continuava em \"Não fazer nada\" mas o volume voltou a cair; só voltou a valer ao trocar a opção e voltar. O autor diz que a troca manual faz mais do que gravar o valor, então gravar só o registro pode não ter efeito. Se acontecer, faça essa troca na aba Comunicações (o Axion não consegue dar esse toque). O efeito não foi medido pelo Axion. " +
				"Quem dispara a redução são os fluxos de áudio de comunicação (chamadas de voz e de vídeo), segundo a documentação da Microsoft.",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcDuckingDefault, srcDuckingOff, srcDuckingValue},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootSignOut, Tags: []string{"audio"}, OneClick: false,
			Recommend: neutral("opcional: ligue se o volume do jogo cai quando alguém fala no Discord ou em outro programa de chamada"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(keyAudioPrefs, "UserDuckingPreference", sys.DWord(3))}
			},
		},
		{
			ID: "game.hags", Category: "jogo", Title: "Agendamento de GPU acelerado por hardware (HAGS)",
			Summary: "Deixa a própria GPU gerenciar sua fila de trabalho, reduzindo o trabalho da CPU no envio de quadros. Requisito do DLSS Frame Generation.",
			Detail: "HwSchMode = 2 em HKLM\\SYSTEM\\CurrentControlSet\\Control\\GraphicsDrivers (1 = desligado). Só tem efeito com driver WDDM 2.7+ que suporte o recurso (NVIDIA GTX 10 em diante, AMD RDNA, Intel Arc). " +
				"O efeito em FPS medido por terceiros é pequeno e varia por jogo; o benefício claro é habilitar o Frame Generation nas RTX 40/50.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcHAGS, srcNvidiaFG},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"latencia", "fps"}, OneClick: true,
			MinBuild: 19041,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !discreteGPU(f) {
					return false, "requer GPU dedicada NVIDIA, AMD ou Intel Arc"
				}
				if f.Gaming.HAGSState == "unsupported" {
					return false, "o driver da sua GPU não oferece o agendamento acelerado por hardware"
				}
				return true, ""
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if nvidiaFGCapable(f) {
					return tweaks.Rec(tweaks.RecYes, "sua RTX suporta Frame Generation, que exige HAGS")
				}
				return tweaks.Rec(tweaks.RecNeutral, "efeito pequeno e variável na sua GPU; compare no seu jogo antes de manter")
			},
			Ops: func(f *facts.Facts) []tweaks.Op {
				// With the value absent the driver may already run HAGS by
				// default (as the graphics kernel reports): that counts as on.
				if f != nil && f.Gaming.HAGSState == "on" {
					return []tweaks.Op{tweaks.Reg(keyGraphicsDrv, "HwSchMode", sys.DWord(2), sys.Absent())}
				}
				return []tweaks.Op{tweaks.Reg(keyGraphicsDrv, "HwSchMode", sys.DWord(2))}
			},
		},
		{
			ID: "game.windowed", Category: "jogo", Title: "Otimizações para jogos em janela (Windows 11)",
			Summary: "Faz jogos DirectX 10/11 em janela ou tela cheia sem bordas usarem o flip model: menor latência e suporte a VRR (G-SYNC/FreeSync) e Auto HDR.",
			Detail: "Acrescenta SwapEffectUpgradeEnable=1; ao valor DirectXUserGlobalSettings em HKCU\\Software\\Microsoft\\DirectX\\UserGpuPreferences, preservando as outras opções que já estiverem nesse valor. " +
				"É a mesma chave do botão em Configurações > Sistema > Tela > Elementos gráficos.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcWindowedOpt, srcFlipModel},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestartApp, Tags: []string{"latencia"}, OneClick: true,
			MinBuild:  22000,
			Recommend: yes("recurso oficial do Windows 11 para jogos em janela/sem bordas"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{dxGlobalOp("SwapEffectUpgradeEnable", "1")}
			},
		},
		{
			ID: "game.hpet.fix", Category: "correcoes", Title: "Correção: remover HPET forçado no boot",
			Summary: "Remove o parâmetro useplatformclock da inicialização, que alguns \"otimizadores\" gravam. Ele força o HPET como relógio do sistema e deixa cada leitura de tempo mais lenta.",
			Detail: "Equivale a \"bcdedit /deletevalue {current} useplatformclock\" (o padrão do Windows é não ter esse parâmetro). A Microsoft documenta que o TSC é a fonte preferida e que o HPET tem custo de acesso muito maior. " +
				"Só aparece quando o parâmetro está presente neste PC.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcQPC, srcBCDEdit},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"latencia", "estabilidade"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Boot.Readable {
					return false, "não foi possível ler a configuração de boot"
				}
				if !f.Boot.On(f.Boot.UsePlatformClock) {
					if f.Boot.UsePlatformClock != "" {
						return false, "o parâmetro está como \"" + f.Boot.UsePlatformClock + "\", que equivale ao padrão"
					}
					return false, "o parâmetro não está presente (padrão correto)"
				}
				return true, ""
			},
			Recommend: yes("useplatformclock está forçado neste PC"),
			Ops:       func(*facts.Facts) []tweaks.Op { return []tweaks.Op{bcdOp("useplatformclock", "")} },
		},
		{
			ID: "game.boottick.fix", Category: "correcoes", Title: "Correção: restaurar o tick do sistema padrão",
			Summary:  "Remove os parâmetros disabledynamictick/useplatformtick da inicialização, gravados por \"otimizadores\" sem evidência de ganho e que aumentam o consumo de energia.",
			Detail:   "bcdedit /deletevalue {current} disabledynamictick e useplatformtick. Com o tick dinâmico o Windows só acorda o processador quando há trabalho; forçar o tick periódico não tem ganho medido em jogos.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcBCDEdit, srcQPC},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"estabilidade"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Boot.Readable {
					return false, "não foi possível ler a configuração de boot"
				}
				if !f.Boot.On(f.Boot.DisableDynamicTick) && !f.Boot.On(f.Boot.UsePlatformTick) {
					if f.Boot.DisableDynamicTick != "" || f.Boot.UsePlatformTick != "" {
						// "No" is what an undo of these tweaks leaves behind: same behavior as the default.
						return false, "os parâmetros estão como \"No\", que equivale ao padrão"
					}
					return false, "os parâmetros não estão presentes (padrão correto)"
				}
				return true, ""
			},
			Recommend: yes("parâmetros de boot não padrão encontrados neste PC"),
			Ops: func(f *facts.Facts) []tweaks.Op {
				var ops []tweaks.Op
				if f.Boot.On(f.Boot.DisableDynamicTick) {
					ops = append(ops, bcdOp("disabledynamictick", ""))
				}
				if f.Boot.On(f.Boot.UsePlatformTick) {
					ops = append(ops, bcdOp("useplatformtick", ""))
				}
				return ops
			},
		},
		{
			ID: "game.refresh", Category: "jogo", Title: "Monitor na taxa de atualização máxima",
			Summary:  "Coloca cada monitor na maior frequência (Hz) disponível na resolução atual, como em Configurações > Tela > Avançado; útil quando o Windows ficou em 60 Hz num monitor mais rápido.",
			Detail:   "ChangeDisplaySettingsEx com a maior frequência listada pelo driver para a resolução atual (a mesma lista de Configurações > Tela > Avançado). Para desfazer, a frequência anterior é restaurada.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcRefresh},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"fps", "latencia"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				for _, d := range f.Displays {
					if d.MaxHzAtRes > d.CurrentHz && d.CurrentHz > 0 {
						return true, ""
					}
				}
				return false, "todos os monitores já estão na frequência máxima"
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				for _, d := range f.Displays {
					// 59 x 60 Hz is the driver rounding 59.94 Hz: not a gain.
					if d.CurrentHz > 0 && d.MaxHzAtRes >= d.CurrentHz+5 && float64(d.MaxHzAtRes) >= float64(d.CurrentHz)*1.04 {
						return tweaks.Rec(tweaks.RecYes, fmt.Sprintf("%s está a %d Hz, mas suporta %d Hz", d.Monitor, d.CurrentHz, d.MaxHzAtRes))
					}
				}
				return tweaks.Rec(tweaks.RecNeutral, "diferença de poucos Hz (arredondamento de 59,94/60 Hz e similares): sem ganho perceptível")
			},
			Ops: func(f *facts.Facts) []tweaks.Op {
				var ops []tweaks.Op
				for _, d := range f.Displays {
					if d.MaxHzAtRes > d.CurrentHz && d.CurrentHz > 0 {
						ops = append(ops, refreshOp(d.Device, d.Monitor))
					}
				}
				return ops
			},
		},
	}
}

var srcRefresh = tweaks.Source{Title: "Microsoft Support — Alterar a taxa de atualização do monitor no Windows", URL: "https://support.microsoft.com/windows/change-your-display-refresh-rate-in-windows-c8ea729e-0678-015c-c415-f806f04aae5a"}

// ---------------------------------------------------------------------------
// DirectXUserGlobalSettings merge op
// ---------------------------------------------------------------------------

// parseDXSettings parses "A=1;B=2;" preserving order.
func parseDXSettings(s string) (keys []string, vals map[string]string) {
	vals = map[string]string{}
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if _, seen := vals[strings.ToLower(k)]; !seen {
			keys = append(keys, k)
		}
		vals[strings.ToLower(k)] = strings.TrimSpace(v)
	}
	return
}

func formatDXSettings(keys []string, vals map[string]string) string {
	var sb strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&sb, "%s=%s;", k, vals[strings.ToLower(k)])
	}
	return sb.String()
}

func dxGlobalOp(name, value string) tweaks.Op {
	return dxPrefOp(keyUserGpuPrefs, valDXGlobal, name, value)
}

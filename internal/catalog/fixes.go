package catalog

import (
	"fmt"
	"strings"
	"time"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// Corrections undo settings left by other "optimizers" and debloat scripts
// that have a documented negative effect. Each one only appears when the
// problem was found on this PC and restores the Windows default.

var (
	srcBitLockerBCD = tweaks.Source{Title: "Microsoft Learn — Configurações de BCD e BitLocker (testsigning, nointegritychecks e nx estão no perfil de validação padrão)", URL: "https://learn.microsoft.com/windows/security/operating-system-security/data-protection/bitlocker/bcd-settings-and-bitlocker"}
	srcTestSigning  = tweaks.Source{Title: "Microsoft Learn — Opção de boot TESTSIGNING", URL: "https://learn.microsoft.com/windows-hardware/drivers/install/the-testsigning-boot-configuration-option"}
	srcEACDSE       = tweaks.Source{Title: "Behaviour Interactive (suporte) — Erro de imposição de driver do EasyAntiCheat: TESTSIGNING/NOINTEGRITYCHECKS", URL: "https://support.deadbydaylight.com/hc/en-us/articles/360000487863-EasyAntiCheat-EAC-Driver-Enforcement-Error"}
	srcDEP          = tweaks.Source{Title: "Microsoft Learn — Prevenção de Execução de Dados (DEP)", URL: "https://learn.microsoft.com/windows/win32/memory/data-execution-prevention"}
	srcUAC          = tweaks.Source{Title: "Microsoft Learn — Configurações do Controle de Conta de Usuário", URL: "https://learn.microsoft.com/windows/security/application-security/application-control/user-account-control/settings-and-configuration"}
	srcTCPFeatures  = tweaks.Source{Title: "Microsoft Learn — Descrição dos recursos TCP do Windows (Tcp1323Opts)", URL: "https://learn.microsoft.com/troubleshoot/windows-server/networking/description-tcp-features"}
	srcTaskOffload  = tweaks.Source{Title: "Microsoft Learn — Valores de registro para habilitar/desabilitar task offload", URL: "https://learn.microsoft.com/windows-hardware/drivers/network/using-registry-values-to-enable-and-disable-task-offloading"}
	srcTDR          = tweaks.Source{Title: "Microsoft Learn — Chaves de registro do TDR (somente para teste/depuração)", URL: "https://learn.microsoft.com/windows-hardware/drivers/display/tdr-registry-keys"}
	srcLargeCache   = tweaks.Source{Title: "Microsoft Learn — LargeSystemCache (servidor x estação de trabalho)", URL: "https://learn.microsoft.com/previous-versions/windows/it-pro/windows-server-2003/cc784562(v=ws.10)"}
	srcPageFile     = tweaks.Source{Title: "Microsoft Learn — Introdução ao arquivo de paginação (limite de confirmação)", URL: "https://learn.microsoft.com/troubleshoot/windows-client/performance/introduction-to-the-page-file"}
	srcPageFileSz   = tweaks.Source{Title: "Microsoft Learn — Como determinar o tamanho adequado do arquivo de paginação", URL: "https://learn.microsoft.com/troubleshoot/windows-client/performance/how-to-determine-the-appropriate-page-file-size-for-64-bit-versions-of-windows"}
	srcNVMeServer   = tweaks.Source{Title: "Microsoft Tech Community — Native NVMe no Windows Server 2025 (anúncio oficial)", URL: "https://techcommunity.microsoft.com/blog/windowsservernewsandbestpractices/announcing-native-nvme-in-windows-server-2025-ushering-in-a-new-era-of-storage-p/4477353"}
	srcNVMeBlocked  = tweaks.Source{Title: "WinBuzzer (24/03/2026) — Microsoft bloqueia o ajuste de Registro do driver NVMe nativo no Windows 11", URL: "https://winbuzzer.com/2026/03/24/microsoft-blocks-registry-hack-faster-nvme-driver-windows-11-xcxwbn/"}
	srcFsutilTrim   = tweaks.Source{Title: "Microsoft Learn — fsutil behavior: disabledeletenotify (TRIM ligado por padrão no NTFS, sem reiniciar)", URL: "https://learn.microsoft.com/windows-server/administration/windows-commands/fsutil-behavior"}
	srcDefragCmd    = tweaks.Source{Title: "Microsoft Learn — defrag: /L faz retrim; a tarefa agendada faz retrim do SSD uma vez por mês", URL: "https://learn.microsoft.com/windows-server/administration/windows-commands/defrag"}
	srcSSDStory     = tweaks.Source{Title: "Scott Hanselman (Microsoft), com a equipe de armazenamento do Windows — o Windows desfragmenta o SSD? (retrim e limite de fragmentação)", URL: "https://www.hanselman.com/blog/the-real-and-complete-story-does-windows-defragment-your-ssd"}
	srcWUSettings   = tweaks.Source{Title: "Microsoft Learn — Gerenciar configurações adicionais do Windows Update", URL: "https://learn.microsoft.com/windows/deployment/update/waas-wu-settings"}
)

const (
	keyUACPolicy = `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`
	keyTcpParams = `HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters`
	keyMemMgmt   = `HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Memory Management`
)

func bootReadable(f *facts.Facts) (bool, string) {
	if !f.Boot.Readable {
		return false, "não foi possível ler a configuração de boot"
	}
	return true, ""
}

func anyAntiCheat(f *facts.Facts) bool { return len(f.Gaming.AntiCheats) > 0 }

func servicesOf(f *facts.Facts, group string) []facts.DisabledService {
	var out []facts.DisabledService
	for _, s := range f.Residue.DisabledServices {
		if s.Group == group {
			out = append(out, s)
		}
	}
	return out
}

func serviceNames(list []facts.DisabledService) string {
	var n []string
	for _, s := range list {
		n = append(n, s.Display)
	}
	return strings.Join(n, ", ")
}

// bitlockerNote explains the BitLocker handling of boot-configuration fixes.
const bitlockerNote = "Com o BitLocker ligado sem o Secure Boot validando o boot, essa mudança faria o Windows pedir a chave de recuperação no próximo reinício; por isso o Axion suspende a proteção do BitLocker só até esse reinício (ela volta sozinha)."

// bitlockerSuspend suspends BitLocker on the Windows drive until the next
// restart after a change (or undo) of testsigning, nointegritychecks or nx:
// these are in BitLocker's default boot validation profile, used when Secure
// Boot does not validate the boot, and a change there asks for the recovery
// key at boot. Suspending for one restart is what Microsoft documents for
// planned boot changes; protection resumes by itself.
func bitlockerSuspend(c *tweaks.Ctx, _ bool) error {
	if c.F == nil || c.F.Security.BitLockerOS != "on" {
		return nil
	}
	drive := c.F.OS.SystemDrive
	if drive == "" {
		drive = "C:"
	}
	res, err := c.P.Run.Run(c.Context, "manage-bde.exe", []string{"-protectors", "-disable", drive, "-RebootCount", "1"}, sys.RunOpts{Timeout: time.Minute})
	if err == nil && res.ExitCode != 0 {
		err = fmt.Errorf("manage-bde terminou com o código %d", res.ExitCode)
	}
	if err != nil {
		return fmt.Errorf("o BitLocker está ligado e não pôde ser suspenso até o próximo reinício (%v): antes de reiniciar, suspenda em Painel de Controle > Criptografia de Unidade de Disco BitLocker > Suspender proteção, ou tenha a chave de recuperação à mão", err)
	}
	if c.Log != nil {
		c.Log("BitLocker suspenso até o próximo reinício (a mudança no boot não vai pedir a chave de recuperação)")
	}
	return nil
}

// bitlockerGuard keeps a boot-configuration fix out of the 1-click plans
// when the BitLocker state could not be read and Secure Boot does not
// validate the boot: Axion would not know whether to suspend BitLocker, and
// a change there could ask for the recovery key at the next restart.
func bitlockerGuard(rec func(*facts.Facts) tweaks.Recommendation) func(*facts.Facts) tweaks.Recommendation {
	return func(f *facts.Facts) tweaks.Recommendation {
		if f.Security.BitLockerOS == "unknown" && f.Security.SecureBoot != "on" {
			return tweaks.Rec(tweaks.RecNeutral, "o Axion não conseguiu ler o estado do BitLocker e o Secure Boot não está validando o boot: se o BitLocker estiver ligado, o Windows pode pedir a chave de recuperação no próximo reinício. Tenha a chave à mão (conta Microsoft > Dispositivos) antes de aplicar")
		}
		return rec(f)
	}
}

func fixTweaks() []*tweaks.Tweak {
	return []*tweaks.Tweak{
		{
			ID: "fix.bcd.testsigning", Category: "correcoes", Title: "Desligar o modo de teste de drivers (TESTSIGNING)",
			Summary: "O Windows está em \"modo de teste\", aceitando drivers sem assinatura válida. Anti-cheats como EasyAntiCheat, Vanguard e FACEIT se recusam a rodar assim.",
			Detail: "Equivale a \"bcdedit /deletevalue {current} testsigning\" (padrão do Windows). O modo de teste costuma ser deixado por ferramentas que instalam drivers não assinados. " +
				"Depois de reiniciar, drivers assinados apenas com certificado de teste deixam de carregar; se algum programa seu depende disso, ele vai avisar. " + bitlockerNote,
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcTestSigning, srcEACDSE, srcBitLockerBCD},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"anticheat", "seguranca"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if ok, why := bootReadable(f); !ok {
					return false, why
				}
				if !f.Boot.On(f.Boot.TestSigning) {
					return false, "o modo de teste está desligado (padrão correto)"
				}
				return true, ""
			},
			Recommend: bitlockerGuard(func(f *facts.Facts) tweaks.Recommendation {
				if anyAntiCheat(f) {
					return tweaks.Rec(tweaks.RecYes, "você tem anti-cheat instalado ("+strings.Join(f.Gaming.AntiCheats, ", ")+"), que não roda em modo de teste")
				}
				return tweaks.Rec(tweaks.RecYes, "modo de teste reduz a proteção contra drivers maliciosos")
			}),
			Ops:  func(*facts.Facts) []tweaks.Op { return []tweaks.Op{bcdOp("testsigning", "")} },
			Post: bitlockerSuspend,
		},
		{
			ID: "fix.bcd.nointegrity", Category: "correcoes", Title: "Remover o parâmetro nointegritychecks do boot",
			Summary:  "O parâmetro nointegritychecks está marcado no boot. Segundo a Microsoft ele é ignorado desde o Windows 7/8 (não desliga a verificação de drivers), mas o EasyAntiCheat pede para removê-lo quando dá erro de imposição de driver.",
			Detail:   "Equivale a \"bcdedit /deletevalue {current} nointegritychecks\" (padrão do Windows). Não é possível marcá-lo com o Secure Boot ligado. " + bitlockerNote,
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcBCDEdit, srcEACDSE, srcBitLockerBCD},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"anticheat", "seguranca"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if ok, why := bootReadable(f); !ok {
					return false, why
				}
				if !f.Boot.On(f.Boot.NoIntegrityChecks) {
					return false, "a verificação de integridade está ligada (padrão correto)"
				}
				return true, ""
			},
			Recommend: bitlockerGuard(yes("parâmetro de boot que anti-cheats rejeitam, sem nenhum uso no Windows atual")),
			Ops:       func(*facts.Facts) []tweaks.Op { return []tweaks.Op{bcdOp("nointegritychecks", "")} },
			Post:      bitlockerSuspend,
		},
		{
			ID: "fix.bcd.nx", Category: "correcoes", Title: "Religar a Prevenção de Execução de Dados (DEP)",
			Summary:  "O DEP está forçado como desligado (nx AlwaysOff). É uma proteção básica de memória exigida por anti-cheats e por muitos programas.",
			Detail:   "Equivale a \"bcdedit /set {current} nx OptIn\", o padrão das edições cliente do Windows. Com AlwaysOff nenhuma proteção DEP vale, nem para componentes do Windows. " + bitlockerNote,
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcDEP, srcBCDEdit, srcBitLockerBCD},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"anticheat", "seguranca"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if ok, why := bootReadable(f); !ok {
					return false, why
				}
				if !strings.EqualFold(strings.TrimSpace(f.Boot.NX), "AlwaysOff") {
					return false, "o DEP não está desligado"
				}
				return true, ""
			},
			Recommend: bitlockerGuard(yes("DEP desligado para todo o sistema")),
			Ops:       func(*facts.Facts) []tweaks.Op { return []tweaks.Op{bcdOp("nx", "OptIn")} },
			Post:      bitlockerSuspend,
		},
		{
			ID: "fix.uac", Category: "correcoes", Title: "Religar o Controle de Conta de Usuário (UAC)",
			Summary: "O UAC foi desligado (EnableLUA = 0): todo programa roda com poder de administrador e apps da Microsoft Store (inclusive o app Xbox) não abrem.",
			Detail: "Restaura EnableLUA = 1 em HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Policies\\System, o padrão. Desligar o UAC não melhora desempenho em jogos; só remove a separação entre usuário e administrador. " +
				"Você voltará a ver a confirmação ao instalar programas.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcUAC},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"seguranca"}, OneClick: false,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Residue.UACOff {
					return false, "o UAC está ligado (padrão correto)"
				}
				return true, ""
			},
			Recommend: yes("UAC desligado não traz ganho de desempenho e abre uma brecha de segurança"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(keyUACPolicy, "EnableLUA", sys.DWord(1))}
			},
		},
		{
			ID: "fix.tcp.windowscaling", Category: "correcoes", Title: "Restaurar o escalonamento de janela TCP",
			Summary:  "Tcp1323Opts foi alterado e desligou o escalonamento de janela TCP: downloads (Steam, atualizações) ficam limitados. Não afeta o ping.",
			Detail:   "Remove o valor Tcp1323Opts de HKLM\\SYSTEM\\CurrentControlSet\\Services\\Tcpip\\Parameters, voltando ao padrão do Windows. Sem window scaling a janela TCP fica limitada a 64 KB, o que corta a velocidade em conexões rápidas ou com ping alto.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcTCPFeatures},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"rede"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Residue.WindowScalingOff() {
					return false, "o escalonamento de janela TCP está no padrão"
				}
				return true, ""
			},
			Recommend: yes("valor não padrão encontrado neste PC"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(keyTcpParams, "Tcp1323Opts", sys.Absent())}
			},
		},
		{
			ID: "fix.tcp.taskoffload", Category: "correcoes", Title: "Religar os offloads de rede (DisableTaskOffload)",
			Summary:  "Um otimizador desligou todos os offloads de rede (checksum, LSO): a CPU passa a fazer esse trabalho no lugar da placa de rede, sem ganho de latência.",
			Detail:   "Remove DisableTaskOffload de HKLM\\SYSTEM\\CurrentControlSet\\Services\\Tcpip\\Parameters. A Microsoft documenta esse valor como chave para depurar drivers de rede; o padrão é deixar o driver decidir.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcTaskOffload},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"rede"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Residue.TaskOffloadOff {
					return false, "offloads no padrão"
				}
				return true, ""
			},
			Recommend: yes("DisableTaskOffload = 1 encontrado neste PC"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(keyTcpParams, "DisableTaskOffload", sys.Absent())}
			},
		},
		{
			ID: "fix.gpu.tdr", Category: "correcoes", Title: "Religar a recuperação do driver de vídeo (TDR)",
			Summary:  "A detecção de travamento da GPU foi desligada (TdrLevel = 0): se o driver travar, a tela congela até reiniciar o PC em vez de o Windows recuperar o vídeo.",
			Detail:   "Remove TdrLevel de HKLM\\SYSTEM\\CurrentControlSet\\Control\\GraphicsDrivers (padrão: recuperar). A Microsoft avisa que as chaves de TDR são só para teste e depuração de drivers.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcTDR},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"estabilidade"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Residue.TdrOff() {
					return false, "detecção de travamento da GPU no padrão"
				}
				return true, ""
			},
			Recommend: yes("TdrLevel = 0 encontrado neste PC"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(`HKLM\SYSTEM\CurrentControlSet\Control\GraphicsDrivers`, "TdrLevel", sys.Absent())}
			},
		},
		{
			ID: "fix.mem.largecache", Category: "correcoes", Title: "Desligar o cache de sistema de servidor (LargeSystemCache)",
			Summary:  "LargeSystemCache = 1 é a configuração de servidor de arquivos: o Windows passa a favorecer o cache de arquivos em vez da memória dos programas e jogos.",
			Detail:   "Restaura LargeSystemCache = 0, o padrão das edições cliente. Otimizadores antigos ligam esse valor sem medição de ganho em jogos.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcLargeCache},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"memoria"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Residue.LargeSystemCache {
					return false, "cache de sistema no padrão"
				}
				return true, ""
			},
			Recommend: yes("valor de servidor encontrado em um PC de jogos"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(keyMemMgmt, "LargeSystemCache", sys.DWord(0))}
			},
		},
		{
			ID: "fix.pagefile", Category: "correcoes", Title: "Arquivo de paginação gerenciado pelo Windows",
			Summary: "Volta o arquivo de paginação para \"gerenciar automaticamente\". Sem ele (ou com tamanho fixo pequeno) jogos podem fechar por falta de memória mesmo com RAM livre.",
			Detail: "Grava PagingFiles = \"?:\\pagefile.sys\" (a opção \"Gerenciar automaticamente o tamanho do arquivo de paginação de todas as unidades\"). " +
				"O Windows reserva memória virtual (commit) para cada programa; quando o total chega ao limite (RAM + arquivo de paginação) programas travam ou fecham. Remover o arquivo de paginação não deixa o PC mais rápido.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcPageFile, srcPageFileSz},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"memoria", "estabilidade"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				switch f.Residue.PageFile {
				case "none", "custom":
					return true, ""
				case "":
					return false, "configuração não lida"
				}
				return false, "o arquivo de paginação já é gerenciado pelo Windows"
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if f.Residue.PageFile == "none" {
					return tweaks.Rec(tweaks.RecYes, "este PC está sem arquivo de paginação")
				}
				if f.Residue.PageFileMaxMB > 0 && f.Residue.PageFileMaxMB < 8192 {
					return tweaks.Rec(tweaks.RecYes, fmt.Sprintf("tamanho fixo de %d MB é pequeno para jogos atuais", f.Residue.PageFileMaxMB))
				}
				return tweaks.Rec(tweaks.RecNeutral, "tamanho personalizado; o gerenciamento automático é o recomendado pela Microsoft")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(keyMemMgmt, "PagingFiles", sys.MultiSZ(`?:\pagefile.sys`))}
			},
		},
		{
			ID: "fix.services.system", Category: "correcoes", Title: "Reativar serviços essenciais do Windows",
			Summary:  "Serviços que o Windows precisa (atualizações, áudio, rede, firewall, WMI) foram desativados por um script de \"otimização\". Eles voltam para a inicialização padrão.",
			Detail:   "Cada serviço volta ao tipo de inicialização de uma instalação limpa do Windows 11 (Manual para os que o Windows inicia sob demanda: em Manual ficam parados até algo precisar deles). Desativados, quebram atualizações, áudio, rede ou firewall.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcServices},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"estabilidade", "seguranca"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if len(servicesOf(f, "system")) == 0 {
					return false, "nenhum serviço essencial desativado"
				}
				return true, ""
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				return tweaks.Rec(tweaks.RecYes, "desativados: "+serviceNames(servicesOf(f, "system")))
			},
			Ops: func(f *facts.Facts) []tweaks.Op {
				var ops []tweaks.Op
				for _, s := range servicesOf(f, "system") {
					ops = append(ops, tweaks.Svc(s.Name, s.Default, false))
				}
				return ops
			},
		},
		{
			ID: "fix.services.apps", Category: "correcoes", Title: "Reativar serviços opcionais (Xbox, Store, Bluetooth, relatórios de erro…)",
			Summary:  "Serviços usados pelo app Xbox/Game Pass (inclusive o salvamento na nuvem), Microsoft Store, Bluetooth, câmera/microfone, teclado virtual e caneta, teclas especiais e relatórios de erro foram desativados. Voltam para Manual (iniciam só quando usados).",
			Detail:   "Voltam ao tipo de inicialização de uma instalação limpa do Windows 11; em Manual ficam parados até algo precisar deles. Desativados, causam erros difíceis de diagnosticar (login do Xbox, Discord sem microfone, controle sem atualização, travamentos sem relatório).",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcServices},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"estabilidade"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if len(servicesOf(f, "apps")) == 0 {
					return false, "nenhum desses serviços desativado"
				}
				return true, ""
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				return tweaks.Rec(tweaks.RecYes, "desativados: "+serviceNames(servicesOf(f, "apps")))
			},
			Ops: func(f *facts.Facts) []tweaks.Op {
				var ops []tweaks.Op
				for _, s := range servicesOf(f, "apps") {
					ops = append(ops, tweaks.Svc(s.Name, s.Default, false))
				}
				return ops
			},
		},
		{
			ID: "fix.update.policies", Category: "correcoes", Title: "Desbloquear o Windows Update",
			Summary: "Políticas ou uma pausa \"infinita\" bloqueiam as atualizações do Windows. Sem elas o PC fica sem correções de segurança e sem drivers e recursos novos.",
			Detail: "Remove apenas os valores de bloqueio encontrados (NoAutoUpdate, DisableWindowsUpdateAccess, servidor WSUS falso, pausa até datas distantes). " +
				"Para controlar atualizações sem bloquear, use \"Pausar\" nas Configurações ou a opção de não receber drivers pelo Windows Update.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcWUSettings, srcWUDrivers},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"seguranca"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if len(f.Residue.UpdateBlocks) == 0 {
					return false, "o Windows Update não está bloqueado"
				}
				return true, ""
			},
			Recommend: yes("bloqueio do Windows Update encontrado neste PC"),
			Ops: func(f *facts.Facts) []tweaks.Op {
				var ops []tweaks.Op
				for _, b := range f.Residue.UpdateBlocks {
					ops = append(ops, tweaks.Reg(b.Path, b.Name, sys.Absent()))
				}
				return ops
			},
		},
		{
			ID: "fix.trim.on", Category: "correcoes", Title: "Religar o TRIM do SSD",
			Summary: "O TRIM foi desligado (DisableDeleteNotify = 1): o SSD deixa de saber o que foi apagado, as gravações ficam mais lentas com o tempo e o desgaste aumenta.",
			Detail: "Equivale a \"fsutil behavior set disabledeletenotify 0\", o padrão do NTFS. Vale na hora, sem reiniciar. " +
				"Depois de religar, faça um retrim (Limpeza > Manutenção do SSD) para o SSD ficar sabendo de todo o espaço liberado enquanto o TRIM esteve desligado. Desfazer volta ao valor anterior.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcFsutilTrim, srcSSDStory},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"armazenamento", "ssd"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Trim.Readable {
					return false, "não foi possível ler a configuração de TRIM"
				}
				if f.Trim.NTFS != 1 {
					return false, "o TRIM está ligado (padrão do Windows)"
				}
				return true, ""
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if !f.HasSSD() {
					return tweaks.Rec(tweaks.RecNeutral, "nenhum SSD encontrado neste PC; em HD o TRIM não é usado")
				}
				return tweaks.Rec(tweaks.RecYes, "TRIM desligado com SSD neste PC")
			},
			Ops: func(*facts.Facts) []tweaks.Op { return []tweaks.Op{trimOnOp()} },
		},
		{
			ID: "fix.defrag.task", Category: "correcoes", Title: "Reativar a otimização automática de unidades",
			Summary: "A tarefa do Windows que faz o retrim mensal do SSD (e desfragmenta HDs) foi desativada, em geral por um script que achava que ela \"desfragmenta o SSD\".",
			Detail: "Reativa " + sys.DefragTask + ". Em SSD ela faz retrim uma vez por mês (reenvia o TRIM de todo o espaço livre, cobrindo avisos que se perdem sob carga) e, segundo a equipe de armazenamento do Windows (2014), só desfragmenta o SSD quando há instantâneos de volume (Restauração do Sistema), " +
				"porque a cópia-na-gravação fica lenta num volume muito fragmentado. Roda na manutenção automática, com o PC ocioso. Desfazer desativa de novo.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcDefragCmd, srcSSDStory},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"armazenamento", "ssd"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				switch {
				case !f.Trim.TaskRead:
					return false, "não foi possível ler a tarefa de otimização"
				case !f.Trim.TaskExists:
					return false, "a tarefa não existe neste Windows (foi removida); use o retrim manual da página Limpeza"
				case f.Trim.TaskOn:
					return false, "a otimização automática está ativa (padrão do Windows)"
				}
				return true, ""
			},
			Recommend: yes("tarefa de otimização desativada neste PC"),
			Ops:       func(*facts.Facts) []tweaks.Op { return []tweaks.Op{taskEnableOp(sys.DefragTask)} },
		},
		{
			ID: "fix.nvme.override", Category: "correcoes", Title: "Remover o ajuste não suportado do driver NVMe nativo",
			Summary: "Outro programa gravou os valores que forçam o driver NVMe do Windows Server (nvmedisk.sys) no Windows 11, configuração que a Microsoft não suporta (em março de 2026 as builds Insider deixaram de aceitá-la).",
			Detail: "Remove os valores " + strings.Join(facts.NVMeOverrideIDs, ", ") + " de HKLM\\SYSTEM\\CurrentControlSet\\Policies\\Microsoft\\FeatureManagement\\Overrides; no próximo reinício o SSD volta ao driver padrão (stornvme). " +
				"Com o driver de servidor, ferramentas como Samsung Magician e WD Dashboard podem não reconhecer o SSD, e a troca de driver já causou pedidos da chave de recuperação do BitLocker.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcNVMeServer, srcNVMeBlocked, srcNVMeHot},
			Risk: tweaks.RiskModerate, RiskNote: "com o BitLocker ligado, tenha a chave de recuperação à mão: a troca de driver pode pedi-la no próximo boot",
			Reboot: tweaks.RebootRestart, Tags: []string{"armazenamento", "estabilidade"}, OneClick: false,
			Applicable: func(f *facts.Facts) (bool, string) {
				if len(f.Residue.NVMeOverrides) == 0 {
					return false, "nenhum ajuste do driver NVMe nativo encontrado"
				}
				return true, ""
			},
			Recommend: yes("ajuste não suportado no Windows 11 encontrado neste PC"),
			Ops: func(f *facts.Facts) []tweaks.Op {
				var ops []tweaks.Op
				for _, id := range f.Residue.NVMeOverrides {
					ops = append(ops, tweaks.Reg(facts.KeyFeatureOverrides, id, sys.Absent()))
				}
				return ops
			},
		},
		{
			ID: "fix.spectre", Category: "correcoes", Title: "Religar as mitigações Spectre/Meltdown",
			Summary: "As proteções contra ataques de execução especulativa foram desligadas por outro programa. Em CPUs atuais o custo delas em jogos é pequeno.",
			Detail: "Remove FeatureSettingsOverride e FeatureSettingsOverrideMask de Memory Management, voltando ao padrão da Microsoft (KB4073119). " +
				"Se você desligou de propósito numa CPU Intel antiga, veja a opção equivalente na categoria Avançado.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcSpectre, srcSpectrePerf},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"seguranca"}, OneClick: false, Group: "spectre",
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Residue.SpectreOverride {
					return false, "mitigações no padrão"
				}
				return true, ""
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if f.CPU.Vendor == "Intel" && f.CPU.IntelGen > 0 && f.CPU.IntelGen <= 4 {
					return tweaks.Rec(tweaks.RecNeutral, "em CPUs Intel de 4ª geração (Haswell) ou anteriores o custo das mitigações é maior; decisão sua")
				}
				return tweaks.Rec(tweaks.RecYes, "na sua CPU o ganho de desligar é mínimo e o risco continua")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{
					tweaks.Reg(keyMemMgmt, "FeatureSettingsOverride", sys.Absent()),
					tweaks.Reg(keyMemMgmt, "FeatureSettingsOverrideMask", sys.Absent()),
				}
			},
		},
		{
			ID: "fix.a11y.filterkeys", Category: "correcoes", Title: "Desligar as Teclas de Filtro",
			Summary:  "As Teclas de Filtro estão ligadas: o Windows ignora toques curtos ou repetições rápidas da mesma tecla, o que atrapalha counter-strafe e troca de armas.",
			Detail:   "Remove apenas o bit FKF_FILTERKEYSON (0x1) de Flags em HKCU\\Control Panel\\Accessibility\\Keyboard Response; o tempo configurado e as outras preferências continuam lá se você quiser religar. Vale ao sair e entrar de novo na conta (ou desligue agora em Configurações > Acessibilidade > Teclado).",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcFilterKeys},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootSignOut, Tags: []string{"entrada"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Peripherals.FilterKeysOn {
					return false, "Teclas de Filtro desligadas"
				}
				return true, ""
			},
			Recommend: yes("as Teclas de Filtro estão ligadas neste PC"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{bitsClearOp(`HKCU\Control Panel\Accessibility\Keyboard Response`, "Flags", 0x1, 126, "Teclas de Filtro")}
			},
		},
		{
			ID: "fix.input.queue", Category: "correcoes", Title: "Fila do mouse e do teclado no padrão (100)",
			Summary:  "Outro otimizador reduziu o buffer de eventos do driver de mouse/teclado. Isso não diminui a latência e pode fazer o driver descartar cliques e movimentos quando o sistema demora a ler.",
			Detail:   "Remove MouseDataQueueSize (mouclass) e/ou KeyboardDataQueueSize (kbdclass) quando estão abaixo de 100, voltando ao padrão documentado pela Microsoft. Vale após reiniciar.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcInputQueue, srcInputQueueTest},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"entrada"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Peripherals.QueueTooSmall() {
					return false, "filas de entrada no padrão"
				}
				return true, ""
			},
			Recommend: yes("fila reduzida encontrada neste PC"),
			Ops: func(f *facts.Facts) []tweaks.Op {
				var ops []tweaks.Op
				p := f.Peripherals
				if p.MouseQueue >= 0 && p.MouseQueue < 100 {
					ops = append(ops, tweaks.Reg(`HKLM\SYSTEM\CurrentControlSet\Services\mouclass\Parameters`, "MouseDataQueueSize", sys.Absent()))
				}
				if p.KeyboardQueue >= 0 && p.KeyboardQueue < 100 {
					ops = append(ops, tweaks.Reg(`HKLM\SYSTEM\CurrentControlSet\Services\kbdclass\Parameters`, "KeyboardDataQueueSize", sys.Absent()))
				}
				return ops
			},
		},
		{
			ID: "fix.kernel.threaddpc", Category: "correcoes", Title: "Religar os DPCs em thread (ThreadDpcEnable)",
			Summary:  "ThreadDpcEnable = 0 faz o trabalho que os drivers marcaram para rodar em thread interromper todas as threads, inclusive as do jogo.",
			Detail:   "Remove ThreadDpcEnable de HKLM\\SYSTEM\\CurrentControlSet\\Control\\Session Manager\\kernel (padrão: ligado). Não há medição pública que mostre ganho com o valor 0. Vale após reiniciar.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcThreadedDPC},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"latencia"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Peripherals.Readable || f.Peripherals.ThreadDPC != 0 {
					return false, "DPCs em thread no padrão"
				}
				return true, ""
			},
			Recommend: yes("ThreadDpcEnable = 0 encontrado neste PC"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\kernel`, "ThreadDpcEnable", sys.Absent())}
			},
		},
		{
			ID: "fix.bcd.hypervisor", Category: "correcoes", Title: "Religar o hipervisor do Windows (hypervisorlaunchtype)",
			Summary: "O boot está com hypervisorlaunchtype Off: a Segurança Baseada em Virtualização (VBS), a Integridade de Memória, o WSL2 e o Hyper-V ficam parados, e o FACEIT exige VBS.",
			Detail: "Equivale a \"bcdedit /set {current} hypervisorlaunchtype Auto\", o padrão do Windows. Com Off, o Windows não inicia o hipervisor, mesmo com a Integridade de Memória ligada nas configurações. " +
				"Se você desligou de propósito para ganhar desempenho, a opção \"Integridade de Memória (HVCI) e VBS desligadas\" (Avançado) faz isso de forma documentada e reversível. " +
				"O BitLocker ignora esse elemento do boot por padrão: não pede a chave de recuperação.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcBCDEdit, srcFaceitReq, srcBitLockerBCD},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"anticheat", "seguranca"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if ok, why := bootReadable(f); !ok {
					return false, why
				}
				if !strings.EqualFold(strings.TrimSpace(f.Boot.HypervisorLaunch), "Off") {
					return false, "o hipervisor não está desligado no boot"
				}
				return true, ""
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				switch {
				case !f.Read(facts.StepGaming):
					return tweaks.Rec(tweaks.RecYes, "os anti-cheats instalados não puderam ser verificados; FACEIT e Vanguard podem exigir VBS, que não roda com o hipervisor desligado")
				case f.RequiresVBS():
					return tweaks.Rec(tweaks.RecYes, "o anti-cheat instalado (FACEIT/Vanguard) pode exigir VBS, que não roda com o hipervisor desligado")
				case f.Security.VBSConfigured && !f.Security.VBSRunning:
					return tweaks.Rec(tweaks.RecYes, "a VBS está configurada para ligar, mas o hipervisor desligado a impede")
				}
				return tweaks.Rec(tweaks.RecNeutral, "sem anti-cheat que exija VBS: escolha sua entre segurança e o pequeno ganho de desempenho")
			},
			Ops: func(*facts.Facts) []tweaks.Op { return []tweaks.Op{bcdOp("hypervisorlaunchtype", "Auto")} },
		},
		{
			ID: "fix.power.throttling", Category: "correcoes", Title: "Religar o EcoQoS (PowerThrottlingOff)",
			Summary: "Um otimizador gravou PowerThrottlingOff = 1: o Windows deixa de reduzir o consumo dos processos em segundo plano, que passam a disputar energia e núcleos com o jogo.",
			Detail: "Remove PowerThrottlingOff de HKLM\\SYSTEM\\CurrentControlSet\\Control\\Power\\PowerThrottling (padrão: ausente). O EcoQoS só age em processos em segundo plano; o jogo em primeiro plano não é limitado. " +
				"Nos Intel com núcleos P e E é esse mecanismo que manda o trabalho de fundo para os núcleos E.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcPowerThrottle},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"fps", "energia"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Residue.Readable || !f.Residue.PowerThrottlingOff {
					return false, "EcoQoS no padrão"
				}
				return true, ""
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if f.CPU.IsIntelHybrid || f.Power.IsLaptop {
					return tweaks.Rec(tweaks.RecYes, "PowerThrottlingOff = 1 encontrado; neste PC o EcoQoS separa o trabalho de fundo do jogo")
				}
				return tweaks.Rec(tweaks.RecYes, "PowerThrottlingOff = 1 encontrado; volta ao padrão do Windows")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(`HKLM\SYSTEM\CurrentControlSet\Control\Power\PowerThrottling`, "PowerThrottlingOff", sys.Absent())}
			},
		},
		{
			ID: "fix.priosep", Category: "correcoes", Title: "Prioridade do primeiro plano no padrão (Win32PrioritySeparation)",
			Summary: "O Windows está dando ao programa em primeiro plano (o jogo) o mesmo tempo de processador que aos programas em segundo plano, a configuração de servidor.",
			Detail: "Volta Win32PrioritySeparation para 2, o padrão documentado pela Microsoft (primeiro plano com 3 vezes o tempo do segundo plano). O valor atual usa intervalos fixos (os dois bits do meio em 10, como o 0x1A que alguns otimizadores gravam) ou tem os dois bits mais baixos em 00, como o 0x18 gravado pela opção " +
				"\"Serviços em segundo plano\" de Opções de desempenho ou por scripts. Nos dois casos a Microsoft documenta o mesmo tempo para o primeiro e o segundo plano. Valores como 0x26 (a opção \"Programas\") não são alterados: equivalem ao padrão.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcPrioSepDoc},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"fps", "latencia"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Residue.NoForegroundBoost() {
					return false, "o primeiro plano já tem prioridade"
				}
				return true, ""
			},
			Recommend: yes("primeiro plano sem vantagem de tempo de processador"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(`HKLM\SYSTEM\CurrentControlSet\Control\PriorityControl`, "Win32PrioritySeparation", sys.DWord(2))}
			},
		},
		{
			ID: "fix.net.throttling", Category: "correcoes", Title: "Limitação de rede do MMCSS no padrão (NetworkThrottlingIndex = 10)",
			Summary: "Outro otimizador desligou a limitação de rede do agendador multimídia (NetworkThrottlingIndex = FFFFFFFF). A única medição pública viu a latência de DPC da placa de rede aumentar com isso, e não há medição de ping menor.",
			Detail: "Volta NetworkThrottlingIndex para 10, o padrão do Windows, em HKLM\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion\\Multimedia\\SystemProfile. " +
				"O valor não aparece na documentação do MMCSS. Na pesquisa de rede do djdallmann (capturas de DPC do xperf numa placa Intel), a latência de DPC do NDIS.sys ficou notavelmente maior com a limitação desligada; o autor nota que desligar só pode aumentar a vazão quando o tráfego passa do limite padrão de pacotes por milissegundo. " +
				"Vale após reiniciar. Desfazer volta ao valor anterior.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcDjdNetwork, srcMMCSS},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"rede", "latencia"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Residue.NetworkThrottlingOff() {
					return false, "limitação de rede do MMCSS no padrão"
				}
				return true, ""
			},
			Recommend: yes("NetworkThrottlingIndex = FFFFFFFF encontrado neste PC"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(facts.KeySystemProfile, "NetworkThrottlingIndex", sys.DWord(10))}
			},
		},
		{
			ID: "fix.mem.compression", Category: "correcoes", Title: "Religar a compressão de memória (MemoryCompression)",
			Summary: "A compressão de memória do Windows está desligada neste PC, como deixa o comando Disable-MMAgent -MemoryCompression que alguns otimizadores rodam. O padrão do Windows é ligada.",
			Detail: "O Axion lê o estado pelo WMI (classe PS_MMAgent, a mesma que o Get-MMAgent usa, sem PowerShell) e, ao aplicar, liga só o componente MemoryCompression; o pré-carregamento de aplicativos e as outras opções do agente não mudam. " +
				"A única fonte encontrada sobre o efeito é uma resposta de consultor independente no Microsoft Q&A, não documentação: com a compressão desligada o arquivo de paginação pode ser mais usado, com risco de lentidão se a RAM encher; ligada, o Windows usa um pouco de CPU e mostra mais memória em uso. " +
				"Não há medição pública de FPS nem de latência dos dois lados, e com RAM sobrando a diferença tende a ser nenhuma. Se você desligou de propósito para ver menos memória em uso, pode manter assim. " +
				"A mesma resposta manda reiniciar depois de mudar esse ajuste, então o Axion pede reinicialização e não coloca a correção no 1 clique. Desfazer volta ao valor que estava antes.",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcMMAgentEnable, srcMMAgentGet, srcMemCompQA},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"memoria"}, OneClick: false,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Residue.MemoryCompressionOff() {
					return false, "compressão de memória ligada ou não pôde ser lida"
				}
				return true, ""
			},
			Recommend: yes("compressão de memória desligada neste PC; o padrão do Windows é ligada (se foi de propósito, pode manter)"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{memCompressionOp(true)}
			},
		},
		{
			ID: "fix.bcd.limits", Category: "correcoes", Title: "Remover limites de processadores e memória do boot",
			Summary:  "As opções \"Número de processadores\" e \"Memória máxima\" do msconfig estão gravadas no boot. Elas existem para testes e, quando menores que o hardware, deixam o Windows usar menos núcleos ou menos RAM.",
			Detail:   "bcdedit /deletevalue {current} numproc, truncatememory e removememory (só os que existirem). Vale após reiniciar.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcBCDEdit},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"fps"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if ok, why := bootReadable(f); !ok {
					return false, why
				}
				if f.Boot.NumProc == "" && f.Boot.TruncateMemory == "" && f.Boot.RemoveMemory == "" {
					return false, "sem limites de processador ou memória no boot"
				}
				return true, ""
			},
			Recommend: yes("limite encontrado na configuração de boot"),
			Ops: func(f *facts.Facts) []tweaks.Op {
				var ops []tweaks.Op
				for _, e := range []struct{ name, val string }{{"numproc", f.Boot.NumProc}, {"truncatememory", f.Boot.TruncateMemory}, {"removememory", f.Boot.RemoveMemory}} {
					if e.val != "" {
						ops = append(ops, bcdOp(e.name, ""))
					}
				}
				return ops
			},
		},
	}
}

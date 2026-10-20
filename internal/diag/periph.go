package diag

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"axion/internal/facts"
	"axion/internal/periph"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

var (
	srcUSBInterval  = srcMS("Microsoft Learn — USB_ENDPOINT_DESCRIPTOR: período de polling por velocidade e bInterval", "https://learn.microsoft.com/windows-hardware/drivers/ddi/usbspec/ns-usbspec-_usb_endpoint_descriptor")
	srcHIDSuspend   = srcMS("Microsoft Learn — Selective suspend for HID over USB devices", "https://learn.microsoft.com/windows-hardware/drivers/hid/selective-suspend-for-hid-over-usb-devices")
	srcBTInterval   = srcMS("Bluetooth SIG — intervalos de conexão (mínimo histórico de 7,5 ms no Bluetooth LE)", "https://www.bluetooth.com/blog/bluetooth-shorter-connection-intervals-paving-the-way-for-bluetooth-innovation/")
	srcRazerPolling = srcMS("Razer — HyperPolling: porta USB nativa, custo de CPU e jogos antigos", "https://www.razer.com/technology/razer-hyperpolling")
	srcHidusbf      = srcMS("hidusbf (README) — exige desligar a Integridade de Memória", "https://github.com/LordOfMice/hidusbf/blob/master/README.md")
	srcFaceitReqs   = srcMS("FACEIT — Problemas conhecidos dos requisitos do anti-cheat (Integridade de Memória pode ser exigida de alguns jogadores)", "https://support.faceit.com/hc/en-us/articles/22851956652956-Known-issues-with-Anti-Cheat-Requirements")
	srcInputQueue   = srcMS("Microsoft KB Q102989 — KeyboardDataQueueSize / MouseDataQueueSize (arquivo)", "https://jeffpar.github.io/kbarchive/kb/102/Q102989/")
	srcThreadedDPC  = srcMS("Microsoft Learn — Introduction to Threaded DPCs (ThreadDpcEnable)", "https://learn.microsoft.com/windows-hardware/drivers/kernel/introduction-to-threaded-dpcs")
	srcFilterKeys   = srcMS("Microsoft Learn — FILTERKEYS (aceitação e repetição de teclas)", "https://learn.microsoft.com/windows/win32/api/winuser/ns-winuser-filterkeys")
	srcBootLimits   = srcMS("Microsoft Learn — BCDEdit /set (numproc, truncatememory, removememory)", "https://learn.microsoft.com/windows-hardware/drivers/devtest/bcdedit--set")
)

func init() {
	rules = append(rules, inputSettingsRule, inputDevicesRule, bootLimitsRule)
}

func pageAction(label string) Action { return Action{Kind: "page", Label: label, Page: "perifericos"} }

// devID gives a stable, short finding suffix for a physical device.
func devID(d facts.InputDev) string {
	if d.VID != 0 {
		return fmt.Sprintf("%04X-%04X", d.VID, d.PID)
	}
	h := fnv.New32a()
	h.Write([]byte(strings.ToUpper(d.Key)))
	return fmt.Sprintf("%08x", h.Sum32())
}

func roleName(d facts.InputDev) string {
	if d.Primary == "mouse" {
		if d.Receiver {
			return "Receptor sem fio"
		}
		return "Mouse"
	}
	return "Teclado"
}

func inputSettingsRule(f *facts.Facts, _ map[string]tweaks.State, add func(Finding)) {
	p := f.Peripherals
	if !p.Readable {
		return
	}
	if p.FilterKeysOn {
		sev, what := SevWarning, "As Teclas de Filtro estão ligadas."
		var ev []string
		if p.FilterDelayMS > 0 {
			ev = append(ev, fmt.Sprintf("toques mais curtos que %d ms são ignorados", p.FilterDelayMS))
		}
		if p.FilterBounceMS > 0 {
			ev = append(ev, fmt.Sprintf("a mesma tecla só é aceita de novo após %d ms", p.FilterBounceMS))
		}
		if len(ev) > 0 {
			sev, what = SevCritical, "As Teclas de Filtro estão ligadas: "+strings.Join(ev, " e ")+"."
		}
		add(Finding{ID: "input.filterkeys", Area: AreaLatencia, Severity: sev, Title: "Teclas de Filtro ligadas: o Windows está segurando ou descartando teclas",
			Evidence:    what + fmt.Sprintf(" (Flags com FKF_FILTERKEYSON; DelayBeforeAcceptance=%d, BounceTime=%d)", p.FilterDelayMS, p.FilterBounceMS),
			Explanation: "É um recurso de acessibilidade para quem tem tremor nas mãos. Ligado sem querer (por exemplo, segurando o Shift direito por 8 segundos), faz o Windows exigir que cada tecla fique pressionada por um tempo mínimo ou ignorar repetições rápidas, o que atrapalha counter-strafe e troca de armas.",
			Actions:     []Action{fixTweaks("Desligar as Teclas de Filtro", "fix.a11y.filterkeys"), {Kind: "url", Label: "Abrir configurações do teclado", URL: "ms-settings:easeofaccess-keyboard"}},
			Sources:     []tweaks.Source{srcFilterKeys}})
	}
	if p.QueueTooSmall() {
		var ev []string
		if p.MouseQueue >= 0 && p.MouseQueue < 100 {
			ev = append(ev, fmt.Sprintf("MouseDataQueueSize = %d", p.MouseQueue))
		}
		if p.KeyboardQueue >= 0 && p.KeyboardQueue < 100 {
			ev = append(ev, fmt.Sprintf("KeyboardDataQueueSize = %d", p.KeyboardQueue))
		}
		add(Finding{ID: "res.inputqueue", Area: AreaLatencia, Severity: SevWarning, Title: "Fila do driver de mouse/teclado reduzida por outro otimizador",
			Evidence:    strings.Join(ev, "; ") + " (padrão do Windows: 100).",
			Explanation: "Esse valor é o tamanho do buffer onde o driver guarda eventos até o sistema lê-los; não é um atraso. Diminuí-lo não reduz a latência e, se o sistema demorar a ler (um travamento breve, por exemplo), os eventos que não cabem são descartados: cliques ou movimentos perdidos.",
			Actions:     []Action{fixTweaks("Voltar ao padrão", "fix.input.queue")},
			Sources:     []tweaks.Source{srcInputQueue}})
	}
	if p.ThreadDPC == 0 {
		add(Finding{ID: "res.threaddpc", Area: AreaLatencia, Severity: SevWarning, Title: "DPCs em thread desligados (ThreadDpcEnable = 0)",
			Evidence:    `ThreadDpcEnable = 0 em HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\kernel.`,
			Explanation: "Drivers podem marcar trabalho como \"DPC em thread\", que roda como uma thread de prioridade muito alta (em PASSIVE_LEVEL, podendo ser interrompido por DPCs comuns e interrupções). Com esse valor em 0, o Windows executa esse trabalho como DPC comum, que interrompe as threads do processador até terminar, inclusive as do jogo. Não há medição que mostre ganho com essa mudança.",
			Actions:     []Action{fixTweaks("Voltar ao padrão", "fix.kernel.threaddpc")},
			Sources:     []tweaks.Source{srcThreadedDPC}})
	}
}

func inputDevicesRule(f *facts.Facts, _ map[string]tweaks.State, add func(Finding)) {
	p := f.Peripherals
	for _, d := range p.Devices {
		if d.Transport == periph.TransportVirtual || d.Transport == periph.TransportUnknown {
			continue
		}
		id, role := devID(d), roleName(d)
		name := d.Name
		switch d.Transport {
		case periph.TransportBT, periph.TransportBLE:
			sev := SevInfo
			if d.Primary == "mouse" {
				sev = SevWarning
			}
			add(Finding{ID: "per.bt." + id, Area: AreaLatencia, Severity: sev, Title: fmt.Sprintf("%s por Bluetooth: %s", role, name),
				Evidence:    fmt.Sprintf("Conectado via %s.", periph.TransportLabel(d.Transport)),
				Explanation: "O Bluetooth troca dados em intervalos de conexão (7,5 ms era o mínimo histórico; o modo de intervalos mais curtos exige suporte nos dois lados, embora rode no hardware de rádio existente, segundo o Bluetooth SIG) e compartilha o rádio com outros aparelhos, então a latência é maior e menos estável que no cabo ou no receptor 2,4 GHz do fabricante, que costumam trabalhar a 1000 Hz ou mais.",
				Actions:     []Action{pageAction("Ver periféricos")},
				Sources:     []tweaks.Source{srcBTInterval}})
			continue
		case periph.TransportUSB:
		default:
			continue
		}
		if d.Speed == sys.USBSpeedLow {
			bi := ""
			if d.BInterval > 0 {
				bi = fmt.Sprintf(", bInterval %d", d.BInterval)
			}
			add(Finding{ID: "per.lowspeed." + id, Area: AreaLatencia, Severity: SevWarning,
				Title:       fmt.Sprintf("%s USB Low-Speed: no máximo 125 Hz (%s)", role, name),
				Evidence:    fmt.Sprintf("%s%s. O Windows lê dispositivos Low-Speed a cada 8 ms ou mais.", periph.SpeedLabel(d.Speed), bi),
				Explanation: "Cada tecla ou movimento espera em média 4 ms para ser lido (contra 0,5 ms a 1000 Hz), cerca de 3,5 ms a mais; no pior caso, 8 ms contra 1 ms. Não existe ajuste seguro no Windows para isso: o \"overclock\" de polling por driver (hidusbf) exige desligar a Integridade de Memória, que o FACEIT pode exigir ligada de alguns jogadores. A solução é um periférico USB Full-Speed ou High-Speed de 1000 Hz ou mais.",
				Actions:     []Action{pageAction("Ver periféricos")},
				Sources:     []tweaks.Source{srcUSBInterval, srcHidusbf, srcFaceitReqs}})
		} else if d.Primary == "mouse" && d.NominalHz > 0 && d.NominalHz < 1000 {
			sev := SevInfo
			if d.NominalHz <= 250 {
				sev = SevWarning
			}
			add(Finding{ID: "per.rate." + id, Area: AreaLatencia, Severity: sev,
				Title:       fmt.Sprintf("%s anuncia no máximo %s (%s)", role, periph.FormatHz(d.NominalHz), name),
				Evidence:    fmt.Sprintf("%s, bInterval %d no endpoint de entrada.", periph.SpeedLabel(d.Speed), d.BInterval),
				Explanation: "O próprio dispositivo pede para ser lido nessa frequência. Se o software do fabricante oferecer 1000 Hz, selecione e rode a medição de novo; 1000 Hz reduz o atraso médio do mouse para cerca de 0,5 ms.",
				Actions:     []Action{pageAction("Medir o polling")},
				Sources:     []tweaks.Source{srcUSBInterval}})
		}
		if d.HubDepth > 0 {
			sev := SevInfo
			if d.Primary == "mouse" && d.NominalHz >= 2000 {
				sev = SevWarning
			}
			add(Finding{ID: "per.hub." + id, Area: AreaLatencia, Severity: sev,
				Title:       fmt.Sprintf("%s ligado através de hub USB (%s)", role, name),
				Evidence:    plural(d.HubDepth, "hub intermediário", "hubs intermediários") + " entre o dispositivo e o controlador " + strings.TrimSpace(d.Controller) + ".",
				Explanation: "Hubs (inclusive os de monitor, de teclado e os que a própria placa usa por dentro, comuns em notebook) adicionam um salto e dividem a banda e o agendamento da porta. Fabricantes de mouses de alta taxa pedem conexão direta numa porta nativa da placa-mãe, de preferência a traseira; no notebook, teste as outras portas.",
				Actions:     []Action{pageAction("Ver topologia USB")},
				Sources:     []tweaks.Source{srcRazerPolling}})
		}
		// The "Allow the computer to turn off this device" box is checked by
		// default; it only makes an input device sleep during use when USB
		// selective suspend is also enabled for it (off by default for HID).
		if d.SelSuspend == 1 {
			ev := []string{"SelectiveSuspendEnabled = 1 no dispositivo"}
			for _, n := range d.PowerNodes {
				if n.Enabled {
					ev = append(ev, "\"Permitir que o computador desligue este dispositivo\" marcado em "+shortInstance(n.Instance))
				}
			}
			add(Finding{ID: "per.suspend." + id, Area: AreaLatencia, Severity: SevWarning,
				Title:       fmt.Sprintf("O Windows pode suspender o %s para economizar energia (%s)", strings.ToLower(role), name),
				Evidence:    strings.Join(ev, "; ") + ".",
				Explanation: "Com a suspensão seletiva, o dispositivo \"dorme\" depois de um tempo parado e o primeiro movimento ou tecla precisa acordá-lo. A própria Microsoft deixa isso desligado por padrão para teclados e mouses USB, justamente para evitar o atraso ao retomar; aqui o fabricante ou outro programa ligou.",
				Actions:     []Action{fixTweaks("Impedir a suspensão", "input.usb.power")},
				Sources:     []tweaks.Source{srcHIDSuspend}})
		}
	}
}

func shortInstance(s string) string {
	if i := strings.LastIndexByte(s, '\\'); i > 0 {
		return s[:i]
	}
	return s
}

func bootLimitsRule(f *facts.Facts, _ map[string]tweaks.State, add func(Finding)) {
	b := f.Boot
	if !b.Readable || (b.NumProc == "" && b.TruncateMemory == "" && b.RemoveMemory == "") {
		return
	}
	sev := SevInfo
	var ev []string
	if b.NumProc != "" {
		ev = append(ev, "numproc = "+b.NumProc)
		if n, err := strconv.Atoi(strings.TrimSpace(b.NumProc)); err == nil && f.CPU.Threads > 0 && n < f.CPU.Threads {
			sev = SevCritical
			ev[len(ev)-1] += fmt.Sprintf(" (o processador tem %d threads)", f.CPU.Threads)
		}
	}
	if b.TruncateMemory != "" {
		ev = append(ev, "truncatememory = "+b.TruncateMemory)
		sev = SevCritical
	}
	if b.RemoveMemory != "" {
		ev = append(ev, "removememory = "+b.RemoveMemory)
		sev = SevCritical
	}
	title := "Limite de processadores ou memória na inicialização"
	if sev == SevInfo {
		title = "Limite de processadores na inicialização (sem efeito, mas desnecessário)"
	}
	add(Finding{ID: "res.bootlimits", Area: AreaDesempenho, Severity: sev, Title: title,
		Evidence:    strings.Join(ev, "; ") + " na entrada de boot atual.",
		Explanation: "São as opções \"Número de processadores\" e \"Memória máxima\" do msconfig (Opções avançadas de inicialização). Guias antigos mandam marcá-las para \"acelerar o boot\", mas elas só servem para testes: limitam o Windows a menos núcleos ou menos RAM do que o PC tem.",
		Actions:     []Action{fixTweaks("Remover os limites", "fix.bcd.limits")},
		Sources:     []tweaks.Source{srcBootLimits}})
}

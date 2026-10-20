package catalog

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

func inputTweaks() []*tweaks.Tweak {
	return []*tweaks.Tweak{
		{
			ID: "input.mouse.accel", Category: "entrada", Title: "Mouse 1:1 (sem \"Aprimorar precisão do ponteiro\")",
			Summary: "Desliga a aceleração do ponteiro do Windows: o cursor passa a andar exatamente proporcional ao movimento físico do mouse.",
			Detail: "HKCU\\Control Panel\\Mouse: MouseSpeed=0 e limiares 0/0, aplicado na hora via SystemParametersInfo(SPI_SETMOUSE). A sensibilidade (6/11) não muda. " +
				"Jogos com raw input (como o CS2) já ignoram essa curva; o ganho aparece no desktop, em menus e em jogos sem raw input.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcMouseAccel, srcRawInput},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"entrada", "mira"}, OneClick: true,
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if f.Gaming.MouseAccelOn {
					return tweaks.Rec(tweaks.RecYes, "a aceleração do ponteiro está ligada neste PC")
				}
				return tweaks.Rec(tweaks.RecYes, "garante movimento 1:1 fora de jogos com raw input")
			},
			Ops: func(*facts.Facts) []tweaks.Op { return []tweaks.Op{mouseOp()} },
		},
		{
			ID: "input.a11y.hotkeys", Category: "entrada", Title: "Atalhos de acessibilidade desligados (Shift 5x, etc.)",
			Summary:  "Impede que apertar Shift cinco vezes (Teclas de Aderência) ou segurar Shift direito (Teclas de Filtro) abra uma janela no meio da partida.",
			Detail:   "Remove apenas o bit HOTKEYACTIVE (0x4) de Flags em HKCU\\Control Panel\\Accessibility\\StickyKeys, \\Keyboard Response e \\ToggleKeys, preservando as outras preferências. Os recursos continuam disponíveis em Configurações.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcStickyKeys},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootSignOut, Tags: []string{"entrada"}, OneClick: true,
			Recommend: yes("evita que o jogo perca o foco por um atalho acidental"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{
					bitsClearOp(`HKCU\Control Panel\Accessibility\StickyKeys`, "Flags", 0x4, 510, "atalho das Teclas de Aderência"),
					bitsClearOp(`HKCU\Control Panel\Accessibility\Keyboard Response`, "Flags", 0x4, 126, "atalho das Teclas de Filtro"),
					bitsClearOp(`HKCU\Control Panel\Accessibility\ToggleKeys`, "Flags", 0x4, 62, "atalho das Teclas de Alternância"),
				}
			},
		},
		{
			ID: "input.usb.power", Category: "entrada", Title: "Mouse e teclado sem suspensão USB",
			Summary: "Impede que o Windows coloque mouse, teclado ou receptor sem fio para \"dormir\" quando ficam parados, o que faz o primeiro movimento ou tecla esperar o dispositivo acordar.",
			Detail: "Só age nos seus dispositivos de entrada e só onde a suspensão está ativa: desmarca \"Permitir que o computador desligue este dispositivo para economizar energia\" pela mesma interface WMI do Gerenciador de Dispositivos (MSPower_DeviceEnable, vale na hora) " +
				"e zera SelectiveSuspendEnabled em Device Parameters quando o fabricante o ligou (vale ao reconectar ou reiniciar). A Microsoft deixa a suspensão seletiva desligada por padrão em teclados e mouses USB justamente para evitar o atraso ao retomar. " +
				"Outros otimizadores fazem isso em todos os dispositivos do PC; aqui hubs, controladores e o resto do sistema não são tocados.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcHIDSuspend, srcUSBSuspend},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"entrada", "latencia"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Peripherals.Readable {
					return false, "periféricos não lidos"
				}
				for _, d := range f.Peripherals.Devices {
					if d.SuspendAllowed() {
						return true, ""
					}
				}
				return false, "nenhum mouse ou teclado pode ser suspenso (padrão do Windows)"
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				for _, d := range f.Peripherals.Devices {
					if d.SelSuspend == 1 {
						return tweaks.Rec(tweaks.RecYes, "a suspensão seletiva USB está ligada em um dispositivo de entrada deste PC")
					}
				}
				return tweaks.Rec(tweaks.RecNeutral, "só a caixa padrão do Windows está marcada; com a suspensão seletiva desligada o ganho em jogo não é comprovado")
			},
			Ops: func(f *facts.Facts) []tweaks.Op {
				var ops []tweaks.Op
				for _, d := range f.Peripherals.Devices {
					for _, n := range d.PowerNodes {
						if n.Enabled && n.WMIName != "" {
							ops = append(ops, devicePowerOp(n.WMIName))
						}
					}
					for _, node := range d.SuspendNodes {
						ops = append(ops, selSuspendOp(node))
					}
				}
				return ops
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Network
// ---------------------------------------------------------------------------

func activePhysical(f *facts.Facts, ethernetOnly bool) []facts.Adapter {
	var out []facts.Adapter
	for _, a := range f.Adapters {
		if !a.Up || a.ClassKey == "" {
			continue
		}
		if a.Kind != "Ethernet" && a.Kind != "Wi-Fi" {
			continue
		}
		if ethernetOnly && a.Kind != "Ethernet" {
			continue
		}
		out = append(out, a)
	}
	return out
}

// Energy-efficient Ethernet keywords (shared with the diagnostic).
var eeeKeywords = facts.EEEKeywords

// physical returns the wired (or wired and Wi-Fi) adapters with a driver
// key, connected or not: driver settings are evaluated and changed on a
// cable that is unplugged today too (the first real report showed an
// unplugged Realtek with a fixed speed that Axion did not see).
func physical(f *facts.Facts, ethernetOnly bool) []facts.Adapter {
	var out []facts.Adapter
	for _, a := range f.PhysicalAdapters() {
		if ethernetOnly && a.Kind != "Ethernet" {
			continue
		}
		out = append(out, a)
	}
	return out
}

// forcedSpeed returns the fixed *SpeedDuplex value of a wired adapter ("" when
// automatic or not exposed) and whether that adapter is connected; a speed of
// 10/100 Mb/s wins over 1 Gb/s, and a connected adapter over an unplugged one.
func forcedSpeed(f *facts.Facts) string {
	v, _ := forcedSpeedOf(f)
	return v
}

func forcedSpeedOf(f *facts.Facts) (string, bool) {
	got, up := "", false
	for _, a := range physical(f, true) {
		k, ok := a.Keywords["*SpeedDuplex"]
		if !ok || k.Current == "" || k.Current == "0" {
			continue
		}
		better := got == "" || (slowSpeed(k.Current) && !slowSpeed(got)) || (slowSpeed(k.Current) == slowSpeed(got) && a.Up && !up)
		if better {
			got, up = k.Current, a.Up
		}
	}
	return got, up
}

// slowSpeed: *SpeedDuplex 1-4 are 10/100 Mb/s (standardized INF keywords).
func slowSpeed(v string) bool {
	return len(v) == 1 && v >= "1" && v <= "4"
}

func nicKeywordOps(f *facts.Facts, keywords []string, want string, ethernetOnly bool) []tweaks.Op {
	var ops []tweaks.Op
	for _, a := range physical(f, ethernetOnly) {
		names := make([]string, 0, len(a.Keywords))
		for k := range a.Keywords {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, kw := range keywords {
			for _, have := range names {
				if !strings.EqualFold(have, kw) {
					continue
				}
				info := a.Keywords[have]
				if len(info.Values) > 0 && !contains(info.Values, want) {
					continue // driver does not accept this value
				}
				// Absent value == driver default: treat "absent with default==want" as in target.
				var alt []sys.RegValue
				if info.Default == want {
					alt = append(alt, sys.Absent())
				}
				ops = append(ops, tweaks.Reg(a.ClassKey, have, sys.SZ(want), alt...))
			}
		}
	}
	return ops
}

// nicDefaultOps writes the driver's default into every advanced property
// that differs from it (see facts.NicRestorable for what is left out).
func nicDefaultOps(f *facts.Facts) []tweaks.Op {
	var ops []tweaks.Op
	for _, a := range physical(f, false) {
		for _, ch := range a.NonDefaults() { // only defaults the driver itself accepts
			ops = append(ops, tweaks.Reg(a.ClassKey, ch.Keyword, sys.SZ(a.Keywords[ch.Keyword].Default), sys.Absent()))
		}
	}
	return ops
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// wmiReturn reads the ReturnValue of a WMI method (0 = success).
func wmiReturn(v any) int64 {
	switch x := v.(type) {
	case nil:
		return 0
	case int64:
		return x
	case int32:
		return int64(x)
	case uint32:
		return int64(x)
	case int:
		return int64(x)
	case uint64:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	}
	return 0
}

// sleepCtx waits d or until the context is cancelled.
func sleepCtx(c *tweaks.Ctx, d time.Duration) {
	if c.Context == nil {
		time.Sleep(d)
		return
	}
	select {
	case <-time.After(d):
	case <-c.Context.Done():
	}
}

// restartTargets picks the adapters to restart: those whose driver key an
// operation of this apply or revert changed; without that information (older
// callers), the connected ones.
func restartTargets(c *tweaks.Ctx, ethernetOnly bool) []facts.Adapter {
	var out []facts.Adapter
	for _, a := range physical(c.F, ethernetOnly) {
		if len(c.Touched) == 0 {
			if a.Up {
				out = append(out, a)
			}
			continue
		}
		prefix := tweaks.Reg(a.ClassKey, "", sys.Absent()).Target()
		for _, t := range c.Touched {
			if strings.HasPrefix(t, prefix) {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

// restartAdapters disables and re-enables the adapters whose settings changed
// so the driver reloads its registry parameters (what
// Set-NetAdapterAdvancedProperty does). An adapter must never be left
// disabled: re-enabling is retried, confirmed by reading NetEnabled back, and
// falls back to netsh. An adapter the user disabled stays disabled: its
// driver reads the new values when it is enabled again.
func restartAdapters(ethernetOnly bool) func(c *tweaks.Ctx, reverted bool) error {
	return func(c *tweaks.Ctx, _ bool) error {
		if c.F == nil {
			return nil
		}
		var errs []string
		for _, a := range restartTargets(c, ethernetOnly) {
			if a.PnPDeviceID == "" {
				continue
			}
			q := fmt.Sprintf("SELECT DeviceID, NetConnectionID, NetEnabled FROM Win32_NetworkAdapter WHERE PNPDeviceID='%s'", wqlString(a.PnPDeviceID))
			rows, err := c.P.WMI.Query(`root\cimv2`, q)
			if err != nil || len(rows) == 0 {
				errs = append(errs, a.Name+": adaptador não encontrado para reiniciar")
				continue
			}
			if _, ok := rows[0]["NetEnabled"]; ok && !rows[0].Bool("NetEnabled") && !a.Up {
				continue // disabled by the user: nothing to reload now
			}
			conn := rows[0].Str("NetConnectionID")
			path := fmt.Sprintf(`Win32_NetworkAdapter.DeviceID="%s"`, rows[0].Str("DeviceID"))
			ret, err := c.P.WMI.Call(`root\cimv2`, path, "Disable")
			if err != nil || wmiReturn(ret) != 0 {
				if err == nil {
					err = fmt.Errorf("código %d", wmiReturn(ret))
				}
				errs = append(errs, a.Name+": não foi possível reiniciar ("+err.Error()+")")
				continue
			}
			sleepCtx(c, 1500*time.Millisecond)
			enabled := func() bool {
				r, err := c.P.WMI.Query(`root\cimv2`, q)
				return err == nil && len(r) > 0 && r[0].Bool("NetEnabled")
			}
			var lastErr string
			for try := 0; try < 3 && !enabled(); try++ {
				if try > 0 {
					sleepCtx(c, time.Second)
				}
				ret, err := c.P.WMI.Call(`root\cimv2`, path, "Enable")
				switch {
				case err != nil:
					lastErr = err.Error()
				case wmiReturn(ret) != 0:
					lastErr = fmt.Sprintf("código %d", wmiReturn(ret))
				default:
					lastErr = ""
					sleepCtx(c, 500*time.Millisecond)
				}
			}
			if !enabled() && conn != "" {
				res, err := c.P.Run.Run(context.Background(), "netsh.exe", []string{"interface", "set", "interface", "name=" + conn, "admin=enabled"}, sys.RunOpts{Timeout: 30 * time.Second})
				if err == nil && res.ExitCode == 0 {
					sleepCtx(c, time.Second)
				}
			}
			if !enabled() {
				msg := a.Name + ": o adaptador não voltou a ser ativado"
				if lastErr != "" {
					msg += " (" + lastErr + ")"
				}
				errs = append(errs, msg+" — ative-o em Configurações > Rede e Internet > Configurações de rede avançadas")
				if c.Log != nil {
					c.Log("ATENÇÃO: adaptador %s (%s) ficou desativado após o reinício do driver", a.Name, conn)
				}
			}
		}
		if len(errs) > 0 {
			msg := strings.Join(errs, "; ")
			if !strings.Contains(msg, "não voltou a ser ativado") {
				msg += " (a mudança vale após reiniciar o PC)"
			}
			return fmt.Errorf("reinício do adaptador: %s", msg)
		}
		return nil
	}
}

func networkTweaks() []*tweaks.Tweak {
	return []*tweaks.Tweak{
		{
			ID: "net.nic.power", Category: "rede", Title: "Placa de rede sem economia de energia",
			Summary:  "Desmarca \"Permitir que o computador desligue este dispositivo para economizar energia\" na placa de rede ativa, evitando quedas e atrasos ao voltar do modo econômico.",
			Detail:   "PnPCapabilities = 24 (0x18) na chave do driver do adaptador (HKLM\\SYSTEM\\CurrentControlSet\\Control\\Class\\{4d36e972-…}\\NNNN), o valor usado para desmarcar essa caixa de seleção (o artigo da Microsoft que o descrevia foi retirado). Pode desativar junto o despertar pela rede (Wake-on-LAN) a partir do repouso. O adaptador é reiniciado (queda de 2–5 s).",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcNicPower},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"rede", "estabilidade"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if len(activePhysical(f, false)) == 0 {
					return false, "nenhum adaptador físico conectado"
				}
				return true, ""
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if f.Power.IsLaptop && !ethernetActive(f) {
					return tweaks.Rec(tweaks.RecNo, "em notebook no Wi-Fi o ganho é pequeno e há custo de bateria")
				}
				return tweaks.Rec(tweaks.RecNeutral, "é o padrão do Windows e só age com o PC em repouso ou sem uso da rede; ajuda se a placa cai ao voltar do repouso, não muda o ping")
			},
			Ops: func(f *facts.Facts) []tweaks.Op {
				var ops []tweaks.Op
				for _, a := range activePhysical(f, false) {
					ops = append(ops, tweaks.Reg(a.ClassKey, "PnPCapabilities", sys.DWord(24)))
				}
				return ops
			},
			Post: restartAdapters(false),
		},
		{
			ID: "net.nic.eee", Category: "rede", Title: "Ethernet sem economia de energia (EEE/Green Ethernet)",
			Summary:  "Desliga o Energy-Efficient Ethernet e equivalentes do fabricante, que colocam o link em baixo consumo entre pacotes.",
			Detail:   "Propriedades avançadas do driver (*EEE e variantes Realtek/Intel: AdvancedEEE, EnableGreenEthernet, GigaLite, PowerSavingMode) = 0, só nas que o driver expõe, em todas as placas com cabo (conectadas ou não). O despertar do link custa microssegundos; o motivo principal é que o EEE causa quedas de link em algumas placas Realtek. O adaptador alterado é reiniciado.",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcNdisKeywords},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"rede", "estabilidade"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if len(nicKeywordOps(f, eeeKeywords, "0", true)) == 0 {
					return false, "nenhuma placa de rede com cabo deste PC expõe o EEE"
				}
				return true, ""
			},
			Recommend: neutral("inofensivo em desktop; ajuda principalmente em placas Realtek com quedas de conexão"),
			Ops:       func(f *facts.Facts) []tweaks.Op { return nicKeywordOps(f, eeeKeywords, "0", true) },
			Post:      restartAdapters(true),
		},
		{
			ID: "net.nic.intmod", Category: "rede", Title: "Moderação de interrupção desligada",
			Summary:  "Faz a placa de rede avisar a CPU a cada pacote, em vez de agrupar vários pacotes por interrupção.",
			Detail:   "*InterruptModeration = 0 no driver. Reduz a latência local em microssegundos, com mais uso de CPU em tráfego pesado. É invisível diante da latência da internet (milissegundos); só vale em LAN ou para quem quer extrair o último microssegundo.",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcIntMod, srcIntModTuning},
			Risk: tweaks.RiskModerate, Reboot: tweaks.RebootNone, Tags: []string{"rede", "latencia"}, OneClick: false,
			Applicable: func(f *facts.Facts) (bool, string) {
				if len(nicKeywordOps(f, []string{"*InterruptModeration"}, "0", true)) == 0 {
					return false, "nenhuma placa de rede com cabo deste PC expõe esta opção"
				}
				return true, ""
			},
			Recommend: neutral("ganho de microssegundos, imperceptível na latência da internet"),
			Ops:       func(f *facts.Facts) []tweaks.Op { return nicKeywordOps(f, []string{"*InterruptModeration"}, "0", true) },
			Post:      restartAdapters(true),
		},
		{
			ID: "net.nagle", Category: "rede", Title: "TCP sem atraso de confirmação (Nagle/ACK atrasado)",
			Summary: "Envia pacotes TCP pequenos imediatamente e confirma cada segmento na hora.",
			Detail: "TcpAckFrequency=1 e TCPNoDelay=1 em HKLM\\SYSTEM\\CurrentControlSet\\Services\\Tcpip\\Parameters\\Interfaces\\{GUID} dos adaptadores ativos. Só afeta TCP: CS2, Valorant, Fortnite, Apex e a maioria dos jogos competitivos usam UDP e não mudam nada. Pode ajudar jogos antigos/MMOs em TCP. " +
				"A Microsoft não recomenda mudar o TcpAckFrequency sem um estudo cuidadoso; o TCPNoDelay não é documentado por ela.",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcNagle},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestart, Tags: []string{"rede"},
			Applicable: func(f *facts.Facts) (bool, string) {
				for _, a := range activePhysical(f, false) {
					if a.InterfaceGUID != "" {
						return true, ""
					}
				}
				return false, "nenhum adaptador ativo"
			},
			Recommend: func(*facts.Facts) tweaks.Recommendation {
				return tweaks.Rec(tweaks.RecNo, "sem efeito em jogos UDP (a maioria dos competitivos) e a Microsoft não recomenda mudar sem estudo; só para jogos TCP")
			},
			Ops: func(f *facts.Facts) []tweaks.Op {
				var ops []tweaks.Op
				for _, a := range activePhysical(f, false) {
					if a.InterfaceGUID == "" {
						continue
					}
					k := `HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\` + a.InterfaceGUID
					ops = append(ops, tweaks.Reg(k, "TcpAckFrequency", sys.DWord(1)), tweaks.Reg(k, "TCPNoDelay", sys.DWord(1)))
				}
				return ops
			},
		},
		{
			ID: "net.nic.msi", Category: "rede", Title: "Modo MSI (Message Signaled Interrupts) na placa Ethernet",
			Summary: "Força o modo MSI na placa Ethernet. Só vale se o driver ainda usa o modo herdado. Opção avançada, fora do 1 clique.",
			Detail: "MSISupported=1 em HKLM\\SYSTEM\\CurrentControlSet\\Enum\\<PnPDeviceID>\\Device Parameters\\Interrupt Management\\MessageSignaledInterruptProperties da placa de rede. O Axion não grava prioridade de interrupção: o valor DevicePriority não está documentado pela Microsoft. " +
				"O MSI é um recurso documentado do Windows, mas nem todo driver de rede o trata bem: se a placa não suportar de verdade, o efeito é perda da conexão ou travamento na inicialização. " +
				"A Microsoft não publica medição de ganho de latência ao forçá-lo; o benefício é uma hipótese a medir (aba Rede e ping, antes e depois). Em caso de problema, desfaça pelo Histórico (em modo seguro, se necessário).",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcMSI},
			Risk: tweaks.RiskModerate, Reboot: tweaks.RebootRestart, Tags: []string{"rede", "latencia"},
			Applicable: func(f *facts.Facts) (bool, string) {
				for _, a := range activePhysical(f, true) {
					if strings.HasPrefix(strings.ToUpper(a.PnPDeviceID), "PCI\\") {
						return true, ""
					}
				}
				return false, "nenhuma placa de rede Ethernet PCI ativa encontrada"
			},
			Recommend: neutral("só para testar com medição; sem sintoma de latência na rede, não há motivo para mexer"),
			Ops: func(f *facts.Facts) []tweaks.Op {
				var ops []tweaks.Op
				for _, a := range activePhysical(f, true) {
					if strings.HasPrefix(strings.ToUpper(a.PnPDeviceID), "PCI\\") {
						base := `HKLM\SYSTEM\CurrentControlSet\Enum\` + a.PnPDeviceID + `\Device Parameters\Interrupt Management`
						ops = append(ops,
							tweaks.Reg(base+`\MessageSignaledInterruptProperties`, "MSISupported", sys.DWord(1)),
						)
					}
				}
				return ops
			},
		},
		{
			ID: "fix.nic.speed", Category: "correcoes", Title: "Correção: velocidade da placa de rede em automático",
			Summary: "Volta \"Velocidade e duplex\" da placa Ethernet para a negociação automática, o padrão do driver.",
			Detail: "*SpeedDuplex = 0 (Auto Negotiation). Fixar a velocidade não reduz a latência: a norma do gigabit por cabo (1000BASE-T) exige autonegociação, então fixar em 1 Gbps só tira a adaptação do link (com um cabo ruim, a placa perde a conexão em vez de cair para 100 Mb/s). " +
				"Fixada em 10 ou 100 Mb/s, ainda limita a velocidade da internet. O adaptador é reiniciado (queda de 2–5 s).",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcSpeedDuplex, srcAutoneg},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"rede", "estabilidade"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if forcedSpeed(f) == "" {
					return false, "a velocidade está em automático (padrão) em todas as placas com cabo"
				}
				return true, ""
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if v, up := forcedSpeedOf(f); slowSpeed(v) {
					if !up {
						return tweaks.Rec(tweaks.RecYes, "uma placa com cabo, hoje desconectada, está fixada em 10 ou 100 Mb/s: vai limitar a internet quando for usada")
					}
					return tweaks.Rec(tweaks.RecYes, "a placa está fixada em 10 ou 100 Mb/s, o que limita a velocidade da internet")
				}
				return tweaks.Rec(tweaks.RecNeutral, "fixada em 1 Gbps: não ganha latência; o automático só evita perder a conexão se o cabo tiver defeito")
			},
			Ops:  func(f *facts.Facts) []tweaks.Op { return nicKeywordOps(f, []string{"*SpeedDuplex"}, "0", true) },
			Post: restartAdapters(true),
		},
		{
			ID: "fix.nic.defaults", Category: "correcoes", Title: "Correção: placas de rede no padrão do driver",
			Summary: "Volta ao padrão do fabricante as opções avançadas das placas de rede (cabo e Wi-Fi) que foram alteradas, como offloads, controle de fluxo, roaming e banda preferida.",
			Detail: "Grava em cada opção alterada o valor padrão que o próprio driver declara (Ndi\\Params\\<opção>\\default), o mesmo que o comando Reset-NetAdapterAdvancedProperty do Windows faz, em todas as placas (conectadas ou não). " +
				"Ficam de fora o endereço MAC, a VLAN e os jumbo frames (configuração da sua rede), o Wake-on-LAN e as opções que só valem com o PC em repouso ou desligado (PME, velocidade reduzida ao desligar), e as que o Axion trata em itens próprios (EEE, moderação de interrupção e velocidade). " +
				"Nenhuma dessas opções tem ganho de ping comprovado: volte ao padrão se a conexão cai ou oscila depois de mexer nelas. A placa alterada é reiniciada (queda de 2–5 s); desfazer devolve os valores anteriores.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcNicReset, srcNicParams, srcIntelWifiAdv},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"rede", "estabilidade"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if len(nicDefaultOps(f)) == 0 {
					return false, "as opções avançadas das placas de rede estão no padrão do driver"
				}
				return true, ""
			},
			Recommend: neutral("volta ao que o fabricante testou; útil se a conexão cai ou oscila depois de mudar essas opções, sem ganho de ping comprovado"),
			Ops:       nicDefaultOps,
			Post:      restartAdapters(false),
		},
		{
			ID: "net.tcp.defaults", Category: "correcoes", Title: "Correção: restaurar TCP padrão (autotuning e RSS)",
			Summary:  "Religa o ajuste automático da janela TCP e o Receive Side Scaling, que alguns \"otimizadores de ping\" desligam e que reduzem a velocidade de download.",
			Detail:   "netsh int tcp set global autotuninglevel=normal rss=enabled. Com autotuning desligado a janela TCP fica em 64 KB e downloads (Steam, atualizações) ficam limitados em conexões rápidas; sem RSS todo o tráfego é processado em um único núcleo.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcTCPSettings, srcRSS, srcIntModTuning},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"rede"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !f.Net.Readable {
					return false, "não foi possível ler a configuração TCP"
				}
				if f.Net.TCPAutoTuning == "normal" && f.Net.RSS == "enabled" {
					return false, "já está no padrão correto"
				}
				return true, ""
			},
			Recommend: yes("a configuração TCP deste PC foi alterada em relação ao padrão"),
			Ops:       func(*facts.Facts) []tweaks.Op { return []tweaks.Op{tcpDefaultsOp()} },
		},
	}
}

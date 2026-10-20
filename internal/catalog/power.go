package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

func schemeName(guid string) string {
	switch strings.ToLower(guid) {
	case sys.SchemeBalanced:
		return "Equilibrado"
	case sys.SchemeHighPerformance:
		return "Alto desempenho"
	case sys.SchemePowerSaver:
		return "Economia de energia"
	case sys.SchemeUltimateTemplate:
		return "Desempenho Máximo"
	}
	return guid
}

// cpuNeedsBalanced reports machines where the Balanced plan is part of how
// the hardware is meant to work: dual-CCD X3D (the AMD driver parks the CCD
// without 3D V-Cache through Balanced + Game Mode) and laptops. (Intel
// hybrid CPUs are not listed: Intel says no power-plan change is needed.)
func cpuNeedsBalanced(f *facts.Facts) (bool, string) {
	switch {
	case f.CPU.IsDualCCDX3D:
		return true, "no " + f.CPU.Name + " o driver da AMD estaciona o CCD sem 3D V-Cache usando o plano Equilibrado + Modo de Jogo; outros planos atrapalham esse mecanismo"
	case f.Power.IsLaptop:
		return true, "em notebook o plano Equilibrado com o Modo de energia \"Melhor desempenho\" na tomada entrega o mesmo desempenho sem desperdiçar bateria"
	}
	return false, ""
}

func powerTweaks() []*tweaks.Tweak {
	return []*tweaks.Tweak{
		{
			ID: "power.plan.balanced", Category: "energia", Title: "Plano de energia Equilibrado",
			Summary: "Ativa o plano Equilibrado. Nos Ryzen X3D de 2 CCDs ele faz parte do mecanismo que manda o jogo para o CCD com 3D V-Cache; em notebooks é o plano correto para jogos.",
			Detail: "PowerSetActiveScheme(381b4222-f694-41f0-9685-ff5bb260df2e). O Equilibrado sobe o clock sob demanda em milissegundos; os planos de alto desempenho mantêm a frequência mínima alta, gastando mais energia e calor. " +
				"Fora dos X3D de 2 CCDs, a diferença em jogos, quando existe, é pequena: meça antes de trocar.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcX3D, srcX3DGuide, srcPowerOpts},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"estabilidade"}, OneClick: true, Group: "power.plan",
			Applicable: func(f *facts.Facts) (bool, string) {
				if len(f.Power.Schemes) > 0 && !f.Power.HasScheme(sys.SchemeBalanced) {
					return false, "o plano Equilibrado não existe neste Windows"
				}
				return true, ""
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if need, why := cpuNeedsBalanced(f); need {
					return tweaks.Rec(tweaks.RecYes, why)
				}
				return tweaks.Rec(tweaks.RecNeutral, "no seu processador qualquer plano funciona; o Equilibrado gasta menos energia com o mesmo desempenho em jogo")
			},
			Ops: func(*facts.Facts) []tweaks.Op { return []tweaks.Op{tweaks.Scheme(sys.SchemeBalanced, "Equilibrado")} },
		},
		{
			ID: "power.plan.high", Category: "energia", Title: "Plano de energia Alto desempenho",
			Summary:  "Ativa o plano Alto desempenho: processador com frequência mínima de 100% e sem economia de energia em PCIe/USB.",
			Detail:   "PowerSetActiveScheme(8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c). Pode reduzir microatrasos de subida de clock em CPUs antigas; em CPUs atuais o efeito medido em FPS é pequeno. Aumenta consumo e temperatura em repouso.",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcPowerOpts},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"fps"}, OneClick: false, Group: "power.plan",
			Applicable: func(f *facts.Facts) (bool, string) {
				if len(f.Power.Schemes) > 0 && !f.Power.HasScheme(sys.SchemeHighPerformance) {
					return false, "este Windows usa apenas o plano Equilibrado (Modern Standby); use o Modo de energia \"Melhor desempenho\""
				}
				return true, ""
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if need, why := cpuNeedsBalanced(f); need {
					return tweaks.Rec(tweaks.RecNo, why)
				}
				return tweaks.Rec(tweaks.RecNeutral, "ganho pequeno em CPUs modernas; compare no seu jogo (1% low) antes de manter")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Scheme(sys.SchemeHighPerformance, "Alto desempenho")}
			},
		},
		{
			ID: "power.plan.ultimate", Category: "energia", Title: "Plano Desempenho Máximo (Ultimate)",
			Summary:  "Cria e ativa o plano oculto \"Desempenho Máximo\" da Microsoft, que remove micro-economias de energia além do Alto desempenho.",
			Detail:   "PowerDuplicateScheme(e9a42b02-d5df-448d-aa00-03f14749eb61) e ativação da cópia. Pensado pela Microsoft para estações de trabalho. Mesmo trade-off do Alto desempenho, com consumo ainda maior em repouso. Ao desfazer, o plano anterior volta a ser o ativo e a cópia criada pelo Axion é apagada.",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcPowerOpts},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"fps"}, OneClick: false, Group: "power.plan",
			Applicable: func(f *facts.Facts) (bool, string) {
				if f.Power.IsLaptop {
					return false, "não indicado para notebook"
				}
				return true, ""
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if need, why := cpuNeedsBalanced(f); need {
					return tweaks.Rec(tweaks.RecNo, why)
				}
				return tweaks.Rec(tweaks.RecNeutral, "ganho pequeno e maior consumo; compare no seu jogo antes de manter")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Scheme(sys.SchemeUltimateTemplate, "Desempenho Máximo")}
			},
		},
		{
			ID: "power.mode.best", Category: "energia", Title: "Modo de energia \"Melhor desempenho\" (na tomada)",
			Summary:  "Liga o modo \"Melhor desempenho\" do Windows quando o PC está na tomada, mantendo o plano Equilibrado.",
			Detail:   "Equivale a Configurações > Sistema > Energia > Modo de energia = Melhor desempenho (sobreposição ded574b5-45a0-4f42-8737-46345c09c238 sobre o plano Equilibrado). Só altera o comportamento na tomada; o que cada modo muda é definido pelo fabricante do PC.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcPowerSlider, srcPowerOpts},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"fps"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if !strings.EqualFold(f.Power.ActiveScheme, sys.SchemeBalanced) {
					return false, "o Modo de energia só existe com o plano Equilibrado ativo"
				}
				if f.Power.OnBattery {
					// The overlay is set for the current power source.
					return false, "conecte o carregador: o modo é aplicado para quando o notebook está na tomada"
				}
				return true, ""
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if f.Power.IsLaptop || f.CPU.IsDualCCDX3D {
					return tweaks.Rec(tweaks.RecYes, "mantém o plano Equilibrado exigido pelo seu hardware, mas com o perfil de desempenho máximo na tomada")
				}
				return tweaks.Rec(tweaks.RecYes, "desempenho máximo na tomada sem abandonar o plano Equilibrado")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{overlayOp(sys.OverlayBestPerformance, "Melhor desempenho")}
			},
		},
		{
			ID: "power.coreparking", Category: "energia", Title: "Desativar estacionamento de núcleos",
			Summary: "Mantém todos os núcleos da CPU acordados na tomada (mínimo de núcleos ativos = 100%).",
			Detail: "Configuração oculta CPMINCORES (54533251…\\0cc5b647…) = 100 no plano ativo, só na tomada. O Windows já desestaciona núcleos em milissegundos; o ganho medido é pequeno e aparece mais em CPUs antigas. " +
				"Nunca use em Ryzen X3D de 2 CCDs: o estacionamento é exatamente o mecanismo que manda os jogos para o CCD com 3D V-Cache.",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcCoreParking, srcX3D},
			Risk: tweaks.RiskModerate, Reboot: tweaks.RebootNone, Tags: []string{"latencia"}, OneClick: false,
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				switch {
				case f.CPU.IsDualCCDX3D:
					return tweaks.Rec(tweaks.RecBlocked, "quebraria o direcionamento de jogos para o CCD com 3D V-Cache do seu "+f.CPU.Name)
				case f.CPU.IsIntelHybrid:
					return tweaks.Rec(tweaks.RecNo, "em CPUs Intel híbridas o estacionamento faz parte de como o Windows reparte o trabalho entre P-cores e E-cores; não há medição de ganho")
				case f.Power.IsLaptop:
					return tweaks.Rec(tweaks.RecNo, "aumenta consumo e temperatura em notebook")
				}
				return tweaks.Rec(tweaks.RecNeutral, "ganho pequeno em CPUs atuais; mantenha só se o 1% low melhorar no seu jogo")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.PowerSetting("Mínimo de núcleos ativos (CPMINCORES)", tweaks.SubProcessor, tweaks.SetCoreParkMin, tweaks.U32(100), nil)}
			},
		},
		{
			ID: "power.usb.suspend", Category: "energia", Title: "Suspensão seletiva de USB desligada",
			Summary: "Desliga a suspensão seletiva USB do plano de energia (na tomada). Mouse e teclado já não são suspensos por padrão; isto afeta os outros dispositivos USB e só ajuda se algum deles desconecta.",
			Detail: "Configuração do plano ativo \"USB selective suspend\" (2a737441…\\48e6b7a6…) = Desativado na tomada. A Microsoft deixa a suspensão seletiva desligada por padrão para mouse e teclado USB (para não haver atraso ao retomar) e recomenda não desativá-la no geral. " +
				"Para um mouse ou teclado cujo driver liga a suspensão, o Axion tem o ajuste específico em Mouse e teclado.",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcHIDSuspend, srcUSBSuspend},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"latencia", "entrada"}, OneClick: true,
			Recommend: neutral("só se um dispositivo USB (áudio, webcam, controle) desconecta sozinho; mouse e teclado já ficam fora da suspensão"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.PowerSetting("Suspensão seletiva de USB", tweaks.SubUSB, tweaks.SetUSBSuspend, tweaks.U32(0), nil)}
			},
		},
		{
			ID: "power.pcie.aspm", Category: "energia", Title: "Economia de energia do PCI Express desligada",
			Summary:  "Desliga o Link State Power Management (ASPM) na tomada, mantendo o link PCIe da GPU/SSD sempre em potência total.",
			Detail:   "Configuração do plano ativo (501a4d13…\\ee12f906…) = Desligado na tomada. O ganho em jogos é pequeno e depende da plataforma; às vezes resolve microtravadas causadas por transições de estado do link.",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcASPM},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"latencia"}, OneClick: true,
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if f.Power.IsLaptop {
					return tweaks.Rec(tweaks.RecNeutral, "no notebook só vale na tomada (já é o que este ajuste faz)")
				}
				return tweaks.Rec(tweaks.RecNeutral, "efeito pequeno; inofensivo em desktop")
			},
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.PowerSetting("PCI Express: Link State Power Management", tweaks.SubPCIe, tweaks.SetASPM, tweaks.U32(0), nil)}
			},
		},
		{
			ID: "power.hibernate", Category: "energia", Title: "Hibernação e Inicialização Rápida desligadas",
			Summary:  "Remove o arquivo de hibernação (libera GBs no SSD) e desliga a Inicialização Rápida, garantindo que \"Desligar\" reinicie drivers de verdade.",
			Detail:   "powercfg /hibernate off. A Inicialização Rápida salva o kernel e os drivers em disco ao desligar; desligá-la evita problemas de drivers que nunca reiniciam e de atualizações pendentes. O boot fica alguns segundos mais lento.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcHibernate, srcPowerStates},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"estabilidade", "espaco"}, OneClick: false,
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if f.Power.IsLaptop {
					return tweaks.Rec(tweaks.RecNo, "em notebook a hibernação protege seu trabalho quando a bateria acaba")
				}
				if !f.Power.HibernateOn {
					return tweaks.Rec(tweaks.RecNeutral, "a hibernação já está desligada")
				}
				return tweaks.Rec(tweaks.RecNeutral, "útil em desktop; libera espaço equivalente a ~40% da RAM")
			},
			Ops: func(*facts.Facts) []tweaks.Op { return []tweaks.Op{hibernateOp()} },
		},
	}
}

// ---------------------------------------------------------------------------
// Power mode overlay op
// ---------------------------------------------------------------------------

type overlaySnap struct {
	Overlay string `json:"overlay"`
}

// errOverlayOnBattery is the answer of the power mode op on battery.
var errOverlayOnBattery = errors.New("conecte o carregador: o modo de energia é lido e gravado para a fonte em uso, e este ajuste é o da tomada")

// overlayNeedsAC refuses the power mode op while a notebook runs on battery.
// A PC without a battery (or whose state is unknown) counts as on AC.
func overlayNeedsAC(c *tweaks.Ctx) error {
	if hasBat, onAC, _, err := c.P.Native.PowerStatus(); err == nil && hasBat && !onAC {
		return errOverlayOnBattery
	}
	return nil
}

func overlayOp(guid, label string) tweaks.Op {
	norm := func(s string) string {
		s = strings.ToLower(strings.Trim(s, "{} "))
		if s == "" {
			return sys.OverlayBalanced
		}
		return s
	}
	return &tweaks.FuncOp{
		KindName: "power-overlay", ID: "ac", Spec: overlaySpec{GUID: guid, Label: label},
		Desc: fmt.Sprintf("Modo de energia na tomada → \"%s\" (%s)", label, guid),
		// Windows reads and writes the power mode of the power source in use
		// (the effective overlay). The op is about the mode on AC, so on a
		// notebook running on battery it refuses to read or write: undoing
		// there would write the battery's mode and leave the AC one changed.
		SnapFn: func(c *tweaks.Ctx) (any, error) {
			if err := overlayNeedsAC(c); err != nil {
				return nil, err
			}
			o, err := c.P.Power.ActiveOverlay()
			return overlaySnap{Overlay: norm(o)}, err
		},
		InFn: func(c *tweaks.Ctx) (bool, error) {
			if err := overlayNeedsAC(c); err != nil {
				return false, err
			}
			o, err := c.P.Power.ActiveOverlay()
			return norm(o) == norm(guid), err
		},
		ApplyFn: func(c *tweaks.Ctx) error {
			if err := overlayNeedsAC(c); err != nil {
				return err
			}
			return c.P.Power.SetActiveOverlay(guid)
		},
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var s overlaySnap
			if err := json.Unmarshal(raw, &s); err != nil {
				return err
			}
			if err := overlayNeedsAC(c); err != nil {
				return err
			}
			return c.P.Power.SetActiveOverlay(norm(s.Overlay))
		},
		CurFn: func(c *tweaks.Ctx) string {
			o, err := c.P.Power.ActiveOverlay()
			if err != nil {
				return "erro: " + err.Error()
			}
			switch norm(o) {
			case sys.OverlayBestPerformance:
				return "Melhor desempenho"
			case sys.OverlayBestEfficiency:
				return "Melhor eficiência de energia"
			case sys.OverlayBalanced:
				return "Equilibrado (padrão)"
			}
			return o
		},
	}
}

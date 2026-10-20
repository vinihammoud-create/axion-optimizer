package diag

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// Thermal signals Windows documents and exposes without a hardware driver.
// None of them is the processor's own temperature (that needs a driver,
// which Axion does not load): they say whether Windows itself shut the PC
// down or slowed it down for heat.
const (
	// ChanThermal is the channel of the passive cooling events (114).
	ChanThermal    = "Microsoft-Windows-Kernel-Power/Thermal-Operational"
	provKProcPower = "Microsoft-Windows-Kernel-Processor-Power"
)

var (
	srcThermalDiag = tweaks.Source{Title: "Microsoft Learn — Diagnóstico térmico no Windows: eventos 125, 86 e 114 e os contadores das zonas térmicas", URL: "https://learn.microsoft.com/windows-hardware/design/device-experiences/examples--requirements-and-diagnostics"}
	srcEvent37     = tweaks.Source{Title: "Microsoft Learn — Evento 37 (Kernel-Processor-Power): o firmware não deixa o Windows usar parte da faixa de frequência do processador", URL: "https://learn.microsoft.com/troubleshoot/windows-server/setup-upgrade-and-drivers/event-id-37-windows-kernel-processor-power"}
	srcProcPower   = tweaks.Source{Title: "Microsoft Learn — PROCESSOR_POWER_INFORMATION: MhzLimit é o limite de frequência por temperatura", URL: "https://learn.microsoft.com/windows/win32/power/processor-power-information-str"}
)

// FirmwareLimitDays is Axion's criterion for the firmware limit notice: the
// event shows up on that many different days of the last 30.
const FirmwareLimitDays = 3

// coolingAdvice is Axion's advice (not Microsoft's) for a PC that gets too
// hot, by form factor.
func coolingAdvice(f *facts.Facts) string {
	if f != nil && f.IsLaptop() {
		return "No notebook: limpe as saídas de ar (poeira acumulada é a causa mais comum), use-o numa superfície rígida, nunca sobre cama ou almofada, e, se jogar muito, uma base com ventilação ajuda. O modo de desempenho do utilitário do fabricante também muda o quanto ele esquenta. Se continuar, a pasta térmica pode estar ressecada: isso é serviço de assistência técnica."
	}
	return "Limpe a poeira do gabinete e do cooler do processador, confira se as ventoinhas giram e se o ar entra pela frente e sai por trás ou por cima. Se continuar, a pasta térmica do processador pode estar ressecada ou o cooler mal encaixado."
}

// distinctDays counts the different days in a list of times.
func distinctDays(ts []time.Time) int {
	seen := map[string]bool{}
	for _, t := range ts {
		seen[t.Format("2006-01-02")] = true
	}
	return len(seen)
}

func latestOf(ts []time.Time) time.Time {
	sort.Slice(ts, func(i, j int) bool { return ts[i].After(ts[j]) })
	return ts[0]
}

// thermalFindings reads the thermal events of the last 30 days and the
// thermal state at the time of the scan.
func thermalFindings(evs []sys.Event, f *facts.Facts) []Finding {
	var shutdowns, cooling, firmware []time.Time
	for _, e := range evs {
		switch {
		case strings.EqualFold(e.Provider, provKPower) && e.ID == 86:
			shutdowns = append(shutdowns, e.Time)
		case e.ID == 114 && strings.EqualFold(e.Channel, ChanThermal):
			cooling = append(cooling, e.Time)
		case strings.EqualFold(e.Provider, provKProcPower) && e.ID == 37:
			firmware = append(firmware, e.Time)
		}
	}
	var out []Finding
	if len(shutdowns) > 0 {
		out = append(out, Finding{ID: "therm.shutdown", Area: AreaEstab, Severity: SevCritical,
			Title:       plural(len(shutdowns), "desligamento por temperatura crítica", "desligamentos por temperatura crítica"),
			Evidence:    "Kernel-Power 86 (\"o sistema foi desligado por um evento térmico crítico\"); último em " + latestOf(shutdowns).Format("02/01/2006 15:04") + ".",
			Explanation: "O Windows registra esse evento depois que o PC desliga sozinho para se proteger do calor. É o sinal mais forte de superaquecimento que o Windows dá: jogar assim arrisca travamentos e perda do que não foi salvo. " + coolingAdvice(f),
			Sources:     []tweaks.Source{srcThermalDiag}})
	}
	if n := distinctDays(cooling); n > 0 {
		out = append(out, Finding{ID: "therm.throttle", Area: AreaDesempenho, Severity: SevWarning,
			Title:       fmt.Sprintf("O Windows reduziu o desempenho por temperatura em %s dos últimos 30", plural(n, "dia", "dias")),
			Evidence:    fmt.Sprintf("Thermal-Operational 114 (uma zona térmica ligou ou desligou o resfriamento passivo): %s, o último em %s.", plural(len(cooling), "registro", "registros"), latestOf(cooling).Format("02/01/2006 15:04")),
			Explanation: "Segundo a Microsoft, esse evento confirma que houve throttling térmico: quando a zona esquenta demais, o Windows limita o desempenho dos dispositivos dela para baixar a temperatura. No jogo isso aparece como queda de FPS depois de alguns minutos, sem nenhuma configuração ter mudado. " + coolingAdvice(f),
			Sources:     []tweaks.Source{srcThermalDiag}})
	}
	if f != nil {
		th := f.Thermal
		var now []string
		for _, z := range th.ThrottledNow() {
			s := fmt.Sprintf("zona %s a %d%% (100%% = sem limite)", z.Name, z.PassiveLimitPct)
			if z.TempC >= 0 {
				s += fmt.Sprintf(", %.0f °C", z.TempC)
			}
			now = append(now, s)
		}
		if th.LimitedNow() {
			now = append(now, fmt.Sprintf("limite de frequência por temperatura do processador em %d de %d MHz", th.MhzLimit, th.MaxMhz))
		}
		if len(now) > 0 {
			out = append(out, Finding{ID: "therm.now", Area: AreaDesempenho, Severity: SevWarning,
				Title:       "O Windows está limitando o desempenho por temperatura agora",
				Evidence:    "Na hora da análise: " + strings.Join(now, "; ") + ".",
				Explanation: "São os contadores das zonas térmicas e o limite de frequência por temperatura que o próprio Windows informa (não é a temperatura do núcleo do processador, que só um driver lê). Com o PC parado isso não deveria acontecer. " + coolingAdvice(f),
				Sources:     []tweaks.Source{srcThermalDiag, srcProcPower}})
		}
	}
	if n := distinctDays(firmware); n >= FirmwareLimitDays {
		why := "As causas mais comuns são temperatura alta, um limite de energia do fabricante ou uma opção de energia na BIOS."
		if f != nil && f.IsLaptop() {
			why += " Em notebook, também aparece na bateria e nos modos de economia do utilitário do fabricante: se acontece na tomada e jogando, olhe a temperatura."
		}
		out = append(out, Finding{ID: "therm.firmware", Area: AreaDesempenho, Severity: SevInfo,
			Title:       fmt.Sprintf("O firmware limitou a velocidade do processador em %s dos últimos 30", plural(n, "dia", "dias")),
			Evidence:    fmt.Sprintf("Kernel-Processor-Power 37 (\"a velocidade do processador está sendo limitada pelo firmware do sistema\"): %s, o último em %s.", plural(len(firmware), "registro", "registros"), latestOf(firmware).Format("02/01/2006 15:04")),
			Explanation: "Segundo a Microsoft, o evento é gravado quando a plataforma decide que o Windows não pode usar parte da faixa de frequência do processador. Ele não diz a causa. " + why + fmt.Sprintf(" Avisar a partir de %d dias diferentes é critério do Axion.", FirmwareLimitDays),
			Sources:     []tweaks.Source{srcEvent37}})
	}
	return out
}

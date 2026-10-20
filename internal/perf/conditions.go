package perf

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Conditions are the state of the PC when a capture started: the things that
// change the FPS without any setting having changed, and that make a
// "before" and an "after" taken in different conditions mean nothing.
type Conditions struct {
	At          time.Time `json:"at"`
	HasBattery  bool      `json:"hasBattery"`
	OnBattery   bool      `json:"onBattery"`
	BatteryPct  int       `json:"batteryPct"`            // -1 unknown
	PowerScheme string    `json:"powerScheme,omitempty"` // name of the active plan
	PowerSaver  bool      `json:"powerSaver,omitempty"`  // the plan is Windows' "Power saver"
	PowerMode   string    `json:"powerMode,omitempty"`   // "Melhor desempenho", "Equilibrado" or "Melhor eficiência energética"
	Efficiency  bool      `json:"efficiency,omitempty"`  // the Windows power mode is "Best power efficiency"
	DisplayHz   int       `json:"displayHz"`             // 0 unknown
	Busy        []Busy    `json:"busy,omitempty"`        // other programs that used the processor during the recording
}

// Busy is another program's use of the processor during the recording, in
// percent of ONE logical processor (100 = one full core).
type Busy struct {
	Name string  `json:"name"`
	Pct  float64 `json:"pct"`
}

// BusyMinPct is the use, in percent of one core, from which another program
// counts as busy while a game is being measured.
const BusyMinPct = 25.0

// Sensors are what was read during the recording.
type Sensors struct {
	GPU *GPUSensors `json:"gpu,omitempty"`
	CPU *CPUSensors `json:"cpu,omitempty"`
}

// GPUSensors summarize the NVIDIA readings (nvidia-smi, once a second).
type GPUSensors struct {
	Samples       int     `json:"samples"`
	ThermalPct    float64 `json:"thermalPct"`    // % of the readings with a thermal slowdown active (hardware or software)
	HWSlowdownPct float64 `json:"hwSlowdownPct"` // % with the hardware slowdown or the power brake active
	PowerCapPct   float64 `json:"powerCapPct"`   // % with the software power cap active
	AvgTempC      float64 `json:"avgTempC"`
	MaxTempC      int     `json:"maxTempC"`
	AvgClockMHz   int     `json:"avgClockMHz"`
	MaxClockMHz   int     `json:"maxClockMHz"`   // highest clock read
	RatedClockMHz int     `json:"ratedClockMHz"` // the card's maximum clock (0 unknown)
	AvgPowerW     float64 `json:"avgPowerW"`     // -1 unknown
	AvgUtilPct    float64 `json:"avgUtilPct"`
}

// CPUSensors summarize the processor readings (a performance counter and the
// system's processor times, once a second).
type CPUSensors struct {
	Samples    int     `json:"samples"`
	AvgFreqPct float64 `json:"avgFreqPct"` // frequency as a share of the maximum (base) frequency
	MinFreqPct float64 `json:"minFreqPct"`
	AvgUtilPct float64 `json:"avgUtilPct"` // use of all logical processors
	// BusyFreqPct is the average frequency over the readings in which the
	// processors were at least BusyUtilPct busy (-1 when none were).
	BusyFreqPct float64 `json:"busyFreqPct"`
	BusySamples int     `json:"busySamples"`
	// LimitSamples are the readings with the thermal frequency limit
	// (CallNtPowerInformation MhzLimit); ThermalLimitPct is the share of them
	// in which Windows held the processors below their maximum for heat, and
	// MinLimitPct the lowest limit read (share of the maximum).
	LimitSamples    int     `json:"limitSamples,omitempty"`
	ThermalLimitPct float64 `json:"thermalLimitPct,omitempty"`
	MinLimitPct     float64 `json:"minLimitPct,omitempty"`
}

// ThermalLimitMinPct is Axion's criterion for the thermal limit remark: the
// share of the readings with Windows' thermal frequency limit below the
// maximum.
const ThermalLimitMinPct = 10.0

// BusyUtilPct is the use of the processors from which a low frequency says
// something: at idle it is low on purpose.
const BusyUtilPct = 60.0

// BelowBasePct is the frequency (share of the base frequency) under which a
// busy processor is being held back by heat or a power limit.
const BelowBasePct = 80.0

// Remark is something worth knowing about a capture's conditions.
type Remark struct {
	Level string `json:"level"` // warn | info
	Title string `json:"title"`
	Text  string `json:"text,omitempty"`
}

func comma(v float64, dec int) string {
	return strings.Replace(fmt.Sprintf("%.*f", dec, v), ".", ",", 1)
}

// Remarks reads the conditions and the sensors of a run. It says only what
// the numbers support, and nothing when nothing was read.
func Remarks(r *Run) []Remark {
	var out []Remark
	warn := func(t, x string) { out = append(out, Remark{"warn", t, x}) }
	info := func(t, x string) { out = append(out, Remark{"info", t, x}) }
	if c := r.Conditions; c != nil {
		if c.OnBattery {
			warn("O PC estava na bateria", "Na bateria o notebook limita a potência da CPU e da GPU: este resultado não vale para jogar na tomada e não deve ser comparado com medições feitas na tomada.")
		}
		if c.PowerSaver {
			warn("O plano de energia era \"Economia de energia\"", "Esse plano reduz o desempenho do processador para poupar energia: meça de novo com um plano equilibrado ou de alto desempenho.")
		}
		if c.Efficiency {
			warn("O modo de energia do Windows era \"Melhor eficiência energética\"", "Esse modo reduz o desempenho para poupar energia: meça de novo em \"Equilibrado\" ou \"Melhor desempenho\".")
		}
		for i, b := range c.Busy {
			if i >= 3 {
				break
			}
			warn(fmt.Sprintf("%s usou cerca de %s%% de um núcleo durante a medição", b.Name, comma(b.Pct, 0)), "Programas em segundo plano disputam o processador com o jogo e mudam o resultado de uma medição para outra. Feche-o e meça de novo para comparar em condições iguais.")
		}
	}
	if g := r.Sensors; g != nil && g.GPU != nil && g.GPU.Samples >= 3 {
		s := g.GPU
		if s.ThermalPct >= 10 {
			warn(fmt.Sprintf("A placa de vídeo reduziu o clock por temperatura em %s%% da medição", comma(s.ThermalPct, 0)),
				fmt.Sprintf("Chegou a %d °C. Com a placa quente o FPS cai sem que nenhuma configuração tenha mudado: limpe a poeira, melhore a ventilação ou, no notebook, use uma base com ventilação e o modo de desempenho do fabricante.", s.MaxTempC))
		}
		if s.HWSlowdownPct >= 5 {
			warn(fmt.Sprintf("A placa de vídeo acionou o freio de hardware em %s%% da medição", comma(s.HWSlowdownPct, 0)),
				"O freio de hardware (hw slowdown ou power brake) é um limite de proteção da própria placa, por temperatura, energia ou cabo de alimentação. Se aparecer sempre, vale conferir a fonte, os cabos e a temperatura.")
		}
		if s.PowerCapPct >= 50 && s.ThermalPct < 10 && s.HWSlowdownPct < 5 {
			info(fmt.Sprintf("A placa de vídeo ficou no limite de potência em %s%% da medição", comma(s.PowerCapPct, 0)),
				"Isso quer dizer que ela trabalhou com toda a potência que o fabricante permite, o que é comum quando o jogo usa a GPU ao máximo, principalmente em notebook. Não é um defeito.")
		}
		clock := fmt.Sprintf("Clock médio de %d MHz", s.AvgClockMHz)
		if s.RatedClockMHz > 0 {
			clock += fmt.Sprintf(" (máximo da placa: %d MHz)", s.RatedClockMHz)
		}
		info(fmt.Sprintf("Placa de vídeo: %s, %s °C de média, %d °C no máximo", clock, comma(s.AvgTempC, 0), s.MaxTempC), "Lido pelo nvidia-smi uma vez por segundo durante a gravação.")
	}
	if g := r.Sensors; g != nil && g.CPU != nil && g.CPU.Samples >= 3 {
		s := g.CPU
		if s.LimitSamples >= 3 && s.ThermalLimitPct >= ThermalLimitMinPct {
			warn(fmt.Sprintf("O Windows limitou a frequência do processador por temperatura em %s%% da medição", comma(s.ThermalLimitPct, 0)),
				fmt.Sprintf("É o limite térmico que o próprio Windows informa (chegou a %s%% da frequência máxima). Com o processador limitado por calor o FPS cai sem nenhuma configuração ter mudado: limpe a poeira e melhore a ventilação; no notebook, use uma base com ventilação e o modo de desempenho do fabricante. Avisar a partir de %s%% das leituras é critério do Axion.", comma(s.MinLimitPct, 0), comma(ThermalLimitMinPct, 0)))
		}
		if s.BusySamples >= 3 && s.BusyFreqPct >= 0 && s.BusyFreqPct < BelowBasePct {
			warn(fmt.Sprintf("O processador trabalhou abaixo da frequência base (%s%%) enquanto estava ocupado", comma(s.BusyFreqPct, 0)),
				"Com o processador em uso, uma frequência abaixo da base costuma indicar limite de temperatura ou de energia (comum em notebook, e também de plano de energia ou de BIOS). Em uso leve a frequência cai de propósito; aqui ele estava ocupado.")
		} else {
			info(fmt.Sprintf("Processador: %s%% de uso, frequência média de %s%% da base", comma(s.AvgUtilPct, 0), comma(s.AvgFreqPct, 0)),
				"Acima de 100% é o turbo. Abaixo de 100% com o processador ocupado pode ser limite de temperatura ou de energia; em uso leve é normal.")
		}
	}
	return out
}

// CompareConditions lists what differs between the conditions of two groups
// of captures (the first run of each is read): a difference here can explain
// a change in the FPS better than the setting that was changed.
func CompareConditions(a, b *Conditions) []string {
	if a == nil || b == nil {
		return nil
	}
	var out []string
	if a.OnBattery != b.OnBattery {
		out = append(out, "Uma das medições foi feita na bateria e a outra na tomada: o FPS muda por isso, não pela configuração.")
	}
	if (a.PowerSaver || a.Efficiency) != (b.PowerSaver || b.Efficiency) {
		out = append(out, "O plano ou o modo de energia era de economia em apenas uma das medições.")
	} else if a.PowerScheme != "" && b.PowerScheme != "" && a.PowerScheme != b.PowerScheme {
		out = append(out, fmt.Sprintf("O plano de energia mudou (%s para %s).", a.PowerScheme, b.PowerScheme))
	} else if a.PowerMode != "" && b.PowerMode != "" && a.PowerMode != b.PowerMode {
		out = append(out, fmt.Sprintf("O modo de energia do Windows mudou (%s para %s).", a.PowerMode, b.PowerMode))
	}
	if a.DisplayHz > 0 && b.DisplayHz > 0 && a.DisplayHz != b.DisplayHz {
		out = append(out, fmt.Sprintf("A taxa de atualização do monitor mudou (%d Hz para %d Hz).", a.DisplayHz, b.DisplayHz))
	}
	names := func(c *Conditions) map[string]bool {
		m := map[string]bool{}
		for _, x := range c.Busy {
			m[x.Name] = true
		}
		return m
	}
	na, nb := names(a), names(b)
	var only []string
	for n := range na {
		if !nb[n] {
			only = append(only, n)
		}
	}
	for n := range nb {
		if !na[n] {
			only = append(only, n)
		}
	}
	if len(only) > 0 {
		sort.Strings(only)
		out = append(out, "Programas ocupando o processador em apenas uma das medições: "+strings.Join(only, ", ")+".")
	}
	return out
}

// Aggregate helpers used by the app when it turns samples into sensors.

// GPUSample is one nvidia-smi reading.
type GPUSample struct {
	Thermal, HWSlowdown, PowerCap bool
	TempC                         int
	ClockMHz, MaxClockMHz         int
	PowerW                        float64 // -1 unknown
	UtilPct                       int     // -1 unknown
}

// SummarizeGPU turns readings into sensors; nil when there are too few.
func SummarizeGPU(ss []GPUSample) *GPUSensors {
	if len(ss) < 3 {
		return nil
	}
	g := &GPUSensors{Samples: len(ss), AvgPowerW: -1}
	var temp, clock, power, util float64
	var nPower, nUtil, nClock int
	for _, s := range ss {
		if s.Thermal {
			g.ThermalPct++
		}
		if s.HWSlowdown {
			g.HWSlowdownPct++
		}
		if s.PowerCap {
			g.PowerCapPct++
		}
		temp += float64(s.TempC)
		if s.TempC > g.MaxTempC {
			g.MaxTempC = s.TempC
		}
		if s.ClockMHz > 0 {
			clock += float64(s.ClockMHz)
			nClock++
			if s.ClockMHz > g.MaxClockMHz {
				g.MaxClockMHz = s.ClockMHz
			}
		}
		if s.MaxClockMHz > g.RatedClockMHz {
			g.RatedClockMHz = s.MaxClockMHz
		}
		if s.PowerW >= 0 {
			power += s.PowerW
			nPower++
		}
		if s.UtilPct >= 0 {
			util += float64(s.UtilPct)
			nUtil++
		}
	}
	n := float64(len(ss))
	g.ThermalPct, g.HWSlowdownPct, g.PowerCapPct = g.ThermalPct/n*100, g.HWSlowdownPct/n*100, g.PowerCapPct/n*100
	g.AvgTempC = temp / n
	if nClock > 0 {
		g.AvgClockMHz = int(math.Round(clock / float64(nClock)))
	}
	if nPower > 0 {
		g.AvgPowerW = power / float64(nPower)
	}
	if nUtil > 0 {
		g.AvgUtilPct = util / float64(nUtil)
	}
	return g
}

// CPUSample is one processor reading.
type CPUSample struct {
	FreqPct float64 // share of the maximum frequency
	UtilPct float64 // use of all logical processors
	// LimitPct is Windows' thermal frequency limit as a share of the maximum
	// specified frequency (100 = no limit; -1 = not read).
	LimitPct float64
}

// SummarizeCPU turns readings into sensors; nil when there are too few.
func SummarizeCPU(ss []CPUSample) *CPUSensors {
	if len(ss) < 3 {
		return nil
	}
	c := &CPUSensors{Samples: len(ss), MinFreqPct: math.MaxFloat64, BusyFreqPct: -1}
	var freq, util, busyFreq float64
	for _, s := range ss {
		freq += s.FreqPct
		util += s.UtilPct
		if s.FreqPct < c.MinFreqPct {
			c.MinFreqPct = s.FreqPct
		}
		if s.UtilPct >= BusyUtilPct {
			busyFreq += s.FreqPct
			c.BusySamples++
		}
	}
	n := float64(len(ss))
	c.AvgFreqPct, c.AvgUtilPct = freq/n, util/n
	if c.BusySamples > 0 {
		c.BusyFreqPct = busyFreq / float64(c.BusySamples)
	}
	limited := 0
	c.MinLimitPct = 100
	for _, s := range ss {
		if s.LimitPct <= 0 {
			continue
		}
		c.LimitSamples++
		if s.LimitPct < 100 {
			limited++
		}
		if s.LimitPct < c.MinLimitPct {
			c.MinLimitPct = s.LimitPct
		}
	}
	if c.LimitSamples > 0 {
		c.ThermalLimitPct = float64(limited) / float64(c.LimitSamples) * 100
	} else {
		c.MinLimitPct = 0
	}
	return c
}

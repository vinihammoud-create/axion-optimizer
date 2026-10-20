package app

import (
	"fmt"
	"math"
	"strings"
	"time"

	"axion/internal/facts"
	"axion/internal/games"
	"axion/internal/netdiag"
	"axion/internal/perf"
	"axion/internal/periph"
	"axion/internal/tweaks"
)

// Kinds of a latency number.
const (
	latMeasured   = "medido"
	latCalculated = "calculado"
	latUnknown    = "não medido"
)

// LatencyPart is one link of the chain between the hand and the screen.
type LatencyPart struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Ms     float64 `json:"ms"`    // average; -1 = not measured
	Worst  float64 `json:"worst"` // upper bound when known; -1 = unknown
	Kind   string  `json:"kind"`  // medido | calculado | não medido
	Detail string  `json:"detail"`
	Page   string  `json:"page,omitempty"` // where it is measured
}

// LatencyLever is a change that shortens the chain, with the gain it gives.
type LatencyLever struct {
	Title  string  `json:"title"`
	GainMs float64 `json:"gainMs"` // average gain; -1 = depends on the scene
	Text   string  `json:"text"`
	Page   string  `json:"page,omitempty"`
	Done   bool    `json:"done,omitempty"` // already at the best setting
}

// LatencyView is the Atraso de entrada page: the end-to-end budget of this
// PC built from its own measurements.
type LatencyView struct {
	Mouse    []LatencyPart   `json:"mouse"` // chain of a mouse action (aim, shot)
	TotalMs  float64         `json:"totalMs"`
	Complete bool            `json:"complete"` // polling, PC and scanout known
	Keyboard *LatencyPart    `json:"keyboard,omitempty"`
	Stack    *LatencyPart    `json:"stack,omitempty"` // USB -> application, measured
	Network  *LatencyPart    `json:"network,omitempty"`
	Levers   []LatencyLever  `json:"levers"`
	Missing  []LatencyLever  `json:"missing"`
	Honest   string          `json:"honest"`
	Sources  []tweaks.Source `json:"sources"`
}

var (
	srcNvLatency  = tweaks.Source{Title: "NVIDIA — Guia de latência do sistema: periférico, PC e monitor (125 Hz soma até 3 ms em média frente a 1000 Hz)", URL: "https://www.nvidia.com/en-us/geforce/guides/system-latency-optimization-guide/"}
	srcSpjut2019  = tweaks.Source{Title: "NVIDIA Research (SIGGRAPH Asia 2019) — Reduzir a latência ajuda a mira mais do que subir a taxa do monitor acima de 60 Hz", URL: "https://research.nvidia.com/publication/2019-11_latency-30-ms-benefits-first-person-targeting-tasks-more-refresh-rate-above-60"}
	srcPMMetrics  = tweaks.Source{Title: "Intel PresentMon — definição das métricas (PC Latency, All Input to Photon, Until Displayed)", URL: "https://github.com/GameTechDev/PresentMon/blob/main/README-ConsoleApplication.md"}
	srcRtingsLag  = tweaks.Source{Title: "RTINGS — Como medem o atraso de entrada dos monitores (por modelo)", URL: "https://www.rtings.com/monitor/tests/inputs/input-lag"}
	srcReflexPage = tweaks.Source{Title: "NVIDIA — Reflex: medir e reduzir a latência nos jogos competitivos", URL: "https://www.nvidia.com/en-us/geforce/news/reflex-low-latency-platform/"}
)

// pollMs is the average and worst wait of a report at a polling rate.
func pollMs(hz float64) (avg, worst float64) {
	if hz <= 0 {
		return -1, -1
	}
	return 1000 / (2 * hz), 1000 / hz
}

func r2(v float64) float64 {
	if v < 0 {
		return v
	}
	return math.Round(v*100) / 100
}

// Latency builds the latency budget from the latest measurements.
func (a *App) Latency() (LatencyView, error) {
	f := a.Facts()
	if f == nil {
		return LatencyView{}, ErrNoScan
	}
	_, cs2 := a.Games()
	var runs []perf.Run
	if a.P != nil {
		runs = perf.List(a.perfDir())
	}
	var net *netdiag.Summary
	if list := netdiag.List(a.netDir()); len(list) > 0 {
		net = &list[0]
	}
	return buildLatency(f, cs2, a.Measurements(), runs, net), nil
}

func buildLatency(f *facts.Facts, cs2 games.CS2Info, meas []periph.Measurement, runs []perf.Run, net *netdiag.Summary) LatencyView {
	v := LatencyView{Sources: []tweaks.Source{srcNvLatency, srcSpjut2019, srcPMMetrics, srcUSBEndpoint, srcRtingsLag, srcReflexPage},
		Honest: "O atraso de entrada é a soma do caminho da mão até a tela: o mouse esperando a próxima leitura, o Windows entregando ao jogo, o jogo e a placa de vídeo produzindo o quadro e o monitor desenhando a imagem. " +
			"O Axion usa as medições deste PC (polling medido em Periféricos, latência do PresentMon em Desempenho) e calcula o que é física simples (a varredura do monitor). A resposta dos pixels do painel não dá para medir por software. " +
			"Pesquisa da NVIDIA com atletas de esports mostrou que reduzir a latência ajuda a mira mais do que subir a taxa do monitor acima de 60 Hz: por isso vale saber de onde vem cada milissegundo."}

	// 1. Mouse polling: measured, else the port's limit.
	mouse := LatencyPart{Key: "mouse", Label: "Mouse até o PC (espera pela próxima leitura)", Ms: -1, Worst: -1, Kind: latUnknown, Page: "perifericos",
		Detail: "Meça o polling na página Periféricos."}
	var mouseHz float64
	var stack *periph.Lag
	var measAt time.Time
	for _, m := range meas {
		for _, d := range m.Devices {
			if d.Primary != "mouse" || (d.Snapped <= 0 && d.MeasuredHz <= 0) {
				continue
			}
			hz := float64(d.Snapped)
			if hz <= 0 {
				hz = d.MeasuredHz
			}
			mouseHz, measAt = hz, m.At
			if d.Lag != nil && d.Lag.Samples > 0 {
				stack = d.Lag
			}
			break
		}
		if mouseHz > 0 {
			break
		}
	}
	if mouseHz > 0 {
		mouse.Ms, mouse.Worst = pollMs(mouseHz)
		mouse.Kind = latMeasured
		mouse.Detail = fmt.Sprintf("Polling de %s medido em %s: o movimento espera em média metade do intervalo entre leituras (no pior caso, o intervalo inteiro).", hzText(mouseHz), measAt.Local().Format("02/01"))
	} else if hz := nominalHz(f, "mouse"); hz > 0 {
		mouse.Ms, mouse.Worst = pollMs(hz)
		mouse.Kind = latCalculated
		mouse.Detail = fmt.Sprintf("Limite da porta USB: %s. O mouse pode estar configurado abaixo disso; meça na página Periféricos.", hzText(hz))
	}
	v.Mouse = append(v.Mouse, mouse)

	// 2. PC: input to frame sent to the display (PresentMon).
	pc := LatencyPart{Key: "pc", Label: "PC: entrada → quadro enviado à tela (Windows, jogo e placa de vídeo)", Ms: -1, Worst: -1, Kind: latUnknown, Page: "desempenho",
		Detail: "Grave uma medição do CS2 na página Desempenho: o PresentMon mede este trecho com os eventos do NVIDIA Reflex."}
	run := latestRun(runs, "cs2.exe")
	if run != nil {
		s := run.Summary
		when := run.Started.Local().Format("02/01")
		switch {
		case s.PCLatencyAvg > 0:
			pc.Ms, pc.Kind = s.PCLatencyAvg, latMeasured
			pc.Detail = fmt.Sprintf("Latência do PC medida pelo PresentMon com os eventos do Reflex (%s, %s, %s FPS médios).", run.Process, when, fmtF(s.AvgFPS, 0))
		case s.InputLatencyAvg > 0:
			pc.Ms, pc.Kind = s.InputLatencyAvg, latMeasured
			pc.Detail = fmt.Sprintf("PresentMon: da entrada registrada pelo Windows até o quadro ir para a tela (%s, %s, %s FPS médios).", run.Process, when, fmtF(s.AvgFPS, 0))
		case s.FTAvg > 0:
			pc.Detail = fmt.Sprintf("A medição de %s (%s) não trouxe os eventos de latência; o tempo de quadro médio foi de %s ms. Ligue o Reflex no jogo e meça de novo.", run.Process, when, fmtF(s.FTAvg, 2))
		}
	}
	v.Mouse = append(v.Mouse, pc)

	// 3. Scanout: the monitor redraws top to bottom once per refresh.
	hz, where := effectiveHz(f, cs2)
	scan := LatencyPart{Key: "scanout", Label: "Monitor: varredura até o centro da tela (a mira)", Ms: -1, Worst: -1, Kind: latUnknown}
	if hz > 0 {
		scan.Ms, scan.Worst = 1000/(2*hz), 1000/hz
		scan.Kind = latCalculated
		scan.Detail = fmt.Sprintf("A %s (%s) a imagem é redesenhada de cima para baixo a cada %s ms; o centro da tela recebe o quadro novo, em média, na metade disso.", hzText(hz), where, fmtF(1000/hz, 2))
	}
	v.Mouse = append(v.Mouse, scan)

	// 4. Panel: processing and pixel response.
	v.Mouse = append(v.Mouse, LatencyPart{Key: "panel", Label: "Painel: processamento e resposta dos pixels", Ms: -1, Worst: -1, Kind: latUnknown,
		Detail: "Depende do modelo e não dá para medir por software. Análises independentes (como a do RTINGS) medem o atraso de cada monitor."})

	v.Complete = mouse.Ms >= 0 && pc.Ms >= 0 && scan.Ms >= 0
	for _, p := range v.Mouse {
		if p.Ms > 0 {
			v.TotalMs += p.Ms
		}
	}
	v.TotalMs = r2(v.TotalMs)

	// Keyboard: the port limit (a keyboard is not measured by the test).
	if khz := nominalHz(f, "keyboard"); khz > 0 {
		avg, worst := pollMs(khz)
		k := LatencyPart{Key: "keyboard", Label: "Teclado até o PC", Ms: r2(avg), Worst: r2(worst), Kind: latCalculated, Page: "perifericos",
			Detail: fmt.Sprintf("A porta USB do teclado permite no máximo %s: cada tecla espera em média %s ms pela leitura.", hzText(khz), fmtF(avg, 1))}
		v.Keyboard = &k
	}

	// Windows input stack, measured by the polling test with the USB trace.
	if stack != nil {
		st := LatencyPart{Key: "stack", Label: "Windows: controlador USB → jogo", Ms: r2(stack.MedianUS / 1000), Worst: r2(stack.P95US / 1000), Kind: latMeasured, Page: "perifericos",
			Detail: fmt.Sprintf("Mediana de %s µs (95%% abaixo de %s µs) em %d relatórios do mouse, medida no teste de polling. Já está dentro do tempo do PC acima. É o trecho que \"otimizações de USB, IRQ e MSI\" prometem encurtar: não há o que ganhar nele.",
				fmtF(stack.MedianUS, 0), fmtF(stack.P95US, 0), stack.Samples)}
		v.Stack = &st
	}

	// Network, for context: it delays the action at the server, not the image.
	if net != nil && net.ValveMs >= 0 {
		n := LatencyPart{Key: "network", Label: "Rede até o relay da Valve (ida e volta)", Ms: r2(net.ValveMs), Worst: -1, Kind: latMeasured, Page: "rede",
			Detail: fmt.Sprintf("Relay de %s no teste de %s. Não atrasa a imagem na sua tela: atrasa quando o servidor recebe a sua ação (cerca de metade disso) e quando você vê a dos outros.", net.ValvePOP, net.At.Local().Format("02/01 15:04"))}
		v.Network = &n
	}

	v.Levers, v.Missing = latencyLevers(f, cs2, mouseHz, run, hz, v)
	return v
}

func latestRun(runs []perf.Run, exe string) *perf.Run {
	for i := range runs {
		if strings.EqualFold(runs[i].Process, exe) {
			return &runs[i]
		}
	}
	if len(runs) > 0 {
		return &runs[0]
	}
	return nil
}

// nominalHz is the highest polling rate the USB port of the main device of
// a kind allows (from the descriptors).
func nominalHz(f *facts.Facts, kind string) float64 {
	best := 0.0
	for _, d := range f.Peripherals.Devices {
		if d.Primary == kind && d.NominalHz > 0 && (best == 0 || d.NominalHz < best) {
			// With several devices of a kind, report the slowest: it is the
			// one that limits (e.g. a second keyboard is rare).
			best = d.NominalHz
		}
	}
	return best
}

// effectiveHz is the refresh rate the game runs at: the one chosen inside
// CS2 in exclusive fullscreen, else the Windows desktop rate.
func effectiveHz(f *facts.Facts, cs2 games.CS2Info) (float64, string) {
	desk := 0.0
	for _, d := range f.Displays {
		if d.Primary || desk == 0 {
			desk = float64(d.CurrentHz)
		}
	}
	if v := cs2.Video; v != nil && v.Fullscreen && v.RefreshHz > 0 {
		return v.RefreshHz, "taxa escolhida no CS2"
	}
	if desk > 0 {
		return desk, "taxa do Windows"
	}
	return 0, ""
}

func maxHz(f *facts.Facts) float64 {
	m := 0.0
	for _, d := range f.Displays {
		if d.Primary || m == 0 {
			m = float64(d.MaxHzAtRes)
		}
	}
	return m
}

func latencyLevers(f *facts.Facts, cs2 games.CS2Info, mouseHz float64, run *perf.Run, hz float64, v LatencyView) (levers, missing []LatencyLever) {
	// Keyboard at the USB low-speed limit: the largest input gain on many setups.
	if khz := nominalHz(f, "keyboard"); v.Keyboard != nil && khz > 0 && khz < 900 {
		gain := v.Keyboard.Ms - 0.5
		levers = append(levers, LatencyLever{Title: "Teclado de 1000 Hz", GainMs: r2(gain), Page: "perifericos",
			Text: fmt.Sprintf("Seu teclado é lido no máximo a %s. Um teclado de 1000 Hz tira cerca de %s ms em média de cada tecla — movimento, pulo e contra-strafe. Nenhum ajuste de software muda o limite da porta.", hzText(khz), fmtF(gain, 1))})
	}
	// Mouse polling.
	switch {
	case mouseHz > 0 && mouseHz < 900:
		cur, _ := pollMs(mouseHz)
		levers = append(levers, LatencyLever{Title: "Polling do mouse em 1000 Hz", GainMs: r2(cur - 0.5), Page: "perifericos",
			Text: fmt.Sprintf("O teste mediu %s. Ajuste para 1000 Hz no software do fabricante (se o mouse aceitar) e meça de novo.", hzText(mouseHz))})
	case mouseHz >= 900 && mouseHz < 3000:
		cur, _ := pollMs(mouseHz)
		gain := cur - 1000.0/(2*8000)
		levers = append(levers, LatencyLever{Title: "Polling de 4000 ou 8000 Hz", GainMs: r2(gain), Page: "perifericos",
			Text: fmt.Sprintf("Com %s a espera média é de %s ms. Em 4000 Hz ela cai para 0,125 ms e em 8000 Hz para 0,06 ms (%s ms a menos) — só se o mouse e o receptor suportarem, com mais uso de CPU e o receptor numa porta direta da placa-mãe.", hzText(mouseHz), fmtF(cur, 2), fmtF(gain, 2))})
	case mouseHz >= 3000:
		levers = append(levers, LatencyLever{Title: "Polling do mouse", Done: true, Page: "perifericos", Text: fmt.Sprintf("Medido em %s: a espera média já é menor que 0,2 ms.", hzText(mouseHz))})
	case mouseHz == 0:
		missing = append(missing, LatencyLever{Title: "Medir o polling do mouse", Page: "perifericos", Text: "Leva 6 segundos: mova o mouse em círculos na janela de teste."})
	}
	// Refresh rate.
	if mx := maxHz(f); hz > 0 && mx > hz+4 {
		gain := 1000/(2*hz) - 1000/(2*mx)
		levers = append(levers, LatencyLever{Title: fmt.Sprintf("Monitor em %s em vez de %s", hzText(mx), hzText(hz)), GainMs: r2(gain), Page: "cs2",
			Text: "A varredura até a mira encurta e cada quadro fica menos tempo na tela. Confira a taxa no Windows (Configurações > Tela > Avançado) e dentro do CS2."})
	}
	// NVIDIA Reflex and V-Sync, read from the CS2 settings.
	if vid := cs2.Video; vid != nil {
		switch vid.LowLatency {
		case 0:
			levers = append(levers, LatencyLever{Title: "Ligar o NVIDIA Reflex no CS2 (Ativado + Boost)", GainMs: -1, Page: "cs2",
				Text: "O Reflex tira a fila de quadros entre o processador e a placa de vídeo. O ganho é maior quando a placa de vídeo limita o FPS; meça antes e depois na página Desempenho."})
		case 1:
			levers = append(levers, LatencyLever{Title: "Reflex com Boost", GainMs: -1, Page: "cs2",
				Text: "O Boost mantém o clock da placa de vídeo alto mesmo quando ela não está no limite. Ganho pequeno; custa um pouco mais de energia."})
		case 2:
			levers = append(levers, LatencyLever{Title: "NVIDIA Reflex", Done: true, Page: "cs2", Text: "Já está em Ativado + Boost no CS2."})
		}
		if vid.VSync > 0 {
			levers = append(levers, LatencyLever{Title: "Desligar o V-Sync do jogo", GainMs: -1, Page: "cs2",
				Text: "Com V-Sync cada quadro espera o ciclo do monitor. Desligue no CS2; com G-SYNC/FreeSync, use V-Sync no painel do driver junto com o Reflex (veja a página CS2)."})
		}
	}
	if run != nil {
		if strings.HasPrefix(run.Summary.PresentMode, "Composed") {
			levers = append(levers, LatencyLever{Title: "Tela cheia de verdade", GainMs: -1, Page: "desempenho",
				Text: "Na medição o Windows compôs a imagem (modo " + run.Summary.PresentMode + "), o que soma pelo menos um ciclo do monitor. Use tela cheia no jogo e feche sobreposições."})
		}
	} else {
		missing = append(missing, LatencyLever{Title: "Medir o FPS e a latência do PC no CS2", Page: "desempenho", Text: "Grave 60 s de partida ou de mapa de treino com o PresentMon."})
	}
	if v.Network == nil {
		missing = append(missing, LatencyLever{Title: "Testar a rede", Page: "rede", Text: "Mede o ping, a oscilação e a perda até os relays da Valve."})
	}
	// Biggest known gains first; unquantified ones after; done ones last.
	sortLevers(levers)
	return levers, missing
}

func sortLevers(l []LatencyLever) {
	rank := func(x LatencyLever) float64 {
		switch {
		case x.Done:
			return -2
		case x.GainMs < 0:
			return -1
		}
		return x.GainMs
	}
	for i := 1; i < len(l); i++ {
		for j := i; j > 0 && rank(l[j]) > rank(l[j-1]); j-- {
			l[j], l[j-1] = l[j-1], l[j]
		}
	}
}

func hzText(hz float64) string {
	if math.Abs(hz-math.Round(hz)) < 0.05 || hz >= 100 {
		return fmt.Sprintf("%.0f Hz", math.Round(hz))
	}
	return strings.Replace(fmt.Sprintf("%.1f Hz", hz), ".", ",", 1)
}

func fmtF(v float64, d int) string {
	return strings.Replace(fmt.Sprintf("%.*f", d, v), ".", ",", 1)
}

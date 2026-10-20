package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/periph"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

var (
	srcUSBEndpoint = tweaks.Source{Title: "Microsoft Learn — USB_ENDPOINT_DESCRIPTOR: período de polling por velocidade e bInterval", URL: "https://learn.microsoft.com/windows-hardware/drivers/ddi/usbspec/ns-usbspec-_usb_endpoint_descriptor"}
	srcUSBETW      = tweaks.Source{Title: "Microsoft Learn — USB Event Tracing for Windows (provedores UCX e USBHUB3)", URL: "https://learn.microsoft.com/windows-hardware/drivers/usbcon/usb-event-tracing-for-windows"}
	srcDeepPoll    = tweaks.Source{Title: "DeepPoll (MIT) — medição de polling pelos eventos ETW do USB", URL: "https://github.com/MariusHeier/deeppoll"}
	srcRazerHP     = tweaks.Source{Title: "Razer — HyperPolling: porta nativa, custo de CPU, jogos antigos e dongle longe de USB 3", URL: "https://www.razer.com/technology/razer-hyperpolling"}
	srcUSB3RFI     = tweaks.Source{Title: "USB-IF / Intel — Interferência de rádio do USB 3.0 em dispositivos de 2,4 GHz", URL: "https://www.usb.org/document-library/usb-30-radio-frequency-interference-impact-24-ghz-wireless-devices"}
	srcHighRateKB  = tweaks.Source{Title: "Neowin — KB5027303: \"melhora o desempenho ao usar um mouse de alta taxa de relatório\"", URL: "https://www.neowin.net/news/its-more-good-news-for-patch-tuesday-as-microsoft-finally-fixes-windows-11-mouse-stutters/"}
	srcBgRawInput  = tweaks.Source{Title: "PC-Optimization-Hub — FAQ de mouse (limite de raw input em segundo plano no Windows 11, citação da Microsoft)", URL: "https://github.com/BoringBoredom/PC-Optimization-Hub/blob/main/content/peripherals/mouse%20faq.md"}
)

// PeripheralView is one physical keyboard/mouse on the Periféricos page.
type PeripheralView struct {
	facts.InputDev
	Role       string               `json:"role"`
	Connection string               `json:"connection"`
	Port       string               `json:"portLabel"`
	Rate       string               `json:"rateLabel"`
	Power      string               `json:"powerLabel"`
	Findings   []diag.Finding       `json:"findings,omitempty"`
	Measured   *periph.DeviceResult `json:"measured,omitempty"`
}

// PeriphSettings are the input settings shown on the page.
type PeriphSettings struct {
	MouseAccelOn   bool `json:"mouseAccelOn"`
	StickyHotkey   bool `json:"stickyHotkey"`
	FilterKeysOn   bool `json:"filterKeysOn"`
	FilterDelayMS  int  `json:"filterDelayMs"`
	MouseQueue     int  `json:"mouseQueue"`
	KeyboardQueue  int  `json:"keyboardQueue"`
	ThreadDPC      int  `json:"threadDpc"`
	USBSuspendPlan int  `json:"usbSuspendPlan"` // power plan USB selective suspend on AC (-1 unknown)
}

// PeripheralsView is the Periféricos page model.
type PeripheralsView struct {
	Scanned        bool                 `json:"scanned"`
	Devices        []PeripheralView     `json:"devices"`
	Settings       PeriphSettings       `json:"settings"`
	Findings       []diag.Finding       `json:"findings"` // settings-level findings
	Measurement    *periph.Measurement  `json:"measurement,omitempty"`
	History        []periph.Measurement `json:"history,omitempty"`
	TraceAvailable bool                 `json:"traceAvailable"`
	Tweaks         []string             `json:"tweaks"`
	Tips           []CheckItem          `json:"tips"`
	Honest         string               `json:"honest"`
	Sources        []tweaks.Source      `json:"sources"`
}

// MeasureRequest starts a polling test.
type MeasureRequest struct {
	Seconds  int  `json:"seconds"`
	USBTrace bool `json:"usbTrace"`
}

const maxMeasurements = 8

func (a *App) measurementsPath() string { return filepath.Join(a.P.DataDir, "perifericos.json") }

// Measurements returns the stored polling tests, newest first.
func (a *App) Measurements() []periph.Measurement {
	a.pmu.Lock()
	defer a.pmu.Unlock()
	if a.measures == nil {
		if b, err := os.ReadFile(a.measurementsPath()); err == nil {
			_ = json.Unmarshal(b, &a.measures)
		}
		if a.measures == nil {
			a.measures = []periph.Measurement{}
		}
	}
	return append([]periph.Measurement(nil), a.measures...)
}

func (a *App) storeMeasurement(m periph.Measurement) {
	_ = a.Measurements() // load
	a.pmu.Lock()
	a.measures = append([]periph.Measurement{m}, a.measures...)
	if len(a.measures) > maxMeasurements {
		a.measures = a.measures[:maxMeasurements]
	}
	a.pmu.Unlock()
	if err := saveFile(a.measurementsPath(), func() ([]byte, error) {
		a.pmu.Lock()
		defer a.pmu.Unlock()
		return json.MarshalIndent(a.measures, "", " ")
	}); err != nil {
		log.Printf("medição de periféricos não foi salva: %v", err)
	}
}

// inputDevices returns the grouped devices of the last scan, or a quick
// inventory when no scan ran yet.
func (a *App) inputDevices() []facts.InputDev {
	if f := a.Facts(); f != nil && len(f.Peripherals.Devices) > 0 {
		return f.Peripherals.Devices
	}
	raw, err := a.P.Native.InputDevices()
	if err != nil {
		return nil
	}
	return periph.Group(raw)
}

// StartMeasure runs the polling test in a job.
func (a *App) StartMeasure(req MeasureRequest) (Job, error) {
	return a.Jobs.Start("measure", "Medindo o polling do mouse", true, a.measureJob(req))
}

// measureJob returns the body of the polling test, so it runs alone
// (StartMeasure) or as a stage of the full evaluation.
func (a *App) measureJob(req MeasureRequest) func(ctx context.Context, c *JobCtl) (any, error) {
	secs := req.Seconds
	if secs <= 0 {
		secs = 6
	}
	if secs < 3 {
		secs = 3
	}
	if secs > 15 {
		secs = 15
	}
	trace := req.USBTrace && a.P.Native.IsElevated()
	return func(ctx context.Context, c *JobCtl) (any, error) {
		devs := a.inputDevices()
		d := time.Duration(secs) * time.Second
		c.Progress(0.02, "Abrindo a janela de teste: mova o mouse sem parar, em círculos rápidos")
		if trace {
			c.Logf("rastreando também os eventos do controlador USB (ETW)")
		}
		done := make(chan struct{})
		go func() {
			start := time.Now()
			tk := time.NewTicker(250 * time.Millisecond)
			defer tk.Stop()
			for {
				select {
				case <-done:
					return
				case <-tk.C:
					p := time.Since(start).Seconds() / d.Seconds()
					if p > 0.97 {
						p = 0.97
					}
					c.Progress(p, fmt.Sprintf("Capturando… %.0f s restantes", (d-time.Since(start)).Seconds()))
				}
			}
		}()
		capt, err := a.P.Native.CaptureInput(ctx, sys.CaptureOpts{
			Duration: d,
			Title:    "Mova o mouse sem parar",
			Message:  "Faça círculos rápidos e contínuos até a contagem terminar. Mantenha esta janela em foco: o Windows 11 limita o mouse enviado a janelas em segundo plano. Esc cancela.",
			USBTrace: trace,
		})
		close(done)
		if err != nil {
			return nil, err
		}
		m := periph.Summarize(devs, capt)
		if !m.Cancelled || len(m.Devices) > 0 {
			a.storeMeasurement(m)
		}
		for _, r := range m.Devices {
			c.Logf("%s: %s", r.Name, r.Summary)
		}
		for _, n := range m.Notes {
			c.Logf("%s", n)
		}
		return m, nil
	}
}

func roleLabel(d facts.InputDev) string {
	switch {
	case d.Receiver && d.Has("mouse") && d.Has("keyboard"):
		return "Receptor sem fio (mouse e teclado)"
	case d.Receiver:
		return "Receptor sem fio"
	case d.Primary == "mouse":
		return "Mouse"
	}
	return "Teclado"
}

func connectionLabel(d facts.InputDev) string {
	if d.Transport != periph.TransportUSB {
		return periph.TransportLabel(d.Transport)
	}
	if d.Speed == sys.USBSpeedUnknown {
		return "USB"
	}
	return periph.SpeedLabel(d.Speed)
}

func portLabel(d facts.InputDev) string {
	if d.Transport != periph.TransportUSB || d.USBNode == "" {
		return ""
	}
	where := "direto no controlador"
	if d.HubDepth == 1 {
		where = "através de 1 hub externo"
	} else if d.HubDepth > 1 {
		where = fmt.Sprintf("através de %d hubs externos", d.HubDepth)
	}
	s := fmt.Sprintf("Porta %d, %s", d.Port, where)
	if d.Controller != "" {
		s += " — " + d.Controller
	}
	return s
}

func rateLabel(d facts.InputDev) string {
	switch {
	case d.Transport == periph.TransportBT || d.Transport == periph.TransportBLE:
		return "definido pelo intervalo de conexão Bluetooth"
	case d.NominalHz > 0:
		return fmt.Sprintf("até %s (a cada %s; bInterval %d)", periph.FormatHz(d.NominalHz), periph.FormatMS(1000/d.NominalHz), d.BInterval)
	case d.Err != "":
		return "não informado (" + d.Err + ")"
	}
	return "não informado"
}

func powerLabel(d facts.InputDev) string {
	if d.Transport != periph.TransportUSB {
		return ""
	}
	if d.SuspendAllowed() {
		return "o Windows pode suspender o dispositivo"
	}
	return "sem suspensão por economia de energia"
}

// Peripherals builds the page model.
func (a *App) Peripherals() PeripheralsView {
	v := PeripheralsView{
		TraceAvailable: a.P.Native.IsElevated(),
		Tweaks:         []string{"input.mouse.accel", "input.a11y.hotkeys", "input.usb.power", "power.usb.suspend", "fix.a11y.filterkeys", "fix.input.queue", "fix.kernel.threaddpc"},
		Honest: "O polling (quantas vezes por segundo o PC lê o mouse) é definido pelo próprio dispositivo e pelo software do fabricante; o Windows só agenda o que o dispositivo pede. " +
			"O que o Axion faz aqui é medir, mostrar o caminho USB de cada periférico e corrigir o que o Windows ou outros programas pioraram. Não existe ajuste de Registro que transforme um teclado de 125 Hz em 1000 Hz com segurança.",
		Sources: []tweaks.Source{srcUSBEndpoint, srcUSBETW, srcDeepPoll, srcRazerHP, srcUSB3RFI, srcHighRateKB, srcBgRawInput},
	}
	hist := a.Measurements()
	if len(hist) > 0 {
		v.Measurement = &hist[0]
		v.History = hist
	}
	f := a.Facts()
	var devs []facts.InputDev
	if f != nil {
		v.Scanned = true
		p := f.Peripherals
		devs = p.Devices
		v.Settings = PeriphSettings{MouseAccelOn: f.Gaming.MouseAccelOn, StickyHotkey: f.Gaming.StickyHotkeyOn, FilterKeysOn: p.FilterKeysOn,
			FilterDelayMS: p.FilterDelayMS, MouseQueue: p.MouseQueue, KeyboardQueue: p.KeyboardQueue, ThreadDPC: p.ThreadDPC, USBSuspendPlan: f.Power.USBSuspendAC}
	} else {
		devs = a.inputDevices()
		v.Settings = PeriphSettings{MouseQueue: -1, KeyboardQueue: -1, ThreadDPC: -1, USBSuspendPlan: -1}
	}
	var findings []diag.Finding
	if r := a.Report(); r != nil {
		findings = r.Findings
	}
	used := map[string]bool{}
	for _, d := range devs {
		pv := PeripheralView{InputDev: d, Role: roleLabel(d), Connection: connectionLabel(d), Port: portLabel(d), Rate: rateLabel(d), Power: powerLabel(d)}
		suffix := "." + devKey(d)
		for _, x := range findings {
			if strings.HasPrefix(x.ID, "per.") && strings.HasSuffix(x.ID, suffix) {
				pv.Findings = append(pv.Findings, x)
				used[x.ID] = true
			}
		}
		if v.Measurement != nil {
			for i := range v.Measurement.Devices {
				if strings.EqualFold(v.Measurement.Devices[i].Key, d.Key) {
					r := v.Measurement.Devices[i]
					pv.Measured = &r
				}
			}
		}
		v.Devices = append(v.Devices, pv)
	}
	for _, x := range findings {
		switch {
		case used[x.ID]:
		case x.ID == "input.filterkeys" || x.ID == "res.inputqueue" || x.ID == "res.threaddpc" || strings.HasPrefix(x.ID, "per."):
			v.Findings = append(v.Findings, x)
		}
	}
	v.Tips = peripheralTips()
	return v
}

// devKey mirrors the diagnostic's device suffix.
func devKey(d facts.InputDev) string {
	if d.VID != 0 {
		return fmt.Sprintf("%04X-%04X", d.VID, d.PID)
	}
	return ""
}

func peripheralTips() []CheckItem {
	return []CheckItem{
		{ID: "tip.port", Title: "Mouse e teclado direto na placa-mãe", Status: "info", Evidence: "média",
			Advice: "Use as portas traseiras do painel da placa-mãe, sem hub de monitor, de teclado ou de gabinete no caminho. Os cartões acima mostram se cada dispositivo está atrás de um hub.",
			Basis:  "orientação de fabricante para taxas altas; cada hub soma um salto e divide a banda e o agendamento da porta.", Sources: []tweaks.Source{srcRazerHP}},
		{ID: "tip.dongle", Title: "Receptor sem fio perto do mouse e longe de USB 3.0", Status: "info", Evidence: "alta",
			Advice: "Use o cabo extensor ou a base que acompanha o mouse para deixar o receptor a poucos centímetros dele, afastado de portas e dispositivos USB 3.0 (SSD externo, hubs).",
			Basis:  "o sinal do USB 3.0 gera ruído na faixa de 2,4 GHz (estudo da Intel publicado pelo USB-IF); a Razer faz a mesma orientação.", Sources: []tweaks.Source{srcUSB3RFI, srcRazerHP}},
		{ID: "tip.rate", Title: "1000 Hz é o ponto de equilíbrio; acima disso, teste", Status: "info", Evidence: "média",
			Advice: "A 1000 Hz o mouse soma em média 0,5 ms de espera. 2000 a 8000 Hz reduzem isso para 0,06–0,25 ms, mas custam CPU e alguns jogos antigos engasgam; se notar engasgos, volte para 1000 ou 2000 Hz. Mantenha o Windows 11 atualizado: a Microsoft corrigiu em 2023 engasgos com mouses de alta taxa.",
			Basis:  "aritmética do intervalo de polling, orientação do fabricante e nota da atualização KB5027303.", Sources: []tweaks.Source{srcRazerHP, srcHighRateKB}},
		{ID: "tip.focus", Title: "Medições e jogos precisam estar em foco", Status: "info", Evidence: "média",
			Advice: "No Windows 11, programas em segundo plano recebem o mouse limitado a cerca de 125 mensagens por segundo (os movimentos chegam agrupados). Por isso testadores de mouse mostram \"125 Hz\" quando a janela perde o foco.",
			Basis:  "explicação de um engenheiro da Microsoft reproduzida no PC-Optimization-Hub.", Sources: []tweaks.Source{srcBgRawInput}},
		{ID: "tip.method", Title: "Como o Axion mede", Status: "info", Evidence: "alta",
			Advice: "A janela de teste registra a hora exata de cada relatório que chega ao aplicativo (raw input). Com o Axion aberto como administrador, ele também registra, pelos eventos ETW do próprio driver USB do Windows, a hora em que cada transferência terminou no controlador: o ritmo real do polling, sem a variação do resto do sistema.",
			Basis:  "eventos Microsoft-Windows-USB-UCX e USBHUB3 documentados pela Microsoft; é o mesmo método do DeepPoll.", Sources: []tweaks.Source{srcUSBETW, srcDeepPoll}},
	}
}

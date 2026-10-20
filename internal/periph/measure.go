package periph

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"axion/internal/facts"
	"axion/internal/sys"
)

// Verdicts of a measured device.
const (
	VerdictOK        = "ok"
	VerdictLow       = "low"
	VerdictIrregular = "irregular"
	VerdictLittle    = "little"
)

// MinSamples is the number of intervals a verdict needs.
const MinSamples = 200

// DeviceResult is the outcome of a capture for one physical device.
type DeviceResult struct {
	Key        string   `json:"key"`
	Name       string   `json:"name"`
	Primary    string   `json:"primary"`
	NominalHz  float64  `json:"nominalHz"`
	Raw        *Stats   `json:"raw,omitempty"` // arrival at the application (raw input)
	USB        *Stats   `json:"usb,omitempty"` // completion in the USB controller driver (ETW)
	Lag        *Lag     `json:"lag,omitempty"` // USB completion → application
	MeasuredHz float64  `json:"measuredHz"`
	Snapped    int      `json:"snapped"`
	Source     string   `json:"source"` // usb | raw
	Verdict    string   `json:"verdict"`
	Summary    string   `json:"summary"`
	Notes      []string `json:"notes,omitempty"`
	Matched    string   `json:"matched,omitempty"` // how the USB pipe was tied to the device
}

// Measurement is a complete polling test.
type Measurement struct {
	At        time.Time      `json:"at"`
	Seconds   float64        `json:"seconds"`
	USBTraced bool           `json:"usbTraced"`
	FocusLost bool           `json:"focusLost"`
	Cancelled bool           `json:"cancelled"`
	Devices   []DeviceResult `json:"devices"`
	Notes     []string       `json:"notes,omitempty"`
	Trace     TraceInfo      `json:"trace"` // counters kept for troubleshooting
}

// TraceInfo summarises what the capture received.
type TraceInfo struct {
	RawDevices int    `json:"rawDevices"`
	RawReports int    `json:"rawReports"`
	USBPipes   int    `json:"usbPipes"`
	USBEvents  int    `json:"usbEvents"`
	USBDevices int    `json:"usbDevices"`
	Lost       uint32 `json:"lost"`
	LostBufs   uint32 `json:"lostBuffers"`
	Error      string `json:"error,omitempty"`
}

type pipeKey struct{ dev, pipe uint64 }

// Summarize matches a capture to the inventory and computes the verdicts.
func Summarize(devs []facts.InputDev, c sys.InputCapture) Measurement {
	m := Measurement{At: time.Now(), Seconds: c.Duration.Seconds(), USBTraced: c.USBTraced, FocusLost: c.FocusLost, Cancelled: c.Cancelled}
	m.Trace = TraceInfo{RawDevices: len(c.RawInput), USBPipes: len(c.USB), USBDevices: len(c.USBDevices), Lost: c.LostEvents, LostBufs: c.LostBuffers, Error: c.TraceError}
	for _, r := range c.RawInput {
		m.Trace.RawReports += len(r.Stamps)
	}
	for _, p := range c.USB {
		m.Trace.USBEvents += len(p.Stamps)
	}
	byKey := map[string]*DeviceResult{}
	var order []string
	rawStamps := map[string][]int64{}
	result := func(key string, d *facts.InputDev, fallbackName string) *DeviceResult {
		uk := strings.ToUpper(key)
		if r, ok := byKey[uk]; ok {
			return r
		}
		r := &DeviceResult{Key: key, Name: fallbackName, Primary: "mouse"}
		if d != nil {
			r.Name, r.Primary, r.NominalHz = d.Name, d.Primary, d.NominalHz
		}
		byKey[uk] = r
		order = append(order, uk)
		return r
	}

	// 1. Raw input: one series per HID collection.
	for _, s := range c.RawInput {
		if s.Kind != "mouse" || len(s.Stamps) == 0 {
			continue
		}
		inst := RawNameToInstance(s.Device)
		d := Find(devs, inst)
		key, name := inst, "Mouse"
		if vid, pid, ok := ParseVIDPID(inst); ok {
			name = fmt.Sprintf("Mouse %04X:%04X", vid, pid)
		}
		if d != nil {
			key = d.Key
		}
		r := result(key, d, name)
		if len(s.Stamps) > len(rawStamps[strings.ToUpper(r.Key)]) {
			rawStamps[strings.ToUpper(r.Key)] = s.Stamps
		}
	}

	// 2. USB trace: pipes grouped by the UCX device handle.
	pipes := map[pipeKey][]int64{}
	for _, p := range c.USB {
		pipes[pipeKey{p.Device, p.Pipe}] = p.Stamps
	}
	usedPipe := map[pipeKey]bool{}
	usbStamps := map[string][]int64{}
	matchedBy := map[string]string{}
	if c.USBTraced {
		for i := range devs {
			d := &devs[i]
			if d.Transport != TransportUSB || d.VID == 0 {
				continue
			}
			var best pipeKey
			n := 0
			for _, td := range c.USBDevices {
				vid, pid, ok := ParseVIDPID(td.Path)
				if !ok || vid != d.VID || pid != d.PID {
					continue
				}
				if d.Port > 0 && td.Port > 0 && td.Port != d.Port && countVIDPID(devs, d.VID, d.PID) > 1 {
					continue
				}
				for k, st := range pipes {
					if k.dev == td.Handle && len(st) > n && !usedPipe[k] {
						best, n = k, len(st)
					}
				}
			}
			uk := strings.ToUpper(d.Key)
			_, hasRaw := byKey[uk]
			// A device with no raw motion only counts when it streamed
			// (a keyboard sends a handful of reports per key press).
			if n >= 10 && (hasRaw || n >= MinSamples) {
				usedPipe[best] = true
				usbStamps[uk] = pipes[best]
				matchedBy[uk] = "identificado pelo driver do hub (VID/PID)"
				result(d.Key, d, d.Name)
			}
		}
		// Fallback: a moving mouse without a rundown match takes the busiest
		// free pipe whose rhythm matches its raw input.
		for _, uk := range order {
			if _, ok := usbStamps[uk]; ok {
				continue
			}
			raw := Analyze(rawStamps[uk], c.Freq, 0)
			if raw == nil {
				continue
			}
			var best pipeKey
			n := 0
			for k, st := range pipes {
				if usedPipe[k] || len(st) <= n {
					continue
				}
				ps := Analyze(st, c.Freq, 0)
				if ps == nil || ps.MedianUS <= 0 {
					continue
				}
				ratio := raw.MedianUS / ps.MedianUS
				if ratio > 0.7 && ratio < 1.3 {
					best, n = k, len(st)
				}
			}
			if n >= MinSamples {
				usedPipe[best] = true
				usbStamps[uk] = pipes[best]
				matchedBy[uk] = "associado pelo ritmo (o driver do hub não informou o dispositivo)"
			}
		}
	}

	for _, uk := range order {
		r := byKey[uk]
		r.Raw = Analyze(rawStamps[uk], c.Freq, r.NominalHz)
		r.USB = Analyze(usbStamps[uk], c.Freq, r.NominalHz)
		r.Matched = matchedBy[uk]
		if r.USB != nil && r.Raw != nil {
			r.Lag = StackLag(usbStamps[uk], rawStamps[uk], c.Freq)
		}
		judge(r)
		m.Devices = append(m.Devices, *r)
	}
	sort.SliceStable(m.Devices, func(a, b int) bool {
		return sampleCount(m.Devices[a]) > sampleCount(m.Devices[b])
	})

	if c.Cancelled {
		m.Notes = append(m.Notes, "Teste interrompido antes do fim.")
	}
	if c.FocusLost {
		m.Notes = append(m.Notes, "A janela de teste perdeu o foco durante a captura. No Windows 11, aplicativos em segundo plano recebem o mouse limitado a cerca de 125 mensagens por segundo, então esses trechos não representam o polling real.")
	}
	if !c.USBTraced {
		note := "Medição no controlador USB (ETW) indisponível; o resultado vem da entrega do Windows ao aplicativo (raw input), que inclui o atraso do sistema."
		if c.TraceError != "" {
			note += " Motivo: " + c.TraceError + "."
		}
		m.Notes = append(m.Notes, note)
	}
	if c.USBTraced && m.Trace.USBEvents == 0 {
		m.Notes = append(m.Notes, "O rastreamento USB foi iniciado, mas não recebeu nenhuma transferência. Isso acontece em controladores com driver próprio do fabricante (fora da pilha USB do Windows); o resultado vem só do raw input.")
	} else if c.USBTraced {
		matched := false
		for _, r := range m.Devices {
			if r.USB != nil {
				matched = true
			}
		}
		if !matched && len(m.Devices) > 0 {
			m.Notes = append(m.Notes, "O rastreamento USB recebeu transferências, mas nenhuma pôde ser associada ao mouse com segurança; o resultado vem do raw input.")
		}
	}
	switch {
	case c.LostBuffers > 0:
		m.Notes = append(m.Notes, fmt.Sprintf("O rastreamento USB ficou incompleto (%d buffers de eventos descartados); os trechos afetados aparecem como pausas no resultado do controlador.", c.LostBuffers))
	case c.LostEvents > 0:
		m.Notes = append(m.Notes, fmt.Sprintf("O rastreamento USB descartou %d eventos por falta de buffer; os intervalos afetados aparecem como pausas.", c.LostEvents))
	}
	if len(m.Devices) == 0 && !c.Cancelled {
		m.Notes = append(m.Notes, "Nenhum movimento de mouse foi registrado. Mova o mouse sem parar enquanto a janela de teste estiver aberta.")
	}
	return m
}

func countVIDPID(devs []facts.InputDev, vid, pid uint16) int {
	n := 0
	for _, d := range devs {
		if d.VID == vid && d.PID == pid {
			n++
		}
	}
	return n
}

func sampleCount(r DeviceResult) int {
	n := 0
	if r.Raw != nil {
		n += r.Raw.Samples
	}
	if r.USB != nil {
		n += r.USB.Samples
	}
	return n
}

func judge(r *DeviceResult) {
	best, src := r.USB, "usb"
	if best == nil || best.Samples < MinSamples {
		if r.Raw != nil && (best == nil || r.Raw.Samples > best.Samples) {
			best, src = r.Raw, "raw"
		}
	}
	r.Source = src
	if best == nil || best.Samples < MinSamples {
		r.Verdict = VerdictLittle
		r.Summary = "Poucos dados para concluir: mova o mouse sem parar (em círculos, rápido) durante todo o teste."
		return
	}
	r.MeasuredHz, r.Snapped = best.RateHz, best.Snapped
	where := "no controlador USB"
	if src == "raw" {
		where = "na entrega do Windows ao aplicativo"
	}
	rate := FormatHz(float64(best.Snapped))
	if best.Snapped == 0 {
		rate = FormatHz(best.RateHz)
	}
	onTimeMin := 85.0
	if src == "raw" {
		onTimeMin = 60
	}
	// The typical rate (from the median interval, as shown in the text)
	// decides "low"; skipped polls lower only the mean and belong to
	// "irregular".
	typical := float64(best.Snapped)
	if typical == 0 {
		typical = best.RateHz
	}
	switch {
	case r.NominalHz > 0 && typical < 0.8*r.NominalHz:
		r.Verdict = VerdictLow
		r.Summary = fmt.Sprintf("Mediu cerca de %s %s, mas a porta USB deste dispositivo aceita até %s. Se você moveu o mouse rápido o tempo todo, o polling está configurado abaixo do máximo no software do fabricante (ou o modo de economia do mouse está ativo).",
			rate, where, FormatHz(r.NominalHz))
	case best.OnTimePct < onTimeMin:
		r.Verdict = VerdictIrregular
		r.Summary = fmt.Sprintf("Cerca de %s %s, mas só %.0f%% dos intervalos ficaram no ritmo esperado. Causas comuns: movimento lento ou interrompido, receptor sem fio longe do mouse ou colado em portas USB 3.0, hub no caminho ou sistema ocupado.",
			rate, where, best.OnTimePct)
	default:
		r.Verdict = VerdictOK
		r.Summary = fmt.Sprintf("Estável em cerca de %s %s (%.0f%% dos intervalos no ritmo esperado).", rate, where, best.OnTimePct)
	}
	if r.USB != nil && r.Raw != nil && r.USB.Snapped > 0 && r.Raw.Snapped > 0 && r.Raw.Snapped < r.USB.Snapped/2 {
		r.Notes = append(r.Notes, fmt.Sprintf("O controlador USB recebeu %s, mas a janela de teste recebeu em lotes (%s). Isso acontece quando a janela perde o foco ou o sistema está ocupado; jogos com raw input no foco recebem todos os relatórios.",
			FormatHz(float64(r.USB.Snapped)), FormatHz(float64(r.Raw.Snapped))))
	}
	if r.Raw != nil && r.Raw.Samples > 0 {
		share := 100 * float64(r.Raw.Coalesced) / float64(r.Raw.Samples)
		if share >= 10 {
			r.Notes = append(r.Notes, fmt.Sprintf("%.0f%% dos relatórios chegaram ao aplicativo agrupados com o anterior. Em taxas altas isso é normal: o jogo lê todos os movimentos acumulados.", share))
		}
	}
	if best.Pauses > 0 {
		r.Notes = append(r.Notes, fmt.Sprintf("%d %s somando %s: trechos sem relatório (mouse parado ou o sistema atrasou o processamento USB).", best.Pauses, map[bool]string{true: "pausa", false: "pausas"}[best.Pauses == 1], FormatMS(best.PauseMS)))
	}
	if r.Lag != nil {
		r.Notes = append(r.Notes, fmt.Sprintf("Atraso entre o controlador USB e a janela de teste: mediana de %.0f µs (95%% abaixo de %.0f µs).", r.Lag.MedianUS, r.Lag.P95US))
	}
	if r.Matched != "" && strings.HasPrefix(r.Matched, "associado") {
		r.Notes = append(r.Notes, "Pipe USB "+r.Matched+".")
	}
}

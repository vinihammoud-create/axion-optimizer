package app

import (
	"strings"
	"testing"
	"time"

	"axion/internal/facts"
	"axion/internal/games"
	"axion/internal/netdiag"
	"axion/internal/perf"
	"axion/internal/periph"
	"axion/internal/sys"
)

// The budget of a setup like the user's: mouse at 1000 Hz, keyboard on a
// USB low-speed port (125 Hz), 240 Hz monitor, CS2 with Reflex + Boost.
func TestLatencyBudget(t *testing.T) {
	f := &facts.Facts{}
	f.Displays = append(f.Displays, sys.DisplayMode{Primary: true, CurrentHz: 240, MaxHzAtRes: 240})
	f.Peripherals.Devices = []facts.InputDev{{Primary: "mouse", NominalHz: 1000}, {Primary: "keyboard", NominalHz: 125}}
	cs2 := games.CS2Info{Video: &games.CS2Video{Fullscreen: true, RefreshHz: 239.964, LowLatency: 2}}
	meas := []periph.Measurement{{At: time.Now(), Devices: []periph.DeviceResult{{Primary: "mouse", Snapped: 1000, MeasuredHz: 951, Lag: &periph.Lag{MedianUS: 31.6, P95US: 47.3, Samples: 5101}}}}}
	runs := []perf.Run{{Process: "cs2.exe", Started: time.Now(), Summary: perf.Summary{AvgFPS: 482, FTAvg: 2.07, PCLatencyAvg: 6.4, PresentMode: "Hardware: Independent Flip"}}}
	net := &netdiag.Summary{ValveMs: 12.3, ValvePOP: "São Paulo", At: time.Now()}
	v := buildLatency(f, cs2, meas, runs, net)
	if !v.Complete || len(v.Mouse) != 4 {
		t.Fatalf("cadeia: %+v", v.Mouse)
	}
	if v.Mouse[0].Ms != 0.5 || v.Mouse[0].Kind != latMeasured || v.Mouse[1].Ms != 6.4 || v.Mouse[2].Kind != latCalculated {
		t.Errorf("partes: %+v", v.Mouse)
	}
	// 0.5 (polling) + 6.4 (PC) + 1000/(2*239.964) (scanout to the center).
	if want := 0.5 + 6.4 + 1000/(2*239.964); v.TotalMs < want-0.02 || v.TotalMs > want+0.02 {
		t.Errorf("total %v, queria %v", v.TotalMs, want)
	}
	if v.Keyboard == nil || v.Keyboard.Ms != 4 || v.Stack == nil || v.Stack.Ms != 0.03 || v.Network == nil {
		t.Errorf("teclado/pilha/rede: %+v %+v %+v", v.Keyboard, v.Stack, v.Network)
	}
	if len(v.Levers) == 0 || v.Levers[0].Title != "Teclado de 1000 Hz" || v.Levers[0].GainMs != 3.5 || !strings.Contains(v.Levers[0].Text, "125 Hz") {
		t.Errorf("o maior ganho é o teclado: %+v", v.Levers)
	}
	last := v.Levers[len(v.Levers)-1]
	if !last.Done || last.Title != "NVIDIA Reflex" {
		t.Errorf("o que já está ótimo vai para o fim: %+v", v.Levers)
	}
	if len(v.Missing) != 0 {
		t.Errorf("nada faltando: %+v", v.Missing)
	}

	// Nothing measured yet: only what can be calculated, and what to measure.
	v = buildLatency(f, games.CS2Info{}, nil, nil, nil)
	if v.Complete || v.Mouse[0].Kind != latCalculated || v.Mouse[1].Ms != -1 || len(v.Missing) != 3 {
		t.Errorf("sem medições: %+v %+v", v.Mouse, v.Missing)
	}
	// A 60 Hz desktop on a 240 Hz monitor is a lever.
	f.Displays[0].CurrentHz = 60
	v = buildLatency(f, games.CS2Info{}, nil, nil, nil)
	found := false
	for _, l := range v.Levers {
		if strings.HasPrefix(l.Title, "Monitor em 240 Hz em vez de 60 Hz") && l.GainMs > 6 {
			found = true
		}
	}
	if !found {
		t.Errorf("monitor abaixo da taxa máxima: %+v", v.Levers)
	}
}

// The gain of a faster polling rate starts from the rate measured, not from
// an assumed 1000 Hz.
func TestLatencyPollingLever(t *testing.T) {
	f := &facts.Facts{}
	f.Displays = append(f.Displays, sys.DisplayMode{Primary: true, CurrentHz: 240, MaxHzAtRes: 240})
	meas := []periph.Measurement{{At: time.Now(), Devices: []periph.DeviceResult{{Primary: "mouse", Snapped: 2000, MeasuredHz: 1990}}}}
	v := buildLatency(f, games.CS2Info{}, meas, nil, nil)
	for _, l := range v.Levers {
		if l.Title == "Polling de 4000 ou 8000 Hz" {
			// 1000/(2*2000) - 1000/(2*8000) = 0.25 - 0.0625
			if l.GainMs != 0.19 || !strings.Contains(l.Text, "2000 Hz") {
				t.Errorf("ganho a partir de 2000 Hz: %+v", l)
			}
			return
		}
	}
	t.Errorf("faltou a alavanca do polling: %+v", v.Levers)
}

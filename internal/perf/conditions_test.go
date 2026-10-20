package perf

import (
	"strings"
	"testing"
	"time"
)

func findRemark(rs []Remark, sub string) *Remark {
	for i := range rs {
		if strings.Contains(rs[i].Title, sub) {
			return &rs[i]
		}
	}
	return nil
}

func TestRemarksAreSilentWithoutData(t *testing.T) {
	if got := Remarks(&Run{}); len(got) != 0 {
		t.Errorf("sem condições nem sensores: %+v", got)
	}
	// An ordinary PC on the mains with the default plan has nothing to say.
	r := &Run{Conditions: &Conditions{BatteryPct: -1, PowerScheme: "Equilibrado", PowerMode: "Equilibrado", DisplayHz: 240}}
	if got := Remarks(r); len(got) != 0 {
		t.Errorf("condições normais: %+v", got)
	}
	// Too few readings say nothing.
	r.Sensors = &Sensors{GPU: &GPUSensors{Samples: 2, ThermalPct: 100}, CPU: &CPUSensors{Samples: 2, BusySamples: 2, BusyFreqPct: 10}}
	if got := Remarks(r); len(got) != 0 {
		t.Errorf("leituras demais poucas: %+v", got)
	}
}

func TestRemarksOnConditions(t *testing.T) {
	r := &Run{Conditions: &Conditions{HasBattery: true, OnBattery: true, BatteryPct: 61, PowerSaver: true, Efficiency: true,
		Busy: []Busy{{"chrome.exe", 90}, {"OneDrive.exe", 40}, {"x.exe", 30}, {"y.exe", 26}}}}
	rs := Remarks(r)
	for _, want := range []string{"na bateria", "Economia de energia", "Melhor eficiência", "chrome.exe usou cerca de 90%", "OneDrive.exe"} {
		if findRemark(rs, want) == nil {
			t.Errorf("faltou %q em %+v", want, rs)
		}
	}
	if findRemark(rs, "y.exe") != nil {
		t.Error("só os três programas mais ocupados entram")
	}
	for _, x := range rs {
		if x.Level != "warn" || x.Text == "" {
			t.Errorf("%+v", x)
		}
	}
}

func TestRemarksOnGPU(t *testing.T) {
	gpu := func(g GPUSensors) []Remark {
		g.Samples = 30
		return Remarks(&Run{Sensors: &Sensors{GPU: &g}})
	}
	// Thermal slowdown in a quarter of the readings is a warning, with the temperature.
	rs := gpu(GPUSensors{ThermalPct: 25, MaxTempC: 87, AvgTempC: 80, AvgClockMHz: 1500, RatedClockMHz: 2100})
	if x := findRemark(rs, "por temperatura em 25%"); x == nil || x.Level != "warn" || !strings.Contains(x.Text, "87 °C") {
		t.Errorf("térmico: %+v", rs)
	}
	// 5% is below the line.
	if findRemark(gpu(GPUSensors{ThermalPct: 9.9, MaxTempC: 80}), "temperatura") != nil {
		t.Error("9,9% de redução térmica não deveria alertar")
	}
	if x := findRemark(gpu(GPUSensors{HWSlowdownPct: 6}), "freio de hardware"); x == nil || x.Level != "warn" {
		t.Error("o freio de hardware precisa alertar")
	}
	// Sitting at the power limit is information, not a defect, and only when nothing worse happened.
	rs = gpu(GPUSensors{PowerCapPct: 80, MaxTempC: 71})
	if x := findRemark(rs, "limite de potência em 80%"); x == nil || x.Level != "info" || !strings.Contains(x.Text, "Não é um defeito") {
		t.Errorf("limite de potência: %+v", rs)
	}
	if findRemark(gpu(GPUSensors{PowerCapPct: 80, ThermalPct: 30}), "limite de potência") != nil {
		t.Error("com redução térmica, o limite de potência não é a notícia")
	}
	// The clock line always shows, with decimals in the Brazilian way.
	if x := findRemark(gpu(GPUSensors{AvgTempC: 71.4, MaxTempC: 75, AvgClockMHz: 1950, RatedClockMHz: 2100}), "Clock médio de 1950 MHz"); x == nil || !strings.Contains(x.Title, "71 °C de média") {
		t.Error("linha de clock e temperatura")
	}
}

func TestRemarksOnCPU(t *testing.T) {
	cpu := func(c CPUSensors) []Remark {
		c.Samples = 30
		return Remarks(&Run{Sensors: &Sensors{CPU: &c}})
	}
	// Busy and under the base frequency: held back.
	if x := findRemark(cpu(CPUSensors{BusySamples: 20, BusyFreqPct: 65, AvgFreqPct: 70, AvgUtilPct: 75}), "abaixo da frequência base (65%)"); x == nil || x.Level != "warn" {
		t.Error("CPU ocupada abaixo da base")
	}
	// Busy with turbo: information only.
	rs := cpu(CPUSensors{BusySamples: 20, BusyFreqPct: 118, AvgFreqPct: 115, AvgUtilPct: 70})
	if x := findRemark(rs, "Processador: 70% de uso"); x == nil || x.Level != "info" || findRemark(rs, "abaixo") != nil {
		t.Errorf("CPU com turbo: %+v", rs)
	}
	// Low frequency at idle is on purpose: never a warning.
	rs = cpu(CPUSensors{BusySamples: 0, BusyFreqPct: -1, AvgFreqPct: 45, AvgUtilPct: 8})
	if findRemark(rs, "abaixo") != nil || findRemark(rs, "Processador") == nil {
		t.Errorf("CPU em repouso: %+v", rs)
	}
	// Fewer than 3 busy readings do not count.
	if findRemark(cpu(CPUSensors{BusySamples: 2, BusyFreqPct: 40, AvgFreqPct: 60, AvgUtilPct: 30}), "abaixo") != nil {
		t.Error("duas leituras ocupadas não bastam")
	}
}

func TestSummarizeGPU(t *testing.T) {
	if SummarizeGPU([]GPUSample{{}, {}}) != nil {
		t.Error("menos de 3 leituras")
	}
	var ss []GPUSample
	for i := 0; i < 10; i++ {
		ss = append(ss, GPUSample{Thermal: i < 3, PowerCap: i >= 5, HWSlowdown: i == 9, TempC: 60 + i*3, ClockMHz: 2000 - i*20, MaxClockMHz: 2100, PowerW: 50, UtilPct: 90})
	}
	ss = append(ss, GPUSample{TempC: 70, ClockMHz: 0, PowerW: -1, UtilPct: -1}) // a reading with unknown clock, power and load
	g := SummarizeGPU(ss)
	if g.Samples != 11 || g.MaxTempC != 87 || g.RatedClockMHz != 2100 || g.MaxClockMHz != 2000 {
		t.Errorf("%+v", g)
	}
	if got := g.ThermalPct; got < 27.2 || got > 27.3 { // 3 of 11
		t.Errorf("térmico: %v", got)
	}
	if g.PowerCapPct < 45 || g.PowerCapPct > 45.5 || g.HWSlowdownPct < 9 || g.HWSlowdownPct > 9.2 {
		t.Errorf("limites: %+v", g)
	}
	if g.AvgPowerW != 50 || g.AvgUtilPct != 90 || g.AvgClockMHz != 1910 {
		t.Errorf("médias: power=%v util=%v clock=%d", g.AvgPowerW, g.AvgUtilPct, g.AvgClockMHz)
	}
	// Nothing known about the power: -1 rather than 0.
	if h := SummarizeGPU([]GPUSample{{PowerW: -1, UtilPct: -1}, {PowerW: -1, UtilPct: -1}, {PowerW: -1, UtilPct: -1}}); h.AvgPowerW != -1 {
		t.Errorf("potência desconhecida: %v", h.AvgPowerW)
	}
}

func TestSummarizeCPU(t *testing.T) {
	if SummarizeCPU([]CPUSample{{}, {}}) != nil {
		t.Error("menos de 3 leituras")
	}
	c := SummarizeCPU([]CPUSample{{FreqPct: 40, UtilPct: 5}, {FreqPct: 100, UtilPct: 70}, {FreqPct: 90, UtilPct: 80}, {FreqPct: 80, UtilPct: 60}})
	if c.BusySamples != 3 || c.BusyFreqPct != 90 || c.MinFreqPct != 40 || c.AvgFreqPct != 77.5 || c.AvgUtilPct != 53.75 {
		t.Errorf("%+v", c)
	}
	idle := SummarizeCPU([]CPUSample{{FreqPct: 40, UtilPct: 5}, {FreqPct: 45, UtilPct: 8}, {FreqPct: 50, UtilPct: 3}})
	if idle.BusySamples != 0 || idle.BusyFreqPct != -1 {
		t.Errorf("sem leitura ocupada: %+v", idle)
	}
}

func TestCompareConditions(t *testing.T) {
	if CompareConditions(nil, &Conditions{}) != nil || CompareConditions(&Conditions{}, nil) != nil {
		t.Error("sem condições de um lado não há o que comparar")
	}
	same := &Conditions{PowerScheme: "Equilibrado", PowerMode: "Equilibrado", DisplayHz: 240}
	if got := CompareConditions(same, same); len(got) != 0 {
		t.Errorf("iguais: %v", got)
	}
	a := &Conditions{PowerScheme: "Equilibrado", DisplayHz: 240, Busy: []Busy{{"chrome.exe", 90}}}
	b := &Conditions{OnBattery: true, PowerScheme: "Alto desempenho", DisplayHz: 60, Busy: []Busy{{"OneDrive.exe", 30}, {"chrome.exe", 80}}}
	joined := strings.Join(CompareConditions(a, b), " | ")
	for _, want := range []string{"na bateria e a outra na tomada", "240 Hz para 60 Hz", "OneDrive.exe"} {
		if !strings.Contains(joined, want) {
			t.Errorf("faltou %q em %s", want, joined)
		}
	}
	if strings.Contains(joined, "chrome.exe") {
		t.Errorf("um programa que estava nas duas não é diferença: %s", joined)
	}
	// Economy plan in only one.
	if got := CompareConditions(&Conditions{}, &Conditions{PowerSaver: true}); len(got) != 1 || !strings.Contains(got[0], "economia") {
		t.Errorf("economia: %v", got)
	}
	// Mode change.
	if got := CompareConditions(&Conditions{PowerMode: "Equilibrado"}, &Conditions{PowerMode: "Melhor desempenho"}); len(got) != 1 || !strings.Contains(got[0], "modo de energia") {
		t.Errorf("modo: %v", got)
	}
}

func TestRemarksSurviveSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	r := &Run{ID: NewID(time.Now()), Process: "cs2.exe", Started: time.Now(), Conditions: &Conditions{HasBattery: true, OnBattery: true, BatteryPct: 40}}
	if err := Save(dir, r); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir, r.ID)
	if err != nil || len(got.Remarks) != 1 || got.Remarks[0].Level != "warn" || got.Conditions == nil || !got.Conditions.OnBattery {
		t.Fatalf("%v %+v", err, got)
	}
	if list := List(dir); len(list) != 1 || len(list[0].Remarks) != 1 {
		t.Errorf("a lista também traz as observações: %+v", list)
	}
}

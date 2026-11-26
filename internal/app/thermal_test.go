package app

import (
	"testing"

	"axion/internal/sys"
)

type thermalAPI struct {
	Thermal struct {
		LimitedNow *bool `json:"limitedNow"`
		Zones      []struct {
			PassiveLimitRead *bool `json:"passiveLimitRead"`
		} `json:"zones"`
	} `json:"thermal"`
}

// "Limitando agora" na tela vem do campo limitedNow da API, o mesmo cálculo
// do diagnóstico; limite de zona não lido não é "0%".
func TestFactsThermalLimitedNow(t *testing.T) {
	m, c := setupServer(t)
	scanThermal := func() thermalAPI {
		var j Job
		c.json("POST", "/api/scan", map[string]any{}, &j)
		if j = c.wait(j.ID); j.Status != JobDone {
			t.Fatalf("análise falhou: %+v", j)
		}
		var f thermalAPI
		c.json("GET", "/api/facts", nil, &f)
		return f
	}

	m.N.ProcPowers = []sys.ProcPower{{MaxMhz: 2400, MhzLimit: 0}}
	m.W.Set(`root\cimv2`, "Win32_PerfFormattedData_Counters_ThermalZoneInformation",
		sys.WMIRow{"Name": "TZ0", "Temperature": uint32(341), "PercentPassiveLimit": nil, "ThrottleReasons": uint32(0)},
		sys.WMIRow{"Name": "TZ1", "Temperature": uint32(341), "PercentPassiveLimit": uint32(100), "ThrottleReasons": uint32(0)})
	f := scanThermal()
	if f.Thermal.LimitedNow == nil || *f.Thermal.LimitedNow {
		t.Errorf("limite lido como 0: limitedNow deveria ser false, veio %v", f.Thermal.LimitedNow)
	}
	if len(f.Thermal.Zones) != 2 || f.Thermal.Zones[0].PassiveLimitRead == nil || *f.Thermal.Zones[0].PassiveLimitRead ||
		f.Thermal.Zones[1].PassiveLimitRead == nil || !*f.Thermal.Zones[1].PassiveLimitRead {
		t.Errorf("zona com limite nulo deveria vir com passiveLimitRead false: %+v", f.Thermal.Zones)
	}

	m.N.ProcPowers = []sys.ProcPower{{MaxMhz: 2400, MhzLimit: 1900}}
	f = scanThermal()
	if f.Thermal.LimitedNow == nil || !*f.Thermal.LimitedNow {
		t.Errorf("limite de 1900 de 2400 MHz: limitedNow deveria ser true, veio %v", f.Thermal.LimitedNow)
	}
}

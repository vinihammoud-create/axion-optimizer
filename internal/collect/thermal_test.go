package collect_test

import (
	"testing"

	"axion/internal/sys"
)

// The thermal zones come from the performance counter class (whole Kelvin);
// impossible temperatures are a zone without a real sensor. The frequency
// limit is the lowest MhzLimit of the processors.
func TestThermalZonesAndLimit(t *testing.T) {
	m, collectNow := collectMock(t)
	th := collectNow().Thermal
	if !th.ZonesRead || len(th.Zones) != 1 || th.Zones[0].TempC < 67 || th.Zones[0].TempC > 68 || th.Zones[0].PassiveLimitPct != 100 || !th.Zones[0].PassiveLimitRead {
		t.Fatalf("zona do simulador: %+v", th)
	}
	if !th.LimitRead || th.MaxMhz != 4200 || th.MhzLimit != 4200 || th.LimitedNow() {
		t.Fatalf("limite do simulador: %+v", th)
	}
	m.W.Set(`root\cimv2`, "Win32_PerfFormattedData_Counters_ThermalZoneInformation",
		sys.WMIRow{"Name": "TZ1", "Temperature": uint32(2732), "PercentPassiveLimit": uint32(60), "ThrottleReasons": uint32(1)})
	m.N.ProcPowers = []sys.ProcPower{{MaxMhz: 2400, MhzLimit: 2400}, {MaxMhz: 2400, MhzLimit: 1900}}
	th = collectNow().Thermal
	if len(th.Zones) != 1 || th.Zones[0].TempC != -1 || len(th.ThrottledNow()) != 1 {
		t.Fatalf("zona sem sensor real, limitando: %+v", th)
	}
	if th.MhzLimit != 1900 || !th.LimitedNow() {
		t.Fatalf("limite: %+v", th)
	}
	// G-009: the first processor reporting no limit (0) does not hide the
	// limit of the others.
	m.N.ProcPowers = []sys.ProcPower{{MaxMhz: 4000, MhzLimit: 0}, {MaxMhz: 4000, MhzLimit: 2000}}
	if th = collectNow().Thermal; th.MhzLimit != 2000 || !th.LimitedNow() {
		t.Fatalf("primeiro núcleo sem limite: %+v", th)
	}
	// G-009: "% Passive Limit" missing or null is not read, and only a value
	// that came is read (a real 100 included).
	m.W.Set(`root\cimv2`, "Win32_PerfFormattedData_Counters_ThermalZoneInformation",
		sys.WMIRow{"Name": "TZ0", "Temperature": uint32(341), "ThrottleReasons": uint32(1)},
		sys.WMIRow{"Name": "TZ1", "Temperature": uint32(341), "PercentPassiveLimit": nil, "ThrottleReasons": uint32(1)},
		sys.WMIRow{"Name": "TZ2", "Temperature": uint32(341), "PercentPassiveLimit": uint32(100), "ThrottleReasons": uint32(0)})
	th = collectNow().Thermal
	if len(th.Zones) != 3 || th.Zones[0].PassiveLimitRead || th.Zones[1].PassiveLimitRead || !th.Zones[2].PassiveLimitRead || th.Zones[2].PassiveLimitPct != 100 {
		t.Fatalf("limite passivo ausente, nulo e lido: %+v", th.Zones)
	}
	if n := th.ThrottledNow(); len(n) != 0 {
		t.Fatalf("limite não lido não é zona limitando: %+v", n)
	}
	m.W.Set(`root\cimv2`, "Win32_PerfFormattedData_Counters_ThermalZoneInformation")
	m.N.ProcPowersErr = sys.ErrNotFound
	th = collectNow().Thermal
	if !th.ZonesRead || len(th.Zones) != 0 || th.LimitRead {
		t.Fatalf("PC sem zona e sem leitura de limite: %+v", th)
	}
}

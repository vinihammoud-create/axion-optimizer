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
	if !th.ZonesRead || len(th.Zones) != 1 || th.Zones[0].TempC < 67 || th.Zones[0].TempC > 68 || th.Zones[0].PassiveLimitPct != 100 {
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
	m.W.Set(`root\cimv2`, "Win32_PerfFormattedData_Counters_ThermalZoneInformation")
	m.N.ProcPowersErr = sys.ErrNotFound
	th = collectNow().Thermal
	if !th.ZonesRead || len(th.Zones) != 0 || th.LimitRead {
		t.Fatalf("PC sem zona e sem leitura de limite: %+v", th)
	}
}

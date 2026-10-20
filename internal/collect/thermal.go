package collect

import (
	"strings"

	"axion/internal/facts"
)

// StepThermal is the collection step that reads the thermal zones and the
// processors' thermal frequency limit.
const StepThermal = "Temperatura e limite térmico"

// thermal reads what Windows exposes about heat without a driver: the ACPI
// thermal zones through their performance counters (temperature in Kelvin,
// "% Passive Limit", throttle reason) and the frequency limit Windows puts on
// the processors for heat (CallNtPowerInformation, MhzLimit).
func (c *collector) thermal() {
	t := &c.f.Thermal
	if rows, err := c.p.WMI.Query(`root\cimv2`, "SELECT Name, Temperature, PercentPassiveLimit, ThrottleReasons FROM Win32_PerfFormattedData_Counters_ThermalZoneInformation"); err == nil {
		t.ZonesRead = true
		for _, r := range rows {
			name := strings.TrimSpace(r.Str("Name"))
			// The counter reports whole Kelvin. 0 or values outside what a PC
			// can reach (below freezing, above 125 °C) are a zone without a
			// real sensor.
			k := r.Int("Temperature")
			temp := -1.0
			if k >= 273 && k <= 398 {
				temp = float64(k) - 273.15
			}
			limit := int(r.Int("PercentPassiveLimit"))
			if _, ok := r["PercentPassiveLimit"]; !ok {
				limit = 100
			}
			t.Zones = append(t.Zones, facts.ThermalZone{Name: name, TempC: temp, PassiveLimitPct: limit, Reasons: uint32(r.Uint("ThrottleReasons"))})
		}
	} else {
		c.errf("zonas térmicas: %v", err)
	}
	if procs, err := c.p.Native.ProcessorPower(); err == nil && len(procs) > 0 {
		t.LimitRead = true
		for i, p := range procs {
			if p.MaxMhz > t.MaxMhz {
				t.MaxMhz = p.MaxMhz
			}
			if i == 0 || (p.MhzLimit > 0 && p.MhzLimit < t.MhzLimit) {
				t.MhzLimit = p.MhzLimit
			}
		}
	} else if err != nil {
		c.errf("limite térmico do processador: %v", err)
	}
}

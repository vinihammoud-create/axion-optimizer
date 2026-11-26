package collect

import (
	"math"
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
			// A missing or null "% Passive Limit" is not read: neither 100
			// (no limit) nor 0.
			v, ok := r["PercentPassiveLimit"]
			read := ok && v != nil
			limit := 0
			if read {
				limit = int(r.Int("PercentPassiveLimit"))
			}
			var reasons uint32 // bit flags; a value wider than 32 bits is not one
			if u := r.Uint("ThrottleReasons"); u <= math.MaxUint32 {
				reasons = uint32(u)
			}
			t.Zones = append(t.Zones, facts.ThermalZone{Name: name, TempC: temp, PassiveLimitPct: limit, PassiveLimitRead: read, Reasons: reasons})
		}
	} else {
		c.errf("zonas térmicas: %v", err)
	}
	if procs, err := c.p.Native.ProcessorPower(); err == nil && len(procs) > 0 {
		t.LimitRead = true
		for _, p := range procs {
			if p.MaxMhz > t.MaxMhz {
				t.MaxMhz = p.MaxMhz
			}
			// A processor that reports no limit (0) does not hide the
			// limit of the others.
			if p.MhzLimit > 0 && (t.MhzLimit == 0 || p.MhzLimit < t.MhzLimit) {
				t.MhzLimit = p.MhzLimit
			}
		}
	} else if err != nil {
		c.errf("limite térmico do processador: %v", err)
	}
}

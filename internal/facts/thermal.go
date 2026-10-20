package facts

// ThermalInfo is what Windows exposes about heat without a hardware driver:
// the ACPI thermal zones (the "Thermal Zone Information" performance
// counters) and the frequency limit Windows puts on the processors for heat
// (CallNtPowerInformation). Neither is the processor's own temperature: a
// thermal zone is an area of the board or chassis chosen by the manufacturer,
// and many PCs (most desktops, many gaming notebooks) expose none.
type ThermalInfo struct {
	// ZonesRead is true when the counter set answered (an empty list is then
	// a real "this PC exposes no thermal zone").
	ZonesRead bool          `json:"zonesRead"`
	Zones     []ThermalZone `json:"zones,omitempty"`
	// LimitRead is true when CallNtPowerInformation answered.
	LimitRead bool `json:"limitRead"`
	// MaxMhz is the highest maximum specified frequency of the logical
	// processors and MhzLimit the lowest thermal limit among them, in MHz.
	MaxMhz   int `json:"maxMhz"`
	MhzLimit int `json:"mhzLimit"`
}

// ThermalZone is one ACPI thermal zone at the time of the scan.
type ThermalZone struct {
	Name string `json:"name"`
	// TempC is the zone's temperature (-1 when the zone reports none or an
	// impossible value).
	TempC float64 `json:"tempC"`
	// PassiveLimitPct is how much the zone lets the devices it controls run:
	// 100 means not throttled (Microsoft).
	PassiveLimitPct int `json:"passiveLimitPct"`
	// Reasons: 0x1 throttled for heat, 0x2 to limit electrical current.
	Reasons uint32 `json:"reasons"`
}

// Thermal throttle reasons of a zone (Microsoft, "Examples, Requirements and
// Diagnostics", performance counters).
const (
	ThrottleThermal = 0x1
	ThrottleCurrent = 0x2
)

// ThrottledNow reports the zones that limit their devices for heat now.
func (t ThermalInfo) ThrottledNow() []ThermalZone {
	var out []ThermalZone
	for _, z := range t.Zones {
		if z.PassiveLimitPct < 100 && z.Reasons&ThrottleThermal != 0 {
			out = append(out, z)
		}
	}
	return out
}

// LimitedNow is true when Windows holds the processors below their maximum
// frequency for heat right now.
func (t ThermalInfo) LimitedNow() bool {
	return t.LimitRead && t.MaxMhz > 0 && t.MhzLimit > 0 && t.MhzLimit < t.MaxMhz
}

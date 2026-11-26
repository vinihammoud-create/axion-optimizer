package app

import "axion/internal/facts"

// thermalZoneView adds to a zone whether its limit was read: the collector
// stores a null "% Passive Limit" as 0, which is not a real limit.
type thermalZoneView struct {
	facts.ThermalZone
	PassiveLimitRead bool `json:"passiveLimitRead"`
}

// thermalView adds the server's own "limiting now" verdict (the one the
// diagnostic uses), so the page does not recompute it.
type thermalView struct {
	facts.ThermalInfo
	Zones      []thermalZoneView `json:"zones,omitempty"`
	LimitedNow bool              `json:"limitedNow"`
}

// factsView is what /api/facts serves: the facts with the thermal block
// extended by thermalView (the outer Thermal field shadows the embedded one).
type factsView struct {
	*facts.Facts
	Thermal thermalView `json:"thermal"`
}

func newFactsView(f *facts.Facts) factsView {
	v := thermalView{ThermalInfo: f.Thermal, LimitedNow: f.Thermal.LimitedNow()}
	for _, z := range f.Thermal.Zones {
		v.Zones = append(v.Zones, thermalZoneView{ThermalZone: z, PassiveLimitRead: z.PassiveLimitPct > 0})
	}
	return factsView{Facts: f, Thermal: v}
}

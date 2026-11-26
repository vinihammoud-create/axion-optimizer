package app

import "axion/internal/facts"

// thermalView adds the server's own "limiting now" verdict (the one the
// diagnostic uses), so the page does not recompute it. Whether a zone's
// limit was read comes from the collector (facts.ThermalZone.PassiveLimitRead).
type thermalView struct {
	facts.ThermalInfo
	LimitedNow bool `json:"limitedNow"`
}

// factsView is what /api/facts serves: the facts with the thermal block
// extended by thermalView (the outer Thermal field shadows the embedded one).
type factsView struct {
	*facts.Facts
	Thermal thermalView `json:"thermal"`
}

func newFactsView(f *facts.Facts) factsView {
	return factsView{Facts: f, Thermal: thermalView{ThermalInfo: f.Thermal, LimitedNow: f.Thermal.LimitedNow()}}
}

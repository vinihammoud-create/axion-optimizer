package diag

import (
	"strings"
	"testing"
	"time"

	"axion/internal/facts"
	"axion/internal/sys"
)

func thermFind(fs []Finding, id string) *Finding {
	for i := range fs {
		if fs[i].ID == id {
			return &fs[i]
		}
	}
	return nil
}

// The three documented events: 86 (shutdown, critical), 114 (passive cooling,
// counted by day and only in its own channel) and 37 (firmware limit, only
// from FirmwareLimitDays different days).
func TestThermalEvents(t *testing.T) {
	// Noon today, so events an hour apart never straddle midnight.
	now := time.Now()
	now = time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	day := 24 * time.Hour
	ev := func(ch, prov string, id int, ago time.Duration) sys.Event {
		return sys.Event{Channel: ch, Provider: prov, ID: id, Time: now.Add(-ago)}
	}
	if fs := thermalFindings(nil, &facts.Facts{}); len(fs) != 0 {
		t.Fatalf("sem eventos: %+v", fs)
	}
	evs := []sys.Event{
		ev("System", provKPower, 86, 5*day),
		ev(ChanThermal, provKPower, 114, 2*day),
		ev(ChanThermal, provKPower, 114, 2*day-time.Minute),
		ev(ChanThermal, provKPower, 114, 9*day),
		// The same ID in the System log is not the passive cooling event.
		ev("System", provKPower, 114, 1*day),
		ev("System", provKProcPower, 37, 1*day),
		ev("System", provKProcPower, 37, 1*day+time.Hour),
	}
	fs := thermalFindings(evs, &facts.Facts{})
	if x := thermFind(fs, "therm.shutdown"); x == nil || x.Severity != SevCritical || !strings.HasPrefix(x.Title, "1 desligamento") {
		t.Errorf("desligamento: %+v", x)
	}
	if x := thermFind(fs, "therm.throttle"); x == nil || x.Severity != SevWarning || !strings.Contains(x.Title, "2 dias") || !strings.Contains(x.Evidence, "3 registros") {
		t.Errorf("resfriamento passivo: %+v", x)
	}
	if thermFind(fs, "therm.firmware") != nil {
		t.Error("evento 37 em um dia só não deveria avisar")
	}
	evs = append(evs, ev("System", provKProcPower, 37, 3*day), ev("System", provKProcPower, 37, 6*day))
	if x := thermFind(thermalFindings(evs, &facts.Facts{}), "therm.firmware"); x == nil || x.Severity != SevInfo || !strings.Contains(x.Title, "3 dias") || !strings.Contains(x.Explanation, "critério do Axion") {
		t.Errorf("limite do firmware: %+v", x)
	}
}

// The state at the scan: a zone throttling for heat, or a thermal frequency
// limit below the maximum; a zone limited for electrical current is not heat.
func TestThermalNow(t *testing.T) {
	f := &facts.Facts{}
	f.Thermal = facts.ThermalInfo{ZonesRead: true, Zones: []facts.ThermalZone{{Name: `\_TZ.TZ00`, TempC: 68, PassiveLimitPct: 100}}, LimitRead: true, MaxMhz: 2400, MhzLimit: 2400}
	if x := thermFind(thermalFindings(nil, f), "therm.now"); x != nil {
		t.Fatalf("sem limite: %+v", x)
	}
	f.Thermal.Zones[0].PassiveLimitPct, f.Thermal.Zones[0].Reasons = 70, facts.ThrottleCurrent
	if x := thermFind(thermalFindings(nil, f), "therm.now"); x != nil {
		t.Fatalf("limite por corrente não é temperatura: %+v", x)
	}
	f.Thermal.Zones[0].Reasons = facts.ThrottleThermal
	x := thermFind(thermalFindings(nil, f), "therm.now")
	if x == nil || x.Severity != SevWarning || !strings.Contains(x.Evidence, "70%") || !strings.Contains(x.Evidence, "68 °C") {
		t.Fatalf("zona limitando: %+v", x)
	}
	f.Thermal.Zones = nil
	f.Thermal.MhzLimit = 1800
	if x := thermFind(thermalFindings(nil, f), "therm.now"); x == nil || !strings.Contains(x.Evidence, "1800 de 2400 MHz") {
		t.Fatalf("limite de frequência: %+v", x)
	}
}

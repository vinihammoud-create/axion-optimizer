package diag

import (
	"strings"
	"time"

	"axion/internal/facts"
	"axion/internal/health"
	"axion/internal/tweaks"
)

func init() {
	rules = append(rules, healthRule)
}

// healthRule raises the checks of the "Saúde do Windows" page that need
// attention. A check that was not read stays silent here (the page says it
// was not read), and the ones an older rule already reports (reboot, uptime,
// Defender real-time) are not repeated.
func healthRule(f *facts.Facts, _ map[string]tweaks.State, add func(Finding)) {
	for _, c := range health.Evaluate(f, time.Now()) {
		if c.Status != health.Attention || c.Covered != "" {
			continue
		}
		x := Finding{ID: "health." + c.ID, Area: healthArea(c.ID), Severity: c.Severity, Title: c.Headline,
			Evidence: c.Value, Explanation: strings.TrimSpace(c.Detail + " " + c.Criterion), Sources: c.Sources}
		for _, l := range c.Links {
			a := Action{Kind: "url", Label: l.Label, URL: l.URL}
			if l.Page != "" {
				a = Action{Kind: "page", Label: l.Label, Page: l.Page}
			}
			x.Actions = append(x.Actions, a)
		}
		add(x)
	}
}

func healthArea(id string) string {
	switch {
	case strings.HasPrefix(id, "disk."):
		return AreaArmaz
	case strings.HasPrefix(id, "battery."):
		return AreaEstab
	case strings.HasPrefix(id, "boot."):
		return AreaDesempenho
	}
	return AreaSeguranca // Defender and Windows Update
}

package diag

import (
	"testing"

	"axion/internal/profile"
)

// 0.7: the report is read for a use profile. Gaming findings do not score
// on a work PC (they are listed apart as off-profile), critical problems
// always count, and the dismissals still apply.
func TestFinalizeByProfile(t *testing.T) {
	fs := []Finding{
		{ID: "per.rate.mouse", Area: AreaLatencia, Severity: SevWarning, Title: "mouse a 125 Hz"},
		{ID: "display.refresh.1", Area: AreaDesempenho, Severity: SevWarning, Title: "60 Hz"},
		{ID: "ram.xmp", Area: AreaDesempenho, Severity: SevWarning, Title: "XMP"},
		{ID: "disk.health.1", Area: AreaArmaz, Severity: SevCritical, Title: "SSD"},
		{ID: "net.bloat", Area: AreaRede, Severity: SevWarning, Title: "bufferbloat"},
		{ID: "gpu.hags", Area: AreaDesempenho, Severity: SevWarning, Title: "HAGS desligado"},
		{ID: "stab.whea", Area: AreaEstab, Severity: SevInfo, Title: "WHEA"},
	}
	cp := func() []Finding { return append([]Finding(nil), fs...) }
	comp := Finalize(Score(cp()), profile.Competitive, nil)
	// 5 warnings, 1 critical, 1 info: 100-12-25-1 = 62 (under the cap of 74).
	if comp.Profile != profile.Competitive || comp.Overall != 62 {
		t.Fatalf("competitivo: perfil %q nota %d", comp.Profile, comp.Overall)
	}
	for _, x := range comp.Findings {
		if x.OffProfile {
			t.Errorf("no competitivo nada fica fora do perfil: %s", x.ID)
		}
	}
	work := Finalize(Score(cp()), profile.Work, map[string]bool{"net.bloat": true})
	// Counted: disk (critical) and WHEA (info) → 100-12-1 = 87, capped at 74.
	if work.Profile != profile.Work || work.Overall != 74 {
		t.Fatalf("trabalho: nota %d (%+v)", work.Overall, work.Scores)
	}
	off, dis := map[string]bool{}, map[string]bool{}
	for _, x := range work.Findings {
		if x.OffProfile {
			off[x.ID] = true
		}
		if x.Dismissed {
			dis[x.ID] = true
		}
	}
	if !off["per.rate.mouse"] || !off["display.refresh.1"] || !off["ram.xmp"] || !off["gpu.hags"] || off["disk.health.1"] || off["stab.whea"] || off["net.bloat"] {
		t.Errorf("fora do perfil trabalho: %v", off)
	}
	if !dis["net.bloat"] || len(dis) != 1 {
		t.Errorf("dispensados: %v", dis)
	}
	// Off-profile findings come after the active ones and before the dismissed.
	seen := 0
	for _, x := range work.Findings {
		switch {
		case x.Dismissed:
			seen = 2
		case x.OffProfile:
			if seen == 2 {
				t.Fatal("fora do perfil depois dos dispensados")
			}
			seen = 1
		default:
			if seen != 0 {
				t.Fatalf("achado ativo depois dos apartados: %s", x.ID)
			}
		}
	}
	// The area score follows: the memory warning is not counted in the work profile.
	for _, s := range work.Scores {
		if s.Area == AreaDesempenho && (s.Score != 100 || s.Count != 0) {
			t.Errorf("desempenho no trabalho: %+v", s)
		}
	}
	// Live: the gaming items count, the HAGS does not.
	live := Finalize(Score(cp()), profile.Live, nil)
	for _, x := range live.Findings {
		if x.OffProfile != (x.ID == "gpu.hags") {
			t.Errorf("live: %s fora do perfil = %v", x.ID, x.OffProfile)
		}
	}
	// A gaming finding is set apart whatever its severity (a fast monitor at
	// 60 Hz is marked critical for a gamer, not for a spreadsheet PC); a
	// critical finding of general scope counts in every profile.
	crit := Finalize(Score([]Finding{{ID: "display.refresh.1", Area: AreaDesempenho, Severity: SevCritical}, {ID: "disk.health.1", Area: AreaArmaz, Severity: SevCritical}}), profile.Work, nil)
	if crit.Overall != 74 || len(crit.Findings) != 2 || crit.Findings[0].ID != "disk.health.1" || crit.Findings[0].OffProfile || !crit.Findings[1].OffProfile {
		t.Errorf("crítico de jogo fora do perfil, crítico geral conta: %+v", crit.Findings)
	}
	// Empty profile = default, and a nil report stays nil.
	if r := Finalize(Score(cp()), "", nil); r.Profile != profile.Competitive || r.Overall != 62 {
		t.Errorf("perfil vazio: %+v", r.Overall)
	}
	if Finalize(nil, profile.Work, nil) != nil {
		t.Error("nil")
	}
}

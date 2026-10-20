package diag_test

import (
	"strings"
	"testing"
	"time"

	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/profile"
)

func healthFindings(r *diag.Report) map[string]diag.Finding {
	out := map[string]diag.Finding{}
	for _, f := range r.Findings {
		if strings.HasPrefix(f.ID, "health.") {
			out[f.ID] = f
		}
	}
	return out
}

func healthyWindows() *facts.Facts {
	f := base()
	now := time.Now()
	f.Security.DefenderRealtime = "on"
	f.Health = facts.HealthInfo{
		Defender:  facts.DefenderHealth{Read: true, Enabled: true, RealTime: true, Tamper: true, Mode: "Normal", SigAgeDays: 0, ExclusionsRead: true},
		DirtyRead: true,
		Updates:   facts.UpdateHealth{Read: true, Last: now.Add(-5 * 24 * time.Hour), LastKB: "5121794"},
		Boot: facts.BootHealth{Read: true, Samples: []facts.BootSample{
			{At: now.Add(-time.Hour), MainMs: 14000}, {At: now.Add(-30 * time.Hour), MainMs: 13000}, {At: now.Add(-60 * time.Hour), MainMs: 15000}}},
	}
	return f
}

func TestHealthyWindowsRaisesNothing(t *testing.T) {
	if got := healthFindings(diag.Build(healthyWindows(), nil)); len(got) != 0 {
		t.Errorf("achados num Windows saudável: %+v", got)
	}
}

// Nothing read: silent. A zero value is not a problem.
func TestUnreadHealthIsSilent(t *testing.T) {
	f := base()
	f.Power.IsLaptop = true
	if got := healthFindings(diag.Build(f, nil)); len(got) != 0 {
		t.Errorf("achados sem leitura: %+v", got)
	}
	f = healthyWindows()
	f.Health.Defender.SigAgeDays = 60
	f.Health.Dirty = []string{"C:"}
	f.Unread = []string{facts.StepHealth} // the step was set aside after a crash
	if got := healthFindings(diag.Build(f, nil)); len(got) != 0 {
		t.Errorf("etapa não lida gerou achados: %+v", got)
	}
}

func TestHealthFindings(t *testing.T) {
	f := healthyWindows()
	f.Power.IsLaptop = true
	f.Health.Battery = facts.BatteryHealth{Read: true, DesignMWh: 54880, FullMWh: 30000, Cycles: 410, WearPct: 45}
	f.Health.Defender.SigAgeDays = 12
	f.Health.Defender.Tamper = false
	f.Health.Defender.ExclPaths = []string{`C:\Users`}
	f.Health.Dirty = []string{"C:"}
	f.Health.Updates.Last = time.Now().Add(-120 * 24 * time.Hour)
	f.Health.Boot.Samples = []facts.BootSample{{MainMs: 80000}, {MainMs: 90000}, {MainMs: 70000}}
	f.Health.Boot.Slow = []facts.BootApp{{Name: "launcher.exe", Boots: 3, WorstMs: 9000}}
	got := healthFindings(diag.Build(f, nil))
	want := map[string]struct{ area, sev string }{
		"health.defender.signatures": {diag.AreaSeguranca, diag.SevWarning},
		"health.defender.tamper":     {diag.AreaSeguranca, diag.SevInfo},
		"health.defender.exclusions": {diag.AreaSeguranca, diag.SevWarning},
		"health.update.recent":       {diag.AreaSeguranca, diag.SevWarning},
		"health.disk.dirty":          {diag.AreaArmaz, diag.SevWarning},
		"health.battery.wear":        {diag.AreaEstab, diag.SevWarning},
		"health.boot.time":           {diag.AreaDesempenho, diag.SevInfo},
		"health.boot.apps":           {diag.AreaDesempenho, diag.SevInfo},
	}
	if len(got) != len(want) {
		t.Errorf("%d achados, esperava %d: %v", len(got), len(want), got)
	}
	for id, w := range want {
		x, ok := got[id]
		if !ok {
			t.Errorf("falta %s", id)
			continue
		}
		if x.Area != w.area || x.Severity != w.sev || x.Title == "" || x.Evidence == "" || x.Explanation == "" || len(x.Sources) == 0 {
			t.Errorf("%s: %+v", id, x)
		}
	}
	if x := got["health.defender.signatures"]; x.Title != "Defender com assinaturas de 12 dias" || len(x.Actions) != 2 || x.Actions[0].Kind != "url" {
		t.Errorf("assinaturas: %+v", x)
	}
	if x := got["health.boot.time"]; len(x.Actions) != 1 || x.Actions[0].Kind != "page" || x.Actions[0].Page != "inicializacao" {
		t.Errorf("ação da inicialização: %+v", x.Actions)
	}
	if x := got["health.defender.exclusions"]; !strings.Contains(x.Evidence, `C:\Users`) || !strings.Contains(x.Explanation, "Critério do Axion") {
		t.Errorf("exclusões: %+v", x)
	}
}

// The reboot, uptime and Defender real-time checks keep their older findings:
// the health rule does not repeat them.
func TestHealthDoesNotRepeatOlderFindings(t *testing.T) {
	f := healthyWindows()
	f.OS.PendingReboot = true
	f.OS.LastBoot, f.OS.UptimeHours = time.Now().Add(-20*24*time.Hour), 24*20
	f.Security.DefenderRealtime = "off"
	r := diag.Build(f, nil)
	if got := healthFindings(r); len(got) != 0 {
		t.Errorf("repetiu achados que já existem: %+v", got)
	}
	for _, id := range []string{"os.reboot", "os.uptime", "sec.defender"} {
		if find(r, id) == nil {
			t.Errorf("o achado antigo %s sumiu", id)
		}
	}
}

// Health findings count in every profile: a dirty disk is not a gaming item.
func TestHealthFindingsCountInEveryProfile(t *testing.T) {
	f := healthyWindows()
	f.Health.Dirty = []string{"C:"}
	for _, p := range []string{"", profile.Competitive, profile.Live, profile.Work, profile.Creation, profile.Dev} {
		r := diag.Finalize(diag.Build(f, nil), p, nil)
		x := healthFindings(r)["health.disk.dirty"]
		if x.ID == "" || x.OffProfile {
			t.Errorf("perfil %q: %+v", p, x)
		}
	}
}

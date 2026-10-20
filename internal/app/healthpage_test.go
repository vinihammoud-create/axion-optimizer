package app

import (
	"strings"
	"testing"

	"axion/internal/diag"
	"axion/internal/health"
	"axion/internal/sys"
)

func scan(t *testing.T, c *client) {
	t.Helper()
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
}

func TestHealthPage(t *testing.T) {
	_, c := setupServer(t)
	var v HealthView
	c.json("GET", "/api/health", nil, &v)
	if v.Scanned || len(v.Checks) != 0 || len(v.Groups) != 4 || v.Honest == "" || v.Privacy == "" {
		t.Fatalf("antes da análise: %+v", v)
	}
	scan(t, c)
	c.json("GET", "/api/health", nil, &v)
	if !v.Scanned || v.Collected.IsZero() || len(v.Checks) < 10 {
		t.Fatalf("depois da análise: %+v", v)
	}
	if s := v.Summary; s.OK+s.Attention+s.Unread != len(v.Checks) || s.Unread != 0 {
		t.Errorf("resumo: %+v", s)
	}
	// The simulated Windows is healthy: whatever needs attention here is an
	// older finding (reboot, uptime), not one of the new checks.
	for _, ch := range v.Checks {
		if ch.Status != health.OK && ch.Covered == "" {
			t.Errorf("%s: %+v", ch.ID, ch)
		}
	}
	for _, id := range []string{"defender.signatures", "defender.exclusions", "update.recent", "disk.dirty", "boot.time"} {
		found := false
		for _, ch := range v.Checks {
			found = found || ch.ID == id
		}
		if !found {
			t.Errorf("falta a verificação %s", id)
		}
	}
}

func TestHealthPageAndDiagnosticAgree(t *testing.T) {
	m, c := setupServer(t)
	m.W.Set(`root\Microsoft\Windows\Defender`, "MSFT_MpComputerStatus", sys.WMIRow{
		"AntivirusEnabled": true, "RealTimeProtectionEnabled": true, "IsTamperProtected": true, "AMRunningMode": "Normal", "AntivirusSignatureAge": int64(20)})
	m.W.Set(`root\cimv2`, "Win32_Volume", sys.WMIRow{"DriveLetter": "C:", "DriveType": int64(3), "DirtyBitSet": true})
	scan(t, c)
	var v HealthView
	c.json("GET", "/api/health", nil, &v)
	attention := map[string]bool{}
	for _, ch := range v.Checks {
		if ch.Status == health.Attention && ch.Covered == "" {
			attention[ch.ID] = true
		}
	}
	if len(attention) != 2 || !attention["defender.signatures"] || !attention["disk.dirty"] {
		t.Fatalf("a página deveria marcar assinaturas e volume sujo: %v", attention)
	}
	var rep diag.Report
	c.json("GET", "/api/report", nil, &rep)
	got := map[string]diag.Finding{}
	for _, f := range rep.Findings {
		if strings.HasPrefix(f.ID, "health.") {
			got[f.ID] = f
		}
	}
	if len(got) != 2 || got["health.defender.signatures"].Title != "Defender com assinaturas de 20 dias" || got["health.disk.dirty"].Area != diag.AreaArmaz {
		t.Fatalf("o diagnóstico deveria trazer os mesmos dois: %+v", got)
	}
	if got["health.disk.dirty"].OffProfile {
		t.Error("um volume sujo conta em qualquer perfil")
	}
}

// The Defender exclusions can carry the account name in a path: the exported
// report replaces it, like it does everywhere else.
func TestHealthExclusionsAreAnonymousInTheExport(t *testing.T) {
	m, c := setupServer(t)
	const user = "joaogamer"
	m.W.Set(`root\Microsoft\Windows\Defender`, "MSFT_MpPreference", sys.WMIRow{
		"ExclusionPath": []any{`C:\Users\` + user + `\Games\CS2`}, "ExclusionExtension": []any{}, "ExclusionProcess": []any{}})
	scan(t, c)
	// The simulated machine has no account name: give the facts one, as a
	// real Windows has.
	a := c.s.App
	a.mu.Lock()
	a.facts.OS.User = `DESKTOP-TESTE\` + user
	a.mu.Unlock()
	_, full := c.do("GET", "/api/export?full=1", nil, nil)
	_, anon := c.do("GET", "/api/export", nil, nil)
	if !strings.Contains(string(full), user+`\\Games\\CS2`) {
		t.Fatalf("a exclusão deveria estar no relatório completo")
	}
	if strings.Contains(string(anon), user) || !strings.Contains(string(anon), `Games\\CS2`) {
		t.Errorf("relatório anônimo com o nome de usuário ou sem a exclusão")
	}
}

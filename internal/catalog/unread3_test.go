package catalog_test

import (
	"strings"
	"testing"

	"axion/internal/facts"
	"axion/internal/tweaks"
)

// G-005: the VBS switch with the security state or the anti-cheats not read.
func TestVBSOffWithUnreadState(t *testing.T) {
	_, _, e := setup(t)
	tw := e.Catalog.Get("adv.vbs.off")

	// Win32_DeviceGuard did not answer: VBS is unknown, not "already off".
	f := &facts.Facts{}
	ok, why := tw.IsApplicable(f)
	if ok || strings.Contains(why, "já estão desligados") || !strings.Contains(why, "não pôde ser lido") {
		t.Errorf("Device Guard não lido: aplicável=%v %q", ok, why)
	}
	f.Security = facts.SecurityInfo{DeviceGuardRead: true, VBSRunning: true}
	if ok, why := tw.IsApplicable(f); !ok {
		t.Errorf("VBS lido e em execução: %q", why)
	}
	f.Security = facts.SecurityInfo{DeviceGuardRead: true}
	if ok, why := tw.IsApplicable(f); ok || !strings.Contains(why, "já estão desligados") {
		t.Errorf("VBS lido e desligado: aplicável=%v %q", ok, why)
	}

	// Anti-cheats not read: blocked, without saying one is installed.
	f = &facts.Facts{Unread: []string{facts.StepGaming}}
	f.Security = facts.SecurityInfo{DeviceGuardRead: true, VBSRunning: true}
	r := tw.Recommend(f)
	if r.Level != tweaks.RecBlocked || strings.Contains(r.Reason, "você tem") || !strings.Contains(r.Reason, "não puderam ser lidos") {
		t.Errorf("anti-cheats não lidos: %+v", r)
	}
	f.Unread = nil
	f.Gaming.AntiCheats = []string{"FACEIT"}
	if r := tw.Recommend(f); r.Level != tweaks.RecBlocked || !strings.Contains(r.Reason, "FACEIT") {
		t.Errorf("FACEIT instalado: %+v", r)
	}

	// Effect with Device Guard not read: true only means "no warning" (the
	// item never shows "not in effect" for something not read); the state
	// "ok" is not displayed as a confirmation. Same answer as with the whole
	// security step unread (TestVBSOffEffect).
	f = &facts.Facts{}
	if ok, why := tw.Effect(f); !ok || why != "" {
		t.Errorf("Device Guard não lido não pode virar \"não está valendo\": %v %q", ok, why)
	}
}

// A-005: the pointer acceleration not read is not "does not appear on", and
// the texts flagged by REVISAO-DE-MUDANCAS.md are gone.
func TestMouseAccelUnreadAndBannedWords(t *testing.T) {
	_, _, e := setup(t)
	tw := e.Catalog.Get("input.mouse.accel")
	r := tw.Recommend(&facts.Facts{})
	if r.Level == tweaks.RecYes || strings.Contains(r.Reason, "não aparece ligada") || !strings.Contains(r.Reason, "não pôde ser lida") {
		t.Errorf("aceleração não lida: %+v", r)
	}
	read := &facts.Facts{}
	read.Gaming.MouseAccelRead = true
	if r := tw.Recommend(read); r.Level == tweaks.RecYes || !strings.Contains(r.Reason, "não aparece ligada") {
		t.Errorf("aceleração lida e desligada: %+v", r)
	}
	dvr := e.Catalog.Get("game.dvr.history").Recommend(&facts.Facts{})
	ultimate := e.Catalog.Get("power.plan.ultimate")
	for _, s := range []string{dvr.Reason, ultimate.Summary} {
		for _, bad := range []string{"garant", "elimin"} {
			if strings.Contains(strings.ToLower(s), bad) {
				t.Errorf("palavra vetada %q em %q", bad, s)
			}
		}
	}
}

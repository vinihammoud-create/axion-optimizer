package catalog_test

import (
	"context"
	"strings"
	"testing"

	"axion/internal/catalog"
	"axion/internal/collect"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

func withGamePath(t *testing.T) {
	t.Helper()
	catalog.GamePath = func(exe string) string { return `C:\Games\` + exe }
	t.Cleanup(func() { catalog.GamePath = func(string) string { return "" } })
}

// An optimizer pack in the Base Profile: five optimizer settings out of the
// driver default go back, one operation each, and undo restores every user
// value (not the predefined one).
func TestNVDRSResidueRoundTrip(t *testing.T) {
	m, _, e := setup(t)
	breakMachine(m)
	f := collect.Collect(context.Background(), m.Platform, nil)
	tw := e.Catalog.Get("fix.nvdrs.residue")
	if ok, why := tw.IsApplicable(f); !ok {
		t.Fatal(why)
	}
	if n := len(tw.Ops(f)); n != 5 {
		t.Fatalf("esperava 5 operações (energia, filtragem, threads, ULL, pré-renderizados), obtido %d", n)
	}
	for _, op := range tw.Ops(f) {
		if !strings.HasPrefix(op.Target(), "nvdrs:base|") {
			t.Errorf("operação fora do Base Profile: %s", op.Target())
		}
	}
	roundTrip(t, m, f, e, tw)
	// After the round trip the "|user" marks are back (roundTrip compared
	// the World); the shader cache and V-Sync were left to their own fixes.
	w := m.World()
	if w["nvdrs|base|00198fff"] != "0|user" || w["nvdrs|base|00a879cf"] != "1620203033|user" {
		t.Errorf("o resíduo mexeu em ajustes de outras correções: %v %v", w["nvdrs|base|00198fff"], w["nvdrs|base|00a879cf"])
	}
}

func TestNVDRSVSyncAndShaderRoundTrip(t *testing.T) {
	m, _, e := setup(t)
	breakMachine(m)
	f := collect.Collect(context.Background(), m.Platform, nil)
	tw := e.Catalog.Get("fix.nvdrs.vsync")
	if r := tw.Recommendation(f); r.Level != tweaks.RecYes || !strings.Contains(r.Reason, "não conhece") {
		t.Errorf("valor inválido deveria ser recomendado: %+v", r)
	}
	roundTrip(t, m, f, e, tw)
	f = collect.Collect(context.Background(), m.Platform, nil)
	roundTrip(t, m, f, e, e.Catalog.Get("fix.nvdrs.shadercache"))
	if got := m.World()["nvdrs|base|00198fff"]; got != "0|user" {
		t.Errorf("desfazer deveria devolver o valor de usuário 0: %q", got)
	}
	// Apply for real and check the predefined value shows again.
	if r := e.Apply(context.Background(), f, []string{"fix.nvdrs.shadercache"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	if got := m.World()["nvdrs|base|00198fff"]; got != "1|pre" {
		t.Errorf("depois de aplicar, vale o predefinido do driver: %q", got)
	}
}

func TestNVDRSFRLFix(t *testing.T) {
	m, _, e := setup(t)
	m.N.Displays[0].CurrentHz = 240
	m.N.DRS.SetUser("app:cs2.exe", sys.NVDRSFrameRateLimit, 120)
	f := collect.Collect(context.Background(), m.Platform, nil)
	tw := e.Catalog.Get("fix.nvdrs.frl")
	if ok, why := tw.IsApplicable(f); !ok {
		t.Fatal(why)
	}
	if ops := tw.Ops(f); len(ops) != 1 || ops[0].Target() != "nvdrs:app:cs2.exe|10835002" {
		t.Fatalf("operações: %+v", ops)
	}
	if r := tw.Recommendation(f); r.Level != tweaks.RecYes {
		t.Errorf("sem VRR: %+v", r)
	}
	roundTrip(t, m, f, e, tw)
	if got := m.World()["nvdrs|app:cs2.exe|10835002"]; got != "120|user" {
		t.Errorf("limite não voltou: %q", got)
	}
	// With G-SYNC, 237 in 240 Hz is the recommended configuration: no op.
	m.N.DRS.SetUser("app:cs2.exe", sys.NVDRSFrameRateLimit, 237)
	m.N.DRS.VRR[`\\.\DISPLAY1`] = sys.NVVRRInfo{Device: `\\.\DISPLAY1`, Read: true, Possible: true}
	f = collect.Collect(context.Background(), m.Platform, nil)
	if ok, why := tw.IsApplicable(f); ok || !strings.Contains(why, "nada a corrigir") {
		t.Errorf("237 em 240 Hz com G-SYNC: aplicável=%v (%s)", ok, why)
	}
	// Far below the rate with G-SYNC: offered, but neutral.
	m.N.DRS.SetUser("app:cs2.exe", sys.NVDRSFrameRateLimit, 120)
	f = collect.Collect(context.Background(), m.Platform, nil)
	if ok, _ := tw.IsApplicable(f); !ok || tw.Recommendation(f).Level != tweaks.RecNeutral {
		t.Errorf("120 em 240 Hz com G-SYNC: %+v", tw.Recommendation(f))
	}
}

func TestNVPowerModeOnlyInGameProfile(t *testing.T) {
	m, f, e := setup(t)
	withGamePath(t)
	tw := e.Resolve(catalog.ProfileID("cs2.exe", "nvpstate"), f)
	if tw == nil {
		t.Fatal("perfil nvpstate não montado")
	}
	if ok, why := tw.IsApplicable(f); !ok {
		t.Fatal(why)
	}
	for _, op := range tw.Ops(f) {
		if !strings.HasPrefix(op.Target(), "nvdrs:app:") {
			t.Fatalf("modo de energia fora do perfil do jogo: %s", op.Target())
		}
	}
	before := m.World()
	if r := e.Apply(context.Background(), f, []string{tw.ID}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	w := m.World()
	if w["nvdrs|app:cs2.exe|1057eb71"] != "1|user" {
		t.Errorf("perfil do jogo: %q", w["nvdrs|app:cs2.exe|1057eb71"])
	}
	if w["nvdrs|base|1057eb71"] != before["nvdrs|base|1057eb71"] || w["nvdrs|base|1057eb71"] != "5|pre" {
		t.Errorf("o perfil global mudou: %q", w["nvdrs|base|1057eb71"])
	}
	f2 := collect.Collect(context.Background(), m.Platform, nil)
	if st := e.StateOf(context.Background(), f2, tw); st.Status != "on" || !st.ByAxion {
		t.Errorf("estado depois de aplicar: %+v", st)
	}
	e.RevertAll(context.Background(), f2, tweaks.RevertOpts{})
	if d := mocksys.DiffWorld(before, m.World()); len(d) > 0 {
		t.Fatalf("desfazer não restaurou: %v", d)
	}
	// A system process never gets the per-game option.
	if sw := catalog.ProfileProvider(catalog.ProfileID("dwm.exe", "nvpstate"), f); sw != nil {
		if ok, _ := sw.Applicable(f); ok {
			t.Error("dwm.exe não pode ter perfil")
		}
	}
}

// The game profile already had the driver's predefined value: undo goes
// back to "predefined", not to a user value with the same number.
func TestNVPowerModeRestoresPredefined(t *testing.T) {
	m, _, e := setup(t)
	withGamePath(t)
	m.N.DRS.SetPredef("app:cs2.exe", sys.NVDRSPowerMode, sys.NVPowerOptimal)
	f := collect.Collect(context.Background(), m.Platform, nil)
	tw := e.Resolve(catalog.ProfileID("cs2.exe", "nvpstate"), f)
	key := "nvdrs|app:cs2.exe|1057eb71"
	if got := m.World()[key]; got != "5|pre" {
		t.Fatalf("estado inicial: %q", got)
	}
	if r := e.Apply(context.Background(), f, []string{tw.ID}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	if got := m.World()[key]; got != "1|user" {
		t.Fatalf("depois de aplicar: %q", got)
	}
	if r := e.RevertTweaks(context.Background(), f, []string{tw.ID}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer: %+v", r[0])
	}
	if got := m.World()[key]; got != "5|pre" {
		t.Fatalf("depois de desfazer: %q", got)
	}
	// Another program changed the setting after Axion: a conflict, undone
	// only when forced.
	if r := e.Apply(context.Background(), f, []string{tw.ID}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar de novo: %+v", r[0])
	}
	m.N.DRS.SetUser("app:cs2.exe", sys.NVDRSPowerMode, sys.NVPowerAdaptive)
	if r := e.RevertTweaks(context.Background(), f, []string{tw.ID}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutConflict {
		t.Fatalf("esperava conflito: %+v", r[0])
	}
	if r := e.RevertTweaks(context.Background(), f, []string{tw.ID}, tweaks.RevertOpts{Force: true}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer forçado: %+v", r[0])
	}
	if got := m.World()[key]; got != "5|pre" {
		t.Fatalf("depois do desfazer forçado: %q", got)
	}
}

// A PC without NVAPI: every item is not applicable, with the reason, and the
// collection reports no error.
func TestNVDRSNotNvidia(t *testing.T) {
	m, _, e := setup(t)
	withGamePath(t)
	m.N.DRS.Absent = true
	f := collect.Collect(context.Background(), m.Platform, nil)
	for _, e := range f.Errors {
		if strings.Contains(e, "perfil do driver") {
			t.Errorf("erro de coleta indevido: %s", e)
		}
	}
	for _, id := range []string{"fix.nvdrs.vsync", "fix.nvdrs.shadercache", "fix.nvdrs.frl", "fix.nvdrs.residue"} {
		if ok, why := e.Catalog.Get(id).IsApplicable(f); ok || !strings.Contains(why, "não foi lido") {
			t.Errorf("%s: aplicável=%v (%s)", id, ok, why)
		}
	}
	tw := e.Resolve(catalog.ProfileID("cs2.exe", "nvpstate"), f)
	if ok, why := tw.IsApplicable(f); ok || !strings.Contains(why, "perfil do driver NVIDIA não foi lido") {
		t.Errorf("nvpstate: aplicável=%v (%s)", ok, why)
	}
	// Read, but the driver has no profile for the game.
	m.N.DRS.Absent = false
	delete(m.N.DRS.Profiles, "app:cs2.exe")
	f = collect.Collect(context.Background(), m.Platform, nil)
	if ok, why := tw.IsApplicable(f); ok || !strings.Contains(why, "não tem um perfil") {
		t.Errorf("sem perfil do jogo: aplicável=%v (%s)", ok, why)
	}
}

// SaveSettings refused (no administrator): the apply fails, the message
// says why and nothing changed in the driver database.
func TestNVDRSSaveFailsChangesNothing(t *testing.T) {
	m, _, e := setup(t)
	breakMachine(m)
	f := collect.Collect(context.Background(), m.Platform, nil)
	m.N.DRS.SaveErr = &sys.NVAPIError{Func: "NvAPI_DRS_SaveSettings", Status: sys.NVAPIInvalidUserPrivilege}
	before := m.World()
	r := e.Apply(context.Background(), f, []string{"fix.nvdrs.shadercache"}, tweaks.ApplyOpts{})
	if r[0].Outcome != tweaks.OutFailed || !strings.Contains(r[0].Message, "administrador") {
		t.Fatalf("esperava falha com o motivo: %+v", r[0])
	}
	if d := mocksys.DiffWorld(before, m.World(), "hklm\\software\\axion"); len(d) > 0 {
		t.Fatalf("a falha deixou mudança: %v", d)
	}
}

func TestNVDRSVSyncWithVRRIsOptional(t *testing.T) {
	m, _, e := setup(t)
	m.N.DRS.SetUser(sys.NVDRSBase, sys.NVDRSVSyncMode, sys.NVVSyncForceOn)
	m.N.DRS.VRR[`\\.\DISPLAY1`] = sys.NVVRRInfo{Device: `\\.\DISPLAY1`, Read: true, Possible: true}
	f := collect.Collect(context.Background(), m.Platform, nil)
	tw := e.Catalog.Get("fix.nvdrs.vsync")
	if ok, _ := tw.IsApplicable(f); !ok {
		t.Fatal("o item continua disponível para quem quiser")
	}
	if r := tw.Recommendation(f); r.Level != tweaks.RecNo {
		t.Errorf("com G-SYNC, desfazer o V-Sync do painel não é recomendado: %+v", r)
	}
	m.N.DRS.VRR[`\\.\DISPLAY1`] = sys.NVVRRInfo{Device: `\\.\DISPLAY1`, Read: true}
	f = collect.Collect(context.Background(), m.Platform, nil)
	if r := tw.Recommendation(f); r.Level != tweaks.RecYes {
		t.Errorf("sem VRR: %+v", r)
	}
	m.N.DRS.VRR = map[string]sys.NVVRRInfo{}
	f = collect.Collect(context.Background(), m.Platform, nil)
	if r := tw.Recommendation(f); r.Level != tweaks.RecNeutral {
		t.Errorf("VRR desconhecido: %+v", r)
	}
	roundTrip(t, m, f, e, tw)
}

// A fix whose condition is gone after applying is still undone by the
// journal alone (the op is rebuilt from its Spec).
func TestNVDRSUndoFromJournalOnly(t *testing.T) {
	m, _, e := setup(t)
	breakMachine(m)
	f := collect.Collect(context.Background(), m.Platform, nil)
	if r := e.Apply(context.Background(), f, []string{"fix.nvdrs.vsync"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	j, err := tweaks.OpenJournal(m.DataDir, m.Key)
	if err != nil {
		t.Fatal(err)
	}
	empty, _ := tweaks.NewCatalog(nil)
	e2 := tweaks.NewEngine(m.Platform, empty, j)
	f2 := collect.Collect(context.Background(), m.Platform, nil)
	res := e2.RevertAll(context.Background(), f2, tweaks.RevertOpts{})
	if len(res) != 1 || res[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer só pelo journal: %+v", res)
	}
	if got := m.World()["nvdrs|base|00a879cf"]; got != "1620203033|user" {
		t.Errorf("valor anterior não voltou: %q", got)
	}
}

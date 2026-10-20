package profile

import "testing"

// 0.7: the profile decides what counts. Gaming items count in the two
// gaming profiles only, performance items wherever raw performance matters,
// and everything else everywhere; with a live the HAGS is the user's call.
func TestScopes(t *testing.T) {
	cases := []struct {
		id    string
		scope string
	}{
		{"per.rate.mouse", ScopeGaming}, {"display.refresh.1", ScopeGaming}, {"net.valve", ScopeGaming}, {"game.mode", ScopeGaming}, {"input.accel", ScopeGaming}, {"gpu.hags", ScopeGaming}, {"net.wifi", ScopeGaming}, {"power.battery", ScopeGaming},
		{"ram.xmp", ScopePerf}, {"gpu.pcie.width", ScopePerf}, {"cpu.hybrid.win10", ScopePerf},
		{"stab.whea", ScopeGeneral}, {"disk.health.1", ScopeGeneral}, {"sec.anticheat", ScopeGeneral}, {"net.bloat", ScopeGeneral}, {"res.inputqueue", ScopeGeneral}, {"os.win10", ScopeGeneral},
	}
	for _, c := range cases {
		if got := FindingScope(c.id); got != c.scope {
			t.Errorf("%s: %q, esperado %q", c.id, got, c.scope)
		}
	}
	work, comp, live, dev := Get(Work), Get(Competitive), Get(Live), Get(Dev)
	if work.FindingCounts("per.rate.mouse") || !comp.FindingCounts("per.rate.mouse") || !live.FindingCounts("per.rate.mouse") {
		t.Error("taxa do mouse só conta nos perfis de jogo")
	}
	if work.FindingCounts("ram.xmp") || !dev.FindingCounts("ram.xmp") || !comp.FindingCounts("ram.xmp") {
		t.Error("perfil de memória conta onde o desempenho bruto importa")
	}
	if !work.FindingCounts("stab.whea") || !work.FindingCounts("disk.health.1") || !work.FindingCounts("sec.anticheat") {
		t.Error("estabilidade, disco e anti-cheat contam em todo perfil")
	}
	if live.FindingCounts("gpu.hags") || !comp.FindingCounts("gpu.hags") {
		t.Error("HAGS é decisão do usuário no perfil live")
	}
	if live.Informed("game.hags") == "" || comp.Informed("game.hags") != "" || live.Informed("game.mode") != "" {
		t.Error("só o HAGS vira decisão informada, e só na live")
	}
	// What the OBS logs say counts only for who streams or records.
	for _, id := range []string{"obs.encoding", "obs.rendering", "obs.network"} {
		if FindingScope(id) != ScopeLive || !live.FindingCounts(id) || comp.FindingCounts(id) || work.FindingCounts(id) || dev.FindingCounts(id) {
			t.Errorf("%s: deveria contar só no perfil jogo + live", id)
		}
	}
	// Catalog items.
	if work.TweakCounts("game.mode", "jogo") || work.TweakCounts("input.mouse.accel", "entrada") || work.TweakCounts("net.nagle", "rede") || work.TweakCounts("update.drivers.exclude", "update") {
		t.Error("ajustes de jogo ficam fora do 1 clique no trabalho")
	}
	if !work.TweakCounts("privacy.ads", "privacidade") || !work.TweakCounts("fix.bcd.limits", "correcoes") || !work.TweakCounts("power.mode.best", "energia") || !work.TweakCounts("net.nic.power", "rede") {
		t.Error("privacidade, correções, energia na tomada e placa de rede valem para todos")
	}
	if !comp.TweakCounts("game.mode", "jogo") || !comp.TweakCounts("profile:cs2.exe:priority", "jogos") {
		t.Error("tudo conta no competitivo")
	}
	if Get("").ID != Competitive || Get("x").ID != Competitive || Valid("x") || !Valid(Creation) {
		t.Error("padrão e validação")
	}
	if work.OffNote() == "" || comp.OffNote() != "" {
		t.Error("nota de itens fora do perfil só nos perfis sem jogo")
	}
}

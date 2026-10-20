package catalog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"axion/internal/catalog"
	"axion/internal/facts"
	"axion/internal/games"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

// The CS2 preset tests write only the simulated cs2_video.txt in the mock's
// data folder (the hooks are globals: no t.Parallel here).

func setupCS2(t *testing.T, content string) (*mocksys.Mock, *facts.Facts, *tweaks.Engine, string) {
	t.Helper()
	m, f, e := setup(t)
	p, err := m.SeedCS2Video(content)
	if err != nil {
		t.Fatal(err)
	}
	catalog.CS2VideoPath = func() string { return p }
	catalog.ProcessRunning = func(exe string) bool { return len(m.N.PIDsByName(exe)) > 0 }
	t.Cleanup(func() {
		catalog.CS2VideoPath = func() string { return "" }
		catalog.ProcessRunning = func(string) bool { return false }
	})
	e.Providers = append(e.Providers, catalog.CS2VideoProvider)
	return m, f, e, p
}

// heavyVideo: the sample with every preset option above its target.
func heavyVideo(t *testing.T) string {
	t.Helper()
	b := []byte(mocksys.CS2VideoSample)
	for k, v := range map[string]string{"setting.videocfg_ao_detail": "2", "setting.videocfg_particle_detail": "2", "setting.shaderquality": "1", "setting.r_texturefilteringquality": "5", "setting.videocfg_shadow_quality": "3"} {
		var err error
		if b, err = games.SetCS2VideoValue(b, k, v); err != nil {
			t.Fatal(err)
		}
	}
	return string(b)
}

func cs2Val(t *testing.T, p, key string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	v, _, err := games.CS2VideoValue(b, key)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func cs2Set(t *testing.T, p, key, val string) {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if b, err = games.SetCS2VideoValue(b, key, val); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func cs2IDs() []string {
	var ids []string
	for _, it := range catalog.CS2PresetItems() {
		ids = append(ids, it.ID)
	}
	return ids
}

func TestCS2PresetRoundTripEveryItem(t *testing.T) {
	for _, it := range catalog.CS2PresetItems() {
		t.Run(it.ID, func(t *testing.T) {
			m, f, e, _ := setupCS2(t, heavyVideo(t))
			tw := e.Resolve(it.ID, f)
			if tw == nil {
				t.Fatal("provider não resolve")
			}
			// DiffWorld empty also in the files of the cfg folder (no .tmp left).
			roundTrip(t, m, f, e, tw)
		})
	}
}

func TestCS2PresetBatchIsByteExact(t *testing.T) {
	_, f, e, p := setupCS2(t, heavyVideo(t))
	orig, _ := os.ReadFile(p)
	ids := cs2IDs()
	for _, r := range e.Apply(context.Background(), f, ids, tweaks.ApplyOpts{Label: catalog.CS2PresetLabel}) {
		if r.Outcome != tweaks.OutApplied {
			t.Fatalf("%s: %s %s", r.TweakID, r.Outcome, r.Message)
		}
	}
	now, _ := os.ReadFile(p)
	for k, want := range map[string]string{"setting.msaa_samples": "0", "setting.r_csgo_cmaa_enable": "1", "setting.r_low_latency": "1", "setting.videocfg_shadow_quality": "1", "setting.r_texturefilteringquality": "3"} {
		if v, _, _ := games.CS2VideoValue(now, k); v != want {
			t.Errorf("%s = %s", k, v)
		}
	}
	for k, keep := range map[string]string{"setting.defaultres": "1280", "setting.defaultresheight": "960", "setting.mat_vsync": "0"} {
		if v, _, _ := games.CS2VideoValue(now, k); v != keep {
			t.Errorf("%s mudou: %s", k, v)
		}
	}
	if !bytes.Contains(now, []byte("\r\n")) {
		t.Error("perdeu o CRLF")
	}
	if bytes.Equal(now, orig) {
		t.Fatal("o preset não mudou nada")
	}
	for _, r := range e.RevertTweaks(context.Background(), f, ids, tweaks.RevertOpts{}) {
		if r.Outcome != tweaks.OutReverted {
			t.Errorf("desfazer %s: %s %s", r.TweakID, r.Outcome, r.Message)
		}
	}
	back, _ := os.ReadFile(p)
	if !bytes.Equal(back, orig) {
		t.Fatal("desfazer não devolveu o arquivo byte a byte")
	}
	ents, _ := os.ReadDir(filepath.Dir(p))
	for _, en := range ents {
		if strings.Contains(en.Name(), ".axion-") {
			t.Errorf("temporário sobrou: %s", en.Name())
		}
	}
	if n := len(e.J.Active()); n != 0 {
		t.Errorf("ainda há %d entradas ativas depois de desfazer tudo", n)
	}
}

func TestCS2PresetRefusedWhileGameRuns(t *testing.T) {
	m, f, e, p := setupCS2(t, mocksys.CS2VideoSample)
	orig, _ := os.ReadFile(p)
	m.N.AddProc(sys.ProcessInfo{PID: 7777, Name: "cs2.exe"})
	r := e.Apply(context.Background(), f, []string{"cs2video:aa"}, tweaks.ApplyOpts{})
	if r[0].Outcome != tweaks.OutNA || !strings.Contains(r[0].Message, "feche o jogo") {
		t.Fatalf("%+v", r[0])
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, orig) {
		t.Fatal("gravou com o jogo aberto")
	}
	if len(e.J.Active()) != 0 {
		t.Fatal("entrada no histórico sem mudança")
	}
	m.N.RemoveProcs("cs2.exe")
	if r := e.Apply(context.Background(), f, []string{"cs2video:aa"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0].Message)
	}
	applied, _ := os.ReadFile(p)
	m.N.AddProc(sys.ProcessInfo{PID: 7778, Name: "cs2.exe"})
	rv := e.RevertTweaks(context.Background(), f, []string{"cs2video:aa"}, tweaks.RevertOpts{})
	if rv[0].Outcome != tweaks.OutFailed || !strings.Contains(rv[0].Message, "Feche o Counter-Strike 2") {
		t.Fatalf("%+v", rv[0])
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, applied) {
		t.Fatal("desfez com o jogo aberto")
	}
	m.N.RemoveProcs("cs2.exe")
	if rv := e.RevertTweaks(context.Background(), f, []string{"cs2video:aa"}, tweaks.RevertOpts{}); rv[0].Outcome != tweaks.OutReverted {
		t.Fatalf("%+v", rv[0])
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, orig) {
		t.Fatal("não voltou ao original")
	}
}

// The player changed the menu after the preset: undo gives back only what
// the Axion changed.
func TestCS2PresetKeepsLaterMenuChanges(t *testing.T) {
	_, f, e, p := setupCS2(t, mocksys.CS2VideoSample)
	if r := e.Apply(context.Background(), f, []string{"cs2video:aa"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0].Message)
	}
	cs2Set(t, p, "setting.defaultres", "1920")
	if rv := e.RevertTweaks(context.Background(), f, []string{"cs2video:aa"}, tweaks.RevertOpts{}); rv[0].Outcome != tweaks.OutReverted {
		t.Fatalf("%+v", rv[0])
	}
	for k, want := range map[string]string{"setting.msaa_samples": "2", "setting.r_csgo_cmaa_enable": "0", "setting.defaultres": "1920"} {
		if v := cs2Val(t, p, k); v != want {
			t.Errorf("%s = %s, esperado %s", k, v, want)
		}
	}
}

func TestCS2PresetConflictAndForce(t *testing.T) {
	_, f, e, p := setupCS2(t, mocksys.CS2VideoSample)
	if r := e.Apply(context.Background(), f, []string{"cs2video:aa"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0].Message)
	}
	cs2Set(t, p, "setting.msaa_samples", "4")
	rv := e.RevertTweaks(context.Background(), f, []string{"cs2video:aa"}, tweaks.RevertOpts{})
	if rv[0].Outcome != tweaks.OutConflict {
		t.Fatalf("esperava conflito: %+v", rv[0])
	}
	if cs2Val(t, p, "setting.msaa_samples") != "4" {
		t.Fatal("o conflito mexeu no arquivo")
	}
	rv = e.RevertTweaks(context.Background(), f, []string{"cs2video:aa"}, tweaks.RevertOpts{Force: true})
	if rv[0].Outcome != tweaks.OutReverted || cs2Val(t, p, "setting.msaa_samples") != "2" {
		t.Fatalf("forçar: %+v msaa=%s", rv[0], cs2Val(t, p, "setting.msaa_samples"))
	}
}

// The game (or the cloud) put values back after the Axion: the state says
// "changed after the Axion", also when only the CMAA2 key goes back.
func TestCS2PresetDrift(t *testing.T) {
	_, f, e, p := setupCS2(t, mocksys.CS2VideoSample)
	tw := e.Resolve("cs2video:aa", f)
	if r := e.Apply(context.Background(), f, []string{tw.ID}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0].Message)
	}
	if st := e.StateOf(context.Background(), f, tw); st.Status != "on" || st.Drifted {
		t.Fatalf("logo depois de aplicar: %+v", st)
	}
	// 1) only the CMAA2 key goes back to 0 (MSAA stays at 0).
	cs2Set(t, p, "setting.r_csgo_cmaa_enable", "0")
	if st := e.StateOf(context.Background(), f, tw); !st.Drifted || !st.ByAxion {
		t.Fatalf("só o CMAA2 voltou: %+v", st)
	}
	// 2) both go back.
	cs2Set(t, p, "setting.msaa_samples", "2")
	if st := e.StateOf(context.Background(), f, tw); !st.Drifted {
		t.Fatalf("os dois voltaram: %+v", st)
	}
}

func TestCS2PresetFileGone(t *testing.T) {
	_, f, e, p := setupCS2(t, mocksys.CS2VideoSample)
	if r := e.Apply(context.Background(), f, []string{"cs2video:aa"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0].Message)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if rv := e.RevertTweaks(context.Background(), f, []string{"cs2video:aa"}, tweaks.RevertOpts{}); rv[0].Outcome != tweaks.OutReverted {
		t.Fatalf("%+v", rv[0])
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatal("desfazer recriou o arquivo")
	}
}

func TestCS2PresetReadOnly(t *testing.T) {
	_, f, e, p := setupCS2(t, mocksys.CS2VideoSample)
	orig, _ := os.ReadFile(p)
	if err := os.Chmod(p, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	r := e.Apply(context.Background(), f, []string{"cs2video:aa"}, tweaks.ApplyOpts{})
	if r[0].Outcome != tweaks.OutNA || !strings.Contains(r[0].Message, "somente leitura") {
		t.Fatalf("%+v", r[0])
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, orig) {
		t.Fatal("mexeu no arquivo somente leitura")
	}
	if st, _ := os.Stat(p); st.Mode().Perm()&0o200 != 0 {
		t.Fatal("o Axion tirou o atributo de somente leitura")
	}
}

func TestCS2PresetCleansOldTempsOnly(t *testing.T) {
	_, f, e, p := setupCS2(t, mocksys.CS2VideoSample)
	dir := filepath.Dir(p)
	old := filepath.Join(dir, "cs2_video.txt.axion-111.tmp")
	fresh := filepath.Join(dir, "cs2_video.txt.axion-222.tmp")
	for _, n := range []string{old, fresh} {
		if err := os.WriteFile(n, []byte("sobra"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	when := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, when, when); err != nil {
		t.Fatal(err)
	}
	if r := e.Apply(context.Background(), f, []string{"cs2video:aa"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0].Message)
	}
	if _, err := os.Stat(old); err == nil {
		t.Error("o temporário de mais de 1 h não foi apagado")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("o temporário recente (pode ser de outra gravação) foi apagado")
	}
}

// A link is never replaced (skipped when the system does not allow links).
func TestCS2PresetRefusesLink(t *testing.T) {
	_, f, e, p := setupCS2(t, mocksys.CS2VideoSample)
	real := filepath.Join(t.TempDir(), "real.txt")
	if err := os.WriteFile(real, []byte(mocksys.CS2VideoSample), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, p); err != nil {
		t.Skipf("sem permissão para criar link: %v", err)
	}
	r := e.Apply(context.Background(), f, []string{"cs2video:reflex"}, tweaks.ApplyOpts{})
	if r[0].Outcome != tweaks.OutFailed || !strings.Contains(r[0].Message, "link") {
		t.Fatalf("%+v", r[0])
	}
	if fi, err := os.Lstat(p); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("o link foi substituído")
	}
	if b, _ := os.ReadFile(real); string(b) != mocksys.CS2VideoSample {
		t.Fatal("o destino do link mudou")
	}
}

func TestCS2PresetReflexOnlyNVIDIA(t *testing.T) {
	_, f, e, _ := setupCS2(t, mocksys.CS2VideoSample)
	tw := e.Resolve("cs2video:reflex", f)
	if ok, why := tw.IsApplicable(f); !ok {
		t.Fatalf("NVIDIA: %s", why)
	}
	f2 := *f
	f2.GPUs = append([]facts.GPUInfo(nil), f.GPUs...)
	for i := range f2.GPUs {
		f2.GPUs[i].Vendor = "AMD"
	}
	if ok, why := tw.IsApplicable(&f2); ok || !strings.Contains(why, "NVIDIA") {
		t.Fatalf("AMD: ok=%v %q", ok, why)
	}
	if ok, _ := e.Resolve("cs2video:aa", &f2).IsApplicable(&f2); !ok {
		t.Error("o AA não depende da marca da placa")
	}
}

func TestCS2PresetMetadata(t *testing.T) {
	items := catalog.CS2PresetItems()
	if len(items) == 0 {
		t.Fatal("sem itens")
	}
	_, f, e, _ := setupCS2(t, mocksys.CS2VideoSample)
	banned := []string{"elimina", "garante", "microssegundos"}
	for _, it := range items {
		tw := e.Resolve(it.ID, f)
		if tw == nil {
			t.Fatalf("%s não resolve", it.ID)
		}
		if tw.Category != "jogos" || tw.Risk != tweaks.RiskSafe || tw.OneClick || tw.Reboot != tweaks.RebootRestartApp {
			t.Errorf("%s: categoria/risco/um clique/reinício: %+v", it.ID, tw)
		}
		if len(tw.Summary) < 40 || len(tw.Detail) < 60 || tw.Evidence == 0 {
			t.Errorf("%s: texto curto ou sem evidência", it.ID)
		}
		if len(tw.Sources) == 0 {
			t.Errorf("%s: sem fonte", it.ID)
		}
		for _, s := range tw.Sources {
			if !strings.HasPrefix(s.URL, "https://") || s.Title == "" {
				t.Errorf("%s: fonte inválida %+v", it.ID, s)
			}
		}
		switch it.Kind {
		case "hipotese":
			hasTag := false
			for _, tg := range tw.Tags {
				hasTag = hasTag || tg == "hipotese"
			}
			if tw.Evidence != tweaks.EvLow || !hasTag || !strings.Contains(it.Summary, "medir") || !strings.Contains(it.Detail, "preferência sua") {
				t.Errorf("%s: hipótese sem o selo de hipótese a medir", it.ID)
			}
		case "medido":
			if tw.Evidence != tweaks.EvMedium {
				t.Errorf("%s: evidência %v", it.ID, tw.Evidence)
			}
		case "latencia":
			if !strings.Contains(it.Summary, "não é ganho de FPS") {
				t.Errorf("%s: o texto precisa dizer que não é ganho de FPS", it.ID)
			}
		default:
			t.Errorf("%s: Kind %q", it.ID, it.Kind)
		}
		low := strings.ToLower(tw.Title + tw.Summary + tw.Detail)
		for _, b := range banned {
			if strings.Contains(low, b) {
				t.Errorf("%s: texto promete demais (%q)", it.ID, b)
			}
		}
		if r := tw.Recommendation(f); r.Level == tweaks.RecYes {
			t.Errorf("%s: recomendado em 1 clique", it.ID)
		}
		if it.Evidence == tweaks.EvHigh && it.ID != "cs2video:reflex" {
			t.Errorf("%s: evidência alta sem documentação do ganho", it.ID)
		}
	}
}

func TestCS2PresetNeverTouchesPersonalKeys(t *testing.T) {
	m, _, _, p := setupCS2(t, mocksys.CS2VideoSample)
	never := map[string]bool{}
	for _, k := range catalog.CS2NeverTouch {
		never[strings.ToLower(k)] = true
	}
	for _, it := range catalog.CS2PresetItems() {
		for _, s := range it.Sets {
			if never[strings.ToLower(s.Key)] {
				t.Errorf("%s grava %s, que é preferência do jogador", it.ID, s.Key)
			}
		}
	}
	// A forged Spec in the journal: another file (valid format, so only the
	// GUARD can refuse), or a key outside the preset.
	bad := filepath.Join(t.TempDir(), "win.ini")
	if err := os.WriteFile(bad, []byte(mocksys.CS2VideoSample), 0o644); err != nil {
		t.Fatal(err)
	}
	forge := func(path, key, want string) tweaks.Op {
		spec, _ := json.Marshal(map[string]any{"path": path, "key": key, "want": want})
		op := tweaks.Reconstruct(&tweaks.OpRecord{Kind: "cs2video", Target: "cs2video:" + strings.ToLower(path) + "|" + strings.ToLower(key), Spec: spec})
		if op == nil {
			t.Fatalf("Reconstruct devolveu nil para %s|%s", path, key)
		}
		return op
	}
	c := &tweaks.Ctx{Context: context.Background(), P: m.Platform}
	type forged struct{ path, key, want string }
	for name, tc := range map[string]forged{
		"outro arquivo":      {bad, "setting.msaa_samples", "0"},
		"resolução":          {p, "setting.defaultres", "640"},
		"valor não numérico": {p, "setting.msaa_samples", "0 -allow"},
	} {
		before, _ := os.ReadFile(tc.path)
		op := forge(tc.path, tc.key, tc.want)
		if err := op.Apply(c); err == nil {
			t.Errorf("%s: gravou", name)
		}
		if after, _ := os.ReadFile(tc.path); !bytes.Equal(before, after) {
			t.Errorf("%s: o arquivo mudou", name)
		}
	}
}

func TestCS2OpsRebuildable(t *testing.T) {
	_, f, e, _ := setupCS2(t, mocksys.CS2VideoSample)
	for _, it := range catalog.CS2PresetItems() {
		for _, op := range e.Resolve(it.ID, f).Ops(f) {
			fo, ok := op.(*tweaks.FuncOp)
			if !ok {
				t.Fatalf("%s: a operação não é FuncOp", it.ID)
			}
			got := tweaks.Reconstruct(&tweaks.OpRecord{Kind: fo.Kind(), Target: fo.Target(), Spec: fo.SpecJSON()})
			if got == nil || got.Target() != fo.Target() {
				t.Errorf("%s: não remonta pelo journal: %v", it.ID, got)
			}
		}
	}
}

func TestCS2PresetHonoursExclusions(t *testing.T) {
	_, f, e, _ := setupCS2(t, mocksys.CS2VideoSample)
	for _, it := range catalog.CS2PresetItems() {
		for _, op := range e.Resolve(it.ID, f).Ops(f) {
			if name, bad := excludedHit(op.Target()); bad {
				t.Errorf("%s grava %s, que a página \"O que não fazemos\" recusa (%s)", it.ID, op.Target(), name)
			}
		}
	}
	for _, tgt := range []string{`x:\steam\userdata\1\730\local\cfg\cs2_user_keys_0_slot0.vcfg|bind`, `x:\cfg\autoexec.cfg|x`, "cl_allow_multi_input_binds"} {
		if _, hit := excludedHit(tgt); !hit {
			t.Errorf("a entrada nova não pega %s", tgt)
		}
	}
	found := false
	for _, ex := range catalog.Excluded() {
		for _, tg := range ex.Targets {
			if tg == "cs2_user_keys" {
				found = true
				if len(ex.Sources) == 0 {
					t.Error("a entrada de binds está sem fonte")
				}
			}
		}
	}
	if !found {
		t.Error("falta a entrada de binds/autoexec em \"O que não fazemos\"")
	}
}

// The Need filter (page and route) does not depend on Applicable.
func TestCS2ItemWanted(t *testing.T) {
	sample := []byte(mocksys.CS2VideoSample)
	if ok, _ := catalog.CS2ItemWanted("cs2video:aa", sample); !ok {
		t.Error("MSAA 2x: o AA é desejado")
	}
	none, _ := games.SetCS2VideoValue(sample, "setting.msaa_samples", "0")
	if ok, why := catalog.CS2ItemWanted("cs2video:aa", none); ok || !strings.Contains(why, "não liga") {
		t.Errorf("sem AA nenhum: ok=%v %q", ok, why)
	}
	cmaa, _ := games.SetCS2VideoValue(none, "setting.r_csgo_cmaa_enable", "1")
	if ok, _ := catalog.CS2ItemWanted("cs2video:aa", cmaa); !ok {
		t.Error("já em CMAA2: continua desejado (o item mostra que já está feito)")
	}
	if ok, _ := catalog.CS2ItemWanted("cs2video:reflex", sample); !ok {
		t.Error("o Reflex não tem filtro")
	}
	if ok, _ := catalog.CS2ItemWanted("game.ducking", sample); ok {
		t.Error("item fora do preset")
	}
}

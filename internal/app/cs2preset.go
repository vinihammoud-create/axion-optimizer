package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"axion/internal/catalog"
	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/games"
	"axion/internal/tweaks"
)

// Preset competitivo do CS2 (0.9): the view of the card on the CS2 page, the
// route that applies it and the safety copy of cs2_video.txt. What is
// written, how and why is in internal/catalog/cs2video.go. The undo is the
// journal (POST /api/revert with Active, or "Desfazer lote" in the history),
// not the copy.

const cs2BackupKeep = 10

var srcFSRGPUOpen = tweaks.Source{Title: "AMD GPUOpen — FidelityFX Super Resolution: gera quadros de alta resolução a partir de entradas de resolução menor (por isso desligar pode baixar o FPS)", URL: "https://gpuopen.com/fidelityfx-superresolution/"}

// CS2PresetChange is one value the preset would change (before → after).
type CS2PresetChange struct {
	Key       string `json:"key"`
	Title     string `json:"title"`
	From      string `json:"from"`
	To        string `json:"to"`
	FromLabel string `json:"fromLabel"`
	ToLabel   string `json:"toLabel"`
}

// CS2PresetItem is one option of the preset on the page.
type CS2PresetItem struct {
	ID         string            `json:"id"`
	Title      string            `json:"title"`
	Summary    string            `json:"summary"`
	Kind       string            `json:"kind"` // medido | latencia | hipotese
	Evidence   string            `json:"evidence"`
	Sources    []tweaks.Source   `json:"sources,omitempty"`
	Changes    []CS2PresetChange `json:"changes"`
	Selected   bool              `json:"selected"`
	Why        string            `json:"why,omitempty"`
	Applicable bool              `json:"applicable"`
	NAReason   string            `json:"naReason,omitempty"`
	Status     string            `json:"status"`
	ByAxion    bool              `json:"byAxion"`
	Drifted    bool              `json:"drifted"`
	EntryID    string            `json:"entryId,omitempty"`
}

// CS2Preset is the card "Preset competitivo (cs2_video.txt)".
type CS2Preset struct {
	Path      string          `json:"path"`
	Running   bool            `json:"running"`
	Cloud     string          `json:"cloud,omitempty"` // "sim" | "não" | "" (not read)
	CloudNote string          `json:"cloudNote"`
	Basis     string          `json:"basis"`
	Honest    string          `json:"honest"`
	Items     []CS2PresetItem `json:"items"`
	Untouched []CheckItem     `json:"untouched"`
	Active    []string        `json:"active"` // IDs applied by the Axion (for "Desfazer preset")
}

// cs2VideoPath is the cs2_video.txt the preset may write. In the demo only
// the file seeded inside the data folder counts: the demo's fake path is a
// real one on a PC that has Steam in that folder. It takes the games lock
// for reading: never call it while that lock is held for writing (scanCS2).
func (a *App) cs2VideoPath() string {
	a.gmu.RLock()
	defer a.gmu.RUnlock()
	if a.cs2.Video == nil {
		return ""
	}
	p := a.cs2.Video.Path
	if a.P.IsMock && !strings.HasPrefix(strings.ToLower(p), strings.ToLower(a.P.DataDir)+string(filepath.Separator)) {
		return ""
	}
	return p
}

// mockCS2VideoPath must match mocksys.CS2VideoPath (a test checks it).
func mockCS2VideoPath(dataDir string) string {
	return filepath.Join(dataDir, "cs2sim", "Steam", "userdata", "123", "730", "local", "cfg", "cs2_video.txt")
}

// cs2Preset builds the card; nil when there is no readable cs2_video.txt.
func (a *App) cs2Preset(f *facts.Facts, info games.CS2Info, m *diag.Measured) *CS2Preset {
	if info.Video == nil || info.Video.Path == "" {
		return nil
	}
	b, err := os.ReadFile(info.Video.Path)
	if err != nil {
		return nil
	}
	p := &CS2Preset{Path: info.Video.Path, Running: len(a.P.Native.PIDsByName("cs2.exe")) > 0,
		Honest: "O preset muda só opções do menu Vídeo no cs2_video.txt, com o jogo fechado, e guarda cada valor anterior no histórico. Ganho de FPS só aparece quando a placa de vídeo limita; meça antes e depois na página Desempenho.",
		Active: []string{}}
	short, basis := gpuShort(f, m)
	p.Basis = basis
	switch info.VideoCloud {
	case "não":
		p.Cloud, p.CloudNote = "não", "Nesta conta, a lista da Steam Cloud do CS2 (remotecache.vdf) não inclui o cs2_video.txt, então a nuvem não deve sobrescrever estas opções. Se algum valor voltar sozinho, o item aparece como alterado depois do Axion."
	case "sim":
		p.Cloud, p.CloudNote = "sim", "Esta conta sincroniza o cs2_video.txt pela Steam Cloud: outro PC pode sobrescrever as opções ao abrir o jogo. Se isso acontecer, o item aparece como alterado depois do Axion."
	default:
		p.CloudNote = "Não deu para ler a lista da Steam Cloud desta conta; se algum valor voltar sozinho, o item aparece como alterado depois do Axion."
	}
	ctx := context.Background()
	for _, it := range catalog.CS2PresetItems() {
		t := a.Engine.Resolve(it.ID, f)
		if t == nil {
			continue
		}
		st := a.Engine.StateOf(ctx, f, t)
		pi := CS2PresetItem{ID: it.ID, Title: it.Title, Summary: it.Summary, Kind: it.Kind, Evidence: it.Evidence.String(), Sources: it.Sources,
			Changes: []CS2PresetChange{}, Applicable: st.Applicable, NAReason: st.NAReason, Status: st.Status, ByAxion: st.ByAxion, Drifted: st.Drifted, EntryID: st.EntryID}
		for _, s := range it.Sets {
			v, ok, _ := games.CS2VideoValue(b, s.Key)
			if !ok || v == s.Want || slices.Contains(s.OK, v) {
				continue
			}
			pi.Changes = append(pi.Changes, CS2PresetChange{Key: s.Key, Title: presetTitle(s.Key), From: v, To: s.Want, FromLabel: presetLabel(s.Key, v), ToLabel: presetLabel(s.Key, s.Want)})
		}
		wanted, wantWhy := catalog.CS2ItemWanted(it.ID, b)
		ready := st.Applicable && st.Status != "on" && len(pi.Changes) > 0 && wanted
		switch {
		case ready && it.Kind != "hipotese":
			pi.Selected = true
		case ready && short:
			pi.Selected, pi.Why = true, "a sua medição mostrou a placa de vídeo limitando: "+basis
		case ready:
			pi.Why = "desmarcado: " + basis + "; baixar esta opção não deve subir o FPS"
		case st.Applicable && !wanted:
			pi.Why = wantWhy
		case st.Applicable && len(pi.Changes) == 0:
			pi.Why = "já no valor do preset"
		}
		if st.ByAxion {
			p.Active = append(p.Active, it.ID)
		}
		p.Items = append(p.Items, pi)
	}
	p.Untouched = cs2Untouched(info)
	return p
}

// gpuShort: the same verdicts as videoAdvice, without the text. It is true
// only when the GPU limited and the 1% low stayed under the monitor's rate.
func gpuShort(f *facts.Facts, m *diag.Measured) (bool, string) {
	if m == nil || m.Perf == nil {
		return false, "sem medição do CS2 (grave 60 s na página Desempenho)"
	}
	s := m.Perf
	gpuBound := s.Limiter == "gpu" || s.GPUBoundPct >= 50
	switch {
	case s.Limiter == "limite" || s.CapLikely:
		return false, "o jogo estava no próprio limite de FPS (fps_max)"
	case s.Limiter == "cpu" || (s.CPUBoundPct >= 50 && !gpuBound):
		return false, "a CPU limitou o FPS na última medição"
	case !gpuBound:
		return false, "a última medição não apontou um limitador claro"
	}
	hz, _ := gameDisplay(f)
	if hz <= 0 {
		return false, "taxa do monitor não lida"
	}
	if time.Since(m.PerfAt) > 45*24*time.Hour {
		return false, "a medição tem mais de 45 dias: grave de novo"
	}
	if s.Low1FPS >= float64(hz) {
		return false, fmt.Sprintf("o 1%% low (%s) já ficou acima dos %d Hz do monitor", fmtNum(s.Low1FPS), hz)
	}
	return true, fmt.Sprintf("placa de vídeo limitou %s%% dos quadros, 1%% low %s", fmtNum(s.GPUBoundPct), fmtNum(s.Low1FPS))
}

// presetTitle: the title of the video option (the page's own list), or the
// name of the keys that only the preset knows.
func presetTitle(key string) string {
	for _, o := range videoOptions {
		if o.Key == key {
			return o.Title
		}
	}
	switch key {
	case "setting.r_csgo_cmaa_enable":
		return "CMAA2"
	case "setting.r_low_latency":
		return "NVIDIA Reflex"
	}
	return key
}

// presetLabel: the label of the value as the game's menu shows it.
func presetLabel(key, v string) string {
	n, err := atoiTrim(v)
	switch key {
	case "setting.r_low_latency":
		if err == nil && n >= 0 && n < 3 {
			return []string{"desativado", "ativado", "ativado + boost"}[n]
		}
	case "setting.r_csgo_cmaa_enable":
		if err == nil && n >= 0 && n < 2 {
			return []string{"desligado", "CMAA2"}[n]
		}
	}
	for _, o := range videoOptions {
		if o.Key == key && err == nil {
			return o.label(n)
		}
	}
	return v
}

func atoiTrim(v string) (int, error) {
	var n int
	_, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &n)
	return n, err
}

// cs2Untouched lists what the preset never changes: the player's choice or
// the hardware's.
func cs2Untouched(info games.CS2Info) []CheckItem {
	raw := map[string]string{}
	if info.Video != nil {
		raw = info.Video.Raw
	}
	val := func(keys ...string) string {
		var out []string
		for _, k := range keys {
			if v, ok := raw[k]; ok {
				out = append(out, v)
			}
		}
		return strings.Join(out, " / ")
	}
	res := ""
	if w, h := raw["setting.defaultres"], raw["setting.defaultresheight"]; w != "" && h != "" {
		res = w + "×" + h
	}
	hz := ""
	if info.Video != nil && info.Video.RefreshHz > 0 {
		hz = strings.Replace(fmt.Sprintf("%.2f", info.Video.RefreshHz), ".", ",", 1) + " Hz"
	}
	mode := ""
	switch raw["setting.fullscreen"] {
	case "1":
		mode = "tela cheia"
	case "0":
		mode = "janela"
	}
	if raw["setting.nowindowborder"] == "1" {
		mode = "janela sem bordas"
	}
	fps := "(não gravado nas convars lidas)"
	if v, ok := info.Convars["fps_max"]; ok {
		fps = v
	}
	contrast := "(não lido)"
	if v, ok := info.Convars["r_player_visibility_mode"]; ok {
		contrast = v
	}
	fsr := val("setting.videocfg_fsr_detail")
	item := func(id, title, advice, current string, src ...tweaks.Source) CheckItem {
		return CheckItem{ID: id, Title: title, Status: "info", Evidence: "—", Advice: advice, Current: current, Sources: src}
	}
	return []CheckItem{
		item("cs2.keep.res", "Resolução e proporção", "Preferência sua; o Axion não muda.", res),
		item("cs2.keep.hz", "Taxa de atualização", "O jogo guarda frações como 164997/1000; um valor inventado pode não bater com o monitor. O Axion não muda.", hz),
		item("cs2.keep.mode", "Modo de tela e monitor", "Preferência sua; o Axion não muda.", mode),
		item("cs2.keep.fsr", "FidelityFX Super Resolution (FSR)", "É um upscaler: desligar pode BAIXAR o FPS de quem depende dele. O Axion não muda.", fsr, srcFSRGPUOpen),
		item("cs2.keep.contrast", "Boost Player Contrast", "Fica no cs2_machine_convars.vcfg (r_player_visibility_mode); o Axion não grava esse arquivo.", contrast),
		item("cs2.keep.fpsmax", "Limite de FPS (fps_max)", "Fica no cs2_machine_convars.vcfg ou no console; veja o item Limite de FPS. O Axion não grava esse arquivo.", fps),
	}
}

// StartCS2Preset applies the checked options as one batch (label "Preset
// competitivo do CS2"): refuses with the game open, copies the file first.
func (a *App) StartCS2Preset(ids []string) (Job, error) {
	f := a.Facts()
	if f == nil {
		return Job{}, ErrNoScan
	}
	if len(ids) == 0 {
		return Job{}, fmt.Errorf("%w: nenhuma opção marcada", errBadRequest)
	}
	for _, id := range ids {
		if !strings.HasPrefix(id, catalog.CS2PresetPrefix) || a.Engine.Resolve(id, f) == nil {
			return Job{}, fmt.Errorf("%w: item fora do preset do CS2: %s", errBadRequest, id)
		}
	}
	if len(a.P.Native.PIDsByName("cs2.exe")) > 0 {
		return Job{}, fmt.Errorf("%w: Feche o Counter-Strike 2 antes de aplicar: com o jogo aberto a mudança só vale ao reabrir e o jogo pode regravar o cs2_video.txt", errBadRequest)
	}
	path := a.cs2VideoPath()
	if path == "" {
		return Job{}, fmt.Errorf("%w: cs2_video.txt não encontrado; abra o CS2 uma vez e analise o PC de novo", errBadRequest)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Job{}, fmt.Errorf("%w: não foi possível ler o cs2_video.txt: %v", errBadRequest, err)
	}
	for _, id := range ids {
		if ok, why := catalog.CS2ItemWanted(id, b); !ok {
			return Job{}, fmt.Errorf("%w: %s: %s", errBadRequest, id, why)
		}
	}
	if _, running := a.Jobs.Running(); running {
		return Job{}, ErrBusy
	}
	if _, err := a.backupCS2Video(path); err != nil {
		return Job{}, fmt.Errorf("cópia de segurança do cs2_video.txt: %w", err)
	}
	return a.startApply(ApplyRequest{IDs: ids, Label: catalog.CS2PresetLabel})
}

// backupCS2Video copies the file to <DataDir>\cs2\cs2_video-AAAAMMDD-HHMMSS.txt
// and keeps the 10 newest. A manual safety net only; the undo is the journal.
func (a *App) backupCS2Video(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(a.P.DataDir, "cs2")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, "cs2_video-"+time.Now().Format("20060102-150405")+".txt")
	for n := 2; ; n++ { // two copies in the same second
		if _, err := os.Stat(dst); err != nil {
			break
		}
		dst = filepath.Join(dir, fmt.Sprintf("cs2_video-%s-%d.txt", time.Now().Format("20060102-150405"), n))
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return "", err
	}
	ents, _ := os.ReadDir(dir)
	var names []string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "cs2_video-") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > cs2BackupKeep {
		_ = os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
	return dst, nil
}

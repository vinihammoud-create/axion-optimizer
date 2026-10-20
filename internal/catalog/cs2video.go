package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"axion/internal/facts"
	"axion/internal/games"
	"axion/internal/tweaks"
)

// Preset competitivo do CS2 (0.9): writes the values of a few options of the
// game's own video menu in cs2_video.txt, with the game CLOSED. One tweak per
// option (generated at run time, "cs2video:<item>"), one operation per key.
//
// The snapshot is per key, not of the whole file: undo gives back only what
// Axion changed (a resolution the player chose later stays), conflict and
// drift are found per option, and a game update that rewrites the file in a
// new format is never overwritten with an old copy. The write replaces only
// the bytes between the quotes of the value (games.SetCS2VideoValue), goes to
// a temporary file in the same folder and is put in place with one rename.
//
// Never written here: autoexec.cfg, binds (cs2_user_keys*.vcfg), the
// account's convars (cs2_user_convars*.vcfg, which go to Steam Cloud),
// protected convars, launch options, resolution, aspect ratio, refresh rate,
// screen mode, monitor, FSR and fps_max (see CS2NeverTouch).

// CS2VideoPath returns the cs2_video.txt of the CS2 page (set by the app;
// "" = unknown).
var CS2VideoPath = func() string { return "" }

// ProcessRunning answers from a snapshot of the process list (no handle to
// the process is opened).
var ProcessRunning = func(exe string) bool { return false }

const (
	CS2PresetPrefix = "cs2video:"
	CS2PresetLabel  = "Preset competitivo do CS2"
)

var (
	srcIntelCMAA2   = tweaks.Source{Title: "Intel — Conservative Morphological Anti-Aliasing 2.0: custo do CMAA2 (0,10 a 0,73 ms por quadro em 1080p, conforme o equipamento (de uma placa GeForce, Vega e dois NUCs da Intel)) e do MSAA, cena Bistro, fora do CS2; a Intel chama os números de estimativas de engenharia", URL: "https://www.intel.com/content/www/us/en/developer/articles/technical/conservative-morphological-anti-aliasing-20.html"}
	srcNVReflexCS2  = tweaks.Source{Title: "NVIDIA — Counter-Strike 2 com NVIDIA Reflex (\"até 35%\" menos latência do sistema; \"Enabled + Boost\" gasta mais energia e baixa um pouco o FPS)", URL: "https://www.nvidia.com/en-us/geforce/news/gfecnt/20239/counter-strike-2-released-featuring-nvidia-reflex/"}
	srcPMIntelCat   = tweaks.Source{Title: "Intel — PresentMon: métrica GPU Busy para ver o equilíbrio CPU × GPU (como medir; não mede o ganho desta opção)", URL: "https://game.intel.com/us/stories/intel-presentmon/"}
	srcCS2SideStep  = tweaks.Source{Title: "Valve — Side-stepping Skill (19/8/2024): \"We are no longer going to allow automation (via scripting or hardware)\"; binds com mais de uma ação de movimento ou ataque deixam de funcionar", URL: "https://store.steampowered.com/news/app/730/view/6909171012611543335"}
	srcCS2Notes0819 = tweaks.Source{Title: "Valve — Notas do CS2 de 19/8/2024: o contra-strafe assistido por hardware \"will now be detected\"; binds com mais de um desses comandos são ignorados e só voltam com a convar protegida cl_allow_multi_input_binds", URL: "https://store.steampowered.com/news/app/730/view/6909171012611544764"}
)

var errCS2Open = errors.New("Feche o Counter-Strike 2 antes: com o jogo aberto a mudança só valeria ao reabrir, e o jogo pode regravar o cs2_video.txt com o que tem na memória (nada foi alterado)")

var reDigits = regexp.MustCompile(`^[0-9]{1,3}$`)

const cs2CommonDetail = " O Axion só grava com o Counter-Strike 2 fechado, troca apenas o valor entre aspas desta opção (formato, ordem e o resto do arquivo ficam iguais), grava numa cópia na mesma pasta e a coloca no lugar de uma vez, e guarda o valor anterior no histórico para desfazer. É o mesmo arquivo e o mesmo valor que o menu Vídeo do jogo grava; nada é injetado no jogo. Vale ao abrir o jogo."

// CS2NeverTouch lists the options of cs2_video.txt that stay the player's
// choice (or belong to the hardware): never written by the Axion. A test
// keeps the preset away from them.
var CS2NeverTouch = []string{"setting.defaultres", "setting.defaultresheight", "setting.aspectratiomode", "setting.refreshrate_numerator", "setting.refreshrate_denominator", "setting.fullscreen", "setting.nowindowborder", "setting.coop_fullscreen", "setting.monitor_index", "setting.high_dpi", "setting.videocfg_fsr_detail", "setting.videocfg_texture_detail", "setting.videocfg_hdr_detail", "version", "vendorid", "deviceid", "autoconfig", "setting.knowndevice", "setting.cpu_level", "setting.gpu_level", "setting.gpu_mem_level"}

// CS2Set is one key of cs2_video.txt a preset item wants: Want is what the
// Axion writes; OK lists the values (besides Want) that already count as done.
type CS2Set struct {
	Key, Want string
	OK        []string
}

// CS2Item is one option of the preset. Kind: "medido" (cost published
// outside CS2), "latencia" (less delay, not FPS) or "hipotese" (no public
// measurement: a hypothesis to measure, the player's preference).
type CS2Item struct {
	ID, Title, Summary, Detail, Kind string
	Sets                             []CS2Set
	Evidence                         tweaks.Evidence
	Sources                          []tweaks.Source
	NVIDIAOnly                       bool
	// Need, when set, is a filter for the page and for the route (NOT for
	// Applicable: a filter that reads what the item itself writes would make
	// the engine call an applied item "on" and hide a later drift).
	Need func(raw map[string]string) (bool, string)
}

var cs2Items = []CS2Item{
	{
		ID: CS2PresetPrefix + "aa", Kind: "medido", Title: "Anti-aliasing: CMAA2 no lugar do MSAA",
		Summary: "Troca o MSAA pelo CMAA2, um anti-aliasing de pós-processamento bem mais barato para a placa de vídeo; as bordas finas ficam um pouco menos lisas.",
		Detail: "setting.msaa_samples = 0 e setting.r_csgo_cmaa_enable = 1. Critério do Axion: é assim que o menu Vídeo → Avançado → Modo de anti-aliasing grava o CMAA2 (dedução, a Valve não documenta; confira no menu depois de aplicar). " +
			"Num artigo da Intel (cena Bistro, fora do CS2; a Intel chama os números de estimativas de engenharia), o CMAA2 custou de 0,10 a 0,73 ms por quadro em 1080p, conforme o equipamento (de uma placa GeForce, Vega e dois NUCs da Intel). " +
			"Em outra tabela do mesmo artigo, sem a cena nomeada, o MSAA 2x custou 1,12 e 1,23 ms em 4K nas GTX 1080 e Vega 64, contra 0,38 e 0,24 ms do CMAA2 em 4K. " +
			"Quanto isso vira de FPS no CS2 depende da placa e da resolução, e só ajuda quando a placa de vídeo é quem limita: meça 60 s antes e depois na página Desempenho.",
		Sets:     []CS2Set{{"setting.msaa_samples", "0", []string{"1"}}, {"setting.r_csgo_cmaa_enable", "1", nil}},
		Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcIntelCMAA2},
		Need: func(raw map[string]string) (bool, string) {
			m, _ := strconv.Atoi(strings.TrimSpace(raw["setting.msaa_samples"]))
			if m > 1 || strings.TrimSpace(raw["setting.r_csgo_cmaa_enable"]) == "1" {
				return true, ""
			}
			return false, "sem anti-aliasing escolhido: o preset não liga um"
		},
	},
	{
		ID: CS2PresetPrefix + "reflex", Kind: "latencia", Title: "NVIDIA Reflex: Ativado",
		Summary: "Liga o NVIDIA Reflex no modo Ativado, que a NVIDIA diz reduzir a latência do sistema no CS2; não é ganho de FPS.",
		Detail: "setting.r_low_latency = 1 (Vídeo → Avançado → NVIDIA Reflex Low Latency → Ativado). A NVIDIA informa até 35% menos latência do sistema no CS2 em placas compatíveis (número do fabricante, não medido pelo Axion); não é ganho de FPS. " +
			"Se você já usa \"Ativado + Boost\" (2), o Axion não mexe: segundo a NVIDIA o Boost gasta mais energia e baixa um pouco o FPS em troca de menos atraso. A escolha é sua.",
		Sets:     []CS2Set{{"setting.r_low_latency", "1", []string{"2"}}},
		Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcNVReflexCS2}, NVIDIAOnly: true,
	},
	cs2Hypothesis("shadows", "Sombras dinâmicas só do sol", "Calcula sombras só do sol em vez de todas as luzes do mapa; as sombras dos jogadores continuam. Ganho sem medição pública: hipótese a medir.",
		"setting.videocfg_dynamic_shadows = 0 (Vídeo → Avançado → Sombras dinâmicas)", CS2Set{"setting.videocfg_dynamic_shadows", "0", nil}),
	cs2Hypothesis("shadowquality", "Qualidade da sombra: média", "Baixa a resolução das sombras até a média, nunca abaixo: a sombra de um jogador é informação. Ganho sem medição pública: hipótese a medir.",
		"setting.videocfg_shadow_quality = 1 (Vídeo → Avançado → Qualidade da sombra → Média); valores 0 e 1 já contam como feitos", CS2Set{"setting.videocfg_shadow_quality", "1", []string{"0"}}),
	cs2Hypothesis("ao", "Oclusão de ambiente desligada", "Desliga um efeito só estético, que custa por pixel. Ganho sem medição pública: hipótese a medir.",
		"setting.videocfg_ao_detail = 0 (Vídeo → Avançado → Oclusão ambiental → Desligada)", CS2Set{"setting.videocfg_ao_detail", "0", nil}),
	cs2Hypothesis("particles", "Detalhe das partículas: baixo", "Reduz as partículas de fumaça, fogo e explosões. Ganho sem medição pública: hipótese a medir.",
		"setting.videocfg_particle_detail = 0 (Vídeo → Avançado → Detalhe de partículas → Baixo)", CS2Set{"setting.videocfg_particle_detail", "0", nil}),
	cs2Hypothesis("shaders", "Detalhe de shader: baixo", "Usa efeitos de superfície mais simples. Ganho sem medição pública: hipótese a medir.",
		"setting.shaderquality = 0 (Vídeo → Avançado → Detalhe de shader → Baixo)", CS2Set{"setting.shaderquality", "0", nil}),
	cs2Hypothesis("texfilter", "Filtragem de textura: anisotrópica 4x", "Baixa a filtragem de textura até a anisotrópica 4x, que já mantém o chão nítido ao longe. Ganho sem medição pública: hipótese a medir.",
		"setting.r_texturefilteringquality = 3 (Vídeo → Avançado → Modo de filtragem de textura → Anisotrópica 4x); valores de 0 a 3 já contam como feitos", CS2Set{"setting.r_texturefilteringquality", "3", []string{"0", "1", "2"}}),
}

// cs2Hypothesis: no public measurement of the cost of these options in CS2
// was found (the guides opened have no measurement or are model estimates).
// So they carry the source of the METHOD (how to see whether the GPU is the
// limit), low evidence, "hipótese a medir", and are only pre-marked on the
// page when the player's last measurement showed the GPU limiting.
func cs2Hypothesis(id, title, summary, what string, set CS2Set) CS2Item {
	return CS2Item{
		ID: CS2PresetPrefix + id, Kind: "hipotese", Title: title, Summary: summary,
		Detail: what + ". Não encontramos medição pública aberta do custo desta opção no CS2: é uma hipótese a medir e uma preferência sua, não um ganho conhecido. " +
			"A ordem das opções é a do custo típico de renderização. A página CS2 só deixa a opção marcada quando a sua última medição mostrou a placa de vídeo limitando abaixo da taxa do monitor; confirme com 60 s gravados antes e depois na página Desempenho.",
		Sets: []CS2Set{set}, Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcPMIntelCat},
	}
}

// cs2Writable: the keys (lower case) the preset may write.
var cs2Writable = map[string]bool{}

func init() {
	for _, it := range cs2Items {
		for _, s := range it.Sets {
			cs2Writable[strings.ToLower(s.Key)] = true
		}
	}
}

// CS2PresetItems returns the options of the preset, in display order.
func CS2PresetItems() []CS2Item { return append([]CS2Item(nil), cs2Items...) }

func cs2ItemByID(id string) (CS2Item, bool) {
	for _, it := range cs2Items {
		if it.ID == id {
			return it, true
		}
	}
	return CS2Item{}, false
}

// CS2ItemWanted tells whether the preset item makes sense for the values the
// file has now (the Need filter of the item, used by the page and the route).
func CS2ItemWanted(id string, file []byte) (bool, string) {
	it, ok := cs2ItemByID(id)
	if !ok {
		return false, "item fora do preset do CS2"
	}
	if it.Need == nil {
		return true, ""
	}
	raw := map[string]string{}
	for _, s := range it.Sets {
		if v, found, err := games.CS2VideoValue(file, s.Key); err == nil && found {
			raw[s.Key] = v
		}
	}
	return it.Need(raw)
}

// cs2VideoSnap is the state of one key before a change.
type cs2VideoSnap struct {
	File    bool   `json:"file"`
	Present bool   `json:"present"`
	Value   string `json:"value"`
}

// CS2VideoProvider builds the tweak "cs2video:<item>".
func CS2VideoProvider(id string, f *facts.Facts) *tweaks.Tweak {
	it, ok := cs2ItemByID(id)
	if !ok {
		return nil
	}
	path := CS2VideoPath()
	return &tweaks.Tweak{
		ID: it.ID, Category: "jogos", Title: it.Title + " (CS2)", Summary: it.Summary, Detail: it.Detail + cs2CommonDetail,
		Evidence: it.Evidence, Sources: it.Sources, Risk: tweaks.RiskSafe,
		RiskNote: "muda só esta opção no cs2_video.txt, como o menu de vídeo do jogo; o valor anterior fica no histórico",
		Reboot:   tweaks.RebootRestartApp, Tags: []string{"cs2", it.Kind}, OneClick: false,
		Recommend:  neutral("escolha sua na página CS2; o ganho real só a sua medição mostra"),
		Applicable: func(f *facts.Facts) (bool, string) { return cs2ItemApplicable(it, path, f) },
		Ops: func(*facts.Facts) []tweaks.Op {
			if path == "" {
				return nil
			}
			ops := make([]tweaks.Op, 0, len(it.Sets))
			for _, s := range it.Sets {
				ops = append(ops, cs2VideoOp(path, s.Key, s.Want, s.OK))
			}
			return ops
		},
	}
}

// cs2ItemApplicable looks only at the file, the game, the read-only
// attribute, the keys and the GPU: never at values the item writes.
func cs2ItemApplicable(it CS2Item, path string, f *facts.Facts) (bool, string) {
	if path == "" {
		return false, "cs2_video.txt não encontrado: abra o CS2 uma vez e analise o PC de novo"
	}
	if ProcessRunning("cs2.exe") {
		return false, "o Counter-Strike 2 está aberto: feche o jogo para aplicar"
	}
	if it.NVIDIAOnly {
		if g := f.PrimaryGPU(); g == nil || g.Vendor != "NVIDIA" {
			return false, "o NVIDIA Reflex só existe em placas NVIDIA"
		}
	}
	st, err := os.Stat(path)
	if err != nil {
		return false, "cs2_video.txt não encontrado em " + path
	}
	if st.Mode().Perm()&0o200 == 0 {
		return false, "o cs2_video.txt está marcado como somente leitura (alguém travou as configurações do jogo); desmarque em Propriedades do arquivo se quiser aplicar"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err.Error()
	}
	for _, s := range it.Sets {
		_, found, err := games.CS2VideoValue(b, s.Key)
		if err != nil {
			return false, err.Error()
		}
		if !found {
			return false, "a opção " + s.Key + " não está no cs2_video.txt desta versão do jogo"
		}
	}
	return true, ""
}

func cs2VideoOp(path, key, want string, ok []string) tweaks.Op {
	bad := cs2VideoSpecErr(path, key, want, ok)
	read := func() (cs2VideoSnap, error) {
		b, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			return cs2VideoSnap{}, nil
		}
		if err != nil {
			return cs2VideoSnap{}, err
		}
		v, found, err := games.CS2VideoValue(b, key)
		if err != nil {
			return cs2VideoSnap{}, err
		}
		return cs2VideoSnap{File: true, Present: found, Value: v}, nil
	}
	in := func(v string) bool {
		v = strings.TrimSpace(v)
		if v == want {
			return true
		}
		for _, o := range ok {
			if v == o {
				return true
			}
		}
		return false
	}
	write := func(c *tweaks.Ctx, value string) error {
		if bad != nil {
			return bad
		}
		if len(c.P.Native.PIDsByName("cs2.exe")) > 0 {
			return errCS2Open
		}
		return writeCS2Video(c, path, key, value)
	}
	return &tweaks.FuncOp{
		KindName: "cs2video", ID: strings.ToLower(path) + "|" + strings.ToLower(key),
		Spec: cs2VideoSpec{Path: path, Key: key, Want: want, OK: ok},
		Desc: fmt.Sprintf(`%s → "%s" = "%s" (só o valor entre aspas muda; o resto do arquivo fica igual)`, path, key, want),
		SnapFn: func(c *tweaks.Ctx) (any, error) {
			if bad != nil {
				return nil, bad
			}
			return read()
		},
		InFn: func(c *tweaks.Ctx) (bool, error) {
			s, err := read()
			switch {
			case err != nil:
				return false, err
			case !s.File:
				return false, errors.New("cs2_video.txt não encontrado")
			case !s.Present:
				return false, fmt.Errorf("a opção %s não está no cs2_video.txt", key)
			}
			return in(s.Value), nil
		},
		ApplyFn: func(c *tweaks.Ctx) error { return write(c, want) },
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var s cs2VideoSnap
			if err := json.Unmarshal(raw, &s); err != nil {
				return err
			}
			if !s.File || !s.Present {
				return nil // Axion never inserts a key
			}
			if bad != nil {
				return bad
			}
			now, err := read()
			if err != nil {
				return err
			}
			if !now.File || !now.Present {
				if c.Log != nil {
					c.Log("%s: o arquivo ou a opção não existe mais; nada a restaurar", key)
				}
				return nil
			}
			if now.Value == s.Value {
				return nil // already the original (even with the game open)
			}
			return write(c, s.Value)
		},
		EqFn: func(a, b json.RawMessage) bool {
			var sa, sb cs2VideoSnap
			if json.Unmarshal(a, &sa) != nil || json.Unmarshal(b, &sb) != nil {
				return false
			}
			if !sa.File || !sb.File || !sa.Present || !sb.Present {
				return true // gone: nothing to compare or restore
			}
			return sa.Value == sb.Value
		},
		CurFn: func(c *tweaks.Ctx) string {
			s, err := read()
			switch {
			case err != nil:
				return "erro: " + err.Error()
			case !s.File:
				return "(cs2_video.txt não encontrado)"
			case !s.Present:
				return "(opção ausente no arquivo)"
			}
			return fmt.Sprintf(`"%s" = "%s"`, key, s.Value)
		},
	}
}

// writeCS2Video: temporary file in the same folder (inherits the cfg folder
// ACL, same volume), then one rename. Read-only files and links are refused,
// never unlocked or replaced.
func writeCS2Video(c *tweaks.Ctx, path, key, value string) error {
	li, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if err := cs2ModeRefusal(li.Mode()); err != nil {
		return err
	}
	cs2RemoveOldTemps(filepath.Dir(path))
	old, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	nb, err := games.SetCS2VideoValue(old, key, value)
	if err != nil {
		return err
	}
	if bytes.Equal(nb, old) {
		return nil
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "cs2_video.txt.axion-*.tmp")
	if err != nil {
		return err
	}
	name, done := tmp.Name(), false
	defer func() {
		if !done {
			_ = os.Remove(name)
		}
	}()
	if _, err := tmp.Write(nb); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if cur, err := os.ReadFile(path); err != nil || !bytes.Equal(cur, old) {
		return errors.New("o cs2_video.txt mudou durante a gravação; nada foi alterado, tente de novo")
	}
	if len(c.P.Native.PIDsByName("cs2.exe")) > 0 {
		return errCS2Open
	}
	// An antivirus or an indexer may hold the file for a moment (Windows
	// refuses to replace a file that is open without delete sharing): try a
	// few times before giving up. The apply rolls back on the error.
	var rerr error
	for i := 0; i < 8; i++ {
		if rerr = os.Rename(name, path); rerr == nil {
			break
		}
		time.Sleep(60 * time.Millisecond)
	}
	if rerr != nil {
		return fmt.Errorf("não foi possível substituir o cs2_video.txt: %w", rerr)
	}
	done = true
	return nil
}

// cs2RemoveOldTemps removes temporaries a crashed write left more than one
// hour ago (a fresh one may belong to a write that is still running).
func cs2RemoveOldTemps(dir string) {
	matches, _ := filepath.Glob(filepath.Join(dir, "cs2_video.txt.axion-*.tmp"))
	for _, m := range matches {
		if st, err := os.Stat(m); err == nil && st.Mode().IsRegular() && time.Since(st.ModTime()) > time.Hour {
			_ = os.Remove(m)
		}
	}
}

// cs2VideoSpecErr guards a Spec rebuilt from the journal: only this file name
// in a Steam userdata cfg folder, only the preset's keys, only menu-like
// numbers.
func cs2VideoSpecErr(path, key, want string, ok []string) error {
	p := strings.ToLower(strings.ReplaceAll(path, "/", `\`))
	if filepath.Clean(path) != path || !strings.HasSuffix(p, `\730\local\cfg\cs2_video.txt`) {
		return fmt.Errorf("caminho fora do cs2_video.txt da Steam: %s", path)
	}
	if !cs2Writable[strings.ToLower(key)] {
		return fmt.Errorf("a opção %s não faz parte do preset do Axion", key)
	}
	for _, v := range append([]string{want}, ok...) {
		if !reDigits.MatchString(v) {
			return fmt.Errorf("valor inválido %q", v)
		}
	}
	return nil
}

// cs2ModeRefusal: a link is never replaced and a read-only file is never
// unlocked (some players lock the game's settings on purpose).
func cs2ModeRefusal(m fs.FileMode) error {
	if m&fs.ModeSymlink != 0 {
		return errors.New("o cs2_video.txt é um atalho/link; o Axion não substitui links (nada foi alterado)")
	}
	if m.Perm()&0o200 == 0 {
		return errors.New("o cs2_video.txt está marcado como somente leitura; nada foi alterado")
	}
	return nil
}

package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/games"
	"axion/internal/profile"
	"axion/internal/tweaks"
)

// The CS2 page guided by the measurement (0.7): with the game limited by
// the GPU and the 1% low under the monitor's rate, the lever is the video
// options, not the Windows registry; with the CPU limiting, the video
// options change nothing. The Steam client's download settings and the
// Valve requirements join the checklist.

// videoOption describes one option of cs2_video.txt: its key, the labels
// of its values (index = value) and the lowest value worth suggesting
// (shadows stay at medium: a player's shadow around a corner is
// information).
type videoOption struct {
	Key, Title, Where string
	Labels            []string
	Floor             int
	Why               string
}

var videoOptions = []videoOption{
	{Key: "setting.msaa_samples", Title: "Anti-aliasing (MSAA)", Where: "Vídeo → Avançado → Modo de anti-aliasing", Labels: []string{"nenhum", "nenhum", "2x MSAA", "", "4x MSAA", "", "", "", "8x MSAA"}, Floor: 0,
		Why: "o MSAA multiplica o trabalho da GPU por borda desenhada; o CMAA2 custa quase nada e alisa boa parte das bordas"},
	{Key: "setting.videocfg_dynamic_shadows", Title: "Sombras dinâmicas", Where: "Vídeo → Avançado → Sombras dinâmicas", Labels: []string{"só o sol", "todas"}, Floor: 0,
		Why: "\"todas\" calcula sombras de cada luz do mapa; \"só o sol\" mantém as sombras dos jogadores"},
	{Key: "setting.videocfg_shadow_quality", Title: "Qualidade das sombras", Where: "Vídeo → Avançado → Qualidade da sombra", Labels: []string{"baixa", "média", "alta", "muito alta"}, Floor: 1,
		Why: "resolução dos mapas de sombra; em média a sombra de um jogador continua visível"},
	{Key: "setting.videocfg_ao_detail", Title: "Oclusão de ambiente", Where: "Vídeo → Avançado → Oclusão ambiental", Labels: []string{"desligada", "média", "alta"}, Floor: 0,
		Why: "efeito só estético, com custo por pixel"},
	{Key: "setting.videocfg_particle_detail", Title: "Detalhe das partículas", Where: "Vídeo → Avançado → Detalhe de partículas", Labels: []string{"baixo", "médio", "alto", "muito alto"}, Floor: 0,
		Why: "fumaças, explosões e fogo custam mais com mais partículas; a fumaça volumétrica continua igual"},
	{Key: "setting.shaderquality", Title: "Qualidade dos shaders", Where: "Vídeo → Avançado → Detalhe de shader", Labels: []string{"baixa", "alta"}, Floor: 0,
		Why: "efeitos de superfície e reflexos"},
	{Key: "setting.r_texturefilteringquality", Title: "Filtragem de texturas", Where: "Vídeo → Avançado → Modo de filtragem de textura", Labels: []string{"bilinear", "trilinear", "anisotrópica 2x", "anisotrópica 4x", "anisotrópica 8x", "anisotrópica 16x"}, Floor: 3,
		Why: "custo pequeno em GPUs atuais; 4x já mantém o chão nítido ao longe"},
}

func (o videoOption) label(v int) string {
	if v >= 0 && v < len(o.Labels) && o.Labels[v] != "" {
		return o.Labels[v]
	}
	return strconv.Itoa(v)
}

// videoAdvice builds the video-options item from the newest CS2 capture.
func videoAdvice(f *facts.Facts, info games.CS2Info, m *diag.Measured) *CheckItem {
	if m == nil || m.Perf == nil {
		return &CheckItem{ID: "cs2.video.measured", Title: "Opções de vídeo pela medição", Status: "info", Evidence: "alta", Where: "Axion → Desempenho (FPS)",
			Advice:  "Grave 60 s do jogo na página Desempenho: o Axion lê se quem limita cada quadro é a placa de vídeo ou o processador e, pela sua taxa de monitor, diz se vale baixar alguma opção de vídeo e qual.",
			Sources: []tweaks.Source{srcPMIntel, srcGPUBusy}}
	}
	s := m.Perf
	hz, mon := gameDisplay(f)
	when := m.PerfAt.Local().Format("02/01 15:04")
	basis := fmt.Sprintf("medição de %s: %s FPS médio, 1%% low %s, GPU limitou %s%% dos quadros", when, fmtNum(s.AvgFPS), fmtNum(s.Low1FPS), fmtNum(s.GPUBoundPct))
	stale := ""
	if time.Since(m.PerfAt) > 45*24*time.Hour {
		stale = " A medição é antiga: repita antes de mexer."
	}
	it := &CheckItem{ID: "cs2.video.measured", Title: "Opções de vídeo pela medição", Evidence: "alta", Where: "CS2 → Configurações → Vídeo → Avançado", Basis: basis, Sources: []tweaks.Source{srcPMIntel, srcGPUBusy}}
	gpuBound := s.Limiter == "gpu" || s.GPUBoundPct >= 50
	switch {
	case s.Limiter == "limite" || s.CapLikely:
		it.Status = "ok"
		it.Advice = "O jogo estava no próprio limite de FPS (fps_max): as opções de vídeo não limitam nada. Para medir o teto do PC, suba ou tire o limite e grave de novo." + stale
		return it
	case s.Limiter == "cpu" || (s.CPUBoundPct >= 50 && !gpuBound):
		it.Status = "ok"
		it.Advice = "Quem limitou o FPS foi o lado da CPU, não a placa de vídeo: baixar opções de vídeo não vai subir o FPS. O que ajuda está no sistema (programas em segundo plano, perfil de memória, o jogo no núcleo certo) — veja o diagnóstico." + stale
		return it
	case !gpuBound:
		// Mixed or unclassified frames: no verdict on the video options.
		it.Status = "info"
		it.Advice = "A medição não apontou um limitador claro (ora a CPU, ora a GPU, ou quadros sem classificação): não há o que mudar nas opções de vídeo com segurança. Grave de novo num trecho de jogo normal, com 60 s, e compare." + stale
		return it
	case hz > 0 && s.Low1FPS >= float64(hz):
		it.Status = "ok"
		it.Advice = fmt.Sprintf("A placa de vídeo limitou a maior parte dos quadros, mas mesmo o 1%% low (%s) ficou acima dos %d Hz do %s: nada a baixar. Se quiser folga, as opções abaixo são as que mais pesam.", fmtNum(s.Low1FPS), hz, orDash(mon)) + stale
		return it
	}
	// GPU-bound and short of the monitor: what is above the floor, heaviest first.
	var sug []string
	raw := map[string]string{}
	if info.Video != nil {
		raw = info.Video.Raw
	}
	for _, o := range videoOptions {
		v, ok := rawInt(raw, o.Key)
		if !ok {
			continue
		}
		if o.Key == "setting.msaa_samples" && v <= 1 {
			continue
		}
		if v > o.Floor {
			to := o.label(o.Floor)
			if o.Key == "setting.msaa_samples" {
				to = "nenhum + CMAA2 (Modo de anti-aliasing: CMAA2)"
			}
			sug = append(sug, fmt.Sprintf("%s: %s → %s (%s)", o.Title, o.label(v), to, o.Why))
		}
	}
	if _, ok := rawInt(raw, "setting.videocfg_fsr_detail"); ok && len(sug) == 0 {
		sug = append(sug, "FidelityFX Super Resolution: só como último recurso; o upscaling tira nitidez dos inimigos ao longe")
	}
	it.Status = "action"
	head := fmt.Sprintf("A placa de vídeo limitou %s%% dos quadros (1%% low de %s; taxa do monitor não lida): aqui o ganho está nas opções de vídeo, não no Windows.", fmtNum(s.GPUBoundPct), fmtNum(s.Low1FPS))
	if hz > 0 {
		head = fmt.Sprintf("A placa de vídeo limitou %s%% dos quadros e o 1%% low (%s) ficou abaixo dos %d Hz do %s: aqui o ganho está nas opções de vídeo, não no Windows.", fmtNum(s.GPUBoundPct), fmtNum(s.Low1FPS), hz, orDash(mon))
	} else {
		it.Status = "info"
	}
	switch {
	case len(sug) == 0 && len(raw) == 0:
		it.Advice = head + " As opções de vídeo não foram lidas (arquivo cs2_video.txt não encontrado); baixe primeiro o MSAA, depois sombras dinâmicas para \"só o sol\" e a oclusão de ambiente, uma de cada vez, medindo 60 s no mesmo mapa depois de cada mudança."
	case len(sug) == 0:
		it.Advice = head + " As opções já estão no mínimo que vale a pena; o que resta é resolução/escala de renderização, ou mais placa de vídeo."
	default:
		it.Advice = head + " Hipótese a medir, uma mudança por vez e sempre com 60 s gravados no mesmo mapa e trajeto (página Desempenho → comparar): " + strings.Join(sug, "; ") + "."
	}
	it.Advice += " A ordem é por custo típico de renderização; o ganho real só a sua medição mostra." + stale
	return it
}

// gameDisplay is the monitor the game runs on: the primary one, at its
// fastest mode in any resolution (CS2 may run at another resolution than
// the desktop); the fastest monitor when none is marked primary.
func gameDisplay(f *facts.Facts) (maxHz int, name string) {
	for _, d := range f.Displays {
		if d.Primary {
			hz := d.MaxHzAny
			if d.MaxHzAtRes > hz {
				hz = d.MaxHzAtRes
			}
			if hz > 0 {
				return hz, d.Monitor
			}
		}
	}
	return fastestDisplay(f)
}

func rawInt(raw map[string]string, key string) (int, bool) {
	s, ok := raw[key]
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	return n, err == nil
}

func fmtNum(v float64) string { return fmt.Sprintf("%.0f", v) }

var (
	srcSteamDownloads = tweaks.Source{Title: "Steam Support — Managing Steam Downloads & Updates (downloads durante o jogo, janela de atualização automática)", URL: "https://help.steampowered.com/faqs/view/71AB-698D-57EB-178C"}
	srcSteamRates     = tweaks.Source{Title: "Steam Support — Setting Client Rates (limite de velocidade de download)", URL: "https://help.steampowered.com/faqs/view/163C-7C89-406E-2F63"}
)

// steamTips reads the Steam client's download settings against what the
// network test measured.
func steamTips(info games.CS2Info, m *diag.Measured) []CheckItem {
	sc := info.Steam
	where := "Steam → Configurações → Downloads"
	src := []tweaks.Source{srcSteamDownloads}
	bloat := ""
	if m != nil && m.Net != nil && m.Net.Grade != "" && m.Net.Grade != "A+" && m.Net.Grade != "A" {
		bloat = fmt.Sprintf(" O seu teste de rede deu nota %s de bufferbloat: com a conexão cheia o ping subiu %s ms.", m.Net.Grade, fmtNum(m.Net.AddedMs))
	}
	if sc == nil || !sc.Found {
		return []CheckItem{{ID: "tip.steam.downloads", Title: "Downloads da Steam durante o jogo", Status: "info", Evidence: "alta", Where: where,
			Advice: "Confira que \"Permitir downloads durante o jogo\" está desligado: um download no meio da partida enche a conexão e o ping sobe." + bloat + " (O arquivo de configuração da Steam não foi lido.)", Sources: src}}
	}
	var out []CheckItem
	dl := CheckItem{ID: "tip.steam.downloads", Title: "Downloads da Steam durante o jogo", Evidence: "alta", Where: where, Sources: src}
	switch sc.DownloadsDuringGameplay {
	case 1:
		dl.Status, dl.Current = "action", "permitidos durante o jogo"
		dl.Advice = "Desligue \"Permitir downloads durante o jogo\": uma atualização de outro jogo no meio da partida enche a conexão e o ping sobe (bufferbloat)." + bloat
	default:
		dl.Status, dl.Current = "ok", "pausados durante o jogo"
		dl.Advice = "A Steam pausa os downloads enquanto um jogo roda (padrão)."
	}
	out = append(out, dl)
	th := CheckItem{ID: "tip.steam.throttle", Title: "Limite de banda dos downloads da Steam", Evidence: "média", Where: where, Sources: []tweaks.Source{srcSteamRates}}
	switch {
	case sc.ThrottleKbps > 0:
		th.Status, th.Current = "ok", fmt.Sprintf("limite definido (DownloadThrottleKbps = %d)", sc.ThrottleKbps)
		th.Advice = "Com limite, um download fora do jogo deixa espaço para quem mais usa a conexão."
	case bloat != "":
		th.Status, th.Current = "info", "sem limite"
		th.Advice = "Sem limite, a Steam enche o download inteiro e o ping de quem joga ou faz chamada na casa sobe." + bloat + " Um limite em torno de 70% da banda medida mantém o download rápido e a fila do roteador curta; o QoS do roteador resolve para todos os programas."
	default:
		th.Status, th.Current = "ok", "sem limite"
		th.Advice = "Sem limite. Como o seu teste de rede não mostrou fila sob carga, não há o que mudar."
	}
	out = append(out, th)
	win := CheckItem{ID: "tip.steam.window", Title: "Janela de atualização automática da Steam", Evidence: "alta", Where: where, Sources: src}
	switch {
	case sc.AutoUpdateWindow != 0 && sc.WindowStart >= 0 && sc.WindowEnd >= 0:
		win.Status, win.Current = "ok", fmt.Sprintf("das %02d:00 às %02d:00", sc.WindowStart, sc.WindowEnd)
		win.Advice = "As atualizações automáticas só rodam nessa janela; deixe-a fora do seu horário de jogo."
	default:
		win.Status, win.Current = "info", "a qualquer hora"
		win.Advice = "Defina uma janela fora do seu horário de jogo (ex.: madrugada): as atualizações automáticas dos outros jogos não vão disputar a conexão quando você joga."
	}
	out = append(out, win)
	return out
}

// requirementsTip summarises the Valve requirements for the checklist.
func requirementsTip(f *facts.Facts) CheckItem {
	it := CheckItem{ID: "tip.requirements", Title: "Requisitos do CS2 (Valve)", Evidence: "alta", Where: "Axion → Avaliação completa → Capacidade", Status: "ok"}
	var bad, warn []string
	for _, x := range diag.Capacity(f, profile.Competitive) {
		switch x.Severity {
		case diag.SevCritical:
			bad = append(bad, x.Title)
		case diag.SevWarning:
			warn = append(warn, x.Title)
		}
	}
	switch {
	case len(bad) > 0:
		it.Status, it.Advice = "action", "Abaixo do mínimo da Valve: "+strings.Join(bad, "; ")+". O jogo pode abrir, mas sem suporte e com FPS instável."
	case len(warn) > 0:
		it.Status, it.Advice = "info", "Atende ao mínimo da Valve (4 threads, 8 GB, placa DirectX 11 com 1 GB), com ressalva: "+strings.Join(warn, "; ")+"."
	default:
		it.Advice = "Acima do mínimo da Valve (4 threads, 8 GB de RAM, placa DirectX 11 com 1 GB). Com isso o que decide o FPS é a configuração, as opções de vídeo e o monitor."
	}
	it.Sources = []tweaks.Source{{Title: "Valve — Counter-Strike 2 na Steam: requisitos de sistema", URL: "https://store.steampowered.com/app/730/CounterStrike_2/"}}
	return it
}

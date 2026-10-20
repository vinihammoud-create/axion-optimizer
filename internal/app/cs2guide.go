package app

import (
	"fmt"
	"math"
	"strings"
	"time"

	"axion/internal/facts"
	"axion/internal/games"
	"axion/internal/netdiag"
	"axion/internal/tweaks"
)

// CheckItem is a setting evaluated against the recommendation.
type CheckItem struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Status   string          `json:"status"` // ok | action | info
	Current  string          `json:"current,omitempty"`
	Advice   string          `json:"advice"`
	Where    string          `json:"where,omitempty"`
	Basis    string          `json:"basis,omitempty"` // what the recommendation rests on
	Evidence string          `json:"evidence"`
	Sources  []tweaks.Source `json:"sources,omitempty"`
	// Open is a Windows settings page (ms-settings:) the interface offers to
	// open when the setting cannot be read or changed from the Axion.
	Open string `json:"open,omitempty"`
}

// CS2Guide is the Counter-Strike 2 page model.
type CS2Guide struct {
	Info     games.CS2Info `json:"info"`
	Settings []CheckItem   `json:"settings"` // read from cs2_video.txt
	Tips     []CheckItem   `json:"tips"`     // evidence-based checklist
	Honest   string        `json:"honest"`
	// Preset is the competitive preset card (only when cs2_video.txt is read).
	Preset *CS2Preset `json:"preset,omitempty"`
}

var (
	srcReflexCS2  = tweaks.Source{Title: "NVIDIA — Counter-Strike 2 com NVIDIA Reflex", URL: "https://www.nvidia.com/en-us/geforce/news/gfecnt/20239/counter-strike-2-released-featuring-nvidia-reflex/"}
	srcAntiLag2   = tweaks.Source{Title: "AMD — Radeon Anti-Lag 2 (integrado ao CS2)", URL: "https://www.amd.com/en/products/software/adrenalin/radeon-software-anti-lag.html"}
	srcBBGsync    = tweaks.Source{Title: "Blur Busters — G-SYNC 101 (p. 14): configurações ótimas (V-Sync no painel, limite ≥ 3 FPS abaixo da taxa, Reflex limita sozinho)", URL: "https://blurbusters.com/gsync/gsync101-input-lag-tests-and-settings/14/"}
	srcCS2Buffer  = tweaks.Source{Title: "Valve — Notas do CS2 de 6/2 e 15/2/2024: buffer contra perda de pacotes (troca atraso por suavidade) e telemetria", URL: "https://store.steampowered.com/news/posts/?feed=steam_community_announcements&appids=730&enddate=1708400000"}
	srcRawInputMS = tweaks.Source{Title: "Microsoft Learn — Movimento de mouse de alta resolução: WM_INPUT (raw input) não recebe a aceleração do ponteiro", URL: "https://learn.microsoft.com/windows/win32/dxtecharts/taking-advantage-of-high-dpi-mouse-movement"}
	srcSDR        = tweaks.Source{Title: "Steamworks — Steam Datagram Relay (rede de relays da Valve)", URL: "https://partner.steamgames.com/doc/features/multiplayer/steamdatagramrelay"}
	srcDND        = tweaks.Source{Title: "Suporte da Microsoft — Notificações e Não perturbe no Windows (ligar automaticamente \"ao jogar um jogo\")", URL: "https://support.microsoft.com/windows/notifications-and-do-not-disturb-in-windows-feeca47f-0baf-5680-16f0-8801db1a8466"}
)

// primaryDisplay is the monitor the game runs on: the primary display
// (like the Atraso de entrada page), or the fastest one when none is marked.
func primaryDisplay(f *facts.Facts) (maxHz int, name string) {
	for _, d := range f.Displays {
		if d.Primary {
			return d.MaxHzAtRes, d.Monitor
		}
	}
	for _, d := range f.Displays {
		if d.MaxHzAtRes > maxHz {
			maxHz, name = d.MaxHzAtRes, d.Monitor
		}
	}
	return
}

// fastestDisplay is the highest refresh rate any connected monitor offers at
// any resolution, and that monitor: CS2 may run at another resolution than
// the desktop (a 4K monitor on HDMI 2.0 does 60 Hz at 4K and 144 Hz at
// 1080p) or on a monitor that is not the primary one.
func fastestDisplay(f *facts.Facts) (maxHz int, name string) {
	for _, d := range f.Displays {
		hz := d.MaxHzAny
		if d.MaxHzAtRes > hz {
			hz = d.MaxHzAtRes
		}
		if hz > maxHz {
			maxHz, name = hz, d.Monitor
		}
	}
	return
}

// CS2Guide evaluates the CS2 installation and settings.
func (a *App) CS2Guide() (CS2Guide, error) {
	f := a.Facts()
	if f == nil {
		return CS2Guide{}, ErrNoScan
	}
	_, info := a.Games()
	g := buildCS2Guide(f, info)
	a.addNetTips(&g, info)
	// 0.7: the video options read from the measurement, the Steam client's
	// download settings and the Valve requirements.
	m := a.measured()
	g.Settings = append(g.Settings, *videoAdvice(f, info, m))
	g.Tips = append(g.Tips, requirementsTip(f))
	g.Tips = append(g.Tips, steamTips(info, m)...)
	g.Preset = a.cs2Preset(f, info, m)
	return g, nil
}

// addNetTips turns the network checks of the CS2 page into verdicts from
// the last network test (Rede e ping page), when there is one.
func (a *App) addNetTips(g *CS2Guide, info games.CS2Info) {
	runs := netdiag.List(a.netDir())
	cur := info.ConvarInt("cl_net_buffer_ticks", -1) // -1: not read
	var last *netdiag.Summary
	if len(runs) > 0 {
		last = &runs[0]
	}
	for i := range g.Tips {
		t := &g.Tips[i]
		if t.ID != "tip.buffer" {
			continue
		}
		if cur >= 0 {
			t.Current = cs2BufferLabel(cur)
		}
		if last == nil || last.ValveMs < 0 {
			continue
		}
		want := 0
		if last.ValveLoss >= netdiag.NetLossPct || last.ValveJit >= netdiag.JitterMs {
			want = 1
		}
		if cur < 0 {
			// The setting was not read (CS2 not found, or another Steam
			// account): recommend, without a verdict on a value unknown.
			t.Advice = "Com a sua rota até a Valve, o recomendado é " + cs2BufferLabel(want) + "."
			continue
		}
		t.Basis = fmt.Sprintf("teste de rede de %s: perda de %s%% e jitter de %s ms até o relay de %s", last.At.Local().Format("02/01 15:04"),
			strings.Replace(fmt.Sprintf("%.1f", last.ValveLoss), ".", ",", 1), strings.Replace(fmt.Sprintf("%.1f", last.ValveJit), ".", ",", 1), last.ValvePOP)
		if cur == want {
			t.Status = "ok"
		} else {
			t.Status = "action"
			t.Advice = "Use " + cs2BufferLabel(want) + ": " + map[bool]string{true: "a sua rota até a Valve perde pacotes ou oscila, e o buffer troca ~15,6 ms de atraso por menos engasgos.", false: "a sua rota até a Valve está limpa; cada pacote de buffer só soma ~15,6 ms de atraso."}[want == 1]
		}
	}
	net := CheckItem{ID: "tip.network", Title: "Ping com a internet em uso (bufferbloat)", Evidence: "alta", Where: "Axion → Rede e ping",
		Advice: "Quando a Steam baixa algo ou alguém da casa vê vídeo, a fila do roteador enche e o ping do jogo sobe. O teste de rede mede isso e mostra como resolver (QoS/SQM do roteador).",
		Status: "info", Sources: []tweaks.Source{srcNetBufferbloat, srcNetSQM}}
	// The grade comes from the latest COMPLETE load test: an incomplete one
	// (a direction failed) has none.
	var graded *netdiag.Summary
	for i := range runs {
		if runs[i].Grade != "" {
			graded = &runs[i]
			break
		}
	}
	if graded != nil {
		net.Current = fmt.Sprintf("nota %s (+%s ms com a conexão cheia; teste de %s)", graded.Grade, strings.Replace(fmt.Sprintf("%.0f", math.Max(graded.AddedMs, 0)), ".", ",", 1), graded.At.Local().Format("02/01 15:04"))
		if graded.Grade == "A+" || graded.Grade == "A" {
			net.Status = "ok"
		} else {
			net.Status = "action"
			net.Advice = "O ping sobe com a internet em uso. Veja na página Rede e ping os valores de QoS sugeridos para o seu roteador e repita o teste depois de ativar."
		}
	}
	g.Tips = append(g.Tips, net)
}

func cs2BufferLabel(n int) string {
	switch {
	case n <= 0:
		return "\"None\" (nenhum)"
	case n == 1:
		return "\"1 packet\""
	}
	return fmt.Sprintf("\"%d packets\"", n)
}

func buildCS2Guide(f *facts.Facts, info games.CS2Info) CS2Guide {
	g := CS2Guide{Info: info,
		Honest: "O CS2 usa entrada de mouse raw e rede UDP (no matchmaking oficial, pela rede de relays da Valve): ajustes de \"ping\" no Windows (Nagle, TCP) e de aceleração do mouse não mudam o jogo. O que muda de verdade: frequência do monitor, Reflex/Anti-Lag, V-Sync, programas em segundo plano, a conexão (cabo, fila do roteador) e, quando a CPU limita, a memória."}
	nvidia, amd := f.HasGPUVendor("NVIDIA"), false
	if p := f.PrimaryGPU(); p != nil {
		nvidia, amd = p.Vendor == "NVIDIA", p.Vendor == "AMD"
	}
	maxHz, mon := primaryDisplay(f)
	anyHz, anyMon := fastestDisplay(f)

	if v := info.Video; v != nil {
		// Refresh rate chosen inside CS2.
		it := CheckItem{ID: "cs2.refresh", Title: "Taxa de atualização no CS2", Where: "CS2 → Configurações → Vídeo → Vídeo → Taxa de atualização", Evidence: "alta"}
		switch {
		case v.RefreshHz == 0:
			it.Status, it.Current = "info", "não definida no arquivo (o jogo usa a padrão)"
			it.Advice = "Selecione manualmente a maior taxa do seu monitor."
		case maxHz > 0 && v.RefreshHz+1 < float64(maxHz):
			it.Status, it.Current = "action", fmt.Sprintf("%.0f Hz", v.RefreshHz)
			it.Advice = fmt.Sprintf("Seu monitor (%s) suporta %d Hz. Com %.0f Hz o jogo mostra menos quadros do que o monitor pode exibir: escolha %d Hz.", mon, maxHz, v.RefreshHz, maxHz)
		case anyHz > 0 && v.RefreshHz > float64(anyHz)+1:
			// The first real report: 165 Hz saved in CS2 on a notebook whose
			// panel goes up to 120 Hz (the setting came from another monitor).
			// Compared with the fastest mode of any connected monitor, so a
			// game resolution faster than the desktop's is not flagged.
			it.Status, it.Current = "action", fmt.Sprintf("%.0f Hz", v.RefreshHz)
			it.Advice = fmt.Sprintf("O CS2 está configurado para %.0f Hz, acima do máximo de qualquer monitor conectado agora (%d Hz, %s). Isso costuma vir de outro monitor usado antes. Se você joga neste monitor, escolha %d Hz no jogo; se joga num monitor externo mais rápido, conecte-o e rode a análise de novo.", v.RefreshHz, anyHz, anyMon, anyHz)
		default:
			it.Status, it.Current = "ok", fmt.Sprintf("%.0f Hz", v.RefreshHz)
			it.Advice = "Na taxa máxima do monitor."
		}
		g.Settings = append(g.Settings, it)

		// Reflex.
		it = CheckItem{ID: "cs2.reflex", Title: "NVIDIA Reflex", Where: "CS2 → Configurações → Vídeo → Avançado → NVIDIA Reflex Low Latency", Evidence: "alta", Sources: []tweaks.Source{srcReflexCS2}}
		switch {
		case !nvidia && amd:
			it = CheckItem{ID: "cs2.antilag", Title: "AMD Anti-Lag 2", Status: "info", Where: "CS2 → Configurações → Vídeo → Avançado", Evidence: "alta",
				Advice: "O CS2 integra o Anti-Lag 2 da AMD, que reduz a latência quando a GPU é o gargalo. Ative no jogo (requer driver AMD recente).", Sources: []tweaks.Source{srcAntiLag2}}
		case !nvidia:
			it.Status, it.Advice = "info", "Disponível apenas em placas NVIDIA."
		case v.LowLatency == 0:
			it.Status, it.Current = "action", "desativado"
			it.Advice = "Ative (\"Ativado\" ou \"Ativado + Boost\"). O Reflex sincroniza CPU e GPU e elimina a fila de quadros quando a GPU é o gargalo, reduzindo a latência do clique até a tela."
		case v.LowLatency == 1:
			it.Status, it.Current, it.Advice = "ok", "ativado", "\"Ativado + Boost\" mantém o clock da GPU alto em cenas leves, com um pouco mais de consumo. Opcional."
		case v.LowLatency == 2:
			it.Status, it.Current, it.Advice = "ok", "ativado + boost", "Configuração de menor latência."
		default:
			it.Status, it.Advice = "info", "Não foi possível ler a opção."
		}
		g.Settings = append(g.Settings, it)

		// V-Sync.
		it = CheckItem{ID: "cs2.vsync", Title: "V-Sync dentro do jogo", Where: "CS2 → Configurações → Vídeo → Avançado → Esperar pela sincronização vertical", Evidence: "alta", Sources: []tweaks.Source{srcBBGsync}}
		switch v.VSync {
		case 0:
			it.Status, it.Current, it.Advice = "ok", "desligado", "Correto para competitivo. Se usa G-SYNC/FreeSync, o recomendado é V-Sync ligado no painel do driver + Reflex, mantendo o do jogo desligado."
		case -1:
			it.Status, it.Advice = "info", "Não foi possível ler a opção."
		default:
			it.Status, it.Current, it.Advice = "action", "ligado", "V-Sync do jogo adiciona fila de quadros e latência. Desligue no jogo; com G-SYNC/FreeSync use o V-Sync do painel do driver junto com o Reflex."
		}
		g.Settings = append(g.Settings, it)

		// Resolution / mode (informative).
		mode := "janela"
		if v.Fullscreen {
			mode = "tela cheia"
		} else if v.NoBorder {
			mode = "janela sem bordas"
		}
		g.Settings = append(g.Settings, CheckItem{ID: "cs2.mode", Title: "Resolução e modo de tela", Status: "info",
			Current:  fmt.Sprintf("%dx%d, %s", v.Width, v.Height, mode),
			Advice:   "Escolha pessoal. No Windows 11, tela cheia e janela sem bordas têm latência equivalente quando o jogo está em \"independent flip\"; na dúvida, use tela cheia.",
			Evidence: "média"})
		if !v.Modified.IsZero() && time.Since(v.Modified) > 180*24*time.Hour {
			g.Settings = append(g.Settings, CheckItem{ID: "cs2.stale", Title: "Arquivo de vídeo antigo", Status: "info", Current: v.Modified.Format("02/01/2006"),
				Advice: "As configurações acima podem estar desatualizadas (arquivo não é alterado há meses).", Evidence: "—"})
		}
	}

	// Refresh rate forced by a launch option.
	if hz, opt, ok := games.LaunchValue(info.LaunchOptions, "-refresh", "-freq"); ok && maxHz > 0 {
		it := CheckItem{ID: "cs2.launch.refresh", Title: "Taxa forçada nas opções de inicialização", Where: "Steam → Biblioteca → CS2 → Propriedades → Opções de inicialização", Evidence: "alta",
			Current: fmt.Sprintf("%s %d", opt, hz), Status: "ok", Advice: fmt.Sprintf("Igual à taxa máxima do monitor (%d Hz).", maxHz)}
		switch {
		case anyHz > 0 && hz > anyHz+1:
			it.Status = "action"
			it.Advice = fmt.Sprintf("Pede %d Hz, mais que o máximo de qualquer monitor conectado agora (%d Hz, %s). Se você joga neste monitor, troque por %s %d ou remova a opção (o jogo usa a taxa escolhida no menu de vídeo); se joga num monitor externo mais rápido, mantenha.", hz, anyHz, anyMon, opt, anyHz)
		case hz > maxHz+1:
			it.Advice = fmt.Sprintf("Acima dos %d Hz do monitor principal na resolução da área de trabalho, mas um monitor conectado chega a %d Hz (em outra resolução ou outro monitor).", maxHz, anyHz)
		case hz+1 < maxHz:
			it.Status = "action"
			it.Advice = fmt.Sprintf("Limita o jogo a %d Hz, mas o monitor (%s) chega a %d Hz: troque por %s %d ou remova a opção.", hz, mon, maxHz, opt, maxHz)
		}
		g.Settings = append(g.Settings, it)
	}

	// Checklist.
	g.Tips = []CheckItem{
		{ID: "tip.monitor", Title: "Monitor na frequência máxima no Windows", Evidence: "alta", Where: "Axion → Otimizações → Sistema e jogo",
			Advice: "O Windows frequentemente deixa monitores de 144/240 Hz em 60 Hz. O Axion detecta e corrige.", Status: statusIf(f, func(f *facts.Facts) bool {
				for _, d := range f.Displays {
					if d.MaxHzAtRes > d.CurrentHz && d.CurrentHz > 0 {
						return false
					}
				}
				return true
			})},
		{ID: "tip.fpscap", Title: "Limite de FPS", Evidence: "média", Where: "Console: fps_max", Current: fpsCapLabel(info),
			Advice: "Sem G-SYNC/FreeSync: Reflex ligado e fps_max 0 (sem limite). Com G-SYNC/FreeSync: V-Sync ligado no painel do driver + Reflex, que já limita o FPS abaixo da taxa; sem Reflex, use um limite pelo menos 3 FPS abaixo da taxa (ex.: 237 em 240 Hz). Limitadores do próprio jogo têm menos latência que os externos (RTSS).", Status: "info", Sources: []tweaks.Source{srcBBGsync}},
		{ID: "tip.buffer", Title: "Buffer contra perda de pacotes", Evidence: "alta", Where: "CS2 → Configurações → Jogo → \"Buffering to smooth over packet loss\"",
			Advice: "Deixe em \"None\" se a conexão é estável: cada pacote de buffer soma cerca de 15,6 ms (um tick). Use 1 pacote só se houver perda de pacotes ou oscilação (comum no Wi-Fi). A página Rede e ping mede a sua perda e o jitter até os relays da Valve para decidir.", Status: "info", Sources: []tweaks.Source{srcCS2Buffer}},
		{ID: "tip.telemetry", Title: "Telemetria de desempenho do jogo", Evidence: "alta", Where: "CS2 → Configurações → Jogo → Telemetria",
			Advice: "Mostra FPS, ping, perda e oscilação da rede na tela. Use para confirmar em jogo o que o teste de rede do Axion mediu e para comparar antes e depois de qualquer mudança.", Status: "info", Sources: []tweaks.Source{srcCS2Buffer}},
		{ID: "tip.cable", Title: "Cabo de rede em vez de Wi-Fi", Evidence: "alta",
			Advice: "O Wi-Fi adiciona variação de latência e perda de pacotes por interferência; é a mudança de rede mais efetiva.", Status: statusIf(f, func(f *facts.Facts) bool {
				a := f.ActiveAdapter()
				return a == nil || a.Kind != "Wi-Fi"
			})},
		{ID: "tip.mouse", Title: "Mouse: o Windows não afeta a mira", Evidence: "alta",
			Advice: "O CS2 lê o mouse em raw input: \"aprimorar precisão do ponteiro\" e a velocidade do ponteiro do Windows não mudam a sensibilidade no jogo. Ajuste DPI/polling no software do mouse e a sensibilidade no jogo.", Status: "info", Sources: []tweaks.Source{srcRawInputMS}},
		{ID: "tip.driverpanel", Title: "Painel NVIDIA/AMD", Evidence: "alta",
			Advice: "Com Reflex/Anti-Lag 2 ativos no jogo, as opções de baixa latência do painel do driver são substituídas; não precisam ser alteradas. Mantenha o driver atualizado.", Status: "info", Sources: []tweaks.Source{srcReflexCS2}},
		{ID: "tip.dnd", Title: "Não perturbe ao jogar", Evidence: "alta", Where: "Configurações → Sistema → Notificações → Ativar \"não perturbe\" automaticamente → \"Ao jogar um jogo\"", Open: "ms-settings:notifications",
			Advice: "Com a opção ligada, o Windows silencia as notificações enquanto um jogo em tela cheia roda: sem banner roubando o foco no meio da partida. A regra fica num formato interno do Windows que o Axion só consegue apontar, não ler; confira na página de Configurações.", Status: "info", Sources: []tweaks.Source{srcDND}},
	}
	return g
}

// fpsCapLabel shows the fps_max the account saved ("" when not read).
func fpsCapLabel(info games.CS2Info) string {
	v := info.ConvarInt("fps_max", -1)
	switch {
	case v < 0:
		return ""
	case v == 0:
		return "fps_max 0 (sem limite)"
	}
	return fmt.Sprintf("fps_max %d", v)
}

func statusIf(f *facts.Facts, ok func(*facts.Facts) bool) string {
	if ok(f) {
		return "ok"
	}
	return "action"
}

func dateOr(t time.Time) string {
	if t.IsZero() {
		return "data desconhecida"
	}
	return t.Format("02/01/2006")
}

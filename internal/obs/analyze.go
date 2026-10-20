package obs

import (
	"fmt"
	"strings"
)

// Levels.
const (
	LevelOK        = "ok"
	LevelAttention = "atencao"
	LevelImportant = "importante"
)

// Source is a citation.
type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

var (
	// Official OBS knowledge base, read: dropped frames mean the connection to
	// the server is unstable or cannot keep up with the bitrate; start at 75%
	// of the upload speed; lower the bitrate, network optimizations, dynamic
	// bitrate.
	SrcStreamKB = Source{Title: "Base de conhecimento do OBS — Stream Connection Troubleshooting (quadros perdidos na rede; comece com 75% do upload)", URL: "https://obsproject.com/kb/stream-connection-troubleshooting"}
	// Official OBS knowledge base, read: HAGS may cause performance issues and
	// failures with OBS and hardware encoders.
	SrcHAGS = Source{Title: "Base de conhecimento do OBS — HAGS (pode causar problemas com o OBS e os encoders por hardware)", URL: "https://obsproject.com/kb/hags"}
	// OBS' own source: what each of these log lines counts.
	SrcLogLines = Source{Title: "Código do OBS — libobs/obs-output.c: o que cada linha de quadros perdidos conta", URL: "https://github.com/obsproject/obs-studio/blob/master/libobs/obs-output.c"}
	// Community, read: a moderator explains rendering lag as an overloaded GPU
	// and encoding lag as an encoder that cannot keep up.
	SrcLagForum = Source{Title: "Fórum do OBS (comunidade) — quadros perdidos por renderização e quadros pulados por codificação", URL: "https://obsproject.com/forum/threads/frames-missed-due-to-rendering-lag-and-skipped-frames-due-to-encoding-lag.169520"}
)

// Finding is what a session says.
type Finding struct {
	ID        string   `json:"id"` // encoding | rendering | network | ok
	Level     string   `json:"level"`
	Title     string   `json:"title"`
	Text      string   `json:"text"`
	Page      string   `json:"page,omitempty"`
	PageLabel string   `json:"pageLabel,omitempty"`
	Sources   []Source `json:"sources,omitempty"`
}

// Analysis is the reading of one session.
type Analysis struct {
	Findings []Finding `json:"findings"`
	Level    string    `json:"level"` // the worst of the findings
}

// Thresholds, in percent of the frames. They are Axion's criterion, not
// OBS': OBS itself warns "Encoding overloaded" from 0.1% of skipped frames,
// which in a long stream is often a handful of frames nobody sees.
const (
	EncodingAttentionPct = 0.5
	EncodingImportantPct = 2.0
	RenderAttentionPct   = 1.0
	RenderImportantPct   = 5.0
	NetworkAttentionPct  = 0.5
	NetworkImportantPct  = 2.0
)

// BitrateShare is the share of the upload speed that OBS' own guidance
// suggests as a starting point for the bitrate.
const BitrateShare = 0.75

func level(v, attention, important float64) string {
	switch {
	case v >= important:
		return LevelImportant
	case v >= attention:
		return LevelAttention
	}
	return LevelOK
}

func pc(v float64) string { return strings.Replace(fmt.Sprintf("%.1f", v), ".", ",", 1) + "%" }

func isHardware(family string) bool { return family == "NVENC" || family == "AMF" || family == "QSV" }

func encoderName(c Context) string {
	switch c.Encoder {
	case "x264":
		return "o codificador x264 (processador)"
	case "NVENC":
		return "o NVENC (placa NVIDIA)"
	case "AMF":
		return "o AMF (placa AMD)"
	case "QSV":
		return "o Quick Sync (Intel)"
	}
	return "o codificador"
}

// Pick returns the session to read: the newest one with a stream, else the
// newest one. Sessions come newest first.
func Pick(ss []Session) *Session {
	for i := range ss {
		if ss[i].Stream() != nil {
			return &ss[i]
		}
	}
	if len(ss) > 0 {
		return &ss[0]
	}
	return nil
}

// Analyze reads a session. uploadMbps is the upload speed measured by the
// network test (0 = never measured).
func Analyze(s *Session, uploadMbps float64) Analysis {
	var a Analysis
	if s == nil {
		return a
	}
	st := s.Stream()
	kind := "transmissão"
	if st == nil {
		kind = "gravação"
		for i := range s.Stats {
			if s.Stats[i].Kind == KindRecording {
				st = &s.Stats[i]
			}
		}
		if st == nil && len(s.Stats) > 0 {
			st = &s.Stats[0]
		}
	}
	if st == nil {
		return a
	}
	add := func(f Finding) {
		a.Findings = append(a.Findings, f)
		if f.Level == LevelImportant || (f.Level == LevelAttention && a.Level != LevelImportant) {
			a.Level = f.Level
		}
	}
	a.Level = LevelOK
	c := s.Context

	// The encoder could not keep up (CPU for x264, the card's encoder otherwise).
	if st.SkippedPct >= 0 {
		if lv := level(st.SkippedPct, EncodingAttentionPct, EncodingImportantPct); lv != LevelOK {
			f := Finding{ID: "encoding", Level: lv, Sources: []Source{SrcLagForum, SrcLogLines}}
			f.Title = fmt.Sprintf("O codificador pulou %s dos quadros (%d de %d)", pc(st.SkippedPct), st.Skipped, st.SkippedOf)
			switch {
			case c.Encoder == "x264":
				f.Text = "O codificador por processador (x264) não deu conta de codificar a tempo, e com o jogo aberto o processador é disputado. O mais usado é trocar para o codificador da placa de vídeo (NVENC, AMF ou Quick Sync), que quase não pesa no processador; se continuar no x264, use um preset mais rápido ou baixe a resolução ou os quadros por segundo de saída."
			case isHardware(c.Encoder):
				f.Text = "O codificador da placa de vídeo não deu conta a tempo, o que costuma acontecer quando o jogo deixa a placa no limite. Limitar o FPS do jogo ou baixar os gráficos libera a placa; baixar a resolução ou os quadros por segundo de saída também reduz o trabalho. A base de conhecimento do OBS diz que o HAGS pode causar problemas com o OBS e com encoders por hardware: vale testar com ele desligado (Otimizações)."
				f.Sources = append(f.Sources, SrcHAGS)
				f.Page, f.PageLabel = "otimizacoes", "Ver o HAGS em Otimizações"
			default:
				f.Text = "O codificador não deu conta de codificar a tempo. Baixe a resolução ou os quadros por segundo de saída, ou use um codificador mais leve para o seu PC."
			}
			add(f)
		}
	}

	// The GPU was too busy for OBS to compose the frames.
	if st.LaggedPct >= 0 {
		if lv := level(st.LaggedPct, RenderAttentionPct, RenderImportantPct); lv != LevelOK {
			add(Finding{ID: "rendering", Level: lv, Sources: []Source{SrcLagForum, SrcHAGS},
				Title: fmt.Sprintf("A placa de vídeo não deu conta de compor %s dos quadros (%d)", pc(st.LaggedPct), st.Lagged),
				Text:  "O OBS compõe cada quadro na placa de vídeo, e quando ela está no limite por causa do jogo o quadro sai atrasado. O caminho que a comunidade do OBS mais recomenda é limitar o FPS do jogo (ou ligar o V-Sync) para sobrar placa para o OBS, e baixar os gráficos se mesmo assim ela passar de uns 90% de uso. Desligar o HAGS também aparece com frequência nas soluções. Meça o jogo na página Desempenho para ver se a placa é mesmo o limite.",
				Page:  "desempenho", PageLabel: "Medir o jogo"})
		}
	}

	// The connection could not carry the bitrate.
	if st.DropPct >= 0 {
		if lv := level(st.DropPct, NetworkAttentionPct, NetworkImportantPct); lv != LevelOK {
			f := Finding{ID: "network", Level: lv, Sources: []Source{SrcStreamKB}, Page: "rede", PageLabel: "Testar a rede"}
			f.Title = fmt.Sprintf("A rede perdeu %s dos quadros da transmissão (%d)", pc(st.DropPct), st.Dropped)
			text := "A página oficial do OBS diz que quadros perdidos na rede indicam uma conexão instável ou que não acompanha a taxa de bits escolhida."
			switch {
			case c.BitrateKbps > 0 && uploadMbps > 0 && float64(c.BitrateKbps)/1000 > BitrateShare*uploadMbps:
				text += fmt.Sprintf(" A taxa de bits da transmissão era de %s Mb/s e o upload medido pelo Axion é de %s Mb/s: passa de 75%% dele, o ponto de partida que o OBS sugere. Baixe a taxa de bits em Configurações > Saída.",
					strings.Replace(fmt.Sprintf("%.1f", float64(c.BitrateKbps)/1000), ".", ",", 1), strings.Replace(fmt.Sprintf("%.0f", uploadMbps), ".", ",", 1))
			case c.BitrateKbps > 0 && uploadMbps > 0:
				text += fmt.Sprintf(" A taxa de bits (%s Mb/s) cabe em 75%% do upload medido (%s Mb/s), então o mais provável é instabilidade da conexão: use cabo em vez de Wi-Fi, ligue o ajuste dinâmico da taxa de bits e as otimizações de rede do OBS (Configurações > Avançado > Rede) e meça o bufferbloat na página Rede.",
					strings.Replace(fmt.Sprintf("%.1f", float64(c.BitrateKbps)/1000), ".", ",", 1), strings.Replace(fmt.Sprintf("%.0f", uploadMbps), ".", ",", 1))
			case c.BitrateKbps > 0:
				text += fmt.Sprintf(" A taxa de bits era de %s Mb/s. Meça o upload na página Rede: o OBS sugere começar com 75%% dele.", strings.Replace(fmt.Sprintf("%.1f", float64(c.BitrateKbps)/1000), ".", ",", 1))
			default:
				text += " Meça o upload na página Rede e comece com 75% dele como taxa de bits."
			}
			f.Text = text
			add(f)
		}
	}

	if len(a.Findings) == 0 {
		var what []string
		if st.SkippedPct >= 0 {
			what = append(what, fmt.Sprintf("codificador pulou %s", pc(st.SkippedPct)))
		}
		if st.LaggedPct >= 0 {
			what = append(what, fmt.Sprintf("placa de vídeo atrasou %s", pc(st.LaggedPct)))
		}
		if st.DropPct >= 0 {
			what = append(what, fmt.Sprintf("rede perdeu %s", pc(st.DropPct)))
		}
		txt := "Nenhum dos três números passou do que o Axion considera relevante."
		if len(what) > 0 {
			txt = "Na última " + kind + ": " + strings.Join(what, ", ") + ". Nada relevante."
		}
		add(Finding{ID: "ok", Level: LevelOK, Title: "A última " + kind + " saiu sem perdas relevantes", Text: txt, Sources: []Source{SrcLogLines}})
	}
	// A very short session does not say much.
	if st.Frames >= 0 && st.Frames < 1800 && a.Level != LevelOK {
		a.Findings = append(a.Findings, Finding{ID: "short", Level: LevelOK, Title: "Foi uma " + kind + " curta", Text: "Com poucos quadros, uma pequena falha pesa muito na porcentagem. Veja se o padrão se repete numa " + kind + " mais longa antes de mexer em algo."})
	}
	return a
}

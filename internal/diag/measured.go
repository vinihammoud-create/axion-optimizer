package diag

import (
	"fmt"
	"strings"
	"time"

	"axion/internal/dpc"
	"axion/internal/facts"
	"axion/internal/netdiag"
	"axion/internal/obs"
	"axion/internal/perf"
	"axion/internal/tweaks"
)

// How old a measurement may be and still speak for the PC today.
const measuredMaxAge = 45 * 24 * time.Hour

// gpuBoundShare is the share of frames limited by the GPU from which the
// memory profile stops being the lever: the CPU (and so the memory) had
// time to spare in most frames.
const gpuBoundShare = 60.0

func fmtMs(v float64) string {
	return strings.Replace(fmt.Sprintf("%.0f", v), ".", ",", 1)
}

func fmtPct(v float64) string {
	return strings.Replace(fmt.Sprintf("%.1f", v), ".", ",", 1)
}

// applyMeasured adds what the measurements say and tempers the rules that
// assumed the worst without them.
func applyMeasured(fs []Finding, f *facts.Facts, m *Measured) []Finding {
	now := time.Now()
	if p := m.Perf; p != nil && (m.PerfAt.IsZero() || now.Sub(m.PerfAt) <= measuredMaxAge) && p.GPUBoundPct >= gpuBoundShare {
		game := m.PerfGame
		if game == "" {
			game = "jogo"
		}
		when := ""
		if !m.PerfAt.IsZero() {
			when = " de " + m.PerfAt.Local().Format("02/01")
		}
		for i := range fs {
			if fs[i].ID != "ram.xmp" || fs[i].Severity != SevWarning {
				continue
			}
			// The kit still runs below its rating, but on this PC the game
			// waited for the GPU in most frames: the profile is not what
			// holds the FPS back, so it stops costing points.
			fs[i].Severity = SevInfo
			fs[i].Explanation += fmt.Sprintf(" Na sua medição%s (%s), a placa de vídeo limitou %s%% dos quadros: com o jogo limitado pela GPU, o ganho do XMP tende a ser pequeno, e o que muda o FPS são as opções de vídeo. Por isso este item vale como sugestão, não como problema; ligue o perfil quando for cômodo e meça.",
				when, game, fmtPct(p.GPUBoundPct))
		}
	}
	if l := m.Link; l != nil && (l.At.IsZero() || now.Sub(l.At) <= measuredMaxAge) {
		fs = linkFinding(fs, f, l, m.LinkGame)
	}
	latest := m.NetLatest
	if latest == nil {
		latest = m.Net
	}
	if m.Net != nil || latest != nil {
		fs = append(fs, netFindings(m.Net, latest, now)...)
	}
	if x, ok := dpcFinding(m, now); ok {
		fs = append(fs, x)
	}
	fs = append(fs, obsFindings(m, now)...)
	return fs
}

// obsFindings turns what the newest OBS log says into findings: one for each
// of the three causes (the encoder, the GPU, the network) that went past the
// line. It stays quiet without a log, with an old one and when nothing did.
func obsFindings(m *Measured, now time.Time) []Finding {
	s, an := m.OBS, m.OBSAnalysis
	if s == nil || an == nil || now.Sub(s.At) > measuredMaxAge {
		return nil
	}
	var ctx []string
	if s.Context.Encoder != "" {
		ctx = append(ctx, s.Context.Encoder)
	}
	if s.Context.BitrateKbps > 0 {
		ctx = append(ctx, fmt.Sprintf("%s Mb/s", strings.Replace(fmt.Sprintf("%.1f", float64(s.Context.BitrateKbps)/1000), ".", ",", 1)))
	}
	if s.Context.OutputRes != "" {
		r := s.Context.OutputRes
		if s.Context.FPS > 0 {
			r += fmt.Sprintf("@%.0f", s.Context.FPS)
		}
		ctx = append(ctx, r)
	}
	evidence := "Lido do log do OBS de " + s.At.Local().Format("02/01/2006 15:04")
	if len(ctx) > 0 {
		evidence += " (" + strings.Join(ctx, ", ") + ")"
	}
	var out []Finding
	for _, f := range an.Findings {
		if f.Level == obs.LevelOK {
			continue
		}
		area := AreaDesempenho
		if f.ID == "network" {
			area = AreaRede
		}
		x := Finding{ID: "obs." + f.ID, Area: area, Severity: SevWarning, Title: f.Title, Evidence: evidence + ".", Explanation: f.Text}
		if f.Page != "" {
			x.Actions = append(x.Actions, Action{Kind: "page", Label: f.PageLabel, Page: f.Page})
		}
		x.Actions = append(x.Actions, Action{Kind: "page", Label: "Ver a análise do OBS", Page: "obs"})
		for _, src := range f.Sources {
			x.Sources = append(x.Sources, tweaks.Source{Title: src.Title, URL: src.URL})
		}
		out = append(out, x)
	}
	return out
}

var srcDPCLimits = tweaks.Source{Title: "Microsoft Learn — Example 15: Measuring DPC/ISR Time (um DPC não deve passar de 100 µs e uma ISR de 25 µs)", URL: "https://learn.microsoft.com/en-us/windows-hardware/drivers/devtest/example-15--measuring-dpc-isr-time"}

// dpcFinding raises the driver that held the processor longest in the newest
// reliable capture of DPCs and interrupts. It stays quiet without one (or
// with an old one, which no longer speaks for the PC) and when nothing in the
// capture was a problem.
func dpcFinding(m *Measured, now time.Time) (Finding, bool) {
	s := m.DPC
	if s == nil || !s.Reliable || (s.Verdict != dpc.VerdictAttention && s.Verdict != dpc.VerdictImportant) {
		return Finding{}, false
	}
	if !m.DPCAt.IsZero() && now.Sub(m.DPCAt) > measuredMaxAge {
		return Finding{}, false
	}
	// The driver to name: the worst verdict, then the longest routine.
	var worst *dpc.Driver
	for i := range s.Drivers {
		d := &s.Drivers[i]
		if d.Verdict == dpc.VerdictOK {
			continue
		}
		if worst == nil || (d.Verdict == dpc.VerdictImportant && worst.Verdict != dpc.VerdictImportant) ||
			(d.Verdict == worst.Verdict && maxF(d.DPC.MaxUS, d.ISR.MaxUS) > maxF(worst.DPC.MaxUS, worst.ISR.MaxUS)) {
			worst = d
		}
	}
	when := m.DPCAt.Local().Format("02/01/2006 15:04")
	if m.DPCLabel != "" {
		when += " (" + m.DPCLabel + ")"
	}
	x := Finding{ID: "dpc.slow", Area: AreaLatencia, Severity: SevWarning,
		Evidence: fmt.Sprintf("Medição de %s: %s", when, s.Headline),
		Explanation: "DPCs e interrupções são rotinas que os drivers rodam no kernel; enquanto uma roda, aquele núcleo não executa o jogo. " +
			"O Axion só aponta rotinas acima de 1 ms ou um driver que ocupa 5% ou mais do processador (a Microsoft recomenda 100 µs por DPC e 25 µs por interrupção, mas até PCs saudáveis passam disso). " +
			"Uma medição mostra onde olhar, não por que o driver demora, e retrata só aquele intervalo: meça de novo com o jogo aberto e depois de mudar algo.",
		Actions: []Action{{Kind: "page", Label: "Ver a medição", Page: "dpc"}},
		Sources: []tweaks.Source{srcDPCLimits}}
	if worst == nil {
		x.Title = "Os drivers gastam muito do processador em DPCs e interrupções"
		return x, true
	}
	name := worst.Module
	if name == "" {
		name = "um driver não identificado"
	}
	// The title says why: a long routine, or a driver that simply takes a big
	// slice of the processor.
	switch {
	case worst.Verdict == dpc.VerdictImportant:
		x.Title = "Rotinas muito lentas de driver: " + name
	case worst.DPC.Slow+worst.ISR.Slow > 0:
		x.Title = "Rotinas lentas de driver: " + name
	default:
		x.Title = "Driver ocupando muito do processador: " + name
	}
	if hint := dpc.HintFor(worst.Module); hint.Text != "" {
		x.Explanation = hint.Area + ": " + hint.Text + " " + x.Explanation
		if hint.Page != "" {
			x.Actions = append(x.Actions, Action{Kind: "page", Label: hint.PageLabel, Page: hint.Page})
		}
	}
	return x, true
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// linkFinding rewrites the PCIe link finding with the reading taken while
// a game was drawing (0.7): with the GPU working the link shows what it
// trained to, so a full-width reading clears the doubt of the idle one and
// a narrow reading under load is the real width.
func linkFinding(fs []Finding, f *facts.Facts, l *perf.GPULink, game string) []Finding {
	if l.Width <= 0 || l.WidthMax <= 0 || !l.UnderLoad() {
		return fs
	}
	if game == "" {
		game = "jogo"
	}
	ev := fmt.Sprintf("Lido pelo nvidia-smi em %s, com o %s rodando: link PCIe x%d de x%d", l.At.Local().Format("02/01 15:04"), game, l.Width, l.WidthMax)
	if l.Gen > 0 && l.GenMax > 0 {
		ev += fmt.Sprintf(", geração %d de %d", l.Gen, l.GenMax)
	}
	if l.UtilPct >= 0 {
		ev += fmt.Sprintf(", GPU a %d%%", l.UtilPct)
	}
	if l.PState != "" {
		ev += ", estado " + l.PState
	}
	ev += "."
	src := []tweaks.Source{srcNvidiaSMI}
	var x Finding
	switch {
	case l.Width >= l.WidthMax:
		x = Finding{ID: "gpu.pcie.width", Area: AreaDesempenho, Severity: SevGood, Title: fmt.Sprintf("GPU em PCIe x%d confirmado com o jogo rodando", l.Width),
			Evidence: ev, Explanation: "Com a GPU em uso o link está na largura máxima da placa: a leitura mais estreita da análise, feita com a placa em repouso, era só economia de energia.", Sources: src}
	case f != nil && f.IsLaptop():
		x = Finding{ID: "gpu.pcie.width", Area: AreaDesempenho, Severity: SevGood, Title: fmt.Sprintf("GPU do notebook ligada em PCIe x%d", l.Width),
			Evidence: ev, Explanation: fmt.Sprintf("Num notebook, quantas linhas PCIe chegam à GPU é decisão do fabricante na placa-mãe: não há slot para trocar nem nada a corrigir. O \"x%d\" é o máximo que o chip aceita.", l.WidthMax), Sources: src}
	default:
		x = Finding{ID: "gpu.pcie.width", Area: AreaDesempenho, Severity: SevWarning, Title: fmt.Sprintf("GPU trabalhando em PCIe x%d com o jogo rodando (suporta x%d)", l.Width, l.WidthMax),
			Evidence: ev, Explanation: "Lido com a GPU em uso, então essa é a largura real. A placa pode estar no slot errado (use o slot PCIe superior, ligado à CPU), mal encaixada, num riser ou dividindo linhas com um SSD M.2 (veja o manual da placa-mãe). Para ter uma ideia do tamanho: numa RTX 4090 em 1080p, o TechPowerUp mediu cerca de 2% a menos em PCIe 4.0 x8, 6% em 4.0 x4 e 21% em 3.0 x4.",
			Sources: append(src, srcPCIeScaling)}
	}
	for i := range fs {
		if fs[i].ID == "gpu.pcie.width" {
			fs[i] = x
			return fs
		}
	}
	// The analysis raised nothing (the idle reading was already full width
	// or nvidia-smi was missing then): a narrow link under load still counts.
	if x.Severity != SevGood {
		fs = append(fs, x)
	}
	return fs
}

// testLabel names a test by its time and label.
func testLabel(n *netdiag.Summary) string {
	when := n.At.Local().Format("02/01/2006 15:04")
	if n.Label != "" {
		when += " (" + n.Label + ")"
	}
	return when
}

// staleNote asks for a new test when the one at hand is old.
func staleNote(n *netdiag.Summary, now time.Time) string {
	if age := now.Sub(n.At); age > measuredMaxAge {
		return fmt.Sprintf(" O teste tem %d dias: repita antes de agir.", int(age.Hours()/24))
	}
	return ""
}

// netFindings turns the network tests into findings of the network area:
// the queue under load (bufferbloat) from the newest test with a grade, and
// the route to Valve from the newest test of all (what the CS2 page uses).
func netFindings(graded, latest *netdiag.Summary, now time.Time) []Finding {
	var out []Finding
	srcs := []tweaks.Source{{Title: netdiag.SrcBufferbloatTest.Title, URL: netdiag.SrcBufferbloatTest.URL}, {Title: netdiag.SrcBufferbloatFix.Title, URL: netdiag.SrcBufferbloatFix.URL}}
	page := Action{Kind: "page", Label: "Ver teste de rede", Page: "rede"}
	if n := graded; n != nil && n.Grade != "" && n.AddedMs >= 0 {
		when, stale := testLabel(n), staleNote(n, now)
		dir := ""
		switch {
		case n.UpAddedMs >= 0 && n.DownAddedMs >= 0 && n.UpAddedMs > n.DownAddedMs*2:
			dir = " O atraso vem do upload (+" + fmtMs(n.UpAddedMs) + " ms contra +" + fmtMs(n.DownAddedMs) + " ms no download)."
		case n.UpAddedMs >= 0 && n.DownAddedMs >= 0 && n.DownAddedMs > n.UpAddedMs*2:
			dir = " O atraso vem do download (+" + fmtMs(n.DownAddedMs) + " ms contra +" + fmtMs(n.UpAddedMs) + " ms no upload)."
		}
		ev := fmt.Sprintf("Teste de %s: nota %s, +%s ms de ping com a conexão cheia; %s Mb/s de download e %s Mb/s de upload.%s", when, n.Grade, fmtMs(n.AddedMs), fmtMs(n.DownMbps), fmtMs(n.UpMbps), dir)
		fix := "Quando algo enche a conexão (download da Steam, live, vídeo de alguém da casa), a fila do roteador segura os pacotes do jogo. O remédio é o QoS/SQM do roteador configurado com a banda medida menos 10 a 15%, e nada enviando dados durante a partida." + stale
		switch n.Grade {
		case "A+", "A":
			out = append(out, Finding{ID: "net.bloat", Area: AreaRede, Severity: SevGood, Title: "Internet sem fila sob carga (nota " + n.Grade + ")",
				Evidence: ev, Explanation: "O ping quase não sobe com a conexão cheia: o roteador ou o provedor já controla a fila.", Sources: srcs})
		case "B":
			out = append(out, Finding{ID: "net.bloat", Area: AreaRede, Severity: SevInfo, Title: "Internet com fila moderada sob carga (nota B)",
				Evidence: ev, Explanation: "Abaixo de 60 ms é aceitável. " + fix, Actions: []Action{page}, Sources: srcs})
		default:
			out = append(out, Finding{ID: "net.bloat", Area: AreaRede, Severity: SevWarning, Title: "Internet com fila sob carga (bufferbloat, nota " + n.Grade + ")",
				Evidence: ev, Explanation: fix, Actions: []Action{page}, Sources: srcs})
		}
	}
	if n := latest; n != nil && n.ValveMs >= 0 && (n.ValveLoss >= netdiag.NetLossPct || n.ValveJit >= netdiag.JitterMs) {
		when, stale := testLabel(n), staleNote(n, now)
		what := []string{}
		if n.ValveLoss >= netdiag.NetLossPct {
			what = append(what, "perda de "+fmtPct(n.ValveLoss)+"%")
		}
		if n.ValveJit >= netdiag.JitterMs {
			what = append(what, "jitter de "+fmtPct(n.ValveJit)+" ms")
		}
		// Loss shows in game (a warning, as in the network test itself);
		// jitter alone is a note that asks for a second test.
		sev := SevInfo
		ex := "Variação alta até o relay deixa o movimento dos outros jogadores menos fluido. Se ela se repetir em outro teste, o buffer de 1 pacote do CS2 pode suavizar (troca ~15,6 ms de atraso por menos engasgos)."
		if n.ValveLoss >= netdiag.NetLossPct {
			sev = SevWarning
			ex = "Perda até os relays da Valve aparece no jogo como teleporte e tiros que não registram. O teste de rede mostra em qual trecho (rede local, provedor, rota) ela surge; enquanto a causa não é resolvida, 1 pacote de buffer no CS2 troca ~15,6 ms de atraso por menos engasgos."
		}
		out = append(out, Finding{ID: "net.valve", Area: AreaRede, Severity: sev, Title: "Rota até a Valve com " + strings.Join(what, " e "),
			Evidence:    fmt.Sprintf("Teste de %s: relay %s a %s ms, %s.", when, orText(n.ValvePOP, "mais próximo"), fmtMs(n.ValveMs), strings.Join(what, ", ")),
			Explanation: ex + stale,
			Actions:     []Action{page}})
	}
	return out
}

package diag

import (
	"fmt"
	"strings"
	"time"

	"axion/internal/facts"
	"axion/internal/netdiag"
	"axion/internal/periph"
	"axion/internal/profile"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// The three readings of the full evaluation (0.7): configuration (the
// diagnostic), capacity (what the hardware can do for the use the PC has)
// and measurement (what the Axion measured on it). Each one is scored like
// the diagnostic, so the same table of penalties and grades applies.

// Area of the two extra pillars (one area each: the bars of the pillar are
// the items themselves).
const (
	AreaCapacidade = "capacidade"
	AreaMedicao    = "medicao"
)

var (
	srcCS2Req    = tweaks.Source{Title: "Valve — Counter-Strike 2 na Steam: requisitos de sistema (4 threads, 8 GB de RAM, placa DirectX 11 com 1 GB, 85 GB de espaço)", URL: "https://store.steampowered.com/app/730/CounterStrike_2/"}
	srcTeamsHW   = tweaks.Source{Title: "Microsoft Learn — Requisitos de hardware do Teams (1,1 GHz com 2 núcleos, 4 GB de RAM; 4 núcleos para reuniões grandes)", URL: "https://learn.microsoft.com/microsoftteams/hardware-requirements-for-the-teams-app"}
	srcTeamsNet  = tweaks.Source{Title: "Microsoft Learn — Preparar a rede para o Teams (ida e volta < 100 ms, perda < 1% e jitter < 30 ms em janelas de 15 s)", URL: "https://learn.microsoft.com/microsoftteams/prepare-network"}
	srcVSReq     = tweaks.Source{Title: "Microsoft Learn — Requisitos do Visual Studio 2022 (mínimo 4 GB, 16 GB recomendados; Windows e o Visual Studio em SSD)", URL: "https://learn.microsoft.com/visualstudio/releases/2022/system-requirements"}
	srcResolveHW = tweaks.Source{Title: "Blackmagic Design — Requisitos do DaVinci Resolve (16 GB de RAM, 32 GB com o Fusion; GPU com pelo menos 2 GB de VRAM, 4 GB nas configurações recomendadas)", URL: "https://www.blackmagicdesign.com/support/family/davinci-resolve-and-fusion"}
	srcNVENC     = tweaks.Source{Title: "NVIDIA — Matriz de suporte do NVENC por geração de GPU", URL: "https://developer.nvidia.com/video-encode-and-decode-gpu-support-matrix-new"}
	srcWin10EOS  = tweaks.Source{Title: "Microsoft — Fim do suporte do Windows 10 (14/10/2025) e Atualizações de Segurança Estendidas para consumidor até 13/10/2026", URL: "https://learn.microsoft.com/windows/whats-new/extended-security-updates"}
	srcRefreshHz = tweaks.Source{Title: "NVIDIA — Taxa de atualização e desempenho competitivo (taxas mais altas reduzem o tempo de cada quadro na tela)", URL: "https://www.nvidia.com/en-us/geforce/news/what-is-fps-and-how-it-helps-you-win-games/"}
)

// Capacity reads the hardware against the needs of the use profile: the
// published minimums of the programs that define each use (Counter-Strike
// 2 for the gaming profiles, Teams for the office, DaVinci Resolve for
// creation, Visual Studio for programming). Below the minimum is a
// problem, below the recommended level a warning, the rest is good.
func Capacity(f *facts.Facts, profileID string) []Finding {
	if f == nil {
		return nil
	}
	p := profile.Get(profileID)
	var out []Finding
	add := func(x Finding) {
		x.Area = AreaCapacidade
		out = append(out, x)
	}
	gb := float64(f.RAM.TotalBytes) / (1 << 30)
	ram := fmt.Sprintf("%.0f GB", gb)
	threads := f.CPU.Threads
	cpu := fmt.Sprintf("%s: %d núcleos / %d threads", orText(f.CPU.Name, "processador"), f.CPU.Cores, threads)

	switch {
	case p.Gaming:
		// CPU threads: Valve's minimum is 4.
		switch {
		case threads == 0:
			add(Finding{ID: "cap.cpu", Severity: SevInfo, Title: "Threads do processador não lidas", Evidence: cpu, Explanation: "A análise não conseguiu contar as threads; a Valve pede 4 para o CS2.", Sources: []tweaks.Source{srcCS2Req}})
		case threads < 4:
			add(Finding{ID: "cap.cpu", Severity: SevCritical, Title: fmt.Sprintf("Processador abaixo do mínimo do CS2 (%d threads; a Valve pede 4)", threads), Evidence: cpu, Explanation: "Com menos de 4 threads o jogo abre, mas a Valve não dá suporte e o FPS fica instável em partidas cheias.", Sources: []tweaks.Source{srcCS2Req}})
		case threads < 8:
			add(Finding{ID: "cap.cpu", Severity: SevInfo, Title: fmt.Sprintf("Processador com %d threads: atende ao mínimo do CS2 (4)", threads), Evidence: cpu, Explanation: "Acima do mínimo da Valve. Com Discord, navegador e um stream abertos, mais threads sobram; a medição de FPS mostra se a CPU limita o jogo.", Sources: []tweaks.Source{srcCS2Req}})
		default:
			add(Finding{ID: "cap.cpu", Severity: SevGood, Title: fmt.Sprintf("Processador com %d threads para o CS2 (mínimo 4)", threads), Evidence: cpu, Sources: []tweaks.Source{srcCS2Req}})
		}
		// RAM: 8 GB minimum.
		switch {
		case f.RAM.TotalBytes == 0:
			add(Finding{ID: "cap.ram", Severity: SevInfo, Title: "Memória não lida", Evidence: "A análise não leu a memória instalada.", Sources: []tweaks.Source{srcCS2Req}})
		case gb < 7.5:
			add(Finding{ID: "cap.ram", Severity: SevCritical, Title: "Memória abaixo do mínimo do CS2 (" + ram + "; a Valve pede 8 GB)", Evidence: "Memória instalada: " + ram + ".", Explanation: "Abaixo de 8 GB o Windows e o jogo disputam a memória e o arquivo de paginação entra em jogo: engasgos e tempos de carga altos.", Sources: []tweaks.Source{srcCS2Req}})
		case gb < 15.5:
			add(Finding{ID: "cap.ram", Severity: SevInfo, Title: "Memória de " + ram + ": atende ao mínimo do CS2 (8 GB)", Evidence: "Memória instalada: " + ram + ".", Explanation: "No mínimo da Valve. O jogo roda; com navegador, Discord e overlay abertos ao mesmo tempo, 16 GB evitam que o Windows use o disco como memória.", Sources: []tweaks.Source{srcCS2Req}})
		default:
			add(Finding{ID: "cap.ram", Severity: SevGood, Title: "Memória de " + ram + " para o CS2 (mínimo 8 GB)", Evidence: "Memória instalada: " + ram + ".", Sources: []tweaks.Source{srcCS2Req}})
		}
		// GPU: DirectX 11 card with 1 GB or more. Every GPU the analysis
		// names is DirectX 11 capable today; what is left is the memory and
		// having a driver at all.
		g := f.PrimaryGPU()
		switch {
		case g == nil:
			add(Finding{ID: "cap.gpu", Severity: SevWarning, Title: "Placa de vídeo não identificada", Evidence: "A análise não encontrou uma placa de vídeo com driver.", Explanation: "Sem o driver do fabricante o Windows usa o adaptador básico, que não serve para o CS2.", Sources: []tweaks.Source{srcCS2Req}})
		case g.BasicDriver:
			add(Finding{ID: "cap.gpu", Severity: SevCritical, Title: "Placa de vídeo sem driver do fabricante: " + g.Name, Evidence: "Driver em uso: adaptador básico da Microsoft (display.inf).", Explanation: "O CS2 exige DirectX 11 com Shader Model 5.0, o que só o driver do fabricante oferece. Instale o driver (página Drivers).", Sources: []tweaks.Source{srcCS2Req}})
		case g.VRAMMiB > 0 && g.VRAMMiB < 1000:
			add(Finding{ID: "cap.gpu", Severity: SevCritical, Title: fmt.Sprintf("Placa de vídeo abaixo do mínimo do CS2 (%d MB; a Valve pede 1 GB)", g.VRAMMiB), Evidence: g.Name + fmt.Sprintf(": %d MB de memória de vídeo.", g.VRAMMiB), Sources: []tweaks.Source{srcCS2Req}})
		case g.Integrated:
			add(Finding{ID: "cap.gpu", Severity: SevWarning, Title: "Só a GPU integrada para o CS2: " + g.Name, Evidence: g.Name + vramText(g.VRAMMiB) + "; nenhuma placa dedicada encontrada.", Explanation: "Atende ao mínimo da Valve, mas uma GPU integrada divide a memória com o sistema e costuma ficar abaixo da taxa de monitores rápidos; a medição de FPS mostra onde ela chega.", Sources: []tweaks.Source{srcCS2Req}})
		default:
			add(Finding{ID: "cap.gpu", Severity: SevGood, Title: "Placa de vídeo para o CS2: " + g.Name, Evidence: g.Name + vramText(g.VRAMMiB) + " (mínimo 1 GB, DirectX 11).", Sources: []tweaks.Source{srcCS2Req}})
		}
		// Windows version.
		if !f.OS.IsWin11 && f.OS.Build > 0 {
			add(Finding{ID: "cap.os", Severity: SevWarning, Title: "Windows 10: suporte encerrado em 14/10/2025", Evidence: osText(f), Explanation: "O CS2 ainda roda no Windows 10, mas a Microsoft encerrou o suporte em 14/10/2025; " + win10ESU(time.Now()) + " Planeje a troca para o Windows 11 (requer TPM 2.0 e Secure Boot, que os anti-cheats também pedem).", Sources: []tweaks.Source{srcWin10EOS}})
		} else if f.OS.IsWin11 {
			add(Finding{ID: "cap.os", Severity: SevGood, Title: "Windows 11 " + f.OS.DisplayVersion, Evidence: osText(f), Sources: []tweaks.Source{srcCS2Req}})
		}
		// Monitor.
		if hz, name := fastestDisplayHz(f); hz > 0 {
			switch {
			case hz < 100:
				add(Finding{ID: "cap.display", Severity: SevInfo, Title: fmt.Sprintf("Monitor de %d Hz: cada quadro fica %.1f ms na tela", hz, 1000/float64(hz)), Evidence: fmt.Sprintf("%s: taxa máxima %d Hz nas resoluções disponíveis.", orText(name, "monitor"), hz), Explanation: "O jogo funciona em qualquer taxa; um monitor de 144 Hz ou mais mostra cada quadro por menos tempo (6,9 ms a 144 Hz, 4,2 ms a 240 Hz), o que dá movimento mais fluido e imagem mais atual. É o upgrade com efeito mais visível quando o PC já entrega FPS acima de 144.", Sources: []tweaks.Source{srcRefreshHz}})
			default:
				add(Finding{ID: "cap.display", Severity: SevGood, Title: fmt.Sprintf("Monitor de %d Hz", hz), Evidence: fmt.Sprintf("%s: taxa máxima %d Hz.", orText(name, "monitor"), hz), Sources: []tweaks.Source{srcRefreshHz}})
			}
		}
	case p.ID == profile.Creation:
		switch {
		case f.RAM.TotalBytes == 0:
			add(Finding{ID: "cap.ram", Severity: SevInfo, Title: "Memória não lida", Evidence: "A análise não leu a memória instalada.", Sources: []tweaks.Source{srcResolveHW}})
		case gb < 15.5:
			add(Finding{ID: "cap.ram", Severity: SevWarning, Title: "Memória de " + ram + ": abaixo do mínimo dos editores de vídeo (16 GB)", Evidence: "Memória instalada: " + ram + ".", Explanation: "O DaVinci Resolve pede 16 GB (32 GB com o Fusion); abaixo disso a edição em 4K e a renderização usam o disco como memória e ficam lentas.", Sources: []tweaks.Source{srcResolveHW}})
		case gb < 31.5:
			add(Finding{ID: "cap.ram", Severity: SevInfo, Title: "Memória de " + ram + ": atende ao mínimo dos editores de vídeo (16 GB)", Evidence: "Memória instalada: " + ram + ".", Explanation: "No mínimo do DaVinci Resolve; 32 GB são o pedido para o Fusion e para projetos em 4K com vários efeitos.", Sources: []tweaks.Source{srcResolveHW}})
		default:
			add(Finding{ID: "cap.ram", Severity: SevGood, Title: "Memória de " + ram + " para edição (mínimo 16 GB, 32 GB com Fusion)", Evidence: "Memória instalada: " + ram + ".", Sources: []tweaks.Source{srcResolveHW}})
		}
		g := f.PrimaryGPU()
		switch {
		case g == nil || g.BasicDriver:
			add(Finding{ID: "cap.gpu", Severity: SevCritical, Title: "Placa de vídeo sem driver do fabricante", Evidence: "A análise não encontrou uma placa de vídeo com driver do fabricante.", Explanation: "Os editores de vídeo usam a GPU para efeitos e para o encoder; sem driver nada disso existe.", Sources: []tweaks.Source{srcResolveHW}})
		case g.VRAMMiB > 0 && g.VRAMMiB < 1900:
			add(Finding{ID: "cap.gpu", Severity: SevWarning, Title: fmt.Sprintf("Placa de vídeo com %d MB: abaixo dos 2 GB mínimos dos editores", g.VRAMMiB), Evidence: g.Name + vramText(g.VRAMMiB) + ".", Explanation: "O DaVinci Resolve pede pelo menos 2 GB de memória de vídeo (4 GB nas configurações recomendadas); com menos, efeitos e timelines em 4K falham por falta de memória da GPU.", Sources: []tweaks.Source{srcResolveHW}})
		case g.VRAMMiB > 0 && g.VRAMMiB < 3900:
			add(Finding{ID: "cap.gpu", Severity: SevInfo, Title: fmt.Sprintf("Placa de vídeo com %d MB: atende ao mínimo dos editores (2 GB), abaixo dos 4 GB recomendados", g.VRAMMiB), Evidence: g.Name + vramText(g.VRAMMiB) + ".", Explanation: "Timelines em 4K com efeitos pedem mais memória de vídeo; em 1080p o trabalho segue normal.", Sources: []tweaks.Source{srcResolveHW}})
		default:
			title := "Placa de vídeo para edição: " + g.Name
			ex := ""
			if g.Vendor == "NVIDIA" {
				ex = "Placas NVIDIA GeForce trazem o encoder NVENC, que o OBS, o Premiere e o Resolve usam para exportar e transmitir sem ocupar o processador."
			}
			add(Finding{ID: "cap.gpu", Severity: SevGood, Title: title, Evidence: g.Name + vramText(g.VRAMMiB) + " (recomendados 4 GB).", Explanation: ex, Sources: []tweaks.Source{srcResolveHW, srcNVENC}})
		}
		cpuCap(f, add, 8, "edição e renderização", srcResolveHW)
	case p.ID == profile.Dev:
		switch {
		case f.RAM.TotalBytes == 0:
			add(Finding{ID: "cap.ram", Severity: SevInfo, Title: "Memória não lida", Evidence: "A análise não leu a memória instalada.", Sources: []tweaks.Source{srcVSReq}})
		case gb < 7.5:
			add(Finding{ID: "cap.ram", Severity: SevWarning, Title: "Memória de " + ram + ": pouca para IDE, contêineres e navegador juntos", Evidence: "Memória instalada: " + ram + ".", Explanation: "O Visual Studio pede 4 GB no mínimo e recomenda 16 GB para soluções profissionais; o Docker Desktop e o WSL reservam memória própria.", Sources: []tweaks.Source{srcVSReq}})
		case gb < 15.5:
			add(Finding{ID: "cap.ram", Severity: SevInfo, Title: "Memória de " + ram + ": o Visual Studio recomenda 16 GB", Evidence: "Memória instalada: " + ram + ".", Explanation: "Acima do mínimo (4 GB); 16 GB é a recomendação da Microsoft para soluções profissionais, e contêineres ou máquinas virtuais pedem mais.", Sources: []tweaks.Source{srcVSReq}})
		default:
			add(Finding{ID: "cap.ram", Severity: SevGood, Title: "Memória de " + ram + " para programação (recomendados 16 GB)", Evidence: "Memória instalada: " + ram + ".", Sources: []tweaks.Source{srcVSReq}})
		}
		cpuCap(f, add, 8, "compilação", srcVSReq)
	default: // work and office
		switch {
		case f.RAM.TotalBytes == 0:
			add(Finding{ID: "cap.ram", Severity: SevInfo, Title: "Memória não lida", Evidence: "A análise não leu a memória instalada.", Sources: []tweaks.Source{srcTeamsHW}})
		case gb < 3.9:
			add(Finding{ID: "cap.ram", Severity: SevCritical, Title: "Memória de " + ram + ": abaixo do mínimo do Teams (4 GB)", Evidence: "Memória instalada: " + ram + ".", Sources: []tweaks.Source{srcTeamsHW}})
		case gb < 7.5:
			add(Finding{ID: "cap.ram", Severity: SevWarning, Title: "Memória de " + ram + ": atende ao Teams, mas com o navegador cheio de abas falta", Evidence: "Memória instalada: " + ram + ".", Explanation: "4 GB é o mínimo do Teams; com navegador, Office e chamada de vídeo ao mesmo tempo, 8 GB evitam que o Windows use o disco como memória.", Sources: []tweaks.Source{srcTeamsHW}})
		default:
			add(Finding{ID: "cap.ram", Severity: SevGood, Title: "Memória de " + ram + " para trabalho e chamadas (mínimo 4 GB)", Evidence: "Memória instalada: " + ram + ".", Sources: []tweaks.Source{srcTeamsHW}})
		}
		switch {
		case threads == 0:
			add(Finding{ID: "cap.cpu", Severity: SevInfo, Title: "Threads do processador não lidas", Evidence: cpu, Sources: []tweaks.Source{srcTeamsHW}})
		case f.CPU.Cores < 2 && f.CPU.Cores > 0:
			add(Finding{ID: "cap.cpu", Severity: SevCritical, Title: "Processador de 1 núcleo: abaixo do mínimo do Teams (2)", Evidence: cpu, Sources: []tweaks.Source{srcTeamsHW}})
		case f.CPU.Cores < 4 && f.CPU.Cores > 0:
			add(Finding{ID: "cap.cpu", Severity: SevInfo, Title: fmt.Sprintf("Processador de %d núcleos: atende ao Teams; 4 são recomendados para reuniões grandes", f.CPU.Cores), Evidence: cpu, Sources: []tweaks.Source{srcTeamsHW}})
		default:
			add(Finding{ID: "cap.cpu", Severity: SevGood, Title: fmt.Sprintf("Processador com %d núcleos para trabalho e chamadas", f.CPU.Cores), Evidence: cpu, Sources: []tweaks.Source{srcTeamsHW}})
		}
	}
	// The system disk: every use wants Windows on an SSD. The Visual Studio
	// page says it in so many words, so it is the source for the programming
	// profile; for the other uses the reading needs no outside source (the
	// disk type is read from the PC), and the explanation of a mechanical
	// disk stands on its own.
	if d := f.SystemDisk(); d != nil {
		var src []tweaks.Source
		why := "Um SSD é o upgrade de maior efeito num PC com disco mecânico: inicialização, abertura de programas e carregamento de mapas em segundos em vez de minutos."
		if profileID == profile.Dev {
			src = []tweaks.Source{srcVSReq}
			why += " A Microsoft recomenda o Windows e o Visual Studio em SSD."
		}
		switch strings.ToUpper(d.MediaType) {
		case "HDD":
			add(Finding{ID: "cap.disk", Severity: SevWarning, Title: "Windows em disco mecânico: " + d.Model, Evidence: fmt.Sprintf("Disco do sistema: %s (%s, %s).", d.Model, d.MediaType, d.BusType), Explanation: why, Sources: src})
		case "SSD":
			add(Finding{ID: "cap.disk", Severity: SevGood, Title: "Windows em SSD (" + d.BusType + "): " + d.Model, Evidence: fmt.Sprintf("Disco do sistema: %s (%s, %s).", d.Model, d.MediaType, d.BusType), Sources: src})
		}
	}
	if !p.Gaming && f.OS.Build > 0 {
		if !f.OS.IsWin11 {
			add(Finding{ID: "cap.os", Severity: SevWarning, Title: "Windows 10: suporte encerrado em 14/10/2025", Evidence: osText(f), Explanation: "A Microsoft encerrou o suporte em 14/10/2025; " + win10ESU(time.Now()) + " Planeje a troca para o Windows 11.", Sources: []tweaks.Source{srcWin10EOS}})
		} else {
			add(Finding{ID: "cap.os", Severity: SevGood, Title: "Windows 11 " + f.OS.DisplayVersion, Evidence: osText(f)})
		}
	}
	return out
}

// cpuCap is the processor item of the uses that scale with cores
// (creation, programming): no published minimum, a note under n threads.
func cpuCap(f *facts.Facts, add func(Finding), n int, use string, src tweaks.Source) {
	cpu := fmt.Sprintf("%s: %d núcleos / %d threads", orText(f.CPU.Name, "processador"), f.CPU.Cores, f.CPU.Threads)
	switch {
	case f.CPU.Threads == 0:
		add(Finding{ID: "cap.cpu", Severity: SevInfo, Title: "Threads do processador não lidas", Evidence: cpu, Sources: []tweaks.Source{src}})
	case f.CPU.Threads < n:
		add(Finding{ID: "cap.cpu", Severity: SevInfo, Title: fmt.Sprintf("Processador com %d threads: %s escala com mais núcleos", f.CPU.Threads, use), Evidence: cpu, Explanation: "Não há mínimo publicado; o tempo de exportação e de build cai quase na proporção dos núcleos. A medição de carga sustentada mostra se o processador é o gargalo.", Sources: []tweaks.Source{src}})
	default:
		add(Finding{ID: "cap.cpu", Severity: SevGood, Title: fmt.Sprintf("Processador com %d threads para %s", f.CPU.Threads, use), Evidence: cpu, Sources: []tweaks.Source{src}})
	}
}

func vramText(mib int) string {
	if mib <= 0 {
		return ""
	}
	if mib >= 1000 {
		return fmt.Sprintf(": %.0f GB de memória de vídeo", float64(mib)/1024)
	}
	return fmt.Sprintf(": %d MB de memória de vídeo", mib)
}

func osText(f *facts.Facts) string {
	return strings.TrimSpace(fmt.Sprintf("%s %s (build %d.%d).", f.OS.Caption, f.OS.DisplayVersion, f.OS.Build, f.OS.UBR))
}

// fastestDisplayHz is the refresh rate the game can use: the primary
// monitor's fastest mode in any resolution (the game runs there), or the
// fastest monitor connected when none is marked primary.
func fastestDisplayHz(f *facts.Facts) (maxHz int, name string) {
	best := func(d sys.DisplayMode) int {
		if d.MaxHzAtRes > d.MaxHzAny {
			return d.MaxHzAtRes
		}
		return d.MaxHzAny
	}
	for _, d := range f.Displays {
		if d.Primary && best(d) > 0 {
			return best(d), d.Monitor
		}
	}
	for _, d := range f.Displays {
		if hz := best(d); hz > maxHz {
			maxHz, name = hz, d.Monitor
		}
	}
	return
}

// Measurement turns what the Axion measured into findings of the
// measurement pillar: the network (queue under load and the route to
// Valve, or the targets of Teams calls), the FPS against the monitor and
// the polling rate of the mouse. Missing lists what the profile measures
// and was not measured yet (nothing is scored for what is absent).
func Measurement(f *facts.Facts, profileID string, m *Measured, periphs []periph.Measurement) (fs []Finding, missing []string) {
	p := profile.Get(profileID)
	now := time.Now()
	add := func(x Finding) {
		x.Area = AreaMedicao
		fs = append(fs, x)
	}
	var latest *netdiag.Summary
	if m != nil {
		latest = m.NetLatest
		if latest == nil {
			latest = m.Net
		}
	}
	switch {
	case latest == nil:
		missing = append(missing, "teste de rede")
	case p.Gaming:
		for _, x := range netFindings(m.Net, latest, now) {
			add(x)
		}
		if m.Net == nil || m.Net.Grade == "" {
			missing = append(missing, "teste de rede com carga (bufferbloat)")
		}
		if latest.ValveMs >= 0 && latest.ValveLoss < netdiag.NetLossPct && latest.ValveJit < netdiag.JitterMs {
			add(Finding{ID: "net.valve", Severity: SevGood, Title: "Rota até a Valve sem perda nem jitter",
				Evidence: fmt.Sprintf("Teste de %s: relay %s a %s ms, jitter %s ms, perda %s%%.", testLabel(latest), orText(latest.ValvePOP, "mais próximo"), fmtMs(latest.ValveMs), fmtPct(latest.ValveJit), fmtPct(latest.ValveLoss)) + staleNote(latest, now)})
		}
	default:
		// Calls: Microsoft's targets for Teams, read on the route to the
		// internet with the connection idle, plus the queue under load.
		if g := m.Net; g != nil && g.Grade != "" {
			for _, x := range netFindings(g, nil, now) {
				if x.ID == "net.bloat" {
					if x.Severity == SevWarning {
						x.Explanation = "Quando algo enche a conexão (backup na nuvem, vídeo de alguém da casa, download), a fila do roteador atrasa o áudio e o vídeo da chamada. O remédio é o QoS/SQM do roteador com a banda medida menos 10 a 15%." + staleNote(g, now)
					}
					add(x)
				}
			}
		} else {
			missing = append(missing, "teste de rede com carga (bufferbloat)")
		}
		if latest.NetMs >= 0 {
			ev := fmt.Sprintf("Teste de %s: internet (1.1.1.1) a %s ms, jitter %s ms, perda %s%% com a conexão parada.", testLabel(latest), fmtMs(latest.NetMs), fmtPct(latest.NetJitter), fmtPct(latest.NetLoss))
			var bad []string
			if latest.NetMs >= 100 { // Microsoft's target is a round trip under 100 ms
				bad = append(bad, "latência")
			}
			if latest.NetLoss >= 1 {
				bad = append(bad, "perda")
			}
			if latest.NetJitter >= 30 {
				bad = append(bad, "jitter")
			}
			if len(bad) == 0 {
				add(Finding{ID: "net.calls", Severity: SevGood, Title: "Rede dentro das metas da Microsoft para chamadas", Evidence: ev, Explanation: "Metas do Teams: latência até 100 ms, perda abaixo de 1% e jitter abaixo de 30 ms em janelas de 15 s.", Sources: []tweaks.Source{srcTeamsNet}})
			} else {
				add(Finding{ID: "net.calls", Severity: SevWarning, Title: "Rede fora das metas da Microsoft para chamadas (" + strings.Join(bad, ", ") + ")", Evidence: ev, Explanation: "Metas do Teams: latência até 100 ms, perda abaixo de 1% e jitter abaixo de 30 ms em janelas de 15 s. Perda e jitter costumam vir do Wi-Fi ou de uma conexão cheia; o teste de rede mostra em qual trecho aparecem." + staleNote(latest, now), Actions: []Action{{Kind: "page", Label: "Ver teste de rede", Page: "rede"}}, Sources: []tweaks.Source{srcTeamsNet}})
			}
		}
	}
	if p.Gaming {
		// FPS against the monitor.
		hz, name := fastestDisplayHz(f)
		switch {
		case m == nil || m.Perf == nil:
			missing = append(missing, "medição de FPS no CS2")
		default:
			s := m.Perf
			when := "a medição"
			if !m.PerfAt.IsZero() {
				when = "a medição de " + m.PerfAt.Local().Format("02/01")
			}
			lim := map[string]string{"gpu": "a placa de vídeo", "cpu": "o processador (ou o limite de FPS do jogo)", "limite": "o limite de FPS do jogo", "misto": "ora a CPU, ora a GPU"}[s.Limiter]
			ev := fmt.Sprintf("%s (%s): %s FPS médio, 1%% low %s, %d quadros em %s s", strings.ToUpper(when[:1])+when[1:], orText(m.PerfGame, "jogo"), fmtMs(s.AvgFPS), fmtMs(s.Low1FPS), s.Frames, fmtMs(s.Seconds))
			if lim != "" {
				ev += "; quem limitou: " + lim
			}
			ev += "."
			stale := ""
			if !m.PerfAt.IsZero() && now.Sub(m.PerfAt) > measuredMaxAge {
				stale = fmt.Sprintf(" A medição tem %d dias: repita antes de agir.", int(now.Sub(m.PerfAt).Hours()/24))
			}
			switch {
			case hz <= 0:
				add(Finding{ID: "meas.fps", Severity: SevInfo, Title: fmt.Sprintf("%s FPS médio, 1%% low %s (monitor não identificado)", fmtMs(s.AvgFPS), fmtMs(s.Low1FPS)), Evidence: ev, Explanation: "Sem a taxa do monitor não dá para dizer se o FPS acompanha a tela." + stale})
			case s.Low1FPS >= float64(hz):
				add(Finding{ID: "meas.fps", Severity: SevGood, Title: fmt.Sprintf("FPS acima da taxa do monitor (%d Hz) até no 1%% low", hz), Evidence: ev, Explanation: fmt.Sprintf("Mesmo nos piores 1%% dos quadros o jogo entregou mais do que o %s mostra: a tela está sempre com quadro novo.", orText(name, "monitor")) + stale})
			case s.AvgFPS >= float64(hz):
				add(Finding{ID: "meas.fps", Severity: SevInfo, Title: fmt.Sprintf("FPS médio acima dos %d Hz do monitor, mas o 1%% low (%s) fica abaixo", hz, fmtMs(s.Low1FPS)), Evidence: ev, Explanation: "Na média a tela recebe quadro novo a cada atualização; nos momentos pesados (fumaça, troca de tiros em cinco) o FPS cai abaixo da taxa e o movimento perde fluidez. " + fpsAdvice(s.Limiter) + stale, Actions: []Action{{Kind: "page", Label: "Ver medições", Page: "desempenho"}}})
			default:
				add(Finding{ID: "meas.fps", Severity: SevWarning, Title: fmt.Sprintf("FPS médio (%s) abaixo dos %d Hz do monitor", fmtMs(s.AvgFPS), hz), Evidence: ev, Explanation: "O monitor atualiza mais vezes do que o jogo entrega quadros: parte da taxa que você pagou fica sem uso. " + fpsAdvice(s.Limiter) + stale, Actions: []Action{{Kind: "page", Label: "Ver medições", Page: "desempenho"}}})
			}
		}
		// Mouse polling.
		if len(periphs) == 0 {
			missing = append(missing, "taxa do mouse")
		} else {
			pm := periphs[0]
			found := false
			for _, d := range pm.Devices {
				if d.Primary != "mouse" || d.MeasuredHz <= 0 {
					continue
				}
				found = true
				ev := fmt.Sprintf("Medição de %s: %s a %s medidos (%s).", pm.At.Local().Format("02/01 15:04"), d.Name, periph.FormatHz(d.MeasuredHz), d.Summary)
				switch {
				case d.Verdict == periph.VerdictOK && d.MeasuredHz >= 450:
					add(Finding{ID: "meas.mouse", Severity: SevGood, Title: "Mouse a " + periph.FormatHz(d.MeasuredHz) + ", estável", Evidence: ev})
				case d.Verdict == periph.VerdictOK:
					add(Finding{ID: "meas.mouse", Severity: SevInfo, Title: "Mouse a " + periph.FormatHz(d.MeasuredHz) + ": estável, mas abaixo de 500 Hz", Evidence: ev, Explanation: "A 125 Hz o PC lê o mouse a cada 8 ms; a 1000 Hz, a cada 1 ms. A taxa é definida no software do fabricante (ou num botão do próprio mouse), não no Windows.", Actions: []Action{{Kind: "page", Label: "Ver periféricos", Page: "perifericos"}}})
				case d.Verdict == periph.VerdictLittle:
					add(Finding{ID: "meas.mouse", Severity: SevInfo, Title: "Mouse: poucos dados na medição", Evidence: ev, Explanation: "Repita a medição movendo o mouse sem parar durante toda a contagem.", Actions: []Action{{Kind: "page", Label: "Ver periféricos", Page: "perifericos"}}})
				default:
					add(Finding{ID: "meas.mouse", Severity: SevWarning, Title: "Mouse abaixo da taxa nominal ou irregular (" + periph.FormatHz(d.MeasuredHz) + ")", Evidence: ev, Explanation: "O mouse entregou menos relatórios do que anuncia ou em intervalos irregulares: hub USB compartilhado, porta USB 2.0 lenta, suspensão seletiva ou software do fabricante. A página Periféricos mostra o caminho USB e as correções.", Actions: []Action{{Kind: "page", Label: "Ver periféricos", Page: "perifericos"}}})
				}
				break
			}
			if !found {
				missing = append(missing, "taxa do mouse")
			}
		}
	}
	return fs, missing
}

func fpsAdvice(limiter string) string {
	switch limiter {
	case "gpu":
		return "Quem limitou foi a placa de vídeo: baixar opções de vídeo (sombras, oclusão de ambiente, MSAA, partículas, escala de resolução) sobe o 1% low; a página CS2 sugere o que baixar."
	case "cpu":
		return "Quem limitou foi o lado da CPU: opções de vídeo pouco mudam; o que ajuda é fechar programas em segundo plano, o perfil de memória (XMP/EXPO) e, num X3D, o jogo no CCD certo."
	case "limite":
		return "O jogo estava no próprio limite de FPS (fps_max): suba ou tire o limite para ver até onde o PC vai."
	}
	return "A página Desempenho mostra quem limitou cada quadro (CPU ou GPU) e o que fazer em cada caso."
}

// win10ESUEnd is the last day of the consumer Extended Security Updates for
// Windows 10 (Microsoft: one year after the end of support, to 13/10/2026).
var win10ESUEnd = time.Date(2026, 10, 13, 23, 59, 59, 0, time.Local)

// win10ESU says where the consumer extended security updates stand.
func win10ESU(now time.Time) string {
	if now.After(win10ESUEnd) {
		return "as atualizações de segurança estendidas para consumidor terminaram em 13/10/2026, então este Windows não recebe mais correções de segurança."
	}
	return "as atualizações de segurança estendidas para consumidor vão até 13/10/2026 (o FACEIT só aceita o 22H2 com elas)."
}

package netdiag

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Thresholds of the verdicts.
const (
	// WiredLossPct: on a cable one lost ping in 200 is already abnormal.
	WiredLossPct = 0.5
	// WiFiLossPct: Wi-Fi retransmits most losses itself; 1% gets through.
	WiFiLossPct = 1.0
	// NetLossPct: loss to the internet or to Valve that shows in game.
	NetLossPct = 1.0
	// JitterMs: mean variation between consecutive pings that starts to
	// show as uneven movement in game.
	JitterMs = 4.0
	// TickMs is one CS2 server tick (64 per second): the latency each
	// packet of buffering adds.
	TickMs = 15.625
	// CS2FullRate is the "unrestricted" value of the rate setting.
	CS2FullRate = 786432
	// WiFiWeakSignalPct: Axion's own line for a weak Wi-Fi signal (the
	// percentage that netsh reports, not a Microsoft figure).
	WiFiWeakSignalPct = 70
)

// wifiHint adds what was read from the Wi-Fi adapter at the start of the test.
// The signal is the value from that moment, saved with the test, so it never
// says "now". The 2.4 GHz remark only repeats what Microsoft's page "Wi-Fi and
// your home layout" says: that band is the one microwaves, cordless phones and
// Bluetooth devices use, and it is more congested than 5 GHz.
func wifiHint(a AdapterInfo) string {
	s := ""
	if a.WiFiSignal > 0 && a.WiFiSignal < WiFiWeakSignalPct {
		s += fmt.Sprintf(" No começo do teste o sinal estava em %d%% (critério do Axion: abaixo de %d%% é sinal fraco).", a.WiFiSignal, WiFiWeakSignalPct)
	}
	if strings.Contains(a.WiFiBand, "2.4") {
		s += " O PC estava na banda de 2,4 GHz, que a Microsoft cita como a mesma usada por micro-ondas, telefones sem fio e aparelhos Bluetooth, e diz que ela fica mais congestionada que a de 5 GHz."
	}
	return s
}

// meshHint: a cable to a repeater or mesh node still crosses Wi-Fi when that
// node reaches the main router without a cable, and the test cannot see it.
const meshHint = "Se o cabo vai até um repetidor ou nó de mesh que fala com o roteador principal sem fio, a perda ou os picos podem estar nesse trecho sem fio: ligue os dois por cabo, se o sistema permitir, ou teste com o PC no roteador principal."

// Sources cited by the network verdicts.
var (
	SrcBufferbloatFix  = Source{"Bufferbloat.net — What can I do about Bufferbloat? (SQM: medir, depois ajustar à velocidade medida)", "https://www.bufferbloat.net/projects/bloat/wiki/What_can_I_do_about_Bufferbloat/"}
	SrcBufferbloatTest = Source{"Bufferbloat.net — Tests for Bufferbloat (acima de 50 ms ou nota abaixo de B é problema)", "https://www.bufferbloat.net/projects/bloat/wiki/Tests_for_Bufferbloat/"}
	SrcTPLinkQoS       = Source{"TP-Link — Como configurar o QoS (informar a banda de download e upload)", "https://www.tp-link.com/us/support/faq/1104/"}
	SrcTPLinkQoSHS     = Source{"TP-Link — QoS do HomeShield (informar a banda exata da conexão)", "https://www.tp-link.com/nordic/support/faq/2992/"}
	SrcAIM             = Source{"Cloudflare — Aggregated Internet Measurement (pontuação para jogos)", "https://developers.cloudflare.com/speed/aim/"}
	SrcAIMCode         = Source{"Cloudflare speedtest (código aberto) — limites da pontuação AIM", "https://github.com/cloudflare/speedtest/blob/main/src/config/internalConfig.ts"}
	SrcSDR             = Source{"Valve — Steam Datagram Relay (rede de relays da Valve)", "https://partner.steamgames.com/doc/features/multiplayer/steamdatagramrelay"}
	SrcCS2Buffer       = Source{"Valve — Notas do CS2 de 6/2 e 15/2/2024 (buffer contra perda de pacotes, cl_ticktiming, telemetria)", "https://store.steampowered.com/news/posts/?feed=steam_community_announcements&appids=730&enddate=1708400000"}
	SrcEEE             = Source{"Tom's Hardware — Intel desliga o EEE a 1/2,5 Gb/s para corrigir quedas das placas I225/I226", "https://www.tomshardware.com/news/intel-patches-stuttering-ethernet-issues-but-its-just-a-workaround-for-now"}
	SrcEEEWake         = Source{"EE News Europe — Tempo para acordar o link no EEE (16,5 µs no gigabit, 30 µs em 100 Mb/s)", "https://www.eenewseurope.com/en/powering-ethernet-part-2-optimizing-power-consumption-during-standby-operation/"}
	SrcCGNAT           = Source{"RFC 6598 — Faixa 100.64.0.0/10 para NAT da operadora (CGNAT)", "https://www.rfc-editor.org/rfc/rfc6598"}
	SrcDNS             = Source{"RFC 1034 — DNS: tradução de nomes em endereços", "https://www.rfc-editor.org/rfc/rfc1034"}
	SrcGigabit         = Source{"IEEE 802.3ab — 1000BASE-T usa os quatro pares do cabo", "https://standards.ieee.org/ieee/802.3ab/1086/"}
)

// Source is a reference shown with a finding.
type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Analyze fills r.Findings and r.AIM from the measurements.
func Analyze(r *Run) {
	var fs []Finding
	add := func(f Finding) { fs = append(fs, f) }
	wired := strings.EqualFold(r.Adapter.Kind, "Ethernet")
	wifi := strings.EqualFold(r.Adapter.Kind, "Wi-Fi")
	lan, inet, valve := r.Target(RoleLAN), r.Target(RoleInternet), r.BestValve()
	limit := WiredLossPct
	if !wired {
		limit = WiFiLossPct
	}
	lanOK := lan != nil && lan.Idle.OK()
	inetOK := inet != nil && inet.Idle.OK()
	// The pings to the internet cross the same cable or Wi-Fi as those to the
	// router, at the same time. When the router "loses" clearly more than the
	// whole path, it is ignoring pings addressed to itself (routers limit what
	// their own processor answers), not dropping the traffic that crosses it.
	routerDrops := lanOK && inetOK && lan.Idle.LossPct >= limit && inet.Idle.LossPct < lan.Idle.LossPct/2
	// homeLoss is the loss the home network explains; lanLossy, that it is
	// large enough to be a problem on its own.
	homeLoss := 0.0
	if lanOK && !routerDrops {
		homeLoss = lan.Idle.LossPct
	}
	lanLossy := homeLoss >= limit
	homeFix := "a conexão até o roteador"
	switch {
	case wired:
		homeFix = "cabo, porta e, se houver, o repetidor ou nó de mesh"
	case wifi:
		homeFix = "o sinal do Wi-Fi: cabo, ou 5 GHz perto do roteador"
	}

	// Local network.
	switch {
	case lan == nil:
	case !lan.Idle.OK():
		add(Finding{ID: "lan.noreply", Area: "lan", Severity: "info", Title: "O roteador não respondeu ao ping",
			Text: fmt.Sprintf("Sem resposta de %s. Alguns roteadores bloqueiam ping na rede local; se a internet funciona, isso não é um problema, mas o teste não consegue separar a sua rede da operadora.", lan.Addr)})
	default:
		spiky := false
		if wired {
			// A router answers ping from its own processor: spikes there only
			// matter when the traffic that passes through it spikes too.
			spiky = lan.Idle.P95 > 5 && inetOK && inet.Idle.P95-inet.Idle.Median > 5
		} else {
			spiky = lan.Idle.P95 > 15
		}
		switch {
		case routerDrops:
			add(Finding{ID: "lan.icmp", Area: "lan", Severity: "info", Title: fmt.Sprintf("O roteador ignora parte dos pings dirigidos a ele (%s)", pct(lan.Idle.LossPct)),
				Text: fmt.Sprintf("Os pings que passam por ele até a internet perderam só %s, então a conexão até o roteador está bem: muitos roteadores limitam as respostas do próprio processador. A latência até ele continua valendo.", pct(inet.Idle.LossPct))})
		case lanLossy && wired:
			add(Finding{ID: "lan.loss", Area: "lan", Severity: "warning", Title: fmt.Sprintf("Perda de pacotes dentro de casa: %s entre o PC e o roteador", pct(lan.Idle.LossPct)),
				Text: "Num cabo isso não deveria acontecer. Causas comuns: cabo ou conector danificado, porta do roteador ou do switch, ou o driver da placa de rede. Troque o cabo e a porta e teste de novo. " + meshHint})
		case lanLossy && wifi:
			txt := "O Wi-Fi reenvia quase tudo sozinho; perda que chega até aqui indica sinal fraco ou interferência." + wifiHint(r.Adapter)
			txt += " Para jogar, prefira cabo; se não der, use 5 GHz (ou 6 GHz) perto do roteador."
			add(Finding{ID: "lan.loss", Area: "lan", Severity: "warning", Title: fmt.Sprintf("Wi-Fi perdendo pacotes: %s até o roteador", pct(lan.Idle.LossPct)), Text: txt,
				Actions: []Action{{Kind: "url", Label: "Microsoft: Problemas de Wi-Fi e o layout da casa", URL: "https://support.microsoft.com/windows/wi-fi-problems-and-your-home-layout-e1ed42e7-a3c5-d1be-2abb-e8fad00ad32a"}}})
		case lanLossy:
			add(Finding{ID: "lan.loss", Area: "lan", Severity: "warning", Title: fmt.Sprintf("Perda de pacotes até o roteador: %s", pct(lan.Idle.LossPct)),
				Text: "Algo entre o PC e o roteador descarta pacotes: o adaptador de rede, o cabo, um repetidor ou o próprio roteador. Para jogar, prefira cabo direto no roteador e teste de novo."})
		case spiky && wired:
			add(Finding{ID: "lan.spikes", Area: "lan", Severity: "warning", Title: fmt.Sprintf("Picos na rede local (P95 de %s ms até o roteador)", num(lan.Idle.P95)),
				Text: "Os picos aparecem também no caminho até a internet. Reinicie o roteador e confira se algum aparelho da casa está usando a rede pesado (download, câmera, backup). " + meshHint})
		case spiky && wifi:
			txt := "No Wi-Fi a latência varia com a distância, paredes e redes vizinhas." + wifiHint(r.Adapter)
			txt += " Para jogar, cabo é o que resolve; sem cabo, use 5 GHz (ou 6 GHz) perto do roteador."
			add(Finding{ID: "lan.spikes", Area: "lan", Severity: "warning", Title: fmt.Sprintf("Wi-Fi com picos de latência (P95 de %s ms até o roteador)", num(lan.Idle.P95)), Text: txt,
				Actions: []Action{{Kind: "url", Label: "Microsoft: Problemas de Wi-Fi e o layout da casa", URL: "https://support.microsoft.com/windows/wi-fi-problems-and-your-home-layout-e1ed42e7-a3c5-d1be-2abb-e8fad00ad32a"}}})
		case spiky:
			add(Finding{ID: "lan.spikes", Area: "lan", Severity: "warning", Title: fmt.Sprintf("Picos de latência até o roteador (P95 de %s ms)", num(lan.Idle.P95)),
				Text: "A latência até o roteador varia muito. Para jogar, prefira cabo direto no roteador."})
		default:
			loss := "sem perda"
			if lan.Idle.LossPct > 0 {
				loss = "perda de " + pct(lan.Idle.LossPct) + " (tolerável)"
			}
			txt := "A conexão até o roteador está estável neste teste."
			switch {
			case wired:
				txt = "Cabo, placa de rede e roteador não estão atrapalhando."
			case wifi:
				txt = "O Wi-Fi até o roteador está estável neste teste."
			}
			add(Finding{ID: "lan.ok", Area: "lan", Severity: "good", Title: fmt.Sprintf("Rede local ok: %s ms até o roteador, %s", num(lan.Idle.Median), loss), Text: txt})
		}
	}

	// Link speed of the cable.
	if wired && r.Adapter.Gigabit && r.Adapter.LinkMbps > 0 && r.Adapter.LinkMbps <= 100 {
		down := -1.0
		if p := r.Bloat.Phase("download"); p != nil && p.Error == "" {
			down = p.Mbps
		}
		t := fmt.Sprintf("A placa de rede conectou a %d Mb/s", r.Adapter.LinkMbps)
		txt := "Esta placa conecta a 1 Gb/s ou mais. O gigabit usa os quatro pares do cabo; com um par ruim ou cabo antigo, o link cai para 100 Mb/s. Troque o cabo (Cat5e ou Cat6) e teste outra porta; se a porta do roteador for de 100 Mb/s, o limite é dela."
		if down >= 0 && down >= 0.85*float64(r.Adapter.LinkMbps) {
			txt = fmt.Sprintf("O download (%s Mb/s) bateu no limite do link. ", num(down)) + txt
		}
		add(Finding{ID: "nic.link", Area: "nic", Severity: "warning", Title: t, Text: txt})
	}

	// Internet.
	if inet != nil {
		switch {
		case !inetOK:
			add(Finding{ID: "net.down", Area: "isp", Severity: "critical", Title: "Sem resposta da internet",
				Text: "Nenhum ping à Cloudflare (1.1.1.1) voltou. Confira se a internet funciona no navegador; alguns firewalls bloqueiam ping."})
		case inet.Idle.LossPct >= NetLossPct:
			// Where the loss happens decides the advice: the part already seen
			// at the router is the home's, the rest is after it.
			beyond := math.Max(inet.Idle.LossPct-homeLoss, 0)
			t := fmt.Sprintf("Perda de pacotes até a internet: %s", pct(inet.Idle.LossPct))
			var txt string
			switch {
			case !lanOK || routerDrops:
				why := "O roteador não responde ao ping"
				if routerDrops {
					why = "O roteador não responde a todos os pings"
				}
				txt = why + ", então não dá para separar a sua rede da operadora. Reinicie o modem e o roteador e teste de novo; se continuar, teste com o PC ligado direto no modem e abra um chamado com estes números, o dia e o horário."
			case lanLossy && beyond >= NetLossPct:
				txt = fmt.Sprintf("Perda em dois trechos: %s já até o roteador (veja a rede local acima) e cerca de %s a mais depois dele, no modem/ONT ou na operadora. Resolva a parte de casa primeiro (%s) e teste de novo; se a de fora continuar, reinicie o modem e abra um chamado com estes números.", pct(homeLoss), pct(beyond), homeFix)
			case beyond < NetLossPct:
				txt = fmt.Sprintf("A maior parte já acontece dentro de casa: até o roteador a perda foi de %s. Resolva esse trecho primeiro (%s) e teste de novo.", pct(homeLoss), homeFix)
			default:
				t = fmt.Sprintf("Perda de pacotes depois do seu roteador: %s até a internet", pct(inet.Idle.LossPct))
				home := "Até o roteador não houve perda"
				if homeLoss > 0 {
					home = "Até o roteador a perda foi de só " + pct(homeLoss)
				}
				txt = home + ": o problema está no modem/ONT ou na operadora. Reinicie o modem; se continuar, abra um chamado com estes números, o dia e o horário."
			}
			add(Finding{ID: "net.loss", Area: "isp", Severity: "warning", Title: t, Text: txt})
		case inet.Idle.Jitter >= JitterMs:
			add(Finding{ID: "net.jitter", Area: "isp", Severity: "info", Title: fmt.Sprintf("Latência oscilando até a internet (jitter de %s ms)", num(inet.Idle.Jitter)),
				Text: "Com a conexão parada o ping não deveria variar tanto. Veja se algo da casa está usando a internet durante o teste e repita em outro horário."})
		default:
			txt := "Sem perda de pacotes com a conexão parada."
			if inet.Idle.LossPct > 0 {
				txt = fmt.Sprintf("Perda de %s com a conexão parada, abaixo do que se nota em jogo.", pct(inet.Idle.LossPct))
			}
			add(Finding{ID: "net.ok", Area: "isp", Severity: "good", Title: fmt.Sprintf("Internet estável: %s ms até a Cloudflare, jitter de %s ms", num(inet.Idle.Median), num(inet.Idle.Jitter)),
				Text: txt})
		}
	}

	// Traffic to the internet leaves through a VPN or virtual adapter.
	if r.Adapter.Via != "" {
		add(Finding{ID: "path.via", Area: "path", Severity: "info", Title: "A internet está passando por outra conexão (" + r.Adapter.Via + ")",
			Text: "O Windows manda o tráfego para a internet por esse adaptador (VPN ou adaptador virtual), então o ping até a internet e até a Valve inclui esse caminho. A rede local foi medida pela placa " + orDash(r.Adapter.Description) + ". Para medir a sua conexão, desligue a VPN e teste de novo."})
	}

	// Valve relays.
	if valve != nil {
		switch {
		case valve.Idle.LossPct >= NetLossPct:
			// base: the loss the path shared with the internet already explains.
			base := homeLoss
			if inetOK && inet.Idle.LossPct > base {
				base = inet.Idle.LossPct
			}
			own := valve.Idle.LossPct - base
			var src string
			switch {
			case !inetOK:
				src = "A Cloudflare não respondeu ao ping, então não dá para comparar com o resto da internet."
			case own < NetLossPct && lanLossy:
				src = "A perda já começa dentro de casa (veja a rede local acima): não é da Valve."
			case own < NetLossPct:
				src = "A perda aparece também até a Cloudflare: é da sua conexão, não da Valve."
			case base >= WiredLossPct:
				src = fmt.Sprintf("Uma parte (%s) aparece também até a Cloudflare; o resto, cerca de %s, só na rota até a Valve.", pct(base), pct(own))
			default:
				src = "Só até a Valve: é a rota da operadora até ela."
			}
			add(Finding{ID: "valve.loss", Area: "valve", Severity: "warning", Title: fmt.Sprintf("Perda de %s até o relay da Valve em %s", pct(valve.Idle.LossPct), valve.Name),
				Text: "No CS2 isso aparece como engasgos e tiros que não registram. " + src + " Confirme em jogo com a telemetria do CS2 (Configurações → Jogo → Telemetria), que mostra a perda dos pacotes do próprio jogo.", Actions: []Action{{Kind: "page", Label: "Ver o CS2", Page: "cs2"}}})
		case valve.Idle.Jitter >= JitterMs:
			add(Finding{ID: "valve.jitter", Area: "valve", Severity: "info", Title: fmt.Sprintf("Oscilação até o relay da Valve em %s (jitter de %s ms)", valve.Name, num(valve.Idle.Jitter)),
				Text: "Variação alta deixa o movimento dos outros jogadores menos fluido. Se ela se repetir em outros testes, o buffer de 1 pacote do CS2 pode suavizar (veja abaixo)."})
		}
		if inetOK && valve.Idle.OK() {
			extra := valve.Idle.Median - inet.Idle.Median
			switch {
			case extra <= 12 && valve.Idle.Median <= 40:
				add(Finding{ID: "valve.route", Area: "valve", Severity: "good", Title: fmt.Sprintf("Rota curta até a Valve: %s ms até o relay de %s", num(valve.Idle.Median), valve.Name),
					Text:    fmt.Sprintf("Só %s ms a mais que até a Cloudflare. No CS2 o seu tráfego entra na rede da Valve pelo relay mais próximo; programas de \"redução de ping\" (GPN) só ajudam quando a rota da operadora até esse relay é ruim, o que não é o seu caso.", num(math.Max(extra, 0))),
					Actions: []Action{{Kind: "url", Label: "Steam Datagram Relay", URL: SrcSDR.URL}}})
			case extra > 25:
				add(Finding{ID: "valve.route", Area: "valve", Severity: "info", Title: fmt.Sprintf("A rota até a Valve soma %s ms", num(extra)),
					Text: fmt.Sprintf("O relay mais próximo (%s) responde em %s ms, contra %s ms da Cloudflare. Pode ser distância (não há relay perto de você) ou rota ruim da operadora. Se o placar do CS2 mostra um ping parecido, é a distância; um GPN só compensa se medir menos com ele — teste por alguns dias e compare aqui.", valve.Name, num(valve.Idle.Median), num(inet.Idle.Median))})
			}
		}
	}

	// Bufferbloat.
	if b := r.Bloat; b != nil {
		if b.Grade == "" && b.Partial {
			add(partialBloat(b))
		} else {
			for _, p := range b.Phases {
				if p.Error != "" {
					add(Finding{ID: "bloat.err." + p.Dir, Area: "bloat", Severity: "info", Title: "Parte do teste de carga falhou (" + dirPT(p.Dir) + ")", Text: p.Error})
				}
			}
		}
		if b.Grade != "" {
			worst := "download"
			if pu, pd := b.Phase("upload"), b.Phase("download"); pu != nil && (pd == nil || pu.AddedMs > pd.AddedMs) {
				worst = "upload"
			}
			qos := qosAdvice(b)
			switch p := b.Phase(worst); {
			case p != nil && p.NoReply:
				add(Finding{ID: "bloat.bad", Area: "bloat", Severity: "warning", Title: "Bufferbloat extremo: com a internet em uso o ping não voltou em 2 s (nota " + b.Grade + ")",
					Text:    "No " + dirPT(worst) + " a fila do modem ou do roteador encheu tanto que nenhum ping voltou a tempo: durante um download ou envio pesado, o jogo fica praticamente sem conexão. " + qos,
					Actions: bloatActions()})
			case b.Grade == "A+" || b.Grade == "A":
				add(Finding{ID: "bloat.ok", Area: "bloat", Severity: "good", Title: fmt.Sprintf("Sem bufferbloat: nota %s (+%s ms com a internet em uso)", b.Grade, num(math.Max(b.AddedMs, 0))),
					Text: "Downloads e envios pesados quase não mexem no seu ping."})
			case b.Grade == "B":
				add(Finding{ID: "bloat.mild", Area: "bloat", Severity: "info", Title: fmt.Sprintf("Bufferbloat leve: nota B, +%s ms no %s", num(b.AddedMs), dirPT(worst)),
					Text:    "Com a internet cheia (Steam baixando, alguém vendo vídeo, backup na nuvem) seu ping sobe esse tanto. Se isso acontece durante as partidas: " + qos,
					Actions: bloatActions()})
			default:
				add(Finding{ID: "bloat.bad", Area: "bloat", Severity: "warning", Title: fmt.Sprintf("Bufferbloat: o ping sobe +%s ms com a internet em uso (nota %s)", num(b.AddedMs), b.Grade),
					Text:    "O pior é no " + dirPT(worst) + ": a fila do modem ou do roteador enche e todo pacote, inclusive o do jogo, espera nela. " + qos,
					Actions: bloatActions()})
			}
		}
	}

	// Path: NAT layers.
	if r.Path.DoubleNAT {
		add(Finding{ID: "path.doublenat", Area: "path", Severity: "info", Title: "Dois roteadores em cascata (NAT duplo)",
			Text: "O PC passa por dois roteadores com rede privada antes da internet (" + strings.Join(homeHops(r.Path), " → ") + "). No CS2 isso não muda o ping, porque a Valve liga todos pelos relays; pode atrapalhar UPnP e jogos que precisam de portas abertas. Para simplificar: modem da operadora em modo bridge, ou o segundo roteador em modo Access Point."})
	}
	if r.Path.CGNAT {
		add(Finding{ID: "path.cgnat", Area: "path", Severity: "info", Title: "A operadora usa CGNAT",
			Text:    "Você divide um endereço público com outros clientes (faixa 100.64.0.0/10). Não aumenta o ping no CS2; só impede abrir portas para hospedar servidores. Se precisar, peça IP público à operadora.",
			Actions: []Action{{Kind: "url", Label: "RFC 6598", URL: SrcCGNAT.URL}}})
	}

	// DNS.
	if f, ok := dnsFinding(r.DNS, r.Adapter); ok {
		add(f)
	}

	// CS2 settings.
	if r.CS2.Found {
		for _, f := range cs2Findings(r.CS2, valve) {
			add(f)
		}
	}

	// Network adapter.
	if wired && r.Adapter.SpeedDuplex != "" && r.Adapter.SpeedDuplex != "0" {
		sev, txt := "info", "Não reduz o ping: a norma do gigabit por cabo exige autonegociação, então fixar só tira a adaptação do link (com um cabo ruim, a conexão cai em vez de continuar a 100 Mb/s). O padrão do driver é o automático."
		if v := r.Adapter.SpeedDuplex; v >= "1" && v <= "4" && len(v) == 1 {
			sev, txt = "warning", "Fixada em 10 ou 100 Mb/s, a placa limita a velocidade da internet. Volte para o automático."
		}
		add(Finding{ID: "nic.speed", Area: "nic", Severity: sev, Title: "Velocidade da placa de rede fixada (" + speedLabel(r.Adapter.SpeedDuplex) + ")", Text: txt,
			Actions: []Action{{Kind: "tweaks", Label: "Voltar ao automático", Tweaks: []string{"fix.nic.speed"}}}})
	}
	if wired && r.Adapter.EEE == "1" {
		add(Finding{ID: "nic.eee", Area: "nic", Severity: "info", Title: "Economia de energia da placa de rede (EEE) ligada",
			Text:    "O EEE (IEEE 802.3az) desliga o link entre um pacote e outro. Acordar leva cerca de 16,5 µs no gigabit, nada perto da latência da internet; o motivo para desligar é compatibilidade: a própria Intel desativou o EEE nas placas I225/I226 para acabar com quedas e travadas. Num desktop, desligar não tem desvantagem além de um consumo um pouco maior com a rede parada.",
			Actions: []Action{{Kind: "tweaks", Label: "Desligar EEE", Tweaks: []string{"net.nic.eee"}}}})
	}

	sort.SliceStable(fs, func(i, j int) bool { return sevRank(fs[i].Severity) < sevRank(fs[j].Severity) })
	r.Findings = fs

	// Cloudflare's AIM class for gaming. It weighs the latency added under
	// load, so without a graded load test there is no class to give.
	r.AIM = nil
	if inet != nil && inet.Idle.OK() && r.Bloat != nil && r.Bloat.Grade != "" {
		loss := inet.Idle.LossPct / 100
		if valve != nil && valve.Idle.LossPct/100 > loss {
			loss = valve.Idle.LossPct / 100
		}
		a := AIMGaming(inet.Idle.Median, loss, math.Max(r.Bloat.AddedMs, 0))
		r.AIM = &a
	}
}

// partialBloat is the verdict of a load test where one direction failed. The
// direction measured cannot vouch for the other one, so there is no grade;
// but when it already shows bufferbloat, that is a real problem whatever the
// other direction would show.
func partialBloat(b *Bloat) Finding {
	var ok, failed *LoadPhase
	for i := range b.Phases {
		p := &b.Phases[i]
		switch {
		case p.Measured() && (ok == nil || p.AddedMs > ok.AddedMs):
			ok = p
		case p.Error != "" && failed == nil:
			failed = p
		}
	}
	missing, reason := "a outra direção", ""
	if failed != nil {
		missing = "o " + dirName(failed.Dir)
		reason = " Motivo da falha: " + strings.TrimRight(strings.TrimSpace(failed.Error), ".") + "."
	}
	if ok == nil {
		return Finding{ID: "bloat.partial", Area: "bloat", Severity: "info", Title: "Teste de bufferbloat incompleto", Text: "Nenhuma direção foi medida." + reason}
	}
	added := math.Max(ok.AddedMs, 0)
	if g := BloatGrade(added); g != "A+" && g != "A" {
		sev := "warning"
		if g == "B" {
			sev = "info"
		}
		return Finding{ID: "bloat.partial", Area: "bloat", Severity: sev,
			Title: fmt.Sprintf("Bufferbloat no %s: o ping sobe +%s ms (teste incompleto)", dirName(ok.Dir), num(added)),
			Text: fmt.Sprintf("Só o %s foi medido (%s falhou), e ele já mostra a fila do modem ou do roteador enchendo: a nota completa seria %s ou pior.%s Para resolver: %s",
				dirName(ok.Dir), missing, g, reason, qosAdvice(b)),
			Actions: bloatActions()}
	}
	return Finding{ID: "bloat.partial", Area: "bloat", Severity: "info",
		Title: fmt.Sprintf("Teste de bufferbloat incompleto: %s não foi medido", missing),
		Text: fmt.Sprintf("No %s o ping subiu só +%s ms, mas isso não diz nada sobre %s: a nota só sai com as duas direções medidas.%s Repita o teste em alguns minutos.",
			dirName(ok.Dir), num(added), missing, reason)}
}

// dirName is the direction without the explanation in parentheses.
func dirName(d string) string {
	if d == "upload" {
		return "upload"
	}
	return "download"
}

func qosAdvice(b *Bloat) string {
	down, up := -1.0, -1.0
	if p := b.Phase("download"); p != nil && p.Error == "" {
		down = p.Mbps
	}
	if p := b.Phase("upload"); p != nil && p.Error == "" {
		up = p.Mbps
	}
	speeds := ""
	if down > 0 && up > 0 {
		speeds = fmt.Sprintf(" com download de %s Mb/s e upload de %s Mb/s (o que este teste mediu)", mbps(down), mbps(up))
	}
	return "ative o QoS (ou o SQM) do roteador" + speeds + " e repita o teste; se a nota não melhorar, reduza os dois para 90% e teste de novo. " +
		"Nos roteadores TP-Link fica em Avançado > QoS (ou HomeShield > QoS no app Tether). O SQM (fq_codel ou CAKE) é a solução mais eficaz; o QoS comum dos roteadores ajuda menos. Sem nenhum dos dois, evite downloads pesados durante as partidas."
}

func bloatActions() []Action {
	return []Action{{Kind: "url", Label: "Como resolver (bufferbloat.net)", URL: SrcBufferbloatFix.URL}, {Kind: "url", Label: "QoS nos roteadores TP-Link", URL: SrcTPLinkQoS.URL}}
}

func homeHops(p Path) []string {
	var out []string
	for _, h := range p.Hops {
		if h.Kind == KindPrivate && h.Addr != "" && (p.PublicTTL == 0 || h.TTL < p.PublicTTL) {
			out = append(out, h.Addr)
		}
	}
	return out
}

func dnsFinding(list []DNSServer, ad AdapterInfo) (Finding, bool) {
	var sys *DNSServer
	var best *DNSServer
	for i := range list {
		d := &list[i]
		if d.System && sys == nil {
			sys = d
		}
		if !d.System && d.Failures == 0 && d.Queries > 0 && (best == nil || d.Median < best.Median) {
			best = d
		}
	}
	if sys == nil || sys.Queries == 0 {
		return Finding{}, false
	}
	note := " O DNS só traduz nomes em endereços antes de conectar: não muda o ping durante a partida."
	switch {
	case sys.Failures*5 >= sys.Queries && sys.Failures > 0:
		t := fmt.Sprintf("O DNS configurado falhou em %d de %d consultas", sys.Failures, sys.Queries)
		txt := "Sites e o login da Steam podem demorar ou falhar."
		if best != nil {
			txt += fmt.Sprintf(" %s (%s) respondeu tudo em %s ms.", best.Name, best.Addr, num(best.Median))
		}
		f := Finding{ID: "dns.fail", Area: "dns", Severity: "warning", Title: t}
		f.Actions, f.Text = dnsAction(best, ad, txt+note)
		return f, true
	case best != nil && sys.Median > 40 && best.Median*2 < sys.Median:
		f := Finding{ID: "dns.slow", Area: "dns", Severity: "info", Title: fmt.Sprintf("DNS configurado lento: %s ms por consulta", num(sys.Median))}
		f.Actions, f.Text = dnsAction(best, ad, fmt.Sprintf("%s (%s) respondeu em %s ms. Trocar deixa a abertura de sites e o login mais rápidos.", best.Name, best.Addr, num(best.Median))+note)
		return f, true
	}
	return Finding{ID: "dns.ok", Area: "dns", Severity: "good", Title: fmt.Sprintf("DNS ok: %s ms por consulta", num(sys.Median)),
		Text: "Trocar de DNS não traria ganho perceptível." + note}, true
}

// dnsAction offers the DNS change for the adapter (only in dns.fail and
// dns.slow, when a public resolver answered everything). It returns the
// actions and the finding text, which says why the change is not offered when
// the adapter is one Axion must not touch.
func dnsAction(best *DNSServer, ad AdapterInfo, text string) ([]Action, string) {
	if best == nil || ad.GUID == "" || !ad.DNSRead {
		return nil, text
	}
	rs, ok := ResolverByAddr(best.Addr)
	if !ok {
		return nil, text
	}
	if ad.DoH {
		return nil, text + " Este adaptador usa DNS criptografado (DNS over HTTPS) configurado por você: o Axion não troca."
	}
	if ad.Via != "" {
		return nil, text + " A internet passa por " + ad.Via + " (VPN ou adaptador virtual): trocar o DNS da placa de rede não mudaria o DNS usado."
	}
	cur := strings.Join(ad.DNS, ", ")
	if ad.DNSAuto || cur == "" {
		cur = "automático, recebido do roteador"
	}
	servers := strings.Join(rs.V4, ", ")
	if ad.IPv6DNS {
		servers += " e " + strings.Join(rs.V6, ", ")
	}
	confirm := "O Axion vai trocar o DNS do adaptador " + ad.Name + " para o " + rs.Of + " (" + servers + "), guardar o DNS atual (" + cur + ") e limpar o cache de nomes do Windows. " +
		"Isso deixa abrir sites e o login mais rápidos quando o DNS atual falha ou demora; não muda o ping das partidas. " +
		"Nomes da rede local (impressora, NAS) ou filtros da operadora podem depender do DNS atual. Depois, refaça o teste com o rótulo \"depois\" para comparar; dá para desfazer no Histórico."
	if rs.Key == "quad9" {
		confirm += " O Quad9 bloqueia domínios que ele classifica como maliciosos (segundo o próprio serviço)."
	}
	return []Action{{Kind: "apply", Label: "Usar o DNS " + rs.Of + " neste adaptador", Tweaks: []string{DNSTweakID(ad.GUID, rs.Key, !ad.IPv6DNS)}, Confirm: confirm}}, text
}

func cs2Findings(c CS2Net, valve *Target) []Finding {
	var out []Finding
	cur := c.BufferTicks
	if cur < 0 {
		cur = 0
	}
	if valve != nil && valve.Idle.OK() {
		want := 0
		if valve.Idle.LossPct >= NetLossPct || valve.Idle.Jitter >= JitterMs {
			want = 1
		}
		setting := "Configurações > Jogo > \"Buffering to smooth over packet loss\""
		switch {
		case cur > want:
			out = append(out, Finding{ID: "cs2.buffer", Area: "cs2", Severity: "info",
				Title: fmt.Sprintf("O buffer de pacotes do CS2 soma ~%s ms sem necessidade", num(float64(cur-want)*TickMs)),
				Text: fmt.Sprintf("Está em %d pacote(s). Até a Valve a perda foi de %s e o jitter de %s ms: use %s em %s. A Valve criou essa opção para quem perde pacotes, trocando atraso por suavidade.",
					cur, pct(valve.Idle.LossPct), num(valve.Idle.Jitter), bufLabel(want), setting),
				Actions: []Action{{Kind: "url", Label: "Notas da Valve", URL: SrcCS2Buffer.URL}}})
		case cur < want:
			out = append(out, Finding{ID: "cs2.buffer", Area: "cs2", Severity: "info",
				Title:   "Sua rota até a Valve perde pacotes ou oscila: experimente o buffer de 1 pacote no CS2",
				Text:    fmt.Sprintf("Em %s, \"1 packet\" troca ~%s ms de atraso por menos engasgos quando pacotes se perdem (Valve, notas de 6/2/2024). Teste algumas partidas; se não sentir diferença, volte para \"None\".", setting, num(TickMs)),
				Actions: []Action{{Kind: "url", Label: "Notas da Valve", URL: SrcCS2Buffer.URL}}})
		default:
			out = append(out, Finding{ID: "cs2.buffer", Area: "cs2", Severity: "good",
				Title: fmt.Sprintf("Buffer de pacotes do CS2 adequado (%s)", bufLabel(cur)), Text: "Combina com a perda e a oscilação medidas até a Valve."})
		}
	}
	if c.Rate > 0 && c.Rate < CS2FullRate {
		out = append(out, Finding{ID: "cs2.rate", Area: "cs2", Severity: "info", Title: fmt.Sprintf("Banda do CS2 limitada (rate %d)", c.Rate),
			Text: "Em rodadas cheias o servidor pode ter que cortar atualizações para caber no limite. Use \"Unrestricted\" em Configurações > Jogo > \"Max Acceptable Game Traffic Bandwidth\". Servidores da comunidade com limite menor mudam esse valor sozinhos quando você entra neles."})
	}
	return out
}

// speedLabel names a *SpeedDuplex value (Microsoft's enumeration).
func speedLabel(v string) string {
	m := map[string]string{"1": "10 Mb/s half duplex", "2": "10 Mb/s full duplex", "3": "100 Mb/s half duplex", "4": "100 Mb/s full duplex",
		"5": "1 Gb/s half duplex", "6": "1 Gb/s full duplex", "7": "10 Gb/s full duplex"}
	if s, ok := m[v]; ok {
		return s
	}
	return "valor " + v
}

func bufLabel(n int) string {
	switch n {
	case 0:
		return "\"None\" (nenhum)"
	case 1:
		return "\"1 packet\""
	}
	return fmt.Sprintf("\"%d packets\"", n)
}

func dirPT(d string) string {
	if d == "upload" {
		return "upload (envio)"
	}
	return "download (recebimento)"
}

func sevRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	}
	return 3
}

// num formats ms/Mbps with a decimal comma.
func num(v float64) string {
	var s string
	switch {
	case math.Abs(v) >= 100:
		s = fmt.Sprintf("%.0f", v)
	case math.Abs(v) >= 10:
		s = fmt.Sprintf("%.1f", v)
	default:
		s = fmt.Sprintf("%.2f", v)
	}
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "" || s == "-" || s == "-0" {
		s = "0"
	}
	return strings.Replace(s, ".", ",", 1)
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func mbps(v float64) string { return strings.Replace(fmt.Sprintf("%.0f", v), ".", ",", 1) }

func pct(v float64) string { return num(v) + "%" }

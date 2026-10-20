package startup

import (
	"path/filepath"
	"strings"
)

// Verdicts.
const (
	Disable = "desativar"
	Keep    = "manter"
	Review  = "avaliar"
)

type rule struct {
	match    []string // lower-case substrings of the executable name or command
	category string
	verdict  string
	advice   string
}

// Rules are checked in order; the first match wins. Advice is honest about
// the trade-off rather than promising FPS: startup programs cost boot time,
// RAM and background CPU, not frame rate by themselves.
var rules = []rule{
	{[]string{"securityhealthsystray", "msascuil", "avgui", "avastui", "mbam.exe", "mbamtray", "avp.exe", "egui.exe", "bdagent", "nortonsecurity", "mcuicnt"}, "Segurança", Keep,
		"Ícone e avisos do antivírus. É leve e importante para você saber de ameaças."},
	{[]string{"rtkaudservice", "rtkaudioservice", "rtkaudUService", "rtkngui", "ravbg64", "ravcpl64", "nahimic", "wavesmaxxaudio", "maxxaudio", "dolby"}, "Driver de áudio", Keep,
		"Parte do driver de áudio: detecção de fone e microfone e efeitos. Desativar pode deixar o painel de áudio sem funcionar."},
	{[]string{"ctfmon", "syntpenh", "etdctrl", "synaptics", "igfxtray", "igfxhk", "hkcmd"}, "Sistema/driver", Keep,
		"Componente de entrada ou do driver de vídeo integrado. Mantenha."},
	{[]string{"lghub", "logitechg", "razer", "synapse", "steelseriesgg", "steelseries engine", "hyperx", "ngenuity", "wooting", "glorious"}, "Periférico", Review,
		"Aplica perfis de DPI, polling, macros e luzes salvos no programa. Se o mouse/teclado guarda o perfil na memória interna (modo on-board), dá para desativar e abrir só para configurar."},
	{[]string{"icue", "armourycrate", "armoury crate", "lightingservice", "auraservice", "aura service", "msi center", "dragoncenter", "mysticlight", "rgbfusion", "gcc.exe", "signalrgb", "openrgb", "nzxt cam", "fancontrol", "argusmonitor"}, "RGB e ventoinhas", Review,
		"Controla luzes e às vezes curvas de ventoinha e bomba. Consome CPU em segundo plano o tempo todo; se as configurações ficam salvas no hardware (ou na BIOS), dá para desativar."},
	{[]string{"msiafterburner", "rtss", "rivatuner"}, "Overlay/monitoramento", Review,
		"Afterburner/RTSS: se você usa limitador de FPS, curva de ventoinha ou overclock por ele, mantenha; se não, desative."},
	{[]string{"discord"}, "Comunicação", Disable,
		"Abre em segundos quando você quiser. Ligado com o Windows fica ocupando RAM e CPU em segundo plano."},
	{[]string{"msteams", "teams.exe", "\\teams\\", "skype", "zoom.exe", "slack", "whatsapp", "telegram"}, "Comunicação", Disable,
		"Pode abrir quando for usar; com o Windows fica ocupando memória e rede em segundo plano."},
	{[]string{"steam.exe", "steamwebhelper"}, "Launcher de jogos", Disable,
		"O Steam abre sozinho quando você inicia um jogo dele. Iniciar com o Windows só adianta login e atualizações."},
	{[]string{"epicgameslauncher", "eadesktop", "eabackground", "origin.exe", "ubisoftconnect", "upc.exe", "battle.net", "galaxyclient", "riotclientservices", "rockstar", "bethesda", "amazon games", "playnite"}, "Launcher de jogos", Disable,
		"Launcher que pode abrir quando você for jogar. Com o Windows, fica atualizando e rodando em segundo plano."},
	{[]string{"spotify", "itunes", "deezer", "tidal", "wallpaper32", "wallpaper64", "wallpaperengine"}, "Mídia", Disable,
		"Pode abrir quando quiser usar; não precisa iniciar com o Windows."},
	{[]string{"onedrive", "dropbox", "googledrivefs", "google drive", "icloud", "megasync"}, "Nuvem", Review,
		"Sincroniza arquivos com a nuvem. Se você usa, mantenha; se não usa, desative."},
	{[]string{"msedge.exe", "chrome.exe", "brave.exe", "opera gx", "\\opera\\", "firefox.exe", "vivaldi"}, "Navegador", Disable,
		"Deixa o navegador pré-carregado para abrir mais rápido. Custa memória desde o boot."},
	{[]string{"googleupdate", "microsoftedgeupdate", "adobearm", "adobegc", "acrotray", "jusched", "javaupdate", "itunesupdater", "softwareupdate", "ccleaner", "driverbooster", "iobit"}, "Atualizador", Disable,
		"Atualizador em segundo plano. O programa continua se atualizando quando é aberto (ou pelo agendamento próprio)."},
	{[]string{"anydesk", "teamviewer", "parsec", "rustdesk"}, "Acesso remoto", Review,
		"Acesso remoto ao PC. Se você não usa com frequência, desative: além de memória, é uma porta a menos aberta."},
}

func classify(it *Item) {
	name := strings.ToLower(filepath.Base(strings.ReplaceAll(it.Exe, `\`, "/")))
	hay := name + " " + strings.ToLower(it.Command) + " " + strings.ToLower(it.Name)
	if it.Missing {
		it.Category, it.Verdict = "Entrada quebrada", Disable
		it.Advice = "O programa não existe mais neste caminho: a entrada sobrou de uma desinstalação e não faz nada."
		return
	}
	for _, r := range rules {
		for _, m := range r.match {
			if strings.Contains(hay, strings.ToLower(m)) {
				it.Category, it.Verdict, it.Advice = r.category, r.verdict, r.advice
				return
			}
		}
	}
	it.Category, it.Verdict = "Outro", Review
	it.Advice = "Programa de terceiro. Se não precisa dele aberto o tempo todo, desative e abra quando for usar; dá para ligar de novo aqui."
}

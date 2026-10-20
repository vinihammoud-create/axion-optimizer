// Package apps is the catalog of programs the Axion installs in one click
// through winget, the package manager that ships with Windows 10 and 11
// (0.8): after a format, or at any time, the user marks what to install
// and each item comes in its newest version from Microsoft's catalog. No
// installer travels inside the Axion: an item is a winget identifier, a
// Microsoft Store product or a link to the maker's page.
package apps

import (
	"regexp"
	"strings"
)

// Category keys in display order.
var Categories = []Category{
	{ID: "base", Title: "Base do sistema", Note: "Jogos e programas antigos quebram sem estes componentes, e o Windows limpo não os traz todos: os quatro primeiros vêm marcados em todo perfil; o Java só para quem precisa dele."},
	{ID: "navegador", Title: "Navegadores", Note: "O Edge já vem no Windows. Todos abaixo são gratuitos; a primeira linha é a opção recomendada."},
	{ID: "jogos", Title: "Jogos e launchers", Note: ""},
	{ID: "perifericos", Title: "Periféricos, placa de vídeo e monitoramento", Note: "O software do mouse ou do teclado só vem marcado quando o Axion reconhece o fabricante (e, na Logitech e na HyperX, a linha gamer) pelo identificador USB e pelo nome do dispositivo; headsets não entram nessa leitura. O driver de vídeo continua vindo da página Drivers (com o link exato para a sua placa)."},
	{ID: "live", Title: "Live, vídeo, áudio e imagem", Note: ""},
	{ID: "comunicacao", Title: "Comunicação", Note: ""},
	{ID: "trabalho", Title: "Trabalho, escritório e estudo", Note: "Programas pagos vêm marcados como pagos e exigem a sua licença; o Axion instala o aplicativo, não a licença."},
	{ID: "dev", Title: "Programação", Note: "Docker Desktop e WSL 2 ligam o hipervisor do Windows (Plataforma de Máquina Virtual): o Windows passa a rodar sobre ele, o que pode custar um pouco de FPS em alguns jogos, e o ajuste avançado de desligar o VBS deixa de fazer diferença. Instale só se for usar."},
	{ID: "seguranca", Title: "Segurança, senhas e acesso remoto", Note: "O Defender do Windows é o antivírus recomendado; o catálogo não traz outro antivírus (os \"de segunda opinião\" assumem a proteção em tempo real no período de teste e desligam a do Defender)."},
}

// Category of the catalog.
type Category struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Note  string `json:"note,omitempty"`
}

// Item is one program of the catalog.
type Item struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	// Winget is the package identifier in Microsoft's catalog ("" when the
	// program is not there); Store is a Microsoft Store product id (msstore
	// source); Manual is the maker's page for what neither has.
	Winget string `json:"winget,omitempty"`
	Store  string `json:"store,omitempty"`
	Manual string `json:"manual,omitempty"`
	For    string `json:"for"`
	Note   string `json:"note,omitempty"`
	Paid   bool   `json:"paid,omitempty"`
	// Verified: the identifier was checked in the winget catalog (04/10/2026).
	Verified bool `json:"verified"`
	// Recommended: the first option of its category (the alternatives are
	// the others).
	Recommended bool `json:"recommended,omitempty"`
	// Profiles where the item comes checked by default; Suggested where it
	// is listed as a suggestion (unchecked).
	Profiles  []string `json:"profiles,omitempty"`
	Suggested []string `json:"suggested,omitempty"`
	// Vendors are USB vendor ids of peripherals that make the item come
	// checked (the maker's software); Laptop marks items for notebooks of
	// the given maker (lower-case fragment of the system manufacturer).
	Vendors []uint16 `json:"-"`
	// Models narrows Vendors to a product line: lower-case fragments of the
	// device name ("g###" stands for a Logitech G model number); PIDs are
	// product ids of that line (its wireless receivers, whose name Windows
	// shows as a generic "USB Receiver"). With both empty, every device of
	// the vendor counts.
	Models []string `json:"-"`
	PIDs   []uint16 `json:"-"`
	Laptop string   `json:"-"`
	// Names are lower-case fragments of the program's name in the installed
	// programs list, to tell it is installed when winget does not know it.
	Names []string `json:"-"`
	// Heavy: runs in the background with a cost; the note says so.
	Heavy bool `json:"heavy,omitempty"`
}

// Profile IDs (the same as the profile package; copied so this package
// stays free of dependencies).
const (
	Competitive = "competitivo"
	Live        = "live"
	Work        = "trabalho"
	Creation    = "criacao"
	Dev         = "programacao"
)

var all = []string{Competitive, Live, Work, Creation, Dev}
var gaming = []string{Competitive, Live}

// All is the catalog in display order (recommended item first in each
// category).
var All = []Item{
	// Base
	{ID: "vcredist", Name: "Visual C++ 2015–2022 (x64)", Category: "base", Winget: "Microsoft.VCRedist.2015+.x64", Verified: true, Recommended: true, Profiles: all,
		For: "Exigido por quase todo jogo e programa nativo de 64 bits", Note: "A versão x86 é o item seguinte, para programas de 32 bits.", Names: []string{"microsoft visual c++ 2015-2022 redistributable (x64)"}},
	{ID: "vcredist86", Name: "Visual C++ 2015–2022 (x86)", Category: "base", Winget: "Microsoft.VCRedist.2015+.x86", Verified: true, Profiles: all,
		For: "Programas de 32 bits", Names: []string{"microsoft visual c++ 2015-2022 redistributable (x86)"}},
	{ID: "dotnet8", Name: ".NET Desktop Runtime 8", Category: "base", Winget: "Microsoft.DotNet.DesktopRuntime.8", Verified: true, Profiles: all,
		For: "Exigido por vários utilitários (CapFrameX, ferramentas de periféricos)", Names: []string{"microsoft windows desktop runtime - 8."}},
	{ID: "directx", Name: "DirectX End-User Runtime", Category: "base", Winget: "Microsoft.DirectX", Verified: true, Profiles: all,
		For: "Bibliotecas antigas (d3dx9, xinput) que jogos mais velhos ainda pedem", Note: "O Windows 11 não as inclui."},
	{ID: "java", Name: "Java (Temurin JRE 21)", Category: "base", Winget: "EclipseAdoptium.Temurin.21.JRE", Verified: true,
		For: "Só para quem usa Minecraft Java ou programas Java", Names: []string{"eclipse temurin jre"}},
	// Browsers
	{ID: "brave", Name: "Brave", Category: "navegador", Winget: "Brave.Brave", Verified: true, Recommended: true, Profiles: gaming,
		For: "Bloqueio de anúncios e rastreadores sem extensão; base Chromium", Note: "Desative as recompensas e a carteira cripto no primeiro uso se não usar.", Names: []string{"brave"}},
	{ID: "chrome", Name: "Google Chrome", Category: "navegador", Winget: "Google.Chrome", Verified: true, Profiles: []string{Work, Creation, Dev},
		For: "Quem vive na conta Google (favoritos, senhas e extensões sincronizados)", Note: "Mais memória em segundo plano com muitas abas.", Names: []string{"google chrome"}},
	{ID: "firefox", Name: "Mozilla Firefox", Category: "navegador", Winget: "Mozilla.Firefox", Verified: true,
		For: "Motor próprio (não Chromium) e privacidade", Note: "Versão pt-BR disponível no catálogo.", Names: []string{"mozilla firefox"}},
	{ID: "vivaldi", Name: "Vivaldi", Category: "navegador", Winget: "Vivaldi.Vivaldi", Verified: true, For: "Quem personaliza muito (abas em grupos, gestos)", Names: []string{"vivaldi"}},
	{ID: "operagx", Name: "Opera GX", Category: "navegador", Winget: "Opera.OperaGX", Verified: true,
		For: "Visual gamer e limitador de RAM/CPU embutido", Note: "O limitador não dá FPS; serve só para o navegador não atrapalhar.", Names: []string{"opera gx"}},
	// Games
	{ID: "steam", Name: "Steam", Category: "jogos", Winget: "Valve.Steam", Verified: true, Recommended: true, Profiles: gaming,
		For: "Todo mundo: o CS2 vive aqui", Note: "Depois de instalar, o Axion lê a biblioteca e as opções de inicialização do CS2.", Names: []string{"steam"}},
	{ID: "discord", Name: "Discord", Category: "jogos", Winget: "Discord.Discord", Verified: true, Profiles: gaming,
		For: "Voz e comunidade", Note: "Overlay e aceleração de hardware usam a GPU em segundo plano; o Axion mostra isso em Processos.", Names: []string{"discord"}},
	{ID: "faceit", Name: "FACEIT", Category: "jogos", Manual: "https://www.faceit.com/", Suggested: gaming,
		For: "Competitivo com anti-cheat próprio", Note: "Não está no winget. Exige TPM 2.0 e Secure Boot, que o Axion confere no diagnóstico.", Names: []string{"faceit"}},
	{ID: "gamersclub", Name: "Gamers Club", Category: "jogos", Manual: "https://gamersclub.com.br/", Suggested: gaming,
		For: "Competitivo brasileiro", Note: "Não está no winget. Secure Boot obrigatório.", Names: []string{"gamersclub", "gamers club"}},
	{ID: "epic", Name: "Epic Games Launcher", Category: "jogos", Winget: "EpicGames.EpicGamesLauncher", Verified: true, Suggested: gaming, For: "Jogos da Epic e os grátis semanais", Names: []string{"epic games launcher"}},
	{ID: "riot", Name: "Riot Client (Valorant, LoL)", Category: "jogos", Manual: "https://www.riotgames.com/", Suggested: gaming,
		For: "Jogos da Riot", Note: "O instalador é por região; o Vanguard exige TPM 2.0 e Secure Boot no Windows 11.", Names: []string{"riot client", "riot vanguard"}},
	{ID: "ubisoft", Name: "Ubisoft Connect", Category: "jogos", Winget: "Ubisoft.Connect", Verified: true, For: "Jogos da Ubisoft", Names: []string{"ubisoft connect"}},
	{ID: "ea", Name: "EA app", Category: "jogos", Winget: "ElectronicArts.EADesktop", Verified: true, For: "Jogos da EA (Battlefield, EA FC)", Names: []string{"ea app"}},
	{ID: "gog", Name: "GOG Galaxy", Category: "jogos", Winget: "GOG.Galaxy", Verified: true, For: "Jogos sem DRM e clássicos", Names: []string{"gog galaxy"}},
	{ID: "battlenet", Name: "Battle.net", Category: "jogos", Winget: "Blizzard.BattleNet", Verified: true, For: "Jogos da Blizzard e Call of Duty", Names: []string{"battle.net"}},
	{ID: "moonlight", Name: "Moonlight", Category: "jogos", Winget: "MoonlightGameStreamingProject.Moonlight", Verified: true, Suggested: gaming,
		For: "Jogar no PC a partir da TV ou do notebook", Note: "Cliente; o servidor no PC é o Apollo (ClassicOldSong.Apollo) ou o Sunshine.", Names: []string{"moonlight"}},
	{ID: "capframex", Name: "CapFrameX", Category: "jogos", Winget: "CXWorld.CapFrameX", Verified: true, Suggested: gaming,
		For: "Quem compara FPS por conta própria", Note: "O Axion já mede com o PresentMon; é complemento, não requisito.", Names: []string{"capframex"}},
	// Peripherals, GPU, monitoring
	{ID: "nvidia", Name: "Driver NVIDIA (GeForce)", Category: "perifericos", Manual: "https://www.nvidia.com/drivers/", Recommended: true,
		For: "Placas GeForce", Note: "A página Drivers do Axion resolve o link exato para a sua placa e confere a assinatura do instalador; o NVIDIA App vem junto do instalador.", Names: []string{"nvidia graphics driver"}},
	{ID: "amd", Name: "Driver AMD Adrenalin", Category: "perifericos", Manual: "https://www.amd.com/en/support/download/drivers.html", For: "Placas Radeon", Note: "Não está no winget.", Names: []string{"amd software"}},
	{ID: "ghub", Name: "Logitech G HUB", Category: "perifericos", Winget: "Logitech.GHUB", Verified: true, Vendors: []uint16{0x046D}, Models: []string{"logitech g", "g###", "g pro", "pro x", "pro wireless", "pro superlight", "lightspeed", "gaming mouse", "gaming keyboard"}, PIDs: []uint16{0xC539, 0xC53A, 0xC53D, 0xC53F, 0xC541, 0xC545, 0xC547},
		For: "Mouse e teclado Logitech G (os de escritório usam o Logi Options+)", Note: "DPI, polling e perfis; grava na memória do mouse.", Names: []string{"logitech g hub"}},
	{ID: "synapse", Name: "Razer Synapse", Category: "perifericos", Manual: "https://www.razer.com/synapse", Vendors: []uint16{0x1532}, For: "Periféricos Razer", Note: "Saiu do winget; link oficial.", Names: []string{"razer synapse"}},
	{ID: "steelseries", Name: "SteelSeries GG", Category: "perifericos", Winget: "SteelSeries.GG", Verified: true, Vendors: []uint16{0x1038}, For: "Periféricos SteelSeries", Names: []string{"steelseries gg"}},
	{ID: "icue", Name: "Corsair iCUE", Category: "perifericos", Winget: "Corsair.iCUE.5", Verified: true, Vendors: []uint16{0x1B1C}, Heavy: true,
		For: "Periféricos e RGB Corsair", Note: "Pesado em segundo plano; só se tiver Corsair.", Names: []string{"corsair icue"}},
	{ID: "ngenuity", Name: "HyperX NGENUITY", Category: "perifericos", Manual: "https://hyperx.com/pages/ngenuity", Vendors: []uint16{0x0951, 0x03F0}, Models: []string{"hyperx"},
		For: "Mouse, teclado e headset HyperX", Note: "App da Microsoft Store (o link abre a página oficial).", Names: []string{"ngenuity"}},
	{ID: "hwinfo", Name: "HWiNFO", Category: "perifericos", Winget: "REALiX.HWiNFO", Verified: true, Profiles: gaming,
		For: "Temperaturas, clocks e sensores", Note: "O Axion usa os sensores do Windows; o HWiNFO é para conferir por fora.", Names: []string{"hwinfo"}},
	{ID: "afterburner", Name: "MSI Afterburner", Category: "perifericos", Winget: "Guru3D.Afterburner", Verified: true,
		For: "Overlay de FPS e temperaturas no jogo", Note: "Overclock fica fora do plano; só o overlay.", Names: []string{"msi afterburner"}},
	{ID: "crystaldiskinfo", Name: "CrystalDiskInfo", Category: "perifericos", Winget: "CrystalDewWorld.CrystalDiskInfo", Verified: true,
		For: "Saúde do SSD", Note: "Útil quando o driver do fabricante bloqueia a leitura do SMART no Axion.", Names: []string{"crystaldiskinfo"}},
	{ID: "dellupdate", Name: "Dell Update / Dell Command Update", Category: "perifericos", Manual: "https://www.dell.com/support/home/", Laptop: "dell",
		For: "Notebook Dell: BIOS e drivers do fabricante", Names: []string{"dell update", "dell command | update"}},
	{ID: "lenovoupdate", Name: "Lenovo System Update", Category: "perifericos", Manual: "https://support.lenovo.com/solutions/ht003029", Laptop: "lenovo", For: "Notebook Lenovo: BIOS e drivers do fabricante", Names: []string{"lenovo system update"}},
	{ID: "myasus", Name: "MyASUS", Category: "perifericos", Manual: "https://www.asus.com/support/myasus/", Laptop: "asus", For: "Notebook ASUS: BIOS e drivers do fabricante", Names: []string{"myasus"}},
	{ID: "hpsa", Name: "HP Support Assistant", Category: "perifericos", Manual: "https://support.hp.com/hp-support-assistant", Laptop: "hp", For: "Notebook HP: BIOS e drivers do fabricante", Names: []string{"hp support assistant"}},
	// Live, video, audio, image
	{ID: "obs", Name: "OBS Studio", Category: "live", Winget: "OBSProject.OBSStudio", Verified: true, Recommended: true, Profiles: []string{Live, Creation},
		For: "Live na Twitch e no YouTube, gravação", Note: "Gratuito e de código aberto.", Names: []string{"obs studio"}},
	{ID: "vlc", Name: "VLC", Category: "live", Winget: "VideoLAN.VLC", Verified: true, Profiles: []string{Creation}, Suggested: []string{Work, Live}, For: "Tocar qualquer vídeo ou áudio", Names: []string{"vlc media player"}},
	{ID: "handbrake", Name: "HandBrake", Category: "live", Winget: "HandBrake.HandBrake", Verified: true, Profiles: []string{Creation}, Suggested: []string{Live},
		For: "Converter e comprimir vídeo (gravações do OBS)", Note: "Usa o NVENC.", Names: []string{"handbrake"}},
	{ID: "resolve", Name: "DaVinci Resolve", Category: "live", Manual: "https://www.blackmagicdesign.com/products/davinciresolve", Suggested: []string{Creation},
		For: "Edição de vídeo profissional, grátis", Note: "Exige cadastro no site; não está no winget.", Names: []string{"davinci resolve"}},
	{ID: "kdenlive", Name: "Kdenlive", Category: "live", Winget: "KDE.Kdenlive", Verified: true, Suggested: []string{Creation}, For: "Edição de vídeo gratuita, mais simples", Names: []string{"kdenlive"}},
	{ID: "audacity", Name: "Audacity", Category: "live", Winget: "Audacity.Audacity", Verified: true, Profiles: []string{Creation}, For: "Editar áudio, limpar ruído do microfone", Names: []string{"audacity"}},
	{ID: "voicemeeter", Name: "Voicemeeter", Category: "live", Winget: "VB-Audio.Voicemeeter", Verified: true, Suggested: []string{Live},
		For: "Misturar microfone, jogo e música na live", Note: "Curva de aprendizado; só para live.", Names: []string{"voicemeeter"}},
	{ID: "broadcast", Name: "NVIDIA Broadcast", Category: "live", Manual: "https://www.nvidia.com/en-us/geforce/broadcasting/broadcast-app/", Suggested: []string{Live},
		For: "Cancelar ruído do microfone com a RTX", Note: "Exige RTX.", Names: []string{"nvidia broadcast"}},
	{ID: "gimp", Name: "GIMP", Category: "live", Winget: "GIMP.GIMP", Verified: true, Profiles: []string{Creation}, For: "Edição de imagem (thumbnails)", Names: []string{"gimp"}},
	{ID: "krita", Name: "Krita", Category: "live", Winget: "KDE.Krita", Verified: true, Suggested: []string{Creation}, For: "Desenho e pintura digital", Names: []string{"krita"}},
	{ID: "blender", Name: "Blender", Category: "live", Winget: "BlenderFoundation.Blender", Verified: true, Profiles: []string{Creation}, For: "3D e animação", Names: []string{"blender"}},
	{ID: "sharex", Name: "ShareX", Category: "live", Winget: "ShareX.ShareX", Verified: true, Suggested: []string{Live, Creation}, For: "Capturas de tela e GIFs", Names: []string{"sharex"}},
	{ID: "spotify", Name: "Spotify", Category: "live", Store: "9NCBCSZSJRSB", Verified: true, Profiles: []string{Live, Creation},
		For: "Música", Note: "Versão da Microsoft Store: o instalador do site recusa rodar como administrador, e o Axion roda elevado. Na live, atenção aos direitos autorais da Twitch.", Names: []string{"spotify"}},
	// Communication
	{ID: "whatsapp", Name: "WhatsApp", Category: "comunicacao", Store: "9NKSQGP7F2NH", Verified: true, Recommended: true, Profiles: []string{Work},
		For: "Todo mundo no Brasil", Note: "Versão nativa da Microsoft Store, não a web.", Names: []string{"whatsapp"}},
	{ID: "telegram", Name: "Telegram Desktop", Category: "comunicacao", Winget: "Telegram.TelegramDesktop", Verified: true, Suggested: []string{Work}, For: "Grupos e arquivos grandes", Names: []string{"telegram desktop"}},
	{ID: "zoom", Name: "Zoom Workplace", Category: "comunicacao", Winget: "Zoom.Zoom", Verified: true, Suggested: []string{Work}, For: "Reuniões e aulas", Names: []string{"zoom"}},
	{ID: "teams", Name: "Microsoft Teams", Category: "comunicacao", Winget: "Microsoft.Teams", Verified: true, Profiles: []string{Work},
		For: "Trabalho corporativo e Microsoft 365", Note: "O perfil de trabalho mede a rede com as metas do Teams.", Names: []string{"microsoft teams"}},
	{ID: "slack", Name: "Slack", Category: "comunicacao", Winget: "SlackTechnologies.Slack", Verified: true, For: "Equipes de trabalho", Names: []string{"slack"}},
	// Work, office, study
	{ID: "office365", Name: "Microsoft 365 (Família, Pessoal ou estudante)", Category: "trabalho", Manual: "https://www.office.com/", Recommended: true, Paid: true, Suggested: []string{Work},
		For: "Word, Excel, PowerPoint, Outlook com a sua assinatura", Note: "Entre com a sua conta e clique em \"Instalar aplicativos\": é o instalador certo para a sua licença. O pacote do winget é a edição para empresas (abaixo).", Names: []string{"microsoft 365", "microsoft office"}},
	{ID: "office", Name: "Microsoft 365 Apps para empresas (winget)", Category: "trabalho", Winget: "Microsoft.Office", Verified: true, Paid: true, Suggested: []string{Work},
		For: "Só para licença Business, E3 ou E5 da sua empresa", Note: "O pacote Microsoft.Office instala a edição \"Apps for enterprise\" (cerca de 3 GB); uma assinatura Família ou Pessoal não a ativa.", Names: []string{"microsoft 365 apps"}},
	{ID: "libreoffice", Name: "LibreOffice", Category: "trabalho", Winget: "TheDocumentFoundation.LibreOffice", Verified: true, Suggested: []string{Work},
		For: "Quem não quer assinatura", Note: "Compatível com .docx e .xlsx.", Names: []string{"libreoffice"}},
	{ID: "acrobat", Name: "Adobe Acrobat Reader", Category: "trabalho", Winget: "Adobe.Acrobat.Reader.64-bit", Verified: true, Profiles: []string{Work},
		For: "PDF com formulários e assinatura", Note: "O Edge também abre PDF.", Names: []string{"adobe acrobat"}},
	{ID: "7zip", Name: "7-Zip", Category: "trabalho", Winget: "7zip.7zip", Verified: true, Profiles: all, For: "Compactar e extrair qualquer formato", Note: "Substitui o WinRAR.", Names: []string{"7-zip"}},
	{ID: "powertoys", Name: "PowerToys", Category: "trabalho", Winget: "Microsoft.PowerToys", Verified: true, Profiles: []string{Work, Dev}, For: "Utilidades da Microsoft (PowerRename, FancyZones, seletor de cor)", Names: []string{"powertoys"}},
	{ID: "everything", Name: "Everything", Category: "trabalho", Winget: "voidtools.Everything", Verified: true, Suggested: []string{Work, Dev}, For: "Busca instantânea de arquivos", Names: []string{"everything"}},
	{ID: "notepadpp", Name: "Notepad++", Category: "trabalho", Winget: "Notepad++.Notepad++", Verified: true, Suggested: []string{Work, Dev}, For: "Editor de texto leve", Names: []string{"notepad++"}},
	{ID: "notion", Name: "Notion", Category: "trabalho", Winget: "Notion.Notion", Verified: true, Suggested: []string{Work}, For: "Notas e organização em nuvem", Note: "Grátis, com planos pagos.", Names: []string{"notion"}},
	{ID: "obsidian", Name: "Obsidian", Category: "trabalho", Winget: "Obsidian.Obsidian", Verified: true, Suggested: []string{Work},
		For: "Notas em arquivos locais (Markdown)", Note: "Bom para resumos de estudo.", Names: []string{"obsidian"}},
	{ID: "gdrive", Name: "Google Drive", Category: "trabalho", Winget: "Google.GoogleDrive", Verified: true, Profiles: []string{Work}, For: "Sincronizar arquivos com a conta Google", Names: []string{"google drive"}},
	{ID: "anki", Name: "Anki", Category: "trabalho", Winget: "Anki.Anki", Verified: true, Suggested: []string{Work}, For: "Flashcards com repetição espaçada (residência, provas)", Names: []string{"anki"}},
	{ID: "zotero", Name: "Zotero", Category: "trabalho", Winget: "DigitalScholar.Zotero", Verified: true, Suggested: []string{Work},
		For: "Referências bibliográficas e PDFs de artigos", Note: "Integra com Word e LibreOffice.", Names: []string{"zotero"}},
	{ID: "calibre", Name: "calibre", Category: "trabalho", Winget: "calibre.calibre", Verified: true, Suggested: []string{Work}, For: "Biblioteca de e-books e PDFs", Names: []string{"calibre"}},
	// Programming
	{ID: "vscode", Name: "Visual Studio Code", Category: "dev", Winget: "Microsoft.VisualStudioCode", Verified: true, Recommended: true, Profiles: []string{Dev}, For: "Editor para qualquer linguagem", Names: []string{"microsoft visual studio code"}},
	{ID: "git", Name: "Git", Category: "dev", Winget: "Git.Git", Verified: true, Profiles: []string{Dev}, For: "Controle de versão", Names: []string{"git"}},
	{ID: "python", Name: "Python 3.13", Category: "dev", Winget: "Python.Python.3.13", Verified: true, Profiles: []string{Dev}, For: "Scripts, dados, automação", Names: []string{"python 3.13"}},
	{ID: "node", Name: "Node.js LTS", Category: "dev", Winget: "OpenJS.NodeJS.LTS", Verified: true, Profiles: []string{Dev}, For: "Web e ferramentas JavaScript", Names: []string{"node.js"}},
	{ID: "terminal", Name: "Windows Terminal", Category: "dev", Winget: "Microsoft.WindowsTerminal", Verified: true, Profiles: []string{Dev}, For: "Terminal moderno", Note: "Já vem no Windows 11; o winget atualiza.", Names: []string{"windows terminal"}},
	{ID: "pwsh", Name: "PowerShell 7", Category: "dev", Winget: "Microsoft.PowerShell", Verified: true, Profiles: []string{Dev}, For: "Shell atual (o Windows traz o 5.1)", Names: []string{"powershell 7"}},
	{ID: "docker", Name: "Docker Desktop", Category: "dev", Winget: "Docker.DockerDesktop", Verified: true, Suggested: []string{Dev},
		For: "Contêineres", Note: "Liga o WSL2 e o hipervisor; o VBS fica ativo.", Names: []string{"docker desktop"}},
	// Security, passwords, remote
	{ID: "bitwarden", Name: "Bitwarden", Category: "seguranca", Winget: "Bitwarden.Bitwarden", Verified: true, Recommended: true, Profiles: []string{Work}, Suggested: gaming,
		For: "Gerenciador de senhas gratuito e de código aberto", Names: []string{"bitwarden"}},
	{ID: "parsec", Name: "Parsec", Category: "seguranca", Winget: "Parsec.Parsec", Verified: true, For: "Acessar o PC de longe com baixa latência", Names: []string{"parsec"}},
	{ID: "anydesk", Name: "AnyDesk", Category: "seguranca", Winget: "AnyDeskSoftwareGmbH.AnyDesk", Suggested: []string{Work},
		For: "Suporte remoto a familiares", Note: "Só instale se for usar: é alvo comum de golpe de \"suporte\". Identificador ainda não conferido no catálogo.", Names: []string{"anydesk"}},
}

// Get returns an item by its id.
func Get(id string) (Item, bool) {
	for _, it := range All {
		if it.ID == id {
			return it, true
		}
	}
	return Item{}, false
}

// has tells whether a list carries s.
func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Periph is one input device found on the PC (USB vendor id and the name
// Windows shows), enough to tell whether its maker's software applies.
type Periph struct {
	VID, PID uint16
	Name     string
}

var gModelRe = regexp.MustCompile(`\bg[0-9]{3}\b`)

// Matches tells whether a peripheral is one this item's software serves:
// its vendor id is in Vendors and, when Models or PIDs narrow the line,
// its product id is one of PIDs or its name carries one of the fragments.
func (it Item) Matches(p Periph) bool {
	found := false
	for _, v := range it.Vendors {
		if v == p.VID {
			found = true
		}
	}
	if !found {
		return false
	}
	if len(it.Models) == 0 && len(it.PIDs) == 0 {
		return true
	}
	for _, pid := range it.PIDs {
		if pid == p.PID {
			return true
		}
	}
	n := strings.ToLower(p.Name)
	for _, m := range it.Models {
		if m == "g###" {
			if gModelRe.MatchString(n) {
				return true
			}
			continue
		}
		if m != "" && strings.Contains(n, m) {
			return true
		}
	}
	return false
}

// LaptopMatch tells whether a system manufacturer string (lower-case) is
// the maker this item is for: a whole word ("hp", "dell") or, for longer
// fragments, a word prefix ("lenovo" in "LENOVO", "hewlett" for
// "Hewlett-Packard"; "hp" also accepts that spelling).
func (it Item) LaptopMatch(maker string) bool {
	if it.Laptop == "" || maker == "" {
		return false
	}
	for _, w := range strings.FieldsFunc(strings.ToLower(maker), func(r rune) bool { return r == ' ' || r == '-' || r == ',' || r == '.' || r == '_' }) {
		if w == it.Laptop || (len(it.Laptop) >= 4 && strings.HasPrefix(w, it.Laptop)) {
			return true
		}
		if it.Laptop == "hp" && strings.HasPrefix(w, "hewlett") {
			return true
		}
	}
	return false
}

// Preset tells how an item starts for a profile and the machine: "marcado"
// (checked), "sugerido" (listed as a suggestion), or "" (listed only).
// periphs are the input devices found; laptopMaker is the system
// manufacturer of a notebook ("" on a desktop); gpuVendor is "NVIDIA" or
// "AMD".
func (it Item) Preset(profile string, periphs []Periph, laptopMaker, gpuVendor string) string {
	for _, p := range periphs {
		if it.Matches(p) {
			return "marcado"
		}
	}
	if it.LaptopMatch(laptopMaker) {
		return "marcado"
	}
	if (it.ID == "nvidia" && gpuVendor == "NVIDIA") || (it.ID == "amd" && gpuVendor == "AMD") {
		return "marcado"
	}
	if has(it.Profiles, profile) {
		return "marcado"
	}
	if has(it.Suggested, profile) {
		return "sugerido"
	}
	return ""
}

// NameMatch tells whether an installed program's name (as the Programs
// page lists it) is this item: the name is the fragment itself, or the
// fragment followed by a version, an edition in parentheses or the words
// "version"/"versão" ("7-Zip 24.08 (x64)", "Mozilla Firefox (x64 pt-BR)",
// "Git version 2.47"), never another word ("Notion Calendar" is not
// Notion, "SteamVR" is not Steam).
func (it Item) NameMatch(installedName string) bool {
	n := strings.ToLower(strings.TrimSpace(installedName))
	for _, frag := range it.Names {
		if frag == "" || !strings.HasPrefix(n, frag) {
			continue
		}
		rest := strings.TrimSpace(n[len(frag):])
		if rest == "" {
			return true
		}
		if r := rest[0]; r >= '0' && r <= '9' || r == '(' || r == '-' || r == 'v' && (strings.HasPrefix(rest, "version") || strings.HasPrefix(rest, "versão") || strings.HasPrefix(rest, "v.") || len(rest) > 1 && rest[1] >= '0' && rest[1] <= '9') {
			return true
		}
	}
	return false
}

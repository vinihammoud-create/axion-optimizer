// Package profile describes what the PC is used for. The profile chosen on
// first use decides what counts in the score, what the one-click plan
// applies and what the full evaluation measures; the machine and the
// catalog stay the same, only the reading changes.
package profile

import "strings"

// Profile IDs (stored in settings.json).
const (
	Competitive = "competitivo"
	Live        = "live"
	Work        = "trabalho"
	Creation    = "criacao"
	Dev         = "programacao"
)

// Default is the profile of an installation that never chose one: the
// reading every version before 0.7 gave.
const Default = Competitive

// Info describes a profile for the picker and the pages.
type Info struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Short is the one-line description in the picker.
	Short string `json:"short"`
	// ScoreTitle heads the score card ("Saúde do PC para jogos").
	ScoreTitle string `json:"scoreTitle"`
	// Gaming: findings and tweaks that only matter in a game (polling rate,
	// mouse acceleration, Game Mode, the route to Valve) count.
	Gaming bool `json:"gaming"`
	// Perf: findings about raw performance (memory profile, PCIe link of the
	// GPU, Thread Director) count even without gaming.
	Perf bool `json:"perf"`
	// Live: streaming or recording with the game; HAGS and MPO become the
	// user's call (the OBS developers ask for HAGS off with NVENC).
	Live bool `json:"live"`
	// Scored lists, for the picker, what enters the score.
	Scored string `json:"scored"`
	// Plan lists what the one-click plan applies.
	Plan string `json:"plan"`
	// Measures lists what the full evaluation runs.
	Measures string `json:"measures"`
}

// All profiles in display order.
var All = []Info{
	{ID: Competitive, Title: "Jogo competitivo", Short: "CS2 e outros FPS: cada quadro e cada milissegundo contam.", ScoreTitle: "Saúde do PC para jogos",
		Gaming: true, Perf: true,
		Scored:   "FPS, latência de entrada, rede, anti-cheat, estabilidade.",
		Plan:     "Todo o catálogo com evidência e sem risco.",
		Measures: "FPS no jogo, rede até a Valve, taxa do mouse."},
	{ID: Live, Title: "Jogo + live", Short: "Joga e transmite ou grava ao mesmo tempo.", ScoreTitle: "Saúde do PC para jogo e live",
		Gaming: true, Perf: true, Live: true,
		Scored:   "O mesmo do competitivo; o agendamento de GPU (HAGS) deixa de pesar na nota.",
		Plan:     "O mesmo; o HAGS vira decisão sua: o OBS recomenda desligá-lo quando a gravação engasga ou o encoder falha.",
		Measures: "FPS no jogo, upload sob carga (bufferbloat), taxa do mouse. Os logs do OBS (quadros pulados, atrasados e perdidos) são lidos na página Live (OBS). FPS com o OBS aberto fica para uma próxima versão."},
	{ID: Work, Title: "Trabalho e escritório", Short: "Documentos, planilhas, navegador e chamadas de vídeo.", ScoreTitle: "Saúde do PC para o trabalho",
		Scored:   "Estabilidade, disco, programas na inicialização, energia e rede para chamadas.",
		Plan:     "Privacidade, correções e energia; nenhum ajuste de jogo.",
		Measures: "Rede com as metas da Microsoft para chamadas (Teams). Tempo de inicialização e bateria ficam para uma próxima versão."},
	{ID: Creation, Title: "Criação (vídeo, foto, áudio)", Short: "Edição, renderização e exportação.", ScoreTitle: "Saúde do PC para criação",
		Perf:     true,
		Scored:   "Memória, disco, placa de vídeo para o encoder e estabilidade.",
		Plan:     "Energia na tomada, disco e correções; o HAGS nunca é desligado à força.",
		Measures: "Rede sob carga (upload de arquivos e chamadas). O teste de carga sustentada fica para uma próxima versão."},
	{ID: Dev, Title: "Programação", Short: "Compilação, contêineres e máquinas virtuais.", ScoreTitle: "Saúde do PC para programação",
		Perf:     true,
		Scored:   "Memória, disco e estabilidade.",
		Plan:     "Correções, privacidade e energia; nada que atrapalhe o Hyper-V ou o WSL.",
		Measures: "Rede sob carga e para chamadas. O tempo de build antes e depois fica para uma próxima versão."},
}

// Valid tells whether id names a profile.
func Valid(id string) bool {
	for _, p := range All {
		if p.ID == id {
			return true
		}
	}
	return false
}

// Get returns the profile, or the default for an unknown or empty id.
func Get(id string) Info {
	for _, p := range All {
		if p.ID == id {
			return p
		}
	}
	return Get(Default)
}

// Scopes of a finding or a tweak.
const (
	ScopeGeneral = ""           // counts for everyone
	ScopeGaming  = "jogo"       // counts in the gaming profiles only
	ScopePerf    = "desempenho" // counts where raw performance matters (gaming, creation, programming)
	ScopeLive    = "live"       // counts only for who streams or records (the OBS logs)
)

// gamingFindings are the diagnostic IDs (prefixes) that only matter in a
// game; perfFindings the ones about raw performance. Everything else is
// general: stability, security, disk, drivers, residues of other
// optimizers, Windows support.
var (
	gamingFindings = []string{"display.refresh.", "cpu.x3d.", "gpu.rebar", "gpu.hags", "gpu.nvdrs.", "game.", "input.", "boot.hpet", "boot.tick", "net.valve", "per.", "dpc.",
		// Playing over Wi-Fi or on battery, the cost of VBS in games, little RAM
		// for games and the network card's power saving only matter when the
		// PC is used to play (residues of optimizers count for everyone).
		"net.wifi", "power.battery", "sec.vbs", "ram.low", "net.eee"}
	perfFindings = []string{"ram.xmp", "ram.single", "ram.samechannel", "cpu.hybrid.win10", "gpu.pcie.width"}
	liveFindings = []string{"obs."}
)

// FindingScope classifies a diagnostic finding by its ID.
func FindingScope(id string) string {
	for _, p := range liveFindings {
		if strings.HasPrefix(id, p) {
			return ScopeLive
		}
	}
	for _, p := range gamingFindings {
		if strings.HasPrefix(id, p) {
			return ScopeGaming
		}
	}
	for _, p := range perfFindings {
		if strings.HasPrefix(id, p) {
			return ScopePerf
		}
	}
	return ScopeGeneral
}

// gamingTweaks are catalog items whose benefit is only in a game (the fixes
// of residues, privacy, interface, services and the Windows Update items
// serve everyone); gamingCategories the whole categories in that case.
var (
	gamingTweaks     = map[string]bool{"net.nagle": true, "net.nic.intmod": true, "power.coreparking": true, "power.pcie.aspm": true, "power.usb.suspend": true, "power.plan.high": true, "power.plan.ultimate": true, "update.drivers.exclude": true}
	gamingCategories = map[string]bool{"jogo": true, "entrada": true, "gpu": true, "jogos": true}
)

// TweakScope classifies a catalog item by its ID and category.
func TweakScope(id, category string) string {
	if gamingTweaks[id] || gamingCategories[category] || strings.HasPrefix(id, "profile:") {
		return ScopeGaming
	}
	return ScopeGeneral
}

// Counts tells whether something of the scope enters the score (or the
// plan) of the profile.
func (p Info) Counts(scope string) bool {
	switch scope {
	case ScopeGaming:
		return p.Gaming
	case ScopePerf:
		return p.Gaming || p.Perf
	case ScopeLive:
		return p.Live
	}
	return true
}

// FindingCounts tells whether a finding enters the score of the profile.
// With a live, HAGS off is the user's own call, so its finding is a note.
func (p Info) FindingCounts(id string) bool {
	if p.Live && id == "gpu.hags" {
		return false
	}
	return p.Counts(FindingScope(id))
}

// informed are the items that stop being recommended in the live profile:
// they are offered, unchecked, with the reason. (The MPO fix is already a
// symptom-only item, never recommended, so only HAGS changes.)
var informed = map[string]string{
	"game.hags": "no perfil jogo + live é decisão sua: a base de conhecimento do OBS diz que o HAGS pode causar engasgos e falhas com o OBS e com os encoders por hardware, e recomenda desligá-lo quando há esses problemas (obsproject.com/kb/hags); o DLSS Frame Generation, por outro lado, exige HAGS ligado",
}

// Informed returns, for the profile, the reason an item is offered as the
// user's call instead of recommended (empty when it stays as it is).
func (p Info) Informed(tweakID string) string {
	if !p.Live {
		return ""
	}
	return informed[tweakID]
}

// TweakCounts tells whether a catalog item belongs to the one-click plan of
// the profile (the user can still apply anything from its own page).
func (p Info) TweakCounts(id, category string) bool {
	return p.Counts(TweakScope(id, category))
}

// OffNote explains, for the pages, why items are set aside.
func (p Info) OffNote() string {
	if p.Gaming {
		return ""
	}
	return "Itens que só fazem diferença em jogo (ou no desempenho bruto, no perfil de trabalho) não entram na nota nem no 1 clique no perfil " + p.Title + "; eles continuam disponíveis nas páginas de cada um."
}

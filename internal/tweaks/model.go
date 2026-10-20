package tweaks

import (
	"axion/internal/facts"
)

// Evidence grades how well a tweak's effect is documented or measured.
type Evidence int

const (
	// EvHigh: documented by the platform vendor (Microsoft/NVIDIA/AMD/Intel) or
	// consistently measured by reputable independent testing.
	EvHigh Evidence = iota + 1
	// EvMedium: real, documented mechanism; benefit depends on the scenario.
	EvMedium
	// EvLow: plausible mechanism, little or inconsistent measured benefit.
	EvLow
)

func (e Evidence) String() string {
	switch e {
	case EvHigh:
		return "alta"
	case EvMedium:
		return "média"
	case EvLow:
		return "baixa"
	}
	return "?"
}

// Risk grades the downside of applying a tweak.
type Risk int

const (
	RiskSafe Risk = iota + 1
	RiskModerate
	RiskAdvanced
)

func (r Risk) String() string {
	switch r {
	case RiskSafe:
		return "seguro"
	case RiskModerate:
		return "moderado"
	case RiskAdvanced:
		return "avançado"
	}
	return "?"
}

// Reboot says what is needed for the change to take effect.
type Reboot int

const (
	RebootNone Reboot = iota
	RebootSignOut
	RebootRestart
	RebootRestartApp // restart the game/app
)

func (r Reboot) String() string {
	switch r {
	case RebootSignOut:
		return "sair e entrar"
	case RebootRestart:
		return "reiniciar o PC"
	case RebootRestartApp:
		return "reabrir o jogo"
	}
	return "imediato"
}

// Source is a citation.
type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// RecLevel is a per-machine recommendation.
type RecLevel int

const (
	RecNeutral RecLevel = iota // optional; no strong signal either way
	RecYes                     // recommended for this PC
	RecNo                      // not recommended for this PC (user may still apply)
	RecBlocked                 // would break something on this PC; apply refused
)

func (r RecLevel) String() string {
	switch r {
	case RecYes:
		return "recomendado"
	case RecNo:
		return "não recomendado"
	case RecBlocked:
		return "bloqueado"
	}
	return "opcional"
}

// Recommendation carries the level and the machine-specific reason.
type Recommendation struct {
	Level  RecLevel `json:"level"`
	Reason string   `json:"reason"`
}

// Rec is a helper to build a Recommendation.
func Rec(l RecLevel, reason string) Recommendation { return Recommendation{Level: l, Reason: reason} }

// Tweak is one user-facing optimization made of reversible operations.
type Tweak struct {
	ID       string
	Category string
	Title    string
	Summary  string // one sentence: what it does and the benefit
	Detail   string // mechanism, caveats, how it was evaluated
	Evidence Evidence
	Sources  []Source
	Risk     Risk
	RiskNote string
	Reboot   Reboot
	Tags     []string
	// OneClick: part of the category "apply recommended" and of the global
	// smart optimization when Recommend returns RecYes.
	OneClick bool
	MinBuild int // 0 = any
	MaxBuild int // 0 = any
	// Applicable returns false (with a reason) when the tweak makes no sense on
	// this machine (wrong GPU vendor, feature missing, Windows 10 only...).
	Applicable func(f *facts.Facts) (bool, string)
	// Recommend returns the machine-specific recommendation.
	Recommend func(f *facts.Facts) Recommendation
	// Ops builds the operations. It receives facts because some operations
	// depend on the machine (e.g. per-interface paths, detected GPU).
	Ops func(f *facts.Facts) []Op
	// Post runs after a successful apply or revert that changed something
	// (e.g. restart a network adapter so the driver reloads its settings).
	// Errors are reported as warnings; the change itself stays recorded.
	// c.Touched lists the targets of the operations that changed.
	Post func(c *Ctx, reverted bool) error
	// Effect, when set, tells whether the change is really in effect on the
	// machine (Windows may ignore a value: a policy, a firmware lock, a
	// reboot not done yet). It is only asked when the values are in place;
	// the reason explains what keeps it from working.
	Effect func(f *facts.Facts) (inEffect bool, reason string)
	// Group makes tweaks mutually exclusive (e.g. power plans). Applying one
	// member first reverts the active member of the same group.
	Group string
	// Profile marks per-game tweaks generated at runtime ("profile:<exe>:...").
	Profile string
}

// IsApplicable evaluates build range and the Applicable func.
func (t *Tweak) IsApplicable(f *facts.Facts) (bool, string) {
	if f != nil && f.OS.Build > 0 {
		if t.MinBuild > 0 && f.OS.Build < t.MinBuild {
			return false, "requer uma versão mais nova do Windows"
		}
		if t.MaxBuild > 0 && f.OS.Build > t.MaxBuild {
			return false, "não se aplica a esta versão do Windows"
		}
	}
	if t.Applicable != nil && f != nil {
		return t.Applicable(f)
	}
	return true, ""
}

// Recommendable reports whether the tweak may enter the one-click
// "Recomendada" set at all (when its recommendation for the PC is RecYes).
// The published rules are enforced here, not by convention: only items
// without risk, never the advanced category and never low evidence.
func (t *Tweak) Recommendable() bool {
	return t.OneClick && t.Risk == RiskSafe && t.Category != "avancado" && t.Evidence != EvLow
}

// OptionalEligible reports whether the tweak may be offered, unchecked, by
// the complete one-click tier (when its recommendation is neutral).
func (t *Tweak) OptionalEligible() bool {
	return t.OneClick && t.Risk == RiskSafe && t.Category != "avancado" && t.Group == ""
}

// Recommendation evaluates Recommend (neutral by default).
func (t *Tweak) Recommendation(f *facts.Facts) Recommendation {
	if t.Recommend == nil || f == nil {
		return Rec(RecNeutral, "")
	}
	return t.Recommend(f)
}

// Category metadata for the UI.
type Category struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

// Categories in display order.
var Categories = []Category{
	{ID: "correcoes", Title: "Correções", Description: "Desfaz configurações prejudiciais deixadas por outros otimizadores e scripts. Só aparece o que foi encontrado neste PC.", Icon: "wrench"},
	{ID: "jogo", Title: "Sistema e jogo", Description: "Recursos do Windows que afetam jogos: Modo de Jogo, gravação em segundo plano, agendamento de GPU e apresentação em janela.", Icon: "gamepad"},
	{ID: "energia", Title: "Energia e CPU", Description: "Plano de energia, estacionamento de núcleos e economia de energia de USB/PCIe, adequados ao seu processador.", Icon: "cpu"},
	{ID: "entrada", Title: "Mouse e teclado", Description: "Entrada 1:1 sem aceleração e atalhos de acessibilidade que interrompem partidas.", Icon: "mouse"},
	{ID: "rede", Title: "Rede", Description: "Ajustes locais de rede com efeito documentado. Nenhum ajuste local muda a rota até o servidor.", Icon: "wifi"},
	{ID: "gpu", Title: "GPU e tela", Description: "Configurações de driver e correções específicas por fabricante.", Icon: "monitor"},
	{ID: "privacidade", Title: "Privacidade e telemetria", Description: "Anúncios, sugestões, telemetria e coleta de dados. Ganho de desempenho pequeno; ganho de privacidade real.", Icon: "shield"},
	{ID: "update", Title: "Windows Update", Description: "Controle sobre drivers automáticos e compartilhamento de atualizações.", Icon: "refresh"},
	{ID: "interface", Title: "Interface", Description: "Responsividade do Explorer e menus. Não altera FPS em tela cheia.", Icon: "layout"},
	{ID: "servicos", Title: "Serviços", Description: "Serviços não essenciais em Manual (o Windows ainda pode iniciá-los se precisar).", Icon: "layers"},
	{ID: "avancado", Title: "Avançado (kernel e segurança)", Description: "Mudanças com trade-off de segurança ou evidência mista. Nunca entram no 1 clique.", Icon: "alert"},
}

// Excluded documents something another optimizer does that Axion refuses to
// do, with the reason. Shown in the UI and exported to the evidence matrix.
type Excluded struct {
	Name string `json:"name"`
	// SeenIn is the project's own reference: it is not sent to the
	// interface nor exported (no product names on screen or in the package).
	SeenIn    []string `json:"-"`
	Mechanism string   `json:"mechanism"`
	Verdict   string   `json:"verdict"` // placebo | sem efeito | risco | impossível
	Reason    string   `json:"reason"`
	Sources   []Source `json:"sources,omitempty"`
	// Targets are lower-case fragments of an Op.Target() that no item of the
	// catalog may write (a value name is written with a leading "|", as in
	// "|maxframelatency"). The "correcoes" items are exempt: they exist to
	// undo these very changes. A test enforces it.
	Targets []string `json:"-"`
}

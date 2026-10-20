package app

import (
	"context"
	"fmt"
	"sort"

	"axion/internal/catalog"
	"axion/internal/profile"
	"axion/internal/tweaks"
)

// Tiers of the one-click optimization.
const (
	TierRecommended = "recomendado" // evidence-based, recommended for this PC
	TierComplete    = "completo"    // + optional items for the user to pick (unchecked)
	TierCS2         = "cs2"         // recommended + Counter-Strike 2 profile
	TierCategory    = "categoria"   // recommended items of one category
)

// PlanItem is one tweak proposed by a tier.
type PlanItem struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Summary       string `json:"summary"`
	Category      string `json:"category"`
	CategoryTitle string `json:"categoryTitle"`
	Rec           string `json:"rec"`
	Reason        string `json:"reason"`
	Evidence      string `json:"evidence"`
	Risk          string `json:"risk"`
	Reboot        string `json:"reboot"`
	Optional      bool   `json:"optional"` // included by "completo" only
	// Checked: selected when the plan opens. Optional items start
	// unchecked: the user picks them knowing the evidence and the risk.
	Checked bool `json:"checked"`
}

// Plan is what a one-click button would do.
type Plan struct {
	Tier    string     `json:"tier"`
	Title   string     `json:"title"`
	Items   []PlanItem `json:"items"`
	Reboot  bool       `json:"reboot"`
	Note    string     `json:"note"`
	Profile string     `json:"profile"` // the use profile the plan was read for
}

// CategoryTitle maps a category ID to its title.
func CategoryTitle(id string) string {
	for _, c := range tweaks.Categories {
		if c.ID == id {
			return c.Title
		}
	}
	if id == "jogos" {
		return "Perfis de jogo"
	}
	if id == "inicializacao" {
		return "Inicialização"
	}
	return id
}

func planItem(t *tweaks.Tweak, st tweaks.State, optional bool) PlanItem {
	return PlanItem{ID: t.ID, Title: t.Title, Summary: t.Summary, Category: t.Category, CategoryTitle: CategoryTitle(t.Category),
		Rec: st.Rec.Level.String(), Reason: st.Rec.Reason, Evidence: t.Evidence.String(), Risk: t.Risk.String(), Reboot: t.Reboot.String(),
		Optional: optional, Checked: !optional}
}

func pending(st tweaks.State) bool {
	return st.Applicable && st.Status != "on" && st.Status != "na"
}

// isRecommended: part of the evidence-based one-click set for this PC. Only
// items without risk, outside the advanced category and with more than low
// evidence enter it (see the Sobre page and Tweak.Recommendable).
func isRecommended(t *tweaks.Tweak, st tweaks.State) bool {
	return t.Recommendable() && pending(st) && st.Rec.Level == tweaks.RecYes
}

// isOptional: offered (unchecked) by the complete tier: one-click items with
// no recommendation either way for this PC and no risk. Items with a risk,
// marked for a specific symptom (RecNo) or kept out of one-click are only
// applied from their own page.
func isOptional(t *tweaks.Tweak, st tweaks.State) bool {
	return pending(st) && st.Rec.Level == tweaks.RecNeutral && t.OptionalEligible()
}

// recommendedFor tells whether the item enters the recommended set of the
// profile. When the machine recommends it but the profile sets it aside (a
// gaming item outside the gaming profiles; the HAGS with a live), the
// reason comes back so the complete tier can offer it unchecked.
func recommendedFor(t *tweaks.Tweak, st tweaks.State, p profile.Info) (bool, string) {
	if !isRecommended(t, st) {
		return false, ""
	}
	if why := p.Informed(t.ID); why != "" {
		return false, why
	}
	if !p.TweakCounts(t.ID, t.Category) {
		return false, "só faz diferença em jogo, por isso fica fora do 1 clique no perfil " + p.Title + "; marque se quiser"
	}
	return true, ""
}

// BuildPlan computes the tweaks a tier would apply for the use profile:
// outside the gaming profiles the items that only matter in a game are
// not recommended (the complete tier still offers them, unchecked), and
// in the live profile the HAGS becomes the user's call.
func (a *App) BuildPlan(tier, category string) (Plan, error) {
	f := a.Facts()
	if f == nil {
		return Plan{}, ErrNoScan
	}
	st := a.States()
	prof := a.profile()
	p := Plan{Tier: tier, Profile: prof.ID}
	switch tier {
	case TierRecommended:
		p.Title = "Otimização recomendada"
		p.Note = "Só itens com evidência de benefício e recomendados para o seu hardware. Cada mudança fica no histórico e pode ser desfeita."
	case TierComplete:
		p.Title = "Otimização completa"
		p.Note = "Os recomendados já vêm marcados. Os opcionais têm benefício pequeno ou dependente do uso (veja a evidência de cada um): marque só os que fizerem sentido para você."
	case TierCS2:
		p.Title = "Perfil Counter-Strike 2"
		p.Note = "Recomendados do sistema + prioridade do cs2.exe. A maior parte do ganho no CS2 vem do sistema e das configurações dentro do jogo (o checklist fica na página CS2)."
	case TierCategory:
		p.Title = "Recomendados: " + CategoryTitle(category)
		p.Note = "Itens recomendados desta categoria para o seu PC."
	default:
		return Plan{}, fmt.Errorf("modo desconhecido: %s", tier)
	}
	// The CS2 tier and a category's own button are the gaming reading
	// whatever the profile: the user asked for the game, or for that
	// category (its page counts the recommended items the same way).
	gaming := prof
	if tier == TierCS2 || tier == TierCategory {
		gaming = profile.Get(profile.Competitive)
	}
	if n := gaming.OffNote(); n != "" && tier != TierCategory {
		p.Note += " " + n
	}
	for _, t := range a.Engine.Catalog.All() {
		s := st[t.ID]
		rec, why := recommendedFor(t, s, gaming)
		switch {
		case tier == TierCategory:
			if t.Category == category && rec {
				p.Items = append(p.Items, planItem(t, s, false))
			}
		case rec:
			p.Items = append(p.Items, planItem(t, s, false))
		case tier == TierComplete && (isOptional(t, s) || why != ""):
			it := planItem(t, s, true)
			if why != "" {
				it.Reason = why
			}
			p.Items = append(p.Items, it)
		}
	}
	if tier == TierCS2 {
		if exe := "cs2.exe"; a.gamePath(exe) != "" {
			for _, feat := range []string{"priority", "gpu"} {
				t := a.Engine.Resolve(catalog.ProfileID(exe, feat), f)
				if t == nil {
					continue
				}
				s := a.Engine.StateOf(context.Background(), f, t)
				if pending(s) && t.Risk == tweaks.RiskSafe && s.Rec.Level != tweaks.RecNo && s.Rec.Level != tweaks.RecBlocked {
					p.Items = append(p.Items, planItem(t, s, s.Rec.Level != tweaks.RecYes))
				}
			}
		} else {
			p.Note += " O CS2 não foi encontrado neste PC, então o perfil do jogo não foi incluído."
		}
	}
	order := map[string]int{}
	for i, c := range tweaks.Categories {
		order[c.ID] = i
	}
	order["jogos"] = len(tweaks.Categories)
	sort.SliceStable(p.Items, func(i, j int) bool { return order[p.Items[i].Category] < order[p.Items[j].Category] })
	for _, it := range p.Items {
		if it.Reboot == tweaks.RebootRestart.String() {
			p.Reboot = true
		}
	}
	return p, nil
}

// PendingRecommended counts recommended tweaks not yet applied (for the
// use profile).
func (a *App) PendingRecommended() int {
	if a.Facts() == nil {
		return 0
	}
	st := a.States()
	prof := a.profile()
	n := 0
	for _, t := range a.Engine.Catalog.All() {
		if rec, _ := recommendedFor(t, st[t.ID], prof); rec {
			n++
		}
	}
	return n
}

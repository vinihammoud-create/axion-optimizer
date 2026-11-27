package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"axion/internal/catalog"
	"axion/internal/tweaks"
)

// The selection file (0.10.1) is the list of optimizations a person chose,
// to apply them again later or on another PC. It holds the IDs and the
// metadata below, nothing read from the machine: no PC name, account, path
// or hardware. Reading it back never applies anything: it opens the normal
// plan, with the same rules of risk, applicability and 1 clique.
const (
	SelectionFormat        = "axion-selecao"
	SelectionFormatVersion = 1
	// Limits of a file to import: far above what the catalog can produce.
	maxSelectionBytes = 64 << 10
	maxSelectionIDs   = 500
	maxSelectionIDLen = 200
)

// SelectionFile is the content of the exported file.
type SelectionFile struct {
	Format        string    `json:"format"`
	FormatVersion int       `json:"formatVersion"`
	AxionVersion  string    `json:"axionVersion"`
	Created       time.Time `json:"created"`
	IDs           []string  `json:"ids"`
}

// SelectionSkip is an item of the file left out of the plan, with the reason.
type SelectionSkip struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

// SelectionImport is what reading a file gives back: the plan to review and
// what stayed out of it. Nothing has been applied.
type SelectionImport struct {
	Plan Plan `json:"plan"`
	// Unknown are IDs this Axion does not know (another version, or a file
	// edited by hand).
	Unknown []string        `json:"unknown,omitempty"`
	Skipped []SelectionSkip `json:"skipped,omitempty"`
	// Of the file, for the person to recognize it.
	AxionVersion string    `json:"axionVersion"`
	Created      time.Time `json:"created"`
}

// SelectionImportRequest is the body of POST /api/selection/import: the text
// of the file the person picked.
type SelectionImportRequest struct {
	Content string `json:"content"`
}

// ownRoute tells why an ID is never taken from a file: these items are
// chosen on their own page, which checks what the general apply does not
// (and their IDs carry names of this PC: startup entries, network adapters).
func ownRoute(id string) string {
	switch {
	case strings.HasPrefix(id, "startup:"):
		return "programa da inicialização: escolha na página Inicialização"
	case catalog.IsDNSID(id):
		return "troca de DNS: só é oferecida pelo teste de rede deste PC"
	case strings.HasPrefix(id, catalog.CS2PresetPrefix):
		return "preset do CS2: use o cartão Preset competitivo da página CS2"
	}
	return ""
}

// ExportSelection lists the optimizations applied by Axion now (for the
// account signed in), in a stable order.
func (a *App) ExportSelection() (SelectionFile, error) {
	seen := map[string]bool{}
	var ids []string
	for _, e := range a.Engine.J.Snapshot() {
		id := e.TweakID
		if seen[id] || ownRoute(id) != "" {
			continue
		}
		seen[id] = true
		if cur := a.Engine.ActiveFor(id); cur != nil && cur.Status == tweaks.StatusApplied {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return SelectionFile{}, fmt.Errorf("%w: nenhuma otimização aplicada pelo Axion nesta conta para exportar", errBadRequest)
	}
	sort.Strings(ids)
	return SelectionFile{Format: SelectionFormat, FormatVersion: SelectionFormatVersion, AxionVersion: a.Version,
		Created: time.Now().UTC().Truncate(time.Second), IDs: ids}, nil
}

// ParseSelection reads a selection file strictly: the format and its
// version, the size, no field beyond the known ones and IDs that are plain
// text. Repeated IDs count once.
func ParseSelection(content string) (SelectionFile, error) {
	bad := func(why string) (SelectionFile, error) {
		return SelectionFile{}, fmt.Errorf("%w: arquivo de seleção inválido: %s", errBadRequest, why)
	}
	if len(content) > maxSelectionBytes {
		return bad(fmt.Sprintf("grande demais (o limite é de %d KB)", maxSelectionBytes>>10))
	}
	if strings.TrimSpace(content) == "" {
		return bad("o arquivo está vazio")
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(content)))
	dec.DisallowUnknownFields()
	var sf SelectionFile
	if err := dec.Decode(&sf); err != nil {
		return bad("não é um arquivo de seleção do Axion (JSON fora do formato)")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return bad("há conteúdo depois da seleção")
	}
	if sf.Format != SelectionFormat {
		return bad("não é um arquivo de seleção do Axion")
	}
	if sf.FormatVersion != SelectionFormatVersion {
		return bad(fmt.Sprintf("versão %d do formato, e este Axion lê a versão %d", sf.FormatVersion, SelectionFormatVersion))
	}
	if len(sf.IDs) == 0 {
		return bad("a lista de otimizações está vazia")
	}
	if len(sf.IDs) > maxSelectionIDs {
		return bad(fmt.Sprintf("mais de %d otimizações", maxSelectionIDs))
	}
	seen := map[string]bool{}
	ids := make([]string, 0, len(sf.IDs))
	for _, id := range sf.IDs {
		if id == "" || len(id) > maxSelectionIDLen || !utf8.ValidString(id) || strings.IndexFunc(id, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
			return bad("há um identificador de otimização malformado")
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sf.IDs = ids
	return sf, nil
}

// ImportSelection reads a file and builds the plan for this PC. It only
// reads: the person reviews the plan and applies it through POST
// /api/apply, as any other plan.
func (a *App) ImportSelection(req SelectionImportRequest) (SelectionImport, error) {
	sf, err := ParseSelection(req.Content)
	if err != nil {
		return SelectionImport{}, err
	}
	if a.Facts() == nil {
		return SelectionImport{}, ErrNoScan
	}
	out := a.selectionPlan(sf.IDs)
	out.AxionVersion, out.Created = sf.AxionVersion, sf.Created
	return out, nil
}

// selectionPlan sorts the IDs of a file into the plan and what stays out.
// An item enters only where a one-click plan of today would offer it (the
// recommended and optional sets, the safe game profile items); the others
// are listed with the reason. Every item that enters starts checked: the
// person chose it in the file.
func (a *App) selectionPlan(ids []string) SelectionImport {
	f := a.Facts()
	st := a.States()
	prof := a.profile()
	out := SelectionImport{Plan: Plan{Tier: "importado", Title: "Seleção importada", Profile: prof.ID,
		Note: "Os itens do arquivo que valem para este PC já vêm marcados. Nada foi aplicado ainda: revise e aplique só o que quiser. Cada mudança fica no histórico e pode ser desfeita."}}
	skip := func(id, title, why string) {
		out.Skipped = append(out.Skipped, SelectionSkip{ID: id, Title: title, Reason: why})
	}
	for _, id := range ids {
		t := a.Engine.Resolve(id, f)
		if why := ownRoute(id); why != "" {
			title := id
			if t != nil {
				title = t.Title
			}
			skip(id, title, why)
			continue
		}
		if t == nil {
			out.Unknown = append(out.Unknown, id)
			continue
		}
		s, ok := st[t.ID]
		if !ok {
			s = a.Engine.StateOf(context.Background(), f, t)
		}
		switch {
		case !s.Applicable || s.Status == "na":
			why := "não se aplica a este PC"
			if s.NAReason != "" {
				why += ": " + s.NAReason
			}
			skip(id, t.Title, why)
			continue
		case s.Status == "on":
			skip(id, t.Title, "já está ativa neste PC")
			continue
		case s.Rec.Level == tweaks.RecBlocked:
			skip(id, t.Title, "bloqueada neste PC: "+s.Rec.Reason)
			continue
		}
		if t.Category == "jogos" {
			// The rule of the game profile in the CS2 plan.
			if pending(s) && t.OneClick && t.Risk == tweaks.RiskSafe && s.Rec.Level != tweaks.RecNo {
				it := planItem(t, s, s.Rec.Level != tweaks.RecYes)
				it.Checked = true
				out.Plan.Items = append(out.Plan.Items, it)
			} else {
				skip(id, t.Title, offOneClick(t, s))
			}
			continue
		}
		rec, why := recommendedFor(t, s, prof)
		switch {
		case rec:
			out.Plan.Items = append(out.Plan.Items, planItem(t, s, false))
		case why != "" || isOptional(t, s):
			it := planItem(t, s, true)
			if why != "" {
				it.Reason = why
			}
			it.Checked = true
			out.Plan.Items = append(out.Plan.Items, it)
		default:
			skip(id, t.Title, offOneClick(t, s))
		}
	}
	sortPlan(&out.Plan)
	return out
}

// offOneClick says why an item stays out of a one-click plan.
func offOneClick(t *tweaks.Tweak, s tweaks.State) string {
	switch {
	case t.Risk != tweaks.RiskSafe:
		return "tem risco (" + t.Risk.String() + "): só se liga pela própria página, em Otimizações"
	case s.Rec.Level == tweaks.RecNo:
		why := "não é indicada para este PC"
		if s.Rec.Reason != "" {
			why += ": " + s.Rec.Reason
		}
		return why + "; se quiser, ligue pela página Otimizações"
	}
	return "fica fora do 1 clique: se quiser, ligue pela página Otimizações"
}

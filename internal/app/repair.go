package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"axion/internal/repair"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

var (
	srcRepairSFC  = tweaks.Source{Title: "Microsoft Support — Use a ferramenta Verificador de Arquivos do Sistema para reparar arquivos de sistema ausentes ou corrompidos (rode o DISM antes do SFC)", URL: "https://support.microsoft.com/en-us/topic/use-the-system-file-checker-tool-to-repair-missing-or-corrupted-system-files-79aa86cb-ca52-166a-92a3-966e85d4094e"}
	srcRepairDISM = tweaks.Source{Title: "Microsoft Learn — Repair a Windows Image (DISM /RestoreHealth; o Windows Update é a origem padrão dos arquivos)", URL: "https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/repair-a-windows-image"}
)

// Verdicts of a repair run.
const (
	RepairClean     = "clean"     // both tools finished and found nothing wrong
	RepairFixed     = "fixed"     // damaged system files were restored
	RepairRestart   = "restart"   // finished or queued: a restart completes it
	RepairUnfixed   = "unfixed"   // damage the tools could not fix
	RepairFailedRun = "failed"    // a tool could not do its work
	RepairInexact   = "unknown"   // finished, but the message was not recognized
	RepairStopped   = "cancelled" // the user cancelled
)

const (
	repairDISMWeight = 0.5 // share of the progress bar that belongs to DISM
	repairDISMLimit  = 90 * time.Minute
	repairSFCLimit   = 60 * time.Minute
)

// RepairStep is one tool of the repair and what it reported.
type RepairStep struct {
	ID       string         `json:"id"` // "dism" or "sfc"
	Title    string         `json:"title"`
	Outcome  repair.Outcome `json:"outcome"`
	ExitCode int            `json:"exitCode"`
	Summary  string         `json:"summary"`
}

// RepairReport is the outcome of a repair run.
type RepairReport struct {
	At      time.Time    `json:"at"`
	Steps   []RepairStep `json:"steps"`
	Verdict string       `json:"verdict"`
	Summary string       `json:"summary"`
	Reboot  bool         `json:"reboot"`
}

// RepairInfo is what the Saúde do Windows page shows next to the button.
type RepairInfo struct {
	Elevated bool            `json:"elevated"`
	Related  []string        `json:"related,omitempty"` // findings that can come from damaged system files
	Last     *RepairReport   `json:"last,omitempty"`
	Sources  []tweaks.Source `json:"sources"`
}

// RepairRequest must carry the user's confirmation.
type RepairRequest struct {
	Confirm bool `json:"confirm"`
}

type repairCache struct {
	mu   sync.Mutex
	last *RepairReport
}

// Repair describes the repair for the page.
func (a *App) Repair() RepairInfo {
	v := RepairInfo{Elevated: a.P.Native.IsElevated(), Sources: []tweaks.Source{srcRepairDISM, srcRepairSFC}}
	if f := a.Facts(); f != nil && f.Health.DirtyRead && len(f.Health.Dirty) > 0 {
		v.Related = append(v.Related, "Volume "+strings.Join(f.Health.Dirty, ", ")+" marcado para verificação de disco (este reparo não substitui a verificação do disco)")
	}
	if r := a.Report(); r != nil {
		for _, fd := range r.Findings {
			if strings.HasPrefix(fd.ID, "stab.") {
				v.Related = append(v.Related, fd.Title)
			}
		}
	}
	a.rep.mu.Lock()
	v.Last = a.rep.last
	a.rep.mu.Unlock()
	return v
}

// StartRepair restores damaged Windows system files: DISM /RestoreHealth and
// then sfc /scannow, the order Microsoft documents. It is not a tweak: it
// puts back the original files, so there is nothing to undo and nothing goes
// through the journal. Like the cleanup, it is a job with progress.
func (a *App) StartRepair(req RepairRequest) (Job, error) {
	if !req.Confirm {
		return Job{}, fmt.Errorf("%w: falta a confirmação do reparo", errBadRequest)
	}
	if !a.P.Native.IsElevated() {
		return Job{}, fmt.Errorf("%w: o reparo de arquivos do Windows exige o Axion como administrador; abra o programa de novo e aceite o pedido de permissão", errBadRequest)
	}
	return a.Jobs.Start("repair", "Reparando arquivos do Windows", true, func(ctx context.Context, c *JobCtl) (any, error) {
		rep := RepairReport{At: time.Now()}
		type tool struct {
			id, title, exe string
			args           []string
			limit          time.Duration
			from, size     float64
			parse          func(int, string) (repair.Outcome, string)
		}
		tools := []tool{
			{"dism", "DISM: reparar o repositório de componentes", "Dism.exe", []string{"/Online", "/Cleanup-Image", "/RestoreHealth"}, repairDISMLimit, 0, repairDISMWeight, repair.ParseDISM},
			{"sfc", "SFC: verificar e repor os arquivos de sistema", "sfc.exe", []string{"/scannow"}, repairSFCLimit, repairDISMWeight, 1 - repairDISMWeight - 0.02, repair.ParseSFC},
		}
		for _, t := range tools {
			if ctx.Err() != nil {
				rep.Steps = append(rep.Steps, RepairStep{ID: t.id, Title: t.title, Outcome: repair.Skipped, Summary: "Não executado: o reparo foi cancelado."})
				continue
			}
			c.Progress(t.from, t.title)
			c.Logf("%s %s (pode levar vários minutos)", t.exe, strings.Join(t.args, " "))
			last := -1.0
			res, err := a.P.Run.Run(ctx, t.exe, t.args, sys.RunOpts{Timeout: t.limit,
				OnLine: func(line string) {
					if p, ok := repair.Percent(line); ok {
						if p != last {
							last = p
							c.Progress(t.from+t.size*p/100, fmt.Sprintf("%s (%.0f%%)", t.title, p))
						}
						return
					}
					if l := strings.TrimSpace(line); l != "" {
						c.Logf("  %s", l)
					}
				}})
			step := RepairStep{ID: t.id, Title: t.title, ExitCode: res.ExitCode}
			switch {
			case ctx.Err() != nil:
				step.Outcome, step.Summary = repair.Cancelled, "Interrompido a seu pedido. Rode o reparo de novo para concluir."
			case errors.Is(err, sys.ErrTimeout):
				step.Outcome, step.Summary = repair.Failed, fmt.Sprintf("Não terminou em %s e foi interrompido.", t.limit)
			case err != nil:
				step.Outcome, step.Summary = repair.Failed, "Não foi possível iniciar: "+err.Error()
			default:
				step.Outcome, step.Summary = t.parse(res.ExitCode, res.Stdout+"\n"+res.Stderr)
			}
			rep.Steps = append(rep.Steps, step)
			c.Logf("%s: %s", t.title, step.Summary)
		}
		c.Progress(0.99, "Concluindo")
		cancelled := ctx.Err() != nil
		rep.Verdict, rep.Summary = repairVerdict(rep.Steps, cancelled)
		for _, s := range rep.Steps {
			if s.Outcome == repair.Reboot {
				rep.Reboot = true
			}
		}
		a.rep.mu.Lock()
		a.rep.last = &rep
		a.rep.mu.Unlock()
		// A cancelled job keeps its result (the interface shows what ran).
		return rep, ctx.Err()
	})
}

// repairVerdict joins the two steps into one sentence for the user, without
// promising more than the tools reported.
func repairVerdict(steps []RepairStep, cancelled bool) (string, string) {
	by := map[string]repair.Outcome{}
	for _, s := range steps {
		by[s.ID] = s.Outcome
	}
	dism, sfc := by["dism"], by["sfc"]
	switch {
	case cancelled:
		return RepairStopped, "Reparo cancelado. O que já tinha sido feito fica; rode de novo para concluir."
	case sfc == repair.Unrepaired:
		return RepairUnfixed, "O Windows ainda tem arquivos de sistema danificados que o SFC não conseguiu corrigir. Veja os detalhes técnicos; se persistir, a Microsoft indica a atualização de reparo ou a reinstalação do Windows."
	case sfc == repair.Repaired && dism == repair.Failed:
		return RepairFixed, "O SFC achou arquivos de sistema danificados e os repôs, mas o DISM não terminou: " + firstFailure(steps)
	case dism == repair.Failed || sfc == repair.Failed:
		return RepairFailedRun, "O reparo não terminou: " + firstFailure(steps)
	case dism == repair.Reboot || sfc == repair.Reboot:
		return RepairRestart, "O reparo terminou, mas o Windows pede um reinício para concluir. Reinicie o PC e rode o reparo de novo para conferir."
	case sfc == repair.Unknown:
		return RepairInexact, "O reparo terminou sem erro, mas o Axion não reconheceu a mensagem final do SFC. Veja os detalhes técnicos."
	case sfc == repair.Repaired:
		return RepairFixed, "O SFC achou arquivos de sistema danificados e os repôs. Reinicie o PC para garantir que tudo foi carregado de novo."
	}
	return RepairClean, "O DISM e o SFC terminaram sem achar arquivos de sistema danificados."
}

func firstFailure(steps []RepairStep) string {
	for _, s := range steps {
		if s.Outcome == repair.Failed {
			return s.Summary
		}
	}
	return "veja os detalhes técnicos."
}

package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"axion/internal/programs"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

var srcUninstall = tweaks.Source{Title: "Microsoft Support — Desinstalar ou remover aplicativos e programas no Windows", URL: "https://support.microsoft.com/windows/uninstall-or-remove-apps-and-programs-in-windows-4b55f974-2cc6-2d2b-d092-5905080eaf98"}

// ProgramsView is the Programas page.
type ProgramsView struct {
	Programs   []programs.Program `json:"programs"`
	Quarantine []programs.Batch   `json:"quarantine"`
	// Recent: programs uninstalled through Axion in this session whose
	// leftovers were not moved yet ("Procurar sobras de novo").
	Recent   []RecentUninstall `json:"recent,omitempty"`
	Elevated bool              `json:"elevated"`
	Honest   string            `json:"honest"`
	Sources  []tweaks.Source   `json:"sources"`
}

// RecentUninstall is a program uninstalled in this session.
type RecentUninstall struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	At    time.Time `json:"at"`
	Found int       `json:"found"` // leftovers found by the last search
}

type pendingLeftovers struct {
	snap  programs.Snapshot
	items []programs.Leftover
	at    time.Time
}

type programState struct {
	mu      sync.Mutex
	pending map[string]*pendingLeftovers // by program entry key (lower case)
}

func (a *App) quarantineRoot() string { return filepath.Join(a.P.DataDir, "quarentena") }

// Programs lists installed programs and the quarantine.
func (a *App) Programs() ProgramsView {
	return ProgramsView{Programs: programs.List(a.P.Reg), Quarantine: programs.Batches(a.quarantineRoot(), a.P.Key), Recent: a.recentUninstalls(), Elevated: a.P.Native.IsElevated(),
		Sources: []tweaks.Source{srcUninstall},
		Honest: "O Axion roda o desinstalador do próprio programa (o mesmo de Configurações > Aplicativos) e depois procura o que ele deixou para trás: pastas, configurações no Registro, entradas de inicialização e tarefas. " +
			"Só entra na lista o que tem o nome do programa ou estava na pasta dele; nada é apagado na hora: vai para a quarentena por 30 dias e pode ser restaurado."}
}

// UninstallRequest names a program.
type UninstallRequest struct {
	ID string `json:"id"`
}

// UninstallResult is returned by the uninstall job.
type UninstallResult struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	Removed   bool                `json:"removed"`
	Note      string              `json:"note,omitempty"`
	Leftovers []programs.Leftover `json:"leftovers"`
}

func (a *App) setPending(snap programs.Snapshot, items []programs.Leftover) {
	a.progs.mu.Lock()
	defer a.progs.mu.Unlock()
	if a.progs.pending == nil {
		a.progs.pending = map[string]*pendingLeftovers{}
	}
	a.progs.pending[strings.ToLower(snap.Program.ID)] = &pendingLeftovers{snap: snap, items: items, at: time.Now()}
}

// recentUninstalls lists the programs uninstalled in this session whose
// leftovers can still be searched (the program is no longer registered).
func (a *App) recentUninstalls() []RecentUninstall {
	a.progs.mu.Lock()
	var list []RecentUninstall
	for _, p := range a.progs.pending {
		list = append(list, RecentUninstall{ID: p.snap.Program.ID, Name: p.snap.Program.Name, At: p.at, Found: len(p.items)})
	}
	a.progs.mu.Unlock()
	out := list[:0]
	for _, r := range list {
		if ok, err := a.P.Reg.KeyExists(r.ID); err == nil && !ok {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

// StartUninstall runs the program's own uninstaller and then looks for leftovers.
func (a *App) StartUninstall(req UninstallRequest) (Job, error) {
	if err := a.requireAdmin("desinstalar programas"); err != nil {
		return Job{}, err
	}
	prog := programs.Find(programs.List(a.P.Reg), req.ID)
	if prog == nil {
		return Job{}, fmt.Errorf("%w: programa não encontrado; atualize a lista", errBadRequest)
	}
	if prog.NoRemove {
		return Job{}, fmt.Errorf("%w: %s não permite desinstalação", errBadRequest, prog.Name)
	}
	p := *prog
	exe, args := programs.Command(p, sys.TrustedEnv(a.P.Native.Paths()))
	if exe == "" {
		return Job{}, fmt.Errorf("%w: %s não tem desinstalador registrado", errBadRequest, p.Name)
	}
	asUser := programs.RunsAsUser(p, exe, a.P.Native.Paths())
	return a.Jobs.Start("uninstall", "Desinstalando "+p.Name, true, func(ctx context.Context, c *JobCtl) (any, error) {
		snap := programs.Snapshot{Program: p}
		a.setPending(snap, nil) // lets "Procurar sobras" work even if the wait is cancelled
		res := UninstallResult{ID: p.ID, Name: p.Name}
		c.Progress(0.05, "Abrindo o desinstalador de "+p.Name+": siga as telas dele")
		log.Printf("desinstalar %q: %s %s (como usuário: %v)", p.Name, exe, args, asUser)
		gone := func() bool { ok, err := a.P.Reg.KeyExists(p.ID); return err == nil && !ok }
		exited := make(chan int, 1)
		started := time.Now()
		taskPath := ""
		if asUser {
			// Per-user programs: a one-shot task runs the uninstaller with the
			// user's normal rights (never elevated through Axion).
			taskPath = `\Axion\` + uninstallTaskPrefix + time.Now().Format("150405")
			if err := a.P.Tasks.Register(taskPath, "Axion: desinstalador de "+p.Name+" com os direitos do usuário", exe, args, sys.TriggerRegistration, "", 0, false); err != nil {
				return nil, fmt.Errorf("não foi possível abrir o desinstalador: %w", err)
			}
			defer func() { _ = a.P.Tasks.Delete(taskPath) }()
		} else {
			go func() {
				code, err := a.P.Native.RunVisible(ctx, exe, args)
				if err != nil && ctx.Err() == nil {
					c.Logf("desinstalador: %v", err)
					code = -1
				}
				exited <- code
			}()
		}
		var exitAt time.Time
		quick := false
		for !gone() {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case code := <-exited:
				exitAt = time.Now()
				// Inno Setup and NSIS copy themselves to TEMP and restart: the
				// first process ends in seconds while the real one keeps going.
				quick = exitAt.Sub(started) < 5*time.Second && code == 0
				c.Logf("o desinstalador terminou (código %d)", code)
			case <-time.After(1500 * time.Millisecond):
			}
			switch {
			case !exitAt.IsZero() && !quick && time.Since(exitAt) > 20*time.Second:
				res.Note = "O desinstalador fechou, mas o programa continua instalado (a desinstalação foi cancelada ou falhou)."
			case time.Since(started) > 45*time.Minute:
				res.Note = "O desinstalador não terminou em 45 minutos."
			}
			if res.Note != "" {
				c.Logf("%s", res.Note)
				return res, nil
			}
			c.Step("Aguardando o desinstalador de " + p.Name + " (se você cancelou a desinstalação, clique em Cancelar)")
		}
		res.Removed = true
		c.Progress(0.8, "Procurando o que ficou para trás")
		time.Sleep(a.settleDelay())
		res.Leftovers = programs.FindLeftovers(a.P, snap, programs.List(a.P.Reg), sys.TrustedEnv(a.P.Native.Paths()))
		a.setPending(snap, res.Leftovers)
		c.Logf("%s desinstalado; %d sobra(s) encontradas", p.Name, len(res.Leftovers))
		a.refreshBackground()
		return res, nil
	})
}

// settleDelay gives uninstallers a moment to finish deleting files.
func (a *App) settleDelay() time.Duration {
	if a.P.IsMock {
		return 0
	}
	return 3 * time.Second
}

// QuarantineRequest moves chosen leftovers of an uninstalled program.
type QuarantineRequest struct {
	ID    string   `json:"id"` // program entry key
	Items []string `json:"items"`
}

// QuarantineLeftovers moves the chosen leftovers found after an uninstall.
// Only items found by Axion for that program are accepted.
func (a *App) QuarantineLeftovers(req QuarantineRequest) (*programs.Batch, error) {
	if err := a.requireAdmin("remover sobras"); err != nil {
		return nil, err
	}
	a.progs.mu.Lock()
	pend := a.progs.pending[strings.ToLower(req.ID)]
	a.progs.mu.Unlock()
	if pend == nil {
		return nil, fmt.Errorf("%w: procure as sobras de novo", errBadRequest)
	}
	want := map[string]bool{}
	for _, id := range req.Items {
		want[id] = true
	}
	// The list may be old (the program reinstalled in the same folder since,
	// or another one now using it): look again and keep only what is still a
	// leftover now.
	if programInstalled(pend.snap.Program.ID, programs.List(a.P.Reg), sys.TrustedEnv(a.P.Native.Paths())) {
		return nil, fmt.Errorf("%w: %s voltou a aparecer como instalado; nada foi movido", errBadRequest, pend.snap.Program.Name)
	}
	still := map[string]bool{}
	for _, l := range programs.FindLeftovers(a.P, pend.snap, programs.List(a.P.Reg), sys.TrustedEnv(a.P.Native.Paths())) {
		still[l.ID] = true
	}
	var chosen []programs.Leftover
	for _, l := range pend.items {
		if want[l.ID] && still[l.ID] {
			chosen = append(chosen, l)
		}
	}
	if len(chosen) == 0 {
		return nil, fmt.Errorf("%w: marque pelo menos um item para mover para a quarentena", errBadRequest)
	}
	root := a.quarantineRoot()
	if err := a.P.Native.ProtectDir(root); err != nil {
		return nil, fmt.Errorf("pasta da quarentena: %w", err)
	}
	// Moved folders keep their own permissions, links and compressed files
	// while in the quarantine, so a restore puts them back exactly; Axion
	// only moves them (never reads them as its own data).
	b, err := programs.Quarantine(a.P, root, pend.snap.Program.Name, chosen, time.Now())
	if err != nil {
		return b, err
	}
	a.progs.mu.Lock()
	delete(a.progs.pending, strings.ToLower(req.ID))
	a.progs.mu.Unlock()
	log.Printf("quarentena %s: %d item(ns), %d erro(s)", b.ID, len(b.Items), len(b.Errors))
	return b, nil
}

// RescanLeftovers looks again for leftovers of a program uninstalled in this
// session (e.g. after the wait was cancelled).
func (a *App) RescanLeftovers(req UninstallRequest) (UninstallResult, error) {
	a.progs.mu.Lock()
	pend := a.progs.pending[strings.ToLower(req.ID)]
	a.progs.mu.Unlock()
	if pend == nil {
		return UninstallResult{}, fmt.Errorf("%w: desinstale o programa pelo Axion primeiro", errBadRequest)
	}
	if ok, _ := a.P.Reg.KeyExists(pend.snap.Program.ID); ok {
		return UninstallResult{}, errors.New("o programa ainda aparece como instalado")
	}
	items := programs.FindLeftovers(a.P, pend.snap, programs.List(a.P.Reg), sys.TrustedEnv(a.P.Native.Paths()))
	a.setPending(pend.snap, items)
	return UninstallResult{ID: pend.snap.Program.ID, Name: pend.snap.Program.Name, Removed: true, Leftovers: items}, nil
}

// BatchRequest names a quarantine batch.
type BatchRequest struct {
	Batch string `json:"batch"`
}

// RestoreBatch puts a quarantine batch back.
func (a *App) RestoreBatch(req BatchRequest) (*programs.Batch, error) {
	if err := a.requireAdmin("restaurar da quarentena"); err != nil {
		return nil, err
	}
	return programs.Restore(a.P, a.quarantineRoot(), req.Batch)
}

// DeleteBatch removes a quarantine batch for good.
func (a *App) DeleteBatch(req BatchRequest) error {
	return programs.Delete(a.quarantineRoot(), req.Batch)
}

// programInstalled reports whether the entry is installed again: listed
// with an uninstaller that exists on disk (an orphan entry left by the
// uninstall has none). An MSI entry counts as installed while it is listed:
// Windows Installer removes it with the product.
func programInstalled(id string, list []programs.Program, env func(string) string) bool {
	for _, p := range list {
		if !strings.EqualFold(p.ID, id) {
			continue
		}
		if p.MSI {
			return true
		}
		exe, _ := programs.Command(p, env)
		if exe == "" || !filepath.IsAbs(exe) {
			return false
		}
		_, err := os.Stat(exe)
		return err == nil
	}
	return false
}

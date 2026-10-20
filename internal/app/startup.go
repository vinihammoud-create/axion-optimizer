package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"axion/internal/catalog"
	"axion/internal/facts"
	"axion/internal/startup"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

var srcStartupApps = tweaks.Source{Title: "Microsoft Support — Configurar aplicativos para iniciar automaticamente (Gerenciador de Tarefas > Aplicativos de inicialização)", URL: "https://support.microsoft.com/windows/configure-startup-applications-in-windows-115a420a-0bff-4a6f-90e0-1934c844e473"}

// StartupView is the Inicialização page.
type StartupView struct {
	Items    []startup.Item  `json:"items"`
	Enabled  int             `json:"enabled"`
	Suggest  int             `json:"suggest"`  // enabled items Axion suggests turning off
	NewCount int             `json:"newCount"` // entries that were not there at the last visit
	Elevated bool            `json:"elevated"`
	Honest   string          `json:"honest"`
	Sources  []tweaks.Source `json:"sources"`
}

// Startup lists what starts with Windows.
func (a *App) Startup() StartupView {
	v := StartupView{Items: startup.List(a.P, sys.TrustedEnv(a.P.Native.Paths())), Elevated: a.P.Native.IsElevated(), Sources: []tweaks.Source{srcStartupApps},
		Honest: "Programas que iniciam com o Windows custam tempo de boot, memória e CPU em segundo plano; não mudam o FPS por si só, mas vários juntos disputam recursos com o jogo. " +
			"Desativar aqui funciona como no Gerenciador de Tarefas: a entrada não é apagada e dá para ligar de novo a qualquer momento."}
	a.markNewStartup(v.Items)
	for i := range v.Items {
		it := &v.Items[i]
		// The current state was put there by Axion (and is undone in the Histórico).
		it.ByAxion = a.Engine.J.ActiveFor(catalog.StartupID(it.ID, it.Enabled)) != nil
	}
	for _, it := range v.Items {
		if it.New {
			v.NewCount++
		}
		if it.Enabled {
			v.Enabled++
			if it.Verdict == startup.Disable {
				v.Suggest++
			}
		}
	}
	return v
}

// StartupRequest turns one or more entries on or off.
type StartupRequest struct {
	IDs     []string `json:"ids"`
	Enabled bool     `json:"enabled"`
}

// SetStartup applies the request and returns the new list. The change goes
// through the tweak engine: the previous value is written to the journal
// before the machine changes, the item shows in the Histórico and the undo
// brings back the exact bytes.
func (a *App) SetStartup(req StartupRequest) (StartupView, error) {
	if len(req.IDs) == 0 {
		return StartupView{}, fmt.Errorf("%w: nada selecionado", errBadRequest)
	}
	env := sys.TrustedEnv(a.P.Native.Paths())
	state := func() map[string]startup.Item {
		m := map[string]startup.Item{}
		for _, it := range startup.List(a.P, env) {
			m[it.ID] = it
		}
		return m
	}
	known := state()
	ctx, f := context.Background(), a.Facts() // f may be nil: the engine accepts it
	const label = "Inicialização"
	var failed, applyIDs []string
	for _, id := range req.IDs {
		it, ok := known[id]
		if !ok {
			failed = append(failed, id+" (não encontrado)")
			continue
		}
		// The opposite state was written by Axion: undoing it brings back the
		// exact bytes from before, which are the state asked for (Axion only
		// records an entry when it flipped the state). Forced: the user asks
		// for it explicitly, even if Task Manager touched the value since.
		// A new entry on top of it would leave two entries over one value.
		if other := catalog.StartupID(id, !req.Enabled); a.Engine.J.ActiveFor(other) != nil {
			undone := true
			for _, r := range a.Engine.RevertTweaks(ctx, f, []string{other}, tweaks.RevertOpts{Force: true, Log: log.Printf}) {
				if r.Outcome != tweaks.OutReverted && r.Outcome != tweaks.OutNothing {
					failed = append(failed, r.Title+": "+r.Message)
					undone = false
				}
			}
			if !undone {
				continue
			}
			it, ok = state()[id]
			if !ok {
				failed = append(failed, id+" (não encontrado)")
				continue
			}
		}
		if it.Enabled == req.Enabled {
			continue // already as asked: nothing to write
		}
		applyIDs = append(applyIDs, catalog.StartupID(id, req.Enabled))
	}
	if len(applyIDs) > 0 {
		res := a.Engine.Apply(ctx, f, applyIDs, tweaks.ApplyOpts{Label: label, Log: log.Printf})
		for _, r := range res {
			if r.Outcome != tweaks.OutApplied && r.Outcome != tweaks.OutAlready {
				failed = append(failed, r.Title+": "+r.Message)
			}
		}
		a.recordBatch(batchLog(ApplyRequest{IDs: applyIDs, Label: label}, nil, res))
	}
	log.Printf("inicialização: %v → ligado=%v (falhas: %d)", req.IDs, req.Enabled, len(failed))
	v := a.Startup()
	// The diagnostic counts startup items: keep it in step with the change.
	sum := facts.StartupSummary{Enabled: v.Enabled, Disabled: len(v.Items) - v.Enabled}
	a.patchFacts(func(f *facts.Facts) { f.Startup = sum })
	if len(failed) > 0 {
		return v, fmt.Errorf("não foi possível alterar: %s", strings.Join(failed, "; "))
	}
	return v, nil
}

// startupVisit remembers, for this run of the program, which startup entries
// counted as new at the first visit. Nothing runs in the background: the file
// in DataDir is read and rewritten only when the page asks for the list.
type startupVisit struct {
	mu        sync.Mutex
	started   bool
	baselined bool
	fresh     map[string]bool
	last      map[string]bool // the list of the previous request of this run
}

func (a *App) startupSeenPath() string { return filepath.Join(a.P.DataDir, "inicializacao-visto.json") }

// markNewStartup sets Item.New on the entries that appeared since the last
// visit. The first visit ever (no file, or a damaged one) only records the
// current list as the baseline and marks nothing. An entry stays marked until
// the program is closed; at the next opening it is part of the baseline.
func (a *App) markNewStartup(items []startup.Item) {
	vs := &a.startVisit
	vs.mu.Lock()
	defer vs.mu.Unlock()
	cur := startup.Keys(items)
	var seen map[string]bool
	stored := false
	if !vs.started {
		vs.started = true
		vs.fresh = map[string]bool{}
		if b, err := os.ReadFile(a.startupSeenPath()); err == nil {
			seen, vs.baselined = startup.ParseSeen(b)
		}
		stored = vs.baselined
	} else {
		// Later requests of the same run: what is not marked yet and not in
		// the list kept from the earlier request is new too.
		seen = vs.last
	}
	for k := range startup.NewSince(seen, vs.baselined, items) {
		vs.fresh[k] = true
	}
	for k := range vs.fresh {
		if !cur[k] {
			delete(vs.fresh, k) // the entry is gone
		}
	}
	for i := range items {
		items[i].New = vs.fresh[startup.Key(items[i].ID)]
	}
	// A list that came back empty is more likely a failed reading than a
	// machine with nothing in startup: keep the old baseline then.
	if len(cur) == 0 && len(seen) > 0 {
		return
	}
	vs.last = cur
	if stored && sameKeys(seen, cur) {
		return
	}
	vs.baselined = true
	if err := saveFile(a.startupSeenPath(), func() ([]byte, error) { return startup.EncodeSeen(cur, time.Now()) }); err != nil {
		log.Printf("lista de inicialização vista não foi salva: %v", err)
	}
}

func sameKeys(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"axion/internal/cleanup"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

var srcShaderCache = tweaks.Source{Title: "Microsoft DirectX Blog — shaders compilados ficam em cache entre as sessões; compilar de novo causa carregamento longo e travadas", URL: "https://devblogs.microsoft.com/directx/introducing-advanced-shader-delivery/"}

// CleanupItem is a target with its current measurement.
type CleanupItem struct {
	cleanup.Target
	Size cleanup.Measure `json:"size"`
}

// CleanupView is the Limpeza page.
type CleanupView struct {
	Items      []CleanupItem  `json:"items"`
	MeasuredAt time.Time      `json:"measuredAt"`
	Free       uint64         `json:"free"`  // system drive free bytes
	Total      uint64         `json:"total"` // system drive size
	NotDone    []CheckItem    `json:"notDone"`
	Last       *CleanupReport `json:"last,omitempty"`
}

// CleanupReport is the outcome of a cleanup run.
type CleanupReport struct {
	At      time.Time        `json:"at"`
	Results []cleanup.Result `json:"results"`
	Freed   int64            `json:"freed"`
	Skipped int              `json:"skipped"`
}

type cleanupCache struct {
	mu   sync.Mutex
	view *CleanupView
	at   time.Time
	last *CleanupReport
}

// What the page deliberately leaves alone, with the reason.
var cleanupNotDone = []CheckItem{
	{ID: "shader", Title: "Cache de shaders (DirectX, NVIDIA, AMD e dos jogos)", Status: "info", Evidence: "alta", Basis: "Microsoft DirectX Blog",
		Advice:  "Guarda os shaders já compilados para a sua placa. Apagar força o jogo a compilar de novo: carregamentos longos e travadas nas primeiras partidas. Só faz sentido para resolver um problema específico.",
		Sources: []tweaks.Source{srcShaderCache}},
	{ID: "prefetch", Title: "Prefetch e histórico de eventos", Status: "info", Evidence: "alta", Basis: "O que não fazemos",
		Advice: "O Prefetch acelera a abertura de programas e os logs de eventos servem para diagnosticar travamentos (veja \"O que não fazemos\")."},
	{ID: "browser", Title: "Cache dos navegadores", Status: "info", Evidence: "média", Basis: "sem ganho de desempenho",
		Advice: "Não deixa o PC mais rápido: o navegador baixa tudo de novo e as páginas abrem mais devagar. Use a opção do próprio navegador se precisar."},
	{ID: "softdist", Title: "Pasta de downloads do Windows Update (SoftwareDistribution)", Status: "info", Evidence: "média", Basis: "Microsoft Support",
		Advice:  "Apagar com o Windows Update em uso pode interromper uma atualização. Para liberar espaço use Configurações > Armazenamento > Arquivos temporários.",
		Sources: []tweaks.Source{{Title: "Microsoft Support — Liberar espaço em disco", URL: "https://support.microsoft.com/windows/free-up-drive-space-in-windows-85529ccb-c365-490d-b548-831022bc9b32"}}},
}

// Cleanup measures every target (cached for a minute).
func (a *App) Cleanup(force bool) CleanupView {
	a.clean.mu.Lock()
	if !force && a.clean.view != nil && time.Since(a.clean.at) < time.Minute {
		v := *a.clean.view
		v.Last = a.clean.last
		a.clean.mu.Unlock()
		return v
	}
	a.clean.mu.Unlock()
	paths := a.P.Native.Paths()
	now := time.Now()
	v := CleanupView{MeasuredAt: now, NotDone: cleanupNotDone}
	keep := a.pendingRenames()
	for _, t := range cleanup.Targets(paths) {
		t.Keep = keep
		it := CleanupItem{Target: t}
		switch t.Kind {
		case cleanup.KindFiles, cleanup.KindLink:
			it.Size = cleanup.Scan(t, now, 2*time.Second)
		case cleanup.KindRecycle:
			// The shell empties the recycle bin of the account the process
			// runs as: elevated with another account, that is not the user's.
			if paths.ElevatedAs != "" {
				it.Size = cleanup.Measure{ID: t.ID, Present: false, Note: "o Axion está rodando com outra conta de administrador (" + paths.ElevatedAs + "): esvazie a lixeira pela área de trabalho"}
				break
			}
			b, n, err := a.P.Native.RecycleBin()
			it.Size = cleanup.Measure{ID: t.ID, Bytes: int64(b), Files: int(n), Present: err == nil}
			if err != nil {
				it.Size.Note = "não foi possível medir: " + err.Error()
			}
		case cleanup.KindDISM:
			it.Size = cleanup.Measure{ID: t.ID, Present: a.P.Native.IsElevated(), Note: "o tamanho só é conhecido depois da limpeza"}
		}
		v.Items = append(v.Items, it)
	}
	if paths.SystemDrive != "" {
		v.Total, v.Free, _ = a.P.Native.DiskSpace(paths.SystemDrive + `\`)
	}
	a.clean.mu.Lock()
	a.clean.view, a.clean.at = &v, now
	v.Last = a.clean.last
	a.clean.mu.Unlock()
	return v
}

// pendingRenames lists the files that a pending restart will still move into
// place (Session Manager\PendingFileRenameOperations, written by updates and
// installers that could not replace a file in use): deleting one of them
// now would break the update waiting for the restart. Pending deletions
// (empty destination) are not listed: those files go away anyway.
func (a *App) pendingRenames() map[string]bool {
	keep := map[string]bool{}
	for _, name := range []string{"PendingFileRenameOperations", "PendingFileRenameOperations2"} {
		v, err := a.P.Reg.GetValue(`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager`, name)
		if err != nil || !v.Exists {
			continue
		}
		for i := 0; i+1 < len(v.Strs); i += 2 {
			if strings.TrimSpace(v.Strs[i+1]) == "" {
				continue
			}
			src := strings.TrimPrefix(strings.TrimSpace(v.Strs[i]), `\??\`)
			if src != "" {
				keep[strings.ToLower(filepath.Clean(src))] = true
			}
		}
	}
	return keep
}

// CleanupRequest selects targets.
type CleanupRequest struct {
	IDs []string `json:"ids"`
}

// StartCleanup deletes the chosen targets.
func (a *App) StartCleanup(req CleanupRequest) (Job, error) {
	chosen := map[string]bool{}
	for _, id := range req.IDs {
		chosen[strings.TrimSpace(id)] = true
	}
	var targets []cleanup.Target
	keep := a.pendingRenames()
	for _, t := range cleanup.Targets(a.P.Native.Paths()) {
		if chosen[t.ID] && t.Kind != cleanup.KindLink {
			t.Keep = keep
			targets = append(targets, t)
		}
	}
	if len(targets) == 0 {
		return Job{}, fmt.Errorf("%w: nada selecionado", errBadRequest)
	}
	return a.Jobs.Start("cleanup", "Limpando o disco", true, func(ctx context.Context, c *JobCtl) (any, error) {
		rep := CleanupReport{At: time.Now()}
		for i, t := range targets {
			if ctx.Err() != nil {
				break
			}
			c.Progress(float64(i)/float64(len(targets))*0.95, t.Title)
			var r cleanup.Result
			switch t.Kind {
			case cleanup.KindFiles:
				r = cleanup.Clean(t, time.Now())
			case cleanup.KindRecycle:
				r = cleanup.Result{ID: t.ID}
				if a.P.Native.Paths().ElevatedAs != "" {
					r.Errors = append(r.Errors, "a lixeira não foi esvaziada: o Axion está rodando com outra conta de administrador")
					break
				}
				before, _, _ := a.P.Native.RecycleBin()
				if err := a.P.Native.EmptyRecycleBin(); err != nil {
					r.Errors = append(r.Errors, err.Error())
				} else {
					r.Freed = int64(before)
				}
			case cleanup.KindDISM:
				r = a.componentCleanup(ctx, c)
			}
			rep.Results = append(rep.Results, r)
			rep.Freed += r.Freed
			rep.Skipped += r.Skipped
			c.Logf("%s: %s liberados, %d apagados, %d mantidos (em uso)", t.Title, gib64(r.Freed), r.Deleted, r.Skipped)
			for _, e := range r.Errors {
				c.Logf("  %s", e)
			}
		}
		a.clean.mu.Lock()
		a.clean.last, a.clean.view = &rep, nil
		a.clean.mu.Unlock()
		c.Progress(0.97, "Atualizando o diagnóstico")
		a.refreshAfterChange(ctx)
		return rep, nil
	})
}

// componentCleanup runs DISM /StartComponentCleanup and measures the free
// space it gave back.
func (a *App) componentCleanup(ctx context.Context, c *JobCtl) cleanup.Result {
	r := cleanup.Result{ID: "winsxs"}
	if !a.P.Native.IsElevated() {
		r.Errors = append(r.Errors, "exige administrador")
		return r
	}
	drive := a.P.Native.Paths().SystemDrive + `\`
	_, before, _ := a.P.Native.DiskSpace(drive)
	c.Logf("DISM /Online /Cleanup-Image /StartComponentCleanup (pode levar vários minutos)")
	res, err := a.P.Run.Run(ctx, "Dism.exe", []string{"/Online", "/Cleanup-Image", "/StartComponentCleanup"}, sys.RunOpts{Timeout: 60 * time.Minute,
		OnLine: func(line string) {
			if l := strings.TrimSpace(line); l != "" && strings.Contains(l, "%") {
				c.Step("Componentes do Windows: " + l)
			}
		}})
	if err != nil {
		r.Errors = append(r.Errors, err.Error())
		return r
	}
	if res.ExitCode != 0 {
		r.Errors = append(r.Errors, fmt.Sprintf("DISM terminou com o código %d", res.ExitCode))
		return r
	}
	if _, after, err := a.P.Native.DiskSpace(drive); err == nil && after > before {
		r.Freed = int64(after - before)
	}
	r.Deleted = 1
	return r
}

func gib64(b int64) string {
	switch {
	case b >= 1<<30:
		return strings.Replace(fmt.Sprintf("%.1f GB", float64(b)/(1<<30)), ".", ",", 1)
	case b >= 1<<20:
		return fmt.Sprintf("%d MB", b>>20)
	case b > 0:
		return fmt.Sprintf("%d KB", (b+1023)>>10)
	}
	return "0 KB"
}

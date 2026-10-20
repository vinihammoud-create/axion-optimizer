package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"axion/internal/drivers"
	"axion/internal/games"
)

// UnusedGames lists the installed Steam games nobody has opened for half a
// year (critério do Axion), largest first. Nothing is removed here.
func (a *App) UnusedGames() []games.Unused {
	gl, _ := a.Games()
	out := games.UnusedGames(gl, time.Now(), games.UnusedDays)
	if out == nil {
		out = []games.Unused{}
	}
	return out
}

// OldPackagesView is the answer of GET /api/drivers/old.
type OldPackagesView struct {
	// Supported is false when this Windows cannot list the driver store with
	// "pnputil /format xml": nothing is then said about old packages.
	Supported bool                 `json:"supported"`
	Packages  []drivers.OldPackage `json:"packages"`
}

// OldDriverPackages lists the display driver packages that an update left
// behind in the driver store. Read only.
func (a *App) OldDriverPackages(ctx context.Context) OldPackagesView {
	pkgs, ok := drivers.ListPackages(ctx, a.P)
	v := OldPackagesView{Supported: ok, Packages: []drivers.OldPackage{}}
	if ok {
		v.Packages = append(v.Packages, drivers.OldDisplayPackages(pkgs)...)
	}
	return v
}

// OldPackagesDeleteRequest names the packages the user confirmed.
type OldPackagesDeleteRequest struct {
	Published []string `json:"published"`
}

// OldPackagesDeleteResult is the result of the delete job.
type OldPackagesDeleteResult struct {
	Removed []string `json:"removed"`
	Failed  []string `json:"failed"`
}

// oldPackagesChosen matches what the user confirmed against the packages that
// are old right now. The page may be stale (an update can have installed a
// driver since it was drawn), so a name that is not in today's list is refused
// instead of being deleted on trust.
func (a *App) oldPackagesChosen(published []string) ([]drivers.OldPackage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pkgs, ok := drivers.ListPackages(ctx, a.P)
	if !ok {
		return nil, fmt.Errorf("%w: este Windows não lista os pacotes de driver (pnputil /format xml)", errBadRequest)
	}
	current := map[string]drivers.OldPackage{}
	for _, o := range drivers.OldDisplayPackages(pkgs) {
		current[strings.ToLower(o.Published)] = o
	}
	var out []drivers.OldPackage
	seen := map[string]bool{}
	for _, name := range published {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		o, ok := current[key]
		if !ok {
			return nil, fmt.Errorf("%w: a lista de pacotes mudou (%s); abra a página de novo", errBadRequest, name)
		}
		out = append(out, o)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: nada selecionado", errBadRequest)
	}
	return out, nil
}

// StartOldPackagesDelete deletes the old display driver packages the user
// confirmed, one by one, never touching a package a device uses (see
// drivers.RemovePackage). A failure on one package does not stop the others.
func (a *App) StartOldPackagesDelete(req OldPackagesDeleteRequest) (Job, error) {
	if err := a.requireAdmin("excluir pacotes de driver"); err != nil {
		return Job{}, err
	}
	chosen, err := a.oldPackagesChosen(req.Published)
	if err != nil {
		return Job{}, err
	}
	return a.Jobs.Start("drivers-old-delete", "Removendo pacotes de vídeo antigos", true, func(ctx context.Context, c *JobCtl) (any, error) {
		res := OldPackagesDeleteResult{Removed: []string{}, Failed: []string{}}
		for i, o := range chosen {
			if ctx.Err() != nil {
				break
			}
			c.Progress(float64(i)/float64(len(chosen)), o.Published+" ("+o.Provider+" "+o.Version+")")
			if err := drivers.RemovePackage(ctx, a.P, o.Published); err != nil {
				c.Logf("%s: não foi excluído (%v)", o.Published, err)
				res.Failed = append(res.Failed, o.Published)
				continue
			}
			c.Logf("excluído: %s (%s, versão %s)", o.Published, o.Provider, o.Version)
			res.Removed = append(res.Removed, o.Published)
		}
		return res, nil
	})
}

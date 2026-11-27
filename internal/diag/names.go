package diag

import (
	"strings"
	"sync"

	"axion/internal/catalog"
	"axion/internal/tweaks"
)

var srcSpectrePerf = tweaks.Source{Title: "Microsoft Security Blog (2018) — impacto das mitigações de Spectre e Meltdown no desempenho do Windows: pequeno do Skylake em diante, maior no Haswell ou anterior", URL: "https://www.microsoft.com/en-us/security/blog/2018/01/09/understanding-the-performance-impact-of-spectre-and-meltdown-mitigations-on-windows-systems/"}

var (
	titlesOnce sync.Once
	titles     map[string]string
)

// tweakNames turns optimization IDs into the names the person sees in
// Otimizar: the title the engine put in the state (it also knows the items
// built per PC: startup entries, DNS, active hours, game profiles), then the
// static catalog; an ID neither knows stays as it is rather than being
// dropped.
func tweakNames(ids []string, st map[string]tweaks.State) string {
	titlesOnce.Do(func() {
		titles = map[string]string{}
		for _, t := range catalog.All() {
			titles[t.ID] = t.Title
		}
	})
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id
		if t := st[id].Title; t != "" {
			out[i] = t
		} else if t := titles[id]; t != "" {
			out[i] = t
		}
	}
	return strings.Join(out, "; ")
}

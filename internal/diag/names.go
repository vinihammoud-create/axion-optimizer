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
// Otimizar. An ID outside the catalog (startup entries are built per PC)
// stays as it is rather than being dropped.
func tweakNames(ids []string) string {
	titlesOnce.Do(func() {
		titles = map[string]string{}
		for _, t := range catalog.All() {
			titles[t.ID] = t.Title
		}
	})
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id
		if t := titles[id]; t != "" {
			out[i] = t
		}
	}
	return strings.Join(out, "; ")
}

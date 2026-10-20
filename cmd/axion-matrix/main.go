// Command axion-matrix prints the evidence matrix (every optimization Axion
// offers and every one it refuses) as Markdown.
package main

import (
	"fmt"
	"os"
	"strings"

	"axion/internal/catalog"
	"axion/internal/tweaks"
)

func esc(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ") }

func main() {
	w := os.Stdout
	fmt.Fprintln(w, "# Axion Optimizer — matriz de evidências")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Gerada a partir do catálogo do programa. **Evidência alta**: documentado pelo fabricante (Microsoft, NVIDIA, AMD, Intel) ou medido de forma consistente por testes independentes. **Média**: mecanismo real e documentado, benefício depende do cenário. **Baixa**: mecanismo plausível, benefício pequeno ou inconsistente (nunca entra no modo recomendado; no modo Completo aparece desmarcado, para você decidir).")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "A recomendação final é calculada para cada PC na análise (o mesmo item pode ser recomendado em uma máquina e bloqueado em outra).")
	byCat := map[string][]*tweaks.Tweak{}
	for _, t := range catalog.All() {
		byCat[t.Category] = append(byCat[t.Category], t)
	}
	for _, c := range tweaks.Categories {
		list := byCat[c.ID]
		if len(list) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n## %s\n\n%s\n\n", c.Title, c.Description)
		fmt.Fprintln(w, "| Otimização | Evidência | Risco | Vale após | 1 clique | Fontes |")
		fmt.Fprintln(w, "|---|---|---|---|---|---|")
		for _, t := range list {
			var src []string
			for _, s := range t.Sources {
				src = append(src, fmt.Sprintf("[%s](%s)", esc(s.Title), s.URL))
			}
			one := "não"
			switch {
			case t.Recommendable():
				one = "sim (se recomendado)"
			case t.OptionalEligible():
				one = "opcional (modo Completo)"
			}
			fmt.Fprintf(w, "| **%s**<br>`%s`<br>%s | %s | %s | %s | %s | %s |\n", esc(t.Title), t.ID, esc(t.Summary), t.Evidence, t.Risk, t.Reboot, one, strings.Join(src, "<br>"))
		}
	}
	fmt.Fprintln(w, "\n## O que o Axion não faz (e por quê)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Ajuste | Veredito | Motivo | Fontes |")
	fmt.Fprintln(w, "|---|---|---|---|")
	for _, x := range catalog.Excluded() {
		var src []string
		for _, s := range x.Sources {
			src = append(src, fmt.Sprintf("[%s](%s)", esc(s.Title), s.URL))
		}
		fmt.Fprintf(w, "| **%s**<br>%s | %s | %s | %s |\n", esc(x.Name), esc(x.Mechanism), x.Verdict, esc(x.Reason), strings.Join(src, "<br>"))
	}
}

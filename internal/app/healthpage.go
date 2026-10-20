package app

import (
	"time"

	"axion/internal/health"
)

// HealthView is the "Saúde do Windows" page: every check with what was read.
type HealthView struct {
	Scanned   bool           `json:"scanned"` // an analysis exists
	Collected time.Time      `json:"collected"`
	Groups    []string       `json:"groups"`
	Checks    []health.Check `json:"checks"`
	Summary   health.Summary `json:"summary"`
	Honest    string         `json:"honest"`
	Privacy   string         `json:"privacy"`
}

// Health judges the Windows health checks from the latest analysis.
func (a *App) Health() HealthView {
	v := HealthView{Groups: health.Groups, Checks: []health.Check{},
		Honest: "Estas verificações leem o estado do Windows a cada análise: o Defender, o histórico do Windows Update, os volumes, a bateria e o tempo de inicialização. " +
			"As verificações só leem: dizem o que olhar e abrem a tela certa do Windows. A única ação desta página é o reparo de arquivos do Windows, que só roda se você confirmar. O que o Windows não deixou ler aparece como \"não lido\" e não conta como problema nem como saudável. " +
			"Os limites marcados como \"critério do Axion\" são do Axion, não da Microsoft.",
		Privacy: "O Axion guarda só números, datas e os nomes que o Windows já mostra (programas lentos, exclusões do Defender, o modelo da bateria). Nada sai do PC; no relatório exportado, nomes de usuário e de PC são trocados.",
	}
	f := a.Facts()
	if f == nil {
		return v
	}
	v.Scanned, v.Collected = true, f.Collected
	v.Checks = health.Evaluate(f, time.Now())
	v.Summary = health.Summarize(v.Checks)
	return v
}

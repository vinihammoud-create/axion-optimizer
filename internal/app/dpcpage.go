package app

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"axion/internal/dpc"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// Length of a DPC capture, in seconds.
const (
	dpcDefaultSeconds = 30
	dpcMinSeconds     = 10
	dpcMaxSeconds     = 120
)

var (
	srcDPCLimits = tweaks.Source{Title: "Microsoft Learn — Example 15: Measuring DPC/ISR Time (um DPC não deve passar de 100 µs e uma ISR de 25 µs)", URL: "https://learn.microsoft.com/en-us/windows-hardware/drivers/devtest/example-15--measuring-dpc-isr-time"}
	srcDPCClass  = tweaks.Source{Title: "Microsoft Learn — DPC class (o evento do kernel registra a entrada da rotina e o endereço dela)", URL: "https://learn.microsoft.com/en-us/windows/win32/etw/dpc"}
)

// DPCView is the Latência do sistema page.
type DPCView struct {
	Runs     []dpc.Line          `json:"runs"`
	Last     *dpc.Run            `json:"last,omitempty"`
	Hints    map[string]dpc.Hint `json:"hints"` // by module name, for the drivers of Last
	Labels   []string            `json:"labels"`
	Honest   string              `json:"honest"`
	Limits   string              `json:"limits"`
	Privacy  string              `json:"privacy"`
	Sources  []tweaks.Source     `json:"sources"`
	Bounds   []float64           `json:"bounds"` // upper edges (µs) of the duration buckets
	Elevated bool                `json:"elevated"`
	Default  int                 `json:"defaultSeconds"`
	MinSecs  int                 `json:"minSeconds"`
	MaxSecs  int                 `json:"maxSeconds"`
}

// DPCRequest starts a DPC/ISR capture.
type DPCRequest struct {
	Seconds int    `json:"seconds"`
	Label   string `json:"label"`
}

// DPCIDRequest names a stored capture.
type DPCIDRequest struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// DPCCompareRequest compares two captures.
type DPCCompareRequest struct {
	A string `json:"a"`
	B string `json:"b"`
}

// DPCCompare is the before x after table.
type DPCCompare struct {
	A    dpc.Line         `json:"a"`
	B    dpc.Line         `json:"b"`
	Rows []dpc.CompareRow `json:"rows"`
	Note string           `json:"note"`
}

func (a *App) dpcDir() string { return filepath.Join(a.P.DataDir, "dpc") }

// DPC builds the page.
func (a *App) DPC() DPCView {
	v := DPCView{Runs: dpc.List(a.dpcDir()), Hints: map[string]dpc.Hint{}, Elevated: a.P.Native.IsElevated(), Bounds: sys.DPCHistBoundsUS[:],
		Default: dpcDefaultSeconds, MinSecs: dpcMinSeconds, MaxSecs: dpcMaxSeconds,
		Honest: "DPCs e ISRs são as rotinas que os drivers rodam no kernel quando um dispositivo (placa de vídeo, rede, USB, disco) pede a atenção do processador. " +
			"Enquanto uma delas roda, aquele núcleo não executa o jogo: se uma demora, o jogo engasga. O Axion liga, por alguns segundos, o registro de eventos do kernel (o mesmo que o LatencyMon e o xperf usam) " +
			"e mede, em cada DPC e interrupção, quanto tempo durou e qual driver a executou.",
		Limits: "A Microsoft recomenda que um DPC não passe de 100 µs e uma ISR de 25 µs, mas é uma orientação para quem escreve drivers e PCs saudáveis passam disso o tempo todo: a tabela mostra quantas rotinas passaram, e isso sozinho não é tratado como problema. " +
			"O Axion marca \"atenção\" quando um driver tem uma rotina acima de 1 ms ou ocupa 5% ou mais do tempo dos processadores, e \"importante\" acima de 4 ms. Esses números são critério do Axion, não da Microsoft: nenhuma medição pública liga um valor a engasgo em jogo. " +
			"Esta página mostra onde olhar; ela não diz por que o driver demora.",
		Privacy: "A medição só lê. Precisa de administrador, não funciona se outro programa estiver usando o registro de eventos do kernel e nada sai deste PC.",
		Sources: []tweaks.Source{srcDPCLimits, srcDPCClass},
	}
	seen := map[string]bool{"antes": true, "depois": true}
	v.Labels = []string{"antes", "depois"}
	for _, r := range v.Runs {
		if r.Label != "" && !seen[r.Label] {
			seen[r.Label] = true
			v.Labels = append(v.Labels, r.Label)
		}
	}
	if len(v.Runs) > 0 {
		if r, err := dpc.Load(a.dpcDir(), v.Runs[0].ID); err == nil {
			v.Last = r
			v.Hints = dpcHints(r)
		}
	}
	return v
}

// DPCRunView is one stored capture with the advice for its drivers (the run's
// own fields sit at the top level of the JSON).
type DPCRunView struct {
	*dpc.Run
	Hints map[string]dpc.Hint `json:"hints"`
}

func dpcHints(r *dpc.Run) map[string]dpc.Hint {
	h := map[string]dpc.Hint{}
	for _, d := range r.Summary.Drivers {
		h[d.Module] = dpc.HintFor(d.Module)
	}
	return h
}

// DPCRun returns one stored capture with the advice for its drivers.
func (a *App) DPCRun(id string) (DPCRunView, error) {
	r, err := dpc.Load(a.dpcDir(), id)
	if err != nil {
		return DPCRunView{}, fmt.Errorf("%w: medição não encontrada", errNotFound)
	}
	return DPCRunView{Run: r, Hints: dpcHints(r)}, nil
}

// DeleteDPCRun removes a stored capture.
func (a *App) DeleteDPCRun(req DPCIDRequest) error {
	if err := dpc.Delete(a.dpcDir(), req.ID); err != nil {
		return fmt.Errorf("%w: medição não encontrada", errBadRequest)
	}
	a.measurementsChanged()
	return nil
}

// LabelDPCRun renames a stored capture.
func (a *App) LabelDPCRun(req DPCIDRequest) (*dpc.Run, error) {
	r, err := dpc.Relabel(a.dpcDir(), req.ID, req.Label)
	if err != nil {
		return nil, fmt.Errorf("%w: medição não encontrada", errBadRequest)
	}
	a.measurementsChanged()
	return r, nil
}

// CompareDPC compares two stored captures (A = before, B = after).
func (a *App) CompareDPC(req DPCCompareRequest) (DPCCompare, error) {
	ra, err := dpc.Load(a.dpcDir(), req.A)
	if err != nil {
		return DPCCompare{}, fmt.Errorf("%w: medição A não encontrada", errBadRequest)
	}
	rb, err := dpc.Load(a.dpcDir(), req.B)
	if err != nil {
		return DPCCompare{}, fmt.Errorf("%w: medição B não encontrada", errBadRequest)
	}
	note := "Diferenças menores que um mínimo prático contam como iguais: duas medições do mesmo PC variam um pouco. Compare medições feitas na mesma situação (as duas em repouso ou as duas com o jogo aberto) e com a mesma duração."
	switch {
	case !ra.Summary.Reliable || !rb.Summary.Reliable:
		note = "Uma das medições não é confiável (veja o aviso dela): não há o que comparar."
	case ra.Summary.Seconds > 0 && rb.Summary.Seconds > 0 && (ra.Summary.Seconds/rb.Summary.Seconds > 1.5 || rb.Summary.Seconds/ra.Summary.Seconds > 1.5):
		note += " Atenção: as durações são bem diferentes, e a medição mais longa tende a achar uma rotina mais lenta."
	}
	return DPCCompare{A: ra.Line(), B: rb.Line(), Rows: dpc.Compare(ra, rb), Note: note}, nil
}

// StartDPC runs a DPC/ISR capture.
func (a *App) StartDPC(req DPCRequest) (Job, error) {
	body, err := a.dpcJob(req)
	if err != nil {
		return Job{}, err
	}
	return a.Jobs.Start("dpc", "Medindo DPCs e interrupções", true, body)
}

// dpcJob checks the request and returns the body of the capture.
func (a *App) dpcJob(req DPCRequest) (func(ctx context.Context, c *JobCtl) (any, error), error) {
	if !a.P.Native.IsElevated() {
		return nil, fmt.Errorf("%w: a medição de DPC precisa do Axion como administrador (ela liga o registro de eventos do kernel). Abra o programa de novo e aceite o pedido de permissão", errBadRequest)
	}
	secs := req.Seconds
	if secs == 0 {
		secs = dpcDefaultSeconds
	}
	if secs < dpcMinSeconds || secs > dpcMaxSeconds {
		return nil, fmt.Errorf("%w: a medição dura de %d a %d segundos", errBadRequest, dpcMinSeconds, dpcMaxSeconds)
	}
	label := dpc.CleanLabel(req.Label)
	return func(ctx context.Context, c *JobCtl) (any, error) {
		d := time.Duration(secs) * time.Second
		start := time.Now()
		c.Progress(0.02, "Ligando o registro de eventos do kernel")
		// Progress follows the clock while the capture blocks.
		tick := time.NewTicker(250 * time.Millisecond)
		done := make(chan struct{})
		go func() {
			defer tick.Stop()
			for {
				select {
				case <-done:
					return
				case <-ctx.Done():
					return
				case <-tick.C:
					el := time.Since(start)
					if el > d {
						el = d
					}
					c.Progress(0.05+0.9*float64(el)/float64(d), fmt.Sprintf("Medindo DPCs e interrupções (%d de %d s): use o PC como de costume ou abra o jogo", int(el.Seconds()), secs))
				}
			}
		}()
		capt, err := a.P.Native.CaptureDPC(ctx, d)
		close(done)
		if err != nil {
			return nil, err
		}
		if capt.Cancelled || ctx.Err() != nil {
			return nil, context.Canceled
		}
		r := &dpc.Run{ID: tweaks.NewID(), At: start, Label: label, Seconds: float64(secs), Capture: capt}
		c.Progress(0.97, "Calculando o resultado")
		if err := dpc.Save(a.dpcDir(), r); err != nil {
			return nil, fmt.Errorf("salvar a medição: %w", err)
		}
		a.measurementsChanged()
		log.Printf("medição de DPC %s: %d eventos (%d descartados, %d perdidos), %.2f%% do processador, veredito %s", r.ID, r.Summary.Events, r.Summary.Discarded, r.Summary.LostEvents, r.Summary.TotalPct, r.Summary.Verdict)
		return r, nil
	}, nil
}

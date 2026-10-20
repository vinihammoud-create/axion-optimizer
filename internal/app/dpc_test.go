package app

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"axion/internal/dpc"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

func TestDPCMeasurementFlow(t *testing.T) {
	m, c := setupServer(t)
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	c.wait(j.ID)

	// Measure: a job, stored with its label, shown on the page with advice.
	c.json("POST", "/api/dpc/test", DPCRequest{Seconds: 10, Label: "antes"}, &j)
	if done := c.wait(j.ID); done.Status != JobDone {
		t.Fatalf("medição: %+v", done)
	}
	if m.N.DPCCaptures != 1 {
		t.Errorf("capturas: %d", m.N.DPCCaptures)
	}
	var v DPCView
	c.json("GET", "/api/dpc", nil, &v)
	if len(v.Runs) != 1 || v.Last == nil || v.Last.Label != "antes" || !v.Elevated {
		t.Fatalf("página: %+v", v)
	}
	s := v.Last.Summary
	if !s.Reliable || s.Verdict != dpc.VerdictAttention || !strings.Contains(s.Headline, "nvlddmkm.sys") || s.Seconds != 10 {
		t.Errorf("resumo: %+v", s)
	}
	if h := v.Hints["nvlddmkm.sys"]; h.Area != "Placa de vídeo" || h.Page != "drivers" {
		t.Errorf("orientação: %+v", h)
	}
	if len(v.Sources) < 2 || !strings.Contains(v.Limits, "100 µs") || !strings.Contains(v.Limits, "critério do Axion") || !strings.Contains(v.Limits, "saudáveis") {
		t.Errorf("textos honestos: %+v", v.Sources)
	}

	// A second capture after a "fix": the graphics DPCs are short now.
	m.N.DPCFn = func(ctx context.Context, d time.Duration) (sys.DPCCapture, error) {
		capt, err := mocksys.NewNative().CaptureDPC(ctx, d)
		for i, st := range capt.Stats {
			if st.Module == "nvlddmkm.sys" && st.Kind == sys.KindDPC {
				h := make([]uint64, len(st.Hist))
				h[5] = st.Count // every routine within 50 µs
				capt.Stats[i].Hist, capt.Stats[i].MaxUS, capt.Stats[i].TotalUS = h, 48, float64(st.Count)*20
			}
		}
		return capt, err
	}
	c.json("POST", "/api/dpc/test", DPCRequest{Seconds: 10, Label: "depois"}, &j)
	if done := c.wait(j.ID); done.Status != JobDone {
		t.Fatalf("segunda medição: %+v", done)
	}
	c.json("GET", "/api/dpc", nil, &v)
	if len(v.Runs) != 2 || v.Runs[0].Label != "depois" || v.Runs[0].Verdict != dpc.VerdictOK || v.Runs[1].Verdict != dpc.VerdictAttention {
		t.Fatalf("histórico: %+v", v.Runs)
	}
	var cmp DPCCompare
	c.json("POST", "/api/dpc/compare", DPCCompareRequest{A: v.Runs[1].ID, B: v.Runs[0].ID}, &cmp)
	better := map[string]string{}
	for _, r := range cmp.Rows {
		better[r.Label] = r.Verdict
	}
	if better["Maior DPC"] != "melhor" || better["nvlddmkm.sys — maior rotina"] != "melhor" || !strings.Contains(cmp.Note, "mínimo prático") {
		t.Errorf("comparação: %v | %s", better, cmp.Note)
	}

	// Relabel, one run, unknown ids, delete.
	var r dpc.Run
	c.json("POST", "/api/dpc/label", DPCIDRequest{ID: v.Runs[0].ID, Label: "com o driver novo"}, &r)
	if r.Label != "com o driver novo" {
		t.Errorf("rótulo: %q", r.Label)
	}
	c.json("GET", "/api/dpc/run/"+v.Runs[1].ID, nil, &r)
	if r.Summary.Verdict != dpc.VerdictAttention || len(r.Summary.Drivers) == 0 {
		t.Errorf("uma medição: %+v", r.Summary)
	}
	if code, _ := c.do("GET", "/api/dpc/run/ffffffffffff", nil, nil); code != http.StatusNotFound {
		t.Errorf("medição inexistente: %d", code)
	}
	for _, path := range []string{"/api/dpc/delete", "/api/dpc/label"} {
		if code, _ := c.do("POST", path, DPCIDRequest{ID: "zz", Label: "x"}, nil); code != http.StatusBadRequest {
			t.Errorf("%s com id inválido: %d", path, code)
		}
	}
	if code, _ := c.do("POST", "/api/dpc/compare", DPCCompareRequest{A: "nope1234", B: v.Runs[0].ID}, nil); code != http.StatusBadRequest {
		t.Errorf("comparar com id inexistente: %d", code)
	}

	// It survives a restart and goes into the exported report.
	if raw := c.s.App.exportRaw(); raw["dpc"] == nil {
		t.Error("o relatório exportado não traz as medições de DPC")
	}
	reopened, err := New(m.Platform, "test")
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.DPC(); len(got.Runs) != 2 || got.Last == nil {
		t.Errorf("depois de reabrir: %+v", got.Runs)
	}
	c.json("POST", "/api/dpc/delete", DPCIDRequest{ID: v.Runs[1].ID}, nil)
	c.json("GET", "/api/dpc", nil, &v)
	if len(v.Runs) != 1 {
		t.Errorf("depois de apagar: %d", len(v.Runs))
	}
}

func TestDPCRequestsAreValidated(t *testing.T) {
	m, c := setupServer(t)
	for _, secs := range []int{1, 9, 121, 3600, -5} {
		if code, b := c.do("POST", "/api/dpc/test", DPCRequest{Seconds: secs}, nil); code != http.StatusBadRequest || !strings.Contains(string(b), "10 a 120") {
			t.Errorf("%d s: %d %s", secs, code, b)
		}
	}
	// Without administrator rights the capture says so before starting anything.
	m.N.Elevated = false
	code, b := c.do("POST", "/api/dpc/test", DPCRequest{Seconds: 30}, nil)
	if code != http.StatusBadRequest || !strings.Contains(string(b), "administrador") || m.N.DPCCaptures != 0 {
		t.Errorf("sem administrador: %d %s (capturas %d)", code, b, m.N.DPCCaptures)
	}
	var v DPCView
	c.json("GET", "/api/dpc", nil, &v)
	if v.Elevated || len(v.Runs) != 0 {
		t.Errorf("página: %+v", v)
	}
}

func TestDPCCancelStoresNothing(t *testing.T) {
	m, c := setupServer(t)
	started := make(chan struct{})
	m.N.DPCFn = func(ctx context.Context, d time.Duration) (sys.DPCCapture, error) {
		close(started)
		<-ctx.Done()
		return sys.DPCCapture{Duration: d, Cancelled: true}, nil
	}
	var j Job
	c.json("POST", "/api/dpc/test", DPCRequest{Seconds: 30}, &j)
	<-started
	c.json("POST", "/api/jobs/"+j.ID+"/cancel", map[string]any{}, nil)
	if done := c.wait(j.ID); done.Status != JobCancelled {
		t.Fatalf("cancelamento: %+v", done)
	}
	var v DPCView
	c.json("GET", "/api/dpc", nil, &v)
	if len(v.Runs) != 0 {
		t.Errorf("uma medição cancelada não pode ficar no histórico: %+v", v.Runs)
	}
	// A failure of the capture (another program holds the kernel logger) is shown as it is.
	m.N.DPCFn = func(context.Context, time.Duration) (sys.DPCCapture, error) {
		return sys.DPCCapture{}, context.DeadlineExceeded
	}
	c.json("POST", "/api/dpc/test", DPCRequest{Seconds: 10}, &j)
	if done := c.wait(j.ID); done.Status != JobFailed || done.Error == "" {
		t.Errorf("falha: %+v", done)
	}
}

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"axion/internal/diag"
	"axion/internal/obs"
)

func TestOBSPageReadsTheLogs(t *testing.T) {
	_, c := setupServer(t)
	var v ObsView
	c.json("GET", "/api/obs", nil, &v)
	if !v.Found || len(v.Rows) != 1 || v.Latest != 0 || v.UploadMbps != 0 || v.Dir != `%APPDATA%\obs-studio\logs` {
		t.Fatalf("página: %+v", v)
	}
	r := v.Rows[0]
	ctx := r.Context
	if ctx.Encoder != "NVENC" || ctx.BitrateKbps != 8000 || ctx.GPU != "NVIDIA GeForce RTX 3060" || ctx.OutputRes != "1920x1080" || ctx.FPS != 60 {
		t.Errorf("contexto: %+v", ctx)
	}
	st := r.Stream()
	if st == nil || st.Output != "adv_stream" || st.DropPct != 3.1 || st.LaggedPct != 1.8 || st.SkippedPct != 0.2 || st.Frames != 433800 {
		t.Fatalf("números: %+v", st)
	}
	// 3,1% lost on the network is above the line; 1,8% late frames is too; 0,2% skipped is not.
	if r.Analysis.Level != obs.LevelImportant {
		t.Errorf("nível: %s", r.Analysis.Level)
	}
	ids := map[string]bool{}
	for _, f := range r.Analysis.Findings {
		ids[f.ID] = true
	}
	if !ids["network"] || !ids["rendering"] || ids["encoding"] || ids["ok"] {
		t.Errorf("achados: %v", ids)
	}
	// The upload was never measured: it says to measure it.
	for _, f := range r.Analysis.Findings {
		if f.ID == "network" && !strings.Contains(f.Text, "Meça o upload") {
			t.Errorf("sem upload medido: %q", f.Text)
		}
	}
	if len(v.Sources) < 4 || !strings.Contains(v.Privacy, "chave da transmissão") || !strings.Contains(v.Criteria, "critérios do Axion") {
		t.Errorf("textos: %+v", v.Sources)
	}
}

func TestOBSNetworkAdviceUsesTheMeasuredUpload(t *testing.T) {
	_, c := setupServer(t)
	c.s.App.netTimingOverride = &netTiming{Idle: 300 * time.Millisecond, Load: 500 * time.Millisecond, Interval: 10 * time.Millisecond, LoadPing: 20 * time.Millisecond, Streams: [2]int{2, 2}}
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	c.wait(j.ID)
	c.json("POST", "/api/net/test", map[string]any{"label": "antes", "load": true, "dns": false}, &j)
	if done := c.wait(j.ID); done.Status != JobDone {
		t.Fatalf("teste de rede: %+v", done)
	}
	var v ObsView
	c.json("GET", "/api/obs", nil, &v)
	if v.UploadMbps <= 0 {
		t.Fatalf("o upload medido não chegou à página: %+v", v.UploadMbps)
	}
	var text string
	for _, f := range v.Rows[0].Analysis.Findings {
		if f.ID == "network" {
			text = f.Text
		}
	}
	if !strings.Contains(text, "75%") || strings.Contains(text, "Meça o upload") || !strings.Contains(text, "8,0 Mb/s") {
		t.Errorf("a conta com o upload medido (%v Mb/s): %q", v.UploadMbps, text)
	}
}

func TestOBSFindingsReachTheDiagnosticOnlyForLive(t *testing.T) {
	_, c := setupServer(t)
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	c.wait(j.ID)
	find := func() map[string]diag.Finding {
		var rep diag.Report
		c.json("GET", "/api/report", nil, &rep)
		out := map[string]diag.Finding{}
		for _, f := range rep.Findings {
			if strings.HasPrefix(f.ID, "obs.") {
				out[f.ID] = f
			}
		}
		return out
	}
	got := find()
	if len(got) != 2 || got["obs.network"].Severity != diag.SevWarning || got["obs.rendering"].Area != diag.AreaDesempenho || got["obs.network"].Area != diag.AreaRede {
		t.Fatalf("achados do OBS: %+v", got)
	}
	n := got["obs.network"]
	if !strings.Contains(n.Evidence, "NVENC") || !strings.Contains(n.Evidence, "8,0 Mb/s") || !strings.Contains(n.Evidence, "1920x1080@60") || len(n.Sources) == 0 {
		t.Errorf("evidência: %q %+v", n.Evidence, n.Sources)
	}
	pages := map[string]bool{}
	for _, a := range n.Actions {
		pages[a.Page] = true
	}
	if !pages["rede"] || !pages["obs"] {
		t.Errorf("ações: %+v", n.Actions)
	}
	// Competitive (the default) sets them aside; the live profile counts them.
	if !n.OffProfile {
		t.Error("no perfil competitivo, o achado do OBS não conta na nota")
	}
	c.json("POST", "/api/settings", map[string]any{"profile": "live"}, nil)
	if got := find(); got["obs.network"].OffProfile || got["obs.rendering"].OffProfile {
		t.Errorf("no perfil live deveria contar: %+v", got)
	}
}

func TestOBSWithoutLogsIsQuiet(t *testing.T) {
	m, c := setupServer(t)
	if err := os.RemoveAll(filepath.Join(m.N.Paths().AppData, "obs-studio")); err != nil {
		t.Fatal(err)
	}
	var v ObsView
	c.json("GET", "/api/obs", nil, &v)
	if v.Found || len(v.Rows) != 0 || v.Latest != -1 {
		t.Errorf("sem OBS: %+v", v)
	}
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	c.wait(j.ID)
	var rep diag.Report
	c.json("GET", "/api/report", nil, &rep)
	for _, f := range rep.Findings {
		if strings.HasPrefix(f.ID, "obs.") {
			t.Errorf("achado do OBS sem log: %+v", f)
		}
	}
	// An exported report carries the OBS sessions (numbers only).
	if raw := c.s.App.exportRaw(); raw["obs"] == nil {
		t.Error("o relatório exportado não traz o OBS")
	}
}

package app

import (
	"encoding/json"
	"strings"
	"testing"

	"axion/internal/diag"
)

// Both exports carry the certainty of every finding: the JSON field and the
// label in the HTML report.
func TestReportCertainty(t *testing.T) {
	_, c := setupServer(t)
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	_, raw := c.do("GET", "/api/export", nil, nil)
	var exp struct {
		Report struct {
			Findings []struct {
				ID        string `json:"id"`
				Severity  string `json:"severity"`
				Certainty string `json:"certainty"`
			} `json:"findings"`
		} `json:"report"`
	}
	if err := json.Unmarshal(raw, &exp); err != nil || len(exp.Report.Findings) == 0 {
		t.Fatalf("exportação JSON: %v (%d achados)", err, len(exp.Report.Findings))
	}
	labels := map[string]bool{}
	for _, x := range exp.Report.Findings {
		l, ok := diag.CertaintyLabels[x.Certainty]
		if !ok {
			t.Errorf("JSON: %s sem certeza (%q)", x.ID, x.Certainty)
		}
		labels[l] = true
	}
	html, err := c.s.App.ExportHTML(false)
	if err != nil {
		t.Fatal(err)
	}
	for l := range labels {
		if !strings.Contains(string(html), `<span class="tag cert">`+l+`</span>`) {
			t.Errorf("HTML sem o rótulo %q", l)
		}
	}
	if !labels["Problema confirmado"] || !labels["Possível problema"] {
		t.Errorf("o PC simulado deveria ter achados confirmados e possíveis: %v", labels)
	}
}

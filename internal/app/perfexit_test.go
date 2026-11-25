package app

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"axion/internal/sys"
)

func TestPresentMonExitCodeMessage(t *testing.T) {
	m, c := setupServer(t)
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	c.wait(j.ID)
	prev := m.X.Handler
	code := 0
	m.X.Handler = func(exe string, args []string, ps string) (sys.Result, bool) {
		if strings.HasPrefix(strings.ToLower(filepath.Base(exe)), "presentmon") && !slices.Contains(args, "--terminate_existing_session") {
			return sys.Result{ExitCode: code}, true
		}
		return prev(exe, args, ps)
	}
	capture := func() Job {
		var j Job
		c.json("POST", "/api/perf/capture", PerfCaptureRequest{Process: "cs2.exe", Seconds: 10}, &j)
		return c.wait(j.ID)
	}

	code = 0xC0000135
	if j := capture(); j.Status != JobFailed || !strings.Contains(j.Error, "0xC0000135") || !strings.Contains(j.Error, "não iniciou") {
		t.Errorf("DLL não encontrada: %+v", j)
	}
	code = 1
	if j := capture(); j.Status != JobFailed || !strings.Contains(j.Error, "código 1)") {
		t.Errorf("código 1: %+v", j)
	}
}

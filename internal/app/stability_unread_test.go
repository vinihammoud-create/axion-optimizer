package app

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"axion/internal/diag"
	"axion/internal/sys"
)

// failingEvents answers like the mock except for the queries listed in fail.
type failingEvents struct {
	inner sys.EventLog
	fail  map[string]bool
	mu    sync.Mutex
	calls map[string]int
}

func (e *failingEvents) Query(channel, xpath string, max int) ([]sys.Event, error) {
	e.mu.Lock()
	e.calls[xpath]++
	e.mu.Unlock()
	if e.fail[xpath] {
		return nil, errors.New("falha simulada")
	}
	return e.inner.Query(channel, xpath, max)
}

func scanWithFailing(t *testing.T, fail ...string) (*App, *failingEvents) {
	t.Helper()
	m, c := setupServer(t)
	fe := &failingEvents{inner: m.E, fail: map[string]bool{}, calls: map[string]int{}}
	for _, x := range fail {
		fe.fail[x] = true
	}
	c.s.App.P.Events = fe
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	return c.s.App, fe
}

func unreadParts(a *App) map[string]bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := map[string]bool{}
	for _, u := range a.facts.Unread {
		out[u] = true
	}
	return out
}

func stabilityFinding(a *App, id string) *diag.Finding {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for i := range a.stability {
		if a.stability[i].ID == id {
			return &a.stability[i]
		}
	}
	return nil
}

func TestSymptomQueryFailureIsUnread(t *testing.T) {
	a, _ := scanWithFailing(t, symptomXPath)
	if !unreadParts(a)[diag.PartSymptoms] {
		t.Errorf("a consulta de sintomas falhou e deveria constar em Unread: %v", a.facts.Unread)
	}
	x := stabilityFinding(a, "stab.whea")
	if x == nil {
		t.Fatal("sem achado WHEA")
	}
	if strings.Contains(x.Evidence, "Nenhum desligamento") {
		t.Errorf("o achado afirma que não há sintoma sem ter lido: %s", x.Evidence)
	}
}

func TestWHEAQueryFailureKeepsOtherQueries(t *testing.T) {
	a, fe := scanWithFailing(t, stabilityXPath)
	un := unreadParts(a)
	if !un[diag.PartWHEA] {
		t.Errorf("a consulta WHEA falhou e deveria constar em Unread: %v", a.facts.Unread)
	}
	for _, p := range []string{diag.PartSymptoms, diag.PartBoots, diag.PartThermal} {
		if un[p] {
			t.Errorf("%s respondeu e não pode constar em Unread", p)
		}
	}
	for name, x := range map[string]string{"sintomas": symptomXPath, "inicializações": bootXPath, "térmicos": thermalXPath} {
		if fe.calls[x] == 0 {
			t.Errorf("a consulta de %s não rodou depois da falha da WHEA", name)
		}
	}
	if stabilityFinding(a, "stab.kp41") == nil {
		t.Error("o desligamento inesperado deveria ser lido mesmo com a consulta WHEA falha")
	}
}

func TestStabilityQueriesAllAnswering(t *testing.T) {
	a, _ := scanWithFailing(t)
	for _, p := range []string{diag.PartWHEA, diag.PartSymptoms, diag.PartBoots, diag.PartThermal} {
		if unreadParts(a)[p] {
			t.Errorf("%s respondeu e não pode constar em Unread", p)
		}
	}
	x := stabilityFinding(a, "stab.whea")
	if x == nil || !strings.Contains(x.Evidence, "No mesmo período há desligamento inesperado") {
		t.Errorf("com tudo lido o achado deveria citar os sintomas: %+v", x)
	}
}

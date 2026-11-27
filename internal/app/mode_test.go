package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"axion/internal/sys/mocksys"
)

// appWithSettings starts the app over the simulated PC with settings.json
// holding body (nil: no file, a new installation).
func appWithSettings(t *testing.T, body []byte) *App {
	t.Helper()
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		if err := os.WriteFile(filepath.Join(m.Platform.DataDir, "settings.json"), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a, err := New(m.Platform, "test")
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// A new installation (no settings.json) opens in the simple level; a
// settings file saved by an earlier version keeps what it had: simple=true
// is the simple level, a file without it (the complete mode, the default
// before 0.10.1) the advanced one.
func TestModeDefaultAndMigration(t *testing.T) {
	cases := []struct {
		name string
		body []byte
		want string
	}{
		{"instalação nova", nil, ModeSimple},
		{"usuário antigo no modo completo", []byte(`{"restorePoint":true,"accepted":true,"profile":"competitivo"}`), ModeAdvanced},
		{"usuário antigo no modo simples", []byte(`{"restorePoint":true,"accepted":true,"simple":true}`), ModeSimple},
		{"arquivo antigo só com o aviso aceito", []byte(`{"accepted":true}`), ModeAdvanced},
		{"modo gravado vale sobre o booleano", []byte(`{"accepted":true,"mode":"avancado","simple":true}`), ModeAdvanced},
		{"modo desconhecido lê o booleano", []byte(`{"accepted":true,"mode":"x","simple":true}`), ModeSimple},
		{"arquivo ilegível fica como antes", []byte(`{não é json`), ModeAdvanced},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := appWithSettings(t, tc.body).Settings()
			if s.Mode != tc.want || s.Simple != (tc.want == ModeSimple) {
				t.Errorf("modo %q simple %v, esperado %q", s.Mode, s.Simple, tc.want)
			}
		})
	}
	// The new installation keeps the other defaults.
	if s := appWithSettings(t, nil).Settings(); !s.RestorePoint || s.Accepted {
		t.Errorf("padrões da instalação nova: %+v", s)
	}
}

// POST /api/settings takes the level (and the older boolean), refuses an
// unknown one, and the file keeps both fields so a version before 0.10.1
// opened afterwards reads the same level.
func TestModeSettingRoute(t *testing.T) {
	_, c := setupServer(t)
	var st Settings
	c.json("POST", "/api/settings", map[string]any{"mode": ModeAdvanced}, &st)
	if st.Mode != ModeAdvanced || st.Simple {
		t.Fatalf("avançado: %+v", st)
	}
	c.json("POST", "/api/settings", map[string]any{"mode": ModeSimple}, &st)
	if st.Mode != ModeSimple || !st.Simple {
		t.Fatalf("simples: %+v", st)
	}
	if code, _ := c.do("POST", "/api/settings", map[string]any{"mode": "completo", "accepted": true}, nil); code != 400 {
		t.Errorf("modo desconhecido: %d", code)
	}
	c.json("GET", "/api/state", nil, &struct {
		Settings *Settings `json:"settings"`
	}{&st})
	if st.Mode != ModeSimple || st.Accepted {
		t.Errorf("o pedido recusado não mudou nada: %+v", st)
	}
	st = Settings{}
	c.json("POST", "/api/settings", map[string]any{"simple": false}, &st)
	if st.Mode != ModeAdvanced || st.Simple {
		t.Errorf("booleano antigo false: %+v", st)
	}
	c.json("POST", "/api/settings", map[string]any{"simple": true}, &st)
	if st.Mode != ModeSimple || !st.Simple {
		t.Errorf("booleano antigo true: %+v", st)
	}
	b, err := os.ReadFile(c.s.App.settingsPath())
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["mode"] != ModeSimple || raw["simple"] != true {
		t.Errorf("arquivo gravado: %s", b)
	}
	// Another start reads the same level from the file.
	a2, err := New(c.s.App.P, "test")
	if err != nil {
		t.Fatal(err)
	}
	if s := a2.Settings(); s.Mode != ModeSimple {
		t.Errorf("modo depois de reabrir: %+v", s)
	}
}

// The simple level offers only the recommended plan: every item there is
// risk-free (Tweak.Recommendable), so the 1-click button of the simple
// screen never brings a moderate or advanced item.
func TestSimpleModePlanOnlySafe(t *testing.T) {
	_, c := setupServer(t)
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise falhou: %+v", j)
	}
	var plan Plan
	c.json("GET", "/api/plan?tier=recomendado", nil, &plan)
	if len(plan.Items) == 0 {
		t.Fatal("plano recomendado vazio no PC simulado")
	}
	for _, it := range plan.Items {
		if it.Risk != "seguro" || it.Optional {
			t.Errorf("item fora do critério do modo Simples: %s (risco %s, opcional %v)", it.ID, it.Risk, it.Optional)
		}
	}
}

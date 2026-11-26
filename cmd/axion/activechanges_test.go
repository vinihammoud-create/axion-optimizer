package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// A history with one entry of each kind the uninstaller must tell apart.
const historyMixed = `{"version": 1, "entries": [
  {"id": "a1", "tweakId": "t1", "status": "applied", "ops": [{"kind": "reg", "before": "1", "skipped": false}]},
  {"id": "a2", "tweakId": "t2", "status": "applied", "ops": [{"kind": "reg", "skipped": true}]},
  {"id": "a3", "tweakId": "t3", "status": "reverted", "ops": [{"kind": "reg", "before": "1", "revertVerified": true}]},
  {"id": "a4", "tweakId": "t4", "status": "interrupted", "ops": [{"kind": "reg", "before": "1"}]},
  {"id": "a5", "tweakId": "t5", "status": "applying", "ops": [{"kind": "reg", "before": "1"}]},
  {"id": "a6", "tweakId": "t6", "status": "failed", "ops": [{"kind": "reg", "before": "1", "revertVerified": true}]},
  {"id": "a7", "tweakId": "t7", "status": "partial-revert", "ops": [{"kind": "reg", "before": "1"}, {"kind": "reg", "before": "2", "revertVerified": true}]},
  {"id": "a8", "tweakId": "t8", "status": "superseded", "ops": [{"kind": "reg", "before": "1"}]},
  null
]}`

func writeFile(t *testing.T, dir, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCountActiveChanges(t *testing.T) {
	cases := []struct {
		name    string
		files   map[string]string
		want    int
		wantErr bool
	}{
		{name: "sem histórico", files: nil, want: 0},
		{name: "histórico vazio", files: map[string]string{"journal.json": `{"version": 1, "entries": []}`}, want: 0},
		// applied with a change, interrupted, applying (interrupted at the
		// next start) and the partial revert still pending: 4.
		{name: "entradas de cada tipo", files: map[string]string{"journal.json": historyMixed}, want: 4},
		{name: "danificado, com cópia .tmp", files: map[string]string{"journal.json": "{", "journal.json.tmp": historyMixed}, want: 4},
		{name: "ausente, com cópia .bak", files: map[string]string{"journal.json.bak": historyMixed}, want: 4},
		{name: "danificado sem cópia", files: map[string]string{"journal.json": "{\"entries\": ["}, wantErr: true},
		{name: "sem versão", files: map[string]string{"journal.json": `{"entries": []}`}, wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			for n, s := range c.files {
				writeFile(t, dir, n, s)
			}
			before := listDir(t, dir)
			n, err := countActiveChanges(dir)
			if (err != nil) != c.wantErr {
				t.Fatalf("erro = %v, esperado erro: %v", err, c.wantErr)
			}
			if !c.wantErr && n != c.want {
				t.Errorf("ativas = %d, esperado %d", n, c.want)
			}
			if after := listDir(t, dir); after != before {
				t.Errorf("a leitura mudou a pasta: antes %q, depois %q", before, after)
			}
		})
	}
}

// The history folder itself may be missing (Axion never ran).
func TestCountActiveChangesNoFolder(t *testing.T) {
	n, err := countActiveChanges(filepath.Join(t.TempDir(), "nao-existe"))
	if err != nil || n != 0 {
		t.Fatalf("countActiveChanges = %d, %v; esperado 0, nil", n, err)
	}
}

// A journal.json that exists but cannot be read is "unknown", never "nothing
// applied" (here it is a folder, which ReadFile refuses on every system).
func TestCountActiveChangesUnreadable(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "journal.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := countActiveChanges(dir); err == nil {
		t.Fatal("journal.json ilegível deveria dar erro")
	}
}

func TestActiveExitCode(t *testing.T) {
	cases := []struct {
		n    int
		err  error
		want int
	}{
		{0, nil, 0},
		{1, nil, 11},
		{7, nil, 17},
		{200, nil, 210},
		{5000, nil, 210},
		{0, errors.New("x"), 1},
		{3, errors.New("x"), 1},
	}
	for _, c := range cases {
		if got := activeExitCode(c.n, c.err); got != c.want {
			t.Errorf("activeExitCode(%d, %v) = %d, esperado %d", c.n, c.err, got, c.want)
		}
	}
}

func listDir(t *testing.T, dir string) string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	s := ""
	for _, e := range es {
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		s += e.Name() + ":" + info.ModTime().String() + ";"
	}
	return s
}

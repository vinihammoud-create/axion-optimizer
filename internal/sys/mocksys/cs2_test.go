package mocksys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSeedCS2VideoShowsInWorld(t *testing.T) {
	m, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := m.World()
	for k := range base {
		if strings.HasPrefix(k, "file|") || strings.HasPrefix(k, "dir|") {
			t.Fatalf("sem WatchDir o World não lista arquivos: %s", k)
		}
	}
	p, err := m.SeedCS2Video(CS2VideoSample)
	if err != nil {
		t.Fatal(err)
	}
	if p != CS2VideoPath(m.DataDir) {
		t.Fatalf("caminho: %s", p)
	}
	if strings.Contains(strings.ToLower(p), "mockfs") {
		t.Error("o arquivo não pode ficar dentro do mockfs (o scan de sobras veria uma Steam falsa)")
	}
	w1 := m.World()
	key := "file|" + strings.ToLower(filepath.ToSlash(p))
	if w1[key] == "" {
		t.Fatalf("o World não lista o arquivo semeado: %v", w1)
	}
	// A changed byte and a stray temporary file both show in DiffWorld.
	if err := os.WriteFile(p, []byte(strings.Replace(CS2VideoSample, `"2"`, `"4"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p+".axion-1.tmp", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if d := DiffWorld(w1, m.World()); len(d) != 2 {
		t.Fatalf("esperava 2 diferenças, obtido %v", d)
	}
	// Watching the same folder twice does not duplicate anything.
	m.WatchDir(filepath.Dir(p))
	if n := len(m.watch); n != 1 {
		t.Errorf("pasta observada duas vezes: %d", n)
	}
	// The file keeps the CRLF layout of the real one.
	if !strings.HasPrefix(CS2VideoSample, "\"video.cfg\"\r\n{\r\n\t\"Version\"\t\t\"16\"\r\n") || !strings.HasSuffix(CS2VideoSample, "}\r\n") {
		t.Error("formato do exemplo")
	}
}

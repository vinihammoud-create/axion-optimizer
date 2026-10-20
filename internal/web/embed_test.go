package web

import (
	"io/fs"
	"strings"
	"testing"
)

// The logo files and every asset index.html points at must be inside the embed.
func TestEmbedIncludesLogoAndIndexAssets(t *testing.T) {
	root := FS()
	for _, name := range []string{"index.html", "app.js", "app.css", "favicon.svg", "logo-48.png", "logo-256.png"} {
		b, err := fs.ReadFile(root, name)
		if err != nil || len(b) == 0 {
			t.Fatalf("%s ausente ou vazio no embed: %v", name, err)
		}
		if strings.HasSuffix(name, ".png") && (len(b) < 4 || string(b[1:4]) != "PNG") {
			t.Errorf("%s nao e PNG valido", name)
		}
	}
	idx, _ := fs.ReadFile(root, "index.html")
	for _, ref := range []string{`href="logo-48.png"`, `href="favicon.svg"`} {
		if !strings.Contains(string(idx), ref) {
			t.Errorf("index.html nao referencia %s", ref)
		}
	}
	js, _ := fs.ReadFile(root, "app.js")
	if !strings.Contains(string(js), "logo-48.png") {
		t.Error("app.js nao usa logo-48.png")
	}
}

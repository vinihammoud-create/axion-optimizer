package games

import (
	"strings"
	"testing"
)

func TestLaunchValue(t *testing.T) {
	cases := []struct {
		opts string
		want int
		opt  string
		ok   bool
	}{
		{"-high -fullscreen -refresh 165 -nojoy +exec autoexec.cfg", 165, "-refresh", true},
		{"-FREQ 144", 144, "-freq", true},
		{"-refresh", 0, "", false},
		{"-refresh abc -freq 240", 240, "-freq", true},
		{"+refresh 165", 0, "", false},
		{"", 0, "", false},
	}
	for _, c := range cases {
		v, o, ok := LaunchValue(c.opts, "-refresh", "-freq")
		if v != c.want || o != c.opt || ok != c.ok {
			t.Errorf("%q: %d %q %v", c.opts, v, o, ok)
		}
	}
}

// 0.6.3: the -allow_third_party_software note says why people use it (OBS
// Game Capture) and what to use instead.
func TestThirdPartyNote(t *testing.T) {
	notes := AnalyzeLaunchOptions("-allow_third_party_software -nojoy")
	var found bool
	for _, n := range notes {
		if n.Option == "-allow_third_party_software" {
			found = true
			if n.Level != "warning" || !strings.Contains(n.Text, "Captura de Jogo") || !strings.Contains(n.Text, "Captura de Janela") {
				t.Errorf("nota: %+v", n)
			}
		}
	}
	if !found {
		t.Fatalf("sem nota: %+v", notes)
	}
}

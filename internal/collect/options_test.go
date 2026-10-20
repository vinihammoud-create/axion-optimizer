package collect

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"axion/internal/facts"
	"axion/internal/sys/mocksys"
)

// Steps and parts can be skipped; parts are announced before and after they
// run, so a collection process that dies inside one can name it.
func TestCollectOptions(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var marks []string
	f := CollectWith(context.Background(), m.Platform, nil, Options{
		Skip: map[string]bool{"Energia": true, PartNVMe: true},
		Mark: func(part string, done bool) {
			if done {
				marks = append(marks, "fim:"+part)
			} else {
				marks = append(marks, part)
			}
		},
	})
	joined := strings.Join(f.Errors, " | ")
	if !strings.Contains(joined, "Energia: etapa pulada") || !strings.Contains(joined, PartNVMe+": pulado") {
		t.Errorf("avisos: %v", f.Errors)
	}
	if f.Power.ActiveScheme != "" {
		t.Errorf("a etapa pulada não roda: %+v", f.Power)
	}
	want := []string{PartHAGS, "fim:" + PartHAGS, PartX3D, "fim:" + PartX3D}
	got := strings.Join(marks, ",")
	if !strings.Contains(got, strings.Join(want, ",")) || strings.Contains(got, PartNVMe) {
		t.Errorf("marcas: %v", marks)
	}
	// Anti-cheats come from the service entries in the registry.
	if len(f.Gaming.AntiCheats) != 1 || f.Gaming.AntiCheats[0] != "FACEIT" {
		t.Errorf("anti-cheats: %v", f.Gaming.AntiCheats)
	}
	// The profile crosses the process boundary as JSON without losing data.
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	var back facts.Facts
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	b2, _ := json.Marshal(&back)
	if string(b) != string(b2) {
		t.Error("o perfil muda ao passar por JSON")
	}
}

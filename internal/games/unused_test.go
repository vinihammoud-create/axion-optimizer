package games

import (
	"testing"
	"time"
)

func TestUnusedGames(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	ago := func(days int) time.Time { return now.Add(-time.Duration(days) * 24 * time.Hour) }
	steam := func(name string, size uint64, played, updated time.Time) Game {
		return Game{ID: "steam:" + name, Name: name, Launcher: "Steam", SizeBytes: size, LastPlayed: played, Updated: updated}
	}
	list := []Game{
		steam("Jogado ontem", 90<<30, ago(1), ago(5)),
		steam("Quase 6 meses", 50<<30, ago(179), ago(200)), // one day short
		steam("Seis meses", 20<<30, ago(180), ago(400)),
		steam("Um ano", 70<<30, ago(365), ago(380)),
		steam("Nunca aberto e velho", 60<<30, time.Time{}, ago(300)),
		steam("Nunca aberto e novo", 80<<30, time.Time{}, ago(10)), // installed recently: not unused
		steam("Sem nenhuma data", 40<<30, time.Time{}, time.Time{}),
		steam("Sem tamanho", 0, ago(500), ago(500)),
		{ID: "epic:fortnite", Name: "Fortnite", Launcher: "Epic", SizeBytes: 100 << 30, LastPlayed: ago(900)}, // Epic does not say
		steam("Relógio adiantado", 1<<30, now.Add(48*time.Hour), time.Time{}),                                 // a date in the future
	}
	got := UnusedGames(list, now, UnusedDays)
	want := []struct {
		name  string
		days  int
		never bool
	}{
		{"Um ano", 365, false},
		{"Nunca aberto e velho", 300, true},
		{"Seis meses", 180, false},
		{"Sem tamanho", 500, false},
	}
	if len(got) != len(want) {
		t.Fatalf("%d jogos: %+v", len(got), got)
	}
	for i, w := range want { // largest first; the one without a size last
		if got[i].Name != w.name || got[i].Days != w.days || got[i].Never != w.never {
			t.Errorf("%d: %+v, esperava %+v", i, got[i], w)
		}
	}
	// A shorter line lists more.
	if n := len(UnusedGames(list, now, 30)); n != 5 { // + "Quase 6 meses"; the recent ones still stay out
		t.Errorf("com 30 dias: %d jogos", n)
	}
	if got := UnusedGames(nil, now, UnusedDays); len(got) != 0 {
		t.Errorf("sem jogos: %+v", got)
	}
}

// Equal sizes: the one untouched for longer comes first.
func TestUnusedGamesTieBreak(t *testing.T) {
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	a := Game{ID: "steam:a", Name: "A", Launcher: "Steam", SizeBytes: 10 << 30, LastPlayed: now.Add(-200 * 24 * time.Hour)}
	b := Game{ID: "steam:b", Name: "B", Launcher: "Steam", SizeBytes: 10 << 30, LastPlayed: now.Add(-400 * 24 * time.Hour)}
	got := UnusedGames([]Game{a, b}, now, UnusedDays)
	if len(got) != 2 || got[0].Name != "B" {
		t.Errorf("%+v", got)
	}
}

package app

import (
	"testing"

	"axion/internal/sys"
)

// M-004: one snapshot of the process list per polling cycle, whatever the
// number of games turned on.
func TestPollGamesTakesOneSnapshot(t *testing.T) {
	m, c := setupServer(t)
	m.N.Procs = append(m.N.Procs, sys.ProcessInfo{PID: 4242, Name: "EldenRing.exe"})
	order := []string{"cs2.exe", "eldenring.exe", "VALORANT-Win64-Shipping.exe", "FortniteClient-Win64-Shipping.exe", "Cyberpunk2077.exe"}
	before := m.N.Snapshots()
	got := c.s.App.pollGames(order)
	if n := m.N.Snapshots() - before; n != 1 {
		t.Fatalf("%d jogos ligados tiraram %d retratos dos processos; esperado 1", len(order), n)
	}
	if !got["eldenring.exe"] || got["Cyberpunk2077.exe"] {
		t.Fatalf("jogos abertos: %v", got)
	}
}

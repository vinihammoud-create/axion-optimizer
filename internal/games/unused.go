package games

import (
	"sort"
	"time"
)

// UnusedDays is how long a game must have sat untouched to be listed: Axion's
// own line, half a year.
const UnusedDays = 180

// Unused is an installed game that has not been opened for a long time.
type Unused struct {
	Game
	// Days since the last time it was played (or, for a game with no play
	// record, since the launcher last updated it).
	Days int `json:"days"`
	// Never: the Steam manifest has no LastPlayed (no record of play). That is
	// not proof the game was never opened: old manifests, or a game installed by
	// another account, may lack the key.
	Never bool `json:"never"`
}

// UnusedGames lists the Steam games nobody has opened for minDays, largest
// first. Only Steam says when a game was last played, so other launchers are
// never listed. A game with no play record (Never) is listed only when its
// last update is old too: a game installed yesterday is not unused.
// Nothing is judged without a date.
func UnusedGames(list []Game, now time.Time, minDays int) []Unused {
	var out []Unused
	for _, g := range list {
		if g.Launcher != "Steam" {
			continue
		}
		switch {
		case !g.LastPlayed.IsZero():
			if d := daysBetween(g.LastPlayed, now); d >= minDays {
				out = append(out, Unused{Game: g, Days: d})
			}
		case !g.Updated.IsZero():
			if d := daysBetween(g.Updated, now); d >= minDays {
				out = append(out, Unused{Game: g, Days: d, Never: true})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SizeBytes != out[j].SizeBytes {
			return out[i].SizeBytes > out[j].SizeBytes
		}
		return out[i].Days > out[j].Days
	})
	return out
}

func daysBetween(from, to time.Time) int {
	d := int(to.Sub(from).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}

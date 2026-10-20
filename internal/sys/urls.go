package sys

import "strings"

// IsSteamUninstallURL reports a "steam://uninstall/<appid>" address, the only
// steam: link the Axion opens. Steam shows its own confirmation before it
// removes a game, so the link by itself deletes nothing.
func IsSteamUninstallURL(u string) bool {
	id, ok := strings.CutPrefix(u, "steam://uninstall/")
	if !ok || len(id) == 0 || len(id) > 10 {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

package games

import (
	"strconv"
	"strings"
)

// SteamClient is what the Steam client settings say about downloads
// (0.7): a download in the middle of a match fills the connection and the
// ping rises (the bufferbloat the network test measures). Read from
// config\config.vdf; -1 = not found in the file (the client's default).
type SteamClient struct {
	Found                   bool   `json:"found"`
	Path                    string `json:"path,omitempty"`
	DownloadsDuringGameplay int    `json:"downloadsDuringGameplay"` // 1 allowed, 0 paused, -1 not set (paused is the default)
	ThrottleKbps            int    `json:"throttleKbps"`            // 0 none, -1 not set (none)
	AutoUpdateWindow        int    `json:"autoUpdateWindow"`        // 1 restricted to a window, 0 any time, -1 not set
	WindowStart             int    `json:"windowStart"`             // hours 0-23 (-1 unknown)
	WindowEnd               int    `json:"windowEnd"`
}

// ParseSteamClient reads the download settings of config.vdf.
func ParseSteamClient(src string) (*SteamClient, bool) {
	v, err := ParseVDF(src)
	if err != nil {
		return nil, false
	}
	sc := &SteamClient{Found: true, DownloadsDuringGameplay: -1, ThrottleKbps: -1, AutoUpdateWindow: -1, WindowStart: -1, WindowEnd: -1}
	// The documented place first, then anywhere in the tree.
	store := v.Path("InstallConfigStore", "Software", "Valve", "Steam")
	get := func(key string) (int, bool) {
		if s := store.Str(key); s != "" {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
				return n, true
			}
		}
		if s, ok := findKey(v, strings.ToLower(key), 0); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
				return n, true
			}
		}
		return 0, false
	}
	if n, ok := get("AllowDownloadsDuringGameplay"); ok {
		sc.DownloadsDuringGameplay = n
	}
	if n, ok := get("DownloadThrottleKbps"); ok {
		sc.ThrottleKbps = n
	}
	if n, ok := get("AutoUpdateWindowEnabled"); ok {
		sc.AutoUpdateWindow = n
	}
	if n, ok := get("AutoUpdateWindowStart"); ok {
		sc.WindowStart = n
	}
	if n, ok := get("AutoUpdateWindowEnd"); ok {
		sc.WindowEnd = n
	}
	return sc, true
}

// findKey looks for a string value under key anywhere in the document.
func findKey(v VDF, key string, depth int) (string, bool) {
	if v == nil || depth > 12 {
		return "", false
	}
	if s, ok := v[key].(string); ok {
		return s, true
	}
	for _, val := range v {
		if sub, ok := val.(VDF); ok {
			if s, ok := findKey(sub, key, depth+1); ok {
				return s, true
			}
		}
	}
	return "", false
}

// steamClient reads the client settings of a Steam installation.
func (s *Scanner) steamClient(root string) *SteamClient {
	p := winJoin(root, "config", "config.vdf")
	b, err := s.FS.ReadFile(p)
	if err != nil {
		return nil
	}
	sc, ok := ParseSteamClient(string(b))
	if !ok {
		return nil
	}
	sc.Path = p
	return sc
}

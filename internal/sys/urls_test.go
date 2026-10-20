package sys

import "testing"

func TestIsSteamUninstallURL(t *testing.T) {
	for u, want := range map[string]bool{
		"steam://uninstall/730":            true,
		"steam://uninstall/1234567890":     true,
		"steam://uninstall/":               false,
		"steam://uninstall/12345678901":    false, // longer than a Steam app id
		"steam://uninstall/730/extra":      false,
		"steam://uninstall/730?x=1":        false,
		"steam://uninstall/-1":             false,
		"steam://uninstall/7 30":           false,
		"steam://run/730":                  false,
		"steam://rungameid/730":            false,
		"steam://install/730":              false,
		"STEAM://uninstall/730":            false,
		"https://steam://uninstall/730":    false,
		"steam://uninstall/730\"&calc.exe": false,
		"":                                 false,
	} {
		if got := IsSteamUninstallURL(u); got != want {
			t.Errorf("%q: %v, esperava %v", u, got, want)
		}
	}
}

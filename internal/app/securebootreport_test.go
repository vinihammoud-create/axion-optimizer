package app

import "testing"

// A real PC's report showed "Secure Boot unsupported": the collected value
// went out untranslated, and an unread value must not look like a state.
func TestSecureBootTextInReport(t *testing.T) {
	for in, want := range map[string]string{"on": "ligado", "off": "desligado", "unsupported": "não suportado", "unknown": "não lido", "": "não lido"} {
		if got := secureBootText(in); got != want {
			t.Errorf("%q: %q, quero %q", in, got, want)
		}
	}
}

package sys

import "testing"

func TestTrustedEnv(t *testing.T) {
	env := TrustedEnv(Paths{WindowsDir: `C:\Windows`, ProgramFiles: `C:\Program Files`, ProgramData: `C:\ProgramData`})
	if got := env("windir"); got != `C:\Windows` {
		t.Errorf("windir = %q", got)
	}
	if got := env("ProgramFiles"); got != `C:\Program Files` {
		t.Errorf("ProgramFiles = %q", got)
	}
	// A variable a user could plant is never expanded.
	if got := env("EVIL_DIR"); got != "" {
		t.Errorf("unknown variable expanded to %q", got)
	}
	if got := env("ProgramFiles(x86)"); got != "" {
		t.Errorf("missing folder should expand to empty, got %q", got)
	}
}

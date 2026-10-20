package profile

import "testing"

// The NVIDIA driver profile findings count only in a game; the residue of
// optimizers in the global profile counts for everyone, like the other
// res.* findings.
func TestNVDRSScopes(t *testing.T) {
	for id, want := range map[string]string{
		"gpu.nvdrs.vsync":       ScopeGaming,
		"gpu.nvdrs.frl.cs2.exe": ScopeGaming,
		"gpu.nvdrs.shadercache": ScopeGaming,
		"gpu.nvdrs.vrr":         ScopeGaming,
		"res.nvdrs":             ScopeGeneral,
	} {
		if got := FindingScope(id); got != want {
			t.Errorf("%s: escopo %q, esperado %q", id, got, want)
		}
	}
}

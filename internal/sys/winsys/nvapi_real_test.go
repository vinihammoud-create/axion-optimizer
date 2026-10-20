//go:build windows

package winsys

import (
	"errors"
	"testing"

	"axion/internal/sys"
)

// Read-only check against the real driver, on a PC with nvapi64.dll: the
// structure layout and the function ids are right when the Base Profile
// comes back with its name and the settings read without a version error.
// It never writes (no SetSetting, DeleteProfileSetting,
// RestoreProfileDefaultSetting or SaveSettings).
func TestRealNVAPIReadsBaseProfile(t *testing.T) {
	if procNvQueryInterface.Find() != nil {
		t.Skip("sem nvapi64.dll neste PC")
	}
	n := &Native{}
	profs, err := n.NVDRSProfiles([]string{sys.NVDRSBase, sys.NVDRSGlobal, sys.NVDRSAppRef("cs2.exe")}, sys.NVDRSReadIDs, true)
	if errors.Is(err, sys.ErrNoNVAPI) {
		t.Skipf("NVAPI sem DRS neste PC: %v", err)
	}
	if err != nil {
		t.Fatalf("leitura do DRS: %v", err)
	}
	if len(profs) != 3 {
		t.Fatalf("esperava 3 perfis, obtido %d", len(profs))
	}
	base := profs[0]
	if !base.Found || base.Name == "" {
		t.Fatalf("Base Profile não lido: %+v", base)
	}
	t.Logf("base: name=%q predefined=%v user=%d", base.Name, base.Predefined, len(base.User))
	if len(base.Settings) != len(sys.NVDRSReadIDs) {
		t.Fatalf("settings do base: %d, esperava %d", len(base.Settings), len(sys.NVDRSReadIDs))
	}
	for _, s := range base.Settings {
		t.Logf("  %s (0x%08X): found=%v type=%d location=%d predefined=%v value=%s inProfile=%v",
			sys.NVDRSName(s.ID), s.ID, s.Found, s.Type, s.Location, s.Predefined, sys.NVDRSValueText(s.ID, s.Value), s.InProfile)
		if s.Found && s.Type != sys.NVDRSTypeDWORD {
			t.Errorf("%s não é DWORD no driver (tipo %d)", sys.NVDRSName(s.ID), s.Type)
		}
	}
	seen := map[uint32]bool{}
	for _, s := range base.User {
		t.Logf("  usuário: %s (0x%08X) = %s (location %d)", sys.NVDRSName(s.ID), s.ID, sys.NVDRSValueText(s.ID, s.Value), s.Location)
		if seen[s.ID] {
			t.Errorf("ajuste 0x%08X repetido na lista de valores de usuário", s.ID)
		}
		seen[s.ID] = true
		if !s.User() {
			t.Errorf("0x%08X na lista de usuário sem ser valor de usuário: %+v", s.ID, s)
		}
	}
	// V-Sync and the power mode always exist in the Base Profile with a value.
	for _, id := range []uint32{sys.NVDRSVSyncMode, sys.NVDRSPowerMode} {
		if s, ok := base.Get(id); !ok || !s.Found {
			t.Errorf("%s ausente do Base Profile: %+v", sys.NVDRSName(id), s)
		}
	}
	g := profs[1]
	t.Logf("global: found=%v name=%q (igual ao base: %v)", g.Found, g.Name, g.Name == base.Name)
	app := profs[2]
	t.Logf("app:cs2.exe: found=%v name=%q predefined=%v user=%d", app.Found, app.Name, app.Predefined, len(app.User))
	for _, s := range app.Settings {
		t.Logf("  %s: found=%v location=%d predefined=%v value=%s", sys.NVDRSName(s.ID), s.Found, s.Location, s.Predefined, sys.NVDRSValueText(s.ID, s.Value))
	}
	v, err := n.NVVRR(`\\.\DISPLAY1`)
	t.Logf("VRR DISPLAY1: %+v (erro: %v)", v, err)
}

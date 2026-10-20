package mocksys

import (
	"errors"
	"testing"

	"axion/internal/sys"
)

func TestDRSSimRoundTrip(t *testing.T) {
	m, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := "nvdrs|base|1057eb71"
	if got := m.World()[key]; got != "5|pre" {
		t.Fatalf("estado inicial do modo de energia: %q", got)
	}
	if err := m.N.NVDRSSet(sys.NVDRSBase, sys.NVDRSPowerMode, sys.NVPowerPreferMax); err != nil {
		t.Fatal(err)
	}
	if got := m.World()[key]; got != "1|user" {
		t.Fatalf("depois de Set: %q", got)
	}
	profs, err := m.N.NVDRSProfiles([]string{sys.NVDRSBase, sys.NVDRSGlobal, "app:cs2.exe", "app:nada.exe"}, []uint32{sys.NVDRSPowerMode}, true)
	if err != nil || len(profs) != 4 {
		t.Fatalf("NVDRSProfiles: %v %d", err, len(profs))
	}
	if s, _ := profs[0].Get(sys.NVDRSPowerMode); !s.User() || s.Value != 1 || !s.PredefValid || s.PredefValue != 5 {
		t.Errorf("base: %+v", s)
	}
	if len(profs[0].User) != 1 || profs[0].User[0].ID != sys.NVDRSPowerMode {
		t.Errorf("User do base: %+v", profs[0].User)
	}
	if !profs[1].Found || profs[1].Name != "Base Profile" {
		t.Errorf("global deveria resolver para o Base Profile: %+v", profs[1])
	}
	if !profs[2].Found || profs[2].Name != "Counter-strike 2" {
		t.Errorf("cs2: %+v", profs[2])
	}
	if s, _ := profs[2].Get(sys.NVDRSPowerMode); s.Found {
		t.Errorf("cs2 não tem o ajuste: %+v", s)
	}
	if profs[3].Found {
		t.Errorf("jogo sem perfil: %+v", profs[3])
	}
	if err := m.N.NVDRSRestoreDefault(sys.NVDRSBase, sys.NVDRSPowerMode); err != nil {
		t.Fatal(err)
	}
	if got := m.World()[key]; got != "5|pre" {
		t.Fatalf("depois de RestoreDefault: %q", got)
	}
	if m.N.DRS.Saves != 2 {
		t.Errorf("SaveSettings chamado %d vezes, esperava 2", m.N.DRS.Saves)
	}
	// Without administrator the driver refuses to save; nothing changes.
	m.N.Elevated = false
	err = m.N.NVDRSSet(sys.NVDRSBase, sys.NVDRSPowerMode, 1)
	var ne *sys.NVAPIError
	if !errors.As(err, &ne) || ne.Status != sys.NVAPIInvalidUserPrivilege {
		t.Fatalf("sem administrador: %v", err)
	}
	if got := m.World()[key]; got != "5|pre" {
		t.Fatalf("gravou sem administrador: %q", got)
	}
	m.N.Elevated = true
	if err := m.N.NVDRSSet("app:nada.exe", sys.NVDRSPowerMode, 1); !errors.As(err, &ne) || ne.Status != sys.NVAPIProfileNotFound {
		t.Errorf("perfil inexistente: %v", err)
	}
	m.N.DRS.Absent = true
	if _, err := m.N.NVDRSProfiles([]string{sys.NVDRSBase}, nil, false); !errors.Is(err, sys.ErrNoNVAPI) {
		t.Errorf("sem NVAPI: %v", err)
	}
	if _, err := m.N.NVVRR(`\\.\DISPLAY1`); !errors.Is(err, sys.ErrNoNVAPI) {
		t.Errorf("VRR sem NVAPI: %v", err)
	}
	m.N.DRS.Absent = false
	if v, err := m.N.NVVRR(`\\.\display1`); err != nil || !v.Read || v.Possible || v.Device != `\\.\display1` {
		t.Errorf("VRR: %+v %v", v, err)
	}
	if v, err := m.N.NVVRR(`\\.\DISPLAY9`); err != nil || v.Read {
		t.Errorf("monitor desconhecido: %+v %v", v, err)
	}
}

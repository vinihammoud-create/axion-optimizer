package collect_test

import (
	"context"
	"strings"
	"testing"

	"axion/internal/collect"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

func hasErr(errs []string, frag string) bool {
	for _, e := range errs {
		if strings.Contains(e, frag) {
			return true
		}
	}
	return false
}

func TestCollectNVDRS(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := collect.Collect(context.Background(), m.Platform, nil)
	if len(f.Errors) > 0 {
		t.Fatalf("erros na coleta: %v", f.Errors)
	}
	d := f.Nvidia.DRS
	if !d.Read || d.Error != "" || !d.GlobalIsBase {
		t.Fatalf("DRS: %+v", d)
	}
	if p := d.Profile(sys.NVDRSBase); p == nil || !p.Found || p.Name != "Base Profile" {
		t.Errorf("base: %+v", p)
	}
	if p := d.Profile(sys.NVDRSGlobal); p != nil {
		t.Errorf("o global igual ao base não é listado à parte: %+v", p)
	}
	if p := d.Profile("app:cs2.exe"); p == nil || !p.Found || p.Name != "Counter-strike 2" {
		t.Errorf("cs2: %+v", p)
	}
	if len(d.VRR) != 1 || !d.VRR[0].Read || d.VRR[0].Possible || d.VRR[0].Device != `\\.\DISPLAY1` {
		t.Errorf("VRR: %+v", d.VRR)
	}
	if s, ok := d.Profile(sys.NVDRSBase).Get(sys.NVDRSVSyncMode); !ok || !s.Found || !s.Predefined || s.Value != sys.NVVSyncPassive {
		t.Errorf("V-Sync do base: %+v", s)
	}
	// A distinct global profile and a user value in it.
	m.N.DRS.Profiles[sys.NVDRSGlobal] = &mocksys.DRSProfile{Name: "Meu global"}
	m.N.DRS.SetUser(sys.NVDRSGlobal, sys.NVDRSPowerMode, sys.NVPowerPreferMax)
	f = collect.Collect(context.Background(), m.Platform, nil)
	d = f.Nvidia.DRS
	if d.GlobalIsBase || d.Profile(sys.NVDRSGlobal) == nil || d.Profile(sys.NVDRSGlobal).Name != "Meu global" {
		t.Fatalf("global próprio: %+v", d)
	}
	if v, ok := d.UserValue(sys.NVDRSGlobal, sys.NVDRSPowerMode); !ok || v != sys.NVPowerPreferMax {
		t.Errorf("valor de usuário do global: %d %v", v, ok)
	}
	if v, from, user := d.Effective("app:cs2.exe", sys.NVDRSPowerMode); v != sys.NVPowerPreferMax || from != sys.NVDRSGlobal || !user {
		t.Errorf("efetivo no cs2: %d %s %v", v, from, user)
	}
}

func TestCollectNVDRSAbsent(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m.N.DRS.Absent = true
	f := collect.Collect(context.Background(), m.Platform, nil)
	d := f.Nvidia.DRS
	if d.Read || d.Error == "" || len(d.Profiles) != 0 || len(d.VRR) != 0 {
		t.Fatalf("sem NVAPI: %+v", d)
	}
	if hasErr(f.Errors, "perfil do driver") {
		t.Errorf("PC sem NVAPI não é erro de coleta: %v", f.Errors)
	}
	if !f.Nvidia.Present {
		t.Error("a placa continua NVIDIA")
	}
}

func TestCollectNVDRSNotNvidia(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m.W.Set(`root\cimv2`, "Win32_VideoController", sys.WMIRow{"Name": "AMD Radeon RX 7800 XT", "DriverVersion": "32.0.12033.1030", "DriverDate": "20250115000000.000000-000",
		"PNPDeviceID": `PCI\VEN_1002&DEV_747E&SUBSYS_0E3D1002&REV_C8\6&1E2A9C0&0&0019`, "Status": "OK", "AdapterRAM": uint64(4293918720)})
	f := collect.Collect(context.Background(), m.Platform, nil)
	if f.HasGPUVendor("NVIDIA") {
		t.Fatal("a placa deveria ser AMD")
	}
	d := f.Nvidia.DRS
	if d.Read || d.Error != "" || len(d.Profiles) != 0 || len(d.VRR) != 0 {
		t.Fatalf("placa AMD: DRS deveria ficar zerado: %+v", d)
	}
	if hasErr(f.Errors, "perfil do driver") {
		t.Errorf("erro indevido: %v", f.Errors)
	}
}

func TestCollectNVDRSSkipped(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := collect.CollectWith(context.Background(), m.Platform, nil, collect.Options{Skip: map[string]bool{collect.PartNVDRS: true}})
	d := f.Nvidia.DRS
	if d.Read || !strings.Contains(d.Error, "pulada") {
		t.Fatalf("parte pulada: %+v", d)
	}
	found := false
	for _, u := range f.Unread {
		if u == collect.PartNVDRS {
			found = true
		}
	}
	if !found {
		t.Errorf("a parte pulada deveria constar em Unread: %v", f.Unread)
	}
}

func TestCollectNVDRSDriverError(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// The driver has NVAPI but the Base Profile cannot be read: an error of
	// the collection, not "no NVAPI".
	delete(m.N.DRS.Profiles, sys.NVDRSBase)
	f := collect.Collect(context.Background(), m.Platform, nil)
	if f.Nvidia.DRS.Read || f.Nvidia.DRS.Error == "" {
		t.Fatalf("sem Base Profile: %+v", f.Nvidia.DRS)
	}
	if !hasErr(f.Errors, "perfil do driver NVIDIA") {
		t.Errorf("falha do driver deveria ser reportada: %v", f.Errors)
	}
}

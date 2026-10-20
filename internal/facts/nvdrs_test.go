package facts

import (
	"testing"

	"axion/internal/sys"
)

func TestNvDRSEffective(t *testing.T) {
	user := func(id, v uint32) sys.NVDRSSetting {
		return sys.NVDRSSetting{ID: id, Found: true, InProfile: true, Value: v}
	}
	pre := func(id, v uint32) sys.NVDRSSetting {
		return sys.NVDRSSetting{ID: id, Found: true, InProfile: true, Predefined: true, Value: v}
	}
	d := NvDRS{Read: true, GlobalIsBase: true, Profiles: []sys.NVDRSProfile{
		{Ref: sys.NVDRSBase, Name: "Base Profile", Found: true, Settings: []sys.NVDRSSetting{user(sys.NVDRSFrameRateLimit, 120), pre(sys.NVDRSVSyncMode, sys.NVVSyncPassive)}},
		{Ref: "app:cs2.exe", Name: "Counter-strike 2", Found: true, Settings: []sys.NVDRSSetting{{ID: sys.NVDRSFrameRateLimit}}},
		{Ref: "app:nada.exe"},
	}}
	if v, from, u := d.Effective("app:cs2.exe", sys.NVDRSFrameRateLimit); v != 120 || from != sys.NVDRSBase || !u {
		t.Errorf("herdado do base: %d %s %v", v, from, u)
	}
	if v, from, u := d.Effective("app:cs2.exe", sys.NVDRSVSyncMode); v != sys.NVVSyncPassive || from != sys.NVDRSBase || u {
		t.Errorf("predefinido do base: %d %s %v", v, from, u)
	}
	if v, from, u := d.Effective("app:cs2.exe", sys.NVDRSPowerMode); v != sys.NVPowerOptimal || from != "padrão" || u {
		t.Errorf("padrão do driver: %d %s %v", v, from, u)
	}
	d.Profiles[1].Settings = []sys.NVDRSSetting{pre(sys.NVDRSFrameRateLimit, 0)}
	if v, from, u := d.Effective("app:cs2.exe", sys.NVDRSFrameRateLimit); v != 0 || from != "app:cs2.exe" || u {
		t.Errorf("predefinido do jogo vence o base: %d %s %v", v, from, u)
	}
	if v, ok := d.UserValue(sys.NVDRSBase, sys.NVDRSFrameRateLimit); !ok || v != 120 {
		t.Errorf("UserValue base: %d %v", v, ok)
	}
	if _, ok := d.UserValue(sys.NVDRSBase, sys.NVDRSVSyncMode); ok {
		t.Error("valor predefinido não é de usuário")
	}
	if _, ok := d.UserValue("app:x.exe", sys.NVDRSVSyncMode); ok {
		t.Error("perfil não lido")
	}
	if refs := d.GlobalRefs(); len(refs) != 1 || refs[0] != sys.NVDRSBase {
		t.Errorf("GlobalRefs com global = base: %v", refs)
	}
	if refs := d.AppRefs(); len(refs) != 1 || refs[0] != "app:cs2.exe" {
		t.Errorf("AppRefs só com os perfis que existem: %v", refs)
	}
	// A distinct global profile sits between the game and the Base Profile.
	d.GlobalIsBase = false
	d.Profiles = append(d.Profiles, sys.NVDRSProfile{Ref: sys.NVDRSGlobal, Name: "Meu perfil", Found: true, Settings: []sys.NVDRSSetting{user(sys.NVDRSPowerMode, sys.NVPowerPreferMax)}})
	if refs := d.GlobalRefs(); len(refs) != 2 || refs[1] != sys.NVDRSGlobal {
		t.Errorf("GlobalRefs com global próprio: %v", refs)
	}
	if v, from, u := d.Effective("app:cs2.exe", sys.NVDRSPowerMode); v != sys.NVPowerPreferMax || from != sys.NVDRSGlobal || !u {
		t.Errorf("global próprio: %d %s %v", v, from, u)
	}
}

func TestNvidiaVRRAndPrimaryHz(t *testing.T) {
	f := &Facts{}
	if c, k := f.NvidiaVRR(); c || k {
		t.Error("sem leitura: nada conhecido")
	}
	f.Nvidia.DRS.VRR = []sys.NVVRRInfo{{Device: `\\.\DISPLAY1`, Read: true}}
	if c, k := f.NvidiaVRR(); c || !k {
		t.Error("lido sem VRR: conhecido, não capaz")
	}
	f.Nvidia.DRS.VRR = append(f.Nvidia.DRS.VRR, sys.NVVRRInfo{Device: `\\.\DISPLAY2`, Read: true, Possible: true})
	if c, k := f.NvidiaVRR(); !c || !k {
		t.Error("um monitor com VRR possível basta")
	}
	if f.PrimaryHz() != 0 {
		t.Error("sem monitores")
	}
	f.Displays = []sys.DisplayMode{{Device: `\\.\DISPLAY2`, CurrentHz: 144, MaxHzAtRes: 165}, {Device: `\\.\DISPLAY1`, CurrentHz: 60, MaxHzAtRes: 240, Primary: true}}
	if hz := f.PrimaryHz(); hz != 60 {
		t.Errorf("taxa atual do principal: %d", hz)
	}
	f.Displays[1].Primary = false
	if hz := f.PrimaryHz(); hz != 144 {
		t.Errorf("sem principal cai no primeiro: %d", hz)
	}
	f.Displays[0].CurrentHz = 0
	if hz := f.PrimaryHz(); hz != 165 {
		t.Errorf("taxa atual desconhecida usa a maior disponível: %d", hz)
	}
}

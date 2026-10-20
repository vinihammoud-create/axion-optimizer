package facts

import "testing"

func TestMatchVulnDrivers(t *testing.T) {
	cases := []struct {
		name   string
		mods   []string
		svcs   []DriverService
		want   []string // IDs, in order
		loaded bool     // first result loaded
		regist bool     // first result registered
	}{
		{name: "nada lido", want: nil},
		{name: "drivers comuns", mods: []string{"ntoskrnl.exe", "ndis.sys", "nvlddmkm.sys"},
			svcs: []DriverService{{Name: "Ndis", Path: `C:\Windows\system32\drivers\ndis.sys`}}, want: nil},
		{name: "módulo carregado, caixa diferente", mods: []string{"ndis.sys", "WinRing0x64.sys"}, want: []string{"winring0"}, loaded: true},
		{name: "serviço pelo nome", svcs: []DriverService{{Name: "WinRing0_1_2_0", State: "Stopped", Start: "Manual"}}, want: []string{"winring0"}, regist: true},
		{name: "serviço pelo arquivo com prefixo do kernel", svcs: []DriverService{{Name: "qualquer", Path: `\??\C:\Users\x\AppData\Local\Temp\AMIFLDRV64.sys`}}, want: []string{"amifldrv"}, regist: true},
		{name: "serviço com aspas e argumentos", svcs: []DriverService{{Name: "x", Path: `"C:\Tools\rwdrv.sys" /a`}}, want: []string{"rwdrv"}, regist: true},
		{name: "amigendrv", mods: []string{"amigendrv64.sys"}, want: []string{"amigendrv"}, loaded: true},
		{name: "carregado vem primeiro", mods: []string{"rwdrv.sys"},
			svcs: []DriverService{{Name: "WinRing0_1_2_0", State: "Stopped"}, {Name: "rwdrv", State: "Running"}}, want: []string{"rwdrv", "winring0"}, loaded: true, regist: true},
		{name: "nome parecido não conta", mods: []string{"winring0x64.sys.bak", "myrwdrv.sys"},
			svcs: []DriverService{{Name: "rwdrv2", Path: `C:\x\rwdrv2.sys`}, {Name: "", Path: ""}}, want: nil},
	}
	for _, tc := range cases {
		got := MatchVulnDrivers(tc.mods, tc.svcs)
		if len(got) != len(tc.want) {
			t.Errorf("%s: %d achados, esperado %d (%+v)", tc.name, len(got), len(tc.want), got)
			continue
		}
		for i, id := range tc.want {
			if got[i].ID != id {
				t.Errorf("%s: posição %d é %s, esperado %s", tc.name, i, got[i].ID, id)
			}
		}
		if len(got) > 0 && (got[0].Loaded != tc.loaded || got[0].Registered != tc.regist) {
			t.Errorf("%s: loaded=%v registered=%v, esperado %v/%v", tc.name, got[0].Loaded, got[0].Registered, tc.loaded, tc.regist)
		}
	}
}

// Every known driver names where it usually comes from, and nothing in the
// list is listed twice (a second entry would hide behind the first).
func TestVulnDriverDefsAreComplete(t *testing.T) {
	seen := map[string]string{}
	for _, d := range VulnDriverDefs {
		if d.ID == "" || d.Display == "" || d.Origin == "" || len(d.Files) == 0 {
			t.Errorf("entrada incompleta: %+v", d)
		}
		for _, n := range append(append([]string{}, d.Files...), d.Service...) {
			for _, c := range n {
				if c >= 'A' && c <= 'Z' {
					t.Errorf("%s: %q deve estar em minúsculas", d.ID, n)
					break
				}
			}
			if o, ok := seen[n]; ok {
				t.Errorf("%q repetido em %s e %s", n, o, d.ID)
			}
			seen[n] = d.ID
		}
	}
}

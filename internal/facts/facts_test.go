package facts

import "testing"

func TestCPUClassification(t *testing.T) {
	cases := []struct {
		name           string
		x3d, dual, rap bool
		am5, hybrid    bool
		cores, threads int
	}{
		{"AMD Ryzen 7 7800X3D 8-Core Processor", true, false, false, true, false, 8, 16},
		{"AMD Ryzen 9 9950X3D 16-Core Processor", true, true, false, true, false, 16, 32},
		{"AMD Ryzen 7 5800X3D 8-Core Processor", true, false, false, false, false, 8, 16},
		{"AMD Ryzen 9 7945HX with Radeon Graphics", false, false, false, false, false, 16, 32},
		{"13th Gen Intel(R) Core(TM) i9-13900K", false, false, true, false, true, 24, 32},
		{"14th Gen Intel(R) Core(TM) i7-14700KF", false, false, true, false, true, 20, 28},
		{"13th Gen Intel(R) Core(TM) i5-13400F", false, false, true, false, true, 10, 16},
		{"13th Gen Intel(R) Core(TM) i7-13700H", false, false, false, false, true, 14, 20},
		{"12th Gen Intel(R) Core(TM) i5-12400F", false, false, false, false, false, 6, 12},
		{"13th Gen Intel(R) Core(TM) i3-13100F", false, false, false, false, false, 4, 8},
	}
	for _, c := range cases {
		cpu := CPUInfo{Name: c.name, Cores: c.cores, Threads: c.threads}
		ClassifyCPU(&cpu)
		if cpu.IsX3D != c.x3d || cpu.IsDualCCDX3D != c.dual || cpu.IsRaptorLakeDesktop() != c.rap || cpu.IsAM5() != c.am5 || cpu.IsIntelHybrid != c.hybrid {
			t.Errorf("%s: x3d=%v dual=%v raptor=%v am5=%v hybrid=%v", c.name, cpu.IsX3D, cpu.IsDualCCDX3D, cpu.IsRaptorLakeDesktop(), cpu.IsAM5(), cpu.IsIntelHybrid)
		}
	}
}

func TestGigabitCapable(t *testing.T) {
	fe := Adapter{Description: "Realtek PCIe FE Family Controller", Keywords: map[string]NicKeyword{"*SpeedDuplex": {Values: []string{"0", "1", "2", "3", "4"}}}}
	gb := Adapter{Description: "Intel(R) Ethernet Connection (7) I219-V", Keywords: map[string]NicKeyword{"*SpeedDuplex": {Values: []string{"0", "1", "2", "3", "4", "6"}}}}
	byName := Adapter{Description: "Realtek PCIe GbE Family Controller"}
	unknown := Adapter{Description: "Some Ethernet Adapter"}
	if fe.GigabitCapable() || !gb.GigabitCapable() || !byName.GigabitCapable() || unknown.GigabitCapable() {
		t.Errorf("fe=%v gb=%v byName=%v unknown=%v", fe.GigabitCapable(), gb.GigabitCapable(), byName.GigabitCapable(), unknown.GigabitCapable())
	}
}

func TestPrimaryGPUSkipsVirtualAdapters(t *testing.T) {
	f := &Facts{GPUs: []GPUInfo{
		{Name: "Parsec Virtual Display Adapter", Vendor: "Other", PNPDeviceID: `ROOT\DISPLAY\0000`},
		{Name: "AMD Radeon RX 6600", Vendor: "AMD", PNPDeviceID: `PCI\VEN_1002&DEV_73FF`},
	}}
	if g := f.PrimaryGPU(); g == nil || g.Vendor != "AMD" {
		t.Fatalf("primary GPU: %+v", g)
	}
}

func TestIntelHybridModel(t *testing.T) {
	for name, want := range map[string]bool{
		"12th Gen Intel(R) Core(TM) i5-12400F": false,
		"12th Gen Intel(R) Core(TM) i5-12600K": true,
		"12th Gen Intel(R) Core(TM) i3-12100F": false,
		"13th Gen Intel(R) Core(TM) i5-13400F": true,
		"13th Gen Intel(R) Core(TM) i7-13700K": true,
		"14th Gen Intel(R) Core(TM) i3-14100":  false,
		"12th Gen Intel(R) Core(TM) i5-12500H": true,
		"Intel(R) Core(TM) i5-10400F CPU":      false,
	} {
		if got := intelHybridModel(name); got != want {
			t.Errorf("%s: got %v want %v", name, got, want)
		}
	}
}

// Win32PrioritySeparation (bits AABBCC): the foreground boost is lost when the
// two lowest bits are 00 or the two middle bits are 10 (fixed interval), per
// the Microsoft documentation of the entry (cc976120).
func TestNoForegroundBoost(t *testing.T) {
	cases := []struct {
		v    int
		want bool
		why  string
	}{
		{2, false, "padrão do Windows cliente: curto, variável, 3:1"},
		{0x26, false, "opção Programas: curto, variável, 3:1"},
		{0x16, false, "longo, variável, 3:1"},
		{0x15, false, "variável, 2:1"},
		{0x18, true, "opção Serviços em segundo plano: fixo e igual"},
		{0x1A, true, "longo, fixo (escrito por otimizadores): o 3:1 não vale em intervalo fixo"},
		{0x29, true, "intervalo fixo, mesmo com 2:1 nos bits finais"},
		{0x2A, true, "curto, fixo"},
		{0x28, true, "fixo e igual"},
		{0x14, true, "bits finais 00 prevalecem sobre o intervalo variável"},
		{0x24, true, "bits finais 00 prevalecem sobre o intervalo variável"},
		{0, true, "tudo em 00: igual"},
		{0x3F, false, "bits do meio 11 = variável no cliente; 3:1"},
		{-1, false, "valor ausente"},
	}
	for _, c := range cases {
		r := Residue{Readable: true, PrioritySeparation: c.v}
		if got := r.NoForegroundBoost(); got != c.want {
			t.Errorf("0x%X (%s): NoForegroundBoost = %v, quer %v", c.v, c.why, got, c.want)
		}
	}
	if (Residue{PrioritySeparation: 0x1A}).NoForegroundBoost() {
		t.Error("residuo não lido nunca conta como problema")
	}
}

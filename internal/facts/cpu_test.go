package facts

import "testing"

func TestClassifyCPUMobileAndHX3D(t *testing.T) {
	for _, c := range []struct {
		name           string
		cores, threads int
		gen            int
		x3d, dual      bool
	}{
		{"Intel(R) Core(TM) i7-1065G7 CPU @ 1.30GHz", 4, 8, 10, false, false},
		{"11th Gen Intel(R) Core(TM) i7-1165G7 @ 2.80GHz", 4, 8, 11, false, false},
		{"12th Gen Intel(R) Core(TM) i7-1255U", 10, 12, 12, false, false},
		{"13th Gen Intel(R) Core(TM) i5-1335U", 10, 12, 13, false, false},
		{"Intel(R) Core(TM) i7-4790K CPU @ 4.00GHz", 4, 8, 4, false, false},
		{"13th Gen Intel(R) Core(TM) i5-13400F", 10, 16, 13, false, false},
		{"AMD Ryzen 9 7945HX3D with Radeon Graphics", 16, 32, 0, true, true},
		{"AMD Ryzen 9 9955HX3D 16-Core Processor", 16, 32, 0, true, true},
		{"AMD Ryzen 7 7800X3D 8-Core Processor", 8, 16, 0, true, false},
		{"Intel(R) Core(TM) 7 150U", 10, 12, 13, false, false},
		{"Intel(R) Core(TM) 5 120U", 10, 12, 13, false, false},
	} {
		cpu := CPUInfo{Name: c.name, Cores: c.cores, Threads: c.threads}
		ClassifyCPU(&cpu)
		if cpu.IntelGen != c.gen || cpu.IsX3D != c.x3d || cpu.IsDualCCDX3D != c.dual {
			t.Errorf("%s: gen=%d x3d=%v dual=%v (esperado %d %v %v)", c.name, cpu.IntelGen, cpu.IsX3D, cpu.IsDualCCDX3D, c.gen, c.x3d, c.dual)
		}
	}
}

func TestIntelArcIntegrated(t *testing.T) {
	for name, integ := range map[string]bool{
		"Intel(R) Arc(TM) Graphics":        true,
		"Intel(R) Arc(TM) 140V GPU (16GB)": true,
		"Intel(R) Arc(TM) A770 Graphics":   false,
		"Intel(R) Arc(TM) B580 Graphics":   false,
		"Intel(R) Arc(TM) A370M Graphics":  false,
		"Intel(R) UHD Graphics 770":        true,
	} {
		if v, i := ClassifyGPUVendor(name, `PCI\VEN_8086&DEV_0000`); v != "Intel" || i != integ {
			t.Errorf("%s: %s integrada=%v (esperado %v)", name, v, i, integ)
		}
	}
}

func TestRadeonStrixHaloIntegrated(t *testing.T) {
	if _, i := ClassifyGPUVendor("AMD Radeon(TM) 8060S Graphics", `PCI\VEN_1002&DEV_0000`); !i {
		t.Error("Radeon 8060S é a gráfica integrada do Ryzen AI Max")
	}
	if _, i := ClassifyGPUVendor("AMD Radeon RX 7600S", `PCI\VEN_1002&DEV_0000`); i {
		t.Error("RX 7600S é dedicada (notebook)")
	}
}

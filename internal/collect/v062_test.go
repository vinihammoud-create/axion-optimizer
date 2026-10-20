package collect_test

import (
	"context"
	"math"
	"strings"
	"testing"

	"axion/internal/collect"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

// "Update Revision": Intel keeps the revision in the high DWORD of 8 bytes,
// AMD in the low one; some systems store 4 bytes or a QWORD. The first
// notebook report read 0 on an Intel CPU.
func TestMicrocodeRevision(t *testing.T) {
	cases := []struct {
		name   string
		v      sys.RegValue
		vendor string
		want   uint32
	}{
		{"Intel, DWORD alto", sys.Binary([]byte{0, 0, 0, 0, 0x2F, 0x01, 0, 0}), "Intel", 0x12F},
		{"Intel, só o DWORD baixo preenchido", sys.Binary([]byte{0x2F, 0x01, 0, 0, 0, 0, 0, 0}), "Intel", 0x12F},
		{"AMD, DWORD baixo", sys.Binary([]byte{0x15, 0x42, 0x60, 0x0A, 0, 0, 0, 0}), "AMD", 0x0A604215},
		{"AMD, só o alto", sys.Binary([]byte{0, 0, 0, 0, 0x15, 0x42, 0x60, 0x0A}), "AMD", 0x0A604215},
		{"4 bytes", sys.Binary([]byte{0x3A, 0, 0, 0}), "Intel", 0x3A},
		{"QWORD", sys.QWord(0x12B << 32), "Intel", 0x12B},
		{"DWORD", sys.DWord(0x12B), "Intel", 0x12B},
		{"vazio", sys.Binary(nil), "Intel", 0},
	}
	for _, c := range cases {
		got, raw := collect.MicrocodeRevision(c.v, c.vendor)
		if got != c.want {
			t.Errorf("%s: 0x%X, esperado 0x%X (raw %s)", c.name, got, c.want, raw)
		}
	}
	if _, raw := collect.MicrocodeRevision(sys.Binary([]byte{0, 0, 0, 0, 0x2F, 0x01, 0, 0}), "Intel"); raw != "00 00 00 00 2F 01 00 00" {
		t.Errorf("dado bruto: %q", raw)
	}
}

func TestParseCPUIdentifier(t *testing.T) {
	if f, m, s := collect.ParseCPUIdentifier("Intel64 Family 6 Model 183 Stepping 1"); f != 6 || m != 0xB7 || s != 1 {
		t.Errorf("Intel: %d %d %d", f, m, s)
	}
	if f, m, s := collect.ParseCPUIdentifier("AMD64 Family 25 Model 97 Stepping 2"); f != 25 || m != 97 || s != 2 {
		t.Errorf("AMD: %d %d %d", f, m, s)
	}
	if f, _, _ := collect.ParseCPUIdentifier("x86"); f != 0 {
		t.Error("identificador sem assinatura")
	}
}

func TestLinkMbps(t *testing.T) {
	if collect.LinkMbps(math.MaxUint64, false) != 0 || collect.LinkMbps(math.MaxUint64, true) != 0 {
		t.Error("velocidade indefinida (2^64-1) deveria virar 0")
	}
	if collect.LinkMbps(1_000_000_000, false) != 0 {
		t.Error("placa desconectada não tem velocidade de link")
	}
	if collect.LinkMbps(866_700_000, true) != 866 {
		t.Error("Wi-Fi 866 Mb/s")
	}
}

// The machine profile of the first notebook report, as the new collection
// reads it.
func TestCollectSecurityProcessorAndNIC(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const cpuKey = `HKLM\HARDWARE\DESCRIPTION\System\CentralProcessor\0`
	_ = m.R.SetValue(cpuKey, "Identifier", sys.SZ("AMD64 Family 25 Model 97 Stepping 2"))
	_ = m.R.SetValue(cpuKey, "Update Revision", sys.Binary([]byte{0x15, 0x42, 0x60, 0x0A, 0, 0, 0, 0}))
	// VBS running with nothing listed, DMA protection available, a policy
	// that keeps VBS on, and two features that start the hypervisor.
	m.W.Set(`root\Microsoft\Windows\DeviceGuard`, "Win32_DeviceGuard", sys.WMIRow{"VirtualizationBasedSecurityStatus": int64(2),
		"SecurityServicesRunning": []any{int64(0)}, "SecurityServicesConfigured": []any{int64(1), int64(2)}, "AvailableSecurityProperties": []any{int64(1), int64(2), int64(3), int64(5)}})
	_ = m.R.SetValue(`HKLM\SOFTWARE\Policies\Microsoft\Windows\DeviceGuard`, "EnableVirtualizationBasedSecurity", sys.DWord(1))
	_ = m.R.SetValue(`HKLM\SOFTWARE\Policies\Microsoft\Windows\DeviceGuard`, "HypervisorEnforcedCodeIntegrity", sys.DWord(1))
	m.W.Set(`root\cimv2`, "Win32_OptionalFeature",
		sys.WMIRow{"Name": "VirtualMachinePlatform", "InstallState": int64(1)},
		sys.WMIRow{"Name": "HypervisorPlatform", "InstallState": int64(1)},
		sys.WMIRow{"Name": "Microsoft-Hyper-V-Hypervisor", "InstallState": int64(2)},
		sys.WMIRow{"Name": "Printing-PrintToPDFServices-Features", "InstallState": int64(1)})
	// Windows reports no firmware virtualization while the hypervisor runs.
	m.W.Set(`root\cimv2`, "Win32_Processor", sys.WMIRow{"Name": "AMD Ryzen 7 7800X3D 8-Core Processor", "Manufacturer": "AuthenticAMD", "NumberOfCores": int64(8), "NumberOfLogicalProcessors": int64(16), "MaxClockSpeed": int64(4201), "VirtualizationFirmwareEnabled": false})
	// Driver texts of an advanced property (Intel-style).
	eth := mocksys.NetClass + `\0001`
	_ = m.R.SetValue(eth+`\Ndi\Params\*FlowControl`, "ParamDesc", sys.SZ("Flow Control"))
	_ = m.R.SetValue(eth+`\Ndi\Params\*FlowControl\enum`, "3", sys.SZ("Rx & Tx Enabled"))
	_ = m.R.SetValue(eth+`\Ndi\Params\*EEE`, "ParamDesc", sys.SZ("@netrtwlane.inf,%eee%;Energy-Efficient Ethernet"))

	f := collect.Collect(context.Background(), m.Platform, nil)
	c := f.CPU
	if c.Family != 25 || c.Model != 97 || c.Stepping != 2 || c.Microcode != 0x0A604215 || !strings.HasPrefix(c.MicrocodeRaw, "Update Revision = 15 42 60 0A") {
		t.Errorf("CPU: %+v", c)
	}
	s := f.Security
	if !s.VBSRunning || s.HVCIRunning || len(s.ServicesRunning) != 0 || len(s.ServicesConfigured) != 2 || s.IOMMU != "on" {
		t.Errorf("Device Guard: %+v", s)
	}
	if s.VBSPolicy != "on" || s.HVCIPolicy != "lock" || !s.VBSLocked {
		t.Errorf("política/bloqueio: %+v", s)
	}
	if !s.VMPEnabled || len(s.HypervisorFeatures) != 2 || !strings.Contains(strings.Join(s.HypervisorFeatures, ","), "Hipervisor do Windows") {
		t.Errorf("recursos do hipervisor: %v (VMP %v)", s.HypervisorFeatures, s.VMPEnabled)
	}
	if !s.HypervisorPresent || !s.VirtualizationFW {
		t.Errorf("com hipervisor em execução a virtualização do firmware está ligada: %+v", s)
	}
	kw := f.ActiveAdapter().Keywords
	if kw["*FlowControl"].Name != "Flow Control" || kw["*FlowControl"].Label("3") != "Rx & Tx Enabled" || kw["*EEE"].Name != "Energy-Efficient Ethernet" {
		t.Errorf("textos do driver: %+v / %+v", kw["*FlowControl"], kw["*EEE"])
	}
	n := f.Nvidia
	if n.PCIeGenCur != 1 || n.PState != "P8" || n.UtilPct != 2 || n.PCIeWidth != 16 {
		t.Errorf("nvidia-smi: %+v", n)
	}
	for _, a := range f.Adapters {
		if !a.Up && a.LinkSpeedMbps != 0 {
			t.Errorf("%s desconectada com %d Mb/s", a.Name, a.LinkSpeedMbps)
		}
	}

	// IOMMU off: DMA protection not in the list. Unknown: the list is empty.
	m.W.Set(`root\Microsoft\Windows\DeviceGuard`, "Win32_DeviceGuard", sys.WMIRow{"VirtualizationBasedSecurityStatus": int64(0), "AvailableSecurityProperties": []any{int64(1), int64(2)}})
	if f := collect.Collect(context.Background(), m.Platform, nil); f.Security.IOMMU != "off" || f.Security.VBSRunning {
		t.Errorf("IOMMU desligado: %+v", f.Security)
	}
	m.W.Set(`root\Microsoft\Windows\DeviceGuard`, "Win32_DeviceGuard", sys.WMIRow{"VirtualizationBasedSecurityStatus": int64(0)})
	if f := collect.Collect(context.Background(), m.Platform, nil); f.Security.IOMMU != "unknown" {
		t.Errorf("IOMMU desconhecido: %q", f.Security.IOMMU)
	}
}

// A driver that refuses one of the new nvidia-smi fields still gives the
// fields of earlier versions.
func TestNvidiaQueryFallback(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prev := m.X.Handler
	queries := 0
	m.X.Handler = func(exe string, args []string, ps string) (sys.Result, bool) {
		if strings.Contains(strings.ToLower(exe), "nvidia-smi") && len(args) > 0 && strings.HasPrefix(args[0], "--query-gpu") {
			queries++
			if strings.Contains(args[0], "pstate") {
				return sys.Result{ExitCode: 2, Stdout: "Field \"pstate\" is not a valid field to query.\r\n"}, true
			}
			return sys.Result{Stdout: "552.22, 8192, 40, 3, 8, 16\r\n"}, true
		}
		return prev(exe, args, ps)
	}
	f := collect.Collect(context.Background(), m.Platform, nil)
	n := f.Nvidia
	if queries != 2 || !n.SMI || n.Driver != "552.22" || n.PCIeWidth != 8 || n.PCIeWidthMax != 16 || n.PCIeGenCur != 0 || n.UtilPct != -1 {
		t.Errorf("consulta de reserva: %d consultas, %+v", queries, n)
	}
}

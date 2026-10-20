package collect_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"axion/internal/collect"
	"axion/internal/diag"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

// NVMe disks use the drive's own health log; other disks the reliability
// counters reached through the WMI association (direct enumeration returned
// nothing in the first real report).
func TestDiskHealthSources(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := collect.Collect(context.Background(), m.Platform, nil)
	if len(f.Disks) != 2 {
		t.Fatalf("discos: %+v", f.Disks)
	}
	nv, sata := f.Disks[0], f.Disks[1]
	if nv.HealthSource != "nvme" || nv.NVMe == nil || nv.WearPct != 81 || nv.TempC != 46 || nv.PowerOnHours != 9120 || !nv.IsSystem {
		t.Errorf("NVMe: %+v", nv)
	}
	if nv.NVMe != nil && nv.NVMe.DataWrittenBytes/1e12 != 48 {
		t.Errorf("TB gravados: %d", nv.NVMe.DataWrittenBytes)
	}
	if sata.HealthSource != "wmi" || sata.NVMe != nil || sata.WearPct != 12 || sata.TempC != 38 || sata.PowerOnHours != 15210 || sata.BusType != "SATA" {
		t.Errorf("SATA: %+v", sata)
	}

	// Health log unavailable: the NVMe disk falls back to the counters and the
	// failure is recorded.
	delete(m.N.NVMe, 0)
	f = collect.Collect(context.Background(), m.Platform, nil)
	if d := f.Disks[0]; d.HealthSource != "wmi" || d.WearPct != 80 || d.NVMe != nil {
		t.Errorf("fallback: %+v", d)
	}
	found := false
	for _, e := range f.Errors {
		if strings.Contains(e, "saúde NVMe do disco 0") {
			found = true
		}
	}
	if !found {
		t.Errorf("falha do log NVMe não registrada: %v", f.Errors)
	}
}

// Real report 0.3.1: the NVMe health log failed and the WMI counters came
// back as zeros (wear 0 %, 0 hours) — that is "unknown", not a new drive.
func TestZeroedCountersAreUnknown(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	delete(m.N.NVMe, 0)
	m.W.SetAssoc(`root\Microsoft\Windows\Storage`, "MSFT_PhysicalDisk.ObjectId="+mocksys.MockDiskObjectID(0), "MSFT_StorageReliabilityCounter",
		sys.WMIRow{"DeviceId": "0", "Wear": int64(0), "Temperature": int64(41), "PowerOnHours": int64(0)})
	f := collect.Collect(context.Background(), m.Platform, nil)
	if d := f.Disks[0]; d.WearPct != -1 || d.TempC != 41 {
		t.Fatalf("contadores zerados deveriam ser desconhecidos: %+v", d)
	}
	r := diag.Build(f, nil)
	found := false
	for _, x := range r.Findings {
		if strings.HasPrefix(x.ID, "disk.health.unknown.") {
			found = true
		}
		if strings.HasPrefix(x.ID, "disk.wear.") {
			t.Errorf("desgaste inventado: %+v", x)
		}
	}
	if !found {
		t.Error("o diagnóstico deveria avisar que o desgaste do SSD do sistema não foi lido")
	}
}

// TRIM, the drive optimizer task and the last retrim per volume.
func TestTrimFacts(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := collect.Collect(context.Background(), m.Platform, nil)
	tr := f.Trim
	if !tr.Readable || tr.NTFS != 0 || tr.ReFS != 0 || tr.Source != "fsutil" {
		t.Errorf("TRIM: %+v", tr)
	}
	if !tr.TaskRead || !tr.TaskExists || !tr.TaskOn || tr.TaskLast.IsZero() {
		t.Errorf("tarefa: %+v", tr)
	}
	c, d := tr.LastRetrim["C:"], tr.LastRetrim["D:"]
	if age := time.Since(c); age < 11*24*time.Hour || age > 13*24*time.Hour {
		t.Errorf("último retrim de C: deveria ser o mais novo (12 dias): %v", c)
	}
	if age := time.Since(d); age < 69*24*time.Hour || age > 71*24*time.Hour {
		t.Errorf("último retrim de D: %v", d)
	}
	if len(f.Volumes) != 2 || f.Volumes[0].Disk != 0 || !f.Volumes[0].SSD || f.Volumes[1].Disk != 1 || !f.Volumes[1].SSD {
		t.Errorf("volumes: %+v", f.Volumes)
	}
	if f.Boot.FwPOSTms != 31800 {
		t.Errorf("tempo de POST: %d", f.Boot.FwPOSTms)
	}
	if len(f.Controllers) != 3 || strings.Join(f.Residue.StorageDrivers, ",") != "storahci,stornvme" {
		t.Errorf("controladoras: %+v / drivers %v", f.Controllers, f.Residue.StorageDrivers)
	}
	if f.Trim.OptimizerEvents != 3 || f.Trim.LastEvent != "retrim C:" {
		t.Errorf("eventos do otimizador: %d %q", f.Trim.OptimizerEvents, f.Trim.LastEvent)
	}

	// fsutil unreadable: the registry value is used.
	prev := m.X.Handler
	m.X.Handler = func(exe string, args []string, ps string) (sys.Result, bool) {
		if strings.Contains(strings.ToLower(exe), "fsutil") {
			return sys.Result{ExitCode: 1, Stdout: "Erro: acesso negado.\r\n"}, true
		}
		return prev(exe, args, ps)
	}
	_ = m.R.SetValue(sys.KeyFileSystem, "DisableDeleteNotification", sys.DWord(1))
	f = collect.Collect(context.Background(), m.Platform, nil)
	if f.Trim.NTFS != 1 || f.Trim.Source != "registro" || !f.Trim.Readable {
		t.Errorf("fallback do registro: %+v", f.Trim)
	}
}

func TestPnPProblemsIgnorePhantomDevices(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m.W.Set(`root\cimv2`, "Win32_PnPEntity",
		sys.WMIRow{"Name": "Pendrive USB", "ConfigManagerErrorCode": int64(45), "PNPClass": "DiskDrive", "Present": false},
		sys.WMIRow{"Name": "Monitor antigo", "ConfigManagerErrorCode": int64(24), "PNPClass": "Monitor", "Present": false},
		sys.WMIRow{"Name": "Placa de som", "ConfigManagerErrorCode": int64(43), "PNPClass": "MEDIA", "Present": true},
		sys.WMIRow{"Name": "Webcam desativada", "ConfigManagerErrorCode": int64(22), "PNPClass": "Camera", "Present": true})
	f := collect.Collect(context.Background(), m.Platform, nil)
	if len(f.PnPProblems) != 1 || f.PnPProblems[0].Name != "Placa de som" {
		t.Fatalf("only the present device with a real problem counts: %+v", f.PnPProblems)
	}
}

func TestParsePagingFilesMixed(t *testing.T) {
	if m, mx := collect.ParsePagingFiles([]string{`C:\pagefile.sys 0 0`, `D:\pagefile.sys 16 16`}); m != "system" || mx != 0 {
		t.Errorf("system-managed + small custom: %s %d", m, mx)
	}
	if m, mx := collect.ParsePagingFiles([]string{`C:\pagefile.sys 4096 4096`, `D:\pagefile.sys 4096 4096`}); m != "custom" || mx != 8192 {
		t.Errorf("two custom files: %s %d", m, mx)
	}
}

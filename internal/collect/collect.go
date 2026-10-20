// Package collect builds the machine profile (facts.Facts) using only the
// platform interfaces, so the same code runs on Windows and on the mock.
package collect

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"axion/internal/bcd"
	"axion/internal/facts"
	"axion/internal/games"
	"axion/internal/programs"
	"axion/internal/startup"
	"axion/internal/sys"
)

// Progress reports the step being collected.
type Progress func(step string, done, total int)

// Options tune a collection.
type Options struct {
	// Skip names steps or parts of a step (see Parts) not to run: they ended
	// an earlier collection on this machine.
	Skip map[string]bool
	// Mark is told when a risky part of a step starts (done=false) and ends
	// (done=true), so a collection that dies inside it can name it.
	Mark func(part string, done bool)
}

// Parts of a step that can be skipped on their own (native probes that talk
// to drivers). Their names are also what Mark reports.
const (
	PartHAGS  = "Configurações de jogo › agendamento de GPU (driver de vídeo)"
	PartX3D   = facts.PartX3D
	PartNVMe  = "Armazenamento › saúde NVMe (driver do disco)"
	PartInput = "Mouse, teclado e portas USB › dispositivos USB e HID"
	PartNVDRS = "Perfil do driver NVIDIA › leitura pelo NVAPI (driver de vídeo)"
)

type collector struct {
	ctx  context.Context
	p    *sys.Platform
	f    *facts.Facts
	opts Options
}

// part runs a risky part of a step under its own name: it can be skipped on
// its own, and a collection that dies inside it names it.
func (c *collector) part(name string, fn func()) {
	if c.opts.Skip[name] {
		c.errf("%s: pulado (encerrou uma análise anterior neste PC)", name)
		c.f.Unread = append(c.f.Unread, name)
		return
	}
	if c.opts.Mark != nil {
		c.opts.Mark(name, false)
		defer c.opts.Mark(name, true)
	}
	fn()
}

func (c *collector) errf(format string, args ...any) {
	c.f.Errors = append(c.f.Errors, fmt.Sprintf(format, args...))
}

func (c *collector) query(ns, wql string) []sys.WMIRow {
	rows, err := c.p.WMI.Query(ns, wql)
	if err != nil {
		c.errf("WMI %s: %v", firstWord(wql), err)
		return nil
	}
	return rows
}

func firstWord(wql string) string {
	up := strings.ToUpper(wql)
	if i := strings.Index(up, " FROM "); i >= 0 {
		rest := strings.Fields(wql[i+6:])
		if len(rest) > 0 {
			return rest[0]
		}
	}
	return wql
}

func (c *collector) regStr(path, name string) string {
	v, err := c.p.Reg.GetValue(path, name)
	if err != nil || !v.Exists {
		return ""
	}
	switch v.Kind {
	case sys.RegDWord, sys.RegQWord:
		return strconv.FormatUint(v.Int, 10)
	case sys.RegMultiSZ:
		return strings.Join(v.Strs, ",")
	}
	return v.Str
}

func (c *collector) regInt(path, name string, def int64) int64 {
	v, err := c.p.Reg.GetValue(path, name)
	if err != nil || !v.Exists {
		return def
	}
	switch v.Kind {
	case sys.RegDWord, sys.RegQWord:
		return int64(v.Int)
	case sys.RegSZ, sys.RegExpandSZ:
		n, err := strconv.ParseInt(strings.TrimSpace(v.Str), 10, 64)
		if err == nil {
			return n
		}
	}
	return def
}

// Collect gathers the full machine profile. It never fails: individual step
// errors are recorded in Facts.Errors.
func Collect(ctx context.Context, p *sys.Platform, progress Progress) *facts.Facts {
	return CollectWith(ctx, p, progress, Options{})
}

// CollectWith is Collect with options.
func CollectWith(ctx context.Context, p *sys.Platform, progress Progress, opts Options) *facts.Facts {
	start := time.Now()
	bcd.Invalidate(p)
	c := &collector{ctx: ctx, p: p, f: &facts.Facts{Collected: start}, opts: opts}
	steps := []struct {
		name string
		fn   func()
	}{
		{facts.StepOS, c.os},
		{"Processador", c.cpu},
		{"Placa de vídeo", c.gpu},
		{"Memória RAM", c.ram},
		{"Placa-mãe e BIOS", c.board},
		{"Armazenamento", c.disks},
		{"TRIM e otimização do SSD", c.trim},
		{"Monitores", c.displays},
		{"Rede", c.network},
		{facts.StepSecurity, c.security},
		{facts.StepGaming, c.gaming},
		{facts.StepPower, c.power},
		{"Driver NVIDIA", c.nvidia},
		{"Perfil do driver NVIDIA", c.nvdrs},
		{"Inicialização do sistema", c.boot},
		{"Processos em execução", c.processes},
		{StepThermal, c.thermal},
		{"Dispositivos com problema", c.pnp},
		{"Dispositivos PCI", c.pciDevices},
		{"Arquivos temporários", c.temp},
		{"Ajustes deixados por outros otimizadores", c.residue},
		{"Mouse, teclado e portas USB", c.peripherals},
		{facts.StepHealth, c.health},
		{facts.StepVulnDrivers, c.vulnDrivers},
	}
	for i, s := range steps {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		if progress != nil {
			progress(s.name, i, len(steps))
		}
		if opts.Skip[s.name] {
			c.errf("%s: etapa pulada (encerrou uma análise anterior neste PC)", s.name)
			c.f.Unread = append(c.f.Unread, s.name)
			continue
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					c.errf("%s: falha interna: %v", s.name, r)
					c.f.Unread = append(c.f.Unread, s.name)
				}
			}()
			s.fn()
		}()
	}
	if progress != nil {
		progress("Concluído", len(steps), len(steps))
	}
	c.f.ScanSeconds = time.Since(start).Seconds()
	return c.f
}

// ---------------------------------------------------------------------------

const keyNTCurrent = `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion`

// KeyActiveHours holds the active hours of Windows Update as the Settings
// app writes them.
const KeyActiveHours = `HKLM\SOFTWARE\Microsoft\WindowsUpdate\UX\Settings`

// KeyActiveHoursPolicy is the group policy that fixes the active hours
// (SetActiveHours, ActiveHoursStart, ActiveHoursEnd); it wins over the
// Settings values.
const KeyActiveHoursPolicy = `HKLM\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate`

func (c *collector) os() {
	f := c.f
	_, _, build, ubr := c.p.Native.WindowsBuild()
	f.OS.Build, f.OS.UBR = build, ubr
	if b := c.regStr(keyNTCurrent, "CurrentBuild"); b != "" && f.OS.Build == 0 {
		f.OS.Build, _ = strconv.Atoi(b)
	}
	f.OS.DisplayVersion = c.regStr(keyNTCurrent, "DisplayVersion")
	f.OS.Edition = c.regStr(keyNTCurrent, "EditionID")
	// Delivery Optimization: the policy wins; otherwise the Settings choice
	// (stored for the Network Service account); otherwise the default, LAN.
	f.OS.DOMode, f.OS.DOSource = 1, "padrão"
	if v, err := c.p.Reg.GetValue(`HKLM\SOFTWARE\Policies\Microsoft\Windows\DeliveryOptimization`, "DODownloadMode"); err == nil && v.Exists && v.Kind == sys.RegDWord {
		f.OS.DOMode, f.OS.DOSource = int(v.Int), "política"
	} else if v, err := c.p.Reg.GetValue(`HKU\S-1-5-20\Software\Microsoft\Windows\CurrentVersion\DeliveryOptimization\Settings`, "DownloadMode"); err == nil && v.Exists && v.Kind == sys.RegDWord {
		f.OS.DOMode, f.OS.DOSource = int(v.Int), "configurações"
	}
	f.OS.IsWin11 = f.OS.Build >= 22000
	f.OS.Firmware = c.p.Native.FirmwareType()
	f.OS.ActiveHoursStart = int(c.regInt(KeyActiveHours, "ActiveHoursStart", -1))
	f.OS.ActiveHoursEnd = int(c.regInt(KeyActiveHours, "ActiveHoursEnd", -1))
	f.OS.ActiveHoursAuto = int(c.regInt(KeyActiveHours, "SmartActiveHoursState", -1))
	if c.regInt(KeyActiveHoursPolicy, "SetActiveHours", 0) == 1 {
		f.OS.ActiveHoursPolicy = true
		f.OS.ActiveHoursPolicyStart = int(c.regInt(KeyActiveHoursPolicy, "ActiveHoursStart", -1))
		f.OS.ActiveHoursPolicyEnd = int(c.regInt(KeyActiveHoursPolicy, "ActiveHoursEnd", -1))
	}
	if ts := c.regInt(keyNTCurrent, "InstallDate", 0); ts > 0 {
		f.OS.InstallDate = time.Unix(ts, 0)
	}
	if rows := c.query(`root\cimv2`, "SELECT Caption, LastBootUpTime, SystemDrive, Locale FROM Win32_OperatingSystem"); len(rows) > 0 {
		r := rows[0]
		f.OS.Caption = strings.TrimSpace(r.Str("Caption"))
		f.OS.LastBoot = r.Time("LastBootUpTime")
		f.OS.SystemDrive = r.Str("SystemDrive")
		f.OS.Locale = r.Str("Locale")
	}
	if f.OS.Caption == "" {
		f.OS.Caption = c.regStr(keyNTCurrent, "ProductName")
		if f.OS.IsWin11 {
			f.OS.Caption = strings.Replace(f.OS.Caption, "Windows 10", "Windows 11", 1)
		}
	}
	if f.OS.SystemDrive == "" {
		f.OS.SystemDrive = "C:"
	}
	if !f.OS.LastBoot.IsZero() {
		f.OS.UptimeHours = time.Since(f.OS.LastBoot).Hours()
	}
	paths := c.p.Native.Paths()
	f.OS.User, f.OS.ElevatedAs = paths.User, paths.ElevatedAs
	for _, k := range []string{
		`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending`,
		`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired`,
	} {
		if ok, _ := c.p.Reg.KeyExists(k); ok {
			f.OS.PendingReboot = true
		}
	}
	if h, err := os.Hostname(); err == nil {
		f.Hostname = h
	}
}

func (c *collector) cpu() {
	rows := c.query(`root\cimv2`, "SELECT Name, Manufacturer, NumberOfCores, NumberOfLogicalProcessors, MaxClockSpeed FROM Win32_Processor")
	for _, r := range rows {
		if c.f.CPU.Name == "" {
			c.f.CPU.Name = strings.Join(strings.Fields(r.Str("Name")), " ")
			c.f.CPU.MaxMHz = int(r.Int("MaxClockSpeed"))
			if strings.Contains(strings.ToLower(r.Str("Manufacturer")), "amd") {
				c.f.CPU.Vendor = "AMD"
			} else if strings.Contains(strings.ToLower(r.Str("Manufacturer")), "intel") {
				c.f.CPU.Vendor = "Intel"
			}
		}
		c.f.CPU.Cores += int(r.Int("NumberOfCores"))
		c.f.CPU.Threads += int(r.Int("NumberOfLogicalProcessors"))
	}
	facts.ClassifyCPU(&c.f.CPU)
	const cpuKey = `HKLM\HARDWARE\DESCRIPTION\System\CentralProcessor\0`
	if id := c.regStr(cpuKey, "Identifier"); id != "" {
		c.f.CPU.Family, c.f.CPU.Model, c.f.CPU.Stepping = ParseCPUIdentifier(id)
	}
	// Running microcode revision ("Update Revision"); when it cannot be read,
	// the revision the firmware loaded ("Previous Update Revision").
	for _, name := range []string{"Update Revision", "Previous Update Revision"} {
		v, err := c.p.Reg.GetValue(cpuKey, name)
		if err != nil || !v.Exists {
			continue
		}
		rev, raw := MicrocodeRevision(v, c.f.CPU.Vendor)
		if raw == "" {
			continue
		}
		if c.f.CPU.MicrocodeRaw != "" {
			c.f.CPU.MicrocodeRaw += "; "
		}
		c.f.CPU.MicrocodeRaw += name + " = " + raw
		if rev != 0 {
			c.f.CPU.Microcode = rev
			break
		}
	}
}

var reCPUIdentifier = regexp.MustCompile(`(?i)Family\s+(\d+)\s+Model\s+(\d+)\s+Stepping\s+(\d+)`)

// ParseCPUIdentifier reads the CPUID signature from the processor
// "Identifier" Windows keeps ("Intel64 Family 6 Model 183 Stepping 1").
func ParseCPUIdentifier(s string) (family, model, stepping int) {
	m := reCPUIdentifier.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, 0
	}
	family, _ = strconv.Atoi(m[1])
	model, _ = strconv.Atoi(m[2])
	stepping, _ = strconv.Atoi(m[3])
	return
}

// MicrocodeRevision decodes the "Update Revision" registry value and returns
// the revision with the raw data in hex. Windows stores it as 8 bytes
// (REG_BINARY) with the Intel revision in the high DWORD and the AMD one in
// the low DWORD (the layout Firefox and OpenJDK read); some systems store 4
// bytes or a QWORD. The half that holds the vendor's revision is preferred;
// the other is used when it is the only one that is not zero.
func MicrocodeRevision(v sys.RegValue, vendor string) (uint32, string) {
	var b []byte
	switch v.Kind {
	case sys.RegBinary:
		b = v.Bin
	case sys.RegQWord:
		b = make([]byte, 8)
		for i := range b {
			b[i] = byte(v.Int >> (8 * i))
		}
	case sys.RegDWord:
		b = []byte{byte(v.Int), byte(v.Int >> 8), byte(v.Int >> 16), byte(v.Int >> 24)}
	default:
		return 0, ""
	}
	raw := strings.ToUpper(fmt.Sprintf("% x", b))
	le := func(p []byte) uint32 {
		return uint32(p[0]) | uint32(p[1])<<8 | uint32(p[2])<<16 | uint32(p[3])<<24
	}
	switch {
	case len(b) >= 8:
		lo, hi := le(b[0:4]), le(b[4:8])
		first, second := hi, lo // Intel
		if vendor == "AMD" {
			first, second = lo, hi
		}
		if first != 0 {
			return first, raw
		}
		return second, raw
	case len(b) >= 4:
		return le(b[0:4]), raw
	}
	return 0, raw
}

const keyDisplayClass = `HKLM\SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`

var reNvVer = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)\.(\d+)`)

// NvidiaVersion converts a Windows driver version (32.0.15.6636) into the
// NVIDIA marketing version (566.36).
func NvidiaVersion(winVer string) string {
	m := reNvVer.FindStringSubmatch(winVer)
	if m == nil {
		return ""
	}
	digits := m[3] + fmt.Sprintf("%04s", m[4])
	digits = strings.ReplaceAll(digits, " ", "0")
	if len(digits) < 5 {
		return ""
	}
	d := digits[len(digits)-5:]
	return strings.TrimLeft(d[:3], "0") + "." + d[3:]
}

func (c *collector) gpu() {
	rows := c.query(`root\cimv2`, "SELECT Name, DriverVersion, DriverDate, PNPDeviceID, Status, AdapterRAM, InfFilename FROM Win32_VideoController")
	// VRAM from the driver key (Win32_VideoController.AdapterRAM caps at 4 GB).
	vram := map[string]int{}
	if subs, err := c.p.Reg.SubKeys(keyDisplayClass); err == nil {
		for _, s := range subs {
			if len(s) != 4 {
				continue
			}
			k := keyDisplayClass + `\` + s
			mid := strings.ToLower(c.regStr(k, "MatchingDeviceId"))
			if v, err := c.p.Reg.GetValue(k, "HardwareInformation.qwMemorySize"); err == nil && v.Exists {
				vram[mid] = int(v.Int >> 20)
			} else if v, err := c.p.Reg.GetValue(k, "HardwareInformation.MemorySize"); err == nil && v.Exists {
				if v.Kind == sys.RegBinary && len(v.Bin) >= 4 {
					n := uint64(v.Bin[0]) | uint64(v.Bin[1])<<8 | uint64(v.Bin[2])<<16 | uint64(v.Bin[3])<<24
					vram[mid] = int(n >> 20)
				} else {
					vram[mid] = int(v.Int >> 20)
				}
			}
		}
	}
	for _, r := range rows {
		g := facts.GPUInfo{
			Name: strings.TrimSpace(r.Str("Name")), DriverVersion: r.Str("DriverVersion"),
			DriverDate: r.Time("DriverDate"), PNPDeviceID: r.Str("PNPDeviceID"), Status: r.Str("Status"),
		}
		g.Vendor, g.Integrated = facts.ClassifyGPUVendor(g.Name, g.PNPDeviceID)
		pnp := strings.ToLower(g.PNPDeviceID)
		for mid, mb := range vram {
			if mid != "" && strings.HasPrefix(pnp, mid) {
				g.VRAMMiB = mb
			}
		}
		if g.VRAMMiB == 0 {
			if n := r.Uint("AdapterRAM"); n > 0 {
				g.VRAMMiB = int(n >> 20)
			}
		}
		if g.Vendor == "NVIDIA" {
			g.VendorVersion = NvidiaVersion(g.DriverVersion)
		}
		// Windows' own basic driver: the vendor still shows in the PCI ID
		// (VEN_10DE for an NVIDIA card) and the name is translated
		// ("Adaptador de Vídeo Básico da Microsoft"), so the driver package
		// decides.
		g.InfFile = strings.ToLower(strings.TrimSpace(r.Str("InfFilename")))
		low := strings.ToLower(g.Name)
		g.BasicDriver = g.InfFile == "display.inf" || g.InfFile == "basicdisplay.inf" ||
			(g.InfFile == "" && (strings.Contains(low, "basic display") || strings.Contains(low, "vídeo básico") || strings.Contains(low, "video básico")))
		if g.BasicDriver {
			g.Status = "sem driver"
		}
		c.f.GPUs = append(c.f.GPUs, g)
	}
}

func (c *collector) ram() {
	rows := c.query(`root\cimv2`, "SELECT Capacity, Speed, ConfiguredClockSpeed, Manufacturer, PartNumber, DeviceLocator, BankLabel, SMBIOSMemoryType FROM Win32_PhysicalMemory")
	for _, r := range rows {
		m := facts.MemModule{
			CapacityBytes: r.Uint("Capacity"), SpeedMTs: int(r.Int("Speed")), ConfiguredMTs: int(r.Int("ConfiguredClockSpeed")),
			Manufacturer: strings.TrimSpace(r.Str("Manufacturer")), PartNumber: strings.TrimSpace(r.Str("PartNumber")),
			Locator: r.Str("DeviceLocator"), Bank: r.Str("BankLabel"), SMBIOSType: int(r.Int("SMBIOSMemoryType")),
		}
		c.f.RAM.Modules = append(c.f.RAM.Modules, m)
		c.f.RAM.TotalBytes += m.CapacityBytes
		if t := facts.MemTypeFromSMBIOS(m.SMBIOSType); t != "" {
			c.f.RAM.Type = t
		}
		if m.ConfiguredMTs > 0 && (c.f.RAM.ConfiguredMTs == 0 || m.ConfiguredMTs < c.f.RAM.ConfiguredMTs) {
			c.f.RAM.ConfiguredMTs = m.ConfiguredMTs
		}
		if m.SpeedMTs > c.f.RAM.RatedMTs {
			c.f.RAM.RatedMTs = m.SpeedMTs
		}
	}
	if ml, err := c.p.Native.MemoryLists(); err == nil {
		c.f.Memory = ml
		if c.f.RAM.TotalBytes == 0 {
			c.f.RAM.TotalBytes = ml.Total
		}
	}
}

func (c *collector) board() {
	if rows := c.query(`root\cimv2`, "SELECT Manufacturer, Product FROM Win32_BaseBoard"); len(rows) > 0 {
		c.f.Board.Manufacturer = strings.TrimSpace(rows[0].Str("Manufacturer"))
		c.f.Board.Product = strings.TrimSpace(rows[0].Str("Product"))
	}
	if rows := c.query(`root\cimv2`, "SELECT Manufacturer, SMBIOSBIOSVersion, ReleaseDate FROM Win32_BIOS"); len(rows) > 0 {
		c.f.Board.BIOSVendor = strings.TrimSpace(rows[0].Str("Manufacturer"))
		c.f.Board.BIOSVersion = strings.TrimSpace(rows[0].Str("SMBIOSBIOSVersion"))
		c.f.Board.BIOSDate = rows[0].Time("ReleaseDate")
	}
	if rows := c.query(`root\cimv2`, "SELECT Manufacturer, Model, PCSystemType, HypervisorPresent FROM Win32_ComputerSystem"); len(rows) > 0 {
		c.f.Board.SystemMaker = strings.TrimSpace(rows[0].Str("Manufacturer"))
		c.f.Board.SystemModel = strings.TrimSpace(rows[0].Str("Model"))
		c.f.Board.PCSystemType = int(rows[0].Int("PCSystemType"))
		c.f.Security.HypervisorPresent = rows[0].Bool("HypervisorPresent")
	}
	if rows := c.query(`root\cimv2`, "SELECT ChassisTypes FROM Win32_SystemEnclosure"); len(rows) > 0 {
		c.f.Board.ChassisTypes = rows[0].Ints("ChassisTypes")
	}
	if c.f.Board.FormFactor() == "laptop" {
		c.f.Power.IsLaptop = true
	}
}

func mediaType(n int64) string {
	switch n {
	case 3:
		return "HDD"
	case 4:
		return "SSD"
	case 5:
		return "SCM"
	}
	return "Unspecified"
}

func busType(n int64) string {
	switch n {
	case 17:
		return "NVMe"
	case 11:
		return "SATA"
	case 7:
		return "USB"
	case 8:
		return "RAID"
	case 3:
		return "ATA"
	case 10:
		return "SAS"
	case 12:
		return "SD"
	}
	return fmt.Sprintf("Bus %d", n)
}

func healthStatus(n int64) string {
	switch n {
	case 0:
		return "Healthy"
	case 1:
		return "Warning"
	case 2:
		return "Unhealthy"
	}
	return "Unknown"
}

const nsStorage = `root\Microsoft\Windows\Storage`

func (c *collector) disks() {
	sysLetter := strings.ToUpper(strings.TrimSuffix(c.f.OS.SystemDrive, ":"))
	sysDisk := -1
	letterDisk := map[string]int{} // "C:" -> disk number
	for _, r := range c.query(nsStorage, "SELECT DiskNumber, DriveLetter FROM MSFT_Partition") {
		dl := r.Str("DriveLetter")
		if n := r.Int("DriveLetter"); n > 0 && n < 128 {
			dl = string(rune(n))
		}
		dl = strings.ToUpper(strings.TrimSpace(strings.TrimRight(dl, ":\x00")))
		if dl == "" {
			continue
		}
		letterDisk[dl+":"] = int(r.Int("DiskNumber"))
		if strings.EqualFold(dl, sysLetter) {
			sysDisk = int(r.Int("DiskNumber"))
		}
	}
	for _, r := range c.query(nsStorage, "SELECT DeviceId, ObjectId, FriendlyName, MediaType, BusType, Size, HealthStatus FROM MSFT_PhysicalDisk") {
		d := facts.Disk{
			Model: strings.TrimSpace(r.Str("FriendlyName")), MediaType: mediaType(r.Int("MediaType")), BusType: busType(r.Int("BusType")),
			SizeBytes: r.Uint("Size"), Health: healthStatus(r.Int("HealthStatus")), WearPct: -1, TempC: -1,
		}
		d.Number, _ = strconv.Atoi(r.Str("DeviceId"))
		if d.BusType == "NVMe" {
			// The drive's own health log: works on every NVMe SSD through the
			// inbox driver, unlike the WMI counters (often empty for NVMe).
			c.part(PartNVMe, func() {
				if h, err := c.p.Native.NVMeHealth(d.Number); err == nil {
					d.NVMe = &h
					d.WearPct, d.TempC, d.PowerOnHours = h.PercentUsed, h.TempC, clampInt(h.PowerOnHours)
					d.HealthSource = "nvme"
				} else {
					c.errf("saúde NVMe do disco %d: %v", d.Number, err)
				}
			})
		}
		if d.HealthSource == "" {
			c.reliability(r.Str("ObjectId"), &d)
		}
		if d.BusType == "NVMe" && d.MediaType == "Unspecified" {
			d.MediaType = "SSD"
		}
		d.IsSystem = d.Number == sysDisk
		c.f.Disks = append(c.f.Disks, d)
	}
	if sysDisk < 0 && len(c.f.Disks) == 1 {
		c.f.Disks[0].IsSystem = true
	}
	for _, r := range c.query(`root\cimv2`, "SELECT Name, DriverName, PNPDeviceID FROM Win32_SCSIController") {
		c.f.Controllers = append(c.f.Controllers, facts.StorageController{
			Name: strings.TrimSpace(r.Str("Name")), Driver: strings.TrimSpace(r.Str("DriverName")), PNP: r.Str("PNPDeviceID")})
	}
	for _, r := range c.query(`root\cimv2`, "SELECT DeviceID, VolumeName, FileSystem, Size, FreeSpace FROM Win32_LogicalDisk WHERE DriveType=3") {
		v := facts.Volume{
			Letter: r.Str("DeviceID"), Label: r.Str("VolumeName"), FS: r.Str("FileSystem"),
			SizeBytes: r.Uint("Size"), FreeBytes: r.Uint("FreeSpace"), Disk: -1,
		}
		if n, ok := letterDisk[strings.ToUpper(v.Letter)]; ok {
			v.Disk = n
			for _, d := range c.f.Disks {
				if d.Number == n && (d.MediaType == "SSD" || d.BusType == "NVMe") {
					v.SSD = true
				}
			}
		}
		c.f.Volumes = append(c.f.Volumes, v)
	}
}

// trim reads TRIM, the drive optimizer task and the last retrim per volume.
func (c *collector) trim() {
	t := &c.f.Trim
	t.NTFS, t.ReFS = -1, -1
	if st, err := sys.QueryTrim(c.ctxOrBG(), c.p); err != nil {
		c.errf("TRIM: %v", err)
	} else {
		t.Readable, t.NTFS, t.ReFS, t.Source = true, st.NTFS, st.ReFS, st.Source
	}
	if ti, err := c.p.Tasks.Get(sys.DefragTask); err != nil {
		c.errf("tarefa de otimização de unidades: %v", err)
	} else {
		t.TaskRead, t.TaskExists, t.TaskOn, t.TaskLast, t.TaskResult = true, ti.Exists, ti.Exists && ti.Enabled, ti.LastRun, ti.LastResult
	}
	t.LastRetrim, t.OptimizerEvents, t.LastEvent, t.Operations = optimizerHistory(c.p)
}

// LastRetrims returns the newest successful retrim per drive letter found in
// the Application log (nil when the log has none).
func LastRetrims(p *sys.Platform) map[string]time.Time {
	m, _, _, _ := optimizerHistory(p)
	return m
}

// RefreshRetrims re-reads the drive optimizer history into t (after a
// retrim made from Axion).
func RefreshRetrims(p *sys.Platform, t *facts.Trim) {
	t.LastRetrim, t.OptimizerEvents, t.LastEvent, t.Operations = optimizerHistory(p)
}

// optimizerHistory reads the drive optimizer's completion events (event 258):
// the newest retrim per drive letter, how many events there are and the
// newest one's data.
func optimizerHistory(p *sys.Platform) (map[string]time.Time, int, string, map[string]int) {
	evs, err := p.Events.Query("Application", "*[System[(EventID=258)]]", 400)
	if err != nil {
		return nil, 0, "", nil
	}
	var out map[string]time.Time
	var ops map[string]int
	n, last := 0, ""
	for _, ev := range evs {
		op, letter, isOpt := sys.OptimizerEvent(ev)
		if !isOpt {
			continue
		}
		n++
		if last == "" {
			last = strings.TrimSpace(op + " " + letter)
		}
		if op != "" && len(op) <= 60 {
			if ops == nil {
				ops = map[string]int{}
			}
			ops[op]++
		}
		if letter == "" || !sys.IsRetrim(op) {
			continue
		}
		if out == nil {
			out = map[string]time.Time{}
		}
		if ev.Time.After(out[letter]) {
			out[letter] = ev.Time
		}
	}
	return out, n, last, ops
}

// reliability reads the Windows storage reliability counters of a disk.
// MSFT_StorageReliabilityCounter cannot be enumerated directly (the query
// returns nothing, as seen in the first real report): it is only reachable
// through its association with MSFT_PhysicalDisk, like Get-StorageReliabilityCounter.
func (c *collector) reliability(objectID string, d *facts.Disk) {
	if objectID == "" {
		return
	}
	wql := "ASSOCIATORS OF {MSFT_PhysicalDisk.ObjectId=" + wqlQuote(objectID) + "} WHERE ResultClass = MSFT_StorageReliabilityCounter"
	rows, err := c.p.WMI.Query(nsStorage, wql)
	if err != nil || len(rows) == 0 {
		return // not supported by this disk or bus (e.g. USB bridges)
	}
	rr := rows[0]
	// Some drivers fill the counters with zeros when the drive's health log
	// cannot be read: wear 0 with 0 power-on hours means "unknown", not new.
	wearKnown := rr.Int("Wear") > 0 || rr.Int("PowerOnHours") > 0
	if v, ok := rr["Wear"]; ok && v != nil && wearKnown {
		d.WearPct = int(rr.Int("Wear"))
		d.HealthSource = "wmi"
	}
	if v, ok := rr["Temperature"]; ok && v != nil {
		if t := int(rr.Int("Temperature")); t > 0 {
			d.TempC = t
			d.HealthSource = "wmi"
		}
	}
	if h := int(rr.Int("PowerOnHours")); h > 0 {
		d.PowerOnHours = h
		d.HealthSource = "wmi"
	}
}

// wqlQuote quotes a key value for a WMI object path.
func wqlQuote(v string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"`
}

func clampInt(v uint64) int {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(v)
}

func (c *collector) displays() {
	d, err := c.p.Native.DisplayModes()
	if err != nil {
		c.errf("monitores: %v", err)
		return
	}
	c.f.Displays = d
}

const keyNetClass = `HKLM\SYSTEM\CurrentControlSet\Control\Class\{4d36e972-e325-11ce-bfc1-08002be10318}`

func (c *collector) network() {
	ads, err := c.p.Native.Adapters()
	if err != nil {
		c.errf("adaptadores: %v", err)
	}
	// Map interface GUID -> driver instance key.
	classKeys := map[string]string{}
	if subs, err := c.p.Reg.SubKeys(keyNetClass); err == nil {
		for _, s := range subs {
			if len(s) != 4 {
				continue
			}
			k := keyNetClass + `\` + s
			if id := c.regStr(k, "NetCfgInstanceId"); id != "" {
				classKeys[strings.ToLower(id)] = k
			}
		}
	}
	pnp := map[string]string{}
	for _, r := range c.query(`root\cimv2`, "SELECT GUID, PNPDeviceID FROM Win32_NetworkAdapter WHERE PhysicalAdapter=TRUE") {
		pnp[strings.ToLower(r.Str("GUID"))] = r.Str("PNPDeviceID")
	}
	for _, a := range ads {
		if !a.Physical {
			continue
		}
		kind := "Other"
		switch a.IfType {
		case 6:
			kind = "Ethernet"
		case 71:
			kind = "Wi-Fi"
		}
		ad := facts.Adapter{
			Name: a.Name, Description: a.Description, Kind: kind, LinkSpeedMbps: LinkMbps(a.SpeedBps, a.Up),
			Up: a.Up, InterfaceGUID: a.GUID, IfIndex: a.IfIndex, HasGateway: len(a.Gateways) > 0,
			Gateways: a.Gateways, DNSServers: a.DNS, DHCP: a.DHCP, MTU: a.MTU, PnPCapabilities: -1,
		}
		if k, ok := classKeys[strings.ToLower(a.GUID)]; ok {
			ad.ClassKey = k
			ad.DriverVersion = c.regStr(k, "DriverVersion")
			ad.PnPCapabilities = c.regInt(k, "PnPCapabilities", -1)
			ad.Keywords = c.nicKeywords(k)
		}
		ad.PnPDeviceID = pnp[strings.ToLower(a.GUID)]
		c.f.Adapters = append(c.f.Adapters, ad)
	}
	// Global TCP state.
	if rows := c.query(`root\StandardCimv2`, "SELECT SettingName, AutoTuningLevelLocal FROM MSFT_NetTCPSetting WHERE SettingName='Internet'"); len(rows) > 0 {
		names := map[int64]string{0: "disabled", 1: "highlyrestricted", 2: "restricted", 3: "normal", 4: "experimental"}
		if s, ok := rows[0]["AutoTuningLevelLocal"].(string); ok && s != "" {
			c.f.Net.TCPAutoTuning = strings.ToLower(s)
		} else {
			c.f.Net.TCPAutoTuning = names[rows[0].Int("AutoTuningLevelLocal")]
		}
		c.f.Net.Readable = true
	}
	if rows := c.query(`root\StandardCimv2`, "SELECT ReceiveSideScaling FROM MSFT_NetOffloadGlobalSetting"); len(rows) > 0 {
		if rows[0].Int("ReceiveSideScaling") == 1 || strings.EqualFold(rows[0].Str("ReceiveSideScaling"), "enabled") {
			c.f.Net.RSS = "enabled"
		} else {
			c.f.Net.RSS = "disabled"
		}
	} else {
		c.f.Net.Readable = false
	}
}

// LinkMbps converts the link speed Windows reports into Mb/s. A disconnected
// adapter reports an undefined speed (the first real report showed
// 18446744073709 Mb/s, i.e. 2^64-1 bits per second): it is 0 here.
func LinkMbps(bps uint64, up bool) int64 {
	if !up || bps == math.MaxUint64 || bps > 1<<50 {
		return 0
	}
	return int64(bps / 1_000_000)
}

// nicKeywords reads the advanced properties the driver exposes.
func (c *collector) nicKeywords(instanceKey string) map[string]facts.NicKeyword {
	out := map[string]facts.NicKeyword{}
	params := instanceKey + `\Ndi\Params`
	subs, err := c.p.Reg.SubKeys(params)
	if err != nil {
		return out
	}
	for _, kw := range subs {
		k := params + `\` + kw
		info := facts.NicKeyword{Default: c.regStr(k, "default"), Name: driverText(c.regStr(k, "ParamDesc"))}
		if vals, err := c.p.Reg.ValueNames(k + `\enum`); err == nil {
			info.Values = vals
			for _, v := range vals {
				if l := driverText(c.regStr(k+`\enum`, v)); l != "" {
					if info.Labels == nil {
						info.Labels = map[string]string{}
					}
					info.Labels[v] = l
				}
			}
		}
		if v, err := c.p.Reg.GetValue(instanceKey, kw); err == nil && v.Exists {
			info.Set = true
			info.Current = c.regStr(instanceKey, kw)
		} else {
			info.Current = info.Default
		}
		out[kw] = info
	}
	return out
}

func (c *collector) security() {
	s := &c.f.Security
	s.IOMMU = "unknown"
	if rows := c.query(`root\Microsoft\Windows\DeviceGuard`, "SELECT VirtualizationBasedSecurityStatus, SecurityServicesRunning, SecurityServicesConfigured, AvailableSecurityProperties FROM Win32_DeviceGuard"); len(rows) > 0 {
		st := rows[0].Int("VirtualizationBasedSecurityStatus")
		s.VBSRunning = st == 2
		s.VBSConfigured = st >= 1
		s.ServicesRunning = ints(rows[0].Ints("SecurityServicesRunning"))
		s.ServicesConfigured = ints(rows[0].Ints("SecurityServicesConfigured"))
		s.AvailableProps = ints(rows[0].Ints("AvailableSecurityProperties"))
		for _, v := range s.ServicesRunning {
			if v == 2 {
				s.HVCIRunning = true
			}
		}
		// "DMA protection available" needs the IOMMU (VT-d/AMD-Vi) on in the
		// firmware. The list is empty when Windows did not evaluate it.
		if len(s.AvailableProps) > 0 {
			s.IOMMU = "off"
			for _, v := range s.AvailableProps {
				if v == 3 {
					s.IOMMU = "on"
				}
			}
		}
	}
	// Group policy and UEFI lock: they decide whether a change in the
	// registry can turn VBS off.
	const dgPolicy = `HKLM\SOFTWARE\Policies\Microsoft\Windows\DeviceGuard`
	switch c.regInt(dgPolicy, "EnableVirtualizationBasedSecurity", -1) {
	case 0:
		s.VBSPolicy = "off"
	case 1:
		s.VBSPolicy = "on"
	}
	switch c.regInt(dgPolicy, "HypervisorEnforcedCodeIntegrity", -1) {
	case 0:
		s.HVCIPolicy = "off"
	case 1:
		s.HVCIPolicy = "lock"
	case 2:
		s.HVCIPolicy = "on"
	}
	const dg = `HKLM\SYSTEM\CurrentControlSet\Control\DeviceGuard`
	s.VBSLocked = s.HVCIPolicy == "lock" || c.regInt(dg, "Locked", 0) == 1 ||
		c.regInt(dg+`\Scenarios\HypervisorEnforcedCodeIntegrity`, "Locked", 0) == 1
	// Every scenario a component enabled asks Windows for VBS, whether or
	// not Win32_DeviceGuard lists a service running for it.
	if subs, err := c.p.Reg.SubKeys(dg + `\Scenarios`); err == nil {
		sort.Strings(subs)
		for _, k := range subs {
			if c.regInt(dg+`\Scenarios\`+k, "Enabled", 0) == 1 {
				s.Scenarios = append(s.Scenarios, k)
			}
		}
	}
	switch v := c.regInt(`HKLM\SYSTEM\CurrentControlSet\Control\SecureBoot\State`, "UEFISecureBootEnabled", -1); v {
	case 1:
		s.SecureBoot = "on"
	case 0:
		s.SecureBoot = "off"
	default:
		s.SecureBoot = "unsupported"
	}
	if rows, err := c.p.WMI.Query(`root\CIMV2\Security\MicrosoftTpm`, "SELECT IsEnabled_InitialValue, IsActivated_InitialValue, SpecVersion FROM Win32_Tpm"); err == nil {
		s.TPMRead = true
		if len(rows) > 0 {
			s.TPMPresent = true
			s.TPMReady = rows[0].Bool("IsEnabled_InitialValue") && rows[0].Bool("IsActivated_InitialValue")
			spec := rows[0].Str("SpecVersion")
			if i := strings.Index(spec, ","); i > 0 {
				spec = spec[:i]
			}
			s.TPMVersion = strings.TrimSpace(spec)
		}
	} else {
		c.errf("TPM: %v", err)
	}
	// Windows features that start the hypervisor by themselves.
	wql := "SELECT Name, InstallState FROM Win32_OptionalFeature WHERE "
	for i, ft := range hypervisorFeatures {
		if i > 0 {
			wql += " OR "
		}
		wql += "Name='" + ft.name + "'"
	}
	enabled := map[string]bool{}
	for _, r := range c.query(`root\cimv2`, wql) {
		if r.Int("InstallState") == 1 {
			enabled[strings.ToLower(r.Str("Name"))] = true
		}
	}
	s.VMPEnabled = enabled["virtualmachineplatform"]
	for _, ft := range hypervisorFeatures {
		if enabled[strings.ToLower(ft.name)] && !containsStr(s.HypervisorFeatures, ft.label) {
			s.HypervisorFeatures = append(s.HypervisorFeatures, ft.label)
		}
	}
	if rows := c.query(`root\cimv2`, "SELECT VirtualizationFirmwareEnabled FROM Win32_Processor"); len(rows) > 0 {
		s.VirtualizationFW = rows[0].Bool("VirtualizationFirmwareEnabled")
	}
	// With a hypervisor running, Windows reports the firmware virtualization
	// as off (the hypervisor owns VT-x/AMD-V); it is on, or the hypervisor
	// would not run.
	if s.HypervisorPresent {
		s.VirtualizationFW = true
	}
	s.DefenderRealtime = "unknown"
	if rows, err := c.p.WMI.Query(`root\Microsoft\Windows\Defender`, "SELECT RealTimeProtectionEnabled FROM MSFT_MpComputerStatus"); err == nil && len(rows) > 0 {
		if rows[0].Bool("RealTimeProtectionEnabled") {
			s.DefenderRealtime = "on"
		} else {
			s.DefenderRealtime = "off"
		}
	}
	s.BitLockerOS = "unknown"
	if rows, err := c.p.WMI.Query(`root\CIMV2\Security\MicrosoftVolumeEncryption`, "SELECT DriveLetter, ProtectionStatus FROM Win32_EncryptableVolume"); err == nil {
		s.BitLockerOS = "off"
		sd := strings.ToUpper(strings.TrimSpace(c.f.OS.SystemDrive))
		if sd == "" {
			sd = "C:"
		}
		for _, r := range rows {
			if strings.EqualFold(strings.TrimSpace(r.Str("DriveLetter")), sd) {
				switch r.Int("ProtectionStatus") {
				case 1:
					s.BitLockerOS = "on"
				case 2:
					s.BitLockerOS = "unknown"
				}
			}
		}
	}
}

// antiCheatPrograms matches installed programs (name or publisher, lower
// case) to the anti-cheat they bring.
var antiCheatPrograms = []struct{ Match, Name string }{
	{"faceit", "FACEIT"},
	{"gamersclub", "Gamers Club"},
	{"gamers club", "Gamers Club"},
	{"riot vanguard", "Vanguard"},
}

// Anti-cheat services that indicate an installed anti-cheat. FACEIT's own
// support pages name the user-mode service "FACEITService"; the kernel driver
// registers as "FACEIT". The first real report missed FACEIT because only
// the driver name was checked.
var antiCheatServices = []struct{ Service, Name string }{
	{"FACEITService", "FACEIT"},
	{"FACEIT", "FACEIT"},
	{"vgc", "Vanguard"},
	{"vgk", "Vanguard"},
	{"EasyAntiCheat", "EasyAntiCheat"},
	{"EasyAntiCheat_EOS", "EasyAntiCheat"},
	{"BEService", "BattlEye"},
	{"EAAntiCheatService", "EA Javelin"},
}

func (c *collector) gaming() {
	g := &c.f.Gaming
	g.GameModeOn = c.regInt(`HKCU\Software\Microsoft\GameBar`, "AutoGameModeEnabled", 1) != 0
	g.BackgroundRecord = c.regInt(`HKCU\Software\Microsoft\Windows\CurrentVersion\GameDVR`, "AppCaptureEnabled", 0) != 0
	g.HistoricalRecord = c.regInt(`HKCU\Software\Microsoft\Windows\CurrentVersion\GameDVR`, "HistoricalCaptureEnabled", 0) != 0
	g.HAGS = int(c.regInt(`HKLM\SYSTEM\CurrentControlSet\Control\GraphicsDrivers`, "HwSchMode", 0))
	c.part(PartHAGS, func() {
		if hs, err := c.p.Native.GPUScheduling(); err == nil {
			switch {
			case !hs.Supported:
				g.HAGSState = "unsupported"
			case hs.Enabled:
				g.HAGSState = "on"
			default:
				g.HAGSState = "off"
			}
			g.HAGSDefault = hs.EnabledByDefault
		}
	})
	g.WindowedOpt = strings.Contains(strings.ReplaceAll(c.regStr(`HKCU\Software\Microsoft\DirectX\UserGpuPreferences`, "DirectXUserGlobalSettings"), " ", ""), "SwapEffectUpgradeEnable=1")
	g.MouseAccelOn = strings.TrimSpace(c.regStr(`HKCU\Control Panel\Mouse`, "MouseSpeed")) != "0"
	g.StickyHotkeyOn = c.regInt(`HKCU\Control Panel\Accessibility\StickyKeys`, "Flags", 510)&4 != 0
	// Anti-cheats are found by their service entry in the registry: nothing
	// is asked of the anti-cheat services themselves.
	seen := map[string]bool{}
	for _, ac := range antiCheatServices {
		if ok, _ := c.p.Reg.KeyExists(`HKLM\SYSTEM\CurrentControlSet\Services\` + ac.Service); ok && !seen[ac.Name] {
			seen[ac.Name] = true
			g.AntiCheats = append(g.AntiCheats, ac.Name)
		}
	}
	// Clients whose service is only registered while the game runs, or whose
	// service name is not documented (Gamers Club), still leave an entry in
	// the installed programs: the second report missed FACEIT and the GC on
	// a PC that plays both.
	for _, pr := range programs.List(c.p.Reg) {
		name := strings.ToLower(pr.Name + " " + pr.Publisher)
		for _, ac := range antiCheatPrograms {
			if strings.Contains(name, ac.Match) && !seen[ac.Name] {
				seen[ac.Name] = true
				g.AntiCheats = append(g.AntiCheats, ac.Name)
			}
		}
	}
	g.X3DServiceState = "não verificado"
	c.part(PartX3D, func() {
		g.X3DServiceState = "ausente"
		if info, err := c.p.Svc.Query("amd3dvcacheSvc"); err == nil && info.Exists {
			g.X3DServiceState = "parado"
			if info.Running {
				g.X3DServiceState = "em execução"
			}
		}
	})
	// Xbox Game Bar package registration for the current user.
	repo := `HKCU\Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages`
	if subs, err := c.p.Reg.SubKeys(repo); err == nil {
		for _, s := range subs {
			if strings.HasPrefix(strings.ToLower(s), "microsoft.xboxgamingoverlay_") {
				g.GameBarInstalled = true
			}
		}
	}
}

func (c *collector) power() {
	pw := &c.f.Power
	if act, err := c.p.Power.ActiveScheme(); err == nil {
		pw.ActiveScheme = strings.ToLower(act)
	} else {
		c.errf("plano de energia: %v", err)
	}
	if list, err := c.p.Power.Schemes(); err == nil {
		pw.Schemes = list
		for _, s := range list {
			if strings.EqualFold(s.GUID, pw.ActiveScheme) {
				pw.ActiveName = s.Name
			}
		}
	}
	if o, err := c.p.Power.ActiveOverlay(); err == nil {
		pw.OverlayAC = o
	}
	if hasBat, onAC, pct, err := c.p.Native.PowerStatus(); err == nil {
		// A battery alone does not make a notebook: a USB UPS ("nobreak")
		// also shows up as the system battery. The firmware's form factor
		// decides when it says something.
		if hasBat && c.f.Board.FormFactor() != "desktop" {
			pw.IsLaptop = true
		}
		pw.OnBattery = hasBat && !onAC && pw.IsLaptop
		pw.BatteryPercent = pct
	}
	pw.HibernateOn = c.regInt(`HKLM\SYSTEM\CurrentControlSet\Control\Power`, "HibernateEnabled", 1) != 0
	pw.FastStartupOn = pw.HibernateOn && c.regInt(`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Power`, "HiberbootEnabled", 1) != 0
	pw.CoreParkMinAC, pw.USBSuspendAC = -1, -1
	if pw.ActiveScheme != "" {
		if v, err := c.p.Power.ReadValue(pw.ActiveScheme, "54533251-82be-4824-96c1-47b60b740d00", "0cc5b647-c1df-4637-891a-dec35c318583", true); err == nil {
			pw.CoreParkMinAC = int(v)
		}
		if v, err := c.p.Power.ReadValue(pw.ActiveScheme, "2a737441-1930-4402-8d77-b2bebba308a3", "48e6b7a6-50f5-4782-a5d4-53bb8f07e226", true); err == nil {
			pw.USBSuspendAC = int(v)
		}
	}
}

func (c *collector) nvidia() {
	if !c.f.HasGPUVendor("NVIDIA") {
		return
	}
	c.f.Nvidia.Present = true
	exe := NvidiaSMIPath(c.p.Native.Paths())
	if c.p.IsMock {
		exe = "nvidia-smi.exe"
	}
	if exe == "" {
		return
	}
	ctx, cancel := context.WithTimeout(c.ctxOrBG(), 12*time.Second)
	defer cancel()
	// The current link generation, the performance state and the load tell
	// whether the GPU was idle when read: NVIDIA documents that the link
	// "may be reduced when the GPU is not in use". New fields go at the end so
	// the first six keep their positions.
	const base = "driver_version,memory.total,temperature.gpu,pcie.link.gen.max,pcie.link.width.current,pcie.link.width.max"
	res, err := c.p.Run.Run(ctx, exe, []string{"--query-gpu=" + base + ",pcie.link.gen.current,pstate,utilization.gpu", "--format=csv,noheader,nounits"}, sys.RunOpts{Timeout: 12 * time.Second})
	if err == nil && res.ExitCode != 0 && ctx.Err() == nil {
		// A driver that does not know one of the new fields refuses the whole
		// query: the fields of earlier versions still say something.
		res, err = c.p.Run.Run(ctx, exe, []string{"--query-gpu=" + base, "--format=csv,noheader,nounits"}, sys.RunOpts{Timeout: 12 * time.Second})
	}
	if err != nil || res.ExitCode != 0 {
		return
	}
	line := strings.TrimSpace(strings.Split(strings.ReplaceAll(res.Stdout, "\r", ""), "\n")[0])
	parts := strings.Split(line, ",")
	get := func(i int) string {
		if i < len(parts) {
			return strings.TrimSpace(parts[i])
		}
		return ""
	}
	c.f.Nvidia.SMI = true
	c.f.Nvidia.Driver = get(0)
	c.f.Nvidia.VRAMMiB, _ = strconv.Atoi(get(1))
	c.f.Nvidia.TempC, _ = strconv.Atoi(get(2))
	c.f.Nvidia.PCIeGenMax, _ = strconv.Atoi(get(3))
	c.f.Nvidia.PCIeWidth, _ = strconv.Atoi(get(4))
	c.f.Nvidia.PCIeWidthMax, _ = strconv.Atoi(get(5))
	c.f.Nvidia.PCIeGenCur, _ = strconv.Atoi(get(6))
	if ps := get(7); strings.HasPrefix(strings.ToUpper(ps), "P") {
		c.f.Nvidia.PState = strings.ToUpper(ps)
	}
	c.f.Nvidia.UtilPct = -1
	if u, err := strconv.Atoi(get(8)); err == nil {
		c.f.Nvidia.UtilPct = u
	}
	c.f.Nvidia.ReBAR = "unknown"
	res, err = c.p.Run.Run(ctx, exe, []string{"-q", "-d", "MEMORY"}, sys.RunOpts{Timeout: 12 * time.Second})
	if err == nil && res.ExitCode == 0 {
		c.f.Nvidia.BAR1MiB = ParseBAR1(res.Stdout)
		switch {
		case c.f.Nvidia.BAR1MiB > 256:
			c.f.Nvidia.ReBAR = "on"
		case c.f.Nvidia.BAR1MiB > 0 && !nvidiaReBARCapable(c.f.GPUs):
			// 256 MiB is also what every GeForce without Resizable BAR
			// reports (GTX 10/16, RTX 20): nothing to turn on.
			c.f.Nvidia.ReBAR = "unsupported"
		case c.f.Nvidia.BAR1MiB > 0:
			c.f.Nvidia.ReBAR = "off"
		}
	}
}

func (c *collector) ctxOrBG() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

// maxNvidiaApps caps the game profiles read from the NVIDIA driver, to
// bound the time of the collection (criterio do Axion).
const maxNvidiaApps = 24

// NvidiaApps lists the executables whose NVIDIA driver profile is read. The
// collection runs in a child process, so the list comes from the games found
// here (not from the app); cs2.exe is always first.
var NvidiaApps = func(p *sys.Platform) []string {
	out, seen := []string{"cs2.exe"}, map[string]bool{"cs2.exe": true}
	if p.IsMock {
		return out // the simulator has no real game tree
	}
	for _, g := range games.NewScanner(p.Reg).Scan() { // competitive games first (Scan sorts)
		e := strings.ToLower(g.ExeName)
		if e == "" || seen[e] || len(out) >= maxNvidiaApps {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out
}

// nvdrs reads the NVIDIA profile database (Base Profile, the current global
// profile when it is another one, the games) and the VRR state of each
// monitor, in one DRS session. A PC without NVAPI (no NVIDIA driver, the
// generic Windows driver) is not an error: the fields stay empty with the
// reason in DRS.Error.
func (c *collector) nvdrs() {
	if !c.f.HasGPUVendor("NVIDIA") {
		return
	}
	d := &c.f.Nvidia.DRS
	d.Error = "a leitura foi pulada porque encerrou uma análise anterior neste PC"
	c.part(PartNVDRS, func() {
		d.Error = ""
		refs := []string{sys.NVDRSBase, sys.NVDRSGlobal}
		apps := NvidiaApps(c.p)
		for _, exe := range apps {
			refs = append(refs, sys.NVDRSAppRef(exe))
		}
		profs, err := c.p.Native.NVDRSProfiles(refs, sys.NVDRSReadIDs, true)
		if err != nil || len(profs) < 2 || !profs[0].Found {
			if err == nil {
				err = fmt.Errorf("o driver não devolveu o Base Profile")
			}
			d.Error = nvdrsErrText(err)
			if !errors.Is(err, sys.ErrNoNVAPI) {
				c.errf("perfil do driver NVIDIA: %v", err)
			}
			return
		}
		d.Read = true
		base := profs[0]
		d.Profiles = append(d.Profiles, base)
		if g := profs[1]; g.Found && !strings.EqualFold(g.Name, base.Name) {
			d.Profiles = append(d.Profiles, g)
		} else {
			d.GlobalIsBase = true
		}
		// Found=false stays listed: "the driver has no profile for this game".
		d.Profiles = append(d.Profiles, profs[2:]...)
		for _, disp := range c.f.Displays { // "Monitores" runs before this step
			v, err := c.p.Native.NVVRR(disp.Device)
			if err != nil {
				// Unknown, not an error: driver before R525, or a monitor on
				// the integrated GPU (notebook).
				v = sys.NVVRRInfo{Device: disp.Device}
			}
			d.VRR = append(d.VRR, v)
		}
	})
}

func nvdrsErrText(err error) string {
	if errors.Is(err, sys.ErrNoNVAPI) {
		return "o driver NVIDIA não oferece o NVAPI (driver ausente, genérico do Windows ou antigo)"
	}
	return err.Error()
}

// NvidiaSMIPath finds nvidia-smi.exe in the folders the driver installs it
// to. The folders come from the platform (Windows API), not from
// environment variables, which a program without administrator rights can
// change for the user and the elevated Axion would inherit.
var NvidiaSMIPath = func(paths sys.Paths) string {
	var cands []string
	if paths.WindowsDir != "" {
		cands = append(cands, filepath.Join(paths.WindowsDir, "System32", "nvidia-smi.exe"))
	}
	if paths.ProgramFiles != "" {
		cands = append(cands, filepath.Join(paths.ProgramFiles, "NVIDIA Corporation", "NVSMI", "nvidia-smi.exe"))
	}
	for _, p := range cands {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

var reBAR1 = regexp.MustCompile(`(?s)BAR1 Memory Usage\s*\n\s*Total\s*:\s*(\d+)\s*MiB`)

// ParseBAR1 extracts the BAR1 total (MiB) from `nvidia-smi -q -d MEMORY`.
func ParseBAR1(out string) int {
	m := reBAR1.FindStringSubmatch(strings.ReplaceAll(out, "\r", ""))
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func (c *collector) boot() {
	// "Last BIOS time" of Task Manager (milliseconds), filled by Windows from
	// the firmware performance record when the firmware provides it.
	c.f.Boot.FwPOSTms = -1
	if v := c.regInt(`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Power`, "FwPOSTTime", -1); v > 0 && v < 600000 {
		c.f.Boot.FwPOSTms = int(v)
	}
	m, err := bcd.Read(c.ctxOrBG(), c.p)
	if err != nil {
		c.errf("bcdedit: %v", err)
		return
	}
	c.f.Boot.Readable = true
	c.f.Boot.UsePlatformClock = m["useplatformclock"]
	c.f.Boot.DisableDynamicTick = m["disabledynamictick"]
	c.f.Boot.UsePlatformTick = m["useplatformtick"]
	c.f.Boot.HypervisorLaunch = m["hypervisorlaunchtype"]
	c.f.Boot.TestSigning = m["testsigning"]
	c.f.Boot.NoIntegrityChecks = m["nointegritychecks"]
	c.f.Boot.NX = m["nx"]
	c.f.Boot.NumProc = m["numproc"]
	c.f.Boot.TruncateMemory = m["truncatememory"]
	c.f.Boot.RemoveMemory = m["removememory"]
	if t, err := c.p.Native.TimerResolution(); err == nil {
		c.f.Timer = t
	}
}

// Process categories for the diagnostic.
var procCategories = map[string][2]string{
	"discord.exe":               {"overlay", "Discord (overlay e aceleração de hardware usam GPU)"},
	"gameoverlayui.exe":         {"overlay", "Overlay da Steam"},
	"nvidia overlay.exe":        {"overlay", "Overlay da NVIDIA (GeForce Experience/NVIDIA App)"},
	"rtss.exe":                  {"overlay", "RivaTuner Statistics Server"},
	"msiafterburner.exe":        {"overlay", "MSI Afterburner"},
	"gamebar.exe":               {"overlay", "Xbox Game Bar"},
	"icue.exe":                  {"rgb", "Corsair iCUE"},
	"armourycrate.service.exe":  {"rgb", "ASUS Armoury Crate"},
	"lightingservice.exe":       {"rgb", "ASUS Aura (LightingService)"},
	"signalrgb.exe":             {"rgb", "SignalRGB"},
	"razer synapse service.exe": {"rgb", "Razer Synapse"},
	"lghub.exe":                 {"rgb", "Logitech G HUB"},
	"lghub_agent.exe":           {"rgb", "Logitech G HUB"},
	"gcc.exe":                   {"rgb", "Gigabyte Control Center"},
	"rgbfusion.exe":             {"rgb", "Gigabyte RGB Fusion"},
	"chrome.exe":                {"browser", "Google Chrome"},
	"msedge.exe":                {"browser", "Microsoft Edge"},
	"firefox.exe":               {"browser", "Mozilla Firefox"},
	"brave.exe":                 {"browser", "Brave"},
	"opera.exe":                 {"browser", "Opera"},
	"obs64.exe":                 {"capture", "OBS Studio"},
	"onedrive.exe":              {"cloud", "OneDrive (sincronização)"},
	"dropbox.exe":               {"cloud", "Dropbox"},
	"googledrivefs.exe":         {"cloud", "Google Drive"},
	"epicgameslauncher.exe":     {"launcher", "Epic Games Launcher"},
	"battle.net.exe":            {"launcher", "Battle.net"},
	"eadesktop.exe":             {"launcher", "EA app"},
	"upc.exe":                   {"launcher", "Ubisoft Connect"},
	"steamwebhelper.exe":        {"launcher", "Steam (navegador interno)"},
	"teams.exe":                 {"other", "Microsoft Teams"},
	"ms-teams.exe":              {"other", "Microsoft Teams"},
	"spotify.exe":               {"other", "Spotify"},
}

// ProcCategory returns the category and label of a known background program.
func ProcCategory(exe string) (cat, label string, ok bool) {
	v, ok := procCategories[strings.ToLower(exe)]
	return v[0], v[1], ok
}

func (c *collector) processes() {
	list, err := ProcSummaries(c.p)
	if err != nil {
		c.errf("processos: %v", err)
		return
	}
	c.f.Processes = list
}

// ProcSummaries groups the running known background programs (RGB,
// browsers, overlays...), largest first.
func ProcSummaries(p *sys.Platform) ([]facts.ProcSummary, error) {
	list, err := p.Native.Processes()
	if err != nil {
		return nil, err
	}
	agg := map[string]*facts.ProcSummary{}
	for _, p := range list {
		cat, ok := procCategories[strings.ToLower(p.Name)]
		if !ok {
			continue
		}
		s := agg[cat[1]]
		if s == nil {
			s = &facts.ProcSummary{Name: cat[1], Category: cat[0]}
			agg[cat[1]] = s
		}
		s.Count++
		s.WorkingSet += p.WorkingSet
		s.PrivateMem += p.PrivateMem
	}
	out := make([]facts.ProcSummary, 0, len(agg))
	for _, s := range agg {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mem() > out[j].Mem() })
	return out, nil
}

func (c *collector) pnp() {
	// Win32_PnPEntity also lists devices that are not connected now (every
	// USB stick or monitor ever attached), with problem code 45: only
	// present devices with a real problem count. Present exists on Windows
	// 10 and later; older builds fall back to the query without it.
	const where = " FROM Win32_PnPEntity WHERE ConfigManagerErrorCode<>0 AND ConfigManagerErrorCode<>22 AND ConfigManagerErrorCode<>45"
	rows, err := c.p.WMI.Query(`root\cimv2`, "SELECT Name, ConfigManagerErrorCode, PNPClass, Present"+where)
	if err != nil {
		rows = c.query(`root\cimv2`, "SELECT Name, ConfigManagerErrorCode, PNPClass"+where)
	}
	for _, r := range rows {
		code := int(r.Int("ConfigManagerErrorCode"))
		if code == 0 || code == 22 || code == 45 { // fine / disabled by the user / not connected
			continue
		}
		if v, ok := r["Present"]; ok && v != nil && !r.Bool("Present") {
			continue
		}
		c.f.PnPProblems = append(c.f.PnPProblems, facts.PnPProblem{Name: r.Str("Name"), Code: code, Class: r.Str("PNPClass")})
	}
}

// pciDevices lists the PCI devices as Device Manager names them: the WHEA
// records name a device only by its IDs, and the first notebook report
// showed the class code logged there in a format that does not match the
// PCI specification (the GPU's own HDMI audio came out as a "network card").
func (c *collector) pciDevices() {
	// WQL escapes the backslash: the query text carries two.
	const wql = `SELECT Name, PNPDeviceID, PNPClass, Present FROM Win32_PnPEntity WHERE PNPDeviceID LIKE 'PCI\\%'`
	rows, err := c.p.WMI.Query(`root\cimv2`, wql)
	if err != nil {
		rows = c.query(`root\cimv2`, `SELECT Name, PNPDeviceID, PNPClass FROM Win32_PnPEntity WHERE PNPDeviceID LIKE 'PCI\\%'`)
	}
	for _, r := range rows {
		if v, ok := r["Present"]; ok && v != nil && !r.Bool("Present") {
			continue
		}
		id := strings.TrimSpace(r.Str("PNPDeviceID"))
		name := strings.TrimSpace(r.Str("Name"))
		if id == "" || name == "" {
			continue
		}
		c.f.PCIDevices = append(c.f.PCIDevices, facts.PCIDevice{Name: name, PNPID: id, Class: strings.TrimSpace(r.Str("PNPClass"))})
	}
}

func (c *collector) temp() {
	var total uint64
	deadline := time.Now().Add(3 * time.Second)
	paths := c.p.Native.Paths()
	for _, dir := range []string{paths.UserTemp, paths.WindowsTemp} {
		if dir == "" {
			continue
		}
		_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if time.Now().After(deadline) {
				return filepath.SkipAll
			}
			if !d.IsDir() {
				if info, err := d.Info(); err == nil {
					total += uint64(info.Size())
				}
			}
			return nil
		})
	}
	c.f.TempBytes = total
	c.f.Startup = StartupSummary(c.p)
}

// StartupSummary counts the startup items: Run keys, Startup folders and
// logon tasks, with the StartupApproved state (same view as the
// Inicialização page).
func StartupSummary(p *sys.Platform) facts.StartupSummary {
	var s facts.StartupSummary
	for _, it := range startup.List(p, sys.TrustedEnv(p.Native.Paths())) {
		if it.Enabled {
			s.Enabled++
		} else {
			s.Disabled++
		}
	}
	return s
}

var reRTXReBAR = regexp.MustCompile(`(?i)RTX\s*(A\d{3,4}|[345]0\d{2}|PRO)`)

// nvidiaReBARCapable reports an NVIDIA card that supports Resizable BAR
// (GeForce RTX 30 series and newer, RTX professional cards). NVIDIA:
// "Resizable BAR ... supported on GeForce RTX 30 Series" and later.
func nvidiaReBARCapable(gpus []facts.GPUInfo) bool {
	for _, g := range gpus {
		if g.Vendor == "NVIDIA" && reRTXReBAR.MatchString(g.Name) {
			return true
		}
	}
	return false
}

// driverText cleans a display string written by a driver INF. Indirect
// strings ("@netwtw10.inf,%Roam%;Roaming Aggressiveness") keep the fallback
// text after the semicolon; without one they cannot be shown.
func driverText(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "@") {
		i := strings.LastIndex(s, ";")
		if i < 0 {
			return ""
		}
		s = strings.TrimSpace(s[i+1:])
	}
	if r := []rune(s); len(r) > 80 {
		s = string(r[:80])
	}
	return s
}

// hypervisorFeatures are the optional features that start the Windows
// hypervisor (Microsoft Learn: Hyper-V, Virtual Machine Platform for WSL2,
// Windows Hypervisor Platform for third-party emulators, Windows Sandbox).
var hypervisorFeatures = []struct{ name, label string }{
	{"Microsoft-Hyper-V-Hypervisor", "Hyper-V"},
	{"VirtualMachinePlatform", "Plataforma de Máquina Virtual (WSL2, Android)"},
	{"HypervisorPlatform", "Plataforma do Hipervisor do Windows (emuladores)"},
	{"Containers-DisposableClientVM", "Área Restrita do Windows (Sandbox)"},
}

// ints converts a Win32_DeviceGuard list; 0 means "none" there and is dropped.
func ints(v []int64) []int {
	var out []int
	for _, x := range v {
		if x != 0 {
			out = append(out, int(x))
		}
	}
	return out
}

func containsStr(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

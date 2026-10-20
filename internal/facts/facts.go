// Package facts holds the machine profile produced by the diagnostic scan.
//
// Tweak applicability, recommendations and diagnostic rules are all pure
// functions of Facts, which keeps them deterministic and unit-testable with
// fixtures that describe very different PCs (desktop X3D, Intel hybrid laptop,
// low-end HDD machine...).
package facts

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"axion/internal/sys"
)

// OSInfo describes Windows.
type OSInfo struct {
	Caption        string    `json:"caption"`
	Edition        string    `json:"edition"`
	DisplayVersion string    `json:"displayVersion"` // 23H2, 24H2...
	Build          int       `json:"build"`
	UBR            int       `json:"ubr"`
	IsWin11        bool      `json:"isWin11"`
	LastBoot       time.Time `json:"lastBoot"`
	UptimeHours    float64   `json:"uptimeHours"`
	InstallDate    time.Time `json:"installDate"`
	PendingReboot  bool      `json:"pendingReboot"`
	SystemDrive    string    `json:"systemDrive"`
	Locale         string    `json:"locale"`
	Firmware       string    `json:"firmware"` // UEFI, BIOS or "" (unknown)
	// DOMode is Delivery Optimization's download mode in effect: 0 HTTP
	// only, 1 LAN (the default), 2 group, 3 Internet peers, 99/100 bypass;
	// DOSource says where it came from (política, configurações, padrão).
	DOMode   int    `json:"doMode"`
	DOSource string `json:"doSource,omitempty"`
	// Active hours of Windows Update (0.7): the window in which Windows does
	// not restart for updates, from HKLM\SOFTWARE\Microsoft\WindowsUpdate\
	// UX\Settings (ActiveHoursStart/End, hours 0-23; -1 = not set, which
	// Windows reads as 8 to 17). ActiveHoursAuto is SmartActiveHoursState as
	// stored (1 = Windows adjusts the window by use; -1 absent).
	ActiveHoursStart int `json:"activeHoursStart"`
	ActiveHoursEnd   int `json:"activeHoursEnd"`
	ActiveHoursAuto  int `json:"activeHoursAuto"`
	// ActiveHoursPolicy: a group policy (HKLM\SOFTWARE\Policies\Microsoft\
	// Windows\WindowsUpdate SetActiveHours = 1) fixes the window, with its
	// own start and end; the Settings values are then ignored.
	ActiveHoursPolicy      bool `json:"activeHoursPolicy,omitempty"`
	ActiveHoursPolicyStart int  `json:"activeHoursPolicyStart,omitempty"`
	ActiveHoursPolicyEnd   int  `json:"activeHoursPolicyEnd,omitempty"`
	// User is the account signed in at the console, whose per-user settings
	// and folders Axion reads and changes; ElevatedAs is the account Axion
	// runs as when UAC was answered with another administrator account.
	User       string `json:"user,omitempty"`
	ElevatedAs string `json:"elevatedAs,omitempty"`
}

// CPUInfo describes the processor.
type CPUInfo struct {
	Name          string `json:"name"`
	Vendor        string `json:"vendor"` // AMD, Intel, Other
	Cores         int    `json:"cores"`
	Threads       int    `json:"threads"`
	MaxMHz        int    `json:"maxMHz"`
	IsX3D         bool   `json:"isX3D"`
	IsDualCCDX3D  bool   `json:"isDualCCDX3D"`
	IsIntelHybrid bool   `json:"isIntelHybrid"`
	SMTEnabled    bool   `json:"smtEnabled"`
	IntelGen      int    `json:"intelGen"`  // 0 if unknown/not Intel Core
	Microcode     uint32 `json:"microcode"` // running microcode revision (0 unknown)
	// MicrocodeRaw is the registry data the revision was read from (hex), so
	// a report shows what Windows really stored when the reading looks wrong.
	MicrocodeRaw string `json:"microcodeRaw,omitempty"`
	// Family, Model and Stepping come from the CPUID signature Windows keeps
	// ("Intel64 Family 6 Model 183 Stepping 1"); 0 when unknown.
	Family   int `json:"family,omitempty"`
	Model    int `json:"model,omitempty"`
	Stepping int `json:"stepping,omitempty"`
}

// Signature is the CPUID signature in the notation of Intel's microcode
// release notes ("06-B7-01"), or "" when unknown.
func (c CPUInfo) Signature() string {
	if c.Family == 0 && c.Model == 0 {
		return ""
	}
	return fmt.Sprintf("%02X-%02X-%02X", c.Family, c.Model, c.Stepping)
}

var reRaptorDesktop = regexp.MustCompile(`(?i)\bi[579]-1[34]\d{3}(KS|KF|K|F|T)?(?:\s|$)`)

// IsRaptorLakeDesktop reports a 13th/14th gen Core i5/i7/i9 desktop processor
// (affected by the Vmin shift instability fixed by microcode 0x12B+).
// Mobile parts (H, HX, P, U) do not match: Intel says they are not affected.
func (c CPUInfo) IsRaptorLakeDesktop() bool {
	return c.Vendor == "Intel" && reRaptorDesktop.MatchString(c.Name+" ")
}

// raptorLakeModel is the CPUID model of the Raptor Lake-S B0 die (06-B7-01,
// the "RPL-S B0" line of Intel's microcode release notes), the die affected
// by the Vmin shift instability.
const raptorLakeModel = 0xB7

// RaptorDie reports whether the processor runs on the Raptor Lake die, the
// one affected by the Vmin shift instability. With the CPUID signature known
// the model decides (06-B7); otherwise the microcode numbering does: the
// Raptor Lake die uses 0x1xx (0x119, 0x129, 0x12B, 0x12F...), while the
// 13th/14th generation models built on the other die (i5-13400F/14400F C0,
// i3) are numbered 0x2x-0x3x and are not affected. Unknown (0) is not
// assumed either.
func (c CPUInfo) RaptorDie() bool {
	if !c.IsRaptorLakeDesktop() {
		return false
	}
	if c.Family == 6 && c.Model > 0 {
		return c.Model == raptorLakeModel && c.Microcode > 0
	}
	return c.Microcode >= 0x100
}

var (
	reAM5Desktop = regexp.MustCompile(`(?i)Ryzen\s+\d\s+[79]\d{3}`)
	reAMDMobile  = regexp.MustCompile(`(?i)[79]\d{3}(H|U|HX|HS)\b`)
)

// IsAM5 reports a Ryzen 7000/9000 desktop processor (socket AM5).
func (c CPUInfo) IsAM5() bool {
	return c.Vendor == "AMD" && reAM5Desktop.MatchString(c.Name) && !reAMDMobile.MatchString(c.Name)
}

// GPUInfo describes a display adapter.
type GPUInfo struct {
	Name          string    `json:"name"`
	Vendor        string    `json:"vendor"` // NVIDIA, AMD, Intel, Other
	DriverVersion string    `json:"driverVersion"`
	VendorVersion string    `json:"vendorVersion"` // e.g. NVIDIA 566.36
	DriverDate    time.Time `json:"driverDate"`
	VRAMMiB       int       `json:"vramMiB"`
	PNPDeviceID   string    `json:"pnpDeviceId"`
	Integrated    bool      `json:"integrated"`
	Status        string    `json:"status"`
	// InfFile is the driver package (display.inf = Windows' own "Microsoft
	// Basic Display Adapter", i.e. no vendor driver installed).
	InfFile     string `json:"infFile,omitempty"`
	BasicDriver bool   `json:"basicDriver,omitempty"`
}

// Virtual reports adapters without hardware (remote/virtual displays such
// as Parsec, Citrix, Hyper-V, Meta Quest Link): ROOT\ and SWD\ devices.
func (g GPUInfo) Virtual() bool {
	p := strings.ToUpper(g.PNPDeviceID)
	return strings.HasPrefix(p, `ROOT\`) || strings.HasPrefix(p, `SWD\`)
}

// MemModule is one DIMM.
type MemModule struct {
	CapacityBytes uint64 `json:"capacityBytes"`
	SpeedMTs      int    `json:"speedMTs"`      // Win32_PhysicalMemory.Speed
	ConfiguredMTs int    `json:"configuredMTs"` // ConfiguredClockSpeed
	Manufacturer  string `json:"manufacturer"`
	PartNumber    string `json:"partNumber"`
	Locator       string `json:"locator"`
	Bank          string `json:"bank"`
	SMBIOSType    int    `json:"smbiosType"` // 26 DDR4, 34 DDR5
}

// RAMInfo summarises memory.
type RAMInfo struct {
	TotalBytes    uint64      `json:"totalBytes"`
	Modules       []MemModule `json:"modules"`
	Type          string      `json:"type"` // DDR4, DDR5, ...
	ConfiguredMTs int         `json:"configuredMTs"`
	RatedMTs      int         `json:"ratedMTs"`
}

// BoardInfo describes motherboard/BIOS.
type BoardInfo struct {
	Manufacturer string    `json:"manufacturer"`
	Product      string    `json:"product"`
	BIOSVendor   string    `json:"biosVendor"`
	BIOSVersion  string    `json:"biosVersion"`
	BIOSDate     time.Time `json:"biosDate"`
	SystemModel  string    `json:"systemModel"`
	SystemMaker  string    `json:"systemMaker"`
	// PCSystemType (Win32_ComputerSystem): 1 desktop, 2 mobile, 3
	// workstation... 0 unknown. ChassisTypes: SMBIOS enclosure types.
	PCSystemType int     `json:"pcSystemType,omitempty"`
	ChassisTypes []int64 `json:"chassisTypes,omitempty"`
}

// portableChassis are SMBIOS enclosure types of portable machines
// (portable, laptop, notebook, handheld, sub-notebook, tablet,
// convertible, detachable...).
var portableChassis = map[int64]bool{8: true, 9: true, 10: true, 11: true, 12: true, 14: true, 18: true, 21: true, 30: true, 31: true, 32: true}

// FormFactor classifies the machine from its firmware tables: "laptop",
// "desktop", or "" when they do not say.
func (b BoardInfo) FormFactor() string {
	switch b.PCSystemType {
	case 2:
		return "laptop"
	case 1, 3, 4, 5, 6, 7:
		return "desktop"
	}
	for _, t := range b.ChassisTypes {
		if portableChassis[t] {
			return "laptop"
		}
	}
	for _, t := range b.ChassisTypes {
		switch t {
		case 3, 4, 5, 6, 7, 13, 15, 16, 17, 23, 24, 35, 36:
			return "desktop" // desktop, tower, all-in-one, mini PC...
		}
	}
	return ""
}

// Disk is a physical disk.
type Disk struct {
	Model        string `json:"model"`
	MediaType    string `json:"mediaType"` // SSD, HDD, Unspecified
	BusType      string `json:"busType"`   // NVMe, SATA, USB...
	SizeBytes    uint64 `json:"sizeBytes"`
	Health       string `json:"health"`  // Healthy, Warning, Unhealthy
	WearPct      int    `json:"wearPct"` // -1 unknown
	TempC        int    `json:"tempC"`   // -1 unknown
	PowerOnHours int    `json:"powerOnHours"`
	IsSystem     bool   `json:"isSystem"`
	Number       int    `json:"number"`
	// NVMe is the drive's own SMART / Health log (NVMe disks only).
	NVMe *sys.NVMeHealth `json:"nvme,omitempty"`
	// HealthSource tells where wear/temperature came from: "nvme" (health
	// log), "wmi" (Windows reliability counters) or "" (not available).
	HealthSource string `json:"healthSource,omitempty"`
}

// Volume is a mounted drive letter.
type Volume struct {
	Letter    string `json:"letter"`
	Label     string `json:"label"`
	FS        string `json:"fs"`
	SizeBytes uint64 `json:"sizeBytes"`
	FreeBytes uint64 `json:"freeBytes"`
	Disk      int    `json:"disk"` // physical disk number, -1 unknown
	SSD       bool   `json:"ssd"`  // the disk is an SSD (MediaType SSD or NVMe)
}

// StorageController is a disk controller as Windows lists it
// (Win32_SCSIController: NVMe, RAID and SATA controllers with a Storport
// miniport). Driver is the service name, e.g. "stornvme".
type StorageController struct {
	Name   string `json:"name"`
	Driver string `json:"driver"`
	PNP    string `json:"pnp,omitempty"`
}

// Trim is TRIM (delete notifications) and the Windows drive optimizer.
type Trim struct {
	Readable bool   `json:"readable"`
	NTFS     int    `json:"ntfs"`   // DisableDeleteNotify: 0 = on (default), 1 = off, -1 unknown
	ReFS     int    `json:"refs"`   // -1 = not set or unknown
	Source   string `json:"source"` // fsutil | registro
	// Task is "Optimize Drives" (\Microsoft\Windows\Defrag\ScheduledDefrag).
	TaskRead   bool      `json:"taskRead"`
	TaskExists bool      `json:"taskExists"`
	TaskOn     bool      `json:"taskOn"`
	TaskLast   time.Time `json:"taskLast"`
	TaskResult int       `json:"taskResult"`
	// LastRetrim: newest successful retrim per drive letter ("C:") found in
	// the Application log (event 258 of the drive optimizer).
	LastRetrim map[string]time.Time `json:"lastRetrim,omitempty"`
	// OptimizerEvents counts the optimizer's completion events found (any
	// operation); LastEvent is the newest one's data, for diagnosis.
	OptimizerEvents int            `json:"optimizerEvents"`
	LastEvent       string         `json:"lastEvent,omitempty"`
	Operations      map[string]int `json:"operations,omitempty"` // operation name (as Windows writes it) -> events
}

// NicKeyword describes one advanced property exposed by the adapter driver
// (HKLM\...\Class\{4d36e972-...}\NNNN\Ndi\Params\<keyword>).
type NicKeyword struct {
	Default string   `json:"default"`
	Values  []string `json:"values"`  // enum values accepted by the driver
	Current string   `json:"current"` // configured value, or Default when absent
	Set     bool     `json:"set"`     // value present under the instance key
	// Name is the property name the driver shows (ParamDesc, e.g. "Roaming
	// Aggressiveness") and Labels the text of each enum value ("0" →
	// "1. Lowest"), both as written by the driver (usually in English).
	Name   string            `json:"name,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
}

// Label is the driver's text for a value of the keyword, or the value itself.
func (k NicKeyword) Label(v string) string {
	if l := strings.TrimSpace(k.Labels[v]); l != "" {
		return l
	}
	return v
}

// NonDefault reports a value written under the instance key that differs
// from the driver's default.
func (k NicKeyword) NonDefault() bool {
	return k.Set && k.Default != "" && strings.TrimSpace(k.Current) != strings.TrimSpace(k.Default)
}

// GigabitCapable reports whether a wired adapter can link at 1 Gb/s or
// more: from the driver's *SpeedDuplex choices (5 = 1 Gb/s half duplex and
// up, per the standardized INF keywords) or, when the driver does not expose
// them, from its name. Unknown counts as not capable, so a 10/100 card (Fast
// Ethernet) is never told its cable is bad.
func (a Adapter) GigabitCapable() bool {
	if kw, ok := a.Keywords["*SpeedDuplex"]; ok && len(kw.Values) > 1 {
		for _, v := range kw.Values {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 5 {
				return true
			}
		}
		return false
	}
	d := strings.ToLower(a.Description)
	for _, k := range []string{"gbe", "gigabit", "2.5g", "2.5 g", "5g ", "10g", "i219", "i225", "i226", "e3100", "killer e2", "rtl8125", "rtl8111", "rtl8168"} {
		if strings.Contains(d, k) {
			return true
		}
	}
	return false
}

// Adapter is a network adapter.
type Adapter struct {
	Name            string                `json:"name"`
	Description     string                `json:"description"`
	Kind            string                `json:"kind"` // Ethernet, Wi-Fi, Other
	LinkSpeedMbps   int64                 `json:"linkSpeedMbps"`
	Up              bool                  `json:"up"`
	DriverVersion   string                `json:"driverVersion"`
	InterfaceGUID   string                `json:"interfaceGuid"`
	IfIndex         int                   `json:"ifIndex"`
	HasGateway      bool                  `json:"hasGateway"`
	Gateways        []string              `json:"gateways"`
	DNSServers      []string              `json:"dnsServers"`
	DHCP            bool                  `json:"dhcp"`
	MTU             int                   `json:"mtu"`
	ClassKey        string                `json:"classKey"`    // driver instance key
	PnPDeviceID     string                `json:"pnpDeviceId"` // for restart via WMI
	Keywords        map[string]NicKeyword `json:"keywords"`
	PnPCapabilities int64                 `json:"pnpCapabilities"` // -1 absent
}

// SecurityInfo describes platform security features relevant to anti-cheat.
type SecurityInfo struct {
	VBSRunning        bool   `json:"vbsRunning"`
	VBSConfigured     bool   `json:"vbsConfigured"`
	HVCIRunning       bool   `json:"hvciRunning"`
	SecureBoot        string `json:"secureBoot"` // "on", "off", "unsupported", "unknown"
	TPMRead           bool   `json:"tpmRead"`    // the TPM query answered (false: unknown, not absent)
	TPMPresent        bool   `json:"tpmPresent"`
	TPMReady          bool   `json:"tpmReady"`
	TPMVersion        string `json:"tpmVersion"`
	HypervisorPresent bool   `json:"hypervisorPresent"`
	VirtualizationFW  bool   `json:"virtualizationFirmware"`
	VMPEnabled        bool   `json:"vmpEnabled"`
	IOMMU             string `json:"iommu"`            // "on", "off", "unknown"
	DefenderRealtime  string `json:"defenderRealtime"` // on/off/unknown
	// ServicesRunning and ServicesConfigured are Win32_DeviceGuard's
	// SecurityServicesRunning/Configured (1 Credential Guard, 2 HVCI, 3 System
	// Guard Secure Launch, 4 SMM firmware measurement, 5 kernel-mode stack
	// protection, 6 the same in audit mode, 7 Hypervisor-Enforced Paging
	// Translation); AvailableProps is AvailableSecurityProperties (3 = DMA
	// protection, which needs the IOMMU).
	ServicesRunning    []int `json:"servicesRunning,omitempty"`
	ServicesConfigured []int `json:"servicesConfigured,omitempty"`
	AvailableProps     []int `json:"availableProps,omitempty"`
	// VBSPolicy and HVCIPolicy are the group policy values under
	// HKLM\SOFTWARE\Policies\Microsoft\Windows\DeviceGuard ("" when not set):
	// VBSPolicy "on"/"off"; HVCIPolicy "off", "on" or "lock" (on with UEFI lock).
	VBSPolicy  string `json:"vbsPolicy,omitempty"`
	HVCIPolicy string `json:"hvciPolicy,omitempty"`
	// VBSLocked: VBS or Memory Integrity was configured with UEFI lock
	// (registry "Locked" = 1 or the policy with lock): turning it off in the
	// registry is ignored by Windows.
	VBSLocked bool `json:"vbsLocked,omitempty"`
	// HypervisorFeatures lists the enabled Windows features that start the
	// hypervisor on their own (Hyper-V, Virtual Machine Platform...).
	HypervisorFeatures []string `json:"hypervisorFeatures,omitempty"`
	// Scenarios lists the Device Guard scenarios enabled in the registry
	// (HKLM\SYSTEM\CurrentControlSet\Control\DeviceGuard\Scenarios\<name>,
	// Enabled = 1): what a component asked VBS for, even when
	// Win32_DeviceGuard lists nothing running (the first notebook report).
	Scenarios []string `json:"vbsScenarios,omitempty"`
	// BitLockerOS is the protection status of the Windows drive: "on"
	// (a change to the boot configuration would ask for the recovery key),
	// "off" or "unknown".
	BitLockerOS string `json:"bitLockerOS"`
}

// vbsServiceNames names the SecurityServicesRunning codes.
var vbsServiceNames = map[int]string{
	1: "Credential Guard",
	2: "Integridade de Memória (HVCI)",
	3: "System Guard (Inicialização Segura)",
	4: "medição de firmware SMM",
	5: "proteção de pilha no modo kernel",
	6: "proteção de pilha no modo kernel (auditoria)",
	7: "tradução de páginas imposta pelo hipervisor (HVPT)",
}

// RunningServices names the VBS features Windows reports running.
func (s SecurityInfo) RunningServices() []string {
	var out []string
	for _, v := range s.ServicesRunning {
		if n, ok := vbsServiceNames[v]; ok {
			out = append(out, n)
		}
	}
	return out
}

// vbsScenarioNames names the Device Guard scenario keys in plain words.
// Unknown keys are shown by their registry name.
var vbsScenarioNames = map[string]string{
	"hypervisorenforcedcodeintegrity":     "Integridade de Memória (HVCI)",
	"credentialguard":                     "Credential Guard",
	"systemguard":                         "System Guard (Inicialização Segura)",
	"kernelshadowstacks":                  "proteção de pilha no modo kernel",
	"securebiometrics":                    "Segurança de entrada aprimorada do Windows Hello (biometria)",
	"hypervisorenforcedpagingtranslation": "tradução de páginas imposta pelo hipervisor (HVPT)",
}

// ScenarioNames names the enabled Device Guard scenarios.
func (s SecurityInfo) ScenarioNames() []string {
	var out []string
	for _, k := range s.Scenarios {
		if n, ok := vbsScenarioNames[strings.ToLower(k)]; ok {
			out = append(out, n)
		} else {
			out = append(out, k)
		}
	}
	return out
}

// VBSHolders explains what keeps VBS running: why Windows can ignore a
// registry change that turns it off. It describes the machine only; whether
// a change still waits for a restart is for the caller, who knows when it
// was made.
func (s SecurityInfo) VBSHolders() []string {
	var out []string
	if s.VBSPolicy == "on" || s.HVCIPolicy == "on" || s.HVCIPolicy == "lock" {
		out = append(out, "uma política de grupo (HKLM\\SOFTWARE\\Policies\\Microsoft\\Windows\\DeviceGuard) mantém o VBS ligado e vale mais que o registro")
	}
	if s.VBSLocked {
		out = append(out, "o VBS foi configurado com bloqueio UEFI: o Windows ignora o registro, e desligar exige o procedimento da Microsoft para remover o bloqueio")
	}
	if svc := s.RunningServices(); len(svc) > 0 {
		out = append(out, "recursos em execução que usam o VBS: "+strings.Join(svc, ", "))
	}
	if sc := s.ScenarioNames(); len(sc) > 0 {
		out = append(out, "cenários do Device Guard ligados no registro: "+strings.Join(sc, ", ")+" (cada um pede o VBS ao Windows)")
	}
	if len(out) == 0 {
		out = append(out, "o Windows não lista nenhum recurso nem cenário usando o VBS; alguns componentes pedem o VBS sem aparecer nessas listas, como a Segurança de entrada aprimorada do Windows Hello (ESS), que a Microsoft documenta como dependente do VBS e que o Windows 11 24H2 ou mais novo mostra em Configurações → Contas → Opções de entrada → Configurações adicionais quando há sensor compatível")
	}
	return out
}

// GamingInfo holds game-related Windows state.
type GamingInfo struct {
	GameModeOn       bool `json:"gameModeOn"`
	BackgroundRecord bool `json:"backgroundRecord"` // AppCaptureEnabled
	HistoricalRecord bool `json:"historicalRecord"`
	HAGS             int  `json:"hags"` // 0 unknown/absent, 1 off, 2 on
	// HAGSState is what the graphics kernel reports: "on", "off",
	// "unsupported" or "" (not read). With the registry value absent the
	// driver may already have it on (HAGSDefault).
	HAGSState        string   `json:"hagsState,omitempty"`
	HAGSDefault      bool     `json:"hagsDefault,omitempty"`
	WindowedOpt      bool     `json:"windowedOpt"`
	GameBarInstalled bool     `json:"gameBarInstalled"`
	AntiCheats       []string `json:"antiCheats"` // FACEIT, Vanguard, EasyAntiCheat, BattlEye, Javelin
	X3DServiceState  string   `json:"x3dServiceState"`
	MouseAccelOn     bool     `json:"mouseAccelOn"`
	StickyHotkeyOn   bool     `json:"stickyHotkeyOn"`
}

// PowerInfo describes power state.
type PowerInfo struct {
	ActiveScheme   string            `json:"activeScheme"`
	ActiveName     string            `json:"activeName"`
	Schemes        []sys.PowerScheme `json:"schemes"`
	OverlayAC      string            `json:"overlayAc"` // power mode overlay GUID ("" = default/balanced)
	IsLaptop       bool              `json:"isLaptop"`
	OnBattery      bool              `json:"onBattery"`
	BatteryPercent int               `json:"batteryPercent"`
	HibernateOn    bool              `json:"hibernateOn"`
	FastStartupOn  bool              `json:"fastStartupOn"`
	CoreParkMinAC  int               `json:"coreParkMinAc"` // -1 unknown
	USBSuspendAC   int               `json:"usbSuspendAc"`  // -1 unknown
}

// HasScheme reports whether a power scheme exists on this machine.
func (p PowerInfo) HasScheme(guid string) bool {
	for _, s := range p.Schemes {
		if strings.EqualFold(s.GUID, guid) {
			return true
		}
	}
	return false
}

// NetInfo holds global TCP/IP state.
type NetInfo struct {
	TCPAutoTuning string `json:"tcpAutoTuning"` // normal, disabled, ...
	RSS           string `json:"rss"`           // enabled, disabled
	Readable      bool   `json:"readable"`
}

// NvidiaInfo holds nvidia-smi derived data.
type NvidiaInfo struct {
	Present      bool   `json:"present"`
	SMI          bool   `json:"smi"`
	Driver       string `json:"driver"`
	BAR1MiB      int    `json:"bar1MiB"`
	VRAMMiB      int    `json:"vramMiB"`
	TempC        int    `json:"tempC"`
	ReBAR        string `json:"rebar"` // on, off, unsupported, unknown
	PCIeGenCur   int    `json:"pcieGenCur"`
	PCIeGenMax   int    `json:"pcieGenMax"`
	PCIeWidth    int    `json:"pcieWidth"`
	PCIeWidthMax int    `json:"pcieWidthMax"`
	// PState and UtilPct describe the GPU at the moment of the reading
	// ("P8", 3 %): NVIDIA says the link width and generation "may be reduced
	// when the GPU is not in use". UtilPct is -1 when unknown.
	PState  string `json:"pstate,omitempty"`
	UtilPct int    `json:"utilPct"`
	// DRS is what the NVIDIA driver's profile database says (NVAPI DRS), read
	// without changing anything. Empty when the GPU is not NVIDIA.
	DRS NvDRS `json:"drs"`
}

// LinkAtFullSpeed reports whether the PCIe link was at its maximum
// generation when read, i.e. not in a power-saving state, so the width read
// is the one the link trained to.
func (n NvidiaInfo) LinkAtFullSpeed() bool {
	return n.PCIeGenCur > 0 && n.PCIeGenMax > 0 && n.PCIeGenCur >= n.PCIeGenMax
}

// BootInfo holds BCD flags of the current entry.
type BootInfo struct {
	UsePlatformClock   string `json:"usePlatformClock"`   // "" absent
	DisableDynamicTick string `json:"disableDynamicTick"` // "" absent
	UsePlatformTick    string `json:"usePlatformTick"`
	HypervisorLaunch   string `json:"hypervisorLaunch"`
	TestSigning        string `json:"testSigning"`       // "" absent
	NoIntegrityChecks  string `json:"noIntegrityChecks"` // "" absent
	NX                 string `json:"nx"`                // OptIn (default), OptOut, AlwaysOn, AlwaysOff
	// Limits set by msconfig > Opções avançadas (or bcdedit): "" when absent.
	NumProc        string `json:"numProc"`
	TruncateMemory string `json:"truncateMemory"`
	RemoveMemory   string `json:"removeMemory"`
	Readable       bool   `json:"readable"`
	// FwPOSTms is how long the firmware took in the last boot (POST, memory
	// training included), the "Last BIOS time" of Task Manager; -1 unknown.
	FwPOSTms int `json:"fwPostMs"`
}

// On reports whether a BCD boolean element is present and true.
func (b BootInfo) On(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "yes", "sim", "sí", "si", "oui", "ja", "true", "on", "1":
		return true
	}
	return false
}

// RegRef points to a registry value found on the machine.
type RegRef struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Display string `json:"display"`
	Why     string `json:"why"`
}

// DisabledService is a service that should not be disabled.
type DisabledService struct {
	Name    string           `json:"name"`
	Display string           `json:"display"`
	Impact  string           `json:"impact"`
	Group   string           `json:"group"` // system | apps
	Default sys.ServiceStart `json:"default"`
}

// KeyFeatureOverrides holds FeatureManagement overrides; three of them were
// used in 2025-2026 to force the Windows Server native NVMe driver
// (nvmedisk.sys) on Windows 11.
const KeyFeatureOverrides = `HKLM\SYSTEM\CurrentControlSet\Policies\Microsoft\FeatureManagement\Overrides`

// KeySystemProfile is the MMCSS key (SystemResponsiveness,
// NetworkThrottlingIndex, Tasks).
const KeySystemProfile = `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Multimedia\SystemProfile`

// StorageDriverNames maps the storage miniport services Axion looks for to a
// readable name. Only stornvme (and the native nvmedisk) pass the NVMe Get
// Log Page request through IOCTL_STORAGE_QUERY_PROPERTY (Microsoft Learn,
// "Working with NVMe drives"); RAID and vendor drivers usually answer
// ERROR_INVALID_FUNCTION.
var StorageDriverNames = map[string]string{
	"stornvme": "driver NVMe padrão do Windows (stornvme)",
	"nvmedisk": "driver NVMe nativo (nvmedisk)",
	"rcbottom": "AMD RAID (rcbottom)",
	"rcraid":   "AMD RAID (rcraid)",
	"iaStorVD": "Intel RST/VMD (iaStorVD)",
	"iaStorAC": "Intel RST (iaStorAC)",
	"secnvme":  "driver NVMe da Samsung (secnvme)",
}

// NVMeOverrideIDs are the FeatureManagement IDs of the native NVMe hack.
var NVMeOverrideIDs = []string{"735209102", "1853569164", "156965516", "3244671118", "1409234060"}

// Residue holds settings commonly broken by third-party "optimizers" and
// debloat scripts. Each one has a documented negative effect.
type Residue struct {
	Readable         bool              `json:"readable"`
	UACOff           bool              `json:"uacOff"`           // EnableLUA = 0
	Tcp1323Opts      int               `json:"tcp1323Opts"`      // -1 absent (default: window scaling on)
	TaskOffloadOff   bool              `json:"taskOffloadOff"`   // DisableTaskOffload = 1
	TdrLevel         int               `json:"tdrLevel"`         // -1 absent (default 3 = recover)
	LargeSystemCache bool              `json:"largeSystemCache"` // LargeSystemCache = 1
	PageFile         string            `json:"pageFile"`         // auto | system | custom | none | unknown
	PageFileMaxMB    int               `json:"pageFileMaxMB"`    // total of the custom maxima
	PagingFilesRaw   []string          `json:"pagingFilesRaw"`
	SpectreOverride  bool              `json:"spectreOverride"` // FeatureSettingsOverride disables mitigations
	NVMeOverrides    []string          `json:"nvmeOverrides"`   // FeatureManagement overrides that force the native NVMe driver
	StorageDrivers   []string          `json:"storageDrivers"`  // services of the physical storage controllers (Win32_SCSIController), lower case
	UpdateBlocks     []RegRef          `json:"updateBlocks"`
	DisabledServices []DisabledService `json:"disabledServices"`
	HostsEntries     int               `json:"hostsEntries"` // -1 unreadable
	HostsSample      []string          `json:"hostsSample"`
	// PowerThrottlingOff: PowerThrottlingOff = 1 (EcoQoS disabled system-wide).
	PowerThrottlingOff bool `json:"powerThrottlingOff"`
	// PrioritySeparation is Win32PrioritySeparation (-1 absent; default 2).
	PrioritySeparation int `json:"prioritySeparation"`

	// NetworkThrottling is MMCSS NetworkThrottlingIndex (-1 absent; Windows
	// ships 10; 0xFFFFFFFF switches the throttling off). int64 because
	// 0xFFFFFFFF does not fit an int on 32-bit builds.
	NetworkThrottling int64 `json:"networkThrottlingIndex"`

	// MMAgentRead tells that PS_MMAgent.Get (WMI, no PowerShell) answered, so
	// MemoryCompression means something. It stays false when the class is
	// missing or access is denied: an unknown state is never a leftover.
	MMAgentRead bool `json:"mmagentRead"`
	// MemoryCompression is the MemoryCompression component of the memory
	// manager agent (Windows ships it on).
	MemoryCompression bool `json:"memoryCompression"`
}

// MemoryCompressionOff reports memory compression read as turned off (what
// Disable-MMAgent -MemoryCompression leaves, as some optimizers do).
func (r Residue) MemoryCompressionOff() bool {
	return r.Readable && r.MMAgentRead && !r.MemoryCompression
}

// NoForegroundBoost reports a Win32PrioritySeparation (bits AABBCC) under which
// foreground threads get the same processor time as background ones. Two cases,
// both from the Microsoft documentation of the entry:
//   - the two lowest bits (CC) are 00: "equal and fixed", which overrides a
//     variable interval chosen in the middle bits (the "Background services"
//     choice, 0x18, or a value from a script);
//   - the two middle bits (BB) are 10: fixed-length intervals, and the CC ratio
//     "is used only when ... variable-length", so foreground and background get
//     the same interval whatever CC says (for example 0x1A, which some
//     optimizers write).
//
// BB = 00 or 11 is variable on a Windows client (it is fixed only on Server),
// so those values keep the 3:1 or 2:1 ratio of CC.
func (r Residue) NoForegroundBoost() bool {
	if !r.Readable || r.PrioritySeparation < 0 {
		return false
	}
	v := r.PrioritySeparation
	return v&3 == 0 || (v>>2)&3 == 2
}

// NetworkThrottlingOff reports NetworkThrottlingIndex = 0xFFFFFFFF, written by
// optimizers to "free the network"; the only public measurement saw the NDIS
// DPC latency rise with it.
func (r Residue) NetworkThrottlingOff() bool {
	return r.Readable && r.NetworkThrottling == 0xFFFFFFFF
}

// WindowScalingOff reports TCP window scaling disabled by Tcp1323Opts.
func (r Residue) WindowScalingOff() bool {
	return r.Readable && r.Tcp1323Opts >= 0 && r.Tcp1323Opts&1 == 0
}

// TdrOff reports GPU timeout detection disabled (TdrLevel = 0).
func (r Residue) TdrOff() bool { return r.Readable && r.TdrLevel == 0 }

// ProcSummary is a notable running process.
type ProcSummary struct {
	Name       string `json:"name"`
	Count      int    `json:"count"`
	WorkingSet uint64 `json:"workingSet"`
	// PrivateMem is memory used only by these processes (the working sets
	// of several browser processes count shared pages more than once).
	PrivateMem uint64 `json:"privateMem,omitempty"`
	Category   string `json:"category"` // overlay, rgb, updater, launcher, browser, capture
	Note       string `json:"note"`
}

// Mem is the memory to show: private bytes when known.
func (p ProcSummary) Mem() uint64 {
	if p.PrivateMem > 0 {
		return p.PrivateMem
	}
	return p.WorkingSet
}

// PnPProblem is a device with a driver problem.
type PnPProblem struct {
	Name  string `json:"name"`
	Code  int    `json:"code"`
	Class string `json:"class"`
}

// PCIDevice is a PCI device as Device Manager names it, used to name the
// device behind a hardware error (the WHEA record carries only the IDs).
type PCIDevice struct {
	Name  string `json:"name"`
	PNPID string `json:"pnpId"` // PCI\VEN_xxxx&DEV_yyyy&SUBSYS_...\instance
	Class string `json:"class,omitempty"`
}

// PCIDeviceName returns the Device Manager name of the device with the given
// VEN_xxxx&DEV_yyyy, or "".
func (f *Facts) PCIDeviceName(id string) string {
	if f == nil || id == "" {
		return ""
	}
	id = strings.ToUpper(id)
	for _, d := range f.PCIDevices {
		if strings.Contains(strings.ToUpper(d.PNPID), id) {
			return strings.TrimSpace(d.Name)
		}
	}
	return ""
}

// StartupItem is summarised for counts.
type StartupSummary struct {
	Enabled  int `json:"enabled"`
	Disabled int `json:"disabled"`
}

// Facts is the whole machine profile.
type Facts struct {
	Collected   time.Time           `json:"collected"`
	ScanSeconds float64             `json:"scanSeconds"`
	Hostname    string              `json:"hostname"`
	OS          OSInfo              `json:"os"`
	CPU         CPUInfo             `json:"cpu"`
	GPUs        []GPUInfo           `json:"gpus"`
	RAM         RAMInfo             `json:"ram"`
	Board       BoardInfo           `json:"board"`
	Disks       []Disk              `json:"disks"`
	Volumes     []Volume            `json:"volumes"`
	Controllers []StorageController `json:"storageControllers"`
	Trim        Trim                `json:"trim"`
	Displays    []sys.DisplayMode   `json:"displays"`
	Adapters    []Adapter           `json:"adapters"`
	Net         NetInfo             `json:"net"`
	Security    SecurityInfo        `json:"security"`
	Gaming      GamingInfo          `json:"gaming"`
	Power       PowerInfo           `json:"power"`
	Nvidia      NvidiaInfo          `json:"nvidia"`
	Boot        BootInfo            `json:"boot"`
	Timer       sys.TimerResolution `json:"timer"`
	Memory      sys.MemoryLists     `json:"memory"`
	Processes   []ProcSummary       `json:"processes"`
	PnPProblems []PnPProblem        `json:"pnpProblems"`
	PCIDevices  []PCIDevice         `json:"pciDevices,omitempty"`
	Startup     StartupSummary      `json:"startup"`
	TempBytes   uint64              `json:"tempBytes"`
	Residue     Residue             `json:"residue"`
	Peripherals PeriphInfo          `json:"peripherals"`
	Health      HealthInfo          `json:"health"`
	VulnDrivers VulnDriverScan      `json:"vulnDrivers"`
	Thermal     ThermalInfo         `json:"thermal"`
	Errors      []string            `json:"errors"`
	// Unread lists the collection steps and parts that were not read (set
	// aside after a crash, or failed midway): their fields hold zero values
	// that must not be taken as "off" or "absent".
	Unread []string `json:"unread,omitempty"`
}

// ---------------------------------------------------------------------------
// Derived helpers used by rules and tweaks
// ---------------------------------------------------------------------------

// HasSSD tells whether any disk is an SSD.
func (f *Facts) HasSSD() bool {
	for _, d := range f.Disks {
		if d.MediaType == "SSD" || d.BusType == "NVMe" {
			return true
		}
	}
	return false
}

// PrimaryGPU returns the discrete GPU if present, otherwise the first one.
func (f *Facts) PrimaryGPU() *GPUInfo {
	// Virtual adapters (Parsec, VR or remote displays) only count when nothing
	// else exists; then a discrete card beats an integrated one (even on the
	// basic display driver, the case where its driver is most needed), a
	// card with its vendor driver beats one without, and NVIDIA breaks a tie
	// between discrete cards.
	rank := func(g *GPUInfo) int {
		r := 0
		if !g.Virtual() {
			r += 8
		}
		if !g.Integrated {
			r += 4 // a discrete card still on the basic driver is still the main one
		}
		if !g.BasicDriver {
			r += 2
		}
		if g.Vendor == "NVIDIA" && !g.Integrated {
			r++
		}
		return r
	}
	var best *GPUInfo
	for i := range f.GPUs {
		g := &f.GPUs[i]
		if best == nil || rank(g) > rank(best) {
			best = g
		}
	}
	return best
}

// HasGPUVendor reports whether any adapter matches vendor.
func (f *Facts) HasGPUVendor(v string) bool {
	for _, g := range f.GPUs {
		if strings.EqualFold(g.Vendor, v) {
			return true
		}
	}
	return false
}

// HasAntiCheat reports whether an anti-cheat that requires VBS/TPM is installed.
func (f *Facts) HasAntiCheat(names ...string) bool {
	for _, a := range f.Gaming.AntiCheats {
		for _, n := range names {
			if strings.EqualFold(a, n) {
				return true
			}
		}
	}
	return false
}

// RequiresVBS is true when an installed anti-cheat requires (or may require) VBS/HVCI.
// FACEIT requires VBS for IOMMU enforcement; Riot Vanguard may require Memory Integrity.
// When the game settings were not read, the anti-cheats are unknown: the
// answer is then true (fail closed), so VBS is never switched off blindly.
func (f *Facts) RequiresVBS() bool {
	return !f.Read(StepGaming) || f.HasAntiCheat("FACEIT", "Vanguard")
}

// Names of the collection steps and parts that rules depend on (the same
// names the collection reports in its progress and in Unread).
const (
	StepOS       = "Sistema operacional"
	StepSecurity = "Segurança e anti-cheat"
	StepGaming   = "Configurações de jogo"
	StepPower    = "Energia"
	PartX3D      = "Configurações de jogo › serviço do 3D V-Cache"
)

// Read reports whether a collection step or part was read completely.
func (f *Facts) Read(name string) bool {
	for _, u := range f.Unread {
		if u == name {
			return false
		}
	}
	return true
}

// Desktop reports a desktop (no battery) system.
func (f *Facts) Desktop() bool { return !f.Power.IsLaptop }

// SystemDisk returns the disk holding Windows, if known.
func (f *Facts) SystemDisk() *Disk {
	for i := range f.Disks {
		if f.Disks[i].IsSystem {
			return &f.Disks[i]
		}
	}
	return nil
}

// SystemVolume returns the system drive volume.
func (f *Facts) SystemVolume() *Volume {
	sd := strings.TrimSuffix(strings.ToUpper(f.OS.SystemDrive), `\`)
	if sd == "" {
		sd = "C:"
	}
	for i := range f.Volumes {
		if strings.EqualFold(strings.TrimSuffix(f.Volumes[i].Letter, `\`), sd) {
			return &f.Volumes[i]
		}
	}
	return nil
}

// ActiveAdapter returns the adapter with a default gateway, preferring Ethernet.
func (f *Facts) ActiveAdapter() *Adapter {
	var best *Adapter
	for i := range f.Adapters {
		a := &f.Adapters[i]
		if !a.Up || !a.HasGateway {
			continue
		}
		if best == nil || (a.Kind == "Ethernet" && best.Kind != "Ethernet") {
			best = a
		}
	}
	return best
}

// PhysicalAdapters returns the wired and Wi-Fi adapters with a driver key,
// connected or not: a setting left on a cable that is unplugged today still
// applies the day it is plugged in.
func (f *Facts) PhysicalAdapters() []Adapter {
	var out []Adapter
	for _, a := range f.Adapters {
		if a.ClassKey == "" || (a.Kind != "Ethernet" && a.Kind != "Wi-Fi") {
			continue
		}
		out = append(out, a)
	}
	return out
}

// IsLaptop reports a notebook: a battery on a machine the firmware does not
// call a desktop, or a portable chassis.
func (f *Facts) IsLaptop() bool {
	return f.Power.IsLaptop || f.Board.FormFactor() == "laptop"
}

// ---------------------------------------------------------------------------
// CPU classification
// ---------------------------------------------------------------------------

var (
	// Two CCDs with 3D V-Cache on one of them (desktop and HX3D laptops).
	reX3DDual   = regexp.MustCompile(`(?i)\b(7900X3D|7950X3D|9900X3D|9950X3D|7945HX3D|9955HX3D)\b`)
	reX3D       = regexp.MustCompile(`(?i)\b\d{4}H?X3D\b`)
	reIntelGen  = regexp.MustCompile(`(?i)\bi[3579]-(\d{4,5})[A-Z]*\d?\b`) // i5-12400F => 12, i7-4790K => 4, i7-1255U => 12
	reIntelNth  = regexp.MustCompile(`(?i)\b(\d{1,2})(?:st|nd|rd|th) Gen Intel`)
	reCoreUltra = regexp.MustCompile(`(?i)Core\(TM\)?\s*Ultra|Core Ultra`)
	reArcDGPU   = regexp.MustCompile(`(?i)Arc(\(TM\))?\s+(Pro\s+)?[AB]\d{2,3}M?\b`) // A770, B580, A370M (notebook)
	// "Core 5 120U", "Core 7 150U": the Core series 1 without "Ultra" (no
	// "i" in the name), hybrid like the 13th generation they come from.
	reIntelSeries1 = regexp.MustCompile(`(?i)Core(\(TM\))?\s+[357]\s+\d{3}[A-Z]{1,2}\b`)
)

// ClassifyCPU fills vendor/X3D/hybrid flags from the CPU name.
func ClassifyCPU(c *CPUInfo) {
	n := c.Name
	switch {
	case strings.Contains(strings.ToUpper(n), "AMD"):
		c.Vendor = "AMD"
	case strings.Contains(strings.ToUpper(n), "INTEL"):
		c.Vendor = "Intel"
	default:
		if c.Vendor == "" {
			c.Vendor = "Other"
		}
	}
	c.IsX3D = reX3D.MatchString(n)
	c.IsDualCCDX3D = reX3DDual.MatchString(n)
	if m := reIntelGen.FindStringSubmatch(n); m != nil {
		d := m[1]
		switch {
		case len(d) == 5: // 10xxx..14xxx (desktop and H/HX)
			c.IntelGen = int(d[0]-'0')*10 + int(d[1]-'0')
		case len(d) == 4 && d[0] == '1': // 1065G7, 1165G7, 1255U, 1335U: 10th-14th gen mobile
			c.IntelGen = 10 + int(d[1]-'0')
		case len(d) == 4: // 2xxx..9xxx
			c.IntelGen = int(d[0] - '0')
		}
	}
	if c.IntelGen == 0 && reIntelSeries1.MatchString(n) {
		c.IntelGen = 13
	}
	if m := reIntelNth.FindStringSubmatch(n); m != nil { // "12th Gen Intel(R) Core(TM) ..."
		if g, err := strconv.Atoi(m[1]); err == nil && g >= 1 && g <= 20 {
			c.IntelGen = g
		}
	}
	// Alder Lake (12th) and newer desktop/mobile Core i5/i7/i9 are hybrid (except
	// some i3/i5 non-K models without E-cores; threads > cores*2-? heuristic below).
	if c.Vendor == "Intel" && (c.IntelGen >= 12 || reCoreUltra.MatchString(n)) {
		// Hybrid parts have fewer threads than 2*cores (E-cores lack HT).
		// (A P-core-only part with Hyper-Threading off has threads equal
		// to cores: not hybrid.)
		if c.Cores > 0 && c.Threads > c.Cores && c.Threads < 2*c.Cores {
			c.IsIntelHybrid = true
		}
		// Hyper-Threading off makes threads equal cores on hybrid parts too:
		// then the model decides.
		if c.Cores > 0 && c.Threads == c.Cores && intelHybridModel(n) {
			c.IsIntelHybrid = true
		}
		if reCoreUltra.MatchString(n) {
			c.IsIntelHybrid = true
		}
	}
	c.SMTEnabled = c.Threads > c.Cores && c.Cores > 0
}

// ClassifyGPUVendor maps an adapter name to a vendor.
func ClassifyGPUVendor(name, pnp string) (vendor string, integrated bool) {
	u := strings.ToUpper(name + " " + pnp)
	switch {
	case strings.Contains(u, "NVIDIA") || strings.Contains(u, "VEN_10DE") || strings.Contains(u, "GEFORCE"):
		vendor = "NVIDIA"
	case strings.Contains(u, "VEN_1002") || strings.Contains(u, "RADEON") || strings.Contains(u, "AMD "):
		vendor = "AMD"
		// Ryzen APU graphics names: "AMD Radeon(TM) Graphics", "Radeon 780M".
		// "Radeon 8060S" / "8050S" are the integrated graphics of Ryzen AI Max.
		integrated = regexp.MustCompile(`(?i)Radeon\(TM\)\s+Graphics|Radeon Graphics|\b\d{3}M\b|Radeon(\(TM\))?\s+80\d0S\b|Vega \d+ Graphics`).MatchString(name)
	case strings.Contains(u, "VEN_8086") || strings.Contains(u, "INTEL"):
		vendor = "Intel"
		// Discrete Arc cards carry a model (A770, B580, Pro A60);
		// "Arc(TM) Graphics" and "Arc 140V" are integrated.
		integrated = !reArcDGPU.MatchString(name)
	case strings.Contains(u, "MICROSOFT BASIC") || strings.Contains(u, "REMOTE"):
		vendor = "Other"
		integrated = true
	default:
		vendor = "Other"
	}
	return
}

// JEDECMax returns the highest JEDEC-standard speed commonly shipped for a
// memory type (MT/s). Kits configured at or below this are very likely running
// without XMP/EXPO.
func JEDECMax(memType string) int {
	switch memType {
	case "DDR4":
		return 3200
	case "DDR5":
		return 5600
	case "DDR3":
		return 1600
	}
	return 0
}

// MemTypeFromSMBIOS converts SMBIOSMemoryType codes.
func MemTypeFromSMBIOS(t int) string {
	switch t {
	case 24:
		return "DDR3"
	case 26:
		return "DDR4"
	case 34:
		return "DDR5"
	case 30:
		return "LPDDR4"
	case 35:
		return "LPDDR5"
	}
	return ""
}

// ---------------------------------------------------------------------------
// Peripherals
// ---------------------------------------------------------------------------

// PowerNode is a device node that exposes the "Allow the computer to turn
// off this device to save power" setting (WMI MSPower_DeviceEnable).
type PowerNode struct {
	Instance string `json:"instance"` // device instance ID
	WMIName  string `json:"wmiName"`  // WMI InstanceName (instance ID + "_0")
	Enabled  bool   `json:"enabled"`
}

// InputDev is one physical keyboard, mouse or wireless receiver with every
// HID collection it exposes (a receiver or gaming mouse usually registers a
// mouse and a keyboard collection).
type InputDev struct {
	Key          string      `json:"key"`  // USB node instance ID, or the input instance ID
	Name         string      `json:"name"` // best display name
	Kinds        []string    `json:"kinds"`
	Primary      string      `json:"primary"` // mouse | keyboard
	Collections  []string    `json:"collections"`
	Transport    string      `json:"transport"` // usb | bluetooth | bluetooth-le | ps2 | internal | virtual | unknown
	VID          uint16      `json:"vid"`
	PID          uint16      `json:"pid"`
	USBNode      string      `json:"usbNode,omitempty"`
	Interfaces   []string    `json:"interfaces,omitempty"`
	Speed        int         `json:"speed"` // sys.USBSpeed*
	BcdUSB       uint16      `json:"bcdUsb"`
	BInterval    int         `json:"bInterval"` // interrupt IN bInterval of the input interface (0 unknown)
	NominalHz    float64     `json:"nominalHz"` // highest polling rate the port allows (0 unknown)
	Port         int         `json:"port"`
	HubDepth     int         `json:"hubDepth"`
	HubIsRoot    bool        `json:"hubIsRoot"`
	Controller   string      `json:"controller,omitempty"`
	ControllerID string      `json:"controllerId,omitempty"`
	Receiver     bool        `json:"receiver"`
	SelSuspend   int         `json:"selSuspend"`             // -1 absent, 0 off, 1 on (vendor-enabled HID selective suspend)
	SuspendNodes []string    `json:"suspendNodes,omitempty"` // nodes with SelectiveSuspendEnabled on
	PowerNodes   []PowerNode `json:"powerNodes,omitempty"`
	Err          string      `json:"err,omitempty"`
}

// Has reports whether the device exposes a collection of the given kind.
func (d InputDev) Has(kind string) bool {
	for _, k := range d.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// SuspendAllowed reports whether Windows may put the device to sleep to save
// power: selective suspend enabled by the vendor or the Device Manager power
// checkbox ticked on one of the device's own nodes.
func (d InputDev) SuspendAllowed() bool {
	if d.SelSuspend == 1 {
		return true
	}
	for _, n := range d.PowerNodes {
		if n.Enabled {
			return true
		}
	}
	return false
}

// PeriphInfo groups the input-device inventory and the input settings that
// optimizers like to change.
type PeriphInfo struct {
	Readable       bool       `json:"readable"`
	Devices        []InputDev `json:"devices"`
	MouseQueue     int        `json:"mouseQueue"`    // MouseDataQueueSize (-1 absent = 100)
	KeyboardQueue  int        `json:"keyboardQueue"` // KeyboardDataQueueSize (-1 absent = 100)
	ThreadDPC      int        `json:"threadDpc"`     // ThreadDpcEnable (-1 absent = enabled)
	FilterKeysOn   bool       `json:"filterKeysOn"`
	FilterDelayMS  int        `json:"filterDelayMs"`  // DelayBeforeAcceptance
	FilterBounceMS int        `json:"filterBounceMs"` // BounceTime
	Err            string     `json:"err,omitempty"`
}

// QueueTooSmall reports whether an input driver queue was shrunk below the
// Windows default of 100 events.
func (p PeriphInfo) QueueTooSmall() bool {
	return p.Readable && ((p.MouseQueue >= 0 && p.MouseQueue < 100) || (p.KeyboardQueue >= 0 && p.KeyboardQueue < 100))
}

var reIntelCore = regexp.MustCompile(`(?i)\bi([3579])-1([234])(\d{2,3})([a-z]*)`)

// intelHybridModel reports 12th–14th gen Core models with E-cores. The
// P-core-only ones are the i3 desktop parts and the i5 12400/12500/12600
// (non-K) family; laptop parts (H/P/U/HX) are all hybrid.
func intelHybridModel(name string) bool {
	m := reIntelCore.FindStringSubmatch(name)
	if m == nil {
		return false
	}
	tier, gen, suffix := m[1], m[2], strings.ToUpper(m[4])
	model, _ := strconv.Atoi(m[3])
	if strings.ContainsAny(suffix, "HPU") {
		return true // mobile
	}
	switch tier {
	case "7", "9":
		return true
	case "3":
		return false
	}
	// i5 desktop
	if gen == "2" {
		return model >= 600 && strings.Contains(suffix, "K")
	}
	return model >= 400
}

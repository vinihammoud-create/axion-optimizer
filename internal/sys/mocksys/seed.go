package mocksys

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"axion/internal/sys"
)

// Seeded identities used by tests.
const (
	EthernetGUID = "{6A3F0C11-2B1D-4F5E-9C0A-7D1E2F3A4B5C}"
	WifiGUID     = "{B1C2D3E4-F5A6-4B7C-8D9E-0F1A2B3C4D5E}"
	NetClass     = `HKLM\SYSTEM\CurrentControlSet\Control\Class\{4d36e972-e325-11ce-bfc1-08002be10318}`
	DisplayClass = `HKLM\SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`
)

// SeedWindowsDefaults models a gaming desktop close to a typical setup:
// Windows 11 24H2, Ryzen 7 7800X3D, RTX 3060 12 GB, 32 GB DDR5 running at
// JEDEC speed, Gigabyte B650M, NVMe DRAM-less SSD, 240 Hz monitor left at
// 60 Hz, Realtek 2.5 GbE with EEE, FACEIT installed, VBS/HVCI running and a
// leftover "useplatformclock" from an old optimizer script.
func SeedWindowsDefaults(m *Mock) {
	r := m.R
	set := func(path, name string, v sys.RegValue) { _ = r.SetValue(path, name, v) }

	// --- OS
	nt := `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion`
	set(nt, "ProductName", sys.SZ("Windows 10 Pro"))
	set(nt, "DisplayVersion", sys.SZ("24H2"))
	set(nt, "EditionID", sys.SZ("Professional"))
	set(nt, "CurrentBuild", sys.SZ("26100"))
	set(nt, "UBR", sys.DWord(4061))
	set(nt, "InstallDate", sys.DWord(uint32(time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC).Unix())))
	set(`HKLM\SYSTEM\CurrentControlSet\Control`, "SystemStartOptions", sys.SZ(" NOEXECUTE=OPTIN  FVEBOOT=2560000  USEPLATFORMCLOCK"))
	m.BCD["useplatformclock"] = "Yes"
	m.BCD["hypervisorlaunchtype"] = "Auto"
	m.BCD["nx"] = "OptIn"

	// --- Input defaults
	set(`HKCU\Control Panel\Mouse`, "MouseSpeed", sys.SZ("1"))
	set(`HKCU\Control Panel\Mouse`, "MouseThreshold1", sys.SZ("6"))
	set(`HKCU\Control Panel\Mouse`, "MouseThreshold2", sys.SZ("10"))
	set(`HKCU\Control Panel\Mouse`, "MouseSensitivity", sys.SZ("10"))
	set(`HKCU\Control Panel\Accessibility\StickyKeys`, "Flags", sys.SZ("510"))
	set(`HKCU\Control Panel\Accessibility\Keyboard Response`, "Flags", sys.SZ("126"))
	set(`HKCU\Control Panel\Accessibility\ToggleKeys`, "Flags", sys.SZ("62"))
	set(`HKCU\Control Panel\Desktop`, "MenuShowDelay", sys.SZ("400"))

	// --- Gaming
	set(`HKCU\Software\Microsoft\Windows\CurrentVersion\GameDVR`, "AppCaptureEnabled", sys.DWord(1))
	set(`HKCU\Software\Microsoft\Windows\CurrentVersion\GameDVR`, "HistoricalCaptureEnabled", sys.DWord(1))
	set(`HKCU\System\GameConfigStore`, "GameDVR_Enabled", sys.DWord(1))
	set(`HKLM\SYSTEM\CurrentControlSet\Control\GraphicsDrivers`, "HwSchMode", sys.DWord(1))
	set(`HKCU\Software\Microsoft\DirectX\UserGpuPreferences`, "DirectXUserGlobalSettings", sys.SZ("VRROptimizeEnable=0;"))
	_, _ = r.CreateKey(`HKCU\Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages\Microsoft.XboxGamingOverlay_7.325.8081.0_x64__8wekyb3d8bbwe`)
	set(`HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Multimedia\SystemProfile`, "SystemResponsiveness", sys.DWord(20))
	set(`HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Multimedia\SystemProfile`, "NetworkThrottlingIndex", sys.DWord(10))
	set(`HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced`, "HideFileExt", sys.DWord(1))
	set(`HKLM\SYSTEM\CurrentControlSet\Control\SecureBoot\State`, "UEFISecureBootEnabled", sys.DWord(1))
	// Device Guard scenarios: Memory Integrity on (matches the Win32_DeviceGuard
	// row), the others present but off.
	set(`HKLM\SYSTEM\CurrentControlSet\Control\DeviceGuard\Scenarios\HypervisorEnforcedCodeIntegrity`, "Enabled", sys.DWord(1))
	set(`HKLM\SYSTEM\CurrentControlSet\Control\DeviceGuard\Scenarios\CredentialGuard`, "Enabled", sys.DWord(0))
	set(`HKLM\SYSTEM\CurrentControlSet\Control\DeviceGuard\Scenarios\KernelShadowStacks`, "Enabled", sys.DWord(0))
	set(`HKLM\SYSTEM\CurrentControlSet\Control\Power`, "HibernateEnabled", sys.DWord(1))
	set(`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Power`, "HiberbootEnabled", sys.DWord(1))
	set(`HKLM\SYSTEM\CurrentControlSet\Control\PriorityControl`, "Win32PrioritySeparation", sys.DWord(2))

	// --- Residue left by other "optimizers" (detected and fixable)
	set(`HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters`, "DisableTaskOffload", sys.DWord(1))
	set(`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Memory Management`, "PagingFiles", sys.MultiSZ(`C:\pagefile.sys 4096 4096`))
	set(`HKLM\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate\AU`, "NoAutoUpdate", sys.DWord(1))
	set(`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Memory Management`, "FeatureSettingsOverride", sys.DWord(3))
	set(`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Memory Management`, "FeatureSettingsOverrideMask", sys.DWord(3))

	// --- Startup (Run keys)
	set(`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "Discord", sys.SZ(`"C:\Users\Player\AppData\Local\Discord\Update.exe" --processStart Discord.exe`))
	set(`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "Steam", sys.SZ(`"C:\Program Files (x86)\Steam\steam.exe" -silent`))
	set(`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "OneDrive", sys.SZ(`"C:\Program Files\Microsoft OneDrive\OneDrive.exe" /background`))
	set(`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, "SecurityHealth", sys.ExpandSZ(`%windir%\system32\SecurityHealthSystray.exe`))
	set(`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, "iCUE", sys.SZ(`"C:\Program Files\Corsair\CORSAIR iCUE 5 Software\iCUE.exe" --autorun`))
	set(`HKLM\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Run`, "Adobe ARM", sys.SZ(`"C:\Program Files (x86)\Common Files\Adobe\ARM\1.0\AdobeARM.exe"`))
	set(`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "EpicGamesLauncher", sys.SZ(`"C:\Program Files (x86)\Epic Games\Launcher\Portal\Binaries\Win64\EpicGamesLauncher.exe" -silent`))
	// Turned off earlier in Task Manager (StartupApproved flag 03 + FILETIME).
	set(`HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`, "EpicGamesLauncher", sys.Binary([]byte{3, 0, 0, 0, 0x10, 0x6c, 0x3e, 0x9b, 0x2c, 0x1d, 0xdb, 0x01}))

	// --- Display adapter driver key
	set(DisplayClass+`\0000`, "MatchingDeviceId", sys.SZ(`pci\ven_10de&dev_2504`))
	set(DisplayClass+`\0000`, "HardwareInformation.qwMemorySize", sys.QWord(12<<30))
	set(DisplayClass+`\0000`, "DriverDesc", sys.SZ("NVIDIA GeForce RTX 3060"))

	// --- Network adapter driver keys (Realtek 2.5GbE + Intel Wi-Fi)
	eth := NetClass + `\0001`
	set(eth, "NetCfgInstanceId", sys.SZ(EthernetGUID))
	set(eth, "DriverDesc", sys.SZ("Realtek Gaming 2.5GbE Family Controller"))
	set(eth, "DriverVersion", sys.SZ("10.68.815.2024"))
	for _, kw := range []struct{ name, def string }{{"*EEE", "1"}, {"EnableGreenEthernet", "1"}, {"AdvancedEEE", "0"}, {"*InterruptModeration", "1"}, {"*FlowControl", "3"}} {
		set(eth+`\Ndi\Params\`+kw.name, "default", sys.SZ(kw.def))
		set(eth+`\Ndi\Params\`+kw.name+`\enum`, "0", sys.SZ("Desativado"))
		set(eth+`\Ndi\Params\`+kw.name+`\enum`, "1", sys.SZ("Ativado"))
	}
	wifi := NetClass + `\0002`
	set(wifi, "NetCfgInstanceId", sys.SZ(WifiGUID))
	set(wifi, "DriverDesc", sys.SZ("Intel(R) Wi-Fi 6E AX210 160MHz"))
	_, _ = r.CreateKey(`HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\` + EthernetGUID)
	_, _ = r.CreateKey(`HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\` + WifiGUID)
	// As on a real Windows in DHCP: the static lists exist and are empty, so
	// putting a stack back to automatic creates no value.
	for _, g := range []string{EthernetGUID, WifiGUID} {
		set(`HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\`+g, "NameServer", sys.SZ(""))
		set(`HKLM\SYSTEM\CurrentControlSet\Services\Tcpip6\Parameters\Interfaces\`+g, "NameServer", sys.SZ(""))
	}

	m.N.NetAdapters = []sys.NetAdapter{
		{Name: "Ethernet", Description: "Realtek Gaming 2.5GbE Family Controller", GUID: EthernetGUID, IfIndex: 12, IfType: 6, Up: true, SpeedBps: 1_000_000_000, MTU: 1500, MAC: "74-56-3C-11-22-33", Gateways: []string{"192.168.0.1"}, DNS: []string{"192.168.0.1"}, IPv4: []string{"192.168.0.23"}, DHCP: true, Physical: true},
		{Name: "Wi-Fi", Description: "Intel(R) Wi-Fi 6E AX210 160MHz", GUID: WifiGUID, IfIndex: 18, IfType: 71, Up: false, MTU: 1500, Physical: true},
	}

	// --- Services
	s := m.S
	s.Add("FACEITService", "FACEIT AntiCheat Service", sys.StartManual, false)
	s.Add("DiagTrack", "Connected User Experiences and Telemetry", sys.StartAuto, true)
	s.Add("MapsBroker", "Downloaded Maps Manager", sys.StartAutoDelayed, false)
	s.Add("TrkWks", "Distributed Link Tracking Client", sys.StartAuto, true)
	s.Add("Fax", "Fax", sys.StartManual, false)
	s.Add("RetailDemo", "Retail Demo Service", sys.StartManual, false)
	s.Add("SysMain", "SysMain", sys.StartAuto, true)
	s.Add("WSearch", "Windows Search", sys.StartAutoDelayed, true)
	s.Add("Spooler", "Print Spooler", sys.StartAuto, true)
	s.Add("wuauserv", "Windows Update", sys.StartManual, false)
	s.Add("BITS", "Background Intelligent Transfer Service", sys.StartManual, false)
	s.Add("AudioSrv", "Windows Audio", sys.StartAuto, true)
	s.Add("XblAuthManager", "Xbox Live Auth Manager", sys.StartDisabled, false)
	s.Add("XboxNetApiSvc", "Xbox Live Networking Service", sys.StartDisabled, false)
	s.Add("iphlpsvc", "IP Helper", sys.StartAuto, true)

	// --- Power defaults (Balanced AC values)
	m.P.Seed("54533251-82be-4824-96c1-47b60b740d00", "0cc5b647-c1df-4637-891a-dec35c318583", 10, 10)
	m.P.Seed("2a737441-1930-4402-8d77-b2bebba308a3", "48e6b7a6-50f5-4782-a5d4-53bb8f07e226", 1, 1)
	m.P.Seed("501a4d13-42af-4429-9fd1-a8218c268e20", "ee12f906-d277-404b-b6da-e5fa1a576df5", 1, 2)

	// --- Scheduled tasks
	t := m.T
	for _, p := range []string{
		`\Microsoft\Windows\Application Experience\Microsoft Compatibility Appraiser`,
		`\Microsoft\Windows\Application Experience\ProgramDataUpdater`,
		`\Microsoft\Windows\Customer Experience Improvement Program\Consolidator`,
		`\Microsoft\Windows\Customer Experience Improvement Program\UsbCeip`,
		`\Microsoft\Windows\Feedback\Siuf\DmClient`,
	} {
		t.Add(p, true, []int{sys.TriggerDaily})
	}
	t.Add(`\GoogleUpdateTaskMachineUA`, true, []int{sys.TriggerLogon}, `C:\Program Files (x86)\Google\Update\GoogleUpdate.exe /ua`)
	t.Add(sys.DefragTask, true, nil, `%windir%\system32\defrag.exe -c -h -o -$`)
	t.SetLastRun(sys.DefragTask, time.Now().Add(-5*24*time.Hour), 0)
	_ = m.R.SetValue(sys.KeyFileSystem, "DisableDeleteNotification", sys.DWord(0))
	// "Last BIOS time" of the last boot: AM5 retraining memory at every boot.
	_ = m.R.SetValue(`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Power`, "FwPOSTTime", sys.DWord(31800))

	// --- WMI tables used by the collector
	w := m.W
	w.Set(`root\cimv2`, "Win32_OperatingSystem", sys.WMIRow{"Caption": "Microsoft Windows 11 Pro", "LastBootUpTime": "20260925081500.500000-180", "SystemDrive": "C:", "Locale": "0416"})
	w.Set(`root\cimv2`, "Win32_Processor", sys.WMIRow{"Name": "AMD Ryzen 7 7800X3D 8-Core Processor           ", "Manufacturer": "AuthenticAMD", "NumberOfCores": int64(8), "NumberOfLogicalProcessors": int64(16), "MaxClockSpeed": int64(4201), "VirtualizationFirmwareEnabled": true})
	w.Set(`root\cimv2`, "Win32_VideoController", sys.WMIRow{"Name": "NVIDIA GeForce RTX 3060", "DriverVersion": "32.0.15.6636", "DriverDate": "20250115000000.000000-000", "PNPDeviceID": `PCI\VEN_10DE&DEV_2504&SUBSYS_397D1462&REV_A1\4&1E2A9C0&0&0019`, "Status": "OK", "AdapterRAM": uint64(4293918720)})
	w.Set(`root\cimv2`, "Win32_PhysicalMemory",
		sys.WMIRow{"Capacity": uint64(16 << 30), "Speed": int64(4800), "ConfiguredClockSpeed": int64(4800), "Manufacturer": "Kingston", "PartNumber": "KF560C36-16", "DeviceLocator": "DIMM 1", "BankLabel": "P0 CHANNEL A", "SMBIOSMemoryType": int64(34)},
		sys.WMIRow{"Capacity": uint64(16 << 30), "Speed": int64(4800), "ConfiguredClockSpeed": int64(4800), "Manufacturer": "Kingston", "PartNumber": "KF560C36-16", "DeviceLocator": "DIMM 1", "BankLabel": "P0 CHANNEL B", "SMBIOSMemoryType": int64(34)})
	w.Set(`root\cimv2`, "Win32_BaseBoard", sys.WMIRow{"Manufacturer": "Gigabyte Technology Co., Ltd.", "Product": "B650M H"})
	w.Set(`root\cimv2`, "Win32_BIOS", sys.WMIRow{"Manufacturer": "American Megatrends International, LLC.", "SMBIOSBIOSVersion": "F31", "ReleaseDate": "20250312000000.000000+000"})
	w.Set(`root\cimv2`, "Win32_ComputerSystem", sys.WMIRow{"Manufacturer": "Gigabyte Technology Co., Ltd.", "Model": "B650M H", "PCSystemType": int64(1), "HypervisorPresent": true})
	w.Set(`root\cimv2`, "Win32_SCSIController",
		sys.WMIRow{"Name": "Controlador NVM Express padrão", "DriverName": "stornvme", "PNPDeviceID": `PCI\VEN_15B7&DEV_5019&SUBSYS_501915B7&REV_01\4&2B1D6F&0&0009`},
		sys.WMIRow{"Name": "Controlador SATA AHCI padrão", "DriverName": "storahci", "PNPDeviceID": `PCI\VEN_1022&DEV_43EB&SUBSYS_10621B21&REV_00\4&2766D23F&0&000A`},
		sys.WMIRow{"Name": "Controlador de Espaços de Armazenamento da Microsoft", "DriverName": "spaceport", "PNPDeviceID": `ROOT\SPACEPORT\0000`})
	w.Set(nsStorageMock, "MSFT_Partition", sys.WMIRow{"DiskNumber": int64(0), "DriveLetter": int64('C')}, sys.WMIRow{"DiskNumber": int64(1), "DriveLetter": int64('D')})
	w.Set(nsStorageMock, "MSFT_PhysicalDisk",
		sys.WMIRow{"DeviceId": "0", "ObjectId": MockDiskObjectID(0), "FriendlyName": "WD Green SN350 480GB", "MediaType": int64(4), "BusType": int64(17), "Size": uint64(480103981056), "HealthStatus": int64(0)},
		sys.WMIRow{"DeviceId": "1", "ObjectId": MockDiskObjectID(1), "FriendlyName": "Crucial MX500 1TB", "MediaType": int64(4), "BusType": int64(11), "Size": uint64(1000204886016), "HealthStatus": int64(0)})
	// Reliability counters are only reachable through the association (as on
	// real Windows); the NVMe disk also has them, but the health log wins.
	w.SetAssoc(nsStorageMock, "MSFT_PhysicalDisk.ObjectId="+MockDiskObjectID(0), "MSFT_StorageReliabilityCounter", sys.WMIRow{"DeviceId": "0", "Wear": int64(80), "Temperature": int64(45), "PowerOnHours": int64(9100)})
	w.SetAssoc(nsStorageMock, "MSFT_PhysicalDisk.ObjectId="+MockDiskObjectID(1), "MSFT_StorageReliabilityCounter", sys.WMIRow{"DeviceId": "1", "Wear": int64(12), "Temperature": int64(38), "PowerOnHours": int64(15210)})
	w.Set(`root\cimv2`, "Win32_LogicalDisk", sys.WMIRow{"DeviceID": "C:", "VolumeName": "Windows", "FileSystem": "NTFS", "Size": uint64(479000000000), "FreeSpace": uint64(41000000000), "DriveType": int64(3)},
		sys.WMIRow{"DeviceID": "D:", "VolumeName": "Jogos", "FileSystem": "NTFS", "Size": uint64(1000202039296), "FreeSpace": uint64(412000000000), "DriveType": int64(3)})
	w.Set(`root\cimv2`, "Win32_NetworkAdapter", sys.WMIRow{"GUID": EthernetGUID, "PNPDeviceID": `PCI\VEN_10EC&DEV_8125&SUBSYS_E0001458&REV_05\01000000684CE00000`, "DeviceID": "7", "PhysicalAdapter": "True", "NetConnectionID": "Ethernet", "NetEnabled": true})
	w.Set(`root\Microsoft\Windows\DeviceGuard`, "Win32_DeviceGuard", sys.WMIRow{"VirtualizationBasedSecurityStatus": int64(2), "SecurityServicesRunning": []any{int64(2)}, "SecurityServicesConfigured": []any{int64(2)},
		"AvailableSecurityProperties": []any{int64(1), int64(2), int64(3), int64(5), int64(7)}})
	w.Set(`root\CIMV2\Security\MicrosoftTpm`, "Win32_Tpm", sys.WMIRow{"IsEnabled_InitialValue": true, "IsActivated_InitialValue": true, "SpecVersion": "2.0, 0, 1.59"})
	w.Set(`root\cimv2`, "Win32_OptionalFeature", sys.WMIRow{"Name": "VirtualMachinePlatform", "InstallState": int64(2)})
	w.Set(`root\cimv2`, "Win32_SystemDriver",
		sys.WMIRow{"Name": "Ndis", "PathName": `C:\Windows\system32\drivers\ndis.sys`, "State": "Running", "StartMode": "Boot"},
		sys.WMIRow{"Name": "nvlddmkm", "PathName": `C:\Windows\System32\DriverStore\FileRepository\nv_dispi.inf_amd64_x\nvlddmkm.sys`, "State": "Running", "StartMode": "Manual"})
	w.Set(`root\Microsoft\Windows\Defender`, "MSFT_MpComputerStatus", sys.WMIRow{"RealTimeProtectionEnabled": true})
	w.Set(`root\StandardCimv2`, "MSFT_NetTCPSetting", sys.WMIRow{"SettingName": "Internet", "AutoTuningLevelLocal": int64(3)})
	w.Set(`root\StandardCimv2`, "MSFT_NetOffloadGlobalSetting", sys.WMIRow{"ReceiveSideScaling": int64(1)})
	// PCI devices as Device Manager names them (the WHEA naming); the GPU's
	// own HDMI audio is the case the first notebook report got wrong.
	w.Set(`root\cimv2`, "Win32_PnPEntity",
		sys.WMIRow{"Name": "NVIDIA GeForce RTX 3060", "PNPDeviceID": `PCI\VEN_10DE&DEV_2504&SUBSYS_397D1462&REV_A1\4&1E2A9C0&0&0019`, "PNPClass": "Display", "Present": true, "ConfigManagerErrorCode": int64(0)},
		sys.WMIRow{"Name": "NVIDIA High Definition Audio", "PNPDeviceID": `PCI\VEN_10DE&DEV_228E&SUBSYS_397D1462&REV_A1\4&1E2A9C0&0&0119`, "PNPClass": "MEDIA", "Present": true, "ConfigManagerErrorCode": int64(0)},
		sys.WMIRow{"Name": "Realtek PCIe 2.5GbE Family Controller", "PNPDeviceID": `PCI\VEN_10EC&DEV_8125&SUBSYS_E0001458&REV_05\01000000684CE00000`, "PNPClass": "Net", "Present": true, "ConfigManagerErrorCode": int64(0)},
		sys.WMIRow{"Name": "AMD PCI Express Root Port", "PNPDeviceID": `PCI\VEN_1022&DEV_14DB&SUBSYS_14DB1022&REV_00\3&11583659&0&08`, "PNPClass": "System", "Present": true, "ConfigManagerErrorCode": int64(0)},
		sys.WMIRow{"Name": "Placa antiga removida", "PNPDeviceID": `PCI\VEN_1B21&DEV_1142&SUBSYS_11421B21&REV_00\4&AAAA&0&0001`, "PNPClass": "USB", "Present": false, "ConfigManagerErrorCode": int64(45)},
		sys.WMIRow{"Name": "USB Receiver", "PNPDeviceID": `USB\VID_046D&PID_C543\6&26c36cb0&0&1`, "PNPClass": "USB", "Present": true, "ConfigManagerErrorCode": int64(0)})
	// Drivers in use (Drivers page inventory).
	w.Set(`root\cimv2`, "Win32_PnPSignedDriver",
		sys.WMIRow{"DeviceName": "NVIDIA GeForce RTX 3060", "DeviceClass": "DISPLAY", "DriverProviderName": "NVIDIA", "Manufacturer": "NVIDIA", "DriverVersion": "32.0.15.6636", "DriverDate": "20250115000000.******+000",
			"HardWareID": `PCI\VEN_10DE&DEV_2504&SUBSYS_397D1462&REV_A1`, "DeviceID": `PCI\VEN_10DE&DEV_2504&SUBSYS_397D1462&REV_A1\4&1E2A9C0&0&0019`, "InfName": "oem42.inf", "IsSigned": true, "Signer": "Microsoft Windows Hardware Compatibility Publisher"},
		sys.WMIRow{"DeviceName": "Realtek PCIe 2.5GbE Family Controller", "DeviceClass": "NET", "DriverProviderName": "Realtek", "Manufacturer": "Realtek", "DriverVersion": "10.68.815.2023", "DriverDate": "20230815000000.******+000",
			"HardWareID": `PCI\VEN_10EC&DEV_8125&SUBSYS_E0001458&REV_05`, "DeviceID": `PCI\VEN_10EC&DEV_8125&SUBSYS_E0001458&REV_05\01000000684CE00000`, "InfName": "oem12.inf", "IsSigned": true},
		sys.WMIRow{"DeviceName": "WAN Miniport (IP)", "DeviceClass": "NET", "DriverProviderName": "Microsoft", "DriverVersion": "10.0.26100.1", "DeviceID": `SWD\MSRRAS\MS_NDISWANIP`, "IsSigned": true},
		sys.WMIRow{"DeviceName": "Realtek High Definition Audio", "DeviceClass": "MEDIA", "DriverProviderName": "Realtek Semiconductor Corp.", "DriverVersion": "6.0.9549.1", "DriverDate": "20230705000000.******+000",
			"HardWareID": `HDAUDIO\FUNC_01&VEN_10EC&DEV_0897&SUBSYS_1458A194&REV_1003`, "DeviceID": `HDAUDIO\FUNC_01&VEN_10EC&DEV_0897&SUBSYS_1458A194&REV_1003\4&2F7F8A1&0&0001`, "IsSigned": true},
		sys.WMIRow{"DeviceName": "SMBus", "DeviceClass": "SYSTEM", "DriverProviderName": "Microsoft", "DriverVersion": "10.0.26100.1", "DriverDate": "20060621000000.******+000",
			"HardWareID": `PCI\VEN_1022&DEV_790B&SUBSYS_50011458&REV_71`, "DeviceID": `PCI\VEN_1022&DEV_790B&SUBSYS_50011458&REV_71\3&11583659&0&A0`, "IsSigned": true},
		sys.WMIRow{"DeviceName": "AMD GPIO Controller", "DeviceClass": "SYSTEM", "DriverProviderName": "Advanced Micro Devices, Inc", "DriverVersion": "2.2.0.130", "DriverDate": "20230601000000.******+000",
			"HardWareID": `ACPI\AMDI0030`, "DeviceID": `ACPI\AMDI0030\0`, "IsSigned": true},
		sys.WMIRow{"DeviceName": "Standard NVM Express Controller", "DeviceClass": "SCSIADAPTER", "DriverProviderName": "Microsoft", "DriverVersion": "10.0.26100.1", "DriverDate": "20060621000000.******+000",
			"HardWareID": `PCI\VEN_15B7&DEV_5019&SUBSYS_501915B7&REV_01`, "DeviceID": `PCI\VEN_15B7&DEV_5019&SUBSYS_501915B7&REV_01\4&AB12CD&0&0009`, "IsSigned": true},
		sys.WMIRow{"DeviceName": "USB xHCI Compliant Host Controller", "DeviceClass": "USB", "DriverProviderName": "Microsoft", "DriverVersion": "10.0.26100.1", "DriverDate": "20060621000000.******+000",
			"HardWareID": `PCI\VEN_1022&DEV_15B6&SUBSYS_50071458&REV_00`, "DeviceID": `PCI\VEN_1022&DEV_15B6&SUBSYS_50071458&REV_00\4&1F&0&0141`, "IsSigned": true},
		sys.WMIRow{"DeviceName": "Intel(R) Wireless Bluetooth(R)", "DeviceClass": "BLUETOOTH", "DriverProviderName": "Intel Corporation", "DriverVersion": "22.160.0.4", "DriverDate": "20221010000000.******+000",
			"HardWareID": `USB\VID_8087&PID_0032&REV_0000`, "DeviceID": `USB\VID_8087&PID_0032\5&2A&0&10`, "IsSigned": true},
		sys.WMIRow{"DeviceName": "HID-compliant mouse", "DeviceClass": "MOUSE", "DriverProviderName": "Microsoft", "DriverVersion": "10.0.26100.1", "DeviceID": `HID\VID_046D&PID_C547&MI_01&COL01\7&1&0&0000`, "IsSigned": true})

	// --- Events (stability history)
	m.E.Events = []sys.Event{
		{Channel: "System", Provider: "Microsoft-Windows-WHEA-Logger", ID: 19, Level: 3, Time: time.Now().Add(-26 * time.Hour), Message: "Um erro de hardware corrigido ocorreu (PCI Express Root Port)."},
		{Channel: "System", Provider: "Microsoft-Windows-Kernel-Power", ID: 41, Level: 1, Time: time.Now().Add(-7 * 24 * time.Hour), Message: "O sistema foi reinicializado sem ser desligado corretamente."},
		{Channel: "Application", Provider: "Microsoft-Windows-Defrag", ID: 258, Level: 4, Time: time.Now().Add(-12 * 24 * time.Hour), Data: map[string]string{"0": "retrim", "1": "Windows (C:)"}},
		{Channel: "Application", Provider: "Microsoft-Windows-Defrag", ID: 258, Level: 4, Time: time.Now().Add(-40 * 24 * time.Hour), Data: map[string]string{"0": "retrim", "1": "Windows (C:)"}},
		{Channel: "Application", Provider: "Microsoft-Windows-Defrag", ID: 258, Level: 4, Time: time.Now().Add(-70 * 24 * time.Hour), Data: map[string]string{"0": "retrim", "1": "Jogos (D:)"}},
		// A long match on a dusty PC: passive cooling engaged and disengaged
		// on two different days.
		{Channel: "Microsoft-Windows-Kernel-Power/Thermal-Operational", Provider: "Microsoft-Windows-Kernel-Power", ID: 114, Level: 4, Time: time.Now().Add(-3*24*time.Hour - 40*time.Minute), Message: "A zona térmica TZ00 ligou o resfriamento passivo."},
		{Channel: "Microsoft-Windows-Kernel-Power/Thermal-Operational", Provider: "Microsoft-Windows-Kernel-Power", ID: 114, Level: 4, Time: time.Now().Add(-3 * 24 * time.Hour), Message: "A zona térmica TZ00 desligou o resfriamento passivo."},
		{Channel: "Microsoft-Windows-Kernel-Power/Thermal-Operational", Provider: "Microsoft-Windows-Kernel-Power", ID: 114, Level: 4, Time: time.Now().Add(-26 * time.Hour), Message: "A zona térmica TZ00 ligou o resfriamento passivo."},
	}
	// One ACPI thermal zone at 68 °C, not limiting anything now.
	m.W.Set(`root\cimv2`, "Win32_PerfFormattedData_Counters_ThermalZoneInformation",
		sys.WMIRow{"Name": `\_TZ.TZ00`, "Temperature": uint32(341), "PercentPassiveLimit": uint32(100), "ThrottleReasons": uint32(0)})

	seedHealth(m)
	m.N.RecycleBytes, m.N.RecycleItems = 1_250_000_000, 37
	m.N.Mouse = [3]int{6, 10, 1}
	seedInputDevices(m)
	SeedMMAgent(m)
}

const nsStorageMock = `root\Microsoft\Windows\Storage`

// simulateTool emulates the Windows tools Axion invokes.
func (m *Mock) simulateTool(exe string, args []string, _ string) (sys.Result, bool) {
	name := strings.ToLower(exe)
	if i := strings.LastIndexAny(name, `\/`); i >= 0 {
		name = name[i+1:]
	}
	low := make([]string, len(args))
	for i, a := range args {
		low[i] = strings.ToLower(a)
	}
	switch name {
	case "bcdedit.exe", "bcdedit":
		m.mu.Lock()
		defer m.mu.Unlock()
		if len(low) >= 2 && low[0] == "/enum" {
			var sb strings.Builder
			sb.WriteString("\r\nWindows Boot Loader\r\n-------------------\r\n")
			sb.WriteString("identifier              {current}\r\n")
			sb.WriteString("device                  partition=C:\r\n")
			sb.WriteString("path                    \\WINDOWS\\system32\\winload.efi\r\n")
			keys := make([]string, 0, len(m.BCD))
			for k := range m.BCD {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(&sb, "%-24s%s\r\n", k, m.BCD[k])
			}
			return sys.Result{Stdout: sb.String()}, true
		}
		if len(low) >= 4 && low[0] == "/set" {
			v := args[3]
			// Like bcdedit: booleans are shown as Yes/No; enumerated
			// elements keep their named value (hypervisorlaunchtype Off).
			switch low[2] {
			case "hypervisorlaunchtype", "nx", "bootmenupolicy", "bootstatuspolicy", "tscsyncpolicy", "x2apicpolicy":
				if strings.EqualFold(v, "off") {
					v = "Off"
				} else if strings.EqualFold(v, "auto") {
					v = "Auto"
				}
			default:
				switch strings.ToLower(v) {
				case "true", "yes", "on":
					v = "Yes"
				case "false", "no", "off":
					v = "No"
				}
			}
			m.BCD[low[2]] = v
			return sys.Result{Stdout: "The operation completed successfully."}, true
		}
		if len(low) >= 3 && low[0] == "/deletevalue" {
			if _, ok := m.BCD[low[2]]; !ok {
				return sys.Result{ExitCode: 1, Stdout: "An error occurred while attempting to delete the specified data.\r\nElement not found."}, true
			}
			delete(m.BCD, low[2])
			return sys.Result{Stdout: "The operation completed successfully."}, true
		}
	case "pnputil.exe", "pnputil":
		m.mu.Lock()
		defer m.mu.Unlock()
		if len(low) >= 4 && low[0] == "/enum-drivers" && low[len(low)-2] == "/format" && low[len(low)-1] == "xml" {
			return sys.Result{Stdout: m.pnpUtilXML()}, true
		}
		if len(low) >= 2 && low[0] == "/delete-driver" {
			// Like the real tool: without /uninstall and /force a package that
			// a device uses is refused. The Axion must never pass those flags,
			// so the simulator fails if it does.
			for _, a := range low[2:] {
				if a == "/uninstall" || a == "/force" {
					return sys.Result{ExitCode: 1, Stdout: "The simulator refuses " + a + ": the Axion must not remove a driver from a device."}, true
				}
			}
			for _, d := range pnpDrivers {
				if strings.ToLower(d.Name) != low[1] {
					continue
				}
				if m.PnpDeleted[low[1]] {
					break
				}
				if d.InUse {
					return sys.Result{ExitCode: 1, Stdout: "Failed to delete driver package: One or more devices are presently installed using the specified INF."}, true
				}
				if m.PnpDeleted == nil {
					m.PnpDeleted = map[string]bool{}
				}
				m.PnpDeleted[low[1]] = true
				return sys.Result{Stdout: "Driver package deleted successfully."}, true
			}
			return sys.Result{ExitCode: 1, Stdout: "Failed to delete driver package: The specified INF is not in the driver store."}, true
		}
	case "powercfg.exe", "powercfg":
		if len(low) >= 1 && low[0] == "/batteryreport" {
			out := ""
			for i, a := range low {
				if a == "/output" && i+1 < len(args) {
					out = args[i+1]
				}
			}
			return m.batteryReport(out), true
		}
		if len(low) >= 2 && (low[0] == "/hibernate" || low[0] == "/h") {
			v := uint32(0)
			if low[1] == "on" {
				v = 1
			}
			_ = m.R.SetValue(`HKLM\SYSTEM\CurrentControlSet\Control\Power`, "HibernateEnabled", sys.DWord(v))
			return sys.Result{}, true
		}
	case "netsh.exe", "netsh":
		if len(low) >= 5 && low[0] == "int" && low[1] == "tcp" && low[2] == "set" && low[3] == "global" {
			k, v, _ := strings.Cut(low[4], "=")
			switch k {
			case "autotuninglevel":
				levels := map[string]int64{"disabled": 0, "highlyrestricted": 1, "restricted": 2, "normal": 3, "experimental": 4}
				m.W.Set(`root\StandardCimv2`, "MSFT_NetTCPSetting", sys.WMIRow{"SettingName": "Internet", "AutoTuningLevelLocal": levels[v]})
			case "rss":
				n := int64(0)
				if v == "enabled" {
					n = 1
				}
				m.W.Set(`root\StandardCimv2`, "MSFT_NetOffloadGlobalSetting", sys.WMIRow{"ReceiveSideScaling": n})
			}
			return sys.Result{Stdout: "Ok."}, true
		}
	case "sfc.exe", "sfc":
		if len(low) == 1 && low[0] == "/scannow" {
			return m.simulateSFC(), true
		}
	case "dism.exe", "dism":
		if len(low) >= 3 && low[0] == "/online" && low[1] == "/cleanup-image" && low[2] == "/restorehealth" {
			return m.simulateRestoreHealth(), true
		}
		if len(low) >= 3 && low[1] == "/cleanup-image" && low[2] == "/startcomponentcleanup" {
			m.N.mu.Lock()
			m.N.FreeBytes += 1_700 << 20 // superseded components removed
			m.N.mu.Unlock()
			return sys.Result{Stdout: "[==========================100.0%==========================]\r\nThe operation completed successfully.\r\n"}, true
		}
		for _, a := range args {
			if strings.HasPrefix(strings.ToLower(a), "/featurename:") {
				feat := a[len("/featurename:"):]
				state := int64(2)
				for _, b := range low {
					if b == "/enable-feature" {
						state = 1
					}
				}
				m.W.Set(`root\cimv2`, "Win32_OptionalFeature", sys.WMIRow{"Name": feat, "InstallState": state})
				return sys.Result{ExitCode: 3010}, true
			}
		}
	case "fsutil.exe", "fsutil":
		if len(low) >= 3 && low[0] == "behavior" && low[2] == "disabledeletenotify" {
			switch {
			case low[1] == "query":
				v, _ := m.R.GetValue(sys.KeyFileSystem, "DisableDeleteNotification")
				n := 0
				if v.Exists && v.Int != 0 {
					n = 1
				}
				what := map[int]string{0: "Permite que as operações de TRIM sejam enviadas ao dispositivo de armazenamento", 1: "Desabilita o envio de operações de TRIM ao dispositivo de armazenamento"}[n]
				return sys.Result{Stdout: fmt.Sprintf("NTFS DisableDeleteNotify = %d  (%s)\r\nReFS DisableDeleteNotify = 0  (Permite que as operações de TRIM sejam enviadas ao dispositivo de armazenamento)\r\n", n, what)}, true
			case low[1] == "set" && len(low) >= 4:
				n := uint32(0)
				if low[len(low)-1] == "1" {
					n = 1
				}
				_ = m.R.SetValue(sys.KeyFileSystem, "DisableDeleteNotification", sys.DWord(n))
				return sys.Result{Stdout: fmt.Sprintf("NTFS DisableDeleteNotify = %d\r\n", n)}, true
			}
		}
	case "defrag.exe", "defrag":
		if len(low) >= 2 && strings.HasSuffix(low[0], ":") {
			retrim := false
			for _, a := range low[1:] {
				if a == "/l" || a == "/retrim" {
					retrim = true
				}
			}
			label, ok := map[string]string{"c:": "Windows", "d:": "Jogos"}[low[0]]
			if !retrim || !ok {
				return sys.Result{ExitCode: 1, Stdout: "O parâmetro está incorreto. (0x89000004)\r\n"}, true
			}
			letter := strings.ToUpper(low[0])
			m.E.mu.Lock()
			m.E.Events = append(m.E.Events, sys.Event{Channel: "Application", Provider: "Microsoft-Windows-Defrag", ID: 258, Level: 4, Time: time.Now(),
				Data: map[string]string{"0": "retrim", "1": label + " (" + letter + ")"}})
			m.E.mu.Unlock()
			return sys.Result{Stdout: "Otimizador de Unidades da Microsoft\r\nCopyright (c) Microsoft Corp.\r\n\r\nInvocando recorte em " + label + " (" + letter + ")...\r\n\r\nA operação foi concluída com êxito.\r\n"}, true
		}
	case "presentmon-2.6.0-x64.exe":
		return m.simulatePresentMon(args), true
	case "winget.exe", "winget":
		return m.simulateWinget(args, low), true
	case "nvidia-smi.exe":
		if len(low) > 0 && strings.HasPrefix(low[0], "--query-gpu=pcie.link.gen.current") {
			// The reading during a capture (0.7): the GPU working, the link at
			// its full generation and width.
			return sys.Result{Stdout: "4, 4, 16, 16, 97, P0\r\n"}, true
		}
		if len(low) > 0 && (strings.HasPrefix(low[0], "--query-gpu=clocks_throttle_reasons") || strings.HasPrefix(low[0], "--query-gpu=clocks_event_reasons")) {
			// Throttling readings during a capture: a graphics card at its power
			// limit, no thermal slowdown, 71 °C, clock near the maximum. One
			// line per second in the loop mode (-l); a few are enough here.
			line := "Not Active, Not Active, Not Active, Not Active, Active, 71, 1950, 2100, 55.40, 98\r\n"
			return sys.Result{Stdout: strings.Repeat(line, 6)}, true
		}
		if len(low) > 0 && strings.HasPrefix(low[0], "--query-gpu") {
			return sys.Result{Stdout: "566.36, 12288, 41, 4, 16, 16, 1, P8, 2\r\n"}, true
		}
		if len(low) >= 3 && low[0] == "-q" {
			return sys.Result{Stdout: "==============NVSMI LOG==============\r\n\r\nAttached GPUs : 1\r\nGPU 00000000:01:00.0\r\n    FB Memory Usage\r\n        Total                             : 12288 MiB\r\n    BAR1 Memory Usage\r\n        Total                             : 256 MiB\r\n        Used                              : 5 MiB\r\n"}, true
		}
	}
	return sys.Result{}, false
}

// MockDiskObjectID is the MSFT_PhysicalDisk.ObjectId of a simulated disk: it
// has backslashes and quotes, like the real ones, to exercise WQL escaping.
func MockDiskObjectID(n int) string {
	return fmt.Sprintf(`{1}\\MOCKPC\root/Microsoft/Windows/Storage/Providers_v2\SPACES_PhysicalDisk.ObjectId="{3f6c1c8a-7c6e-4b2a-9d0e-6a7f0b1c2d3e}:PD:{8d2a0c11-5b6e-4f7a-a1c2-0d9e8f7a6b%02d}"`, n)
}

// WingetInstalled are the packages the simulated winget reports installed
// (id -> version); WingetUpgrades the ones with a newer version (id ->
// available). WingetFail lists ids whose install fails with the code.
var (
	wingetDefaultInstalled = map[string]string{"Valve.Steam": "1.0.0", "Discord.Discord": "1.0.9212", "7zip.7zip": "24.08", "Google.Chrome": "130.0.6723.117"}
	wingetDefaultUpgrades  = map[string]string{"Discord.Discord": "1.0.9230", "Google.Chrome": "131.0.6778.86"}
)

// simulateWinget emulates the package manager: export writes the JSON of
// the installed packages, upgrade lists the newer versions, install and
// upgrade of an id succeed (and the id joins the installed list), except
// the ids in WingetFail.
func (m *Mock) simulateWinget(args, low []string) sys.Result {
	m.mu.Lock()
	delay := m.WingetDelay
	m.mu.Unlock()
	if delay > 0 && len(low) > 0 && (low[0] == "install" || low[0] == "upgrade") && (len(low) < 2 || low[1] != "--accept-source-agreements") {
		time.Sleep(delay) // an installer takes its time (not the upgrade list)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.WingetInstalled == nil {
		m.WingetInstalled = map[string]string{}
		for k, v := range wingetDefaultInstalled {
			m.WingetInstalled[k] = v
		}
		m.WingetUpgrades = map[string]string{}
		for k, v := range wingetDefaultUpgrades {
			m.WingetUpgrades[k] = v
		}
	}
	arg := func(flag string) string {
		for i, a := range low {
			if a == flag && i+1 < len(args) {
				return args[i+1]
			}
		}
		return ""
	}
	has := func(flag string) bool {
		for _, a := range low {
			if a == flag {
				return true
			}
		}
		return false
	}
	if len(low) == 0 {
		return sys.Result{Stdout: "Windows Package Manager v1.9.25200\r\n"}
	}
	switch low[0] {
	case "--version", "-v":
		return sys.Result{Stdout: "v1.9.25200\r\n"}
	case "export":
		type pkg struct {
			PackageIdentifier string `json:"PackageIdentifier"`
			Version           string `json:"Version,omitempty"`
		}
		var pkgs []pkg
		ids := make([]string, 0, len(m.WingetInstalled))
		for id := range m.WingetInstalled {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			pkgs = append(pkgs, pkg{id, m.WingetInstalled[id]})
		}
		doc := map[string]any{"$schema": "https://aka.ms/winget-packages.schema.2.0.json", "CreationDate": time.Now().Format(time.RFC3339),
			"Sources":       []any{map[string]any{"Packages": pkgs, "SourceDetails": map[string]any{"Argument": "https://cdn.winget.microsoft.com/cache", "Identifier": "Microsoft.Winget.Source_8wekyb3d8bbwe", "Name": "winget", "Type": "Microsoft.PreIndexed.Package"}}},
			"WinGetVersion": "1.9.25200"}
		b, _ := json.MarshalIndent(doc, "", "  ")
		if out := arg("-o"); out != "" {
			if err := os.WriteFile(out, b, 0o644); err != nil {
				return sys.Result{ExitCode: 1, Stdout: "Failed: " + err.Error() + "\r\n"}
			}
		}
		return sys.Result{Stdout: "Installed package is not available from any source: Programa Local\r\n"}
	case "upgrade":
		if id := arg("--id"); id != "" {
			if m.WingetFail[id] != 0 {
				return sys.Result{ExitCode: m.WingetFail[id], Stdout: "Installer failed with exit code: 1603\r\n"}
			}
			if _, ok := m.WingetUpgrades[id]; !ok {
				return sys.Result{ExitCode: int(uint32(0x8A15002B)), Stdout: "No applicable update found.\r\n"}
			}
			m.WingetInstalled[id] = m.WingetUpgrades[id]
			delete(m.WingetUpgrades, id)
			return sys.Result{Stdout: "Found " + id + "\r\nDownloading…\r\nSuccessfully installed\r\n"}
		}
		if has("--all") {
			n, failed := 0, 0
			var sb strings.Builder
			for id, v := range m.WingetUpgrades {
				if m.WingetFail[id] != 0 {
					failed++
					sb.WriteString("Found " + id + "\r\nInstaller failed with exit code: 1603\r\n")
					continue
				}
				m.WingetInstalled[id] = v
				delete(m.WingetUpgrades, id)
				n++
				sb.WriteString("Found " + id + "\r\nSuccessfully installed\r\n")
			}
			if failed > 0 {
				sb.WriteString(fmt.Sprintf("%d package(s) upgraded, %d failed.\r\n", n, failed))
				return sys.Result{ExitCode: int(uint32(0x8A15002C)), Stdout: sb.String()}
			}
			sb.WriteString(fmt.Sprintf("%d package(s) upgraded.\r\n", n))
			return sys.Result{Stdout: sb.String()}
		}
		if len(m.WingetUpgrades) == 0 {
			return sys.Result{Stdout: "No installed package found matching input criteria.\r\n"}
		}
		var sb strings.Builder
		sb.WriteString("Name                 Id                 Version        Available        Source\r\n")
		sb.WriteString("-----------------------------------------------------------------------------------\r\n")
		ids := make([]string, 0, len(m.WingetUpgrades))
		for id := range m.WingetUpgrades {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			sb.WriteString(fmt.Sprintf("%-20s %-18s %-14s %-16s winget\r\n", "Programa "+id[strings.LastIndex(id, ".")+1:]+"…", id, m.WingetInstalled[id], m.WingetUpgrades[id]))
		}
		sb.WriteString(fmt.Sprintf("%d upgrades available.\r\n", len(ids)))
		return sys.Result{Stdout: sb.String()}
	case "install":
		id := arg("--id")
		if id == "" {
			return sys.Result{ExitCode: 1, Stdout: "Argument --id missing\r\n"}
		}
		if code := m.WingetFail[id]; code != 0 {
			return sys.Result{ExitCode: code, Stdout: "Installer failed with exit code: 1603\r\n"}
		}
		if _, ok := m.WingetInstalled[id]; ok {
			if _, up := m.WingetUpgrades[id]; !up {
				return sys.Result{ExitCode: int(uint32(0x8A150061)), Stdout: "Found an existing package already installed. Trying to upgrade the installed package...\r\nNo available upgrade found.\r\nNo newer package versions are available from the configured sources.\r\n"}
			}
		}
		if strings.Contains(id, ".") || has("msstore") {
			m.WingetInstalled[id] = "1.0.0"
			delete(m.WingetUpgrades, id)
			return sys.Result{Stdout: "Found " + id + "\r\nDownloading https://example.invalid/" + id + "\r\n  ██████████████████████████████  12.0 MB / 12.0 MB\r\nSuccessfully verified installer hash\r\nStarting package install...\r\nSuccessfully installed\r\n"}
		}
		return sys.Result{ExitCode: int(uint32(0x8A150014)), Stdout: "No package found matching input criteria.\r\n"}
	}
	return sys.Result{ExitCode: 1, Stdout: "Unrecognized command\r\n"}
}

// pnpDrivers are the packages of the simulated driver store, as
// "pnputil /enum-drivers /devices /format xml" prints them: the running NVIDIA
// and Intel display packages plus an older NVIDIA package that a newer one
// replaced. A package with InUse has a device installed on it.
var pnpDrivers = []struct {
	Name  string
	InUse bool
	XML   string
}{
	{"oem172.inf", true, `    <Driver DriverName="oem172.inf">
        <OriginalName>iigd_dch.inf</OriginalName>
        <ProviderName>Intel Corporation</ProviderName>
        <ClassName>Display</ClassName>
        <ClassGuid>{4d36e968-e325-11ce-bfc1-08002be10318}</ClassGuid>
        <DriverVersion>06/17/2026 32.0.101.7088</DriverVersion>
        <Devices><Device InstanceId="PCI\VEN_8086&amp;DEV_A78B"><DeviceDescription>Intel(R) UHD Graphics</DeviceDescription><Status>Started</Status></Device></Devices>
    </Driver>
`},
	{"oem148.inf", true, `    <Driver DriverName="oem148.inf">
        <OriginalName>nvdmi.inf</OriginalName>
        <ProviderName>NVIDIA</ProviderName>
        <ClassName>Display</ClassName>
        <ClassGuid>{4d36e968-e325-11ce-bfc1-08002be10318}</ClassGuid>
        <DriverVersion>09/17/2026 32.0.16.1714</DriverVersion>
        <Devices><Device InstanceId="PCI\VEN_10DE&amp;DEV_25EC"><DeviceDescription>NVIDIA GeForce RTX 3050</DeviceDescription><Status>Started</Status></Device></Devices>
    </Driver>
`},
	{"oem120.inf", false, `    <Driver DriverName="oem120.inf">
        <OriginalName>nvdmi.inf</OriginalName>
        <ProviderName>NVIDIA</ProviderName>
        <ClassName>Display</ClassName>
        <ClassGuid>{4d36e968-e325-11ce-bfc1-08002be10318}</ClassGuid>
        <DriverVersion>03/10/2026 32.0.15.9000</DriverVersion>
    </Driver>
`},
}

// pnpUtilXML is the output of "pnputil /enum-drivers /devices /format xml" now:
// the packages that were not deleted. The caller holds m.mu.
func (m *Mock) pnpUtilXML() string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	sb.WriteString(`<PnpUtil Version="10.0.26300" Command="/enum-drivers /devices /format xml">` + "\n")
	for _, d := range pnpDrivers {
		if !m.PnpDeleted[strings.ToLower(d.Name)] {
			sb.WriteString(d.XML)
		}
	}
	sb.WriteString("</PnpUtil>")
	return sb.String()
}

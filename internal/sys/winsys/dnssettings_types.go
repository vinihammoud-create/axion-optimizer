package winsys

import (
	"net"
	"strings"
	"unsafe"
)

// Layout of the structures of GetInterfaceDnsSettings and
// SetInterfaceDnsSettings (netioapi.h, x64: every pointer has 8 bytes). The
// fields and their order are those of the Microsoft Learn pages of
// DNS_INTERFACE_SETTINGS, DNS_INTERFACE_SETTINGS_EX and
// DNS_INTERFACE_SETTINGS3. Pointers are *uint16 and unsafe.Pointer, never
// uintptr, so go vet (unsafeptr) stays quiet and the collector sees them.

// Versions of the structure (netioapi.h; the Learn pages name them but do not
// show the numbers).
const (
	dnsInterfaceSettingsVersion1 = 1 // DNS_INTERFACE_SETTINGS
	dnsInterfaceSettingsVersion2 = 2 // DNS_INTERFACE_SETTINGS_EX
	dnsInterfaceSettingsVersion3 = 3 // DNS_INTERFACE_SETTINGS3 (build 19645+)
)

// Flags of DNS_INTERFACE_SETTINGS (Learn).
const (
	dnsSettingIPv6       = 0x0001 // DNS_SETTING_IPV6: only the IPv6 stack
	dnsSettingNameServer = 0x0002 // DNS_SETTING_NAMESERVER: static servers in NameServer
	dnsSettingDoH        = 0x1000 // DNS_SETTING_DOH (documentation only: Axion never writes DoH)
	dnsSettingDoHProfile = 0x2000 // DNS_SETTING_DOH_PROFILE (same)
)

const (
	ipAdapterDHCPEnabled = 0x4 // IP_ADAPTER_DHCP_ENABLED

	keyTcpipIfaces  = `HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\`
	keyTcpip6Ifaces = `HKLM\SYSTEM\CurrentControlSet\Services\Tcpip6\Parameters\Interfaces\`
)

// keyDohIfaces is where the DNS client keeps the DoH settings of an adapter
// (not documented by Microsoft; only a fallback to detect DoH).
func keyDohIfaces(guid string) string {
	return `HKLM\SYSTEM\CurrentControlSet\Services\Dnscache\InterfaceSpecificParameters\` + guid + `\DohInterfaceSettings`
}

// dnsInterfaceSettings mirrors DNS_INTERFACE_SETTINGS (version 1): 64 bytes.
type dnsInterfaceSettings struct {
	Version             uint32  // 0
	_                   uint32  // 4 (alignment of the ULONG64)
	Flags               uint64  // 8
	Domain              *uint16 // 16
	NameServer          *uint16 // 24
	SearchList          *uint16 // 32
	RegistrationEnabled uint32  // 40
	RegisterAdapterName uint32  // 44
	EnableLLMNR         uint32  // 48
	QueryAdapterName    uint32  // 52
	ProfileNameServer   *uint16 // 56
}

// dnsInterfaceSettingsEx mirrors DNS_INTERFACE_SETTINGS_EX (version 2): 80
// bytes. Defined only for the layout test.
type dnsInterfaceSettingsEx struct {
	V1                          dnsInterfaceSettings // 0
	DisableUnconstrainedQueries uint32               // 64
	_                           uint32               // 68
	SupplementalSearchList      *uint16              // 72
}

// dnsInterfaceSettings3 mirrors DNS_INTERFACE_SETTINGS3 (version 3): 112 bytes.
type dnsInterfaceSettings3 struct {
	Version                     uint32         // 0
	_                           uint32         // 4
	Flags                       uint64         // 8
	Domain                      *uint16        // 16
	NameServer                  *uint16        // 24
	SearchList                  *uint16        // 32
	RegistrationEnabled         uint32         // 40
	RegisterAdapterName         uint32         // 44
	EnableLLMNR                 uint32         // 48
	QueryAdapterName            uint32         // 52
	ProfileNameServer           *uint16        // 56
	DisableUnconstrainedQueries uint32         // 64
	_                           uint32         // 68
	SupplementalSearchList      *uint16        // 72
	CServerProperties           uint32         // 80
	_                           uint32         // 84
	ServerProperties            unsafe.Pointer // 88 (*DNS_SERVER_PROPERTY: only counted, never read)
	CProfileServerProperties    uint32         // 96
	_                           uint32         // 100
	ProfileServerProperties     unsafe.Pointer // 104
}

// splitServers reads a list of DNS servers separated by commas, semicolons or
// spaces and keeps the valid addresses in their canonical form.
func splitServers(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		if ip := net.ParseIP(strings.TrimSpace(f)); ip != nil {
			out = append(out, ip.String())
		}
	}
	return out
}

package winsys

import (
	"reflect"
	"testing"
	"unsafe"
)

// The structures are passed to iphlpapi.dll: size and offsets must be the
// ones of netioapi.h on x64 (checked against the Microsoft Learn pages).
func TestDNSSettingsLayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("layout de 64 bits")
	}
	if s := unsafe.Sizeof(dnsInterfaceSettings{}); s != 64 {
		t.Errorf("DNS_INTERFACE_SETTINGS = %d bytes", s)
	}
	if s := unsafe.Sizeof(dnsInterfaceSettingsEx{}); s != 80 {
		t.Errorf("DNS_INTERFACE_SETTINGS_EX = %d bytes", s)
	}
	if s := unsafe.Sizeof(dnsInterfaceSettings3{}); s != 112 {
		t.Errorf("DNS_INTERFACE_SETTINGS3 = %d bytes", s)
	}
	var v1 dnsInterfaceSettings
	var ex dnsInterfaceSettingsEx
	var v3 dnsInterfaceSettings3
	for name, c := range map[string][2]uintptr{
		"v1.Flags":                       {unsafe.Offsetof(v1.Flags), 8},
		"v1.NameServer":                  {unsafe.Offsetof(v1.NameServer), 24},
		"v1.RegistrationEnabled":         {unsafe.Offsetof(v1.RegistrationEnabled), 40},
		"v1.ProfileNameServer":           {unsafe.Offsetof(v1.ProfileNameServer), 56},
		"ex.DisableUnconstrainedQueries": {unsafe.Offsetof(ex.DisableUnconstrainedQueries), 64},
		"ex.SupplementalSearchList":      {unsafe.Offsetof(ex.SupplementalSearchList), 72},
		"v3.NameServer":                  {unsafe.Offsetof(v3.NameServer), 24},
		"v3.ProfileNameServer":           {unsafe.Offsetof(v3.ProfileNameServer), 56},
		"v3.DisableUnconstrainedQueries": {unsafe.Offsetof(v3.DisableUnconstrainedQueries), 64},
		"v3.SupplementalSearchList":      {unsafe.Offsetof(v3.SupplementalSearchList), 72},
		"v3.CServerProperties":           {unsafe.Offsetof(v3.CServerProperties), 80},
		"v3.ServerProperties":            {unsafe.Offsetof(v3.ServerProperties), 88},
		"v3.CProfileServerProperties":    {unsafe.Offsetof(v3.CProfileServerProperties), 96},
		"v3.ProfileServerProperties":     {unsafe.Offsetof(v3.ProfileServerProperties), 104},
	} {
		if c[0] != c[1] {
			t.Errorf("%s no offset %d, esperado %d", name, c[0], c[1])
		}
	}
}

func TestSplitServers(t *testing.T) {
	got := splitServers("1.1.1.1, 8.8.8.8 2606:4700:4700:0:0:0:0:1111;lixo")
	want := []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("splitServers = %v", got)
	}
	if splitServers("") != nil || splitServers(" , ") != nil {
		t.Error("lista vazia deve ser nula")
	}
}

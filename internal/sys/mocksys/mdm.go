package mocksys

import (
	"errors"
	"strings"

	"axion/internal/sys"
)

// MDMRegistered implements sys.Native (IsDeviceRegisteredWithManagement).
func (n *Native) MDMRegistered() (bool, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.MDMErr != nil {
		return false, n.MDMErr
	}
	return n.MDMEnrolled, nil
}

// ErrManagementRead is the failure FailManagementRead simulates.
var ErrManagementRead = errors.New("tempo esgotado (simulado)")

// SetDomain simulates a PC joined to the domain name (Win32_ComputerSystem
// PartOfDomain = true, DomainRole 1 "member workstation"); an empty name
// takes it out of the domain (PartOfDomain = false, workgroup WORKGROUP).
func (m *Mock) SetDomain(name string) {
	row := m.computerSystem()
	if name == "" {
		row["PartOfDomain"], row["Domain"], row["DomainRole"] = false, "WORKGROUP", int64(0)
	} else {
		row["PartOfDomain"], row["Domain"], row["DomainRole"] = true, name, int64(1)
	}
	m.W.Set(`root\cimv2`, "Win32_ComputerSystem", row)
}

// SetDomainUnknown simulates the NULL PartOfDomain Microsoft documents as
// "not in a domain or the status is unknown".
func (m *Mock) SetDomainUnknown() {
	row := m.computerSystem()
	delete(row, "PartOfDomain")
	m.W.Set(`root\cimv2`, "Win32_ComputerSystem", row)
}

// SetMDM simulates a PC enrolled (or not) in a mobile device management
// service such as Intune.
func (m *Mock) SetMDM(enrolled bool) {
	m.N.mu.Lock()
	defer m.N.mu.Unlock()
	m.N.MDMEnrolled, m.N.MDMErr = enrolled, nil
}

// FailManagementRead makes both management reads fail: the
// Win32_ComputerSystem query and IsDeviceRegisteredWithManagement.
func (m *Mock) FailManagementRead() {
	m.W.FailQuery(`root\cimv2`, "Win32_ComputerSystem", ErrManagementRead)
	m.N.mu.Lock()
	defer m.N.mu.Unlock()
	m.N.MDMErr = ErrManagementRead
}

// computerSystem returns a copy of the simulated Win32_ComputerSystem row.
func (m *Mock) computerSystem() sys.WMIRow {
	m.W.mu.Lock()
	defer m.W.mu.Unlock()
	row := sys.WMIRow{}
	if rows := m.W.Tables[strings.ToLower(`root\cimv2`)+"|win32_computersystem"]; len(rows) > 0 {
		for k, v := range rows[0] {
			row[k] = v
		}
	}
	return row
}

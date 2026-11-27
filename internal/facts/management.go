package facts

// ManagementInfo tells whether an organization manages this PC: joined to an
// Active Directory domain (Win32_ComputerSystem.PartOfDomain) or enrolled in
// a mobile device management service such as Intune (MDM, read with
// IsDeviceRegisteredWithManagement). On such a PC the Windows policies come
// from the organization. A read that failed is "unknown", never "not
// managed": the Read flags say which answers came back. The name of the
// domain is not kept: it names the organization and nothing here needs it.
type ManagementInfo struct {
	// DomainRead is true when Win32_ComputerSystem answered PartOfDomain with
	// true or false (Microsoft: NULL means "not in a domain or the status is
	// unknown", so NULL is not read).
	DomainRead bool `json:"domainRead"`
	Domain     bool `json:"domain"`
	// MDMRead is true when IsDeviceRegisteredWithManagement answered.
	MDMRead bool `json:"mdmRead"`
	MDM     bool `json:"mdm"`
}

// Managed tells whether a read answered that the PC is managed.
func (m ManagementInfo) Managed() bool {
	return (m.DomainRead && m.Domain) || (m.MDMRead && m.MDM)
}

// KnownUnmanaged tells whether both reads answered and neither found an
// organization. Anything else (a read that failed) is not "unmanaged".
func (m ManagementInfo) KnownUnmanaged() bool {
	return m.DomainRead && !m.Domain && m.MDMRead && !m.MDM
}

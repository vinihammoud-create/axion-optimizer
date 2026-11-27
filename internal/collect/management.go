package collect

// management reads whether an organization manages this PC. Domain:
// Win32_ComputerSystem.PartOfDomain ("If True, the computer is part of a
// domain. If the value is NULL, the computer is not in a domain or the status
// is unknown"), so only true or false count as read. MDM:
// IsDeviceRegisteredWithManagement. Both only read; a read that failed stays
// unread and never means "not managed".
func (c *collector) management() {
	m := &c.f.Management
	rows, err := c.p.WMI.Query(`root\cimv2`, "SELECT PartOfDomain FROM Win32_ComputerSystem")
	switch {
	case err != nil:
		c.errf("domínio (Win32_ComputerSystem): %v", err)
	case len(rows) == 0:
		c.errf("domínio (Win32_ComputerSystem): consulta sem resposta")
	default:
		if v, ok := rows[0]["PartOfDomain"].(bool); ok {
			m.DomainRead, m.Domain = true, v
		}
	}
	if on, err := c.p.Native.MDMRegistered(); err != nil {
		c.errf("gerenciamento MDM (IsDeviceRegisteredWithManagement): %v", err)
	} else {
		m.MDMRead, m.MDM = true, on
	}
}

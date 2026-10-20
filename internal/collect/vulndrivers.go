package collect

import (
	"strings"

	"axion/internal/facts"
)

// vulnDrivers looks for known vulnerable kernel drivers left on the PC. It
// only reads: the list of loaded kernel modules and the kernel-driver
// services (Win32_SystemDriver). Either source may fail on its own (the module
// list needs administrator rights); the search counts as read when at least
// one of them was.
func (c *collector) vulnDrivers() {
	v := &c.f.VulnDrivers
	var mods []string
	if m, err := c.p.Native.KernelModules(); err != nil {
		c.errf("Módulos do kernel: %v", err)
	} else if len(m) > 0 {
		mods = m
		v.Read = true
	}
	var svcs []facts.DriverService
	if rows, err := c.p.WMI.Query(`root\cimv2`, "SELECT Name, PathName, State, StartMode FROM Win32_SystemDriver"); err != nil {
		c.errf("WMI Win32_SystemDriver: %v", err)
	} else if len(rows) > 0 {
		v.Read = true
		for _, r := range rows {
			svcs = append(svcs, facts.DriverService{Name: r.Str("Name"), Path: r.Str("PathName"), State: strings.TrimSpace(r.Str("State")), Start: strings.TrimSpace(r.Str("StartMode"))})
		}
	}
	if v.Read {
		v.Found = facts.MatchVulnDrivers(mods, svcs)
	}
}

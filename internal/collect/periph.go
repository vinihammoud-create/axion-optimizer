package collect

import (
	"fmt"
	"strings"

	"axion/internal/facts"
	"axion/internal/periph"
	"axion/internal/sys"
)

const (
	keyMouclass    = `HKLM\SYSTEM\CurrentControlSet\Services\mouclass\Parameters`
	keyKbdclass    = `HKLM\SYSTEM\CurrentControlSet\Services\kbdclass\Parameters`
	keyKernel      = `HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\kernel`
	keyFilterKeys  = `HKCU\Control Panel\Accessibility\Keyboard Response`
	keyEnum        = `HKLM\SYSTEM\CurrentControlSet\Enum\`
	fkfFilterKeyOn = 0x1
)

// peripherals inventories keyboards and mice (USB port, speed, polling
// interval, power management) and the input settings optimizers change.
func (c *collector) peripherals() {
	p := &c.f.Peripherals
	p.MouseQueue = int(c.regInt(keyMouclass, "MouseDataQueueSize", -1))
	p.KeyboardQueue = int(c.regInt(keyKbdclass, "KeyboardDataQueueSize", -1))
	p.ThreadDPC = int(c.regInt(keyKernel, "ThreadDpcEnable", -1))
	p.FilterKeysOn = c.regInt(keyFilterKeys, "Flags", 126)&fkfFilterKeyOn != 0
	p.FilterDelayMS = int(c.regInt(keyFilterKeys, "DelayBeforeAcceptance", 0))
	p.FilterBounceMS = int(c.regInt(keyFilterKeys, "BounceTime", 0))
	p.Readable = true

	var raw []sys.InputDevice
	err := fmt.Errorf("pulado")
	c.part(PartInput, func() { raw, err = c.p.Native.InputDevices() })
	if err != nil {
		p.Err = err.Error()
		c.errf("periféricos: %v", err)
		return
	}
	p.Devices = periph.Group(raw)

	// "Allow the computer to turn off this device to save power" as exposed
	// through WMI; the class is absent when no driver supports it.
	rows, _ := c.p.WMI.Query(`root\WMI`, "SELECT InstanceName, Enable FROM MSPower_DeviceEnable")
	for i := range p.Devices {
		d := &p.Devices[i]
		own := append(append([]string{d.USBNode}, d.Interfaces...), d.Collections...)
		for _, r := range rows {
			inst := wmiInstance(r.Str("InstanceName"))
			for _, n := range own {
				if n != "" && strings.EqualFold(inst, n) {
					d.PowerNodes = append(d.PowerNodes, powerNode(inst, r.Str("InstanceName"), r))
					break
				}
			}
		}
		for _, n := range append([]string{d.USBNode}, d.Interfaces...) {
			if n == "" {
				continue
			}
			v, err := c.p.Reg.GetValue(keyEnum+n+`\Device Parameters`, "SelectiveSuspendEnabled")
			if err != nil || !v.Exists {
				continue
			}
			if regTruthy(v) {
				d.SelSuspend = 1
				d.SuspendNodes = append(d.SuspendNodes, n)
			} else if d.SelSuspend < 0 {
				d.SelSuspend = 0
			}
		}
	}
}

func powerNode(inst, wmiName string, r sys.WMIRow) facts.PowerNode {
	return facts.PowerNode{Instance: inst, WMIName: wmiName, Enabled: r.Bool("Enable")}
}

// wmiInstance strips the "_0" instance suffix WMI appends to device paths.
func wmiInstance(s string) string {
	i := strings.LastIndexByte(s, '_')
	if i < 0 || i == len(s)-1 {
		return s
	}
	for _, ch := range s[i+1:] {
		if ch < '0' || ch > '9' {
			return s
		}
	}
	return s[:i]
}

func regTruthy(v sys.RegValue) bool {
	switch v.Kind {
	case sys.RegBinary:
		for _, b := range v.Bin {
			if b != 0 {
				return true
			}
		}
		return false
	case sys.RegSZ, sys.RegExpandSZ:
		s := strings.TrimSpace(v.Str)
		return s != "" && s != "0"
	}
	return v.Int != 0
}

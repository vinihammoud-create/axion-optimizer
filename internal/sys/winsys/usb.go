//go:build windows

package winsys

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"axion/internal/sys"
)

// Input device inventory: SetupAPI lists the Mouse and Keyboard class
// devices, cfgmgr32 walks each one up to its USB port and host controller,
// and the USB hub IOCTLs (the same ones USBView uses) report the bus speed,
// the device descriptor and the endpoint descriptors (bInterval).

var (
	modCfgmgr32               = windows.NewLazySystemDLL("cfgmgr32.dll")
	procCMLocateDevNodeW      = modCfgmgr32.NewProc("CM_Locate_DevNodeW")
	procCMGetDevNodePropertyW = modCfgmgr32.NewProc("CM_Get_DevNode_PropertyW")
)

func mkGUID(a uint32, b, c uint16, d ...byte) windows.GUID {
	g := windows.GUID{Data1: a, Data2: b, Data3: c}
	copy(g.Data4[:], d)
	return g
}

var (
	guidClassMouse    = mkGUID(0x4d36e96f, 0xe325, 0x11ce, 0xbf, 0xc1, 0x08, 0x00, 0x2b, 0xe1, 0x03, 0x18)
	guidClassKeyboard = mkGUID(0x4d36e96b, 0xe325, 0x11ce, 0xbf, 0xc1, 0x08, 0x00, 0x2b, 0xe1, 0x03, 0x18)
	guidIfaceUSBHub   = mkGUID(0xf18a0e88, 0xc30c, 0x11d0, 0x88, 0x15, 0x00, 0xa0, 0xc9, 0x06, 0xbe, 0xd8)
	fmtDevice         = mkGUID(0xa45c254e, 0xdf1c, 0x4efd, 0x80, 0x20, 0x67, 0xd1, 0x46, 0xa8, 0x50, 0xe0)
	fmtRelations      = mkGUID(0x4340a6c5, 0x93fa, 0x4706, 0x97, 0x2c, 0x7b, 0x64, 0x80, 0x08, 0xa5, 0xa7)
	fmtBusReported    = mkGUID(0x540b947e, 0x8b40, 0x45bc, 0xa8, 0xa2, 0x6a, 0x0b, 0x89, 0x4c, 0xbd, 0xa2)
)

func pkey(g windows.GUID, pid uint32) windows.DEVPROPKEY {
	return windows.DEVPROPKEY{FmtID: windows.DEVPROPGUID(g), PID: windows.DEVPROPID(pid)}
}

var (
	pkDeviceDesc   = pkey(fmtDevice, 2)
	pkManufacturer = pkey(fmtDevice, 13)
	pkFriendlyName = pkey(fmtDevice, 14)
	pkAddress      = pkey(fmtDevice, 30)
	pkParent       = pkey(fmtRelations, 8)
	pkBusReported  = pkey(fmtBusReported, 4)
)

const (
	ioctlUSBGetNodeConnectionInfoEx   = 0x00220448
	ioctlUSBGetDescriptorFromNodeConn = 0x00220410
)

func locateNode(instanceID string) (windows.DEVINST, error) {
	p, err := windows.UTF16PtrFromString(instanceID)
	if err != nil {
		return 0, err
	}
	var inst windows.DEVINST
	r, _, _ := procCMLocateDevNodeW.Call(uintptr(unsafe.Pointer(&inst)), uintptr(unsafe.Pointer(p)), 0)
	if r != 0 {
		return 0, windows.CONFIGRET(r)
	}
	return inst, nil
}

func nodeProperty(inst windows.DEVINST, key *windows.DEVPROPKEY) (windows.DEVPROPTYPE, []byte, error) {
	var typ windows.DEVPROPTYPE
	var size uint32
	r, _, _ := procCMGetDevNodePropertyW.Call(uintptr(inst), uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(&typ)), 0, uintptr(unsafe.Pointer(&size)), 0)
	if r == 0 {
		return typ, nil, nil
	}
	if windows.CONFIGRET(r) != windows.CR_BUFFER_SMALL || size == 0 {
		return 0, nil, windows.CONFIGRET(r)
	}
	buf := make([]byte, size)
	r, _, _ = procCMGetDevNodePropertyW.Call(uintptr(inst), uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0)
	if r != 0 {
		return 0, nil, windows.CONFIGRET(r)
	}
	return typ, buf[:size], nil
}

func nodeString(instanceID string, key windows.DEVPROPKEY) string {
	inst, err := locateNode(instanceID)
	if err != nil {
		return ""
	}
	typ, b, err := nodeProperty(inst, &key)
	if err != nil || typ != windows.DEVPROP_TYPE_STRING || len(b) < 2 {
		return ""
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return windows.UTF16ToString(u)
}

func nodeUint32(instanceID string, key windows.DEVPROPKEY) (uint32, bool) {
	inst, err := locateNode(instanceID)
	if err != nil {
		return 0, false
	}
	typ, b, err := nodeProperty(inst, &key)
	if err != nil || typ != windows.DEVPROP_TYPE_UINT32 || len(b) < 4 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(b), true
}

// parentChain returns the parents of a device, nearest first.
func parentChain(instanceID string) []string {
	var out []string
	cur := instanceID
	for i := 0; i < 16; i++ {
		p := nodeString(cur, pkParent)
		if p == "" || strings.EqualFold(p, `HTREE\ROOT\0`) {
			break
		}
		out = append(out, p)
		cur = p
	}
	return out
}

func classInstances(class *windows.GUID) ([]string, error) {
	set, err := windows.SetupDiGetClassDevsEx(class, "", 0, windows.DIGCF_PRESENT, 0, "")
	if err != nil {
		return nil, err
	}
	defer set.Close()
	var out []string
	for i := 0; i < 512; i++ {
		data, err := set.EnumDeviceInfo(i)
		if err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
				break
			}
			continue
		}
		if id, err := set.DeviceInstanceID(data); err == nil && id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}

func hubInterface(instanceID string) string {
	list, err := windows.CM_Get_Device_Interface_List(instanceID, &guidIfaceUSBHub, windows.CM_GET_DEVICE_INTERFACE_LIST_PRESENT)
	if err != nil || len(list) == 0 {
		return ""
	}
	return list[0]
}

func isRootHub(id string) bool { return strings.HasPrefix(strings.ToUpper(id), `USB\ROOT_HUB`) }

// InputDevices implements sys.Native.
func (n *Native) InputDevices() ([]sys.InputDevice, error) {
	var out []sys.InputDevice
	var firstErr error
	hubs := map[string]string{} // instance -> hub interface path ("" = not a hub)
	isHub := func(id string) string {
		if p, ok := hubs[id]; ok {
			return p
		}
		p := hubInterface(id)
		hubs[id] = p
		return p
	}
	for _, class := range []struct {
		guid *windows.GUID
		kind string
	}{{&guidClassMouse, "mouse"}, {&guidClassKeyboard, "keyboard"}} {
		ids, err := classInstances(class.guid)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, id := range ids {
			d := sys.InputDevice{InstanceID: id, Kind: class.kind, Speed: sys.USBSpeedUnknown}
			d.Name = nodeString(id, pkFriendlyName)
			if d.Name == "" {
				d.Name = nodeString(id, pkDeviceDesc)
			}
			d.Manufacturer = nodeString(id, pkManufacturer)
			d.Chain = parentChain(id)
			n.describeUSB(&d, isHub)
			out = append(out, d)
		}
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

// describeUSB fills the USB port details of a collection.
func (n *Native) describeUSB(d *sys.InputDevice, isHub func(string) string) {
	nodes := d.Chain
	hubIdx := -1
	for i, id := range nodes {
		up := strings.ToUpper(id)
		if strings.HasPrefix(up, `HID\`) && i == 0 {
			continue // tolerate a HID parent right above the collection
		}
		// A USB input device is an unbroken run of USB\ nodes up to the
		// port. Anything else (BTHENUM, BTHLEDEVICE, ACPI...) means the
		// device is not USB even if the chain later reaches a USB radio.
		if !strings.HasPrefix(up, `USB\`) || isRootHub(id) {
			break
		}
		if i+1 < len(nodes) && isHub(nodes[i+1]) != "" {
			d.USBNode, d.Hub, hubIdx = id, nodes[i+1], i+1
			break
		}
		if strings.Contains(up, "&MI_") && d.USBInterface == "" {
			d.USBInterface = id
		}
	}
	if d.USBNode == "" {
		return
	}
	d.HubIsRoot = isRootHub(d.Hub)
	for _, id := range nodes[hubIdx:] {
		if strings.HasPrefix(strings.ToUpper(id), `PCI\`) {
			d.ControllerID = id
			d.Controller = nodeString(id, pkFriendlyName)
			if d.Controller == "" {
				d.Controller = nodeString(id, pkDeviceDesc)
			}
			break
		}
		if isHub(id) != "" && !isRootHub(id) {
			d.HubDepth++
		}
	}
	d.Product = nodeString(d.USBNode, pkBusReported)
	if d.Product == "" && d.USBInterface != "" {
		d.Product = nodeString(d.USBInterface, pkBusReported)
	}
	port, ok := nodeUint32(d.USBNode, pkAddress)
	if !ok || port == 0 {
		d.Err = "porta USB não informada pelo driver"
		return
	}
	d.Port = int(port)
	if err := hubQuery(isHub(d.Hub), d); err != nil {
		d.Err = err.Error()
	}
}

// hubQuery asks the hub driver about the device on the port.
func hubQuery(path string, d *sys.InputDevice) error {
	if path == "" {
		return errors.New("hub sem interface de consulta")
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return fmt.Errorf("abrir hub: %w", err)
	}
	defer windows.CloseHandle(h)

	buf := make([]byte, sys.NodeConnInfoExSize+32*sys.PipeInfoSize)
	binary.LittleEndian.PutUint32(buf, uint32(d.Port))
	var got uint32
	if err := windows.DeviceIoControl(h, ioctlUSBGetNodeConnectionInfoEx, &buf[0], uint32(len(buf)), &buf[0], uint32(len(buf)), &got, nil); err != nil {
		return fmt.Errorf("consulta da porta: %w", err)
	}
	conn, ok := sys.ParseNodeConnection(buf, int(got))
	if !ok || conn.Status != 1 {
		return errors.New("porta sem dispositivo conectado")
	}
	d.Speed, d.BcdUSB = conn.Speed, conn.BcdUSB
	d.Endpoints = conn.OpenPipes

	// Full configuration descriptor: ties each endpoint to its interface.
	req := make([]byte, sys.DescriptorRequestSize+4096)
	binary.LittleEndian.PutUint32(req[0:], uint32(d.Port))
	req[4], req[5] = 0x80, 0x06 // standard device-to-host GET_DESCRIPTOR
	binary.LittleEndian.PutUint16(req[6:], 0x0200)
	binary.LittleEndian.PutUint16(req[10:], uint16(len(req)-sys.DescriptorRequestSize))
	if err := windows.DeviceIoControl(h, ioctlUSBGetDescriptorFromNodeConn, &req[0], uint32(len(req)), &req[0], uint32(len(req)), &got, nil); err == nil && int(got) > sys.DescriptorRequestSize {
		if eps := sys.ParseConfigDescriptor(req[sys.DescriptorRequestSize:got]); len(eps) > 0 {
			d.Endpoints = eps
		}
	}
	return nil
}

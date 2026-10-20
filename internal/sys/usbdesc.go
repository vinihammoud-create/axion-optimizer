package sys

import "encoding/binary"

// Sizes and offsets of the packed structures returned by the USB hub IOCTLs
// (usbioctl.h and usbspec.h are declared with 1-byte packing).
const (
	// USB_NODE_CONNECTION_INFORMATION_EX without the variable PipeList.
	NodeConnInfoExSize = 35
	// USB_PIPE_INFO: USB_ENDPOINT_DESCRIPTOR (7) + ScheduleOffset (4).
	PipeInfoSize = 11
	// USB_DESCRIPTOR_REQUEST header: ConnectionIndex (4) + SetupPacket (8).
	DescriptorRequestSize = 12
)

// NodeConnection is the decoded USB_NODE_CONNECTION_INFORMATION_EX.
type NodeConnection struct {
	BcdUSB     uint16
	VendorID   uint16
	ProductID  uint16
	Speed      int
	IsHub      bool
	Address    uint16
	Status     uint32 // USB_CONNECTION_STATUS (1 = DeviceConnected)
	OpenPipes  []USBEndpoint
	ConfigNum  uint8
	DeviceDesc []byte // raw 18-byte device descriptor
}

// ParseNodeConnection decodes the output buffer of
// IOCTL_USB_GET_NODE_CONNECTION_INFORMATION_EX (n = bytes returned).
func ParseNodeConnection(b []byte, n int) (NodeConnection, bool) {
	var c NodeConnection
	if n > len(b) {
		n = len(b)
	}
	if n < NodeConnInfoExSize {
		return c, false
	}
	le := binary.LittleEndian
	dd := b[4:22]
	c.DeviceDesc = append([]byte(nil), dd...)
	c.BcdUSB = le.Uint16(dd[2:4])
	c.VendorID = le.Uint16(dd[8:10])
	c.ProductID = le.Uint16(dd[10:12])
	c.ConfigNum = b[22]
	c.Speed = int(b[23])
	c.IsHub = b[24] != 0
	c.Address = le.Uint16(b[25:27])
	pipes := int(le.Uint32(b[27:31]))
	c.Status = le.Uint32(b[31:35])
	for i := 0; i < pipes; i++ {
		off := NodeConnInfoExSize + i*PipeInfoSize
		if off+PipeInfoSize > n {
			break
		}
		ep := b[off : off+7]
		c.OpenPipes = append(c.OpenPipes, USBEndpoint{
			Interface: -1, Class: -1, Protocol: -1,
			Address: ep[2], Attributes: ep[3], MaxPacket: le.Uint16(ep[4:6]), Interval: ep[6],
		})
	}
	return c, true
}

// ParseConfigDescriptor walks a configuration descriptor and returns every
// endpoint of alternate setting 0 with the interface it belongs to.
func ParseConfigDescriptor(b []byte) []USBEndpoint {
	var out []USBEndpoint
	iface, class, proto, alt := -1, -1, -1, 0
	for i := 0; i+2 <= len(b); {
		l := int(b[i])
		if l < 2 || i+l > len(b) {
			break
		}
		switch b[i+1] {
		case 0x04: // INTERFACE
			if l >= 9 {
				iface, alt, class, proto = int(b[i+2]), int(b[i+3]), int(b[i+5]), int(b[i+7])
			}
		case 0x05: // ENDPOINT
			if l >= 7 && alt == 0 {
				out = append(out, USBEndpoint{
					Interface: iface, Class: class, Protocol: proto,
					Address: b[i+2], Attributes: b[i+3],
					MaxPacket: binary.LittleEndian.Uint16(b[i+4 : i+6]), Interval: b[i+6],
				})
			}
		}
		i += l
	}
	return out
}

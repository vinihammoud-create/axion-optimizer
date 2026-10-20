package etw

import (
	"encoding/binary"
	"runtime"
	"testing"
	"unicode/utf16"
	"unsafe"
)

// The native trace code passes these structures straight to advapi32, so
// their layout must match the Windows SDK for 64-bit builds exactly.
func TestLayoutsMatchWindowsSDK(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("layouts pinned for 64-bit")
	}
	checks := []struct {
		name      string
		got, want uintptr
	}{
		{"sizeof(WNODE_HEADER)", unsafe.Sizeof(WnodeHeader{}), 48},
		{"sizeof(EVENT_TRACE_PROPERTIES)", unsafe.Sizeof(TraceProperties{}), 120},
		{"EVENT_TRACE_PROPERTIES.LoggerThreadId", unsafe.Offsetof(TraceProperties{}.LoggerThreadID), 104},
		{"EVENT_TRACE_PROPERTIES.LoggerNameOffset", unsafe.Offsetof(TraceProperties{}.LoggerNameOffset), 116},
		{"sizeof(EVENT_TRACE_HEADER)", unsafe.Sizeof(EventTraceHeader{}), 48},
		{"sizeof(EVENT_TRACE)", unsafe.Sizeof(EventTrace{}), 88},
		{"sizeof(TIME_ZONE_INFORMATION)", unsafe.Sizeof(TimeZoneInformation{}), 172},
		{"sizeof(TRACE_LOGFILE_HEADER)", unsafe.Sizeof(TraceLogfileHeader{}), 280},
		{"TRACE_LOGFILE_HEADER.BootTime", unsafe.Offsetof(TraceLogfileHeader{}.BootTime), 248},
		{"EVENT_TRACE_LOGFILEW.CurrentEvent", unsafe.Offsetof(EventTraceLogfile{}.CurrentEvent), 32},
		{"EVENT_TRACE_LOGFILEW.LogfileHeader", unsafe.Offsetof(EventTraceLogfile{}.LogfileHeader), 120},
		{"EVENT_TRACE_LOGFILEW.BufferCallback", unsafe.Offsetof(EventTraceLogfile{}.BufferCallback), 400},
		{"EVENT_TRACE_LOGFILEW.EventRecordCallback", unsafe.Offsetof(EventTraceLogfile{}.EventRecordCallback), 424},
		{"EVENT_TRACE_LOGFILEW.Context", unsafe.Offsetof(EventTraceLogfile{}.Context), 440},
		{"sizeof(EVENT_TRACE_LOGFILEW)", unsafe.Sizeof(EventTraceLogfile{}), 448},
		{"sizeof(EVENT_DESCRIPTOR)", unsafe.Sizeof(EventDescriptor{}), 16},
		{"EVENT_HEADER.TimeStamp", unsafe.Offsetof(EventHeader{}.TimeStamp), 16},
		{"EVENT_HEADER.ProviderId", unsafe.Offsetof(EventHeader{}.ProviderID), 24},
		{"EVENT_HEADER.EventDescriptor", unsafe.Offsetof(EventHeader{}.Descriptor), 40},
		{"sizeof(EVENT_HEADER)", unsafe.Sizeof(EventHeader{}), 80},
		{"EVENT_RECORD.UserDataLength", unsafe.Offsetof(EventRecord{}.UserDataLength), 86},
		{"EVENT_RECORD.UserData", unsafe.Offsetof(EventRecord{}.UserData), 96},
		{"sizeof(EVENT_RECORD)", unsafe.Sizeof(EventRecord{}), 112},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d, esperado %d", c.name, c.got, c.want)
		}
	}
}

func TestParseURB(t *testing.T) {
	b := make([]byte, 5*8+2)
	for i, v := range []uint64{0xC0, 0xDE, 0xB1, 0x12, 0x5A} {
		binary.LittleEndian.PutUint64(b[i*8:], v)
	}
	dev, pipe, urb, ok := ParseURB(b, 8)
	if !ok || dev != 0xDE || pipe != 0xB1 || urb != 0x5A {
		t.Errorf("64 bits: %x %x %x %v", dev, pipe, urb, ok)
	}
	if _, _, _, ok := ParseURB(b[:30], 8); ok {
		t.Error("payload curto deveria falhar")
	}
	b32 := make([]byte, 5*4)
	for i, v := range []uint32{1, 2, 3, 4, 5} {
		binary.LittleEndian.PutUint32(b32[i*4:], v)
	}
	if dev, pipe, urb, ok := ParseURB(b32, 4); !ok || dev != 2 || pipe != 3 || urb != 5 {
		t.Errorf("32 bits: %d %d %d", dev, pipe, urb)
	}
}

func wstr(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(u)+2)
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return b
}

func TestParseHubDevice(t *testing.T) {
	var b []byte
	put := func(v uint64) {
		x := make([]byte, 8)
		binary.LittleEndian.PutUint64(x, v)
		b = append(b, x...)
	}
	put(0x1111)   // fid_HubDevice
	put(0xABCDEF) // fid_UsbDevice
	b = append(b, 3, 0, 0, 0)
	b = append(b, wstr("Dispositivo Composto USB")...)
	b = append(b, wstr(`\\?\USB#VID_046D&PID_C547#5&2f5e1c3&0&3#{a5dcbf10-6530-11d2-901f-00c04fb951ed}`)...)
	b = append(b, 0x12, 0x01, 0x00, 0x02) // start of the device descriptor (ignored)
	h, port, desc, path, ok := ParseHubDevice(b, 8)
	if !ok || h != 0xABCDEF || port != 3 || desc != "Dispositivo Composto USB" || path[:21] != `\\?\USB#VID_046D&PID_` {
		t.Errorf("rundown: %x %d %q %q %v", h, port, desc, path, ok)
	}
	if _, _, _, _, ok := ParseHubDevice(b[:30], 8); ok {
		t.Error("string sem terminador deveria falhar")
	}
}

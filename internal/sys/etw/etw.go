// Package etw holds the Event Tracing for Windows structures and the event
// decoders used by the USB polling trace. The structures mirror the Windows
// SDK layouts (evntrace.h, evntcons.h) for 64-bit Windows; the layout tests
// in this package pin every size and offset the native code relies on.
//
// Providers and events (from the providers' instrumentation manifests):
//   - Microsoft-Windows-USB-UCX {36DA592D-E43A-4E28-AF6F-4BC57C5A11E8}:
//     task URB_FUNCTION_BULK_OR_INTERRUPT_TRANSFER (21), opcodes win:Start /
//     win:Stop, keyword HeadersBusTrace (0x40). Payload starts with the
//     pointers fid_UcxController, fid_UsbDevice, fid_PipeHandle, fid_IRP_Ptr,
//     fid_URB_Ptr.
//   - Microsoft-Windows-USB-USBHUB3 {AC52AD17-CC01-4F85-8DF5-4DCE4333C99B}:
//     rundown event 6 (keywords Default|Rundown) with fid_HubDevice,
//     fid_UsbDevice, fid_PortNumber, fid_DeviceDescription and
//     fid_DeviceInterfacePath, which ties the UCX device handle to VID/PID.
package etw

import (
	"encoding/binary"
	"unicode/utf16"
	"unsafe"
)

// GUID matches the Windows GUID layout.
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// Provider GUIDs.
var (
	ProviderUCX     = GUID{0x36da592d, 0xe43a, 0x4e28, [8]byte{0xaf, 0x6f, 0x4b, 0xc5, 0x7c, 0x5a, 0x11, 0xe8}}
	ProviderUSBHUB3 = GUID{0xac52ad17, 0xcc01, 0x4f85, [8]byte{0x8d, 0xf5, 0x4d, 0xce, 0x43, 0x33, 0xc9, 0x9b}}
)

// Event identifiers and keywords.
const (
	TaskBulkOrInterrupt    = 21 // URB_FUNCTION_BULK_OR_INTERRUPT_TRANSFER
	OpcodeStart            = 1  // win:Start
	OpcodeStop             = 2  // win:Stop
	KeywordHeadersBusTrace = 0x40
	KeywordRundown         = 0x8000
	KeywordPnP             = 0x10
	EventHubDeviceRundown  = 6  // USB3.0HubDriverRundownInformation6
	EventHubPrepareHW      = 43 // PrepareHardwareInformation43 (same payload)
	LevelInformation       = 4
)

// Flags and modes.
const (
	WnodeFlagTracedGUID         = 0x00020000
	EventTraceRealTimeMode      = 0x00000100
	ProcessTraceModeRealTime    = 0x00000100
	ProcessTraceModeRawTime     = 0x00001000
	ProcessTraceModeEventRecord = 0x10000000
	EventTraceControlStop       = 1
	ControlCodeDisableProvider  = 0
	ControlCodeEnableProvider   = 1
	ControlCodeCaptureState     = 2
	EventHeaderFlag32BitHeader  = 0x0020
	ClientContextQPC            = 1
)

// WnodeHeader is WNODE_HEADER.
type WnodeHeader struct {
	BufferSize        uint32
	ProviderID        uint32
	HistoricalContext uint64
	TimeStamp         int64
	GUID              GUID
	ClientContext     uint32
	Flags             uint32
}

// TraceProperties is EVENT_TRACE_PROPERTIES (names follow in the buffer).
type TraceProperties struct {
	Wnode               WnodeHeader
	BufferSize          uint32
	MinimumBuffers      uint32
	MaximumBuffers      uint32
	MaximumFileSize     uint32
	LogFileMode         uint32
	FlushTimer          uint32
	EnableFlags         uint32
	AgeLimit            int32
	NumberOfBuffers     uint32
	FreeBuffers         uint32
	EventsLost          uint32
	BuffersWritten      uint32
	LogBuffersLost      uint32
	RealTimeBuffersLost uint32
	LoggerThreadID      uintptr
	LogFileNameOffset   uint32
	LoggerNameOffset    uint32
}

// EventTraceHeader is EVENT_TRACE_HEADER.
type EventTraceHeader struct {
	Size           uint16
	FieldTypeFlags uint16
	Version        uint32
	ThreadID       uint32
	ProcessID      uint32
	TimeStamp      int64
	GUID           GUID
	ProcessorTime  uint64
}

// EventTrace is EVENT_TRACE.
type EventTrace struct {
	Header           EventTraceHeader
	InstanceID       uint32
	ParentInstanceID uint32
	ParentGUID       GUID
	MofData          uintptr
	MofLength        uint32
	ClientContext    uint32
}

// SystemTime is SYSTEMTIME.
type SystemTime struct {
	Year, Month, DayOfWeek, Day, Hour, Minute, Second, Milliseconds uint16
}

// TimeZoneInformation is TIME_ZONE_INFORMATION.
type TimeZoneInformation struct {
	Bias         int32
	StandardName [32]uint16
	StandardDate SystemTime
	StandardBias int32
	DaylightName [32]uint16
	DaylightDate SystemTime
	DaylightBias int32
}

// TraceLogfileHeader is TRACE_LOGFILE_HEADER.
type TraceLogfileHeader struct {
	BufferSize         uint32
	Version            uint32
	ProviderVersion    uint32
	NumberOfProcessors uint32
	EndTime            int64
	TimerResolution    uint32
	MaximumFileSize    uint32
	LogFileMode        uint32
	BuffersWritten     uint32
	LogInstanceGUID    GUID
	LoggerName         *uint16
	LogFileName        *uint16
	TimeZone           TimeZoneInformation
	BootTime           int64
	PerfFreq           int64
	StartTime          int64
	ReservedFlags      uint32
	BuffersLost        uint32
}

// EventTraceLogfile is EVENT_TRACE_LOGFILEW.
type EventTraceLogfile struct {
	LogFileName         *uint16
	LoggerName          *uint16
	CurrentTime         int64
	BuffersRead         uint32
	ProcessTraceMode    uint32
	CurrentEvent        EventTrace
	LogfileHeader       TraceLogfileHeader
	BufferCallback      uintptr
	BufferSize          uint32
	Filled              uint32
	EventsLost          uint32
	EventRecordCallback uintptr
	IsKernelTrace       uint32
	Context             uintptr
}

// EventDescriptor is EVENT_DESCRIPTOR.
type EventDescriptor struct {
	ID      uint16
	Version uint8
	Channel uint8
	Level   uint8
	Opcode  uint8
	Task    uint16
	Keyword uint64
}

// EventHeader is EVENT_HEADER.
type EventHeader struct {
	Size          uint16
	HeaderType    uint16
	Flags         uint16
	EventProperty uint16
	ThreadID      uint32
	ProcessID     uint32
	TimeStamp     int64
	ProviderID    GUID
	Descriptor    EventDescriptor
	ProcessorTime uint64
	ActivityID    GUID
}

// EventRecord is EVENT_RECORD.
type EventRecord struct {
	Header            EventHeader
	ProcessorNumber   uint8
	Alignment         uint8
	LoggerID          uint16
	ExtendedDataCount uint16
	UserDataLength    uint16
	ExtendedData      unsafe.Pointer
	UserData          unsafe.Pointer // payload inside the ETW buffer
	UserContext       unsafe.Pointer
}

// PointerSize returns the size of pointers in the event payload.
func (h *EventHeader) PointerSize() int {
	if h.Flags&EventHeaderFlag32BitHeader != 0 {
		return 4
	}
	return 8
}

func readPtr(b []byte, off, size int) (uint64, bool) {
	if off+size > len(b) {
		return 0, false
	}
	if size == 4 {
		return uint64(binary.LittleEndian.Uint32(b[off:])), true
	}
	return binary.LittleEndian.Uint64(b[off:]), true
}

// ParseURB decodes the leading pointers of a UCX transfer event.
func ParseURB(b []byte, ptr int) (device, pipe, urb uint64, ok bool) {
	if ptr != 4 && ptr != 8 {
		return 0, 0, 0, false
	}
	var ok1, ok2, ok3 bool
	device, ok1 = readPtr(b, 1*ptr, ptr)
	pipe, ok2 = readPtr(b, 2*ptr, ptr)
	urb, ok3 = readPtr(b, 4*ptr, ptr)
	return device, pipe, urb, ok1 && ok2 && ok3
}

// readWString reads a NUL-terminated UTF-16LE string starting at off and
// returns it with the offset after the terminator.
func readWString(b []byte, off int) (string, int, bool) {
	var u []uint16
	for i := off; i+1 < len(b); i += 2 {
		c := binary.LittleEndian.Uint16(b[i:])
		if c == 0 {
			return string(utf16.Decode(u)), i + 2, true
		}
		u = append(u, c)
	}
	return "", len(b), false
}

// ParseHubDevice decodes the device rundown of Microsoft-Windows-USB-USBHUB3
// (event 6 and 43): the UCX device handle, the hub port, the description and
// the device interface path (\\?\USB#VID_xxxx&PID_yyyy#...).
func ParseHubDevice(b []byte, ptr int) (handle uint64, port int, desc, path string, ok bool) {
	if ptr != 4 && ptr != 8 {
		return 0, 0, "", "", false
	}
	h, ok1 := readPtr(b, ptr, ptr)
	off := 2 * ptr
	if !ok1 || off+4 > len(b) {
		return 0, 0, "", "", false
	}
	port = int(binary.LittleEndian.Uint32(b[off:]))
	off += 4
	desc, off, ok = readWString(b, off)
	if !ok {
		return 0, 0, "", "", false
	}
	path, _, ok = readWString(b, off)
	if !ok {
		return 0, 0, "", "", false
	}
	return h, port, desc, path, true
}

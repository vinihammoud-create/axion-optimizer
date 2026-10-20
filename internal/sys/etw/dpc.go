package etw

import "encoding/binary"

// Kernel DPC and ISR events, from the "NT Kernel Logger" session.
//
// Sources (Microsoft Learn): the PerfInfo class
// {CE1DBFB4-137E-4DA6-87B0-3F59AA102CBC}; the DPC class (EventType 66
// ThreadDPC, 68 DPC, 69 TimerDPC) and the ISR class (EventType 67), whose
// payload starts with InitialTime (the entry time, 8 bytes) followed by Routine
// (the routine's address, pointer sized); and "Example 15: Measuring DPC/ISR
// Time", which enables the session with the DPC and interrupt flags, asks for
// the performance counter clock and notes that the event's time stamp is the
// END of the routine, so the duration is TimeStamp - InitialTime. With the
// QueryPerformanceCounter clock (ClientContext 1) and raw time stamps both
// values are in the same ticks.
const (
	// EnableFlags of EVENT_TRACE_PROPERTIES for the NT Kernel Logger.
	FlagDPC       = 0x00000020 // EVENT_TRACE_FLAG_DPC
	FlagInterrupt = 0x00000040 // EVENT_TRACE_FLAG_INTERRUPT

	// KernelLoggerName is the only session name that carries kernel events.
	KernelLoggerName = "NT Kernel Logger"

	// PerfInfo event types (EventHeader.EventDescriptor.Opcode).
	OpThreadedDPC = 66
	OpISR         = 67
	OpDPC         = 68
	OpTimerDPC    = 69
)

// Kernel GUIDs.
var (
	// SystemTraceControlGUID is the NT Kernel Logger's session GUID.
	SystemTraceControlGUID = GUID{0x9e814aad, 0x3204, 0x11d2, [8]byte{0x9a, 0x82, 0x00, 0x60, 0x08, 0xa8, 0x69, 0x39}}
	// ProviderPerfInfo is the class GUID of the DPC and ISR events.
	ProviderPerfInfo = GUID{0xce1dbfb4, 0x137e, 0x4da6, [8]byte{0x87, 0xb0, 0x3f, 0x59, 0xaa, 0x10, 0x2c, 0xbc}}
)

// ParseRoutine decodes the payload shared by the DPC and ISR events: the time
// the routine was entered (in the session's clock ticks) and its address.
func ParseRoutine(b []byte, ptr int) (entry, routine uint64, ok bool) {
	if ptr != 4 && ptr != 8 {
		return 0, 0, false
	}
	if len(b) < 8+ptr {
		return 0, 0, false
	}
	entry = binary.LittleEndian.Uint64(b)
	routine, ok = readPtr(b, 8, ptr)
	return entry, routine, ok
}

// IsRoutineEvent reports whether the opcode is a DPC or ISR entry that
// ParseRoutine understands.
func IsRoutineEvent(opcode uint8) (dpc, isr bool) {
	switch opcode {
	case OpDPC, OpTimerDPC, OpThreadedDPC:
		return true, false
	case OpISR:
		return false, true
	}
	return false, false
}

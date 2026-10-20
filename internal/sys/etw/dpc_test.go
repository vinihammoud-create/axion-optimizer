package etw

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

func guidString(g GUID) string {
	return strings.ToLower(fmt.Sprintf("{%08x-%04x-%04x-%02x%02x-%02x%02x%02x%02x%02x%02x}",
		g.Data1, g.Data2, g.Data3, g.Data4[0], g.Data4[1], g.Data4[2], g.Data4[3], g.Data4[4], g.Data4[5], g.Data4[6], g.Data4[7]))
}

// The GUIDs and flags come from Microsoft Learn (the PerfInfo class page and
// the EVENT_TRACE_PROPERTIES documentation); a typo would silently make the
// tracer see nothing.
func TestKernelConstants(t *testing.T) {
	if got := guidString(ProviderPerfInfo); got != "{ce1dbfb4-137e-4da6-87b0-3f59aa102cbc}" {
		t.Errorf("PerfInfo: %s", got)
	}
	if got := guidString(SystemTraceControlGUID); got != "{9e814aad-3204-11d2-9a82-006008a86939}" {
		t.Errorf("SystemTraceControl: %s", got)
	}
	if FlagDPC != 0x20 || FlagInterrupt != 0x40 {
		t.Errorf("flags: dpc=%#x interrupt=%#x", FlagDPC, FlagInterrupt)
	}
	if OpISR != 67 || OpDPC != 68 || OpThreadedDPC != 66 || OpTimerDPC != 69 {
		t.Error("event types do not match the DPC and ISR class pages")
	}
	if KernelLoggerName != "NT Kernel Logger" {
		t.Errorf("session name: %q", KernelLoggerName)
	}
}

func TestParseRoutine(t *testing.T) {
	// 64-bit payload: InitialTime, then a kernel address.
	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b[0:], 123456789)
	binary.LittleEndian.PutUint64(b[8:], 0xfffff80412345678)
	entry, routine, ok := ParseRoutine(b, 8)
	if !ok || entry != 123456789 || routine != 0xfffff80412345678 {
		t.Errorf("64 bits: %d %#x %v", entry, routine, ok)
	}
	// ISR payloads carry more fields after the routine (ReturnValue, Vector...).
	isr := append(append([]byte{}, b...), 1, 0x30, 0x00, 0, 0, 0, 0)
	if e, r, ok := ParseRoutine(isr, 8); !ok || e != 123456789 || r != 0xfffff80412345678 {
		t.Errorf("ISR com campos extras: %d %#x %v", e, r, ok)
	}
	// 32-bit payload: the routine is 4 bytes.
	b32 := make([]byte, 12)
	binary.LittleEndian.PutUint64(b32[0:], 42)
	binary.LittleEndian.PutUint32(b32[8:], 0x80123456)
	if e, r, ok := ParseRoutine(b32, 4); !ok || e != 42 || r != 0x80123456 {
		t.Errorf("32 bits: %d %#x %v", e, r, ok)
	}
	// Truncated and invalid sizes are refused instead of read past the end.
	for _, c := range []struct {
		name string
		b    []byte
		ptr  int
	}{
		{"curto", b[:15], 8}, {"sem rotina", b[:8], 8}, {"vazio", nil, 8}, {"ponteiro 3", b, 3}, {"ponteiro 0", b, 0},
	} {
		if _, _, ok := ParseRoutine(c.b, c.ptr); ok {
			t.Errorf("%s: aceito", c.name)
		}
	}
}

// moduleEntry builds one RTL_PROCESS_MODULE_INFORMATION (296 bytes).
func moduleEntry(base uint64, size uint32, path string, fileOff uint16) []byte {
	e := make([]byte, 296)
	binary.LittleEndian.PutUint64(e[16:], base)
	binary.LittleEndian.PutUint32(e[24:], size)
	binary.LittleEndian.PutUint16(e[38:], fileOff)
	copy(e[40:], path)
	return e
}

func TestParseModulesAndLookup(t *testing.T) {
	buf := make([]byte, 8)
	entries := [][]byte{
		moduleEntry(0xfffff80400000000, 0xa00000, `\SystemRoot\system32\ntoskrnl.exe`, 21),
		moduleEntry(0xfffff80412000000, 0x80000, `\SystemRoot\System32\drivers\ndis.sys`, 29),
		moduleEntry(0xfffff80413000000, 0x30000, `\SystemRoot\System32\drivers\nvlddmkm.sys`, 29),
		moduleEntry(0, 0x1000, `\skipped.sys`, 1), // no base: ignored
	}
	binary.LittleEndian.PutUint32(buf, uint32(len(entries)))
	for _, e := range entries {
		buf = append(buf, e...)
	}
	mods := ParseModules(buf)
	if len(mods) != 3 || mods[0].Name != "ntoskrnl.exe" || mods[1].Name != "ndis.sys" || mods[2].Name != "nvlddmkm.sys" {
		t.Fatalf("módulos: %+v", mods)
	}
	m := NewModuleMap(mods, mods[:1]) // a repeated list changes nothing
	for addr, want := range map[uint64]string{
		0xfffff80400000000: "ntoskrnl.exe", 0xfffff804009fffff: "ntoskrnl.exe",
		0xfffff80412000010: "ndis.sys", 0xfffff8041207ffff: "ndis.sys",
		0xfffff80413020000: "nvlddmkm.sys",
		0xfffff80400a00000: "", // just past the kernel image, before any driver
		0xfffff80412080000: "", // one past the end of ndis.sys
		0x1000:             "", // below every module
		0xffffffffffffffff: "", // above every module
	} {
		if got := m.Lookup(addr); got != want {
			t.Errorf("Lookup(%#x) = %q, esperado %q", addr, got, want)
		}
	}
	// A truncated buffer yields only the entries that fit.
	if got := ParseModules(buf[:8+296+100]); len(got) != 1 {
		t.Errorf("buffer truncado: %d módulos", len(got))
	}
	if ParseModules(nil) != nil || ParseModules([]byte{1, 0}) != nil {
		t.Error("buffer vazio ou curto deveria dar nenhuma lista")
	}
}

func TestIsRoutineEvent(t *testing.T) {
	for op, want := range map[uint8][2]bool{
		OpDPC: {true, false}, OpTimerDPC: {true, false}, OpThreadedDPC: {true, false}, OpISR: {false, true}, 0: {false, false}, 51: {false, false}, 70: {false, false},
	} {
		dpc, isr := IsRoutineEvent(op)
		if dpc != want[0] || isr != want[1] {
			t.Errorf("opcode %d: dpc=%v isr=%v", op, dpc, isr)
		}
	}
}

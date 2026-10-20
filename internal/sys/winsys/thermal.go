//go:build windows

package winsys

import (
	"fmt"
	"unsafe"

	"axion/internal/sys"

	"golang.org/x/sys/windows"
)

// modPowrprof is declared in power.go.
var procCallNtPowerInformation = modPowrprof.NewProc("CallNtPowerInformation")

const (
	powerInfoProcessorInformation = 11 // POWER_INFORMATION_LEVEL ProcessorInformation
	statusBufferTooSmall          = 0xC0000023
	allProcessorGroups            = 0xFFFF
)

// processorPowerInformation is PROCESSOR_POWER_INFORMATION (left out of
// WinNT.h; Microsoft documents the layout on its own page).
type processorPowerInformation struct {
	Number           uint32
	MaxMhz           uint32
	CurrentMhz       uint32
	MhzLimit         uint32
	MaxIdleState     uint32
	CurrentIdleState uint32
}

// ProcessorPower implements sys.Native.
func (n *Native) ProcessorPower() ([]sys.ProcPower, error) {
	if err := procCallNtPowerInformation.Find(); err != nil {
		return nil, err
	}
	count := int(windows.GetActiveProcessorCount(allProcessorGroups))
	if count <= 0 {
		count = 64
	}
	for try := 0; try < 4; try++ {
		buf := make([]processorPowerInformation, count)
		size := uintptr(len(buf)) * unsafe.Sizeof(buf[0])
		r, _, _ := procCallNtPowerInformation.Call(powerInfoProcessorInformation, 0, 0, uintptr(unsafe.Pointer(&buf[0])), size)
		if r == statusBufferTooSmall {
			count *= 2
			continue
		}
		if r != 0 {
			return nil, fmt.Errorf("CallNtPowerInformation: 0x%x", r)
		}
		out := make([]sys.ProcPower, 0, len(buf))
		for _, p := range buf {
			if p.MaxMhz == 0 {
				continue // slots past the processors that exist
			}
			out = append(out, sys.ProcPower{MaxMhz: int(p.MaxMhz), CurrentMhz: int(p.CurrentMhz), MhzLimit: int(p.MhzLimit)})
		}
		return out, nil
	}
	return nil, fmt.Errorf("CallNtPowerInformation: buffer pequeno demais")
}

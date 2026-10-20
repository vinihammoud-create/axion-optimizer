//go:build windows

package winsys

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CPU frequency through the performance counters (PDH). The counter is asked
// for by its English name (PdhAddEnglishCounter), so it works whatever the
// language of Windows: the localized names ("Informações do processador")
// are what make typeperf unusable here.
//
// "\Processor Information(_Total)\% of Maximum Frequency" is the processors'
// frequency as a share of their maximum (the base frequency in Windows'
// terms), so it goes above 100 with turbo and below it when the processor is
// held back by heat or by a power limit.

var (
	modPdh                          = windows.NewLazySystemDLL("pdh.dll")
	procPdhOpenQuery                = modPdh.NewProc("PdhOpenQueryW")
	procPdhAddEnglishCounter        = modPdh.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData         = modPdh.NewProc("PdhCollectQueryData")
	procPdhGetFormattedCounterValue = modPdh.NewProc("PdhGetFormattedCounterValue")
)

const (
	pdhFmtDouble     = 0x00000200
	pdhFmtNoCap100   = 0x00008000
	pdhCStatusValid  = 0
	pdhCStatusNewDat = 1
)

// pdhFmtCounterValue is PDH_FMT_COUNTERVALUE with the double member.
type pdhFmtCounterValue struct {
	CStatus uint32
	_       uint32
	Double  float64
}

var cpuFreq struct {
	mu      sync.Mutex
	query   uintptr
	counter uintptr
	err     error
	opened  bool
	primed  bool
}

func openCPUFreq() error {
	if cpuFreq.opened {
		return cpuFreq.err
	}
	cpuFreq.opened = true
	for _, p := range []*windows.LazyProc{procPdhOpenQuery, procPdhAddEnglishCounter, procPdhCollectQueryData, procPdhGetFormattedCounterValue} {
		if err := p.Find(); err != nil {
			cpuFreq.err = err
			return err
		}
	}
	if r, _, _ := procPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&cpuFreq.query))); r != 0 {
		cpuFreq.err = fmt.Errorf("PdhOpenQuery: 0x%x", r)
		return cpuFreq.err
	}
	path, _ := windows.UTF16PtrFromString(`\Processor Information(_Total)\% of Maximum Frequency`)
	if r, _, _ := procPdhAddEnglishCounter.Call(cpuFreq.query, uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&cpuFreq.counter))); r != 0 {
		cpuFreq.err = fmt.Errorf("contador de frequência indisponível (PDH 0x%x)", r)
		return cpuFreq.err
	}
	return nil
}

// CPUFreqPct implements sys.Native: the average frequency of the logical
// processors as a percentage of their maximum, since the previous call (the
// first call only starts the measurement and returns an error to say so).
func (n *Native) CPUFreqPct() (float64, error) {
	cpuFreq.mu.Lock()
	defer cpuFreq.mu.Unlock()
	if err := openCPUFreq(); err != nil {
		return 0, err
	}
	if r, _, _ := procPdhCollectQueryData.Call(cpuFreq.query); r != 0 {
		return 0, fmt.Errorf("PdhCollectQueryData: 0x%x", r)
	}
	if !cpuFreq.primed {
		cpuFreq.primed = true
		return 0, errors.New("medição iniciada: leia de novo em um instante")
	}
	var v pdhFmtCounterValue
	var typ uint32
	if r, _, _ := procPdhGetFormattedCounterValue.Call(cpuFreq.counter, pdhFmtDouble|pdhFmtNoCap100, uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&v))); r != 0 {
		return 0, fmt.Errorf("PdhGetFormattedCounterValue: 0x%x", r)
	}
	if v.CStatus != pdhCStatusValid && v.CStatus != pdhCStatusNewDat {
		return 0, fmt.Errorf("contador sem valor (estado %d)", v.CStatus)
	}
	return v.Double, nil
}

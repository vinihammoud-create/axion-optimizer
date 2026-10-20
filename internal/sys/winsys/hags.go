//go:build windows

package winsys

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"axion/internal/sys"
)

var (
	procD3DKMTEnumAdapters2    = modGdi32.NewProc("D3DKMTEnumAdapters2")
	procD3DKMTQueryAdapterInfo = modGdi32.NewProc("D3DKMTQueryAdapterInfo")
	procD3DKMTCloseAdapter     = modGdi32.NewProc("D3DKMTCloseAdapter")
)

// D3DKMT structures (d3dkmthk.h), x64 layout.
type d3dkmtAdapterInfo struct {
	hAdapter       uint32
	luidLow        uint32
	luidHigh       int32
	numSources     uint32
	precisePresent int32
}

type d3dkmtEnumAdapters2 struct {
	numAdapters uint32
	_           uint32
	adapters    *d3dkmtAdapterInfo
}

type d3dkmtQueryAdapterInfo struct {
	hAdapter uint32
	typ      uint32
	data     unsafe.Pointer
	size     uint32
	_        uint32
}

// kmtqaitypeWDDM27Caps is KMTQAITYPE_WDDM_2_7_CAPS.
const kmtqaitypeWDDM27Caps = 70

// GPUScheduling implements sys.Native: every adapter is asked for its WDDM
// 2.7 caps (HwSchSupported, HwSchEnabled, HwSchEnabledByDefault bits).
func (n *Native) GPUScheduling() (sys.GPUScheduling, error) {
	var g sys.GPUScheduling
	for _, p := range []*windows.LazyProc{procD3DKMTEnumAdapters2, procD3DKMTQueryAdapterInfo, procD3DKMTCloseAdapter} {
		if err := p.Find(); err != nil {
			return g, err
		}
	}
	var e d3dkmtEnumAdapters2
	if st, _, _ := procD3DKMTEnumAdapters2.Call(uintptr(unsafe.Pointer(&e))); st != 0 {
		return g, fmt.Errorf("D3DKMTEnumAdapters2: NTSTATUS 0x%08X", uint32(st))
	}
	if e.numAdapters == 0 {
		return g, nil
	}
	list := make([]d3dkmtAdapterInfo, e.numAdapters)
	e.adapters = &list[0]
	if st, _, _ := procD3DKMTEnumAdapters2.Call(uintptr(unsafe.Pointer(&e))); st != 0 {
		return g, fmt.Errorf("D3DKMTEnumAdapters2: NTSTATUS 0x%08X", uint32(st))
	}
	for i := 0; i < int(e.numAdapters) && i < len(list); i++ {
		a := list[i]
		var caps uint32
		q := d3dkmtQueryAdapterInfo{hAdapter: a.hAdapter, typ: kmtqaitypeWDDM27Caps, data: unsafe.Pointer(&caps), size: 4}
		if st, _, _ := procD3DKMTQueryAdapterInfo.Call(uintptr(unsafe.Pointer(&q))); st == 0 && caps&1 != 0 {
			g.Supported = true
			if caps&2 != 0 {
				g.Enabled = true
			}
			if caps&4 != 0 {
				g.EnabledByDefault = true
			}
		}
		h := a.hAdapter
		procD3DKMTCloseAdapter.Call(uintptr(unsafe.Pointer(&h)))
	}
	return g, nil
}

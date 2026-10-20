package sys

import (
	"fmt"
	"strings"
)

// The memory manager agent (MMAgent) is reached through the WMI class that
// the Windows MMAgent PowerShell module wraps (Get-MMAgent, Enable-MMAgent and
// Disable-MMAgent are cmdlets-over-objects over it: see
// C:\Windows\System32\WindowsPowerShell\v1.0\Modules\MMAgent\ps_mmagent_v1.0.cdxml).
// Axion calls the class directly, without PowerShell.
//
// All three methods are static. Get returns the state as an embedded
// MMAgentComponents instance in its output parameter "cmdletOutput". Enable
// and Disable take one Boolean per component; a component that is left out is
// not touched ("remains in its current state", per the Enable-MMAgent page).
const (
	MMAgentNamespace = `root\Microsoft\Windows\PS_MMAgent`
	MMAgentClass     = "PS_MMAgent"
)

// MMAgentState is the part of Get-MMAgent that Axion reads.
type MMAgentState struct {
	// MemoryCompression is the "MemoryCompression" component: Windows keeps
	// pages it would page out compressed in RAM.
	MemoryCompression bool
}

// rowFold reads a key of a row ignoring case: WMI property names are case
// insensitive and the class declares the output parameter as "cmdletOutput".
func rowFold(r WMIRow, key string) (any, bool) {
	if v, ok := r[key]; ok {
		return v, true
	}
	for k, v := range r {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return nil, false
}

// wmiCode turns the return value of a method into an int64 (0, success, when
// the answer carries none).
func wmiCode(r WMIRow) int64 {
	v, ok := rowFold(r, "ReturnValue")
	if !ok || v == nil {
		return 0
	}
	return WMIRow{"v": v}.Int("v")
}

// ReadMMAgent reads the MMAgent state with PS_MMAgent.Get. It fails when the
// class is missing (older Windows, damaged WMI repository), when access is
// denied (Axion not elevated) or when the answer has no MemoryCompression.
func ReadMMAgent(w WMI) (MMAgentState, error) {
	if w == nil {
		return MMAgentState{}, fmt.Errorf("WMI indisponível")
	}
	out, err := w.CallOut(MMAgentNamespace, MMAgentClass, "Get", nil)
	if err != nil {
		return MMAgentState{}, err
	}
	if code := wmiCode(out); code != 0 {
		return MMAgentState{}, fmt.Errorf("PS_MMAgent.Get devolveu o código %d", code)
	}
	raw, _ := rowFold(out, "cmdletOutput")
	comp, ok := raw.(WMIRow)
	if !ok {
		return MMAgentState{}, fmt.Errorf("PS_MMAgent.Get não devolveu o estado dos componentes")
	}
	mc, ok := rowFold(comp, "MemoryCompression")
	if !ok || mc == nil {
		return MMAgentState{}, fmt.Errorf("PS_MMAgent.Get não informou MemoryCompression")
	}
	return MMAgentState{MemoryCompression: WMIRow{"v": mc}.Bool("v")}, nil
}

// SetMemoryCompression turns memory compression on or off with
// PS_MMAgent.Enable or Disable, passing only the MemoryCompression switch so
// the other components (prefetching, page combining...) stay as they are.
func SetMemoryCompression(w WMI, on bool) error {
	if w == nil {
		return fmt.Errorf("WMI indisponível")
	}
	method := "Disable"
	if on {
		method = "Enable"
	}
	out, err := w.CallOut(MMAgentNamespace, MMAgentClass, method, map[string]any{"MemoryCompression": true})
	if err != nil {
		return err
	}
	if code := wmiCode(out); code != 0 {
		return fmt.Errorf("PS_MMAgent.%s devolveu o código %d", method, code)
	}
	return nil
}

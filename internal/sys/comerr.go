package sys

import (
	"errors"
	"regexp"
)

// HResult extracts the HRESULT of a COM error. Calls through IDispatch fail
// with DISP_E_EXCEPTION (0x80020009, "Exceção.") and carry the real HRESULT
// (e.g. 0x80070002 for a missing scheduled task) in the EXCEPINFO, which
// go-ole exposes as the wrapped error. Returns 0 for non-COM errors.
func HResult(err error) uint32 {
	var sc interface{ SCODE() uint32 }
	if errors.As(err, &sc) && sc.SCODE() != 0 {
		return sc.SCODE()
	}
	var oe interface{ Code() uintptr }
	if errors.As(err, &oe) {
		return uint32(oe.Code())
	}
	return 0
}

var reSelectStar = regexp.MustCompile(`(?i)^\s*SELECT\s+\*\s+FROM\s`)

// IsSelectStar tells whether a WQL query returns full instances.
func IsSelectStar(wql string) bool { return reSelectStar.MatchString(wql) }

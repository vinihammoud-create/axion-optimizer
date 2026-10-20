package catalog_test

import (
	"time"

	"axion/internal/sys"
)

func mocksysSZ(s string) sys.RegValue { return sys.SZ(s) }
func mocksysDW(v uint32) sys.RegValue { return sys.DWord(v) }

const sysStartDisabled = sys.StartDisabled

var sysRunOpts = sys.RunOpts{Timeout: 5 * time.Second}

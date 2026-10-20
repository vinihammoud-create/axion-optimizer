//go:build windows

package winsys

import (
	"context"
	"errors"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"axion/internal/sys"
)

// Services implements sys.Services with the Service Control Manager.
type Services struct{}

// alignedBytes returns a zeroed byte slice of at least n bytes whose start is
// 8-byte aligned (it is backed by a []uint64 on the heap). A buffer handed to
// a Windows API that fills it with a structure holding pointer fields
// (QueryServiceConfig and the other RPC-based SCM calls) must be aligned like
// the structure: a plain make([]byte, 1024) of constant size is kept by the
// compiler on the goroutine stack with 1-byte alignment, and the RPC
// unmarshaller then faults inside the Windows DLL — an exception the Go
// runtime cannot recover from, which ended Axion 0.5.0–0.5.2 during the scan.
func alignedBytes(n int) []byte {
	if n < 8 {
		n = 8
	}
	words := make([]uint64, (n+7)/8)
	return unsafe.Slice((*byte)(unsafe.Pointer(&words[0])), len(words)*8)
}

func withService(name string, access uint32, fn func(h windows.Handle) error) error {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return err
	}
	defer windows.CloseServiceHandle(scm)
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	h, err := windows.OpenService(scm, n, access)
	if err != nil {
		return err
	}
	defer windows.CloseServiceHandle(h)
	return fn(h)
}

// Query implements sys.Services. The service is opened only with the rights
// needed to read it (QUERY_CONFIG | QUERY_STATUS): anti-cheat and other
// protected services deny SERVICE_ALL_ACCESS even to administrators, and
// were then taken as "not installed". A service whose even that is denied
// exists (its start type is then unknown).
func (Services) Query(name string) (sys.ServiceInfo, error) {
	info := sys.ServiceInfo{Name: name}
	err := withService(name, windows.SERVICE_QUERY_CONFIG|windows.SERVICE_QUERY_STATUS, func(h windows.Handle) error {
		info.Exists = true
		var needed uint32
		buf := alignedBytes(1024)
		for {
			cfg := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buf[0]))
			err := windows.QueryServiceConfig(h, cfg, uint32(len(buf)), &needed)
			if err == nil {
				info.DisplayName = windows.UTF16PtrToString(cfg.DisplayName)
				switch cfg.StartType {
				case windows.SERVICE_BOOT_START:
					info.Start = sys.StartBoot
				case windows.SERVICE_SYSTEM_START:
					info.Start = sys.StartSystem
				case windows.SERVICE_AUTO_START:
					// Automatic or automatic (delayed): when the delayed flag cannot
					// be read the start type stays unknown, so an undo never turns
					// a delayed service into a plain automatic one.
					var d windows.SERVICE_DELAYED_AUTO_START_INFO
					switch err := windows.QueryServiceConfig2(h, windows.SERVICE_CONFIG_DELAYED_AUTO_START_INFO, (*byte)(unsafe.Pointer(&d)), uint32(unsafe.Sizeof(d)), &needed); {
					case err != nil:
						info.Start = sys.StartUnknown
					case d.IsDelayedAutoStartUp != 0:
						info.Start = sys.StartAutoDelayed
					default:
						info.Start = sys.StartAuto
					}
				case windows.SERVICE_DEMAND_START:
					info.Start = sys.StartManual
				case windows.SERVICE_DISABLED:
					info.Start = sys.StartDisabled
				}
				break
			}
			if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) || int(needed) <= len(buf) || needed > 1<<20 {
				return err
			}
			buf = alignedBytes(int(needed))
		}
		var st windows.SERVICE_STATUS
		if windows.QueryServiceStatus(h, &st) == nil {
			info.Running = st.CurrentState == windows.SERVICE_RUNNING || st.CurrentState == windows.SERVICE_START_PENDING
		}
		return nil
	})
	switch {
	case err == nil:
		return info, nil
	case errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST):
		return sys.ServiceInfo{Name: name}, nil
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		info.Exists = true // protected: it is there, the details are not readable
		return info, nil
	}
	return info, err
}

// SetStart implements sys.Services. Only the start type (and the delayed-start
// flag) are changed; every other field is passed as SERVICE_NO_CHANGE.
func (Services) SetStart(name string, start sys.ServiceStart) error {
	var st uint32
	delayed := false
	switch start {
	case sys.StartAuto:
		st = windows.SERVICE_AUTO_START
	case sys.StartAutoDelayed:
		st, delayed = windows.SERVICE_AUTO_START, true
	case sys.StartManual:
		st = windows.SERVICE_DEMAND_START
	case sys.StartDisabled:
		st = windows.SERVICE_DISABLED
	case sys.StartBoot:
		st = windows.SERVICE_BOOT_START
	case sys.StartSystem:
		st = windows.SERVICE_SYSTEM_START
	default:
		return errors.New("tipo de inicialização inválido")
	}
	return withService(name, windows.SERVICE_CHANGE_CONFIG|windows.SERVICE_QUERY_CONFIG, func(h windows.Handle) error {
		if err := windows.ChangeServiceConfig(h, windows.SERVICE_NO_CHANGE, st, windows.SERVICE_NO_CHANGE, nil, nil, nil, nil, nil, nil, nil); err != nil {
			return err
		}
		if st == windows.SERVICE_AUTO_START {
			info := windows.SERVICE_DELAYED_AUTO_START_INFO{}
			if delayed {
				info.IsDelayedAutoStartUp = 1
			}
			return windows.ChangeServiceConfig2(h, windows.SERVICE_CONFIG_DELAYED_AUTO_START_INFO, (*byte)(unsafe.Pointer(&info)))
		}
		return nil
	})
}

// Stop implements sys.Services (waits up to 20 s).
func (Services) Stop(ctx context.Context, name string) error {
	return withService(name, windows.SERVICE_STOP|windows.SERVICE_QUERY_STATUS, func(h windows.Handle) error {
		var st windows.SERVICE_STATUS
		if err := windows.ControlService(h, windows.SERVICE_CONTROL_STOP, &st); err != nil {
			if errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
				return nil
			}
			return err
		}
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			if ctx != nil && ctx.Err() != nil {
				return ctx.Err()
			}
			if err := windows.QueryServiceStatus(h, &st); err != nil {
				return err
			}
			if st.CurrentState == windows.SERVICE_STOPPED {
				return nil
			}
			time.Sleep(250 * time.Millisecond)
		}
		return errors.New("o serviço não parou em 20 segundos")
	})
}

// Start implements sys.Services.
func (Services) Start(name string) error {
	return withService(name, windows.SERVICE_START, func(h windows.Handle) error {
		err := windows.StartService(h, 0, nil)
		if errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
			return nil
		}
		return err
	})
}

//go:build windows

package winsys

import (
	"errors"
	"fmt"
	"testing"

	"golang.org/x/sys/windows"

	"axion/internal/sys"
)

// A service whose query is denied is a read error, not "exists, stopped";
// one that does not exist is a state.
func TestServiceQueryOutcome(t *testing.T) {
	denied := fmt.Errorf("abrir: %w", windows.ERROR_ACCESS_DENIED)
	info, err := queryOutcome("svc", sys.ServiceInfo{Name: "svc", Exists: true}, denied)
	if err == nil || !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("acesso negado deveria ser erro de leitura: %+v, %v", info, err)
	}
	if info.Running || info.Start != sys.StartUnknown {
		t.Errorf("acesso negado não informa estado: %+v", info)
	}
	info, err = queryOutcome("svc", sys.ServiceInfo{Name: "svc", Exists: true}, windows.ERROR_SERVICE_DOES_NOT_EXIST)
	if err != nil || info.Exists {
		t.Errorf("serviço inexistente é estado, não erro: %+v, %v", info, err)
	}
	ok := sys.ServiceInfo{Name: "svc", Exists: true, Start: sys.StartManual, Running: true}
	if info, err = queryOutcome("svc", ok, nil); err != nil || info != ok {
		t.Errorf("leitura completa: %+v, %v", info, err)
	}
}

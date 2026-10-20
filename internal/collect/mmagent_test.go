package collect_test

import (
	"errors"
	"testing"

	"axion/internal/sys"
)

// Memory compression is read from the memory manager agent over WMI. Only a
// state that was really read can be a leftover of another optimizer.
func TestMemoryCompressionResidue(t *testing.T) {
	m, collectNow := collectMock(t)
	r := collectNow().Residue
	if !r.MMAgentRead || !r.MemoryCompression || r.MemoryCompressionOff() {
		t.Fatalf("padrão do Windows (ligada): lida=%v ligada=%v desligada=%v", r.MMAgentRead, r.MemoryCompression, r.MemoryCompressionOff())
	}
	m.SetMemoryCompression(false)
	r = collectNow().Residue
	if !r.MMAgentRead || r.MemoryCompression || !r.MemoryCompressionOff() {
		t.Errorf("desligada: lida=%v ligada=%v desligada=%v", r.MMAgentRead, r.MemoryCompression, r.MemoryCompressionOff())
	}
	// Access denied (Axion not elevated) or class missing: unknown, never "off".
	m.W.OutHandler = func(ns, path, method string, in map[string]any) (sys.WMIRow, error) {
		return nil, errors.New("Acesso negado (0x80041003)")
	}
	r = collectNow().Residue
	if r.MMAgentRead || r.MemoryCompressionOff() {
		t.Errorf("leitura negada: lida=%v desligada=%v", r.MMAgentRead, r.MemoryCompressionOff())
	}
	// An answer without the component is not "off" either.
	m.W.OutHandler = func(ns, path, method string, in map[string]any) (sys.WMIRow, error) {
		return sys.WMIRow{"ReturnValue": int64(0), "cmdletOutput": sys.WMIRow{"PageCombining": false}}, nil
	}
	if r = collectNow().Residue; r.MMAgentRead || r.MemoryCompressionOff() {
		t.Errorf("resposta sem MemoryCompression: lida=%v desligada=%v", r.MMAgentRead, r.MemoryCompressionOff())
	}
	// The method failed (non-zero return value).
	m.W.OutHandler = func(ns, path, method string, in map[string]any) (sys.WMIRow, error) {
		return sys.WMIRow{"ReturnValue": int64(1)}, nil
	}
	if r = collectNow().Residue; r.MMAgentRead || r.MemoryCompressionOff() {
		t.Errorf("código de erro: lida=%v desligada=%v", r.MMAgentRead, r.MemoryCompressionOff())
	}
}

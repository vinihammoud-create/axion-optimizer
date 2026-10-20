package sys_test

import (
	"strings"
	"testing"

	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

// The helpers must pass named parameters, only for the component being changed,
// and read the state from the embedded output instance.
func TestMMAgentHelpers(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st, err := sys.ReadMMAgent(m.W)
	if err != nil || !st.MemoryCompression {
		t.Fatalf("padrão simulado (ligada): %+v, %v", st, err)
	}
	var seen []map[string]any
	m.W.OutHandler = func(ns, path, method string, in map[string]any) (sys.WMIRow, error) {
		if !strings.EqualFold(ns, sys.MMAgentNamespace) || path != sys.MMAgentClass {
			t.Errorf("chamada fora da classe do agente: %s %s", ns, path)
		}
		seen = append(seen, map[string]any{"method": method, "in": in})
		return sys.WMIRow{"ReturnValue": uint32(0), "cmdletOutput": sys.WMIRow{"MemoryCompression": false}}, nil
	}
	if st, err = sys.ReadMMAgent(m.W); err != nil || st.MemoryCompression {
		t.Fatalf("lendo desligada: %+v, %v", st, err)
	}
	if err := sys.SetMemoryCompression(m.W, true); err != nil {
		t.Fatal(err)
	}
	if err := sys.SetMemoryCompression(m.W, false); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 3 || seen[1]["method"] != "Enable" || seen[2]["method"] != "Disable" {
		t.Fatalf("métodos chamados: %+v", seen)
	}
	for _, c := range seen[1:] {
		in := c["in"].(map[string]any)
		if len(in) != 1 || in["MemoryCompression"] != true {
			t.Errorf("só MemoryCompression deveria ser passado: %+v", in)
		}
	}
	if err := sys.SetMemoryCompression(nil, true); err == nil {
		t.Error("sem WMI deveria falhar")
	}
	// The built-in simulation refuses an unknown parameter instead of ignoring it.
	m.W.OutHandler = nil
	if _, err := m.W.CallOut(sys.MMAgentNamespace, sys.MMAgentClass, "Enable", map[string]any{"Inexistente": true}); err == nil {
		t.Error("parâmetro desconhecido deveria falhar")
	}
	if _, err := m.W.CallOut(`root\cimv2`, "Win32_Qualquer", "Metodo", nil); err == nil {
		t.Error("método não simulado deveria falhar")
	}
}

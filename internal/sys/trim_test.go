package sys

import (
	"testing"
	"time"
)

func TestParseDeleteNotify(t *testing.T) {
	for _, c := range []struct {
		name       string
		out        string
		ntfs, refs int
		ok         bool
	}{
		{"inglês", "NTFS DisableDeleteNotify = 0  (Allows TRIM operations to be sent to the storage device)\r\nReFS DisableDeleteNotify = 0  (Allows TRIM operations to be sent to the storage device)\r\n", 0, 0, true},
		{"documentação", "NTFS DisableDeleteNotify = 1\r\nReFS DisableDeleteNotify is not currently set\r\n", 1, -1, true},
		{"português", "NTFS DisableDeleteNotify = 1  (Desabilitado)\r\nReFS DisableDeleteNotify = 0  (Habilitado)\r\n", 1, 0, true},
		{"formato antigo", "DisableDeleteNotify = 0\r\n", 0, -1, true},
		{"erro", "Erro: Acesso negado.\r\n", -1, -1, false},
	} {
		n, r, ok := ParseDeleteNotify(c.out)
		if n != c.ntfs || r != c.refs || ok != c.ok {
			t.Errorf("%s: obtido ntfs=%d refs=%d ok=%v", c.name, n, r, ok)
		}
	}
}

func TestOptimizerEvent(t *testing.T) {
	ev := Event{Provider: "Microsoft-Windows-Defrag", ID: 258, Time: time.Now(), Data: map[string]string{"0": "retrim", "1": "Windows (C:)"}}
	op, l, ok := OptimizerEvent(ev)
	if !ok || l != "C:" || !IsRetrim(op) {
		t.Fatalf("op=%q letra=%q ok=%v", op, l, ok)
	}
	ev.Data = map[string]string{"0": "defragmentation", "1": "Jogos (d: )"}
	if op, l, ok := OptimizerEvent(ev); !ok || l != "D:" || IsRetrim(op) {
		t.Fatalf("desfragmentação: op=%q letra=%q ok=%v", op, l, ok)
	}
	ev.Provider = "Outro"
	if _, _, ok := OptimizerEvent(ev); ok {
		t.Fatal("evento 258 de outro provedor não é do otimizador")
	}
	// A volume without a drive letter (e.g. the recovery partition).
	ev.Provider, ev.Data = "Microsoft-Windows-Defrag", map[string]string{"0": "retrim", "1": "WINRE_DRV"}
	if op, l, ok := OptimizerEvent(ev); !ok || l != "" || op != "retrim" {
		t.Fatalf("volume sem letra: op=%q letra=%q ok=%v", op, l, ok)
	}
	// Translated operation names (Portuguese Windows).
	for _, op := range []string{"recorte", "repetição de corte", "retrim", "Retrim"} {
		if !IsRetrim(op) {
			t.Errorf("%q deveria contar como retrim", op)
		}
	}
	for _, op := range []string{"análise", "desfragmentação", "analysis", "otimização de inicialização"} {
		if IsRetrim(op) {
			t.Errorf("%q não é retrim", op)
		}
	}
	// Named data (manifest-based events) and the classic source name.
	ev.Provider, ev.Data = "Defrag", map[string]string{"param1": "retrim", "param2": "Windows (C:)"}
	if op, l, ok := OptimizerEvent(ev); !ok || l != "C:" || !IsRetrim(op) {
		t.Fatalf("dados nomeados: op=%q letra=%q ok=%v", op, l, ok)
	}
}

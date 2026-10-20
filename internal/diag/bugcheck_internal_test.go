package diag

import (
	"strings"
	"testing"
)

// Every entry of the table is complete: a name, what it says, a kind that has
// advice, a Microsoft page under the bug check reference and, when it names an
// Axion page, a page that exists in the label map.
func TestBugcodeTableIsComplete(t *testing.T) {
	if len(bugcodes) < 15 {
		t.Fatalf("a tabela encolheu: %d códigos", len(bugcodes))
	}
	for c, b := range bugcodes {
		if b.Name == "" || b.Says == "" || bugAdvice[b.Kind] == "" || !strings.HasPrefix(b.URL, bcBase+"bug-check-0x") {
			t.Errorf("0x%X incompleto: %+v", c, b)
		}
		// The page name carries the code and the name, as Microsoft's do.
		if !strings.Contains(b.URL, strings.ToLower(strings.ReplaceAll(b.Name, "_", "-"))) {
			t.Errorf("0x%X: a URL não é a página de %s: %s", c, b.Name, b.URL)
		}
		if b.Page != "" && b.Page != "drivers" && b.Page != "dpc" {
			t.Errorf("0x%X: página %q sem rótulo de ação", c, b.Page)
		}
		if strings.HasSuffix(b.Says, ".") {
			t.Errorf("0x%X: o texto termina com ponto e a frase o repete: %q", c, b.Says)
		}
	}
	for k, v := range bugAdvice {
		if v == "" || !strings.HasSuffix(v, ".") {
			t.Errorf("conselho de %s: %q", k, v)
		}
	}
}

// Microsoft's pages (opened and read): 0x154 talks about the kernel memory
// store and points to Windows Memory Diagnostics, not to the disk; 0x139 lists
// drivers that corrupt KEVENT, KTIMER and LIST_ENTRY as the common causes and
// says to update or remove new drivers.
func TestBugcodeKindsFollowMicrosoftPages(t *testing.T) {
	if b := bugcodes[0x154]; b.Kind != bcMemory {
		t.Errorf("0x154 deve ser de memória: %+v", b)
	}
	if b := bugcodes[0x139]; b.Kind != bcDriver || b.Page != "drivers" {
		t.Errorf("0x139 deve ser de driver, com a página Drivers: %+v", b)
	}
}

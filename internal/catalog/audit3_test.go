package catalog_test

import (
	"strings"
	"testing"

	"axion/internal/catalog"
	"axion/internal/facts"
	"axion/internal/tweaks"
)

// High-confidence findings of docs/audit/CATALOGO-3.md: evidence and wording
// that the cited source does not sustain.

// F17 and F24: the cited Microsoft page documents the Settings option or the
// policy, not the value or the key the item writes, so the evidence is not
// high. F23: no source documents the StartupApproved format, so it is low.
func TestPart3EvidenceMatchesTheSource(t *testing.T) {
	withGamePath(t)
	gpu := catalog.ProfileProvider(catalog.ProfileID("cs2.exe", "gpu"), &facts.Facts{})
	if gpu == nil {
		t.Fatal("perfil de GPU do cs2.exe não foi montado")
	}
	if gpu.Evidence == tweaks.EvHigh {
		t.Error("GpuPreference=2: a página documenta a opção de Configurações, não o valor gravado")
	}
	if r := gpu.Recommend(&facts.Facts{}); strings.Contains(r.Reason, "garante") || strings.Contains(gpu.Summary, "Força") {
		t.Errorf("preferência de Configurações apresentada como imposição: %q / %q", gpu.Summary, r.Reason)
	}
	prio := catalog.ProfileProvider(catalog.ProfileID("cs2.exe", "priority"), &facts.Facts{})
	if prio == nil || strings.Contains(prio.Detail, "mecanismo de prioridade persistente do próprio Windows") {
		t.Errorf("CpuPriorityClass descrito como mecanismo documentado: %+v", prio)
	}
	if ah := catalog.ActiveHoursProvider(catalog.ActiveHoursID(8, 23), &facts.Facts{}); ah == nil || ah.Evidence == tweaks.EvHigh {
		t.Errorf("horário ativo: a fonte é a política, em outra chave; evidência não pode ser alta: %+v", ah)
	}
}

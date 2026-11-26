package diag_test

import (
	"strings"
	"testing"
	"time"

	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/sys"
)

func wheaOnly() []sys.Event {
	return []sys.Event{{Channel: "System", Provider: "Microsoft-Windows-WHEA-Logger", ID: 17, Level: 3, Time: time.Now().Add(-26 * time.Hour),
		Data: map[string]string{"PrimaryDeviceName": `PCI\VEN_10DE&DEV_2504`}}}
}

func TestWHEAFindingDoesNotClaimNoSymptomsWhenUnread(t *testing.T) {
	const claim = "Nenhum desligamento"
	f := &facts.Facts{Unread: []string{diag.PartSymptoms}}
	x := whFindingWith(t, wheaOnly(), f, nil)
	if strings.Contains(x.Evidence, claim) {
		t.Errorf("a consulta de sintomas falhou, mas o achado afirma que não há sintoma: %s", x.Evidence)
	}
	if !strings.Contains(x.Evidence, "Não foi possível ler") {
		t.Errorf("o achado deveria dizer que os sintomas não foram lidos: %s", x.Evidence)
	}

	x = whFindingWith(t, wheaOnly(), &facts.Facts{}, nil)
	if !strings.Contains(x.Evidence, claim) {
		t.Errorf("com a leitura completa o texto continua: %s", x.Evidence)
	}
	x = whFindingWith(t, wheaOnly(), nil, nil)
	if !strings.Contains(x.Evidence, claim) {
		t.Errorf("sem o perfil do PC o texto continua: %s", x.Evidence)
	}
}

func TestWHEATitleAndExplanationDoNotClaimNoSymptomsWhenUnread(t *testing.T) {
	x := whFindingWith(t, wheaOnly(), &facts.Facts{Unread: []string{diag.PartSymptoms}}, nil)
	if strings.Contains(x.Title, "sem sintoma") {
		t.Errorf("a consulta de sintomas falhou, mas o título afirma que não há sintoma: %s", x.Title)
	}
	if strings.Contains(x.Explanation, "sem sintoma") {
		t.Errorf("a consulta de sintomas falhou, mas a explicação afirma que não há sintoma: %s", x.Explanation)
	}

	full := whFindingWith(t, wheaOnly(), &facts.Facts{}, nil)
	if !strings.Contains(full.Title, "sem sintoma") || !strings.Contains(full.Explanation, "sem erro grave, sem sintoma e sem rajada") {
		t.Errorf("com a leitura completa o texto continua: %s / %s", full.Title, full.Explanation)
	}
	if x.Severity != full.Severity {
		t.Errorf("a severidade mudou: %v em vez de %v", x.Severity, full.Severity)
	}
}

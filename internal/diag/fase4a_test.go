package diag_test

import (
	"context"
	"strings"
	"testing"

	"axion/internal/collect"
	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/sys/mocksys"
)

// The critical anti-cheat finding (Secure Boot off) with a TPM that could
// not be read says "não lido", not "não encontrado" (as sec.anticheat.gc).
func TestAntiCheatCriticalUnreadTPMIsNotAbsent(t *testing.T) {
	f := base()
	f.Gaming.AntiCheats = []string{"FACEIT"}
	f.Security = facts.SecurityInfo{TPMRead: false, SecureBoot: "off", VBSRunning: true, DeviceGuardRead: true}
	x := find(diag.Build(f, nil), "sec.anticheat")
	if x == nil || x.Severity != diag.SevCritical {
		t.Fatalf("Secure Boot desligado com FACEIT deveria ser crítico: %+v", x)
	}
	if strings.Contains(x.Evidence, "não encontrado") || !strings.Contains(x.Evidence, "TPM: não lido") {
		t.Errorf("TPM não lido citado como ausente: %s", x.Evidence)
	}
}

// G-015 b / CATALOGO-2: on a notebook the EEE finding does not use the
// desktop text (on a notebook EEE saves battery).
func TestEEEFindingNotebookText(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := collect.Collect(context.Background(), m.Platform, nil)
	f.Power.IsLaptop = true
	x := find(diag.Build(f, nil), "net.eee")
	if x == nil {
		t.Fatal("o PC simulado tem EEE ligado na placa com cabo")
	}
	if strings.Contains(x.Explanation, "desktop") || !strings.Contains(x.Explanation, "bateria") {
		t.Errorf("texto do EEE em notebook: %s", x.Explanation)
	}
}

package diag_test

import (
	"strings"
	"testing"

	"axion/internal/diag"
	"axion/internal/facts"
)

func TestAntiCheatGoodEvidenceOnlyCitesWhatWasRead(t *testing.T) {
	f := base()
	f.OS = facts.OSInfo{Caption: "Microsoft Windows 10 Pro", Build: 19045, SystemDrive: "C:"}
	f.Gaming.AntiCheats = []string{"Vanguard"}
	f.Security = facts.SecurityInfo{TPMRead: true, TPMReady: true, TPMVersion: "2.0", SecureBoot: "off", VBSRunning: true}
	r := diag.Build(f, nil)
	for _, x := range r.Findings {
		if strings.Contains(x.Evidence, "Secure Boot ligado") || strings.Contains(x.Title, "Secure Boot ligado") {
			t.Errorf("Secure Boot desligado citado como ligado: %+v", x)
		}
	}
	if x := find(r, "sec.anticheat"); x == nil || x.Severity != diag.SevGood || !strings.Contains(x.Evidence, "Secure Boot desligado") {
		t.Errorf("evidência deveria citar o Secure Boot desligado: %+v", x)
	}

	f.Security.SecureBoot = "on"
	if x := find(diag.Build(f, nil), "sec.anticheat"); x == nil || !strings.Contains(x.Evidence, "Secure Boot ligado") || !strings.Contains(x.Evidence, "TPM 2.0") {
		t.Errorf("evidência com Secure Boot ligado e TPM lido: %+v", x)
	}
}

func TestAntiCheatGoodNotEmittedWithoutTPMRead(t *testing.T) {
	f := base()
	f.Gaming.AntiCheats = []string{"FACEIT"}
	f.Security = facts.SecurityInfo{TPMRead: false, SecureBoot: "on", VBSRunning: true}
	for _, x := range diag.Build(f, nil).Findings {
		if x.ID == "sec.anticheat" && x.Severity == diag.SevGood {
			t.Errorf("TPM não lido não pode virar requisito atendido: %+v", x)
		}
	}
}

package diag_test

import (
	"context"
	"testing"

	"axion/internal/collect"
	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/profile"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

// checkCertainty: every finding carries one of the four certainties, a good
// one is "normal" and nothing else is.
func checkCertainty(t *testing.T, where string, fs []diag.Finding) {
	t.Helper()
	for _, x := range fs {
		if _, ok := diag.CertaintyLabels[x.Certainty]; !ok {
			t.Errorf("%s: %s sem certeza válida (%q)", where, x.ID, x.Certainty)
		}
		if (x.Severity == diag.SevGood) != (x.Certainty == diag.CertNormal) {
			t.Errorf("%s: %s com severidade %s e certeza %s", where, x.ID, x.Severity, x.Certainty)
		}
	}
}

func TestCertaintyOnSimulatedPCs(t *testing.T) {
	f, r, _ := mockReport(t)
	if len(r.Findings) < 10 {
		t.Fatalf("poucos achados no PC simulado: %d", len(r.Findings))
	}
	checkCertainty(t, "diagnóstico", r.Findings)
	for _, p := range profile.All {
		checkCertainty(t, "capacidade "+p.ID, diag.Score(diag.Capacity(f, p.ID)).Findings)
		fs, _ := diag.Measurement(f, p.ID, nil, nil)
		checkCertainty(t, "medição "+p.ID, diag.Score(fs).Findings)
		checkCertainty(t, "perfil "+p.ID, diag.Finalize(diag.Build(f, nil), p.ID, map[string]bool{"input.accel": true}).Findings)
	}
	// The same PC with the steps that can fail set aside.
	for _, skip := range []string{facts.StepSecurity, facts.StepGaming, facts.StepHealth, collect.PartNVMe} {
		m, err := mocksys.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		g := collect.CollectWith(context.Background(), m.Platform, nil, collect.Options{Skip: map[string]bool{skip: true}})
		checkCertainty(t, "sem "+skip, diag.Build(g, map[string]tweaks.State{}).Findings)
	}
	// Explicit cases on the simulated PC: a registry value that states the
	// problem, a line drawn by the Axion and a reading that failed.
	for prefix, want := range map[string]string{
		"game.dvr":       diag.CertConfirmed, // HistoricalCaptureEnabled = 1
		"boot.hpet":      diag.CertConfirmed, // bcdedit useplatformclock
		"gpu.driver.old": diag.CertPossible,  // 6 months is the Axion's criterion
		"disk.wear.":     diag.CertPossible,  // 81%: the 70% line is the Axion's
		"input.accel":    diag.CertPossible,  // raw input games are not affected
	} {
		x := find(r, prefix)
		if x == nil {
			t.Errorf("achado %s ausente no PC simulado", prefix)
			continue
		}
		if x.Certainty != want {
			t.Errorf("%s: certeza %q, esperado %q", x.ID, x.Certainty, want)
		}
	}
	g := *f
	g.RAM.TotalBytes = 0
	g.CPU.Threads = 0
	for _, x := range diag.Score(diag.Capacity(&g, profile.Competitive)).Findings {
		if (x.ID == "cap.ram" || x.ID == "cap.cpu") && x.Certainty != diag.CertInsufficient {
			t.Errorf("%s não lido: certeza %q, esperado insuficiente", x.ID, x.Certainty)
		}
	}
}

// The low-end PC of TestLowEndMachine: the problems read straight from the
// machine are confirmed, the Axion's own lines are possible.
func TestCertaintyLowEnd(t *testing.T) {
	f := &facts.Facts{
		OS:       facts.OSInfo{Caption: "Microsoft Windows 10 Home", Build: 19045, SystemDrive: "C:"},
		RAM:      facts.RAMInfo{TotalBytes: 8 << 30, Type: "DDR4", ConfiguredMTs: 2666, Modules: []facts.MemModule{{CapacityBytes: 8 << 30, PartNumber: "M378A1K43DB2-CTD"}}},
		Disks:    []facts.Disk{{Model: "ST1000DM010", MediaType: "HDD", BusType: "SATA", Health: "Healthy", IsSystem: true, WearPct: -1, TempC: -1}},
		Gaming:   facts.GamingInfo{AntiCheats: []string{"FACEIT"}},
		Adapters: []facts.Adapter{{Description: "Realtek PCIe GbE Family Controller", Kind: "Ethernet", LinkSpeedMbps: 100, Up: true, HasGateway: true}},
		Security: facts.SecurityInfo{TPMRead: true, SecureBoot: "off"},
	}
	r := diag.Build(f, nil)
	checkCertainty(t, "PC fraco", r.Findings)
	for id, want := range map[string]string{
		"disk.hdd":      diag.CertConfirmed,
		"ram.single":    diag.CertConfirmed,
		"game.mode":     diag.CertConfirmed,
		"net.link100":   diag.CertConfirmed,
		"sec.anticheat": diag.CertConfirmed,
		"ram.low":       diag.CertPossible, // 16 GB is the Axion's criterion
	} {
		x := find(r, id)
		if x == nil {
			t.Errorf("achado %s ausente", id)
		} else if x.Certainty != want {
			t.Errorf("%s: certeza %q, esperado %q", id, x.Certainty, want)
		}
	}
}

// Every step and part the collection can leave unread belongs to an area,
// and the area whose part was not read says so.
func TestUnreadAreas(t *testing.T) {
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	collect.Collect(context.Background(), m.Platform, func(step string, done, total int) {
		if done < total {
			names = append(names, step)
		}
	})
	if len(names) < 20 {
		t.Fatalf("poucas etapas: %v", names)
	}
	names = append(names, collect.PartHAGS, collect.PartX3D, collect.PartNVMe, collect.PartInput, collect.PartNVDRS,
		diag.PartWHEA, diag.PartSymptoms, diag.PartBoots, diag.PartThermal)
	valid := map[string]bool{}
	for _, a := range diag.Areas {
		valid[a] = true
	}
	for _, n := range names {
		as := diag.UnreadAreas(n)
		if len(as) == 0 {
			t.Errorf("%q não está ligado a nenhuma área", n)
		}
		for _, a := range as {
			if !valid[a] {
				t.Errorf("%q: área desconhecida %q", n, a)
			}
		}
	}
	if as := diag.UnreadAreas("etapa que não existe"); len(as) != 0 {
		t.Errorf("nome desconhecido deveria ficar sem área: %v", as)
	}

	f := collect.CollectWith(context.Background(), m.Platform, nil, collect.Options{Skip: map[string]bool{facts.StepSecurity: true}})
	r := diag.Build(f, map[string]tweaks.State{})
	unread := map[string]bool{}
	for _, s := range r.Scores {
		unread[s.Area] = s.Unread
	}
	if !unread[diag.AreaSeguranca] {
		t.Error("segurança não lida: a área deveria marcar Unread")
	}
	if unread[diag.AreaRede] || unread[diag.AreaArmaz] {
		t.Errorf("áreas lidas marcadas como não lidas: %+v", r.Scores)
	}
	// Finalize keeps the mark when it scores again.
	r = diag.Finalize(r, profile.Work, nil)
	for _, s := range r.Scores {
		if s.Area == diag.AreaSeguranca && !s.Unread {
			t.Error("Finalize perdeu a marca de área não lida")
		}
	}
	_, full, _ := mockReport(t)
	for _, s := range full.Scores {
		if s.Unread {
			t.Errorf("coleta completa com área %s marcada como não lida", s.Area)
		}
	}
}

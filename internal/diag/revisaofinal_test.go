package diag_test

import (
	"context"
	"testing"

	"axion/internal/collect"
	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/sys/mocksys"
)

func hasUnread(list []string, part string) bool {
	for _, u := range list {
		if u == part {
			return true
		}
	}
	return false
}

// The AMD chipset service only matters on the two-CCD X3D processors. A
// query of it that fails makes the analysis incomplete there, and only
// there: on any other PC nothing depends on that reading, so the report must
// not say "análise incompleta" because of it.
func TestX3DServiceFailureOnlyCountsWhereItMatters(t *testing.T) {
	// The simulated PC has a single-CCD processor (Ryzen 7 7800X3D).
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := *m.Platform
	p.Svc = failingSvcQuery{Services: m.S, name: "amd3dvcacheSvc"}
	f := collect.Collect(context.Background(), &p, nil)
	if f.CPU.IsDualCCDX3D {
		t.Fatal("o PC simulado deveria ter um processador de um CCD")
	}
	if hasUnread(f.Unread, facts.PartX3D) {
		t.Errorf("serviço que não se aplica a este processador entrou nas partes não lidas: %v", f.Unread)
	}
	if r := diag.Build(f, nil); hasUnread(r.Unread, facts.PartX3D) {
		t.Errorf("o relatório ficou incompleto por uma leitura que não se aplica: %v", r.Unread)
	}
	if len(f.Errors) == 0 {
		t.Error("a falha da consulta deveria continuar nos avisos da coleta")
	}

	// On a two-CCD X3D the same failure is a part not read.
	d := dualX3DMock(t)
	p = *d.Platform
	p.Svc = failingSvcQuery{Services: d.S, name: "amd3dvcacheSvc"}
	f = collect.Collect(context.Background(), &p, nil)
	if !f.CPU.IsDualCCDX3D || !hasUnread(f.Unread, facts.PartX3D) {
		t.Errorf("num X3D de dois CCDs a consulta que falha é parte não lida: dual=%v %v", f.CPU.IsDualCCDX3D, f.Unread)
	}
}

package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"axion/internal/repair"
	"axion/internal/sys/mocksys"
)

func repairCalls(m *mocksys.Mock) []string {
	var out []string
	for _, c := range m.X.Recorded() {
		n := strings.ToLower(c.Exe)
		if n == "dism.exe" || n == "sfc.exe" {
			out = append(out, n+" "+strings.ToLower(strings.Join(c.Args, " ")))
		}
	}
	return out
}

func runRepair(t *testing.T, c *client) (Job, RepairReport) {
	t.Helper()
	var j Job
	c.json("POST", "/api/repair/run", RepairRequest{Confirm: true}, &j)
	j = c.wait(j.ID)
	b, _ := json.Marshal(j.Result)
	var rep RepairReport
	_ = json.Unmarshal(b, &rep)
	return j, rep
}

func TestRepairJobEndToEnd(t *testing.T) {
	m, c := setupServer(t)
	var info RepairInfo
	c.json("GET", "/api/repair", nil, &info)
	if !info.Elevated || info.Last != nil || len(info.Sources) != 2 {
		t.Fatalf("página inicial: %+v", info)
	}

	// Nothing damaged: DISM first, then SFC, both from the System32 folder
	// by name (the runner resolves them), no console window.
	j, rep := runRepair(t, c)
	if j.Status != JobDone || j.Progress != 1 {
		t.Fatalf("tarefa: %+v", j)
	}
	if got := repairCalls(m); len(got) != 2 || got[0] != "dism.exe /online /cleanup-image /restorehealth" || got[1] != "sfc.exe /scannow" {
		t.Fatalf("chamadas: %q", got)
	}
	if rep.Verdict != RepairClean || len(rep.Steps) != 2 || rep.Steps[0].Outcome != repair.OK || rep.Steps[1].Outcome != repair.OK || rep.Reboot {
		t.Fatalf("relatório: %+v", rep)
	}
	if rep.Summary == "" || strings.Contains(strings.ToLower(rep.Summary), "fps") {
		t.Errorf("texto do resultado: %q", rep.Summary)
	}
	if !strings.Contains(strings.Join(j.Log, "\n"), "/RestoreHealth") {
		t.Errorf("o log deveria mostrar o comando: %v", j.Log)
	}
	c.json("GET", "/api/repair", nil, &info)
	if info.Last == nil || info.Last.Verdict != RepairClean {
		t.Errorf("último reparo não aparece: %+v", info.Last)
	}

	// The tool answers in Brazilian Portuguese and repairs files.
	m.RepairPT, m.RepairSFC = true, "repaired"
	if _, rep = runRepair(t, c); rep.Verdict != RepairFixed || rep.Steps[1].Outcome != repair.Repaired {
		t.Errorf("SFC em pt-BR reparou: %+v", rep)
	}

	// Damage that SFC could not fix.
	m.RepairPT, m.RepairSFC = false, "unrepaired"
	if _, rep = runRepair(t, c); rep.Verdict != RepairUnfixed {
		t.Errorf("dano sem conserto: %+v", rep)
	}

	// A restart is needed.
	m.RepairSFC = "reboot"
	if _, rep = runRepair(t, c); rep.Verdict != RepairRestart || !rep.Reboot {
		t.Errorf("reinício: %+v", rep)
	}

	// DISM cannot find the source files: SFC still runs and the failure is told.
	m.RepairSFC, m.RepairDISM = "", 0x800f081f
	n := len(repairCalls(m))
	j, rep = runRepair(t, c)
	if j.Status != JobDone || rep.Verdict != RepairFailedRun || rep.Steps[0].Outcome != repair.Failed || !strings.Contains(rep.Steps[0].Summary, "arquivos originais") || len(repairCalls(m)) != n+2 {
		t.Errorf("DISM sem origem: %+v (%d chamadas)", rep, len(repairCalls(m))-n)
	}
	if !strings.Contains(rep.Summary, "não terminou") {
		t.Errorf("resumo: %q", rep.Summary)
	}
}

func TestRepairRefusals(t *testing.T) {
	m, c := setupServer(t)
	if code, _ := c.do("POST", "/api/repair/run", RepairRequest{}, nil); code != http.StatusBadRequest {
		t.Errorf("sem confirmação: esperado 400, veio %d", code)
	}
	m.N.Elevated = false
	var info RepairInfo
	c.json("GET", "/api/repair", nil, &info)
	if info.Elevated {
		t.Error("a página deveria dizer que falta administrador")
	}
	code, body := c.do("POST", "/api/repair/run", RepairRequest{Confirm: true}, nil)
	if code != http.StatusBadRequest || !strings.Contains(string(body), "administrador") {
		t.Errorf("sem administrador: %d %s", code, body)
	}
	if got := repairCalls(m); len(got) != 0 {
		t.Errorf("nada deveria rodar: %q", got)
	}
}

func TestRepairCancel(t *testing.T) {
	m, c := setupServer(t)
	release := make(chan struct{})
	defer close(release)
	m.X.Hold = func(exe string, args []string) <-chan struct{} {
		if strings.EqualFold(exe, "dism.exe") {
			return release // DISM "runs" until cancelled
		}
		return nil
	}
	var j Job
	c.json("POST", "/api/repair/run", RepairRequest{Confirm: true}, &j)
	deadline := time.Now().Add(5 * time.Second)
	for len(repairCalls(m)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("o DISM não começou")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// While it runs nothing else that changes the PC may start.
	if code, _ := c.do("POST", "/api/repair/run", RepairRequest{Confirm: true}, nil); code == http.StatusOK {
		t.Error("um segundo reparo não deveria começar")
	}
	var ok map[string]any
	c.json("POST", "/api/jobs/"+j.ID+"/cancel", map[string]any{}, &ok)
	j = c.wait(j.ID)
	if j.Status != JobCancelled {
		t.Fatalf("status %s, esperado cancelado", j.Status)
	}
	b, _ := json.Marshal(j.Result)
	var rep RepairReport
	_ = json.Unmarshal(b, &rep)
	if rep.Verdict != RepairStopped || len(rep.Steps) != 2 || rep.Steps[0].Outcome != repair.Cancelled || rep.Steps[1].Outcome != repair.Skipped {
		t.Fatalf("relatório do cancelamento: %+v", rep)
	}
	if got := repairCalls(m); len(got) != 1 {
		t.Errorf("o SFC não deveria rodar depois do cancelamento: %q", got)
	}
	// The next run is free again.
	m.X.Hold = nil
	if _, rep = runRepair(t, c); rep.Verdict != RepairClean {
		t.Errorf("novo reparo: %+v", rep)
	}
}

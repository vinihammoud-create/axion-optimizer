package app

import (
	"encoding/json"
	"testing"
	"time"

	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

const mockDisplay = `\\.\DISPLAY1`

func hzNow(m *mocksys.Mock) int {
	list, _ := m.N.DisplayModes()
	if len(list) == 0 {
		return 0
	}
	return list[0].CurrentHz
}

// applyRefresh scans, starts game.refresh and waits for its question.
func applyRefresh(t *testing.T) (*mocksys.Mock, *client, Job) {
	t.Helper()
	m, c := setupServer(t)
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	c.json("POST", "/api/apply", ApplyRequest{IDs: []string{"game.refresh"}}, &j)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		c.json("GET", "/api/jobs/"+j.ID, nil, &j)
		if j.Confirm != nil || j.Status != JobRunning {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if j.Confirm == nil || j.Confirm.ID == "" || j.Confirm.Prompt == "" || j.Confirm.Seconds != 15 {
		t.Fatalf("a tarefa deveria perguntar (15 s): %+v", j)
	}
	if hzNow(m) != 240 || m.N.DisplayWrites != 0 {
		t.Fatalf("durante a pergunta: %d Hz, %d gravações (esperado 240 Hz, nada gravado)", hzNow(m), m.N.DisplayWrites)
	}
	return m, c, j
}

// follow waits for the job to end without answering anything.
func follow(t *testing.T, c *client, id string) Job {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		var j Job
		c.json("GET", "/api/jobs/"+id, nil, &j)
		if j.Status != JobRunning {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("tarefa não terminou")
	return Job{}
}

func refreshOutcome(t *testing.T, j Job) tweaks.Result {
	t.Helper()
	b, _ := json.Marshal(j.Result)
	var br BatchResult
	_ = json.Unmarshal(b, &br)
	if j.Status != JobDone || len(br.Results) != 1 {
		t.Fatalf("tarefa: %+v", j)
	}
	return br.Results[0]
}

// The state the PC must be in when the new rate was not kept.
func checkWentBack(t *testing.T, m *mocksys.Mock, c *client, r tweaks.Result) {
	t.Helper()
	if r.Outcome != tweaks.OutFailed {
		t.Errorf("sem Manter a operação deveria falhar: %s %s", r.Outcome, r.Message)
	}
	if hzNow(m) != 60 || m.N.DisplayWrites != 0 {
		t.Errorf("o monitor deveria voltar a 60 Hz sem gravar nada: %d Hz, %d gravações", hzNow(m), m.N.DisplayWrites)
	}
	if c.s.App.Engine.ActiveFor("game.refresh") != nil {
		t.Error("o histórico não deveria ficar com o ajuste aplicado")
	}
}

func TestRefreshConfirmKeep(t *testing.T) {
	m, c, j := applyRefresh(t)
	c.json("POST", "/api/jobs/"+j.ID+"/confirm", map[string]any{"confirm": j.Confirm.ID, "keep": true}, nil)
	r := refreshOutcome(t, follow(t, c, j.ID))
	if r.Outcome != tweaks.OutApplied {
		t.Fatalf("Manter deveria aplicar: %s %s", r.Outcome, r.Message)
	}
	if saved, ok := m.N.SavedDisplay(mockDisplay); !ok || saved.CurrentHz != 240 || m.N.DisplayWrites != 1 {
		t.Fatalf("depois de Manter: gravado %+v %v (%d gravações)", saved, ok, m.N.DisplayWrites)
	}
	if c.s.App.Engine.ActiveFor("game.refresh") == nil {
		t.Fatal("o histórico deveria ter o ajuste aplicado")
	}
}

func TestRefreshConfirmRevert(t *testing.T) {
	m, c, j := applyRefresh(t)
	c.json("POST", "/api/jobs/"+j.ID+"/confirm", map[string]any{"confirm": j.Confirm.ID, "keep": false}, nil)
	checkWentBack(t, m, c, refreshOutcome(t, follow(t, c, j.ID)))
}

// The window closing while the question is open: nobody can answer, so it
// goes back at once instead of waiting for the 15 s.
func TestRefreshConfirmWindowClosing(t *testing.T) {
	m, c, _ := applyRefresh(t)
	start := time.Now()
	if !c.s.App.Shutdown(10*time.Second, nil) {
		t.Fatal("a tarefa deveria terminar ao fechar a janela")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("fechar a janela esperou o prazo inteiro: %v", d)
	}
	if hzNow(m) != 60 || m.N.DisplayWrites != 0 {
		t.Errorf("ao fechar sem resposta: %d Hz, %d gravações", hzNow(m), m.N.DisplayWrites)
	}
}

// Nobody answers: the server goes back by itself at the time limit (here
// shortened), without the interface.
func TestRefreshConfirmTimeout(t *testing.T) {
	m, c := setupServer(t)
	c.s.App.Jobs.confirmMax = 150 * time.Millisecond
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	c.json("POST", "/api/apply", ApplyRequest{IDs: []string{"game.refresh"}}, &j)
	var confirmID string
	for start := time.Now(); confirmID == "" && time.Since(start) < 10*time.Second; time.Sleep(5 * time.Millisecond) {
		var cur Job
		c.json("GET", "/api/jobs/"+j.ID, nil, &cur)
		if cur.Confirm != nil {
			confirmID = cur.Confirm.ID
		}
	}
	if confirmID == "" {
		t.Fatal("a tarefa deveria perguntar")
	}
	end := follow(t, c, j.ID)
	checkWentBack(t, m, c, refreshOutcome(t, end))
	// An answer that arrives after the limit changes nothing.
	if code, _ := c.do("POST", "/api/jobs/"+j.ID+"/confirm", map[string]any{"confirm": confirmID, "keep": true}, nil); code != 400 {
		t.Errorf("resposta depois do prazo deveria ser recusada: %d", code)
	}
	if hzNow(m) != 60 || m.N.DisplayWrites != 0 {
		t.Errorf("a resposta atrasada não pode gravar: %d Hz, %d gravações", hzNow(m), m.N.DisplayWrites)
	}
}

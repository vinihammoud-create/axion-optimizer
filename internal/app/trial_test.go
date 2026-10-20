package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"axion/internal/perf"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

// trialServer: a mock PC with the game open and the analysis done. The
// captures of "depois" repeat the random draws of the ones of "antes"
// (PMSeed), so the only difference between the groups is the frame time the
// test asks for (PMBase): the verdicts do not depend on luck.
func trialServer(t *testing.T) (*mocksys.Mock, *client) {
	t.Helper()
	m, c := setupServer(t)
	c.s.App.sensorEvery = 10 * time.Millisecond
	m.N.Procs = append(m.N.Procs, sys.ProcessInfo{PID: 7777, Name: "cs2.exe", WorkingSet: 2 << 30, Session: 1, Path: `C:\Program Files (x86)\Steam\steamapps\common\Counter-Strike Global Offensive\game\bin\win64\cs2.exe`})
	m.PMSeed = func(run int) int { return (run-1)%3 + 1 }
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	return m, c
}

// waitFor is client.wait with its own deadline (six captures take longer than 20 s on a slow PC).
func (c *client) waitFor(id string, d time.Duration) Job {
	c.t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		var j Job
		c.json("GET", "/api/jobs/"+id, nil, &j)
		if j.Status != JobRunning {
			return j
		}
		time.Sleep(20 * time.Millisecond)
	}
	c.t.Fatal("tarefa não terminou")
	return Job{}
}

func startTrial(c *client, id string) Job {
	c.t.Helper()
	var j Job
	c.json("POST", "/api/trial/start", TrialRequest{TweakID: id, Process: "cs2.exe", Seconds: 30, Runs: 3}, &j)
	return c.waitFor(j.ID, 180*time.Second)
}

func trialResultOf(t *testing.T, j Job) TrialResult {
	t.Helper()
	b, _ := json.Marshal(j.Result)
	var r TrialResult
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func tweakView(t *testing.T, c *client, id string) TweakView {
	t.Helper()
	var tw struct {
		Tweaks []TweakView `json:"tweaks"`
	}
	c.json("GET", "/api/tweaks", nil, &tw)
	for _, v := range tw.Tweaks {
		if v.ID == id {
			return v
		}
	}
	t.Fatalf("ajuste %s não está em /api/tweaks", id)
	return TweakView{}
}

func trialRuns(t *testing.T, c *client) []perf.Run {
	t.Helper()
	var pv PerfView
	c.json("GET", "/api/perf", nil, &pv)
	return pv.Runs
}

func TestTrialKeepsWhatImproves(t *testing.T) {
	m, c := trialServer(t)
	m.PMBase = func(run int) float64 {
		if run <= 3 {
			return 2.42
		}
		return 2.20 // cerca de 10% mais FPS depois
	}
	before := m.World()
	j := startTrial(c, "power.plan.high")
	if j.Status != JobDone {
		t.Fatalf("prova: %+v", j)
	}
	r := trialResultOf(t, j)
	if r.Outcome.Verdict != perf.TrialBetter || r.Outcome.Action != perf.TrialKept || r.Outcome.EntryID == "" || len(r.Outcome.RunsA) != 3 || len(r.Outcome.RunsB) != 3 {
		t.Fatalf("resultado: %+v", r.Outcome)
	}
	for _, w := range r.Outcome.Warnings {
		if strings.Contains(w, "plano de energia mudou") {
			t.Errorf("o plano é o próprio ajuste: %s", w)
		}
	}
	if d := mocksys.DiffWorld(before, m.World()); len(d) == 0 {
		t.Error("o plano Alto desempenho deveria estar ativo")
	}
	if e := c.s.App.Engine.J.Get(r.Outcome.EntryID); e == nil || e.BatchLabel != trialBatchLabel || !reTrialID.MatchString(e.Batch) || e.Batch != r.Outcome.ID {
		t.Errorf("histórico: %+v", e)
	}
	// O item do catálogo mostra o resultado e já não pode ser provado (está ativo).
	v := tweakView(t, c, "power.plan.high")
	if v.Trial == nil || v.Trial.Verdict != perf.TrialBetter || v.State.Status != "on" || !v.State.ByAxion || v.TrialOK {
		t.Errorf("item do catálogo: trial=%+v estado=%s trialOk=%v", v.Trial, v.State.Status, v.TrialOK)
	}
	// As 6 medições levam a marca e o resultado.
	tagged := 0
	for _, run := range trialRuns(t, c) {
		if run.Trial != nil && run.Trial.ID == r.Outcome.ID && run.Trial.Outcome != nil && run.Trial.Outcome.Action == perf.TrialKept {
			tagged++
		}
	}
	if tagged != 6 {
		t.Errorf("medições marcadas: %d", tagged)
	}
	var u Job
	c.json("POST", "/api/trial/undo", TrialIDRequest{Trial: r.Outcome.ID}, &u)
	if u = c.wait(u.ID); u.Status != JobDone || u.Kind != "revert" {
		t.Fatalf("desfazer: %+v", u)
	}
	if d := mocksys.DiffWorld(before, m.World()); len(d) != 0 {
		t.Errorf("depois do desfazer: %v", d)
	}
	var tv TrialView
	c.json("GET", "/api/trial", nil, &tv)
	if len(tv.Recent) != 1 || tv.Recent[0].Action != perf.TrialUndone || len(tv.Interrupted) != 0 {
		t.Errorf("recentes: %+v interrompidas: %+v", tv.Recent, tv.Interrupted)
	}
	if code, _ := c.do("POST", "/api/trial/undo", TrialIDRequest{Trial: r.Outcome.ID}, nil); code != 400 {
		t.Errorf("desfazer de novo: %d", code)
	}
}

func TestTrialUndoesWhatDoesNotImprove(t *testing.T) {
	m, c := trialServer(t)
	m.PMBase = func(int) float64 { return 2.42 }
	before := m.World()
	j := startTrial(c, "power.pcie.aspm")
	r := trialResultOf(t, j)
	if j.Status != JobDone || r.Outcome.Verdict != perf.TrialSame || r.Outcome.Action != perf.TrialAutoUndone {
		t.Fatalf("%+v / %+v", j, r.Outcome)
	}
	if d := mocksys.DiffWorld(before, m.World()); len(d) != 0 {
		t.Errorf("o PC deveria estar como antes: %v", d)
	}
	if e := c.s.App.Engine.J.Get(r.Outcome.EntryID); e == nil || e.Status != tweaks.StatusReverted {
		t.Errorf("entrada: %+v", e)
	}
	// A medição de "depois" de um ajuste desfeito não descreve o PC de agora.
	var started time.Time
	for _, run := range trialRuns(t, c) { // do mais novo para o mais antigo
		if run.Trial != nil && run.Trial.Phase == perf.TrialPhaseBefore {
			started = run.Started
			break
		}
	}
	meas := c.s.App.measured()
	if meas.Perf == nil || started.IsZero() || !meas.PerfAt.Equal(started) {
		t.Errorf("diagnóstico: PerfAt=%v, esperado %v", meas.PerfAt, started)
	}
	v := tweakView(t, c, "power.pcie.aspm")
	if v.State.Status != "off" || v.Trial == nil || v.Trial.Action != perf.TrialAutoUndone || !v.TrialOK {
		t.Errorf("item do catálogo: estado=%s trial=%+v trialOk=%v", v.State.Status, v.Trial, v.TrialOK)
	}
}

func TestTrialUndoesWhatGetsWorse(t *testing.T) {
	m, c := trialServer(t)
	m.PMBase = func(run int) float64 {
		if run <= 3 {
			return 2.42
		}
		return 2.70
	}
	before := m.World()
	r := trialResultOf(t, startTrial(c, "power.plan.high"))
	if r.Outcome.Verdict != perf.TrialWorse || r.Outcome.Action != perf.TrialAutoUndone {
		t.Fatalf("%+v", r.Outcome)
	}
	if d := mocksys.DiffWorld(before, m.World()); len(d) != 0 {
		t.Errorf("%v", d)
	}
}

func TestTrialRefusals(t *testing.T) {
	m, c := trialServer(t)
	cases := []struct {
		name string
		req  TrialRequest
		want string
	}{
		{"reinício", TrialRequest{TweakID: "game.hags", Process: "cs2.exe"}, "reiniciar"},
		{"correção", TrialRequest{TweakID: "fix.uac", Process: "cs2.exe"}, "correç"},
		{"desconhecido", TrialRequest{TweakID: "nao.existe", Process: "cs2.exe"}, "desconhecido"},
		{"recomendado", TrialRequest{TweakID: "power.mode.best", Process: "cs2.exe"}, "recomendado"},
		{"uma medição", TrialRequest{TweakID: "power.plan.high", Process: "cs2.exe", Runs: 1}, "entre 2 e 5"},
		{"seis medições", TrialRequest{TweakID: "power.plan.high", Process: "cs2.exe", Runs: 6}, "entre 2 e 5"},
		{"curta", TrialRequest{TweakID: "power.plan.high", Process: "cs2.exe", Seconds: 10}, "entre 30 e 180"},
		{"jogo fechado", TrialRequest{TweakID: "power.plan.high", Process: "r5apex.exe"}, "abra o jogo"},
	}
	for _, tc := range cases {
		if code, b := c.do("POST", "/api/trial/start", tc.req, nil); code != 400 || !strings.Contains(string(b), tc.want) {
			t.Errorf("%s: %d %s", tc.name, code, b)
		}
	}
	m.N.HasBattery, m.N.OnBattery = true, true
	if code, b := c.do("POST", "/api/trial/start", TrialRequest{TweakID: "power.plan.high", Process: "cs2.exe"}, nil); code != 400 || !strings.Contains(string(b), "carregador") {
		t.Errorf("bateria: %d %s", code, b)
	}
	m.N.HasBattery, m.N.OnBattery = false, false
	var j Job
	c.json("POST", "/api/apply", ApplyRequest{IDs: []string{"power.pcie.aspm"}}, &j)
	c.wait(j.ID)
	if code, b := c.do("POST", "/api/trial/start", TrialRequest{TweakID: "power.pcie.aspm", Process: "cs2.exe"}, nil); code != 400 || !strings.Contains(string(b), "já está") {
		t.Errorf("já ativo: %d %s", code, b)
	}
	if m.PMRuns != 0 {
		t.Errorf("nenhuma captura deveria ter rodado: %d", m.PMRuns)
	}
}

func TestTrialCancelUndoes(t *testing.T) {
	m, c := trialServer(t)
	prev := m.X.Handler
	var n atomic.Int32
	m.X.Handler = func(exe string, args []string, ps string) (sys.Result, bool) {
		if strings.EqualFold(filepath.Base(exe), "PresentMon-2.6.0-x64.exe") && !slices.Contains(args, "--terminate_existing_session") && n.Add(1) >= 4 {
			time.Sleep(700 * time.Millisecond) // as capturas "depois" demoram
		}
		return prev(exe, args, ps)
	}
	before := m.World()
	var j Job
	c.json("POST", "/api/trial/start", TrialRequest{TweakID: "power.plan.high", Process: "cs2.exe", Seconds: 30, Runs: 3}, &j)
	deadline := time.Now().Add(120 * time.Second)
	for {
		var cur Job
		c.json("GET", "/api/jobs/"+j.ID, nil, &cur)
		if strings.HasPrefix(cur.Step, "Depois") {
			break
		}
		if cur.Status != JobRunning || time.Now().After(deadline) {
			t.Fatalf("não chegou ao depois: %+v", cur)
		}
		time.Sleep(10 * time.Millisecond)
	}
	c.json("POST", "/api/jobs/"+j.ID+"/cancel", map[string]any{}, nil)
	done := c.waitFor(j.ID, 120*time.Second)
	r := trialResultOf(t, done)
	if done.Status != JobCancelled || r.Applied == nil || r.Outcome.Action != perf.TrialAutoUndone {
		t.Fatalf("%+v / %+v", done, r)
	}
	if d := mocksys.DiffWorld(before, m.World()); len(d) != 0 {
		t.Errorf("%v", d)
	}
	// Nenhuma medição da prova cancelada fica, e a página não a mostra como interrompida.
	for _, run := range trialRuns(t, c) {
		if run.Trial != nil && run.Trial.ID == r.Outcome.ID {
			t.Errorf("medição da prova cancelada ficou: %+v", run.Trial)
		}
	}
	var tv TrialView
	c.json("GET", "/api/trial", nil, &tv)
	if len(tv.Interrupted) != 0 || len(tv.Recent) != 0 {
		t.Errorf("interrompidas %+v, recentes %+v", tv.Interrupted, tv.Recent)
	}
}

func TestTrialDropsShortCapture(t *testing.T) {
	// Uma medição de "depois" que acaba antes do tempo (a tecla apertada de novo)
	// invalida a prova: o ajuste é desfeito e as medições somem.
	m, c := trialServer(t)
	prev := m.X.Handler
	var n atomic.Int32
	m.X.Handler = func(exe string, args []string, ps string) (sys.Result, bool) {
		if strings.EqualFold(filepath.Base(exe), "PresentMon-2.6.0-x64.exe") && !slices.Contains(args, "--terminate_existing_session") && n.Add(1) == 5 {
			for i, a := range args {
				if a == "--timed" {
					args = append([]string(nil), args...)
					args[i+1] = "10" // a gravação dura 10 s dos 30 pedidos
				}
			}
		}
		return prev(exe, args, ps)
	}
	before := m.World()
	j := startTrial(c, "power.plan.high")
	if j.Status != JobFailed || !strings.Contains(j.Error, "durou") || !strings.Contains(j.Error, "desfeito") {
		t.Fatalf("%+v", j)
	}
	r := trialResultOf(t, j)
	if r.Outcome.Action != perf.TrialAutoUndone {
		t.Errorf("%+v", r.Outcome)
	}
	if d := mocksys.DiffWorld(before, m.World()); len(d) != 0 {
		t.Errorf("%v", d)
	}
	for _, run := range trialRuns(t, c) {
		if run.Trial != nil {
			t.Errorf("sobrou medição da prova: %+v", run.Trial)
		}
	}
}

func TestTrialInterrupted(t *testing.T) {
	m, c := trialServer(t)
	a := c.s.App
	before := m.World()
	res := a.Engine.Apply(context.Background(), a.Facts(), []string{"power.plan.high"}, tweaks.ApplyOpts{Batch: "prova-0123456789ab", Label: trialBatchLabel})
	if res[0].Outcome != tweaks.OutApplied {
		t.Fatal(res)
	}
	var tv TrialView
	c.json("GET", "/api/trial", nil, &tv)
	if len(tv.Interrupted) != 1 || tv.Interrupted[0].EntryID != res[0].EntryID {
		t.Fatalf("%+v", tv.Interrupted)
	}
	var j Job
	c.json("POST", "/api/revert", RevertRequest{Entries: []string{res[0].EntryID}}, &j)
	c.wait(j.ID)
	c.json("GET", "/api/trial", nil, &tv)
	if len(tv.Interrupted) != 0 || len(mocksys.DiffWorld(before, m.World())) != 0 {
		t.Errorf("depois de desfazer: %+v", tv.Interrupted)
	}
	a.Engine.Apply(context.Background(), a.Facts(), []string{"power.plan.high"}, tweaks.ApplyOpts{Batch: "prova-ba9876543210", Label: trialBatchLabel})
	c.json("POST", "/api/trial/keep", TrialIDRequest{Trial: "prova-ba9876543210"}, nil)
	c.json("GET", "/api/trial", nil, &tv)
	if len(tv.Interrupted) != 0 {
		t.Errorf("deixado aplicado: %+v", tv.Interrupted)
	}
	if s := a.Settings(); !slices.Contains(s.KeptTrials, "prova-ba9876543210") || len(s.Dismissed) != 0 {
		t.Errorf("configurações: %+v", s)
	}
	if code, _ := c.do("POST", "/api/trial/keep", TrialIDRequest{Trial: "../x"}, nil); code != 400 {
		t.Errorf("id inválido: %d", code)
	}
}

func TestTrialCandidates(t *testing.T) {
	_, c := trialServer(t)
	var tv TrialView
	c.json("GET", "/api/trial", nil, &tv)
	if !tv.Ready || !tv.Elevated || tv.Rule == "" || tv.Honest == "" || len(tv.Sources) == 0 {
		t.Errorf("página: %+v", tv)
	}
	by := map[string]TrialCandidate{}
	seenNo := false
	for _, x := range tv.Candidates {
		by[x.ID] = x
		if x.Category == "correcoes" {
			t.Errorf("correção na lista: %s", x.ID)
		}
		if x.OK && seenNo {
			t.Errorf("%s: os que podem ser provados vêm antes dos outros", x.ID)
		}
		if !x.OK {
			seenNo = true
		}
		if !x.OK && x.Why == "" {
			t.Errorf("%s: recusado sem motivo", x.ID)
		}
	}
	if x := by["power.plan.high"]; !x.OK {
		t.Errorf("power.plan.high: %+v", x)
	}
	if x := by["game.hags"]; x.OK || !strings.Contains(x.Why, "reiniciar") {
		t.Errorf("game.hags: %+v", x)
	}
	if x := by["power.mode.best"]; x.OK || !strings.Contains(x.Why, "recomendado") {
		t.Errorf("power.mode.best: %+v", x)
	}
	// O ajuste de taxa de atualização muda uma condição da medição.
	if x, ok := by["game.refresh"]; ok && x.OK {
		t.Errorf("game.refresh não deveria entrar: %+v", x)
	}
}

package app

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"axion/internal/perf"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

// startSessionTest prepares the match report over the simulator: the analysis
// (GPU for nvidia-smi and the game library), short waits, and a PresentMon
// that ends at once when the game is not in the process list.
func startSessionTest(t *testing.T) (*mocksys.Mock, *client, *App) {
	t.Helper()
	m, c := setupServer(t)
	scan(t, c)
	a := c.s.App
	a.sensorEvery = 10 * time.Millisecond
	a.sessTimingOverride = &sessionTiming{Poll: 20 * time.Millisecond, Segment: 10 * time.Second, Max: time.Hour, MinRec: 15 * time.Second, Gone: 2}
	t.Cleanup(func() { a.StopSessions(10 * time.Second) })
	m.PMNeedsProc = true
	return m, c, a
}

var cs2Proc = sys.ProcessInfo{PID: 7777, Name: "cs2.exe", Session: 1, Priority: 0x20, Path: `D:\SteamLibrary\steamapps\common\Counter-Strike Global Offensive\game\bin\win64\cs2.exe`}

// waitReports waits until n reports exist and no session is being recorded.
func waitReports(t *testing.T, c *client, n int) SessionsView {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var v SessionsView
		c.json("GET", "/api/sessions", nil, &v)
		if len(v.Reports) >= n && v.Live == nil {
			return v
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("o relatório %d não saiu", n)
	return SessionsView{}
}

// sessionHook wraps a hook so that it only sees the windows of the match
// report (never the manual measurement or anything else), numbered from 1.
func sessionHook(k *atomic.Int32, fn func(n int32)) func(int, []string) {
	return func(_ int, args []string) {
		if !slices.Contains(args, sessionPMName) {
			return
		}
		fn(k.Add(1))
	}
}

func TestSessionReportLifecycle(t *testing.T) {
	m, c, a := startSessionTest(t)
	world0 := m.World()
	calls0 := len(m.X.Calls)
	timer0, purges0 := m.N.Timer, m.N.Purges
	var k atomic.Int32
	m.PMHook = sessionHook(&k, func(n int32) {
		switch n {
		case 2: // the game must still be there, untouched
			list, _ := m.N.Processes()
			for _, p := range list {
				if strings.EqualFold(p.Name, "cs2.exe") && p.Priority != 0x20 {
					t.Errorf("prioridade do jogo mudou: %#x", p.Priority)
				}
			}
			if len(m.N.PIDsByName("cs2.exe")) != 1 {
				t.Errorf("o jogo sumiu da lista na janela 2")
			}
		case 4, 8: // the game closes in the middle of window 4 (and 8)
			m.N.RemoveProcs("cs2.exe")
		}
	})
	m.N.AddProc(cs2Proc) // already open before the report is turned on: Late
	var v SessionsView
	c.json("POST", "/api/sessions/auto", AutoReportRequest{Exe: "cs2.exe", On: true}, &v)
	var cs2 *SessionGame
	for i := range v.Games {
		if v.Games[i].Exe == "cs2.exe" {
			cs2 = &v.Games[i]
		}
	}
	if cs2 == nil || !cs2.Auto || !cs2.Competitive || cs2.AntiCheat != "VAC" {
		t.Fatalf("CS2 na lista: %+v", v.Games)
	}
	if got := a.Settings().AutoReport; len(got) != 1 || got[0] != "cs2.exe" {
		t.Fatalf("configuração: %v", got)
	}
	v = waitReports(t, c, 1)
	var rep SessionReport
	c.json("GET", "/api/sessions/run/"+v.Reports[0].ID, nil, &rep)
	r := rep.Run
	if r.Session == nil || r.Session.Reason != "fim" || len(r.Session.Segments) != 3 || !r.Session.Late || !strings.Contains(r.Note, "já estava aberto") {
		t.Fatalf("sessão: %+v / %q", r.Session, r.Note)
	}
	s := r.Summary
	if s.Seconds < 29 || s.Seconds > 31 || s.Frames < 10000 || s.Low1FPS <= 0 || s.Low01FPS <= 0 || s.Limiter != "gpu" || len(r.Series) != 1200 {
		t.Fatalf("números: %+v (série %d)", s, len(r.Series))
	}
	if r.Sensors == nil || r.Sensors.GPU == nil || r.Sensors.GPU.MaxTempC != 71 || r.Conditions == nil {
		t.Fatalf("sensores e condições: %+v %+v", r.Sensors, r.Conditions)
	}
	if rep.Previous != nil || rep.Compare != nil {
		t.Error("a primeira partida não tem anterior")
	}
	first := r.ID
	var st StateResp
	c.json("GET", "/api/state", nil, &st)
	if len(st.Unseen) != 1 || st.Unseen[0].ID != first {
		t.Fatalf("aviso de relatório novo: %+v", st.Unseen)
	}
	c.json("POST", "/api/sessions/seen", PerfIDRequest{ID: first}, nil)
	var st2 StateResp // a fresh value: Unmarshal keeps the fields the JSON leaves out
	c.json("GET", "/api/state", nil, &st2)
	if len(st2.Unseen) != 0 {
		t.Errorf("depois de abrir, o aviso deveria sumir: %+v", st2.Unseen)
	}
	if csv, _ := filepath.Glob(filepath.Join(a.sessionDir(), "*.csv")); len(csv) != 0 {
		t.Errorf("CSV de janela esquecido: %v", csv)
	}
	if n := len(perf.List(a.perfDir())); n != 0 {
		t.Errorf("a partida apareceu na lista da medição manual: %d", n)
	}

	// Second session: Axion has seen the game closed, so it is not late. The
	// wait makes sure the watcher looked at the empty list at least twice.
	p0 := v.Polls
	deadline := time.Now().Add(5 * time.Second)
	for {
		var cur SessionsView
		c.json("GET", "/api/sessions", nil, &cur)
		if cur.Live == nil && cur.Polls >= p0+2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("o Axion não voltou a procurar o jogo")
		}
		time.Sleep(20 * time.Millisecond)
	}
	m.N.AddProc(cs2Proc)
	v = waitReports(t, c, 2)
	var rep2 SessionReport
	c.json("GET", "/api/sessions/run/"+v.Reports[0].ID, nil, &rep2)
	rep = rep2
	if rep.Run.ID == first || rep.Run.Session.Late {
		t.Fatalf("segunda partida: %+v", rep.Run.Session)
	}
	if rep.Previous == nil || rep.Previous.ID != first || rep.Compare == nil || !rep.Compare.Enough || len(rep.Compare.Rows) == 0 {
		t.Fatalf("comparação: %+v", rep.Compare)
	}
	if rep.Compare.LabelA != "partida anterior" || len(rep.Compare.Warnings) == 0 || !strings.Contains(rep.Compare.Warnings[0], "não medições repetidas") {
		t.Errorf("avisos: %v", rep.Compare.Warnings)
	}
	for _, row := range rep.Compare.Rows {
		if row.Verdict != "indicativo" || row.PAdj != -1 || row.P != -1 {
			t.Errorf("a linha %s não deveria ter veredito estatístico: %+v", row.Key, row)
		}
	}
	for _, w := range rep.Compare.Warnings {
		if strings.HasPrefix(w, "Com 2 medições") || strings.HasPrefix(w, "As medições do grupo") {
			t.Errorf("aviso de medição repetida numa partida: %q", w)
		}
	}

	a.StopSessions(10 * time.Second)
	// Only PresentMon and nvidia-smi ran since the start, and every PresentMon
	// used its own ETW session.
	for _, call := range m.X.Calls[calls0:] {
		exe := strings.ToLower(filepath.Base(call.Exe))
		if exe != strings.ToLower(presentMonName) && exe != "nvidia-smi.exe" {
			t.Errorf("programa inesperado durante a partida: %s %v", call.Exe, call.Args)
		}
		if exe == strings.ToLower(presentMonName) && !slices.Contains(call.Args, sessionPMName) {
			t.Errorf("PresentMon sem a sessão própria: %v", call.Args)
		}
	}
	if d := mocksys.DiffWorld(world0, m.World()); len(d) > 0 {
		t.Errorf("o relatório da partida alterou o PC: %v", d)
	}
	if m.N.Timer != timer0 || m.N.Purges != purges0 {
		t.Errorf("timer ou memória mudaram: %+v %d", m.N.Timer, m.N.Purges)
	}
}

// A manual measurement has priority: it cuts the window being recorded (what
// was recorded is kept) and the report goes on after it.
func TestSessionConflictsWithManualCapture(t *testing.T) {
	m, c, a := startSessionTest(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	rel := func() { once.Do(func() { close(release) }) }
	t.Cleanup(rel)
	var k atomic.Int32
	m.PMHook = sessionHook(&k, func(n int32) {
		switch n {
		case 1:
			close(entered)
			<-release
		case 3:
			m.N.RemoveProcs("cs2.exe")
		}
	})
	m.N.AddProc(cs2Proc)
	c.json("POST", "/api/sessions/auto", AutoReportRequest{Exe: "cs2.exe", On: true}, nil)
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("a janela 1 não começou")
	}
	if a.pmUser() != "sessao" {
		t.Fatalf("dono do PresentMon: %q", a.pmUser())
	}
	var j Job
	c.json("POST", "/api/perf/capture", PerfCaptureRequest{Process: "cs2.exe", Seconds: 20, Mode: "delay", Delay: 3}, &j)
	var st StateResp
	c.json("GET", "/api/state", nil, &st)
	if st.Session == nil || st.Session.Exe != "cs2.exe" {
		t.Fatalf("estado durante a gravação: %+v", st.Session)
	}
	rel()
	if done := c.wait(j.ID); done.Status != JobDone {
		t.Fatalf("medição manual: %+v", done)
	}
	v := waitReports(t, c, 1)
	var rep SessionReport
	c.json("GET", "/api/sessions/run/"+v.Reports[0].ID, nil, &rep)
	if rep.Run.Session == nil || !rep.Run.Session.Paused || !strings.Contains(rep.Run.Note, "medição manual") {
		t.Fatalf("a partida deveria registrar a pausa: %+v %q", rep.Run.Session, rep.Run.Note)
	}
	if a.pmUser() != "" {
		t.Errorf("PresentMon ficou com %q", a.pmUser())
	}
	if n := len(perf.List(a.perfDir())); n != 1 {
		t.Errorf("a medição manual deveria estar salva à parte: %d", n)
	}
	// The claim itself.
	if !a.claimPM("perf") || a.claimPM("sessao") {
		t.Fatal("dono do PresentMon")
	}
	a.releasePM("perf")
	if !a.claimPM("sessao") || a.claimPM("perf") {
		t.Fatal("dono do PresentMon (partida)")
	}
	a.releasePM("sessao")
	if a.pmUser() != "" {
		t.Fatal("PresentMon deveria estar livre")
	}
}

func TestSessionStopsWithAxion(t *testing.T) {
	m, c, a := startSessionTest(t)
	a.sessTimingOverride.MinRec = 5 * time.Second
	stopped := make(chan bool, 1)
	var k atomic.Int32
	m.PMHook = sessionHook(&k, func(n int32) {
		if n == 2 {
			go func() { stopped <- a.StopSessions(10 * time.Second) }()
		}
	})
	m.N.AddProc(cs2Proc)
	c.json("POST", "/api/sessions/auto", AutoReportRequest{Exe: "cs2.exe", On: true}, nil)
	select {
	case ok := <-stopped:
		if !ok {
			t.Fatal("StopSessions não terminou")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("StopSessions não respondeu")
	}
	select {
	case <-a.sess.done:
	default:
		t.Fatal("a goroutine de detecção continua viva")
	}
	if a.sessionLive() != nil || a.pmUser() != "" {
		t.Errorf("sobrou estado: %+v %q", a.sessionLive(), a.pmUser())
	}
	runs := perf.List(a.sessionDir())
	if len(runs) != 1 || runs[0].Session == nil || runs[0].Session.Reason != "fechado" || len(runs[0].Session.Segments) < 2 || !strings.Contains(runs[0].Note, "fechado durante o jogo") {
		t.Fatalf("relatório do encerramento: %+v", runs)
	}
	if csv, _ := filepath.Glob(filepath.Join(a.sessionDir(), "*.csv")); len(csv) != 0 {
		t.Errorf("CSV de janela esquecido: %v", csv)
	}
	// After the stop the watcher never comes back.
	c.json("POST", "/api/sessions/auto", AutoReportRequest{Exe: "cs2.exe", On: true}, nil)
	a.sess.mu.Lock()
	again := a.sess.cancel
	a.sess.mu.Unlock()
	if again != nil {
		t.Error("o acompanhamento voltou depois de StopSessions")
	}
}

func TestSessionFailureHoldsUntilGameCloses(t *testing.T) {
	m, c, a := startSessionTest(t)
	var k atomic.Int32
	m.PMHook = sessionHook(&k, func(int32) {})
	m.PMFail = "falha simulada"
	m.N.AddProc(cs2Proc)
	c.json("POST", "/api/sessions/auto", AutoReportRequest{Exe: "cs2.exe", On: true}, nil)
	deadline := time.Now().Add(10 * time.Second)
	for k.Load() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("o PresentMon deveria ter sido tentado 3 vezes: %d", k.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	if k.Load() != 3 {
		t.Errorf("recomeçou a gravar com o jogo ainda aberto: %d tentativas", k.Load())
	}
	var v SessionsView
	c.json("GET", "/api/sessions", nil, &v)
	if len(v.Reports) != 0 || v.Live != nil {
		t.Errorf("sem quadros não há relatório: %+v", v)
	}
	c.json("POST", "/api/sessions/auto", AutoReportRequest{Exe: "cs2.exe", On: false}, nil)
	if got := a.Settings().AutoReport; len(got) != 0 {
		t.Errorf("configuração: %v", got)
	}
}

// Before the analysis there is no GPU identified and no library: the report
// does not start recording and says why.
func TestSessionWaitsForAnalysis(t *testing.T) {
	m, c := setupServer(t)
	a := c.s.App
	a.sessTimingOverride = &sessionTiming{Poll: 20 * time.Millisecond, Segment: 10 * time.Second, Max: time.Hour, MinRec: 15 * time.Second, Gone: 2}
	t.Cleanup(func() { a.StopSessions(10 * time.Second) })
	m.PMNeedsProc = true
	var k atomic.Int32
	m.PMHook = sessionHook(&k, func(int32) {})
	m.N.AddProc(cs2Proc)
	var v SessionsView
	c.json("POST", "/api/sessions/auto", AutoReportRequest{Exe: "cs2.exe", On: true}, &v)
	if v.Ready || !strings.Contains(v.Why, "análise") {
		t.Errorf("antes da análise: ready=%v why=%q", v.Ready, v.Why)
	}
	time.Sleep(250 * time.Millisecond)
	if k.Load() != 0 || a.sessionLive() != nil {
		t.Errorf("gravou sem a análise: %d", k.Load())
	}
	if p := a.Settings().AutoReport; len(p) != 1 {
		t.Errorf("configuração: %v", p)
	}
}

func TestAutoReportValidation(t *testing.T) {
	_, c, a := startSessionTest(t)
	for _, bad := range []string{"../x.exe", "-o.exe", ""} {
		if code, _ := c.do("POST", "/api/sessions/auto", AutoReportRequest{Exe: bad, On: true}, nil); code != 400 {
			t.Errorf("%q: %d", bad, code)
		}
	}
	code, body := c.do("POST", "/api/sessions/auto", AutoReportRequest{Exe: "jogoquenaoexiste.exe", On: true}, nil)
	if code != 400 || !strings.Contains(string(body), "não está na biblioteca") {
		t.Errorf("jogo fora da biblioteca: %d %s", code, body)
	}
	c.json("POST", "/api/sessions/auto", AutoReportRequest{Exe: "CS2", On: true}, nil) // no .exe, upper case
	c.json("POST", "/api/sessions/auto", AutoReportRequest{Exe: "cs2.exe", On: true}, nil)
	if got := a.Settings().AutoReport; len(got) != 1 || got[0] != "cs2.exe" {
		t.Fatalf("ligar de novo não pode duplicar: %v", got)
	}
	// A game that left the library can still be turned off, and stays on the page while it is on.
	if err := a.UpdateSettings(func(s *Settings) { s.AutoReport = append(s.AutoReport, "sumiu.exe") }); err != nil {
		t.Fatal(err)
	}
	var v SessionsView
	c.json("GET", "/api/sessions", nil, &v)
	found := false
	for _, g := range v.Games {
		found = found || (g.Exe == "sumiu.exe" && g.Auto)
	}
	if !found {
		t.Errorf("jogo ligado fora da biblioteca sumiu da página: %+v", v.Games)
	}
	c.json("POST", "/api/sessions/auto", AutoReportRequest{Exe: "sumiu.exe", On: false}, nil)
	c.json("POST", "/api/sessions/auto", AutoReportRequest{Exe: "cs2.exe", On: false}, nil)
	if got := a.Settings().AutoReport; len(got) != 0 {
		t.Errorf("desligar deveria esvaziar: %v", got)
	}
	if code, _ := c.do("GET", "/api/sessions/run/abc", nil, nil); code != 400 {
		t.Errorf("id inválido: %d", code)
	}
	if code, _ := c.do("GET", "/api/sessions/run/20260101-000000-abcdef", nil, nil); code != 400 {
		t.Errorf("partida inexistente: %d", code)
	}
}

func TestDurText(t *testing.T) {
	for in, want := range map[int]string{10: "10 s", 59: "59 s", 60: "1 min", 180: "3 min", 90: "1 min 30 s", 3600: "1 h", 10800: "3 h", 5400: "90 min"} {
		if got := durText(in); got != want {
			t.Errorf("durText(%d) = %q, esperado %q", in, got, want)
		}
	}
	n := sessionNote(perf.SessionInfo{Reason: "limite"}, 3*time.Hour)
	if !strings.Contains(n, "primeiras 3 h da partida") {
		t.Errorf("nota do limite: %q", n)
	}
}

// The automatic report never touches the game process: it only looks at the
// process list by name and runs PresentMon and nvidia-smi.
func TestSessionCodeNeverTouchesTheGame(t *testing.T) {
	b, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"Native.Processes(", "perfProcesses(", "KillProcess", "SetPriorityClass", "SuspendProcess", "ResumeProcess", "OpenProcess",
		"WriteProcessMemory", "ReadProcessMemory", "CreateRemoteThread", "SetWindowsHookEx", "MessageBox", "SetTimerResolution", "PurgeStandbyList"} {
		if bytes.Contains(b, []byte(bad)) {
			t.Errorf("session.go usa %s", bad)
		}
	}
}

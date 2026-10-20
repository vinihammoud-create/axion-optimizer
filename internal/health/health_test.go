package health_test

import (
	"strings"
	"testing"
	"time"

	"axion/internal/facts"
	"axion/internal/health"
)

var now = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func samples(mainMs ...int) []facts.BootSample {
	var out []facts.BootSample
	for i, ms := range mainMs {
		out = append(out, facts.BootSample{At: now.Add(-time.Duration(i+1) * 26 * time.Hour), TotalMs: ms + 11000, MainMs: ms})
	}
	return out
}

// healthy is a desktop with everything in order.
func healthy() *facts.Facts {
	f := &facts.Facts{}
	f.OS.LastBoot, f.OS.UptimeHours = now.Add(-30*time.Hour), 30
	f.Security.DefenderRealtime = "on"
	f.Health = facts.HealthInfo{
		Defender: facts.DefenderHealth{Read: true, Enabled: true, RealTime: true, Tamper: true, Mode: "Normal", SigAgeDays: 1,
			ExclusionsRead: true, ExclPaths: []string{`D:\Jogos\Steam`}},
		DirtyRead: true,
		Updates:   facts.UpdateHealth{Read: true, Last: now.Add(-12 * 24 * time.Hour), LastKB: "5121794"},
		Boot:      facts.BootHealth{Read: true, Samples: samples(14200, 13100, 12200, 13800, 15100)},
	}
	return f
}

func laptop(f *facts.Facts, wear int) {
	f.Power.IsLaptop = true
	f.Health.Battery = facts.BatteryHealth{Read: true, DesignMWh: 54880, FullMWh: 54880 * (100 - wear) / 100, Cycles: 32, WearPct: wear}
}

func byID(cs []health.Check, id string) *health.Check {
	for i := range cs {
		if cs[i].ID == id {
			return &cs[i]
		}
	}
	return nil
}

func TestHealthyMachineIsAllOK(t *testing.T) {
	cs := health.Evaluate(healthy(), now)
	for _, c := range cs {
		if c.Status != health.OK {
			t.Errorf("%s deveria estar OK: %+v", c.ID, c)
		}
		if c.Value == "" {
			t.Errorf("%s sem o valor lido", c.ID)
		}
	}
	want := []string{"defender.realtime", "defender.signatures", "defender.tamper", "defender.exclusions", "update.recent", "update.failing",
		"disk.dirty", "restart.pending", "restart.uptime", "boot.time", "boot.apps"}
	for _, id := range want {
		if byID(cs, id) == nil {
			t.Errorf("falta a verificação %s", id)
		}
	}
	// A desktop has no battery check at all.
	if byID(cs, "battery.wear") != nil {
		t.Error("desktop não tem verificação de bateria")
	}
	if s := health.Summarize(cs); s.Attention != 0 || s.Unread != 0 || s.OK != len(want) {
		t.Errorf("resumo: %+v", s)
	}
	// The page keeps the groups together, in order.
	last := -1
	for _, c := range cs {
		idx := -1
		for i, g := range health.Groups {
			if g == c.Group {
				idx = i
			}
		}
		if idx < last {
			t.Errorf("grupos fora de ordem em %s", c.ID)
		}
		last = idx
	}
}

func TestEachProblemNeedsAttention(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(f *facts.Facts)
		id    string
		want  health.Status
		sev   string
		match string // in the value
	}{
		{"assinaturas de 7 dias", func(f *facts.Facts) { f.Health.Defender.SigAgeDays = 7 }, "defender.signatures", health.Attention, health.SevWarning, "7 dias"},
		{"assinaturas de 6 dias", func(f *facts.Facts) { f.Health.Defender.SigAgeDays = 6 }, "defender.signatures", health.OK, "", "6 dias"},
		{"proteção contra violação desligada", func(f *facts.Facts) { f.Health.Defender.Tamper = false }, "defender.tamper", health.Attention, health.SevInfo, "desligada"},
		{"exclusão ampla", func(f *facts.Facts) { f.Health.Defender.ExclPaths = []string{`C:\`, `D:\Jogos`} }, "defender.exclusions", health.Attention, health.SevWarning, `pasta C:\`},
		{"exclusão de jogo é aceita", func(f *facts.Facts) { f.Health.Defender.ExclPaths = []string{`D:\Jogos`, `C:\Games\CS2`} }, "defender.exclusions", health.OK, "", "específicas"},
		{"sem exclusões", func(f *facts.Facts) { f.Health.Defender.ExclPaths = nil }, "defender.exclusions", health.OK, "", "nenhuma"},
		{"90 dias sem atualização", func(f *facts.Facts) { f.Health.Updates.Last = now.Add(-90 * 24 * time.Hour) }, "update.recent", health.Attention, health.SevWarning, "KB5121794"},
		{"89 dias sem atualização", func(f *facts.Facts) { f.Health.Updates.Last = now.Add(-89 * 24 * time.Hour) }, "update.recent", health.OK, "", "89 dias"},
		{"falha depois do último sucesso", func(f *facts.Facts) {
			f.Health.Updates.Fail, f.Health.Updates.FailKB, f.Health.Updates.FailCode = now.Add(-2*24*time.Hour), "5129195", "0x800f0820"
		}, "update.failing", health.Attention, health.SevWarning, "0x800f0820"},
		{"falha antiga já resolvida", func(f *facts.Facts) {
			f.Health.Updates.Fail, f.Health.Updates.FailKB = now.Add(-30*24*time.Hour), "5119001"
		}, "update.failing", health.OK, "", "resolvida"},
		{"volume sujo", func(f *facts.Facts) { f.Health.Dirty = []string{"C:", "D:"} }, "disk.dirty", health.Attention, health.SevWarning, "C:, D:"},
		{"bateria com 40% de desgaste", func(f *facts.Facts) { laptop(f, 40) }, "battery.wear", health.Attention, health.SevWarning, "40%"},
		{"bateria com 39% de desgaste", func(f *facts.Facts) { laptop(f, 39) }, "battery.wear", health.OK, "", "39%"},
		{"boot de 60 s", func(f *facts.Facts) { f.Health.Boot.Samples = samples(60000, 60000, 60000, 30000, 90000) }, "boot.time", health.Attention, health.SevInfo, "60,0 s"},
		{"boot de 59,9 s", func(f *facts.Facts) { f.Health.Boot.Samples = samples(59900, 59900, 59900) }, "boot.time", health.OK, "", "59,9 s"},
		{"programa lento", func(f *facts.Facts) {
			f.Health.Boot.Slow = []facts.BootApp{{Name: "OCControl.Service.exe", Boots: 4, WorstMs: 7300}}
		}, "boot.apps", health.Attention, health.SevInfo, "OCControl.Service.exe (atrasou 4 boots, até 7,3 s)"},
		{"reinício pendente", func(f *facts.Facts) { f.OS.PendingReboot = true }, "restart.pending", health.Attention, health.SevWarning, ""},
		{"uma semana e meia ligado", func(f *facts.Facts) { f.OS.UptimeHours = 24 * 9 }, "restart.uptime", health.Attention, health.SevInfo, "9 dias"},
		{"defender em tempo real desligado", func(f *facts.Facts) { f.Security.DefenderRealtime = "off" }, "defender.realtime", health.Attention, health.SevInfo, "desligada"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := healthy()
			tc.edit(f)
			c := byID(health.Evaluate(f, now), tc.id)
			if c == nil {
				t.Fatalf("sem a verificação %s", tc.id)
			}
			if c.Status != tc.want || c.Severity != tc.sev || !strings.Contains(c.Value, tc.match) {
				t.Errorf("%+v", c)
			}
			if c.Status == health.Attention && (c.Headline == "" || c.Detail == "") {
				t.Errorf("atenção sem título ou explicação: %+v", c)
			}
		})
	}
}

// What was not read is never a problem and never "healthy": it says so.
func TestUnreadIsNeitherProblemNorHealthy(t *testing.T) {
	f := &facts.Facts{}
	f.Power.IsLaptop = true // the battery check applies, but nothing was read
	f.OS.LastBoot, f.OS.UptimeHours = now.Add(-time.Hour), 1
	f.Security.DefenderRealtime = "unknown"
	cs := health.Evaluate(f, now)
	for _, c := range cs {
		switch c.ID {
		case "restart.pending", "restart.uptime":
			continue // read from the OS step, which was read
		}
		if c.Status != health.Unread || c.Detail == "" || c.Value != "não lido" {
			t.Errorf("%s: %+v", c.ID, c)
		}
	}
	if s := health.Summarize(cs); s.Attention != 0 {
		t.Errorf("nada lido não pode ter atenção: %+v", s)
	}

	// The whole step set aside after a crash: even filled fields are not trusted.
	f2 := healthy()
	f2.Health.Defender.SigAgeDays = 40
	f2.Unread = []string{facts.StepHealth}
	for _, c := range health.Evaluate(f2, now) {
		if strings.HasPrefix(c.ID, "defender.signatures") && c.Status != health.Unread {
			t.Errorf("etapa não lida: %+v", c)
		}
	}
}

func TestSpecificUnreadReasons(t *testing.T) {
	f := healthy()
	f.Health.Defender.ExclusionsRead = false
	f.Health.Defender.SigAgeDays = -1
	f.Health.Updates.Last = time.Time{}
	f.Health.Boot.Samples = samples(14000, 13000) // two boots: too few
	cs := health.Evaluate(f, now)
	for id, frag := range map[string]string{
		"defender.exclusions": "administrador",
		"defender.signatures": "idade",
		"update.recent":       "180 dias",
		"boot.time":           "pelo menos 3",
		"boot.apps":           "Poucos boots",
	} {
		c := byID(cs, id)
		if c == nil || c.Status != health.Unread || !strings.Contains(c.Detail, frag) {
			t.Errorf("%s: %+v", id, c)
		}
	}
}

// A Defender in passive mode beside another antivirus is not the one
// protecting: its signatures and tamper setting say nothing.
func TestPassiveDefenderIsNotJudged(t *testing.T) {
	f := healthy()
	f.Health.Defender.Mode, f.Health.Defender.ThirdParty = "Passive Mode", "Kaspersky"
	f.Health.Defender.SigAgeDays, f.Health.Defender.Tamper = 45, false
	cs := health.Evaluate(f, now)
	for _, id := range []string{"defender.signatures", "defender.tamper"} {
		if c := byID(cs, id); c == nil || c.Status != health.OK {
			t.Errorf("%s: %+v", id, c)
		}
	}
	if c := byID(cs, "defender.signatures"); !strings.Contains(c.Value, "Kaspersky") {
		t.Errorf("deveria dizer quem protege: %q", c.Value)
	}
	// Real-time off with another antivirus registered is not a finding either.
	f.Security.DefenderRealtime, f.Health.Defender.RealTime = "off", false
	if c := byID(health.Evaluate(f, now), "defender.realtime"); c.Status != health.OK || !strings.Contains(c.Value, "Kaspersky") {
		t.Errorf("tempo real com outro antivírus: %+v", c)
	}
}

// What the older diagnostic rules already report is marked so the diagnostic
// does not repeat it.
func TestCoveredChecks(t *testing.T) {
	f := healthy()
	f.OS.PendingReboot = true
	f.OS.UptimeHours = 24 * 20
	f.Security.DefenderRealtime = "off"
	f.Health.Defender.ThirdParty = ""
	cs := health.Evaluate(f, now)
	for id, want := range map[string]string{"restart.pending": "os.reboot", "restart.uptime": "os.uptime", "defender.realtime": "sec.defender"} {
		if c := byID(cs, id); c == nil || c.Covered != want || c.Status != health.Attention {
			t.Errorf("%s: %+v", id, c)
		}
	}
	for _, c := range cs {
		if c.Covered == "" && (strings.HasPrefix(c.ID, "restart.") || c.ID == "defender.realtime") {
			t.Errorf("%s deveria apontar o achado que já o cobre", c.ID)
		}
	}
}

func TestBroadExclusions(t *testing.T) {
	broad := facts.DefenderHealth{
		ExclPaths: []string{`C:`, `c:\`, `C:\*`, `D:\`, `C:\Users`, `C:\Users\*`, `C:\Users\joao`, `C:\Users\joao\Downloads`, `C:\Users\joao\AppData\Local\Temp`,
			`C:\Windows`, `c:/windows/system32`, `C:\Temp`, `C:\Program Files`, `%USERPROFILE%`, `%systemdrive%`, `*`},
		ExclExts:  []string{".exe", "DLL", "ps1", ".bat", "*"},
		ExclProcs: []string{"powershell.exe", `C:\Windows\System32\cmd.exe`, `C:\Program Files\Java\bin\java.exe`, "SVCHOST.EXE"},
	}
	got := health.BroadExclusions(broad)
	want := len(broad.ExclPaths) + len(broad.ExclExts) + len(broad.ExclProcs)
	if len(got) != want {
		t.Errorf("%d de %d amplas: %v", len(got), want, got)
	}
	narrow := facts.DefenderHealth{
		ExclPaths: []string{`D:\Jogos\Steam`, `C:\Games\CS2`, `C:\Users\joao\Documents\Projeto`, `C:\Program Files\Valve`, `C:\Users\joao\AppData\Local\Programs\App`, `\\servidor\share`},
		ExclExts:  []string{".log", "vmdk", ".iso"},
		ExclProcs: []string{`C:\Games\game.exe`, "obs64.exe", "steam.exe"},
	}
	if got := health.BroadExclusions(narrow); len(got) != 0 {
		t.Errorf("exclusões específicas marcadas como amplas: %v", got)
	}
}

// The title and the value read must say the same number of days.
func TestUptimeTitleAndValueAgree(t *testing.T) {
	f := healthy()
	f.OS.UptimeHours = 24 * 10.5
	c := byID(health.Evaluate(f, now), "restart.uptime")
	if !strings.HasPrefix(c.Headline, "11 dias") || !strings.HasSuffix(c.Value, "há 11 dias") {
		t.Errorf("título %q, valor %q", c.Headline, c.Value)
	}
}

func TestMedianBoot(t *testing.T) {
	// Newest first; only the five newest count, the median is the middle one.
	b := facts.BootHealth{Samples: samples(10000, 90000, 12000, 11000, 95000, 500000, 500000)}
	b.Samples[1].Degraded = true
	med, n, deg := health.MedianBoot(b)
	if med != 12000 || n != 5 || deg != 1 {
		t.Errorf("mediana %d, boots %d, degradados %d", med, n, deg)
	}
	if med, n, _ := health.MedianBoot(facts.BootHealth{}); med != 0 || n != 0 {
		t.Errorf("sem boots: %d %d", med, n)
	}
}

func TestWindowsFlaggedBootsAreMentioned(t *testing.T) {
	f := healthy()
	f.Health.Boot.Samples[0].Degraded = true
	f.Health.Boot.Samples[2].Degraded = true
	if c := byID(health.Evaluate(f, now), "boot.time"); !strings.Contains(c.Value, "marcou 2 como mais lento") {
		t.Errorf("%q", c.Value)
	}
}

// Every check that sends the user somewhere cites where its limit or its data
// comes from.
func TestChecksCarrySourcesAndCriteria(t *testing.T) {
	f := healthy()
	laptop(f, 10)
	for _, c := range health.Evaluate(f, now) {
		if c.Covered != "" {
			continue
		}
		if len(c.Sources) == 0 {
			t.Errorf("%s sem fonte", c.ID)
		}
		for _, s := range c.Sources {
			if !strings.HasPrefix(s.URL, "https://") || s.Title == "" {
				t.Errorf("%s: fonte inválida %+v", c.ID, s)
			}
		}
		if c.ID != "defender.tamper" && c.ID != "update.failing" && c.ID != "disk.dirty" && c.Criterion == "" {
			t.Errorf("%s sem o critério dito como do Axion", c.ID)
		}
		if c.Criterion != "" && !strings.HasPrefix(c.Criterion, "Critério do Axion") {
			t.Errorf("%s: critério sem dizer de quem é: %q", c.ID, c.Criterion)
		}
	}
}

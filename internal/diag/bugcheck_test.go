package diag_test

import (
	"strings"
	"testing"
	"time"

	"axion/internal/diag"
	"axion/internal/sys"
)

func bsod(ago time.Duration, data map[string]string, msg string) sys.Event {
	return sys.Event{Channel: "System", Provider: "Microsoft-Windows-WER-SystemErrorReporting", ID: 1001, Level: 2, Time: time.Now().Add(-ago), Data: data, Message: msg}
}

func TestBugcheckCodeReading(t *testing.T) {
	cases := []struct {
		name string
		ev   sys.Event
		want int64
		ok   bool
	}{
		{"param1 com parâmetros", bsod(0, map[string]string{"param1": "0x00000116 (0xffffe0008b64c4c0, 0xfffff8003e9d4650, 0x0000000000000000, 0x000000000000000d)", "param2": `C:\WINDOWS\MEMORY.DMP`, "param3": "10281589-8be9-d71c-c713-e024f5515a45"}, ""), 0x116, true},
		{"campos sem nome (posição)", bsod(0, map[string]string{"0": "0x0000009f (0x3, 0xffff, 0x0, 0x0)", "1": `C:\WINDOWS\Minidump\x.dmp`}, ""), 0x9F, true},
		{"nome desconhecido", bsod(0, map[string]string{"BugcheckCode": "0x00000133 (0x0, 0x1, 0x2, 0x3)"}, ""), 0x133, true},
		{"só a mensagem renderizada", bsod(0, nil, `0x00000124 (0x0, 0xffffd, 0x0, 0x0) · C:\WINDOWS\MEMORY.DMP`), 0x124, true},
		{"param1 vence os outros campos", bsod(0, map[string]string{"param1": "0x0000001a (0x41792)", "0": "0x00000050 (0x1)"}, ""), 0x1A, true},
		{"código fora da tabela", bsod(0, map[string]string{"param1": "0xa0000001 (0x0, 0x0, 0x0, 0x0)"}, ""), 0xA0000001, true},
		{"maiúsculas", bsod(0, map[string]string{"param1": "0X0000003B (0x1)"}, ""), 0x3B, true},
		{"só o caminho do dump", bsod(0, map[string]string{"param2": `C:\WINDOWS\MEMORY.DMP`}, ""), 0, false},
		{"código zero", bsod(0, map[string]string{"param1": "0x00000000 (0x0)"}, ""), 0, false},
		{"texto sem código", bsod(0, map[string]string{"param1": "BugCheck"}, "BugCheck"), 0, false},
		{"código no meio do texto não conta", bsod(0, map[string]string{"param1": "erro 0x116 (x)"}, ""), 0, false},
		{"vazio", bsod(0, nil, ""), 0, false},
	}
	for _, tc := range cases {
		got, ok := diag.BugcheckCode(tc.ev)
		if got != tc.want || ok != tc.ok {
			t.Errorf("%s: %#x %v, esperava %#x %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func findingByID(fs []diag.Finding, id string) *diag.Finding {
	for i := range fs {
		if fs[i].ID == id {
			return &fs[i]
		}
	}
	return nil
}

func TestBlueScreenGuidance(t *testing.T) {
	code := func(c string) map[string]string { return map[string]string{"param1": c + " (0x1, 0x2, 0x3, 0x4)"} }
	evs := []sys.Event{
		bsod(30*24*time.Hour, code("0x00000124"), ""),
		bsod(10*24*time.Hour, code("0x00000124"), ""),
		bsod(1*24*time.Hour, code("0x00000133"), ""),
		bsod(3*24*time.Hour, code("0x00000116"), ""),
		bsod(40*24*time.Hour, code("0xa0000001"), ""),
	}
	x := findingByID(diag.Stability(evs), "stab.bsod")
	if x == nil || x.Severity != diag.SevCritical || x.Title != "5 telas azuis registradas" {
		t.Fatalf("achado: %+v", x)
	}
	// Most frequent first, then the most recent; the code outside the table is
	// listed too, without advice.
	order := []string{"0x124 WHEA_UNCORRECTABLE_ERROR (2 vezes)", "0x133 DPC_WATCHDOG_VIOLATION", "0x116 VIDEO_TDR_FAILURE", "0xA0000001"}
	pos := -1
	for _, want := range order {
		i := strings.Index(x.Evidence, want)
		if i < 0 || i < pos {
			t.Errorf("%q fora de ordem ou ausente em: %s", want, x.Evidence)
		}
		pos = i
	}
	for _, want := range []string{"o significado é da Microsoft; a orientação é do Axion", "Ocorreu um erro fatal de hardware", "tire overclock e undervolt do processador",
		"Uma única rotina de driver (DPC) demorou demais", "A tentativa de reiniciar o driver de vídeo", "se repetiu 2 vezes"} {
		if !strings.Contains(x.Explanation, want) {
			t.Errorf("a explicação deveria dizer %q:\n%s", want, x.Explanation)
		}
	}
	// One piece of advice per kind: the hardware one appears once.
	if n := strings.Count(x.Explanation, "tire overclock e undervolt do processador"); n != 1 {
		t.Errorf("conselho de hardware %d vezes", n)
	}
	if strings.Contains(x.Explanation, "A0000001") {
		t.Error("um código fora da tabela não ganha orientação inventada")
	}
	urls := map[string]bool{}
	for _, s := range x.Sources {
		urls[s.URL] = true
	}
	for _, u := range []string{
		"https://learn.microsoft.com/windows-hardware/drivers/debugger/bug-check-0x124---whea-uncorrectable-error",
		"https://learn.microsoft.com/windows-hardware/drivers/debugger/bug-check-0x133-dpc-watchdog-violation",
		"https://learn.microsoft.com/windows-hardware/drivers/debugger/bug-check-0x116---video-tdr-failure"} {
		if !urls[u] {
			t.Errorf("falta a fonte %s: %+v", u, x.Sources)
		}
	}
	pages := map[string]bool{}
	for _, a := range x.Actions {
		pages[a.Page] = true
	}
	if !pages["dpc"] || !pages["drivers"] || len(x.Actions) != 2 {
		t.Errorf("ações: %+v", x.Actions)
	}
}

// Only three codes get an explanation, however many appear.
func TestBlueScreenGuidanceIsCapped(t *testing.T) {
	var evs []sys.Event
	for i, c := range []string{"0x0000000a", "0x0000001a", "0x0000003b", "0x00000050", "0x000000d1"} {
		evs = append(evs, bsod(time.Duration(i+1)*24*time.Hour, map[string]string{"param1": c + " (0x1)"}, ""))
	}
	x := findingByID(diag.Stability(evs), "stab.bsod")
	if len(x.Sources) != 3 || strings.Contains(x.Explanation, "PAGE_FAULT") || strings.Contains(x.Explanation, "DRIVER_IRQL") {
		t.Errorf("só três códigos ganham explicação: %d fontes", len(x.Sources))
	}
	// 0xA and 0x3B are both driver faults: the driver advice is said once.
	if n := strings.Count(x.Explanation, "Atualize ou reinstale os drivers que mudaram"); n != 1 {
		t.Errorf("conselho de driver %d vezes", n)
	}
	// Every code appeared once: nothing "repeated".
	if strings.Contains(x.Explanation, "se repetiu") {
		t.Errorf("nenhum código se repetiu: %s", x.Explanation)
	}
	for _, c := range []string{"0xA IRQL_NOT_LESS_OR_EQUAL", "0x1A MEMORY_MANAGEMENT", "0x3B SYSTEM_SERVICE_EXCEPTION", "0x50 PAGE_FAULT_IN_NONPAGED_AREA", "0xD1 DRIVER_IRQL_NOT_LESS_OR_EQUAL"} {
		if !strings.Contains(x.Evidence, c) {
			t.Errorf("a evidência lista todos os códigos: falta %s", c)
		}
	}
}

// A blue screen whose code cannot be read keeps the generic finding exactly.
func TestBlueScreenWithoutReadableCodeIsUnchanged(t *testing.T) {
	last := time.Now().Add(-5 * 24 * time.Hour)
	evs := []sys.Event{bsod(5*24*time.Hour, nil, ""), bsod(9*24*time.Hour, map[string]string{"param2": `C:\WINDOWS\MEMORY.DMP`}, "")}
	x := findingByID(diag.Stability(evs), "stab.bsod")
	if x == nil {
		t.Fatal("sem achado")
	}
	if x.Evidence != "BugCheck; último em "+last.Format("02/01/2006 15:04")+"." || len(x.Sources) != 0 || len(x.Actions) != 0 ||
		x.Explanation != "Telas azuis indicam driver com defeito ou hardware instável. Reverta overclocks e atualize drivers de chipset e vídeo." {
		t.Errorf("o achado genérico mudou: %+v", x)
	}
}

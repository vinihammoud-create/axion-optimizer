package main

import (
	"fmt"
	"strings"
)

// hasCrashReport reports whether text holds a Go crash report (a panic, a
// fatal error or a Windows exception with its traceback). Other output on
// the standard error — WebView2's own log lines at shutdown, for example — is
// not a crash and must not be announced as one.
func hasCrashReport(text string) bool {
	for _, l := range strings.Split(text, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "Exception 0x") || strings.HasPrefix(t, "panic:") || strings.HasPrefix(t, "fatal error:") {
			return true
		}
	}
	return false
}

// crashSummary condenses a Go crash report (what a process wrote to its
// standard error before dying) into one line: the line that names the failure
// and the first frame of Axion's own code, with its file and line. A report
// without those parts yields its last non-empty line.
func crashSummary(report string) string {
	lines := strings.Split(report, "\n")
	var first, frame, last string
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		last = t
		if first == "" {
			if strings.HasPrefix(t, "Exception ") || strings.HasPrefix(t, "panic:") || strings.HasPrefix(t, "fatal error:") {
				first = t
			}
			continue
		}
		if frame != "" || strings.HasPrefix(t, "created by") || !(strings.HasPrefix(t, "axion/") || strings.HasPrefix(t, "main.")) {
			continue
		}
		frame = t
		if i+1 < len(lines) {
			if f := strings.Fields(strings.TrimSpace(lines[i+1])); len(f) > 0 && strings.Contains(f[0], ".go:") {
				frame += " (" + f[0] + ")"
			}
		}
	}
	if first == "" {
		return last
	}
	if strings.HasPrefix(first, "Exception ") {
		// "Exception <code> <read/write> <address> <pc>": say what it is.
		if f := strings.Fields(first); len(f) >= 4 {
			op := map[string]string{"0x0": "leitura", "0x1": "escrita", "0x8": "execução"}[f[2]]
			if op == "" {
				op = "acesso"
			}
			first = fmt.Sprintf("exceção %s do Windows (%s em %s) dentro de uma chamada ao sistema", f[1], op, f[3])
		}
	}
	if frame != "" {
		first += " — em " + frame
	}
	return first
}

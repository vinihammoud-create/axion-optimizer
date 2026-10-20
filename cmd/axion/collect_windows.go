//go:build windows

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"axion/internal/app"
	"axion/internal/collect"
	"axion/internal/facts"
	"axion/internal/sys/winsys"
)

// collectChildFlag starts Axion as the collection process: no window, no
// lock, no log file; it gathers the machine profile and writes it to stdout.
const collectChildFlag = "--axion-coleta"

// collectTimeout ends a collection that stopped answering (a probe that
// hangs); the part it was in is then set aside like one that crashed.
const collectTimeout = 4 * time.Minute

// collectMsg is one line of the collection process's output.
type collectMsg struct {
	T     string       `json:"t"` // step, part, facts
	Name  string       `json:"name,omitempty"`
	I     int          `json:"i,omitempty"`
	N     int          `json:"n,omitempty"`
	Done  bool         `json:"done,omitempty"`
	Facts *facts.Facts `json:"facts,omitempty"`
}

// runCollectChild is the collection process. args[0] is the JSON list of
// parts to skip. Every line is written at once, so the parent always knows
// which step or part was running if this process is ended.
func runCollectChild(args []string) int {
	var skip []string
	if len(args) > 0 {
		_ = json.Unmarshal([]byte(args[0]), &skip)
	}
	set := map[string]bool{}
	for _, s := range skip {
		set[s] = true
	}
	p, err := winsys.New(filepath.Join(programData(), "Axion"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "plataforma: %v\n", err)
		return 2
	}
	var mu sync.Mutex
	enc := json.NewEncoder(os.Stdout) // one JSON value per line
	send := func(m collectMsg) {
		mu.Lock()
		defer mu.Unlock()
		_ = enc.Encode(m)
	}
	f := collect.CollectWith(context.Background(), p, func(step string, i, n int) {
		send(collectMsg{T: "step", Name: step, I: i, N: n})
	}, collect.Options{Skip: set, Mark: func(part string, done bool) {
		send(collectMsg{T: "part", Name: part, Done: done})
	}})
	send(collectMsg{T: "facts", Facts: f})
	return 0
}

// childCollector runs each collection in a separate Axion process (exe). What
// that process writes to its standard error is kept (the beginning, where a
// crash report names the failure) and, when it dies, saved whole in
// coleta-erro.log inside dataDir.
func childCollector(exe, dataDir string) app.Collector {
	return func(parent context.Context, skip []string, progress collect.Progress, mark func(string, bool)) (*facts.Facts, error) {
		ctx, cancel := context.WithTimeout(parent, collectTimeout)
		defer cancel()
		if skip == nil {
			skip = []string{}
		}
		arg, _ := json.Marshal(skip)
		cmd := exec.CommandContext(ctx, exe, collectChildFlag, string(arg))
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
		errHead := &headBuffer{max: 256 << 10}
		cmd.Stderr = errHead
		out, err := cmd.StdoutPipe()
		if err != nil {
			return nil, err
		}
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("iniciar a coleta de informações: %w", err)
		}
		var result *facts.Facts
		step, current := "", ""
		rd := bufio.NewReaderSize(out, 1<<16)
		for {
			line, rerr := rd.ReadBytes('\n')
			var m collectMsg
			if len(line) > 0 && json.Unmarshal(line, &m) == nil {
				switch m.T {
				case "step":
					step, current = m.Name, m.Name
					if progress != nil {
						progress(m.Name, m.I, m.N)
					}
				case "part":
					if m.Done {
						current = step
					} else {
						current = m.Name
						if mark != nil {
							mark(m.Name, false)
						}
					}
				case "facts":
					result = m.Facts
				}
			}
			if rerr != nil {
				break
			}
		}
		werr := cmd.Wait()
		switch {
		case result != nil:
			return result, nil
		case parent.Err() != nil:
			return nil, parent.Err()
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			return nil, &app.CollectCrash{Part: current, Detail: fmt.Sprintf("não respondeu em %d minutos", int(collectTimeout.Minutes()))}
		}
		report := errHead.String()
		if strings.TrimSpace(report) != "" {
			path := filepath.Join(dataDir, "coleta-erro.log")
			if err := os.WriteFile(path, []byte(report), 0o644); err != nil {
				log.Printf("relatório da coleta: %v", err)
			} else {
				log.Printf("relatório completo da falha na coleta salvo em %s", path)
			}
		}
		return nil, &app.CollectCrash{Part: current, Detail: exitDetail(cmd, werr, report)}
	}
}

// exitDetail describes how the collection process ended: the exit code (an
// NTSTATUS such as 0xC0000005 when Windows ended it over a fault) and a
// one-line summary of what it wrote to stderr (the failure and the first
// frame of Axion's own code).
func exitDetail(cmd *exec.Cmd, werr error, stderr string) string {
	d := "sem código de saída"
	if cmd.ProcessState != nil {
		code := uint32(cmd.ProcessState.ExitCode())
		d = fmt.Sprintf("código de saída %d", code)
		if code >= 0x80000000 {
			d = fmt.Sprintf("código de saída 0x%08X", code)
		}
	} else if werr != nil {
		d = werr.Error()
	}
	if s := crashSummary(stderr); s != "" {
		d += "; " + s
	}
	return d
}

// headBuffer keeps the first max bytes written to it (a crash report names
// the failure at its beginning; the end is a register dump).
type headBuffer struct {
	mu  sync.Mutex
	max int
	b   []byte
}

func (t *headBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := len(p)
	if room := t.max - len(t.b); room > 0 {
		if len(p) > room {
			p = p[:room]
		}
		t.b = append(t.b, p...)
	}
	return n, nil
}

func (t *headBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.b)
}

//go:build windows

package winsys

import (
	"context"
	"os/exec"
	"sync"
	"testing"
	"time"

	"axion/internal/sys"
)

// The GPU sensors rely on one nvidia-smi process that prints a line a second
// (-l 1). With the output redirected to a pipe a program may buffer it until
// it exits, which would make the sampling useless: this checks, on a PC that
// has nvidia-smi, that the lines arrive while the process is still running.
func TestNvidiaSMILoopStreamsThroughAPipe(t *testing.T) {
	exe, err := exec.LookPath("nvidia-smi.exe")
	if err != nil {
		t.Skip("sem nvidia-smi neste PC")
	}
	var mu sync.Mutex
	var at []time.Duration
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 4500*time.Millisecond)
	defer cancel()
	_, _ = (&Runner{}).Run(ctx, exe, []string{"--query-gpu=temperature.gpu,clocks.sm", "--format=csv,noheader,nounits", "-l", "1"}, sys.RunOpts{Timeout: time.Minute, OnLine: func(string) {
		mu.Lock()
		at = append(at, time.Since(start))
		mu.Unlock()
	}})
	mu.Lock()
	defer mu.Unlock()
	t.Logf("linhas recebidas em: %v", at)
	if len(at) < 3 {
		t.Fatalf("só %d linhas em 4,5 s: o nvidia-smi não entrega uma por segundo", len(at))
	}
	// They must come spread over the run, not all at the end when it was killed.
	if at[0] > 2*time.Second || at[len(at)-1]-at[0] < 1500*time.Millisecond {
		t.Errorf("as linhas chegaram juntas (buffer): %v", at)
	}
}

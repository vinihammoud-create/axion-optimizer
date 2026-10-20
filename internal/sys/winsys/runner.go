//go:build windows

package winsys

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/sys/windows"

	"axion/internal/sys"
)

const createNoWindow = 0x08000000

// Runner implements sys.Runner: hidden processes, decoded output.
type Runner struct{}

var procGetOEMCP = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetOEMCP")

// decodeOutput converts console output to UTF-8. Windows tools write either
// UTF-16LE (sfc, some DISM builds), UTF-8 (winget, newer tools) or the OEM
// code page (bcdedit, netsh, chkdsk on localized Windows).
func decodeOutput(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		return utf16le(b[2:])
	}
	// Heuristic: many NULs in odd positions => UTF-16LE.
	if len(b) >= 4 {
		zeros := 0
		for i := 1; i < len(b) && i < 400; i += 2 {
			if b[i] == 0 {
				zeros++
			}
		}
		n := len(b)
		if n > 400 {
			n = 400
		}
		if zeros > n/4 {
			return utf16le(b)
		}
	}
	if utf8.Valid(b) {
		return string(b)
	}
	return oemToUTF8(b)
}

func utf16le(b []byte) string {
	if len(b)%2 == 1 {
		b = b[:len(b)-1]
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return strings.ReplaceAll(string(utf16.Decode(u)), "\x00", "")
}

func oemToUTF8(b []byte) string {
	cp, _, _ := procGetOEMCP.Call()
	n, err := windows.MultiByteToWideChar(uint32(cp), 0, &b[0], int32(len(b)), nil, 0)
	if err != nil || n == 0 {
		return string(b)
	}
	w := make([]uint16, n)
	if _, err := windows.MultiByteToWideChar(uint32(cp), 0, &b[0], int32(len(b)), &w[0], n); err != nil {
		return string(b)
	}
	return string(utf16.Decode(w))
}

type lineWriter struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	onLine func(string)
	pend   []byte
	mode   int // 0 unknown, 1 bytes (UTF-8/OEM), 2 UTF-16LE
}

func (w *lineWriter) emit(b []byte) {
	if line := strings.TrimSpace(decodeOutput(b)); line != "" {
		w.onLine(line)
	}
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	if w.onLine == nil {
		return len(p), nil
	}
	w.pend = append(w.pend, p...)
	if w.mode == 0 && len(w.pend) >= 4 {
		if (w.pend[0] == 0xFF && w.pend[1] == 0xFE) || (w.pend[1] == 0 && w.pend[3] == 0) {
			w.mode = 2
		} else {
			w.mode = 1
		}
	}
	switch w.mode {
	case 1:
		for {
			i := bytes.IndexAny(w.pend, "\r\n")
			if i < 0 {
				break
			}
			w.emit(w.pend[:i])
			w.pend = w.pend[i+1:]
		}
	case 2:
		for {
			cut := -1
			for i := 0; i+1 < len(w.pend); i += 2 {
				if (w.pend[i] == '\r' || w.pend[i] == '\n') && w.pend[i+1] == 0 {
					cut = i
					break
				}
			}
			if cut < 0 {
				break
			}
			seg := w.pend[:cut]
			if len(seg) >= 2 && seg[0] == 0xFF && seg[1] == 0xFE {
				seg = seg[2:]
			}
			if line := strings.TrimSpace(utf16le(seg)); line != "" {
				w.onLine(line)
			}
			w.pend = w.pend[cut+2:]
		}
	}
	return len(p), nil
}

// Run implements sys.Runner.
func (Runner) Run(ctx context.Context, exe string, args []string, opts sys.RunOpts) (sys.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	parent := ctx
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	path, err := systemExe(exe)
	if err != nil {
		return sys.Result{ExitCode: -1}, err
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	// After a cancel or a timeout only the program itself is stopped; a child
	// it started (an installer started by winget) may keep its output pipe
	// open. WaitDelay stops waiting for that pipe instead of hanging.
	cmd.WaitDelay = 10 * time.Second
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	if len(opts.Env) > 0 {
		cmd.Env = append(cmd.Environ(), opts.Env...)
	}
	if opts.Stdin != "" {
		cmd.Stdin = strings.NewReader(opts.Stdin)
	}
	out := &lineWriter{onLine: opts.OnLine}
	errw := &lineWriter{onLine: opts.OnLine}
	cmd.Stdout, cmd.Stderr = out, errw
	start := time.Now()
	err = cmd.Run()
	res := sys.Result{Stdout: decodeOutput(out.buf.Bytes()), Stderr: decodeOutput(errw.buf.Bytes()), Duration: time.Since(start)}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	// A process stopped because of the timeout or a cancel exits with a
	// code too; report why instead of a misleading exit code.
	if ctx.Err() != nil {
		if parent.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return res, fmt.Errorf("%s não terminou em %s: %w", filepath.Base(exe), opts.Timeout, sys.ErrTimeout)
		}
		return res, ctx.Err()
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return res, nil // non-zero exit is reported through ExitCode
	}
	return res, err
}

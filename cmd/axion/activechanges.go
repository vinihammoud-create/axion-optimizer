package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"

	"axion/internal/tweaks"
)

// activeChangesFlag makes Axion.exe only read the history of changes and
// answer with its exit code (see activeExitCode): no window, no lock, no log,
// nothing written. The uninstaller (installer/axion.iss) runs it before
// removing the program, to warn when changes made by Axion are still applied.
// The name is part of the contract with the installer: keep it.
const activeChangesFlag = "--axion-ajustes-ativos"

// Exit codes of activeChangesFlag.
const (
	exitNoActiveChanges = 0  // history read (or never written): nothing to undo
	exitHistoryUnread   = 1  // history exists but could not be read: unknown
	exitActiveBase      = 10 // 10+n: n entries still applied (n capped at maxActiveReported)
	maxActiveReported   = 200
)

// countActiveChanges reads the history in dir the way Axion does at start
// (journal.json, else the newest readable copy among journal.json.tmp and
// journal.json.bak) without changing anything, and counts the entries whose
// changes may still be on the machine: what the History page offers to undo.
// An apply that never finished ("applying", turned into "interrupted" at the
// next start) counts like an interrupted one. No history at all is not an
// error: Axion never changed anything here.
func countActiveChanges(dir string) (int, error) {
	// Only these three names, inside dir.
	fsys := os.DirFS(dir)
	var firstErr error
	found := false
	for _, p := range []string{"journal.json", "journal.json.tmp", "journal.json.bak"} {
		b, err := fs.ReadFile(fsys, p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		found = true
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		var f struct {
			Version int             `json:"version"`
			Entries []*tweaks.Entry `json:"entries"`
		}
		if err := json.Unmarshal(b, &f); err != nil || f.Version == 0 {
			if firstErr == nil {
				firstErr = errors.New(p + " ilegível")
			}
			continue
		}
		n := 0
		for _, e := range f.Entries {
			if e == nil {
				continue
			}
			if e.Status == tweaks.StatusApplying {
				e.Status = tweaks.StatusInterrupted
			}
			if e.Active() {
				n++
			}
		}
		return n, nil
	}
	if found {
		return 0, firstErr
	}
	return 0, nil
}

// activeExitCode turns the count into the exit code the uninstaller reads.
// A history that could not be read is never reported as "nothing applied".
func activeExitCode(n int, err error) int {
	switch {
	case err != nil:
		return exitHistoryUnread
	case n <= 0:
		return exitNoActiveChanges
	case n > maxActiveReported:
		n = maxActiveReported
	}
	return exitActiveBase + n
}

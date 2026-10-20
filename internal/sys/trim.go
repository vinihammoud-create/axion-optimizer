package sys

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// TRIM (delete notifications) and the Windows drive optimizer.
//
// Microsoft Learn, "fsutil behavior": "disabledeletenotify" 0 lets NTFS send
// TRIM/unmap to the storage device (the NTFS default), 1 stops it; the change
// takes effect with the next delete, no restart. "defrag <volume> /L" performs
// a retrim, and the scheduled task below runs retrim on SSDs once a month.

// DefragTask is the scheduled task behind "Optimize Drives".
const DefragTask = `\Microsoft\Windows\Defrag\ScheduledDefrag`

// KeyFileSystem holds DisableDeleteNotification, the value fsutil changes
// for NTFS (used only when fsutil cannot be read).
const KeyFileSystem = `HKLM\SYSTEM\CurrentControlSet\Control\FileSystem`

// Output of "fsutil behavior query disabledeletenotify", e.g.
//
//	NTFS DisableDeleteNotify = 0  (Allows TRIM operations to be sent to the storage device)
//	ReFS DisableDeleteNotify is not currently set
//
// Older builds print a single "DisableDeleteNotify = 0" line (NTFS). The
// text in parentheses is translated; the part matched here is not.
var reDeleteNotify = regexp.MustCompile(`(?im)^\s*(?:(NTFS|ReFS)\s+)?DisableDeleteNotify\s*=\s*(\d+)`)

// ParseDeleteNotify returns the NTFS and ReFS values (-1 = not set or not
// shown) and whether any value was found.
func ParseDeleteNotify(out string) (ntfs, refs int, ok bool) {
	ntfs, refs = -1, -1
	for _, m := range reDeleteNotify.FindAllStringSubmatch(out, -1) {
		v, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		if strings.EqualFold(m[1], "ReFS") {
			refs = v
		} else {
			ntfs = v
		}
		ok = true
	}
	return ntfs, refs, ok
}

// TrimState is the current delete-notification setting.
type TrimState struct {
	NTFS   int    `json:"ntfs"`   // 0 = TRIM on (default), 1 = off, -1 unknown
	ReFS   int    `json:"refs"`   // -1 = not set (file system default) or unknown
	Source string `json:"source"` // fsutil | registro
}

// QueryTrim reads the setting with fsutil and falls back to the registry
// value fsutil writes.
func QueryTrim(ctx context.Context, p *Platform) (TrimState, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := p.Run.Run(ctx, "fsutil.exe", []string{"behavior", "query", "disabledeletenotify"}, RunOpts{Timeout: 20 * time.Second})
	if err == nil && res.ExitCode == 0 {
		if n, r, ok := ParseDeleteNotify(res.Stdout + "\n" + res.Stderr); ok && n >= 0 {
			return TrimState{NTFS: n, ReFS: r, Source: "fsutil"}, nil
		}
	}
	v, gerr := p.Reg.GetValue(KeyFileSystem, "DisableDeleteNotification")
	if gerr != nil {
		if err == nil {
			err = fmt.Errorf("saída do fsutil não reconhecida")
		}
		return TrimState{NTFS: -1, ReFS: -1}, fmt.Errorf("fsutil: %v; registro: %w", err, gerr)
	}
	st := TrimState{NTFS: 0, ReFS: -1, Source: "registro"} // absent = NTFS default (on)
	if v.Exists {
		if v.Int != 0 {
			st.NTFS = 1
		}
	}
	return st, nil
}

// SetTrim turns NTFS delete notifications on (off=false) or off.
func SetTrim(ctx context.Context, p *Platform, off bool) error {
	v := "0"
	if off {
		v = "1"
	}
	res, err := p.Run.Run(ctx, "fsutil.exe", []string{"behavior", "set", "disabledeletenotify", v}, RunOpts{Timeout: 20 * time.Second})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("fsutil behavior set disabledeletenotify %s falhou: %s", v, strings.TrimSpace(res.Stdout+res.Stderr))
	}
	return nil
}

// Event 258 of the drive optimizer ("Defrag" in the Application log): "The
// storage optimizer successfully completed retrim on Windows (C:)". The first
// insertion string is the operation, the second the volume.
var reVolumeLetter = regexp.MustCompile(`\(([A-Za-z]):\s*\)`)

// OptimizerEvent decodes a drive optimizer completion event: the operation
// ("retrim", "defragmentation"...) and the drive letter ("C:"). ok means the
// event is the optimizer's; letter is "" for volumes without one. The data
// may come as unnamed values ("0", "1") or named ones, so every value is
// looked at: the one with "(X:)" is the volume, the first other is the
// operation.
func OptimizerEvent(ev Event) (op, letter string, ok bool) {
	if ev.ID != 258 || !strings.Contains(strings.ToLower(ev.Provider), "defrag") {
		return "", "", false
	}
	keys := make([]string, 0, len(ev.Data))
	for k := range ev.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := strings.TrimSpace(ev.Data[k])
		if v == "" {
			continue
		}
		if m := reVolumeLetter.FindStringSubmatch(v); m != nil {
			if letter == "" {
				letter = strings.ToUpper(m[1]) + ":"
			}
			continue
		}
		if op == "" {
			op = strings.ToLower(v)
		}
	}
	if letter == "" {
		if m := reVolumeLetter.FindStringSubmatch(ev.Message); m != nil {
			letter = strings.ToUpper(m[1]) + ":"
		}
	}
	return op, letter, true
}

// IsRetrim tells whether an optimizer operation is a retrim. The operation
// name in the event is translated (a Portuguese Windows writes "análise" for
// analysis, as seen in a real report), so the English word and the
// Portuguese/Spanish "corte"/"recorte" are accepted; an unknown translation
// is just not counted (the page then shows no date, never a false alarm).
func IsRetrim(op string) bool {
	op = strings.ToLower(op)
	return strings.Contains(op, "trim") || strings.Contains(op, "corte")
}

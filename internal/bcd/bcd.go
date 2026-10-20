// Package bcd reads and edits boot configuration elements of the current
// Windows boot entry through bcdedit.exe.
//
// bcdedit localizes some labels on non-English Windows (e.g. "identifier"
// becomes "identificador"), but the low-level elements Axion cares about
// (useplatformclock, disabledynamictick, useplatformtick, hypervisorlaunchtype,
// tscsyncpolicy) are matched by their canonical names, and the kernel's
// SystemStartOptions string is used as a second, language-independent signal.
package bcd

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"axion/internal/sys"
)

// The state view of the catalog reads the boot configuration once per tweak;
// bcdedit takes ~50 ms per run, so reads are cached briefly per platform and
// the cache is dropped on every write and at the start of each scan.
var cache = struct {
	sync.Mutex
	m map[*sys.Platform]cached
}{m: map[*sys.Platform]cached{}}

type cached struct {
	at   time.Time
	vals map[string]string
}

const cacheTTL = 5 * time.Second

// Invalidate drops the cached boot configuration of a platform.
func Invalidate(p *sys.Platform) {
	cache.Lock()
	delete(cache.m, p)
	cache.Unlock()
}

// Elements Axion inspects.
var Known = []string{"useplatformclock", "disabledynamictick", "useplatformtick", "hypervisorlaunchtype", "tscsyncpolicy", "testsigning", "nointegritychecks", "nx", "numproc", "truncatememory", "removememory"}

// Parse extracts "name value" pairs from `bcdedit /enum {current}` output.
// Keys are lower-cased. Only the first entry block is considered.
func Parse(out string) map[string]string {
	res := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(out))
	blocks := 0
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "---") {
			blocks++
			if blocks > 1 {
				break
			}
			continue
		}
		fields := strings.Fields(trim)
		if len(fields) < 2 {
			continue
		}
		key := strings.ToLower(fields[0])
		val := strings.TrimSpace(trim[len(fields[0]):])
		if _, ok := res[key]; !ok {
			res[key] = val
		}
	}
	return res
}

// Truthy interprets localized boolean output.
func Truthy(v string) (val bool, known bool) {
	// bcdedit prints booleans in the display language of Windows.
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "yes", "sim", "sí", "si", "sì", "oui", "ja", "tak", "да", "是", "はい", "evet", "ano", "igen", "true", "on", "1":
		return true, true
	case "no", "não", "nao", "non", "nein", "nee", "nie", "нет", "否", "いいえ", "hayır", "hayir", "ne", "nem", "false", "off", "0":
		return false, true
	}
	return false, false
}

// Read returns the known elements present on {current}. startOptions is the
// kernel's SystemStartOptions (may be empty).
func Read(ctx context.Context, p *sys.Platform) (map[string]string, error) {
	cache.Lock()
	if c, ok := cache.m[p]; ok && time.Since(c.at) < cacheTTL {
		cache.Unlock()
		return copyMap(c.vals), nil
	}
	cache.Unlock()
	vals, err := readUncached(ctx, p)
	if err != nil {
		return nil, err
	}
	cache.Lock()
	cache.m[p] = cached{at: time.Now(), vals: vals}
	cache.Unlock()
	return copyMap(vals), nil
}

// ReadFresh bypasses the cache (used to capture the state before a change).
func ReadFresh(ctx context.Context, p *sys.Platform) (map[string]string, error) {
	Invalidate(p)
	return Read(ctx, p)
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func readUncached(ctx context.Context, p *sys.Platform) (map[string]string, error) {
	res, err := p.Run.Run(ctx, "bcdedit.exe", []string{"/enum", "{current}"}, sys.RunOpts{Timeout: 20 * time.Second})
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("bcdedit retornou %d: %s", res.ExitCode, strings.TrimSpace(res.Stdout+res.Stderr))
	}
	all := Parse(res.Stdout)
	out := map[string]string{}
	for _, k := range Known {
		if v, ok := all[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}

// BootedWith reports whether the running kernel was started with an option
// (SystemStartOptions reflects the current boot; bcdedit changes only apply
// after a restart).
func BootedWith(p *sys.Platform, option string) bool {
	so, err := p.Reg.GetValue(`HKLM\SYSTEM\CurrentControlSet\Control`, "SystemStartOptions")
	if err != nil || !so.Exists {
		return false
	}
	return strings.Contains(" "+strings.ToUpper(so.Str)+" ", " "+strings.ToUpper(option)+" ")
}

// Set runs bcdedit /set {current} name value.
func Set(ctx context.Context, p *sys.Platform, name, value string) error {
	defer Invalidate(p)
	res, err := p.Run.Run(ctx, "bcdedit.exe", []string{"/set", "{current}", name, value}, sys.RunOpts{Timeout: 20 * time.Second})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("bcdedit /set %s falhou: %s", name, strings.TrimSpace(res.Stdout+res.Stderr))
	}
	return nil
}

// Delete runs bcdedit /deletevalue {current} name.
func Delete(ctx context.Context, p *sys.Platform, name string) error {
	defer Invalidate(p)
	res, err := p.Run.Run(ctx, "bcdedit.exe", []string{"/deletevalue", "{current}", name}, sys.RunOpts{Timeout: 20 * time.Second})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		out := strings.ToLower(res.Stdout + res.Stderr)
		// Deleting an element that is not present is fine.
		if strings.Contains(out, "not found") || strings.Contains(out, "não foi encontrad") || strings.Contains(out, "element not") {
			return nil
		}
		return fmt.Errorf("bcdedit /deletevalue %s falhou: %s", name, strings.TrimSpace(res.Stdout+res.Stderr))
	}
	return nil
}

// enumElements take named values, not booleans: "Off" there is a value of
// its own (hypervisorlaunchtype Off), not a translation of "No".
var enumElements = map[string]bool{
	"hypervisorlaunchtype": true, "nx": true, "bootmenupolicy": true, "bootstatuspolicy": true,
	"tscsyncpolicy": true, "pae": true, "x2apicpolicy": true,
	"numproc": true, "truncatememory": true, "removememory": true,
}

// NormalizeFor converts the displayed value of element name into a value
// accepted by /set: localized booleans become true/false, the values of
// enumerated and numeric elements are kept as shown.
func NormalizeFor(name, v string) string {
	if enumElements[strings.ToLower(name)] {
		return strings.TrimSpace(v)
	}
	return Normalize(v)
}

// Normalize converts a displayed boolean-or-other value into a value
// accepted by /set (use NormalizeFor when the element is known).
func Normalize(v string) string {
	if b, ok := Truthy(v); ok {
		if b {
			return "true"
		}
		return "false"
	}
	return strings.TrimSpace(v)
}

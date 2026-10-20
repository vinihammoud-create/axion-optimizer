package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"axion/internal/bcd"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// ---------------------------------------------------------------------------
// BCD element
// ---------------------------------------------------------------------------

type bcdSnap struct {
	Present bool   `json:"present"`
	Value   string `json:"value"`
}

// bcdOp sets (want != "") or deletes (want == "") an element of {current}.
func bcdOp(name, want string) tweaks.Op {
	readWith := func(c *tweaks.Ctx, fresh bool) (bcdSnap, error) {
		rd := bcd.Read
		if fresh {
			rd = bcd.ReadFresh
		}
		m, err := rd(c.Context, c.P)
		if err != nil {
			return bcdSnap{}, err
		}
		v, ok := m[name]
		return bcdSnap{Present: ok, Value: v}, nil
	}
	read := func(c *tweaks.Ctx) (bcdSnap, error) { return readWith(c, false) }
	desc := fmt.Sprintf("bcdedit /deletevalue {current} %s", name)
	if want != "" {
		desc = fmt.Sprintf("bcdedit /set {current} %s %s", name, want)
	}
	return &tweaks.FuncOp{
		KindName: "bcd", ID: name, Desc: desc, Spec: bcdSpec{Name: name, Want: want},
		SnapFn: func(c *tweaks.Ctx) (any, error) { return readWith(c, true) },
		InFn: func(c *tweaks.Ctx) (bool, error) {
			s, err := read(c)
			if err != nil {
				return false, err
			}
			if want == "" {
				return !s.Present, nil
			}
			return s.Present && strings.EqualFold(bcd.NormalizeFor(name, s.Value), bcd.NormalizeFor(name, want)), nil
		},
		ApplyFn: func(c *tweaks.Ctx) error {
			if want == "" {
				return bcd.Delete(c.Context, c.P, name)
			}
			return bcd.Set(c.Context, c.P, name, want)
		},
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var s bcdSnap
			if err := json.Unmarshal(raw, &s); err != nil {
				return err
			}
			if !s.Present {
				return bcd.Delete(c.Context, c.P, name)
			}
			return bcd.Set(c.Context, c.P, name, bcd.NormalizeFor(name, s.Value))
		},
		CurFn: func(c *tweaks.Ctx) string {
			s, err := read(c)
			if err != nil {
				return "erro: " + err.Error()
			}
			if !s.Present {
				return "(ausente — padrão do Windows)"
			}
			return s.Value
		},
	}
}

// ---------------------------------------------------------------------------
// Mouse acceleration (registry + live SPI_SETMOUSE)
// ---------------------------------------------------------------------------

const keyMouse = `HKCU\Control Panel\Mouse`

type mouseSnap struct {
	Speed sys.RegValue `json:"speed"`
	T1    sys.RegValue `json:"t1"`
	T2    sys.RegValue `json:"t2"`
}

func atoiDef(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}

func mouseOp() tweaks.Op {
	read := func(c *tweaks.Ctx) (mouseSnap, error) {
		var s mouseSnap
		var err error
		if s.Speed, err = c.P.Reg.GetValue(keyMouse, "MouseSpeed"); err != nil {
			return s, err
		}
		if s.T1, err = c.P.Reg.GetValue(keyMouse, "MouseThreshold1"); err != nil {
			return s, err
		}
		s.T2, err = c.P.Reg.GetValue(keyMouse, "MouseThreshold2")
		return s, err
	}
	write := func(c *tweaks.Ctx, s mouseSnap) error {
		for _, kv := range []struct {
			n string
			v sys.RegValue
		}{{"MouseSpeed", s.Speed}, {"MouseThreshold1", s.T1}, {"MouseThreshold2", s.T2}} {
			var err error
			if kv.v.Exists {
				err = c.P.Reg.SetValue(keyMouse, kv.n, kv.v)
			} else {
				err = c.P.Reg.DeleteValue(keyMouse, kv.n)
			}
			if err != nil {
				return err
			}
		}
		// Windows defaults when values are absent: speed 1, thresholds 6/10.
		return c.P.Native.SetMouseParams(atoiDef(s.T1.Str, 6), atoiDef(s.T2.Str, 10), atoiDef(s.Speed.Str, 1))
	}
	return &tweaks.FuncOp{
		KindName: "mouse", ID: "acceleration", Spec: struct{}{},
		Desc:   `HKCU\Control Panel\Mouse → MouseSpeed="0", MouseThreshold1="0", MouseThreshold2="0" e SPI_SETMOUSE (aplica na hora, sem sair da conta)`,
		SnapFn: func(c *tweaks.Ctx) (any, error) { return read(c) },
		InFn: func(c *tweaks.Ctx) (bool, error) {
			s, err := read(c)
			if err != nil {
				return false, err
			}
			return s.Speed.Exists && strings.TrimSpace(s.Speed.Str) == "0", nil
		},
		ApplyFn: func(c *tweaks.Ctx) error {
			return write(c, mouseSnap{Speed: sys.SZ("0"), T1: sys.SZ("0"), T2: sys.SZ("0")})
		},
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var s mouseSnap
			if err := json.Unmarshal(raw, &s); err != nil {
				return err
			}
			return write(c, s)
		},
		CurFn: func(c *tweaks.Ctx) string {
			s, err := read(c)
			if err != nil {
				return "erro: " + err.Error()
			}
			state := "ligada"
			if strings.TrimSpace(s.Speed.Str) == "0" {
				state = "desligada"
			}
			return fmt.Sprintf("aceleração %s (MouseSpeed=%s, limiares %s/%s)", state, s.Speed.Str, s.T1.Str, s.T2.Str)
		},
	}
}

// ---------------------------------------------------------------------------
// Numeric REG_SZ flag clear (accessibility hotkeys)
// ---------------------------------------------------------------------------

// flagClearOp clears mask bits of a numeric string flag value, preserving
// every other bit the user configured.
func flagClearOp(path, name string, mask int, def int, label string) tweaks.Op {
	path = sys.NormalizeRegPath(path)
	read := func(c *tweaks.Ctx) (sys.RegValue, error) { return c.P.Reg.GetValue(path, name) }
	cur := func(v sys.RegValue) int {
		if !v.Exists {
			return def
		}
		return atoiDef(v.Str, def)
	}
	return &tweaks.FuncOp{
		KindName: "registry-flag", ID: strings.ToLower(path + "|" + name), Spec: flagSpec{Path: path, Name: name, Mask: mask, Def: def, Label: label},
		Desc:   fmt.Sprintf(`%s → "%s": desliga o bit 0x%X (%s), preservando os demais`, path, name, mask, label),
		SnapFn: func(c *tweaks.Ctx) (any, error) { return read(c) },
		InFn: func(c *tweaks.Ctx) (bool, error) {
			v, err := read(c)
			if err != nil {
				return false, err
			}
			return cur(v)&mask == 0, nil
		},
		ApplyFn: func(c *tweaks.Ctx) error {
			v, err := read(c)
			if err != nil {
				return err
			}
			return c.P.Reg.SetValue(path, name, sys.SZ(strconv.Itoa(cur(v)&^mask)))
		},
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var v sys.RegValue
			if err := json.Unmarshal(raw, &v); err != nil {
				return err
			}
			if !v.Exists {
				return c.P.Reg.DeleteValue(path, name)
			}
			return c.P.Reg.SetValue(path, name, v)
		},
		CurFn: func(c *tweaks.Ctx) string {
			v, err := read(c)
			if err != nil {
				return "erro: " + err.Error()
			}
			on := "ativo"
			if cur(v)&mask == 0 {
				on = "desligado"
			}
			return fmt.Sprintf("atalho %s (Flags=%d)", on, cur(v))
		},
	}
}

// ---------------------------------------------------------------------------
// Hibernation (powercfg /hibernate) + Fast Startup
// ---------------------------------------------------------------------------

const (
	keyPower        = `HKLM\SYSTEM\CurrentControlSet\Control\Power`
	keySessionPower = `HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Power`
)

type hiberSnap struct {
	Enabled   bool         `json:"enabled"`
	Hiberboot sys.RegValue `json:"hiberboot"`
}

func hibernateEnabled(c *tweaks.Ctx) bool {
	v, err := c.P.Reg.GetValue(keyPower, "HibernateEnabled")
	if err == nil && v.Exists {
		return v.Int != 0
	}
	drive := c.P.Native.Paths().SystemDrive
	if drive == "" {
		drive = "C:"
	}
	_, err = os.Stat(filepath.Join(drive+`\`, "hiberfil.sys"))
	return err == nil
}

func powercfgHibernate(c *tweaks.Ctx, on bool) error {
	arg := "off"
	if on {
		arg = "on"
	}
	res, err := c.P.Run.Run(c.Context, "powercfg.exe", []string{"/hibernate", arg}, sys.RunOpts{Timeout: 60 * time.Second})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("powercfg /hibernate %s falhou: %s", arg, strings.TrimSpace(res.Stdout+res.Stderr))
	}
	return nil
}

func hibernateOp() tweaks.Op {
	return &tweaks.FuncOp{
		KindName: "hibernate", ID: "hibernate", Spec: struct{}{},
		Desc: "powercfg /hibernate off (remove o hiberfil.sys e, com ele, a Inicialização Rápida)",
		SnapFn: func(c *tweaks.Ctx) (any, error) {
			hb, err := c.P.Reg.GetValue(keySessionPower, "HiberbootEnabled")
			if err != nil {
				return nil, err
			}
			return hiberSnap{Enabled: hibernateEnabled(c), Hiberboot: hb}, nil
		},
		InFn:    func(c *tweaks.Ctx) (bool, error) { return !hibernateEnabled(c), nil },
		ApplyFn: func(c *tweaks.Ctx) error { return powercfgHibernate(c, false) },
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var s hiberSnap
			if err := json.Unmarshal(raw, &s); err != nil {
				return err
			}
			if s.Enabled {
				if err := powercfgHibernate(c, true); err != nil {
					return err
				}
			}
			if s.Hiberboot.Exists {
				return c.P.Reg.SetValue(keySessionPower, "HiberbootEnabled", s.Hiberboot)
			}
			return nil
		},
		CurFn: func(c *tweaks.Ctx) string {
			if hibernateEnabled(c) {
				return "hibernação ligada"
			}
			return "hibernação desligada"
		},
	}
}

// ---------------------------------------------------------------------------
// TCP global defaults (read via WMI, written with netsh)
// ---------------------------------------------------------------------------

type tcpGlobals struct {
	AutoTuning string `json:"autotuning"` // disabled, highlyrestricted, restricted, normal, experimental
	RSS        string `json:"rss"`        // enabled, disabled
}

var autoTuningNames = map[int64]string{0: "disabled", 1: "highlyrestricted", 2: "restricted", 3: "normal", 4: "experimental"}

func readTCP(c *tweaks.Ctx) (tcpGlobals, error) {
	var g tcpGlobals
	rows, err := c.P.WMI.Query(`root\StandardCimv2`, "SELECT SettingName, AutoTuningLevelLocal FROM MSFT_NetTCPSetting WHERE SettingName='Internet'")
	if err != nil {
		return g, err
	}
	if len(rows) > 0 {
		if s, ok := rows[0]["AutoTuningLevelLocal"].(string); ok && s != "" {
			g.AutoTuning = strings.ToLower(s)
		} else if n, ok := autoTuningNames[rows[0].Int("AutoTuningLevelLocal")]; ok {
			g.AutoTuning = n
		}
	}
	rows, err = c.P.WMI.Query(`root\StandardCimv2`, "SELECT ReceiveSideScaling FROM MSFT_NetOffloadGlobalSetting")
	if err != nil {
		return g, err
	}
	if len(rows) > 0 {
		if rows[0].Int("ReceiveSideScaling") == 1 || strings.EqualFold(rows[0].Str("ReceiveSideScaling"), "Enabled") {
			g.RSS = "enabled"
		} else {
			g.RSS = "disabled"
		}
	}
	return g, nil
}

func netshTCP(c *tweaks.Ctx, key, value string) error {
	res, err := c.P.Run.Run(c.Context, "netsh.exe", []string{"int", "tcp", "set", "global", key + "=" + value}, sys.RunOpts{Timeout: 30 * time.Second})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("netsh int tcp set global %s=%s falhou: %s", key, value, strings.TrimSpace(res.Stdout+res.Stderr))
	}
	return nil
}

func tcpDefaultsOp() tweaks.Op {
	return &tweaks.FuncOp{
		KindName: "tcp-global", ID: "autotuning-rss", Spec: struct{}{},
		Desc: "netsh int tcp set global autotuninglevel=normal e rss=enabled (padrões do Windows)",
		// Both values must be read before anything is written: a value that
		// could not be read would be written by the apply and skipped by the
		// undo, leaving the entry half undone for good.
		SnapFn: func(c *tweaks.Ctx) (any, error) {
			g, err := readTCP(c)
			if err == nil && (g.AutoTuning == "" || g.RSS == "") {
				err = fmt.Errorf("o Windows não informou o ajuste automático da janela TCP e o RSS (autotuning=%q, RSS=%q)", g.AutoTuning, g.RSS)
			}
			return g, err
		},
		InFn: func(c *tweaks.Ctx) (bool, error) {
			g, err := readTCP(c)
			if err != nil {
				return false, err
			}
			return g.AutoTuning == "normal" && g.RSS == "enabled", nil
		},
		ApplyFn: func(c *tweaks.Ctx) error {
			if err := netshTCP(c, "autotuninglevel", "normal"); err != nil {
				return err
			}
			return netshTCP(c, "rss", "enabled")
		},
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var g tcpGlobals
			if err := json.Unmarshal(raw, &g); err != nil {
				return err
			}
			if g.AutoTuning != "" {
				if err := netshTCP(c, "autotuninglevel", g.AutoTuning); err != nil {
					return err
				}
			}
			if g.RSS != "" {
				return netshTCP(c, "rss", g.RSS)
			}
			return nil
		},
		CurFn: func(c *tweaks.Ctx) string {
			g, err := readTCP(c)
			if err != nil {
				return "erro: " + err.Error()
			}
			return fmt.Sprintf("autotuning=%s, RSS=%s", g.AutoTuning, g.RSS)
		},
	}
}

// ---------------------------------------------------------------------------
// Scheduled tasks (COM Task Scheduler)
// ---------------------------------------------------------------------------

type taskSnap struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Enabled bool   `json:"enabled"`
}

// tasksOp disables a list of scheduled tasks (missing tasks are ignored).
func tasksOp(id string, paths []string) tweaks.Op {
	read := func(c *tweaks.Ctx, list []string) ([]taskSnap, error) {
		out := make([]taskSnap, 0, len(list))
		for _, p := range list {
			t, err := c.P.Tasks.Get(p)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			out = append(out, taskSnap{Path: p, Exists: t.Exists, Enabled: t.Enabled})
		}
		return out, nil
	}
	return &tweaks.FuncOp{
		KindName: "tasks", ID: id, Spec: tasksSpec{ID: id, Paths: paths},
		Desc:   "Desativar tarefas agendadas: " + strings.Join(paths, ", "),
		SnapFn: func(c *tweaks.Ctx) (any, error) { return read(c, paths) },
		InFn: func(c *tweaks.Ctx) (bool, error) {
			list, err := read(c, paths)
			if err != nil {
				return false, err
			}
			for _, t := range list {
				if t.Exists && t.Enabled {
					return false, nil
				}
			}
			return true, nil
		},
		ApplyFn: func(c *tweaks.Ctx) error {
			list, err := read(c, paths)
			if err != nil {
				return err
			}
			for _, t := range list {
				if t.Exists && t.Enabled {
					if err := c.P.Tasks.SetEnabled(t.Path, false); err != nil {
						return fmt.Errorf("%s: %w", t.Path, err)
					}
				}
			}
			return nil
		},
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var before []taskSnap
			if err := json.Unmarshal(raw, &before); err != nil {
				return err
			}
			// Only tasks this op manages are touched, whatever the snapshot
			// lists (a history record edited outside Axion cannot enable an
			// arbitrary task). A task that Windows removed since is skipped,
			// and one failure does not stop the others.
			own := map[string]bool{}
			for _, p := range paths {
				own[strings.ToLower(p)] = true
			}
			var errs []string
			for _, t := range before {
				if !t.Exists || !t.Enabled || !own[strings.ToLower(t.Path)] {
					continue
				}
				cur, err := c.P.Tasks.Get(t.Path)
				if err == nil && !cur.Exists {
					continue
				}
				if err == nil && cur.Enabled {
					continue
				}
				if err := c.P.Tasks.SetEnabled(t.Path, true); err != nil {
					errs = append(errs, t.Path+": "+err.Error())
				}
			}
			if len(errs) > 0 {
				return errors.New(strings.Join(errs, "; "))
			}
			return nil
		},
		EqFn: func(a, b json.RawMessage) bool {
			// Tasks missing on either side are not compared: a task removed
			// by a Windows update is nothing to restore.
			var la, lb []taskSnap
			if json.Unmarshal(a, &la) != nil || json.Unmarshal(b, &lb) != nil {
				return false
			}
			m := map[string]taskSnap{}
			for _, t := range lb {
				m[strings.ToLower(t.Path)] = t
			}
			for _, t := range la {
				o, ok := m[strings.ToLower(t.Path)]
				if !ok || !t.Exists || !o.Exists {
					continue
				}
				if t.Enabled != o.Enabled {
					return false
				}
			}
			return true
		},
		CurFn: func(c *tweaks.Ctx) string {
			list, err := read(c, paths)
			if err != nil {
				return "erro: " + err.Error()
			}
			on, off, missing := 0, 0, 0
			for _, t := range list {
				switch {
				case !t.Exists:
					missing++
				case t.Enabled:
					on++
				default:
					off++
				}
			}
			return fmt.Sprintf("%d ativas, %d desativadas, %d inexistentes neste Windows", on, off, missing)
		},
	}
}

// ---------------------------------------------------------------------------
// Windows optional feature (read via WMI, changed with DISM)
// ---------------------------------------------------------------------------

type featSnap struct {
	State int64 `json:"state"` // Win32_OptionalFeature.InstallState: 1 enabled, 2 disabled, 3 absent
}

// Win32_OptionalFeature enumerates every feature on each query (often 1-3 s),
// so reads are cached briefly per platform; DISM changes drop the cache.
var featCache = struct {
	sync.Mutex
	m map[string]featCached
}{m: map[string]featCached{}}

type featCached struct {
	at    time.Time
	state int64
}

func featKey(c *tweaks.Ctx, name string) string {
	return fmt.Sprintf("%p|%s", c.P, strings.ToLower(name))
}

func featureState(c *tweaks.Ctx, name string) (int64, error) {
	k := featKey(c, name)
	featCache.Lock()
	if v, ok := featCache.m[k]; ok && time.Since(v.at) < 10*time.Second {
		featCache.Unlock()
		return v.state, nil
	}
	featCache.Unlock()
	rows, err := c.P.WMI.Query(`root\cimv2`, fmt.Sprintf("SELECT Name, InstallState FROM Win32_OptionalFeature WHERE Name='%s'", name))
	if err != nil {
		return 0, err
	}
	st := int64(3)
	if len(rows) > 0 {
		st = rows[0].Int("InstallState")
	}
	featCache.Lock()
	featCache.m[k] = featCached{at: time.Now(), state: st}
	featCache.Unlock()
	return st, nil
}

func dropFeatureCache(c *tweaks.Ctx, name string) {
	featCache.Lock()
	delete(featCache.m, featKey(c, name))
	featCache.Unlock()
}

func dismFeature(c *tweaks.Ctx, name string, enable bool) error {
	defer dropFeatureCache(c, name)
	verb := "/disable-feature"
	if enable {
		verb = "/enable-feature"
	}
	res, err := c.P.Run.Run(c.Context, "dism.exe", []string{"/online", verb, "/featurename:" + name, "/norestart", "/quiet"}, sys.RunOpts{Timeout: 20 * time.Minute})
	if err != nil {
		return err
	}
	// 3010 = success, restart required.
	if res.ExitCode != 0 && res.ExitCode != 3010 {
		return fmt.Errorf("DISM %s %s retornou %d", verb, name, res.ExitCode)
	}
	return nil
}

func featureOp(name, label string) tweaks.Op {
	return &tweaks.FuncOp{
		KindName: "feature", ID: name, Spec: featureSpec{Name: name, Label: label},
		Desc: fmt.Sprintf("DISM /online /disable-feature /featurename:%s (%s)", name, label),
		SnapFn: func(c *tweaks.Ctx) (any, error) {
			s, err := featureState(c, name)
			return featSnap{State: s}, err
		},
		InFn: func(c *tweaks.Ctx) (bool, error) {
			s, err := featureState(c, name)
			return s != 1, err
		},
		ApplyFn: func(c *tweaks.Ctx) error { return dismFeature(c, name, false) },
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var s featSnap
			if err := json.Unmarshal(raw, &s); err != nil {
				return err
			}
			if s.State == 1 {
				return dismFeature(c, name, true)
			}
			return nil
		},
		CurFn: func(c *tweaks.Ctx) string {
			s, err := featureState(c, name)
			if err != nil {
				return "erro: " + err.Error()
			}
			switch s {
			case 1:
				return "ativado"
			case 2:
				return "desativado"
			case 3:
				return "(não existe nesta edição)"
			}
			return "desconhecido"
		},
	}
}

// ---------------------------------------------------------------------------
// Display refresh rate
// ---------------------------------------------------------------------------

type displaySnap struct {
	Device string `json:"device"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Hz     int    `json:"hz"`
}

func findDisplay(c *tweaks.Ctx, device string) (sys.DisplayMode, bool) {
	list, err := c.P.Native.DisplayModes()
	if err != nil {
		return sys.DisplayMode{}, false
	}
	for _, d := range list {
		if strings.EqualFold(d.Device, device) {
			return d, true
		}
	}
	return sys.DisplayMode{}, false
}

// refreshOp sets a display to the highest refresh rate available at its
// current resolution.
func refreshOp(device, monitor string) tweaks.Op {
	return &tweaks.FuncOp{
		KindName: "display", ID: strings.ToLower(device), Spec: displaySpec{Device: device, Monitor: monitor},
		Desc: fmt.Sprintf("%s (%s) → maior taxa de atualização disponível na resolução atual (ChangeDisplaySettingsEx)", monitor, device),
		SnapFn: func(c *tweaks.Ctx) (any, error) {
			d, ok := findDisplay(c, device)
			if !ok {
				return nil, fmt.Errorf("monitor %s não encontrado", device)
			}
			return displaySnap{Device: d.Device, Width: d.Width, Height: d.Height, Hz: d.CurrentHz}, nil
		},
		InFn: func(c *tweaks.Ctx) (bool, error) {
			d, ok := findDisplay(c, device)
			if !ok {
				return true, nil
			}
			return d.MaxHzAtRes == 0 || d.CurrentHz >= d.MaxHzAtRes, nil
		},
		ApplyFn: func(c *tweaks.Ctx) error {
			d, ok := findDisplay(c, device)
			if !ok {
				return fmt.Errorf("monitor %s não encontrado", device)
			}
			return c.P.Native.SetDisplayMode(d.Device, d.Width, d.Height, d.MaxHzAtRes)
		},
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var s displaySnap
			if err := json.Unmarshal(raw, &s); err != nil {
				return err
			}
			return c.P.Native.SetDisplayMode(s.Device, s.Width, s.Height, s.Hz)
		},
		CurFn: func(c *tweaks.Ctx) string {
			d, ok := findDisplay(c, device)
			if !ok {
				return "(monitor não conectado)"
			}
			return fmt.Sprintf("%dx%d a %d Hz (máximo nesta resolução: %d Hz)", d.Width, d.Height, d.CurrentHz, d.MaxHzAtRes)
		},
	}
}

// ---------------------------------------------------------------------------
// TRIM (fsutil behavior disabledeletenotify)
// ---------------------------------------------------------------------------

type trimSnap struct {
	NTFS int `json:"ntfs"` // 0 on, 1 off
}

// trimOnOp turns NTFS delete notifications (TRIM) back on; undo restores the
// previous value.
func trimOnOp() tweaks.Op {
	read := func(c *tweaks.Ctx) (int, error) {
		st, err := sys.QueryTrim(c.Context, c.P)
		if err != nil {
			return -1, err
		}
		return st.NTFS, nil
	}
	return &tweaks.FuncOp{
		KindName: "trim", ID: "ntfs", Spec: struct{}{},
		Desc: "fsutil behavior set disabledeletenotify 0 (TRIM ligado, padrão do NTFS; vale sem reiniciar)",
		SnapFn: func(c *tweaks.Ctx) (any, error) {
			n, err := read(c)
			return trimSnap{NTFS: n}, err
		},
		InFn: func(c *tweaks.Ctx) (bool, error) {
			n, err := read(c)
			return n == 0, err
		},
		ApplyFn: func(c *tweaks.Ctx) error { return sys.SetTrim(c.Context, c.P, false) },
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var s trimSnap
			if err := json.Unmarshal(raw, &s); err != nil {
				return err
			}
			return sys.SetTrim(c.Context, c.P, s.NTFS == 1)
		},
		CurFn: func(c *tweaks.Ctx) string {
			n, err := read(c)
			switch {
			case err != nil:
				return "erro: " + err.Error()
			case n == 0:
				return "TRIM ligado (DisableDeleteNotify = 0)"
			}
			return fmt.Sprintf("TRIM desligado (DisableDeleteNotify = %d)", n)
		},
	}
}

// ---------------------------------------------------------------------------
// Re-enable a scheduled task
// ---------------------------------------------------------------------------

// taskEnableOp enables a scheduled task disabled by another tool; undo
// disables it again only if it was disabled before.
func taskEnableOp(path string) tweaks.Op {
	read := func(c *tweaks.Ctx) (taskSnap, error) {
		t, err := c.P.Tasks.Get(path)
		if err != nil {
			return taskSnap{}, err
		}
		return taskSnap{Path: path, Exists: t.Exists, Enabled: t.Enabled}, nil
	}
	return &tweaks.FuncOp{
		KindName: "task-enable", ID: strings.ToLower(path), Spec: taskEnableSpec{Path: path},
		Desc:   "Reativar a tarefa agendada " + path,
		SnapFn: func(c *tweaks.Ctx) (any, error) { return read(c) },
		InFn: func(c *tweaks.Ctx) (bool, error) {
			t, err := read(c)
			return !t.Exists || t.Enabled, err
		},
		ApplyFn: func(c *tweaks.Ctx) error {
			t, err := read(c)
			if err != nil {
				return err
			}
			if !t.Exists {
				return fmt.Errorf("a tarefa %s não existe neste Windows", path)
			}
			return c.P.Tasks.SetEnabled(path, true)
		},
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var before taskSnap
			if err := json.Unmarshal(raw, &before); err != nil {
				return err
			}
			if before.Exists && !before.Enabled {
				return c.P.Tasks.SetEnabled(path, false)
			}
			return nil
		},
		CurFn: func(c *tweaks.Ctx) string {
			t, err := read(c)
			switch {
			case err != nil:
				return "erro: " + err.Error()
			case !t.Exists:
				return "(a tarefa não existe neste Windows)"
			case t.Enabled:
				return "ativa"
			}
			return "desativada"
		},
	}
}

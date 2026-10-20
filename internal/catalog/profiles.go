package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// GamePath resolves the full path of a game executable (set by the games
// library at startup). It returns "" when unknown.
var GamePath = func(exe string) string { return "" }

// ProfileFeatures lists the per-game options in display order.
var ProfileFeatures = []string{"priority", "gpu", "fso", "nvpstate"}

const keyIFEO = `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options`

var srcIFEO = tweaks.Source{Title: "Microsoft Learn — Image File Execution Options (chave IFEO)", URL: "https://learn.microsoft.com/windows-hardware/drivers/debugger/gflags-overview"}
var srcGpuPref = tweaks.Source{Title: "Microsoft Support — Otimizações para jogos em janela: opções personalizadas por aplicativo, incluindo a GPU (Configurações > Tela > Elementos gráficos)", URL: "https://support.microsoft.com/windows/optimizations-for-windowed-games-in-windows-11-3f006843-2c7e-4ed0-9a5e-f9389e535952"}
var srcFSO = tweaks.Source{Title: "DirectX Developer Blog — Demystifying Fullscreen Optimizations", URL: "https://devblogs.microsoft.com/directx/demystifying-full-screen-optimizations/"}

// ProfileID builds a profile tweak ID.
func ProfileID(exe, feature string) string {
	return "profile:" + strings.ToLower(exe) + ":" + feature
}

// ParseProfileID splits a profile tweak ID.
func ParseProfileID(id string) (exe, feature string, ok bool) {
	if !strings.HasPrefix(id, "profile:") {
		return "", "", false
	}
	rest := strings.TrimPrefix(id, "profile:")
	i := strings.LastIndex(rest, ":")
	if i <= 0 {
		return "", "", false
	}
	return rest[:i], rest[i+1:], true
}

func multiGPU(f *facts.Facts) bool {
	n := 0
	for _, g := range f.GPUs {
		if g.Vendor == "NVIDIA" || g.Vendor == "AMD" || g.Vendor == "Intel" {
			n++
		}
	}
	return n > 1
}

// systemExes are Windows processes whose priority or compatibility flags a
// game profile must never change (raising csrss.exe or dwm.exe to High can
// starve the rest of the system; see "O que não fazemos").
var systemExes = map[string]bool{
	"system": true, "registry": true, "smss.exe": true, "csrss.exe": true, "wininit.exe": true, "winlogon.exe": true,
	"services.exe": true, "lsass.exe": true, "lsaiso.exe": true, "svchost.exe": true, "dwm.exe": true, "explorer.exe": true,
	"audiodg.exe": true, "fontdrvhost.exe": true, "sihost.exe": true, "taskhostw.exe": true, "ctfmon.exe": true,
	"conhost.exe": true, "spoolsv.exe": true, "searchindexer.exe": true, "searchhost.exe": true, "msmpeng.exe": true,
	"runtimebroker.exe": true, "dllhost.exe": true, "rundll32.exe": true, "cmd.exe": true, "powershell.exe": true,
	"pwsh.exe": true, "wmiprvse.exe": true, "mmc.exe": true, "regedit.exe": true, "taskmgr.exe": true,
	"startmenuexperiencehost.exe": true, "shellexperiencehost.exe": true, "textinputhost.exe": true,
	"securityhealthservice.exe": true, "memcompression": true, "wudfhost.exe": true, "lsm.exe": true,
	"logonui.exe": true, "userinit.exe": true, "consent.exe": true, "werfault.exe": true, "smartscreen.exe": true,
	"mpdefendercoreservice.exe": true, "nissrv.exe": true,
}

// inWindowsDir reports whether path is inside the Windows folder: whatever
// lives there is part of Windows and never gets a game profile, even when
// its name is not in systemExes.
func inWindowsDir(path string) bool {
	root := strings.TrimSpace(os.Getenv("SystemRoot"))
	if root == "" || path == "" {
		return false
	}
	p := strings.ToLower(filepath.Clean(path))
	r := strings.ToLower(filepath.Clean(root))
	return strings.HasPrefix(p, r+string(filepath.Separator))
}

// SystemExe reports whether name (a file name like "dwm.exe") is a Windows
// system process that game profiles must not touch.
func SystemExe(name string) bool {
	return systemExes[strings.ToLower(strings.TrimSpace(name))]
}

// ProfileProvider builds per-game tweaks on demand.
func ProfileProvider(id string, f *facts.Facts) *tweaks.Tweak {
	exe, feature, ok := ParseProfileID(id)
	if !ok || !strings.HasSuffix(exe, ".exe") || strings.ContainsAny(exe, `\/:*?"<>|`) {
		return nil
	}
	path := GamePath(exe)
	gameOnly := func(*facts.Facts) (bool, string) {
		if SystemExe(exe) {
			return false, "processo do Windows: perfis de jogo não alteram processos do sistema"
		}
		if path == "" {
			return false, "caminho do jogo não encontrado"
		}
		if inWindowsDir(path) {
			return false, "programa da pasta do Windows: perfis de jogo não alteram componentes do sistema"
		}
		return true, ""
	}
	switch feature {
	case "priority":
		return &tweaks.Tweak{
			Applicable: gameOnly,
			ID:         id, Category: "jogos", Profile: exe, Title: "Prioridade alta de CPU para " + exe,
			Summary: "O Windows passa a iniciar " + exe + " sempre com prioridade Alta, dando preferência às threads do jogo quando algo em segundo plano disputa a CPU.",
			Detail: "CpuPriorityClass=3 (Alta) em " + keyIFEO + "\\" + exe + "\\PerfOptions — o mecanismo de prioridade persistente do próprio Windows, aplicado ao abrir o processo. " +
				"O ganho aparece quando há concorrência de CPU (gravação, navegador, antivírus); com a CPU livre o efeito é nulo. Não use \"Tempo real\".",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcIFEO},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestartApp, Tags: []string{"fps"}, OneClick: true,
			Recommend: neutral("útil quando há programas pesados abertos junto com o jogo"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{tweaks.Reg(keyIFEO+`\`+exe+`\PerfOptions`, "CpuPriorityClass", sys.DWord(3))}
			},
		}
	case "gpu":
		return &tweaks.Tweak{
			ID: id, Category: "jogos", Profile: exe, Title: "GPU de alto desempenho para " + exe,
			Summary:  "Força o jogo a rodar na GPU dedicada em PCs com vídeo integrado + dedicado (comum em notebooks).",
			Detail:   "GpuPreference=2 no valor do caminho do executável em HKCU\\Software\\Microsoft\\DirectX\\UserGpuPreferences, o mesmo que Configurações > Tela > Elementos gráficos > Alto desempenho.",
			Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcGpuPref},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestartApp, Tags: []string{"fps"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) {
				if ok, why := gameOnly(f); !ok {
					return false, why
				}
				if !multiGPU(f) {
					return false, "o PC tem apenas uma GPU"
				}
				return true, ""
			},
			Recommend: yes("garante que o jogo não rode no vídeo integrado"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{dxPrefOp(keyUserGpuPrefs, path, "GpuPreference", "2")}
			},
		}
	case "fso":
		return &tweaks.Tweak{
			ID: id, Category: "jogos", Profile: exe, Title: "Desativar otimizações de tela cheia para " + exe,
			Summary: "Faz o jogo usar tela cheia exclusiva \"clássica\" em vez do modo otimizado do Windows.",
			Detail: "Acrescenta DISABLEDXMAXIMIZEDWINDOWEDMODE às camadas de compatibilidade do executável (HKCU\\Software\\Microsoft\\Windows NT\\CurrentVersion\\AppCompatFlags\\Layers), o mesmo que a caixa \"Desabilitar otimizações de tela inteira\" nas propriedades do .exe. " +
				"A Microsoft afirma que o modo otimizado tem desempenho, em média, igual ou melhor que a tela cheia exclusiva, mas reconhece casos de queda de desempenho ou atraso de entrada; relatos variam por jogo e driver. Compare no seu jogo antes de manter.",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcFSO},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestartApp, Tags: []string{"latencia"}, OneClick: false,
			Applicable: gameOnly,
			Recommend:  neutral("efeito depende do jogo; meça antes e depois"),
			Ops: func(*facts.Facts) []tweaks.Op {
				return []tweaks.Op{layerFlagOp(path, "DISABLEDXMAXIMIZEDWINDOWEDMODE")}
			},
		}
	case "nvpstate":
		return nvPStateProfile(id, exe, gameOnly)
	}
	return nil
}

// dxPrefOp merges NAME=VALUE; into a DirectX preference string value.
func dxPrefOp(key, valueName, name, value string) tweaks.Op {
	key = sys.NormalizeRegPath(key)
	read := func(c *tweaks.Ctx) (sys.RegValue, error) { return c.P.Reg.GetValue(key, valueName) }
	return &tweaks.FuncOp{
		KindName: "registry-merge", ID: strings.ToLower(key + "|" + valueName + "|" + name), Spec: dxSpec{Key: key, ValueName: valueName, Name: name, Value: value},
		Desc:   fmt.Sprintf(`%s → "%s" contém %s=%s; (outras opções preservadas)`, key, valueName, name, value),
		SnapFn: func(c *tweaks.Ctx) (any, error) { return read(c) },
		InFn: func(c *tweaks.Ctx) (bool, error) {
			v, err := read(c)
			if err != nil || !v.Exists {
				return false, err
			}
			_, vals := parseDXSettings(v.Str)
			return vals[strings.ToLower(name)] == value, nil
		},
		ApplyFn: func(c *tweaks.Ctx) error {
			v, err := read(c)
			if err != nil {
				return err
			}
			keys, vals := parseDXSettings(v.Str)
			if _, ok := vals[strings.ToLower(name)]; !ok {
				keys = append(keys, name)
			}
			vals[strings.ToLower(name)] = value
			return c.P.Reg.SetValue(key, valueName, sys.SZ(formatDXSettings(keys, vals)))
		},
		// Only the option this op owns is put back; other options in the
		// same string (changed later by Settings, e.g. Auto HDR) are kept.
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var before sys.RegValue
			if err := json.Unmarshal(raw, &before); err != nil {
				return err
			}
			cur, err := read(c)
			if err != nil {
				return err
			}
			keys, vals := parseDXSettings(cur.Str)
			bk, bv := parseDXSettings(before.Str)
			lname := strings.ToLower(name)
			if old, had := bv[lname]; before.Exists && had {
				if _, ok := vals[lname]; !ok {
					keys = append(keys, keyName(bk, name))
				}
				vals[lname] = old
			} else {
				delete(vals, lname)
				keys = dropKey(keys, name)
			}
			if len(keys) == 0 && !before.Exists {
				return c.P.Reg.DeleteValue(key, valueName)
			}
			return c.P.Reg.SetValue(key, valueName, sys.SZ(formatDXSettings(keys, vals)))
		},
		EqFn: func(a, b json.RawMessage) bool {
			var va, vb sys.RegValue
			if json.Unmarshal(a, &va) != nil || json.Unmarshal(b, &vb) != nil {
				return false
			}
			_, xa := parseDXSettings(va.Str)
			_, xb := parseDXSettings(vb.Str)
			oa, ha := xa[strings.ToLower(name)]
			ob, hb := xb[strings.ToLower(name)]
			return ha == hb && oa == ob
		},
		CurFn: func(c *tweaks.Ctx) string {
			v, err := read(c)
			if err != nil {
				return "erro: " + err.Error()
			}
			return v.Display()
		},
	}
}

// keyName returns the spelling of name used in keys (case-insensitive).
func keyName(keys []string, name string) string {
	for _, k := range keys {
		if strings.EqualFold(k, name) {
			return k
		}
	}
	return name
}

// dropKey removes name (case-insensitive) from keys.
func dropKey(keys []string, name string) []string {
	out := keys[:0]
	for _, k := range keys {
		if !strings.EqualFold(k, name) {
			out = append(out, k)
		}
	}
	return out
}

// layerFlagOp adds a compatibility layer flag to an executable's Layers value.
func layerFlagOp(exePath, flag string) tweaks.Op {
	key := `HKCU\Software\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Layers`
	read := func(c *tweaks.Ctx) (sys.RegValue, error) { return c.P.Reg.GetValue(key, exePath) }
	has := func(s string) bool {
		for _, t := range strings.Fields(s) {
			if strings.EqualFold(t, flag) {
				return true
			}
		}
		return false
	}
	return &tweaks.FuncOp{
		KindName: "registry-layer", ID: strings.ToLower(exePath + "|" + flag), Spec: layerSpec{Exe: exePath, Flag: flag},
		Desc:   fmt.Sprintf(`%s → "%s" contém %s`, key, exePath, flag),
		SnapFn: func(c *tweaks.Ctx) (any, error) { return read(c) },
		InFn: func(c *tweaks.Ctx) (bool, error) {
			v, err := read(c)
			if err != nil || !v.Exists {
				return false, err
			}
			return has(v.Str), nil
		},
		ApplyFn: func(c *tweaks.Ctx) error {
			v, err := read(c)
			if err != nil {
				return err
			}
			if v.Exists && has(v.Str) {
				return nil
			}
			tokens := strings.Fields(v.Str)
			if len(tokens) == 0 || tokens[0] != "~" {
				tokens = append([]string{"~"}, tokens...)
			}
			tokens = append(tokens, flag)
			return c.P.Reg.SetValue(key, exePath, sys.SZ(strings.Join(tokens, " ")))
		},
		// Only this flag is put back or removed; other compatibility flags
		// set later (by the user in the .exe properties) are kept.
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var before sys.RegValue
			if err := json.Unmarshal(raw, &before); err != nil {
				return err
			}
			cur, err := read(c)
			if err != nil {
				return err
			}
			want := before.Exists && has(before.Str)
			var tokens []string
			for _, t := range strings.Fields(cur.Str) {
				if !strings.EqualFold(t, flag) {
					tokens = append(tokens, t)
				}
			}
			if want {
				if len(tokens) == 0 || tokens[0] != "~" {
					tokens = append([]string{"~"}, tokens...)
				}
				tokens = append(tokens, flag)
			}
			rest := 0
			for _, t := range tokens {
				if t != "~" {
					rest++
				}
			}
			if rest == 0 {
				if !before.Exists {
					return c.P.Reg.DeleteValue(key, exePath)
				}
				return c.P.Reg.SetValue(key, exePath, before) // it held only a marker: back as it was
			}
			return c.P.Reg.SetValue(key, exePath, sys.SZ(strings.Join(tokens, " ")))
		},
		EqFn: func(a, b json.RawMessage) bool {
			var va, vb sys.RegValue
			if json.Unmarshal(a, &va) != nil || json.Unmarshal(b, &vb) != nil {
				return false
			}
			return (va.Exists && has(va.Str)) == (vb.Exists && has(vb.Str))
		},
		CurFn: func(c *tweaks.Ctx) string {
			v, err := read(c)
			if err != nil {
				return "erro: " + err.Error()
			}
			return v.Display()
		},
	}
}

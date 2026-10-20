package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"axion/internal/sys"
	"axion/internal/tweaks"
)

// ---------------------------------------------------------------------------
// Bits of a REG_SZ flags value (accessibility settings)
// ---------------------------------------------------------------------------

// bitsSnap records only the bits an operation owns, so two tweaks that
// change different bits of the same value (Filter Keys on/off and the Filter
// Keys hotkey) can be applied and undone independently.
type bitsSnap struct {
	Exists bool `json:"exists"`
	Bits   int  `json:"bits"`
}

type bitsSpec struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Mask  int    `json:"mask"`
	Def   int    `json:"def"`
	Label string `json:"label"`
}

// bitsClearOp clears mask in a decimal REG_SZ flags value (the format of
// HKCU\Control Panel\Accessibility\*\Flags), preserving every other bit.
func bitsClearOp(path, name string, mask, def int, label string) tweaks.Op {
	path = sys.NormalizeRegPath(path)
	read := func(c *tweaks.Ctx) (sys.RegValue, error) { return c.P.Reg.GetValue(path, name) }
	cur := func(v sys.RegValue) int {
		if !v.Exists {
			return def
		}
		if v.Kind == sys.RegDWord {
			return int(v.Int)
		}
		return atoiDef(v.Str, def)
	}
	// set keeps the value's own type: Windows stores these Flags as decimal
	// strings, but a DWORD left by another tool stays a DWORD.
	set := func(c *tweaks.Ctx, v sys.RegValue, n int) error {
		if v.Exists && v.Kind == sys.RegDWord {
			return c.P.Reg.SetValue(path, name, sys.DWord(uint32(n)))
		}
		return c.P.Reg.SetValue(path, name, sys.SZ(strconv.Itoa(n)))
	}
	return &tweaks.FuncOp{
		KindName: "registry-bits", ID: strings.ToLower(path + "|" + name + "|" + strconv.Itoa(mask)),
		Spec: bitsSpec{Path: path, Name: name, Mask: mask, Def: def, Label: label},
		Desc: fmt.Sprintf(`%s → "%s": desliga o bit 0x%X (%s), preservando os demais`, path, name, mask, label),
		SnapFn: func(c *tweaks.Ctx) (any, error) {
			v, err := read(c)
			if err != nil {
				return nil, err
			}
			return bitsSnap{Exists: v.Exists, Bits: cur(v) & mask}, nil
		},
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
			return set(c, v, cur(v)&^mask)
		},
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var s bitsSnap
			if err := json.Unmarshal(raw, &s); err != nil {
				return err
			}
			v, err := read(c)
			if err != nil {
				return err
			}
			// Only this op's bits come back from the record, whatever it holds.
			n := cur(v)&^mask | (s.Bits & mask)
			if !s.Exists && n == def {
				return c.P.Reg.DeleteValue(path, name)
			}
			return set(c, v, n)
		},
		CurFn: func(c *tweaks.Ctx) string {
			v, err := read(c)
			if err != nil {
				return "erro: " + err.Error()
			}
			state := "ligado"
			if cur(v)&mask == 0 {
				state = "desligado"
			}
			return fmt.Sprintf("%s %s (Flags=%d)", label, state, cur(v))
		},
	}
}

// ---------------------------------------------------------------------------
// "Allow the computer to turn off this device to save power"
// ---------------------------------------------------------------------------

type devPowerSpec struct {
	WMIName string `json:"wmiName"`
}

type devPowerSnap struct {
	Found   bool `json:"found"`
	Enabled bool `json:"enabled"`
}

func wqlString(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `'`, `\'`)
}

// devicePowerOp unticks the Device Manager power checkbox of one device node
// through WMI (MSPower_DeviceEnable), the same interface Device Manager uses;
// the driver applies it immediately.
func devicePowerOp(wmiName string) tweaks.Op {
	q := fmt.Sprintf(`SELECT InstanceName, Enable FROM MSPower_DeviceEnable WHERE InstanceName='%s'`, wqlString(wmiName))
	// Writes need the full instance: Put_ of a partial one fails with
	// "Objeto inválido" (real report 0.3.2).
	qAll := fmt.Sprintf(`SELECT * FROM MSPower_DeviceEnable WHERE InstanceName='%s'`, wqlString(wmiName))
	read := func(c *tweaks.Ctx) (devPowerSnap, error) {
		rows, err := c.P.WMI.Query(`root\WMI`, q)
		if err != nil {
			return devPowerSnap{}, err
		}
		if len(rows) == 0 {
			return devPowerSnap{}, nil
		}
		return devPowerSnap{Found: true, Enabled: rows[0].Bool("Enable")}, nil
	}
	dev := strings.TrimSuffix(wmiName, "_0")
	return &tweaks.FuncOp{
		KindName: "device-power", ID: strings.ToUpper(wmiName), Spec: devPowerSpec{WMIName: wmiName},
		Desc:   fmt.Sprintf(`%s → desmarca "Permitir que o computador desligue este dispositivo para economizar energia" (WMI MSPower_DeviceEnable)`, dev),
		SnapFn: func(c *tweaks.Ctx) (any, error) { return read(c) },
		// A device that is no longer connected (a wireless receiver moved to
		// another port gets a new instance) has nothing to restore: it is
		// neither a change by another program nor a failed undo.
		EqFn: func(a, b json.RawMessage) bool {
			var sa, sb devPowerSnap
			if json.Unmarshal(a, &sa) != nil || json.Unmarshal(b, &sb) != nil {
				return false
			}
			return !sa.Found || !sb.Found || sa.Enabled == sb.Enabled
		},
		InFn: func(c *tweaks.Ctx) (bool, error) {
			s, err := read(c)
			if err != nil {
				return false, err
			}
			return !s.Found || !s.Enabled, nil
		},
		ApplyFn: func(c *tweaks.Ctx) error {
			n, err := c.P.WMI.Put(`root\WMI`, qAll, "Enable", false)
			if err != nil {
				return err
			}
			if n == 0 {
				return errors.New("dispositivo não encontrado (foi desconectado?)")
			}
			return nil
		},
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var s devPowerSnap
			if err := json.Unmarshal(raw, &s); err != nil {
				return err
			}
			if !s.Found {
				return nil
			}
			_, err := c.P.WMI.Put(`root\WMI`, qAll, "Enable", s.Enabled)
			return err
		},
		CurFn: func(c *tweaks.Ctx) string {
			s, err := read(c)
			switch {
			case err != nil:
				return "erro: " + err.Error()
			case !s.Found:
				return "dispositivo sem essa opção (ou desconectado)"
			case s.Enabled:
				return "o Windows pode desligar o dispositivo"
			}
			return "desligamento para economia bloqueado"
		},
	}
}

// ---------------------------------------------------------------------------
// HID selective suspend enabled by the vendor (Device Parameters)
// ---------------------------------------------------------------------------

type selSuspendSpec struct {
	Node string `json:"node"`
}

const keyEnumRoot = `HKLM\SYSTEM\CurrentControlSet\Enum\`

func regOn(v sys.RegValue) bool {
	if !v.Exists {
		return false
	}
	switch v.Kind {
	case sys.RegBinary:
		for _, b := range v.Bin {
			if b != 0 {
				return true
			}
		}
		return false
	case sys.RegSZ, sys.RegExpandSZ:
		s := strings.TrimSpace(v.Str)
		return s != "" && s != "0"
	}
	return v.Int != 0
}

// selSuspendOp turns SelectiveSuspendEnabled off for one USB node, keeping
// the value type the vendor INF used (REG_BINARY in Microsoft's example).
// The HID driver reads it when the device starts: replug or restart.
func selSuspendOp(node string) tweaks.Op {
	path := keyEnumRoot + node + `\Device Parameters`
	const name = "SelectiveSuspendEnabled"
	read := func(c *tweaks.Ctx) (sys.RegValue, error) { return c.P.Reg.GetValue(path, name) }
	return &tweaks.FuncOp{
		KindName: "selective-suspend", ID: strings.ToUpper(node), Spec: selSuspendSpec{Node: node},
		Desc:   fmt.Sprintf(`%s → "%s" = 0 (mesmo tipo de valor)`, path, name),
		SnapFn: func(c *tweaks.Ctx) (any, error) { return read(c) },
		InFn: func(c *tweaks.Ctx) (bool, error) {
			v, err := read(c)
			if err != nil {
				return false, err
			}
			return !regOn(v), nil
		},
		ApplyFn: func(c *tweaks.Ctx) error {
			v, err := read(c)
			if err != nil {
				return err
			}
			off := sys.DWord(0)
			switch v.Kind {
			case sys.RegBinary:
				n := len(v.Bin)
				if n == 0 {
					n = 1
				}
				off = sys.Binary(make([]byte, n))
			case sys.RegSZ:
				off = sys.SZ("0")
			}
			return c.P.Reg.SetValue(path, name, off)
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
			if regOn(v) {
				return "suspensão seletiva ligada pelo fabricante"
			}
			return "suspensão seletiva desligada"
		},
	}
}

func init() {
	register("registry-bits", func(s bitsSpec) tweaks.Op { return bitsClearOp(s.Path, s.Name, s.Mask, s.Def, s.Label) })
	register("device-power", func(s devPowerSpec) tweaks.Op { return devicePowerOp(s.WMIName) })
	register("selective-suspend", func(s selSuspendSpec) tweaks.Op { return selSuspendOp(s.Node) })
}

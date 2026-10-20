package catalog

import (
	"encoding/json"

	"axion/internal/tweaks"
)

// Specs of the catalog's custom operations. They are stored in the journal
// so an operation can be rebuilt for undo even after the machine changed
// (e.g. the monitor is no longer below its max refresh rate).
type (
	bcdSpec struct {
		Name string `json:"name"`
		Want string `json:"want"`
	}
	flagSpec struct {
		Path  string `json:"path"`
		Name  string `json:"name"`
		Mask  int    `json:"mask"`
		Def   int    `json:"def"`
		Label string `json:"label"`
	}
	tasksSpec struct {
		ID    string   `json:"id"`
		Paths []string `json:"paths"`
	}
	featureSpec struct {
		Name  string `json:"name"`
		Label string `json:"label"`
	}
	displaySpec struct {
		Device  string `json:"device"`
		Monitor string `json:"monitor"`
	}
	overlaySpec struct {
		GUID  string `json:"guid"`
		Label string `json:"label"`
	}
	dxSpec struct {
		Key       string `json:"key"`
		ValueName string `json:"valueName"`
		Name      string `json:"name"`
		Value     string `json:"value"`
	}
	layerSpec struct {
		Exe  string `json:"exe"`
		Flag string `json:"flag"`
	}
	taskEnableSpec struct {
		Path string `json:"path"`
	}
	mmagentSpec struct {
		Want bool `json:"want"`
	}
	dnsSpec struct {
		GUID string   `json:"guid"`
		V4   []string `json:"v4"`
		V6   []string `json:"v6"`
	}
	// cs2VideoSpec: one key of cs2_video.txt. Want and OK are here (not only
	// Path and Key) so the journal can rebuild InTarget and the drift without
	// the facts. The strings are stored in the journal: never change them.
	cs2VideoSpec struct {
		Path string   `json:"path"`
		Key  string   `json:"key"`
		Want string   `json:"want"`
		OK   []string `json:"ok,omitempty"`
	}
	// nvdrsSpec: one setting of one NVIDIA driver profile ("base", "global"
	// or "app:<exe>"); Mode "set" writes Value, "default" removes the user
	// value. The strings are stored in the journal: never change them.
	nvdrsSpec struct {
		Profile string `json:"profile"`
		ID      uint32 `json:"id"`
		Mode    string `json:"mode"`
		Value   uint32 `json:"value,omitempty"`
	}
)

func register[T any](kind string, build func(s T) tweaks.Op) {
	tweaks.RegisterKind(kind, func(raw json.RawMessage) (tweaks.Op, error) {
		var s T
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		return build(s), nil
	})
}

func init() {
	register("bcd", func(s bcdSpec) tweaks.Op { return bcdOp(s.Name, s.Want) })
	register("mouse", func(struct{}) tweaks.Op { return mouseOp() })
	register("registry-flag", func(s flagSpec) tweaks.Op { return flagClearOp(s.Path, s.Name, s.Mask, s.Def, s.Label) })
	register("hibernate", func(struct{}) tweaks.Op { return hibernateOp() })
	register("tcp-global", func(struct{}) tweaks.Op { return tcpDefaultsOp() })
	register("tasks", func(s tasksSpec) tweaks.Op { return tasksOp(s.ID, s.Paths) })
	register("feature", func(s featureSpec) tweaks.Op { return featureOp(s.Name, s.Label) })
	register("display", func(s displaySpec) tweaks.Op { return refreshOp(s.Device, s.Monitor) })
	register("power-overlay", func(s overlaySpec) tweaks.Op { return overlayOp(s.GUID, s.Label) })
	register("registry-merge", func(s dxSpec) tweaks.Op { return dxPrefOp(s.Key, s.ValueName, s.Name, s.Value) })
	register("registry-layer", func(s layerSpec) tweaks.Op { return layerFlagOp(s.Exe, s.Flag) })
	register("trim", func(struct{}) tweaks.Op { return trimOnOp() })
	register("task-enable", func(s taskEnableSpec) tweaks.Op { return taskEnableOp(s.Path) })
	register("mmagent", func(s mmagentSpec) tweaks.Op { return memCompressionOp(s.Want) })
	register("dns", func(s dnsSpec) tweaks.Op { return dnsOp(s.GUID, s.V4, s.V6) })
	register("nvdrs", func(s nvdrsSpec) tweaks.Op { return nvdrsOp(s.Profile, s.ID, s.Mode, s.Value) })
	register("cs2video", func(s cs2VideoSpec) tweaks.Op { return cs2VideoOp(s.Path, s.Key, s.Want, s.OK) })
}

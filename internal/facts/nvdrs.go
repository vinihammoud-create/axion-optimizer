package facts

import (
	"strings"

	"axion/internal/sys"
)

// NvDRS is what the NVIDIA driver's profile database says (NVAPI DRS), read
// without changing anything.
type NvDRS struct {
	Read bool `json:"read"`
	// Error says why it was not read (pt-BR).
	Error string `json:"error,omitempty"`
	// GlobalIsBase: the current global profile is the Base Profile itself
	// (the usual case), so "global" is not listed apart.
	GlobalIsBase bool               `json:"globalIsBase"`
	Profiles     []sys.NVDRSProfile `json:"profiles,omitempty"`
	VRR          []sys.NVVRRInfo    `json:"vrr,omitempty"`
}

// Profile returns the profile read for ref, or nil.
func (d NvDRS) Profile(ref string) *sys.NVDRSProfile {
	for i := range d.Profiles {
		if d.Profiles[i].Ref == ref {
			return &d.Profiles[i]
		}
	}
	return nil
}

// GlobalRefs are the profiles that apply to every program: the Base Profile
// and, when it is a different one, the current global profile.
func (d NvDRS) GlobalRefs() []string {
	if !d.GlobalIsBase && d.Profile(sys.NVDRSGlobal) != nil {
		return []string{sys.NVDRSBase, sys.NVDRSGlobal}
	}
	return []string{sys.NVDRSBase}
}

// AppRefs are the game profiles that exist in the driver, in the order read.
func (d NvDRS) AppRefs() []string {
	var out []string
	for _, p := range d.Profiles {
		if strings.HasPrefix(p.Ref, sys.NVDRSAppPrefix) && p.Found {
			out = append(out, p.Ref)
		}
	}
	return out
}

// UserValue is the value a user or a program wrote for id in ref (not
// inherited, not the driver's predefined value).
func (d NvDRS) UserValue(ref string, id uint32) (uint32, bool) {
	p := d.Profile(ref)
	if p == nil {
		return 0, false
	}
	s, ok := p.Get(id)
	if !ok || !s.User() {
		return 0, false
	}
	return s.Value, true
}

// Effective is the value that applies to ref: the game profile, then the
// current global profile, then the Base Profile, then the driver default.
// from is the ref it came from, or "padrão"; user says whether it is a user
// value there.
func (d NvDRS) Effective(ref string, id uint32) (v uint32, from string, user bool) {
	var chain []string
	if strings.HasPrefix(ref, sys.NVDRSAppPrefix) {
		chain = append(chain, ref)
	}
	if !d.GlobalIsBase {
		chain = append(chain, sys.NVDRSGlobal)
	}
	chain = append(chain, sys.NVDRSBase)
	for _, r := range chain {
		p := d.Profile(r)
		if p == nil || !p.Found {
			continue
		}
		if s, ok := p.Get(id); ok && s.Found && s.InProfile {
			return s.Value, r, !s.Predefined
		}
	}
	return sys.NVDRSDefaults[id], "padrão", false
}

// NVFRLFixable is the rule, shared by the diagnostic and the catalog, for a
// driver frame limit the fix may remove: above 0 and below the current
// refresh rate, and either the driver says there is no VRR, or there is
// VRR and the limit is far below the rate (less than 90 %, criterio do
// Axion). A limit a few FPS below the rate with VRR is the recommended
// configuration, and with the VRR state unknown Axion does not act.
func NVFRLFixable(v uint32, hz int, capable, known bool) bool {
	if hz <= 0 || v == 0 || int(v) >= hz {
		return false
	}
	return (known && !capable) || (capable && int(v) < hz*90/100)
}

// NvidiaVRR: capable when the driver reports VRR possible or on for some
// monitor; known when the driver answered for some monitor at all.
func (f *Facts) NvidiaVRR() (capable, known bool) {
	for _, v := range f.Nvidia.DRS.VRR {
		if v.Read {
			known = true
			if v.Possible || v.Enabled {
				capable = true
			}
		}
	}
	return
}

// PrimaryHz is the refresh rate the primary monitor runs at now (the one
// the G-SYNC ceiling and a frame limit relate to); the highest available
// at the resolution only when the current one is unknown. 0 = unknown.
func (f *Facts) PrimaryHz() int {
	var d *sys.DisplayMode
	for i := range f.Displays {
		if f.Displays[i].Primary {
			d = &f.Displays[i]
			break
		}
	}
	if d == nil && len(f.Displays) > 0 {
		d = &f.Displays[0]
	}
	if d == nil {
		return 0
	}
	if d.CurrentHz > 0 {
		return d.CurrentHz
	}
	return d.MaxHzAtRes
}

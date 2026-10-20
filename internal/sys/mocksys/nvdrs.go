package mocksys

import (
	"sort"
	"strings"
	"sync"

	"axion/internal/sys"
)

// DRSProfile is one profile of the simulated NVIDIA driver database.
type DRSProfile struct {
	Name   string
	Predef map[uint32]uint32 // the driver's own values in this profile
	User   map[uint32]uint32 // values written by a user or a program
}

// DRSSim simulates the NVIDIA profile database (NVAPI DRS) with the same
// semantics as the real API: a user value wins over the predefined one,
// Delete and RestoreDefault remove the user value (the predefined one comes
// back), and a setting that is not in the profile is Found=false.
type DRSSim struct {
	mu sync.Mutex
	// Absent: a PC without NVAPI; every call returns sys.ErrNoNVAPI.
	Absent bool
	// SaveErr makes SaveSettings fail (e.g. &sys.NVAPIError{Func:
	// "NvAPI_DRS_SaveSettings", Status: sys.NVAPIInvalidUserPrivilege});
	// nothing is written when it is set.
	SaveErr error
	// Profiles: "base"; "global" only when the current global profile is
	// NOT the Base Profile; "app:cs2.exe".
	Profiles map[string]*DRSProfile
	// VRR per GDI device, upper case key.
	VRR   map[string]sys.NVVRRInfo
	Saves int
}

func defaultDRS() *DRSSim {
	return &DRSSim{
		Profiles: map[string]*DRSProfile{
			sys.NVDRSBase: {Name: "Base Profile", Predef: map[uint32]uint32{
				sys.NVDRSVSyncMode: sys.NVVSyncPassive, sys.NVDRSFrameRateLimit: 0, sys.NVDRSPowerMode: sys.NVPowerOptimal,
				sys.NVDRSVRRMode: 1, sys.NVDRSShaderCache: 1, sys.NVDRSPrerenderLimit: 0}, User: map[uint32]uint32{}},
			sys.NVDRSAppRef("cs2.exe"): {Name: "Counter-strike 2", Predef: map[uint32]uint32{}, User: map[uint32]uint32{}},
		},
		// The driver answered: no VRR on the simulated monitor.
		VRR: map[string]sys.NVVRRInfo{`\\.\DISPLAY1`: {Device: `\\.\DISPLAY1`, Read: true}},
	}
}

func (d *DRSSim) resolve(ref string) *DRSProfile {
	if ref == sys.NVDRSGlobal {
		if g := d.Profiles[sys.NVDRSGlobal]; g != nil {
			return g
		}
		return d.Profiles[sys.NVDRSBase]
	}
	return d.Profiles[ref]
}

// ensure returns the profile of ref, creating an app profile when missing.
func (d *DRSSim) ensure(ref string) *DRSProfile {
	p := d.resolve(ref)
	if p == nil {
		p = &DRSProfile{Name: ref}
		d.Profiles[ref] = p
	}
	if p.Predef == nil {
		p.Predef = map[uint32]uint32{}
	}
	if p.User == nil {
		p.User = map[uint32]uint32{}
	}
	return p
}

// SetUser writes a user value, as another program would (tests set the PC up
// with it).
func (d *DRSSim) SetUser(ref string, id, v uint32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ensure(ref).User[id] = v
}

// SetPredef sets the driver's own value of a setting in a profile.
func (d *DRSSim) SetPredef(ref string, id, v uint32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ensure(ref).Predef[id] = v
}

func (p *DRSProfile) setting(id uint32) sys.NVDRSSetting {
	s := sys.NVDRSSetting{ID: id}
	pv, hasPre := p.Predef[id]
	if hasPre {
		s.PredefValid, s.PredefValue = true, pv
	}
	if v, ok := p.User[id]; ok {
		s.Found, s.InProfile, s.Value = true, true, v
		return s
	}
	if hasPre {
		s.Found, s.InProfile, s.Predefined, s.Value = true, true, true, pv
	}
	return s
}

func sortedKeys(m map[uint32]uint32) []uint32 {
	out := make([]uint32, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// NVDRSProfiles implements sys.Native.
func (n *Native) NVDRSProfiles(refs []string, ids []uint32, withUser bool) ([]sys.NVDRSProfile, error) {
	d := n.DRS
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]sys.NVDRSProfile, 0, len(refs))
	if d.Absent {
		return out, sys.ErrNoNVAPI
	}
	for _, ref := range refs {
		prof := sys.NVDRSProfile{Ref: ref}
		p := d.resolve(ref)
		if p == nil {
			out = append(out, prof)
			continue
		}
		prof.Found, prof.Name = true, p.Name
		for _, id := range ids {
			prof.Settings = append(prof.Settings, p.setting(id))
		}
		if withUser {
			for _, id := range sortedKeys(p.User) {
				prof.User = append(prof.User, p.setting(id))
			}
		}
		out = append(out, prof)
	}
	return out, nil
}

func (n *Native) drsWrite(ref, fn string, w func(p *DRSProfile)) error {
	n.mu.Lock()
	elevated := n.Elevated
	n.mu.Unlock()
	d := n.DRS
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.Absent {
		return sys.ErrNoNVAPI
	}
	p := d.resolve(ref)
	if p == nil {
		return &sys.NVAPIError{Func: fn, Status: sys.NVAPIProfileNotFound}
	}
	if !elevated {
		return &sys.NVAPIError{Func: "NvAPI_DRS_SaveSettings", Status: sys.NVAPIInvalidUserPrivilege}
	}
	if d.SaveErr != nil {
		return d.SaveErr // fails before changing: nothing written
	}
	w(p)
	d.Saves++
	return nil
}

// NVDRSSet implements sys.Native.
func (n *Native) NVDRSSet(ref string, id, v uint32) error {
	return n.drsWrite(ref, "NvAPI_DRS_SetSetting", func(p *DRSProfile) { p.User[id] = v })
}

// NVDRSRestoreDefault implements sys.Native.
func (n *Native) NVDRSRestoreDefault(ref string, id uint32) error {
	return n.drsWrite(ref, "NvAPI_DRS_RestoreProfileDefaultSetting", func(p *DRSProfile) { delete(p.User, id) })
}

// NVDRSDelete implements sys.Native.
func (n *Native) NVDRSDelete(ref string, id uint32) error {
	return n.drsWrite(ref, "NvAPI_DRS_DeleteProfileSetting", func(p *DRSProfile) { delete(p.User, id) })
}

// NVVRR implements sys.Native.
func (n *Native) NVVRR(device string) (sys.NVVRRInfo, error) {
	d := n.DRS
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.Absent {
		return sys.NVVRRInfo{Device: device}, sys.ErrNoNVAPI
	}
	if v, ok := d.VRR[strings.ToUpper(device)]; ok {
		v.Device = device
		return v, nil
	}
	return sys.NVVRRInfo{Device: device}, nil
}

// dump adds the database to a World snapshot: "nvdrs|<ref>|<id>" =
// "<value>|user" or "<value>|pre" (the predefined value only shows when no
// user value hides it), so a round trip catches a change of origin with the
// same value.
func (d *DRSSim) dump(w map[string]string, key func(ref string, id uint32) string, val func(v uint32, user bool) string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for ref, p := range d.Profiles {
		for id, v := range p.User {
			w[key(ref, id)] = val(v, true)
		}
		for id, v := range p.Predef {
			if _, u := p.User[id]; !u {
				w[key(ref, id)] = val(v, false)
			}
		}
	}
}

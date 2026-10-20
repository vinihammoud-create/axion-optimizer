package winsys

import (
	"encoding/binary"
	"testing"
	"unsafe"

	"axion/internal/sys"
)

// The sizes and offsets NVAPI expects (nvapi.h). A wrong layout makes the
// driver refuse the structure version or read garbage.
func TestNVAPILayout(t *testing.T) {
	sizes := map[string][2]uintptr{
		"NVDRS_SETTING_V1":     {unsafe.Sizeof(nvdrsSetting{}), 12320},
		"NVDRS_APPLICATION_V4": {unsafe.Sizeof(nvdrsApplication{}), 20492},
		"NVDRS_PROFILE_V1":     {unsafe.Sizeof(nvdrsProfileInfo{}), 4116},
		"NV_GET_VRR_INFO_V1":   {unsafe.Sizeof(nvVRRInfo{}), 24},
	}
	for name, s := range sizes {
		if s[0] != s[1] {
			t.Errorf("%s: %d bytes, o NVAPI espera %d", name, s[0], s[1])
		}
	}
	var st nvdrsSetting
	offs := map[string][2]uintptr{
		"SettingName":         {unsafe.Offsetof(st.SettingName), 4},
		"SettingID":           {unsafe.Offsetof(st.SettingID), 4100},
		"SettingType":         {unsafe.Offsetof(st.SettingType), 4104},
		"SettingLocation":     {unsafe.Offsetof(st.SettingLocation), 4108},
		"IsCurrentPredefined": {unsafe.Offsetof(st.IsCurrentPredefined), 4112},
		"IsPredefinedValid":   {unsafe.Offsetof(st.IsPredefinedValid), 4116},
		"Predefined":          {unsafe.Offsetof(st.Predefined), 4120},
		"Current":             {unsafe.Offsetof(st.Current), 8220},
	}
	var app nvdrsApplication
	for k, v := range map[string][2]uintptr{
		"AppName": {unsafe.Offsetof(app.AppName), 8}, "UserFriendlyName": {unsafe.Offsetof(app.UserFriendlyName), 4104},
		"Launcher": {unsafe.Offsetof(app.Launcher), 8200}, "FileInFolder": {unsafe.Offsetof(app.FileInFolder), 12296},
		"Flags": {unsafe.Offsetof(app.Flags), 16392}, "CommandLine": {unsafe.Offsetof(app.CommandLine), 16396},
	} {
		offs["NVDRS_APPLICATION."+k] = v
	}
	var pi nvdrsProfileInfo
	for k, v := range map[string][2]uintptr{
		"ProfileName": {unsafe.Offsetof(pi.ProfileName), 4}, "GPUSupport": {unsafe.Offsetof(pi.GPUSupport), 4100},
		"IsPredefined": {unsafe.Offsetof(pi.IsPredefined), 4104}, "NumOfApps": {unsafe.Offsetof(pi.NumOfApps), 4108},
		"NumOfSettings": {unsafe.Offsetof(pi.NumOfSettings), 4112},
	} {
		offs["NVDRS_PROFILE."+k] = v
	}
	var vr nvVRRInfo
	offs["NV_GET_VRR_INFO.Flags"] = [2]uintptr{unsafe.Offsetof(vr.Flags), 4}
	offs["NV_GET_VRR_INFO.ReservedEx"] = [2]uintptr{unsafe.Offsetof(vr.ReservedEx), 8}
	for name, o := range offs {
		if o[0] != o[1] {
			t.Errorf("deslocamento de %s = %d, esperado %d", name, o[0], o[1])
		}
	}
	for name, v := range map[string][3]uint32{
		"NVDRS_SETTING_VER":     {uint32(unsafe.Sizeof(nvdrsSetting{})) | 1<<16, nvdrsSettingVer, 0x00013020},
		"NVDRS_APPLICATION_VER": {uint32(unsafe.Sizeof(nvdrsApplication{})) | 4<<16, nvdrsApplicationVer, 0x0004500C},
		"NVDRS_PROFILE_VER":     {uint32(unsafe.Sizeof(nvdrsProfileInfo{})) | 1<<16, nvdrsProfileVer, 0x00011014},
		"NV_GET_VRR_INFO_VER":   {uint32(unsafe.Sizeof(nvVRRInfo{})) | 1<<16, nvVRRInfoVer, 0x00010018},
	} {
		if v[0] != v[1] || v[1] != v[2] {
			t.Errorf("%s: calculado 0x%08X, constante 0x%08X, nvapi.h 0x%08X", name, v[0], v[1], v[2])
		}
	}
	seen := map[uint32]bool{}
	for _, id := range nvapiIDs {
		if id == 0 || seen[id] {
			t.Errorf("id de nvapi_QueryInterface repetido ou zero: 0x%08X", id)
		}
		seen[id] = true
	}
	if len(seen) != 16 {
		t.Errorf("esperava 16 ids, há %d", len(seen))
	}

	// settingFromRaw: a user value stored in the profile being read.
	raw := &nvdrsSetting{SettingID: sys.NVDRSVSyncMode, SettingType: sys.NVDRSTypeDWORD, SettingLocation: sys.NVDRSLocCurrent}
	binary.LittleEndian.PutUint32(raw.Current[:4], sys.NVVSyncForceOn)
	if s := settingFromRaw("app:cs2.exe", raw); s.Value != sys.NVVSyncForceOn || !s.User() || !s.InProfile || s.Predefined {
		t.Errorf("valor de usuário no perfil: %+v", s)
	}
	raw.SettingLocation = sys.NVDRSLocBase
	if s := settingFromRaw(sys.NVDRSBase, raw); !s.InProfile {
		t.Errorf("location BASE lendo o Base Profile é 'no perfil': %+v", s)
	}
	if s := settingFromRaw(sys.NVDRSGlobal, raw); s.InProfile {
		t.Errorf("location BASE lendo o perfil global é herdado, não 'no perfil': %+v", s)
	}
	raw.SettingLocation = sys.NVDRSLocCurrent
	if s := settingFromRaw(sys.NVDRSGlobal, raw); !s.InProfile {
		t.Errorf("location CURRENT no perfil global é 'no perfil': %+v", s)
	}
	raw.SettingLocation = sys.NVDRSLocGlobal
	if s := settingFromRaw("app:cs2.exe", raw); s.InProfile || s.User() {
		t.Errorf("valor herdado do global num perfil de jogo: %+v", s)
	}
	// Predefined value with a valid predefined copy.
	raw.SettingLocation, raw.IsCurrentPredefined, raw.IsPredefinedValid = sys.NVDRSLocCurrent, 1, 1
	binary.LittleEndian.PutUint32(raw.Predefined[:4], sys.NVVSyncPassive)
	if s := settingFromRaw(sys.NVDRSBase, raw); s.User() || !s.Predefined || !s.PredefValid || s.PredefValue != sys.NVVSyncPassive {
		t.Errorf("valor predefinido: %+v", s)
	}
	// A non-DWORD setting keeps Value at 0.
	raw.SettingType = 1
	if s := settingFromRaw(sys.NVDRSBase, raw); s.Value != 0 || s.PredefValid {
		t.Errorf("ajuste binário não tem valor DWORD: %+v", s)
	}
}

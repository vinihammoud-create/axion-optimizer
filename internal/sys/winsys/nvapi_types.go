package winsys

import (
	"encoding/binary"

	"axion/internal/sys"
)

// NVAPI structures (nvapi.h, github.com/NVIDIA/nvapi), x64 layout. They have
// no build tag so the layout test runs anywhere; the calls live in nvapi.go.

// nvdrsSetting is NVDRS_SETTING_V1 (nvapi.h, #pragma pack(push,4)): 12320
// bytes (0x3020). The two unions hold a DWORD in their first 4 bytes.
type nvdrsSetting struct {
	Version             uint32       // @0
	SettingName         [2048]uint16 // @4    NvAPI_UnicodeString
	SettingID           uint32       // @4100
	SettingType         uint32       // @4104 NVDRS_SETTING_TYPE
	SettingLocation     uint32       // @4108 NVDRS_SETTING_LOCATION
	IsCurrentPredefined uint32       // @4112
	IsPredefinedValid   uint32       // @4116
	Predefined          [4100]byte   // @4120 union (u32 in the first 4 bytes; NVDRS_BINARY_SETTING = 4+4096)
	Current             [4100]byte   // @8220 union
}

const nvdrsSettingVer uint32 = 12320 | 1<<16 // 0x00013020 = MAKE_NVAPI_VERSION(NVDRS_SETTING_V1,1)

// nvdrsApplication is NVDRS_APPLICATION_V4 (no pack; everything 4-aligned):
// 20492 bytes (0x500C).
type nvdrsApplication struct {
	Version          uint32       // @0
	IsPredefined     uint32       // @4
	AppName          [2048]uint16 // @8
	UserFriendlyName [2048]uint16 // @4104
	Launcher         [2048]uint16 // @8200
	FileInFolder     [2048]uint16 // @12296
	Flags            uint32       // @16392 isMetro:1 isCommandLine:1 reserved:30
	CommandLine      [2048]uint16 // @16396
}

const nvdrsApplicationVer uint32 = 20492 | 4<<16 // 0x0004500C

// nvdrsProfileInfo is NVDRS_PROFILE_V1: 4116 bytes (0x1014).
type nvdrsProfileInfo struct {
	Version       uint32       // @0
	ProfileName   [2048]uint16 // @4
	GPUSupport    uint32       // @4100 NVDRS_GPU_SUPPORT (32 bits)
	IsPredefined  uint32       // @4104
	NumOfApps     uint32       // @4108
	NumOfSettings uint32       // @4112
}

const nvdrsProfileVer uint32 = 4116 | 1<<16 // 0x00011014

// nvVRRInfo is NV_GET_VRR_INFO_V1: 24 bytes.
type nvVRRInfo struct {
	Version    uint32    // @0
	Flags      uint32    // @4 bit0 bIsVRREnabled, bit1 bIsVRRPossible, bit2 bIsVRRRequested, bit3 bIsVRRIndicatorEnabled, bit4 bIsDisplayInVRRMode
	ReservedEx [4]uint32 // @8
}

const nvVRRInfoVer uint32 = 24 | 1<<16 // 0x00010018

// nvapi_QueryInterface ids (nvapi_interface.h).
const (
	nvidInitialize                   uint32 = 0x0150E828
	nvidCreateSession                uint32 = 0x0694D52E
	nvidDestroySession               uint32 = 0xDAD9CFF8
	nvidLoadSettings                 uint32 = 0x375DBD6B
	nvidSaveSettings                 uint32 = 0xFCBC7E14
	nvidGetBaseProfile               uint32 = 0xDA8466A0
	nvidGetCurrentGlobalProfile      uint32 = 0x617BFF9F
	nvidFindApplicationByName        uint32 = 0xEEE566B2
	nvidGetSetting                   uint32 = 0x73BF8338
	nvidSetSetting                   uint32 = 0x577DD202
	nvidRestoreProfileDefaultSetting uint32 = 0x53F0381E
	nvidDeleteProfileSetting         uint32 = 0xE4A26362
	nvidGetProfileInfo               uint32 = 0x61CD6FD6
	nvidEnumSettings                 uint32 = 0xAE3039DA
	nvidGetDisplayIdByDisplayName    uint32 = 0xAE457190
	nvidGetVRRInfo                   uint32 = 0xDF8FDA57
)

// nvapiIDs lists every id, for the test that checks they are distinct.
var nvapiIDs = []uint32{nvidInitialize, nvidCreateSession, nvidDestroySession, nvidLoadSettings, nvidSaveSettings, nvidGetBaseProfile,
	nvidGetCurrentGlobalProfile, nvidFindApplicationByName, nvidGetSetting, nvidSetSetting, nvidRestoreProfileDefaultSetting,
	nvidDeleteProfileSetting, nvidGetProfileInfo, nvidEnumSettings, nvidGetDisplayIdByDisplayName, nvidGetVRRInfo}

// settingFromRaw converts a NVDRS_SETTING the driver filled in. "In the
// profile" means stored in this very profile: settingLocation CURRENT, as
// nvidiaProfileInspector decides it for every profile; reading the Base
// Profile directly the driver may report BASE for its own values, which is
// accepted only for ref "base".
func settingFromRaw(ref string, s *nvdrsSetting) sys.NVDRSSetting {
	out := sys.NVDRSSetting{ID: s.SettingID, Found: true, Type: s.SettingType, Location: s.SettingLocation, Predefined: s.IsCurrentPredefined != 0}
	if s.SettingType == sys.NVDRSTypeDWORD {
		out.Value = binary.LittleEndian.Uint32(s.Current[:4])
		if s.IsPredefinedValid != 0 {
			out.PredefValid, out.PredefValue = true, binary.LittleEndian.Uint32(s.Predefined[:4])
		}
	}
	out.InProfile = s.SettingLocation == sys.NVDRSLocCurrent || (ref == sys.NVDRSBase && s.SettingLocation == sys.NVDRSLocBase)
	return out
}

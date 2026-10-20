package sys

import (
	"errors"
	"fmt"
	"strings"
)

// NVIDIA driver profiles (NVAPI DRS). The types and constants here are shared
// by the Windows implementation (winsys/nvapi.go), the simulator and the
// catalog. The profile references are stored in the journal Spec of the
// "nvdrs" operation: do not change them after a release.
const (
	NVDRSBase      = "base"   // NvAPI_DRS_GetBaseProfile ("Base Profile")
	NVDRSGlobal    = "global" // NvAPI_DRS_GetCurrentGlobalProfile
	NVDRSAppPrefix = "app:"   // NvAPI_DRS_FindApplicationByName(exe)
)

// NVDRSAppRef is the profile reference of a game executable.
func NVDRSAppRef(exe string) string {
	return NVDRSAppPrefix + strings.ToLower(strings.TrimSpace(exe))
}

// Setting IDs. Source: NvApiDriverSettings.h (github.com/NVIDIA/nvapi),
// except the Ultra Low Latency switch, which is only in the
// CustomSettingNames.xml of nvidiaProfileInspector.
const (
	NVDRSVSyncMode       uint32 = 0x00A879CF // VSYNCMODE_ID "Vertical Sync"
	NVDRSFrameRateLimit  uint32 = 0x10835002 // FRL_FPS_ID "Frame Rate Limiter" (0 = off)
	NVDRSPowerMode       uint32 = 0x1057EB71 // PREFERRED_PSTATE_ID "Power management mode"
	NVDRSVRRMode         uint32 = 0x1194F158 // VRR_MODE_ID (0 off, 1 fullscreen only = default, 2 fullscreen and windowed)
	NVDRSShaderCache     uint32 = 0x00198FFF // PS_SHADERDISKCACHE_ID (0 off, 1 on = default)
	NVDRSShaderCacheSize uint32 = 0x00AC8497 // PS_SHADERDISKCACHE_MAX_SIZE_ID (default 0x4000)
	NVDRSPrerenderLimit  uint32 = 0x007BA09E // PRERENDERLIMIT_ID (0 = application controlled = default)
	NVDRSUltraLowLatency uint32 = 0x10835000 // "Ultra Low Latency - Enabled" (0 off, 1 on)
	NVDRSRefreshOverride uint32 = 0x0064B541 // REFRESH_RATE_OVERRIDE_ID (0 = application controlled = default, 1 = highest available)
	NVDRSTextureQuality  uint32 = 0x00CE2691 // QUALITY_ENHANCEMENTS_ID (0 quality = default, 0x14 high performance, 0xFFFFFFF6 high quality, 0x0A performance)
	NVDRSOGLThreads      uint32 = 0x20C1221E // OGL_THREAD_CONTROL_ID (0 auto = default, 1 on, 2 off)
	NVDRSOGLTripleBuffer uint32 = 0x20FDD1F9 // OGL_TRIPLE_BUFFER_ID (0 = default)
)

// Values of VSYNCMODE (EValues_VSYNCMODE) and PREFERRED_PSTATE.
const (
	NVVSyncPassive  uint32 = 0x60925292 // default: "use the 3D application setting"
	NVVSyncForceOff uint32 = 0x08416747
	NVVSyncForceOn  uint32 = 0x47814940
	NVVSyncFlip2    uint32 = 0x32610244
	NVVSyncFlip3    uint32 = 0x71271021
	NVVSyncFlip4    uint32 = 0x13245256
	NVVSyncVirtual  uint32 = 0x18888888 // "Fast" (Fast Sync)

	NVPowerAdaptive   uint32 = 0
	NVPowerPreferMax  uint32 = 1
	NVPowerDriver     uint32 = 2
	NVPowerConsistent uint32 = 3
	NVPowerPreferMin  uint32 = 4
	NVPowerOptimal    uint32 = 5 // default
)

// NVDRSTypeDWORD is NVDRS_DWORD_TYPE (1 BINARY, 2 STRING, 3 WSTRING, 4 QWORD).
const NVDRSTypeDWORD uint32 = 0

// NVDRS_SETTING_LOCATION: where the current value of a setting comes from.
const (
	NVDRSLocCurrent uint32 = 0
	NVDRSLocGlobal  uint32 = 1
	NVDRSLocBase    uint32 = 2
	NVDRSLocDefault uint32 = 3
)

// NvAPI_Status (nvapi_lite_common.h).
const (
	NVAPIOK                        int32 = 0
	NVAPIGenericError              int32 = -1
	NVAPILibraryNotFound           int32 = -2
	NVAPINoImplementation          int32 = -3
	NVAPINotInitialized            int32 = -4
	NVAPIInvalidArgument           int32 = -5
	NVAPIDeviceNotFound            int32 = -6
	NVAPIEndEnumeration            int32 = -7
	NVAPIIncompatibleStructVersion int32 = -9
	NVAPINotSupported              int32 = -104
	NVAPIInvalidUserPrivilege      int32 = -137
	NVAPISettingNotFound           int32 = -160
	NVAPIProfileNotFound           int32 = -163
	NVAPIExecutableNotFound        int32 = -166
	NVAPIProfileRemoved            int32 = -169
	NVAPIPathAmbiguous             int32 = -182
)

// NVDRSSetting is one setting as read from a profile.
type NVDRSSetting struct {
	ID          uint32 `json:"id"`
	Found       bool   `json:"found"`      // GetSetting returned NVAPI_OK
	Type        uint32 `json:"type"`       // NVDRS_SETTING_TYPE
	Location    uint32 `json:"location"`   // NVDRS_SETTING_LOCATION
	InProfile   bool   `json:"inProfile"`  // stored in this profile (not inherited)
	Predefined  bool   `json:"predefined"` // isCurrentPredefined != 0 (the driver's own value)
	Value       uint32 `json:"value"`      // u32CurrentValue (DWORD only)
	PredefValid bool   `json:"predefValid,omitempty"`
	PredefValue uint32 `json:"predefValue,omitempty"`
}

// User reports whether the setting holds a value written by a user or a
// program in this profile (not inherited, not the driver's predefined one).
func (s NVDRSSetting) User() bool { return s.Found && s.InProfile && !s.Predefined }

// NVDRSProfile is one profile of the driver database.
type NVDRSProfile struct {
	Ref        string         `json:"ref"`
	Name       string         `json:"name,omitempty"` // "Base Profile", "Counter-strike 2"
	Found      bool           `json:"found"`
	Predefined bool           `json:"predefined,omitempty"`
	Settings   []NVDRSSetting `json:"settings,omitempty"` // the requested ids, in order
	User       []NVDRSSetting `json:"user,omitempty"`     // every setting with a user value (EnumSettings), by ID
}

// Get looks a setting up in Settings and then in User.
func (p NVDRSProfile) Get(id uint32) (NVDRSSetting, bool) {
	for _, s := range p.Settings {
		if s.ID == id {
			return s, true
		}
	}
	for _, s := range p.User {
		if s.ID == id {
			return s, true
		}
	}
	return NVDRSSetting{ID: id}, false
}

// NVVRRInfo is what the NVIDIA driver says about variable refresh on a
// display (NvAPI_Disp_GetVRRInfo).
type NVVRRInfo struct {
	Device    string `json:"device"`
	Read      bool   `json:"read"`      // the driver answered
	Enabled   bool   `json:"enabled"`   // bIsVRREnabled
	Possible  bool   `json:"possible"`  // bIsVRRPossible
	Requested bool   `json:"requested"` // bIsVRRRequested
	InVRRMode bool   `json:"inVrrMode"` // bIsDisplayInVRRMode
}

// NVDRSDefaults are the driver defaults (NvApiDriverSettings.h) of the
// settings Axion knows.
var NVDRSDefaults = map[uint32]uint32{
	NVDRSVSyncMode: NVVSyncPassive, NVDRSFrameRateLimit: 0, NVDRSPowerMode: NVPowerOptimal, NVDRSVRRMode: 1,
	NVDRSShaderCache: 1, NVDRSShaderCacheSize: 0x4000, NVDRSPrerenderLimit: 0, NVDRSUltraLowLatency: 0,
	NVDRSRefreshOverride: 0, NVDRSTextureQuality: 0, NVDRSOGLThreads: 0, NVDRSOGLTripleBuffer: 0,
}

// NVDRSReadIDs are the settings the collection reads in every profile.
var NVDRSReadIDs = []uint32{NVDRSVSyncMode, NVDRSFrameRateLimit, NVDRSPowerMode, NVDRSVRRMode, NVDRSShaderCache, NVDRSPrerenderLimit, NVDRSUltraLowLatency}

// NVDRSOptimizerIDs are the settings optimizer packs write in the global
// profile (nvidiaProfileInspector imports) and that
// fix.nvdrs.residue puts back to the driver default (V-Sync, the shader
// cache and the frame limiter have fixes of their own).
var NVDRSOptimizerIDs = []uint32{NVDRSPowerMode, NVDRSPrerenderLimit, NVDRSUltraLowLatency, NVDRSRefreshOverride, NVDRSTextureQuality, NVDRSOGLThreads, NVDRSOGLTripleBuffer, NVDRSShaderCacheSize}

// NVDRSName is the pt-BR name of a setting, as the NVIDIA Control Panel
// shows it where it has one.
func NVDRSName(id uint32) string {
	switch id {
	case NVDRSVSyncMode:
		return "Sincronização vertical (V-Sync)"
	case NVDRSFrameRateLimit:
		return "Taxa de quadros máxima (limitador do driver)"
	case NVDRSPowerMode:
		return "Modo de gerenciamento de energia"
	case NVDRSVRRMode:
		return "G-SYNC (ativação global)"
	case NVDRSShaderCache:
		return "Cache de shader"
	case NVDRSShaderCacheSize:
		return "Tamanho do cache de shader"
	case NVDRSPrerenderLimit:
		return "Máximo de quadros pré-renderizados (modo de baixa latência)"
	case NVDRSUltraLowLatency:
		return "Baixa latência Ultra"
	case NVDRSRefreshOverride:
		return "Taxa de atualização preferida"
	case NVDRSTextureQuality:
		return "Filtragem de textura - qualidade"
	case NVDRSOGLThreads:
		return "Otimização em threads"
	case NVDRSOGLTripleBuffer:
		return "Buffer triplo (OpenGL)"
	}
	return fmt.Sprintf("ajuste 0x%08X", id)
}

// NVDRSValueText is the pt-BR text of a value of a setting.
func NVDRSValueText(id, v uint32) string {
	switch id {
	case NVDRSVSyncMode:
		switch v {
		case NVVSyncPassive:
			return "usar a configuração do aplicativo (padrão)"
		case NVVSyncForceOff:
			return "desligado (forçado)"
		case NVVSyncForceOn:
			return "ligado (forçado)"
		case NVVSyncFlip2:
			return "ligado a 1/2 da taxa"
		case NVVSyncFlip3:
			return "ligado a 1/3 da taxa"
		case NVVSyncFlip4:
			return "ligado a 1/4 da taxa"
		case NVVSyncVirtual:
			return "Rápido (Fast Sync)"
		}
		return fmt.Sprintf("0x%08X (fora da lista do driver)", v)
	case NVDRSPowerMode:
		switch v {
		case NVPowerAdaptive:
			return "Adaptável"
		case NVPowerPreferMax:
			return "Preferir desempenho máximo"
		case NVPowerDriver:
			return "controlado pelo driver"
		case NVPowerConsistent:
			return "desempenho consistente"
		case NVPowerPreferMin:
			return "Preferir consumo mínimo"
		case NVPowerOptimal:
			return "Ideal (padrão)"
		}
	case NVDRSFrameRateLimit:
		if v == 0 {
			return "desligado (padrão)"
		}
		return fmt.Sprintf("%d FPS", v)
	case NVDRSShaderCache:
		switch v {
		case 0:
			return "desligado"
		case 1:
			return "ligado (padrão)"
		}
	case NVDRSShaderCacheSize:
		// The value is the cache size in MB (NvApiDriverSettings.h: default
		// 0x4000 = 16 GB; the Profile Inspector names 0xFFFFFFFF "Unlimited").
		switch {
		case v == 0:
			return "desligado"
		case v == 0xFFFFFFFF:
			return "ilimitado"
		case v%1024 == 0:
			return fmt.Sprintf("%d GB", v/1024)
		}
		return fmt.Sprintf("%d MB", v)
	case NVDRSVRRMode:
		switch v {
		case 0:
			return "desligado"
		case 1:
			return "só tela cheia (padrão)"
		case 2:
			return "tela cheia e janela"
		}
	case NVDRSTextureQuality:
		switch v {
		case 0:
			return "Qualidade (padrão)"
		case 0x0A:
			return "Desempenho"
		case 0x14:
			return "Alto desempenho"
		case 0xFFFFFFF6:
			return "Alta qualidade"
		}
	case NVDRSOGLThreads:
		switch v {
		case 0:
			return "automático (padrão)"
		case 1:
			return "ligado"
		case 2:
			return "desligado"
		}
	case NVDRSRefreshOverride:
		switch v {
		case 0:
			return "controlada pelo aplicativo (padrão)"
		case 1:
			return "maior disponível"
		}
	case NVDRSUltraLowLatency:
		switch v {
		case 0:
			return "desligado (padrão)"
		case 1:
			return "ligado"
		}
	case NVDRSPrerenderLimit:
		if v == 0 {
			return "controlado pelo aplicativo (padrão)"
		}
		if v == 1 {
			return "1 quadro"
		}
		return fmt.Sprintf("%d quadros", v)
	case NVDRSOGLTripleBuffer:
		switch v {
		case 0:
			return "desligado (padrão)"
		case 1:
			return "ligado"
		}
	}
	return fmt.Sprintf("%d (0x%X)", v, v)
}

// NVVSyncKnown reports whether v is one of the V-Sync values the driver
// defines. Values outside the list (the 0x60925619 some optimizer "revert" options write)
// are not "off": they are unknown.
func NVVSyncKnown(v uint32) bool {
	switch v {
	case NVVSyncPassive, NVVSyncForceOff, NVVSyncForceOn, NVVSyncFlip2, NVVSyncFlip3, NVVSyncFlip4, NVVSyncVirtual:
		return true
	}
	return false
}

// NVVSyncForcedOn reports whether v forces V-Sync on (at full or a fraction
// of the refresh rate).
func NVVSyncForcedOn(v uint32) bool {
	switch v {
	case NVVSyncForceOn, NVVSyncFlip2, NVVSyncFlip3, NVVSyncFlip4:
		return true
	}
	return false
}

// ErrNoNVAPI: the PC has no NVAPI (no NVIDIA driver, the generic Windows
// driver, or a driver without the function).
var ErrNoNVAPI = errors.New("NVAPI indisponível: driver NVIDIA ausente, genérico do Windows ou sem esta função")

// NVAPIError is a non-OK NvAPI_Status from one function.
type NVAPIError struct {
	Func   string
	Status int32
}

func (e *NVAPIError) Error() string {
	return fmt.Sprintf("%s: %s (NvAPI_Status %d)", e.Func, NVAPIStatusText(e.Status), e.Status)
}

// Unwrap maps the statuses that mean "no NVAPI on this PC" to ErrNoNVAPI.
func (e *NVAPIError) Unwrap() error {
	switch e.Status {
	case NVAPILibraryNotFound, NVAPINoImplementation, NVAPIDeviceNotFound:
		return ErrNoNVAPI
	}
	return nil
}

// NVAPIStatusText is the pt-BR text of an NvAPI_Status.
func NVAPIStatusText(st int32) string {
	switch st {
	case NVAPIOK:
		return "OK"
	case NVAPIInvalidUserPrivilege:
		return "o driver só grava o perfil com o Axion aberto como administrador"
	case NVAPISettingNotFound:
		return "ajuste não encontrado no perfil"
	case NVAPIProfileNotFound:
		return "perfil não encontrado"
	case NVAPIExecutableNotFound:
		return "o jogo não está em nenhum perfil do driver"
	case NVAPIPathAmbiguous:
		return "nome do executável ambíguo no driver"
	case NVAPIIncompatibleStructVersion:
		return "versão de estrutura não suportada por este driver"
	case NVAPINoImplementation:
		return "função ausente neste driver"
	case NVAPIDeviceNotFound:
		return "nenhuma placa NVIDIA com driver"
	case NVAPILibraryNotFound:
		return "biblioteca do NVAPI não encontrada"
	case NVAPINotInitialized:
		return "NVAPI não inicializado"
	case NVAPIInvalidArgument:
		return "argumento inválido"
	case NVAPINotSupported:
		return "não suportado nesta placa"
	}
	return "erro do driver NVIDIA"
}

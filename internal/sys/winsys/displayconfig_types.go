package winsys

// Layout of the display configuration structures (wingdi.h), kept free of
// build tags so their sizes are checked on any platform (see the test).

type luid struct {
	LowPart  uint32
	HighPart int32
}

// dcPathInfo is DISPLAYCONFIG_PATH_INFO (72 bytes).
type dcPathInfo struct {
	SourceAdapter     luid
	SourceID          uint32
	SourceModeIdx     uint32
	SourceStatus      uint32
	TargetAdapter     luid
	TargetID          uint32
	TargetModeIdx     uint32
	OutputTechnology  uint32
	Rotation          uint32
	Scaling           uint32
	RefreshNumerator  uint32
	RefreshDenom      uint32
	ScanLineOrdering  uint32
	TargetAvailable   int32
	TargetStatusFlags uint32
	Flags             uint32
}

// dcModeInfo is DISPLAYCONFIG_MODE_INFO (64 bytes); only its size matters.
type dcModeInfo struct {
	InfoType  uint32
	ID        uint32
	AdapterID luid
	Data      [48]byte
}

// dcHeader is DISPLAYCONFIG_DEVICE_INFO_HEADER (20 bytes).
type dcHeader struct {
	Type      uint32
	Size      uint32
	AdapterID luid
	ID        uint32
}

// dcSourceName is DISPLAYCONFIG_SOURCE_DEVICE_NAME (84 bytes).
type dcSourceName struct {
	Header            dcHeader
	ViewGdiDeviceName [32]uint16
}

// dcTargetName is DISPLAYCONFIG_TARGET_DEVICE_NAME (420 bytes).
type dcTargetName struct {
	Header                    dcHeader
	Flags                     uint32
	OutputTechnology          uint32
	EdidManufactureID         uint16
	EdidProductCodeID         uint16
	ConnectorInstance         uint32
	MonitorFriendlyDeviceName [64]uint16
	MonitorDevicePath         [128]uint16
}

// Output technologies of a built-in panel (DISPLAYCONFIG_VIDEO_OUTPUT_TECHNOLOGY).
const (
	outputTechLVDS                = 6
	outputTechDisplayPortEmbedded = 11
	outputTechUDIEmbedded         = 13
	outputTechInternal            = 0x80000000
)

// internalOutput reports a notebook or all-in-one panel.
func internalOutput(tech uint32) bool {
	switch tech {
	case outputTechInternal, outputTechLVDS, outputTechDisplayPortEmbedded, outputTechUDIEmbedded:
		return true
	}
	return false
}

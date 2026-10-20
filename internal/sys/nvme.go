package sys

import (
	"encoding/binary"
	"errors"
	"math"
)

// NVMe SMART / Health Information log (log page 02h), read with
// IOCTL_STORAGE_QUERY_PROPERTY + StorageDeviceProtocolSpecificProperty, as
// described in Microsoft Learn "Working with NVMe drives". The inbox
// stornvme.sys driver passes this Get Log Page command through for every
// NVMe SSD, so it works without vendor tools. Layout: NVM Express Base
// Specification, "SMART / Health Information (Log Identifier 02h)".

// Critical Warning bits (byte 0 of the log).
const (
	NVMeWarnSpare       = 1 << 0 // available spare below the threshold
	NVMeWarnTemperature = 1 << 1 // temperature above/below a threshold
	NVMeWarnReliability = 1 << 2 // reliability degraded (media or internal errors)
	NVMeWarnReadOnly    = 1 << 3 // media placed in read-only mode
	NVMeWarnBackup      = 1 << 4 // volatile memory backup device failed
	NVMeWarnPMR         = 1 << 5 // persistent memory region read-only
)

// NVMeHealth is the decoded health log of one NVMe controller.
type NVMeHealth struct {
	CriticalWarning  uint8  `json:"criticalWarning"`
	TempC            int    `json:"tempC"`          // composite temperature
	SparePct         int    `json:"sparePct"`       // available spare (%)
	SpareThreshold   int    `json:"spareThreshold"` // the controller's warning threshold (%)
	PercentUsed      int    `json:"percentUsed"`    // vendor estimate of endurance used; may exceed 100
	DataReadBytes    uint64 `json:"dataReadBytes"`
	DataWrittenBytes uint64 `json:"dataWrittenBytes"`
	PowerCycles      uint64 `json:"powerCycles"`
	PowerOnHours     uint64 `json:"powerOnHours"`
	UnsafeShutdowns  uint64 `json:"unsafeShutdowns"`
	MediaErrors      uint64 `json:"mediaErrors"` // unrecovered data integrity errors
	ErrorLogEntries  uint64 `json:"errorLogEntries"`
	WarnTempMinutes  uint32 `json:"warnTempMinutes"` // time above the warning temperature
	CritTempMinutes  uint32 `json:"critTempMinutes"` // time above the critical temperature
}

// Layout of the request/response buffer.
const (
	StoragePropertyQueryHeader = 8  // PropertyId + QueryType, AdditionalParameters follows
	ProtocolSpecificDataSize   = 40 // STORAGE_PROTOCOL_SPECIFIC_DATA
	ProtocolDataDescriptorSize = 8 + ProtocolSpecificDataSize
	NVMeHealthLogSize          = 512
	NVMeHealthQuerySize        = StoragePropertyQueryHeader + ProtocolSpecificDataSize + NVMeHealthLogSize

	storageDeviceProtocolSpecificProperty  = 50
	storageAdapterProtocolSpecificProperty = 49
	protocolTypeNvme                       = 3
	nvmeDataTypeLogPage                    = 2
	nvmeLogPageHealthInfo                  = 0x02
)

// IOCTLStorageQueryProperty is CTL_CODE(IOCTL_STORAGE_BASE, 0x500, METHOD_BUFFERED, FILE_ANY_ACCESS).
const IOCTLStorageQueryProperty = 0x2D1400

// NVMeHealthQuery builds the input buffer (also used as output buffer) for
// the device (namespace) scope, as in Microsoft's example.
func NVMeHealthQuery() []byte { return NVMeHealthQueryScope(false) }

// NVMeHealthQueryScope builds the query for the device scope or, with
// adapter set, for the controller (StorageAdapterProtocolSpecificProperty).
// Controllers that only keep the SMART/Health log for the whole controller
// (LPA bit 0 clear) reject the namespace-scoped request; the adapter scope
// asks for the controller-wide log instead.
func NVMeHealthQueryScope(adapter bool) []byte {
	b := make([]byte, NVMeHealthQuerySize)
	le := binary.LittleEndian
	prop := uint32(storageDeviceProtocolSpecificProperty)
	if adapter {
		prop = storageAdapterProtocolSpecificProperty
	}
	le.PutUint32(b[0:], prop)
	le.PutUint32(b[4:], 0) // PropertyStandardQuery
	p := b[StoragePropertyQueryHeader:]
	le.PutUint32(p[0:], protocolTypeNvme)
	le.PutUint32(p[4:], nvmeDataTypeLogPage)
	le.PutUint32(p[8:], nvmeLogPageHealthInfo)
	le.PutUint32(p[12:], 0)                        // log page offset (low)
	le.PutUint32(p[16:], ProtocolSpecificDataSize) // ProtocolDataOffset, from the start of this struct
	le.PutUint32(p[20:], NVMeHealthLogSize)        // ProtocolDataLength
	return b
}

// ParseNVMeHealthResponse validates the STORAGE_PROTOCOL_DATA_DESCRIPTOR
// returned in buf (n bytes) and decodes the log that follows it.
func ParseNVMeHealthResponse(buf []byte, n int) (NVMeHealth, error) {
	if n > len(buf) {
		n = len(buf)
	}
	if n < ProtocolDataDescriptorSize {
		return NVMeHealth{}, errors.New("resposta curta do driver NVMe")
	}
	le := binary.LittleEndian
	if le.Uint32(buf[0:]) != ProtocolDataDescriptorSize || le.Uint32(buf[4:]) != ProtocolDataDescriptorSize {
		return NVMeHealth{}, errors.New("descritor de dados do protocolo inválido")
	}
	psd := buf[8:ProtocolDataDescriptorSize]
	off, length := int(le.Uint32(psd[16:])), int(le.Uint32(psd[20:]))
	if off < ProtocolSpecificDataSize || length < NVMeHealthLogSize {
		return NVMeHealth{}, errors.New("deslocamento/tamanho do log NVMe inválido")
	}
	start := 8 + off
	if start+NVMeHealthLogSize > n {
		return NVMeHealth{}, errors.New("log de saúde NVMe incompleto")
	}
	return ParseNVMeHealthLog(buf[start : start+NVMeHealthLogSize])
}

// ParseNVMeHealthLog decodes the 512-byte SMART / Health Information log.
func ParseNVMeHealthLog(b []byte) (NVMeHealth, error) {
	if len(b) < NVMeHealthLogSize {
		return NVMeHealth{}, errors.New("log de saúde NVMe curto")
	}
	le := binary.LittleEndian
	h := NVMeHealth{
		CriticalWarning: b[0],
		SparePct:        int(b[3]),
		SpareThreshold:  int(b[4]),
		PercentUsed:     int(b[5]),
		PowerCycles:     u128(b[112:128]),
		PowerOnHours:    u128(b[128:144]),
		UnsafeShutdowns: u128(b[144:160]),
		MediaErrors:     u128(b[160:176]),
		ErrorLogEntries: u128(b[176:192]),
		WarnTempMinutes: le.Uint32(b[192:]),
		CritTempMinutes: le.Uint32(b[196:]),
	}
	if k := int(le.Uint16(b[1:])); k > 0 {
		h.TempC = k - 273
	} else {
		h.TempC = -1
	}
	// Data units are thousands of 512-byte blocks, rounded up.
	h.DataReadBytes = mulSat(u128(b[32:48]), 512_000)
	h.DataWrittenBytes = mulSat(u128(b[48:64]), 512_000)
	if h.TempC < -40 || h.TempC > 200 { // sensor not reported
		h.TempC = -1
	}
	return h, nil
}

// u128 reads a little-endian 128-bit counter, saturating at MaxUint64.
func u128(b []byte) uint64 {
	le := binary.LittleEndian
	if le.Uint64(b[8:16]) != 0 {
		return math.MaxUint64
	}
	return le.Uint64(b[0:8])
}

func mulSat(a, m uint64) uint64 {
	if a != 0 && a > math.MaxUint64/m {
		return math.MaxUint64
	}
	return a * m
}

// Warnings describes the Critical Warning bits in Portuguese.
func (h NVMeHealth) Warnings() []string {
	var w []string
	if h.CriticalWarning&NVMeWarnSpare != 0 {
		w = append(w, "reserva de blocos abaixo do limite do fabricante")
	}
	if h.CriticalWarning&NVMeWarnTemperature != 0 {
		w = append(w, "temperatura fora do limite")
	}
	if h.CriticalWarning&NVMeWarnReliability != 0 {
		w = append(w, "confiabilidade degradada por erros de mídia ou internos")
	}
	if h.CriticalWarning&NVMeWarnReadOnly != 0 {
		w = append(w, "mídia colocada em modo somente leitura")
	}
	if h.CriticalWarning&NVMeWarnBackup != 0 {
		w = append(w, "falha no backup da memória volátil")
	}
	if h.CriticalWarning&NVMeWarnPMR != 0 {
		w = append(w, "região de memória persistente somente leitura")
	}
	return w
}

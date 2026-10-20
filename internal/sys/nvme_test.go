package sys

import (
	"encoding/binary"
	"math"
	"testing"
)

// fakeNVMeResponse builds what stornvme returns for the health log query.
func fakeNVMeResponse(log func(b []byte)) []byte {
	buf := NVMeHealthQuery()
	le := binary.LittleEndian
	// The driver overwrites the query header with the data descriptor.
	le.PutUint32(buf[0:], ProtocolDataDescriptorSize)
	le.PutUint32(buf[4:], ProtocolDataDescriptorSize)
	log(buf[8+ProtocolSpecificDataSize:])
	return buf
}

func TestNVMeHealthQueryLayout(t *testing.T) {
	q := NVMeHealthQuery()
	le := binary.LittleEndian
	if len(q) != 560 {
		t.Fatalf("tamanho %d, esperado 560 (8 + 40 + 512)", len(q))
	}
	for off, want := range map[int]uint32{0: 50, 4: 0, 8: 3, 12: 2, 16: 2, 20: 0, 24: 40, 28: 512} {
		if got := le.Uint32(q[off:]); got != want {
			t.Errorf("offset %d = %d, esperado %d", off, got, want)
		}
	}
}

func TestParseNVMeHealth(t *testing.T) {
	le := binary.LittleEndian
	buf := fakeNVMeResponse(func(b []byte) {
		b[0] = NVMeWarnReliability
		le.PutUint16(b[1:], 318) // 45 °C
		b[3], b[4], b[5] = 100, 10, 87
		le.PutUint64(b[32:], 20_000_000)
		le.PutUint64(b[48:], 71_234_567)
		le.PutUint64(b[112:], 2_100)
		le.PutUint64(b[128:], 12_345)
		le.PutUint64(b[144:], 321)
		le.PutUint64(b[160:], 3)
		le.PutUint64(b[176:], 40)
		le.PutUint32(b[192:], 15)
	})
	h, err := ParseNVMeHealthResponse(buf, len(buf))
	if err != nil {
		t.Fatal(err)
	}
	want := NVMeHealth{CriticalWarning: 4, TempC: 45, SparePct: 100, SpareThreshold: 10, PercentUsed: 87,
		DataReadBytes: 20_000_000 * 512_000, DataWrittenBytes: 71_234_567 * 512_000, PowerCycles: 2_100, PowerOnHours: 12_345,
		UnsafeShutdowns: 321, MediaErrors: 3, ErrorLogEntries: 40, WarnTempMinutes: 15}
	if h != want {
		t.Fatalf("got %+v\nwant %+v", h, want)
	}
	if w := h.Warnings(); len(w) != 1 || w[0] != "confiabilidade degradada por erros de mídia ou internos" {
		t.Errorf("alertas: %v", w)
	}
}

func TestParseNVMeHealthRejectsBadDescriptor(t *testing.T) {
	buf := fakeNVMeResponse(func([]byte) {})
	binary.LittleEndian.PutUint32(buf[0:], 50) // query header left in place: driver did not answer
	if _, err := ParseNVMeHealthResponse(buf, len(buf)); err == nil {
		t.Fatal("descritor inválido aceito")
	}
	good := fakeNVMeResponse(func([]byte) {})
	if _, err := ParseNVMeHealthResponse(good, 100); err == nil {
		t.Fatal("resposta truncada aceita")
	}
}

func TestNVMeCountersSaturate(t *testing.T) {
	buf := fakeNVMeResponse(func(b []byte) {
		b[48+8] = 1 // bit 64 set: beyond uint64
		binary.LittleEndian.PutUint64(b[32:], math.MaxUint64/1000)
	})
	h, err := ParseNVMeHealthResponse(buf, len(buf))
	if err != nil {
		t.Fatal(err)
	}
	if h.DataWrittenBytes != math.MaxUint64 || h.DataReadBytes != math.MaxUint64 {
		t.Fatalf("sem saturação: %+v", h)
	}
	if h.TempC != -1 {
		t.Errorf("temperatura ausente deveria ser -1, veio %d", h.TempC)
	}
}

// Drives that keep the SMART/Health log only per controller reject the
// namespace-scoped query ("Incorrect function" in the real report); the
// retry uses the adapter (controller) scope.
func TestNVMeQueryScopes(t *testing.T) {
	dev, adp := NVMeHealthQueryScope(false), NVMeHealthQueryScope(true)
	if got := binary.LittleEndian.Uint32(dev[0:]); got != 50 {
		t.Errorf("escopo do dispositivo: PropertyId %d, esperado 50", got)
	}
	if got := binary.LittleEndian.Uint32(adp[0:]); got != 49 {
		t.Errorf("escopo do controlador: PropertyId %d, esperado 49", got)
	}
	if string(dev[4:]) != string(adp[4:]) {
		t.Error("as duas consultas só podem diferir no PropertyId")
	}
}

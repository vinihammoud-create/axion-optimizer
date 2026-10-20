package sys

import (
	"encoding/binary"
	"math"
	"testing"
	"time"
)

func TestOLEDate(t *testing.T) {
	cases := map[float64]time.Time{
		2:       time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC),
		45000:   time.Date(2023, 3, 15, 0, 0, 0, 0, time.UTC),
		46287.5: time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC),
	}
	for in, want := range cases {
		if got := OLEDate(in); !got.Equal(want) {
			t.Errorf("OLEDate(%v) = %v, esperado %v", in, got, want)
		}
	}
	if !OLEDate(0).IsZero() || !OLEDate(math.NaN()).IsZero() {
		t.Error("datas vazias deveriam virar zero")
	}
}

func dec(scale, sign byte, hi uint32, lo uint64) []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint16(b[0:], 14) // VT_DECIMAL in the reserved word
	b[2], b[3] = scale, sign
	binary.LittleEndian.PutUint32(b[4:], hi)
	binary.LittleEndian.PutUint64(b[8:], lo)
	return b
}

func TestDecodeDecimal(t *testing.T) {
	for _, c := range []struct {
		b    []byte
		want int64
	}{
		{dec(0, 0, 0, 1_039_000_000), 1_039_000_000}, // 990.85 MB driver
		{dec(2, 0, 0, 12345), 123},                   // 123.45 → 123
		{dec(0, 0x80, 0, 5), -5},
		{dec(0, 0, 1, 0), math.MaxInt64}, // 2^64 saturates
	} {
		got, err := DecodeDecimal(c.b)
		if err != nil || got != c.want {
			t.Errorf("DecodeDecimal = %d, %v; esperado %d", got, err, c.want)
		}
	}
	if _, err := DecodeDecimal(dec(29, 0, 0, 1)); err == nil {
		t.Error("escala inválida aceita")
	}
}

func TestWUError(t *testing.T) {
	if got := WUError(0x80070422); got != "o serviço Windows Update está desativado (algum otimizador pode ter desligado) (0x80070422)" {
		t.Errorf("%q", got)
	}
	if got := WUError(0x80249999); got != "erro 0x80249999 do Windows Update" {
		t.Errorf("%q", got)
	}
}

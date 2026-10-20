package sys

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"time"
)

// Helpers for OLE Automation values returned by COM APIs (Windows Update
// Agent). They are platform independent so they can be unit tested.

// OLEDate converts an Automation DATE (days since 1899-12-30, fraction =
// time of day) to UTC. Driver dates are whole days.
func OLEDate(days float64) time.Time {
	if math.IsNaN(days) || math.IsInf(days, 0) || days <= 0 {
		return time.Time{}
	}
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	whole := math.Floor(days)
	frac := days - whole
	return base.AddDate(0, 0, int(whole)).Add(time.Duration(frac * 24 * float64(time.Hour)).Round(time.Second))
}

// DecodeDecimal converts the 16 bytes of an OLE DECIMAL (the VARIANT memory
// of a VT_DECIMAL value: wReserved, scale, sign, Hi32, Lo64) to an int64,
// truncating the fraction and saturating at the int64 range.
func DecodeDecimal(b []byte) (int64, error) {
	if len(b) < 16 {
		return 0, fmt.Errorf("DECIMAL curto")
	}
	le := binary.LittleEndian
	scale, sign := b[2], b[3]
	if scale > 28 {
		return 0, fmt.Errorf("DECIMAL com escala inválida %d", scale)
	}
	hi := le.Uint32(b[4:8])
	lo := le.Uint64(b[8:16])
	v := new(big.Int).SetUint64(uint64(hi))
	v.Lsh(v, 64)
	v.Or(v, new(big.Int).SetUint64(lo))
	if scale > 0 {
		v.Quo(v, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil))
	}
	if !v.IsInt64() {
		if sign&0x80 != 0 {
			return math.MinInt64, nil
		}
		return math.MaxInt64, nil
	}
	n := v.Int64()
	if sign&0x80 != 0 {
		n = -n
	}
	return n, nil
}

// WUErrorText explains common Windows Update Agent HRESULTs in Portuguese.
func WUErrorText(hr uint32) string {
	switch hr {
	case 0:
		return ""
	case 0x80070422:
		return "o serviço Windows Update está desativado (algum otimizador pode ter desligado)"
	case 0x80070005:
		return "acesso negado: execute o Axion como administrador"
	case 0x8024402C, 0x80072EE7, 0x80072EFD, 0x80072EFE, 0x80072F8F, 0x8024401C, 0x80072EE2, 0x8024402F:
		return "sem conexão com os servidores do Windows Update (internet, proxy, DNS ou bloqueio no arquivo hosts)"
	case 0x8024001E:
		return "o serviço Windows Update está sendo encerrado; tente de novo em instantes"
	case 0x80240016:
		return "outra instalação está em andamento ou há um reinício pendente"
	case 0x80240024:
		return "não há atualizações para instalar"
	case 0x80240022:
		return "todas as atualizações falharam"
	case 0x80240017:
		return "a atualização não se aplica mais a este PC"
	case 0x80246007:
		return "a atualização ainda não foi baixada"
	case 0x8024200B:
		return "o instalador do driver falhou"
	case 0x800F020B:
		return "o dispositivo não está conectado agora"
	case 0x80070643:
		return "falha na instalação"
	case 0x80D02002:
		return "o download não avançou (Otimização de Entrega); verifique a conexão"
	case 0x8024A10A, 0x8024A105:
		return "o Windows Update está ocupado ou reiniciando o serviço; tente de novo"
	}
	return ""
}

// WUError formats an HRESULT with its explanation.
func WUError(hr uint32) string {
	if t := WUErrorText(hr); t != "" {
		return fmt.Sprintf("%s (0x%08X)", t, hr)
	}
	return fmt.Sprintf("erro 0x%08X do Windows Update", hr)
}

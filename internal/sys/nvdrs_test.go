package sys

import (
	"errors"
	"strings"
	"testing"
)

func TestNVDRSTexts(t *testing.T) {
	var ids []uint32
	ids = append(ids, NVDRSReadIDs...)
	ids = append(ids, NVDRSOptimizerIDs...)
	for _, id := range ids {
		if strings.HasPrefix(NVDRSName(id), "ajuste 0x") {
			t.Errorf("0x%08X sem nome em português", id)
		}
		def, ok := NVDRSDefaults[id]
		if !ok {
			t.Errorf("0x%08X sem valor padrão em NVDRSDefaults", id)
			continue
		}
		if txt := NVDRSValueText(id, def); strings.Contains(txt, "(0x") {
			t.Errorf("%s: o valor padrão aparece como número (%s)", NVDRSName(id), txt)
		}
	}
	for _, id := range NVDRSOptimizerIDs {
		if txt := NVDRSValueText(id, NVDRSDefaults[id]); strings.Contains(txt, "(0x") {
			t.Errorf("resíduo %s: texto numérico para o padrão: %s", NVDRSName(id), txt)
		}
	}
	if NVVSyncKnown(0x60925619) {
		t.Error("0x60925619 (o 'reverter' de otimizadores) não existe no driver")
	}
	if !NVVSyncKnown(NVVSyncPassive) {
		t.Error("NVVSyncPassive é um valor do driver")
	}
	if NVVSyncForcedOn(NVVSyncForceOff) {
		t.Error("desligado forçado não é 'ligado'")
	}
	if !NVVSyncForcedOn(NVVSyncFlip2) {
		t.Error("1/2 da taxa força o V-Sync ligado")
	}
	if txt := NVDRSValueText(NVDRSVSyncMode, 0x60925619); !strings.Contains(txt, "fora da lista") {
		t.Errorf("valor desconhecido de V-Sync: %s", txt)
	}
	if txt := NVDRSValueText(NVDRSTextureQuality, 0xFFFFFFF6); txt != "Alta qualidade" {
		t.Errorf("filtragem alta qualidade: %s", txt)
	}
	if txt := NVDRSValueText(NVDRSPrerenderLimit, 1); txt != "1 quadro" {
		t.Errorf("pré-renderizados = 1: %s", txt)
	}
	if !errors.Is(&NVAPIError{Func: "x", Status: NVAPIDeviceNotFound}, ErrNoNVAPI) {
		t.Error("NVAPI_NVIDIA_DEVICE_NOT_FOUND deveria ser ErrNoNVAPI")
	}
	if errors.Is(&NVAPIError{Status: NVAPIInvalidUserPrivilege}, ErrNoNVAPI) {
		t.Error("falta de administrador não é 'sem NVAPI'")
	}
	if got := (&NVAPIError{Func: "NvAPI_DRS_SaveSettings", Status: NVAPIInvalidUserPrivilege}).Error(); !strings.Contains(got, "administrador") || !strings.Contains(got, "-137") {
		t.Errorf("texto do erro: %s", got)
	}
	if NVDRSAppRef(" CS2.exe ") != "app:cs2.exe" {
		t.Errorf("NVDRSAppRef: %q", NVDRSAppRef(" CS2.exe "))
	}
	p := NVDRSProfile{Settings: []NVDRSSetting{{ID: 1, Found: true, Value: 7}}, User: []NVDRSSetting{{ID: 2, Found: true, InProfile: true, Value: 9}}}
	if s, ok := p.Get(2); !ok || s.Value != 9 || !s.User() {
		t.Errorf("Get em User: %+v %v", s, ok)
	}
	if _, ok := p.Get(3); ok {
		t.Error("Get de id ausente")
	}
	if (NVDRSSetting{Found: true, InProfile: true, Predefined: true}).User() {
		t.Error("valor predefinido não é de usuário")
	}
}

// The shader cache size is in MB: the report of a real PC showed
// "4294967295 (padrão do driver: padrão do driver)".
func TestNVDRSShaderCacheSizeText(t *testing.T) {
	for v, want := range map[uint32]string{
		0x4000:     "16 GB",
		0xFFFFFFFF: "ilimitado",
		0:          "desligado",
		0x400:      "1 GB",
		0x80:       "128 MB",
		0x2800:     "10 GB",
	} {
		if got := NVDRSValueText(NVDRSShaderCacheSize, v); got != want {
			t.Errorf("0x%X: %q, esperava %q", v, got, want)
		}
	}
}

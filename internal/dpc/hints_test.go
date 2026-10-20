package dpc_test

import (
	"strings"
	"testing"

	"axion/internal/dpc"
)

func TestHintsPointAtTheRightPlace(t *testing.T) {
	for module, area := range map[string]string{
		"nvlddmkm.sys": "Placa de vídeo", "amdkmdag.sys": "Placa de vídeo", "dxgkrnl.sys": "Placa de vídeo",
		"ndis.sys": "Rede", "tcpip.sys": "Rede", "Netwtw14.sys": "Rede", "rt640x64.sys": "Rede",
		"USBXHCI.SYS": "USB", "usbhub3.sys": "USB",
		"stornvme.sys": "Armazenamento", "storport.sys": "Armazenamento",
		"ACPI.sys": "Firmware (BIOS)", "Wdf01000.sys": "Framework de drivers", "ntoskrnl.exe": "Núcleo do Windows",
		"CorsairVBusDriver.sys": "Driver de terceiros", "": "Driver não identificado",
	} {
		if got := dpc.HintFor(module); got.Area != area || got.Text == "" {
			t.Errorf("%q: %+v, esperado área %q", module, got, area)
		}
	}
	// Every button leads to a page that exists in the interface.
	pages := map[string]bool{"drivers": true, "otimizacoes": true, "perifericos": true, "bios": true}
	for _, m := range []string{"nvlddmkm.sys", "ndis.sys", "USBXHCI.SYS", "stornvme.sys", "ACPI.sys"} {
		if h := dpc.HintFor(m); h.Page != "" && (!pages[h.Page] || h.PageLabel == "") {
			t.Errorf("%s: página %q sem botão ou inexistente", m, h.Page)
		}
	}
	// Advice never promises a result.
	for _, m := range []string{"nvlddmkm.sys", "ndis.sys", "USBXHCI.SYS", "stornvme.sys", "ACPI.sys", "foo.sys"} {
		for _, bad := range []string{"garante", "elimina", "resolve", "sempre"} {
			if strings.Contains(strings.ToLower(dpc.HintFor(m).Text), bad) {
				t.Errorf("%s: o texto promete (%q)", m, bad)
			}
		}
	}
}

package dpc

import "strings"

// Hint says where to look when a driver's routines are slow. It points; it
// does not promise: the measurement shows which driver runs long, not why.
type Hint struct {
	Area      string `json:"area"` // what the driver is for
	Text      string `json:"text"`
	Page      string `json:"page,omitempty"`      // page of the interface that helps
	PageLabel string `json:"pageLabel,omitempty"` // button text
}

type hintRule struct {
	match func(name string) bool
	hint  Hint
}

func has(subs ...string) func(string) bool {
	return func(n string) bool {
		for _, s := range subs {
			if strings.Contains(n, s) {
				return true
			}
		}
		return false
	}
}

var hintRules = []hintRule{
	{has("nvlddmkm", "amdkmdag", "amdkmpfd", "atikmdag", "igdkmd", "dxgkrnl", "dxgmms"), Hint{
		Area:      "Placa de vídeo",
		Text:      "Costuma melhorar com o driver de vídeo atualizado. Desligar sobreposições (overlays) e a gravação em segundo plano também ajuda; veja as opções do jogo.",
		Page:      "drivers",
		PageLabel: "Ver drivers",
	}},
	{has("ndis", "tcpip", "netio", "e1d", "e2f", "rt640", "rtl8", "netwtw", "netwbw", "athw", "mtkwl", "bcmwl", "ndiswan", "wdiwifi", "vwifi", "intelwifi", "killer"), Hint{
		Area:      "Rede",
		Text:      "A placa de rede e o driver dela são o primeiro ponto a conferir: o driver atualizado, a economia de energia da placa e a moderação de interrupções (em Otimizações > Rede). No Wi-Fi, prefira o cabo para testar.",
		Page:      "otimizacoes",
		PageLabel: "Ver opções de rede",
	}},
	{has("usbxhci", "usbhub", "usbport", "ucx01000", "usbccgp", "hidusb", "mouhid", "kbdhid"), Hint{
		Area:      "USB",
		Text:      "Ligue o mouse e o teclado numa porta direto da placa-mãe (as de trás do gabinete), sem hub, e confira a energia dos dispositivos USB em Periféricos.",
		Page:      "perifericos",
		PageLabel: "Ver periféricos",
	}},
	{has("storport", "stornvme", "nvme", "iastor", "amdsata", "amdnvme", "storahci", "disk.sys", "partmgr", "ntfs", "fltmgr", "wdfilter"), Hint{
		Area:      "Armazenamento",
		Text:      "Costuma vir do driver do controlador do SSD ou de um antivírus lendo muitos arquivos. Atualize o driver do disco e confira a economia de energia do PCIe (em Otimizações > Energia).",
		Page:      "otimizacoes",
		PageLabel: "Ver opções de energia",
	}},
	{has("acpi.sys", "pci.sys", "msisadrv"), Hint{
		Area: "Firmware (BIOS)",
		Text: "As rotinas de ACPI e do barramento PCI dependem do firmware. Uma BIOS atualizada é o caminho mais comum; veja o Guia de BIOS.",
		Page: "bios", PageLabel: "Abrir o Guia de BIOS",
	}},
	{has("hdaudbus", "portcls", "hdaudio", "rtkvhd", "rtkio", "ks.sys", "usbaudio", "asio"), Hint{
		Area: "Áudio",
		Text: "Atualize o driver de áudio (o do fabricante da placa-mãe, em vez do genérico) e desligue os efeitos de áudio do Windows e do fabricante.",
	}},
	{has("wdf01000", "wdfldr"), Hint{
		Area: "Framework de drivers",
		Text: "É a base usada por vários drivers; a causa está no driver que roda por cima dela, não nela.",
	}},
	{has("ntoskrnl", "ntkrnl", "hal.dll", "ntkrpamp"), Hint{
		Area: "Núcleo do Windows",
		Text: "O núcleo executa as rotinas de temporizadores e do agendador a pedido de outros componentes; raramente é a causa. Compare com os outros drivers da lista.",
	}},
}

// HintFor returns the advice for a kernel module; the zero Hint when there is
// nothing useful to say.
func HintFor(module string) Hint {
	n := strings.ToLower(module)
	if n == "" {
		return Hint{Area: "Driver não identificado", Text: "O Windows não informou qual driver executou estas rotinas."}
	}
	for _, r := range hintRules {
		if r.match(n) {
			return r.hint
		}
	}
	return Hint{Area: "Driver de terceiros", Text: "Procure o fabricante deste driver: uma versão mais nova costuma corrigir rotinas lentas. Se ele pertence a um programa que você não usa, desinstale o programa."}
}

package app

import (
	"fmt"
	"strings"

	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/tweaks"
)

// BiosItem is one firmware setting with its status on this PC.
type BiosItem struct {
	ID      string          `json:"id"`
	Title   string          `json:"title"`
	Status  string          `json:"status"` // ok | action | info | unknown
	Current string          `json:"current"`
	Why     string          `json:"why"`
	Path    string          `json:"path"`
	Caution string          `json:"caution,omitempty"`
	Sources []tweaks.Source `json:"sources,omitempty"`
}

// BiosGuide is the firmware checklist for the detected board.
type BiosGuide struct {
	Vendor   string     `json:"vendor"`
	Board    string     `json:"board"`
	BIOS     string     `json:"bios"`
	Firmware string     `json:"firmware"`
	Enter    string     `json:"enter"`
	Update   string     `json:"update"`
	Support  string     `json:"support"`
	Items    []BiosItem `json:"items"`
	Note     string     `json:"note"`
}

func boardVendor(m string) string {
	u := strings.ToUpper(m)
	switch {
	case strings.Contains(u, "GIGABYTE"):
		return "Gigabyte"
	case strings.Contains(u, "ASUS"):
		return "ASUS"
	case strings.Contains(u, "MICRO-STAR") || strings.Contains(u, "MSI"):
		return "MSI"
	case strings.Contains(u, "ASROCK"):
		return "ASRock"
	}
	return "Outro"
}

// menu paths: [vendor][item][cpuVendor]; "" key = any CPU.
var biosPaths = map[string]map[string]map[string]string{
	"Gigabyte": {
		"xmp":    {"": "Tweaker → Extreme Memory Profile (X.M.P.) → Profile1 (em placas AM5 a mesma opção lista os perfis EXPO/XMP)"},
		"rebar":  {"": "Settings → IO Ports → Above 4G Decoding: Enabled e Re-Size BAR Support: Auto"},
		"csm":    {"": "Boot → CSM Support: Disabled"},
		"tpm":    {"AMD": "Settings → Miscellaneous → AMD CPU fTPM: Enabled", "Intel": "Settings → Miscellaneous → Intel Platform Trust Technology (PTT): Enabled"},
		"sb":     {"": "Boot → Secure Boot → Secure Boot: Enabled (se pedir chaves: Restore Factory Keys)"},
		"virt":   {"AMD": "Tweaker → Advanced CPU Settings → SVM Mode: Enabled; Settings → Miscellaneous → IOMMU: Enabled", "Intel": "Tweaker → Advanced CPU Settings → Intel Virtualization Technology: Enabled; VT-d: Enabled"},
		"update": {"": "Q-Flash (tecla End ou botão Q-Flash na BIOS)"},
		"mcr":    {"": "Tweaker → Advanced Memory Settings → Memory Context Restore (em algumas versões: Settings → AMD CBS → UMC Common Options → DDR Options)"},
	},
	"ASUS": {
		"xmp":    {"AMD": "Ai Tweaker → Ai Overclock Tuner → EXPO I (placas AM4: D.O.C.P.)", "Intel": "Ai Tweaker → Ai Overclock Tuner → XMP I"},
		"rebar":  {"": "Advanced → PCI Subsystem Settings → Above 4G Decoding: Enabled e Re-Size BAR Support: Auto"},
		"csm":    {"": "Boot → CSM (Compatibility Support Module) → Launch CSM: Disabled"},
		"tpm":    {"AMD": "Advanced → AMD fTPM configuration → TPM Device Selection: Firmware TPM", "Intel": "Advanced → PCH-FW Configuration → PTT: Enable"},
		"sb":     {"": "Boot → Secure Boot → OS Type: Windows UEFI mode (Key Management → Install Default Secure Boot keys, se necessário)"},
		"virt":   {"AMD": "Advanced → CPU Configuration → SVM Mode: Enabled; Advanced → AMD CBS → NBIO Common Options → IOMMU: Enabled", "Intel": "Advanced → CPU Configuration → Intel (VMX) Virtualization Technology: Enabled; Advanced → System Agent Configuration → VT-d: Enabled"},
		"update": {"": "EZ Flash (Tool → ASUS EZ Flash Utility)"},
		"mcr":    {"": "Ai Tweaker (ROG: Extreme Tweaker) → DRAM Timing Control → Memory Context Restore"},
	},
	"MSI": {
		"xmp":    {"": "EZ Mode: botão XMP/EXPO (Profile 1) — ou OC → Extreme Memory Profile (XMP/EXPO)"},
		"rebar":  {"": "Settings → Advanced → PCI Subsystem Settings → Above 4G memory/Crypto Currency mining: Enabled e Re-Size BAR Support: Enabled"},
		"csm":    {"": "Settings → Advanced → Windows OS Configuration → BIOS UEFI/CSM Mode: UEFI"},
		"tpm":    {"AMD": "Settings → Security → Trusted Computing → Security Device Support: Enable; AMD fTPM switch: AMD CPU fTPM", "Intel": "Settings → Security → Trusted Computing → Security Device Support: Enable; TPM Device Selection: PTT"},
		"sb":     {"": "Settings → Security → Secure Boot → Secure Boot: Enabled"},
		"virt":   {"AMD": "OC → Advanced CPU Configuration → SVM Mode: Enabled; Settings → Advanced → AMD CBS → NBIO Common Options → IOMMU: Enabled", "Intel": "OC → Advanced CPU Configuration → Intel Virtualization Tech: Enabled; Intel VT-D Tech: Enabled"},
		"update": {"": "M-Flash (Settings → M-Flash ou botão na tela principal)"},
	},
	"ASRock": {
		"xmp":    {"": "OC Tweaker → DRAM Configuration → Load XMP Setting / EXPO: Profile 1"},
		"rebar":  {"": "Advanced → Chipset Configuration → Above 4G Decoding: Enabled e C.A.M. (Clever Access Memory)/Re-Size BAR: Enabled"},
		"csm":    {"": "Boot → CSM (Compatibility Support Module): Disabled"},
		"tpm":    {"AMD": "Security → AMD fTPM switch: AMD CPU fTPM", "Intel": "Security → Intel Platform Trust Technology: Enabled"},
		"sb":     {"": "Security → Secure Boot → Secure Boot: Enabled"},
		"virt":   {"AMD": "Advanced → CPU Configuration → SVM Mode: Enabled; Advanced → AMD CBS → NBIO Common Options → IOMMU: Enabled", "Intel": "Advanced → CPU Configuration → Intel Virtualization Technology: Enabled; Advanced → Chipset Configuration → VT-d: Enabled"},
		"update": {"": "Instant Flash (Tool → Instant Flash)"},
	},
}

var genericPaths = map[string]string{
	"xmp":    "Procure por XMP, EXPO, D.O.C.P. ou \"Memory Profile\" na seção de memória/overclock",
	"rebar":  "Procure por \"Above 4G Decoding\" e \"Re-Size BAR\" (ou Smart Access Memory) nas opções de PCI/PCIe",
	"csm":    "Procure por \"CSM\" ou \"Boot Mode\" e escolha UEFI",
	"tpm":    "Procure por fTPM (AMD) ou PTT (Intel) na seção de segurança",
	"sb":     "Procure por \"Secure Boot\" na seção de boot/segurança",
	"virt":   "Procure por SVM/AMD-V (AMD) ou VT-x/VMX (Intel) e por IOMMU/VT-d",
	"update": "Use a ferramenta de atualização da própria BIOS ou o site da fabricante",
	"mcr":    "Procure por \"Memory Context Restore\" nas opções de memória (em muitas placas: AMD CBS → UMC Common Options → DDR Options)",
}

var supportPages = map[string]string{
	"Gigabyte": "https://www.gigabyte.com/Support",
	"ASUS":     "https://www.asus.com/support/download-center/",
	"MSI":      "https://www.msi.com/support/download",
	"ASRock":   "https://www.asrock.com/support/index.asp",
}

var (
	srcReBARNvidia = tweaks.Source{Title: "NVIDIA — Resizable BAR: requisitos e ativação", URL: "https://www.nvidia.com/en-us/geforce/news/geforce-rtx-30-series-resizable-bar-support/"}
	srcMBR2GPT     = tweaks.Source{Title: "Microsoft Learn — MBR2GPT (converter o disco para UEFI sem formatar)", URL: "https://learn.microsoft.com/windows/deployment/mbr-to-gpt"}
	srcTPMWin      = tweaks.Source{Title: "Microsoft Support — Ativar o TPM 2.0 no seu PC", URL: "https://support.microsoft.com/windows/enable-tpm-2-0-on-your-pc-1fd5a332-360d-4f46-a1e7-ae6b0c90645c"}
	srcSecureBoot  = tweaks.Source{Title: "Microsoft Learn — Secure Boot", URL: "https://learn.microsoft.com/windows-hardware/design/device-experiences/oem-secure-boot"}
	srcAMDEXPO     = tweaks.Source{Title: "AMD — EXPO (perfis de memória para Ryzen)", URL: "https://www.amd.com/en/products/processors/technologies/expo.html"}
	srcIntelXMP    = tweaks.Source{Title: "Intel — Extreme Memory Profile (XMP)", URL: "https://www.intel.com/content/www/us/en/gaming/extreme-memory-profile-xmp.html"}
	srcAM5Training = tweaks.Source{Title: "ASUS ROG (moderador da ASUS) — Treino de memória no AM5 (Q-Code 15) e Memory Context Restore", URL: "https://rog-forum.asus.com/t5/amd-800-series/understanding-memory-training-on-amd-am5-motherboards-q-code-15/td-p/1099400"}
)

func pathFor(vendor, item, cpu string) string {
	if v, ok := biosPaths[vendor]; ok {
		if m, ok := v[item]; ok {
			if p, ok := m[cpu]; ok {
				return p
			}
			if p, ok := m[""]; ok {
				return p
			}
		}
	}
	return genericPaths[item]
}

// BiosGuide builds the firmware checklist from the scan.
func (a *App) BiosGuide() (BiosGuide, error) {
	f := a.Facts()
	if f == nil {
		return BiosGuide{}, ErrNoScan
	}
	return buildBiosGuide(f), nil
}

func buildBiosGuide(f *facts.Facts) BiosGuide {
	v := boardVendor(f.Board.Manufacturer)
	cpu := f.CPU.Vendor
	g := BiosGuide{Vendor: v, Board: strings.TrimSpace(f.Board.Manufacturer + " " + f.Board.Product), Firmware: f.OS.Firmware,
		BIOS: strings.TrimSpace(f.Board.BIOSVersion + " (" + dateOr(f.Board.BIOSDate) + ")"), Support: supportPages[v],
		Enter:  "Reinicie e pressione Delete (ou F2) repetidamente na tela inicial. No Windows: Configurações → Sistema → Recuperação → Inicialização avançada → Solução de problemas → Configurações de firmware UEFI.",
		Update: pathFor(v, "update", cpu),
		Note:   "Os caminhos são os comuns nas BIOS atuais de cada fabricante e podem mudar entre modelos e versões. Salve com F10. Se algo der errado, \"Load Optimized Defaults\" volta ao padrão."}
	if v == "Outro" {
		g.Note = "Placa de fabricante de PC (OEM) ou marca não reconhecida: muitas opções podem não existir. " + g.Note
	}
	legacy := strings.EqualFold(f.OS.Firmware, "BIOS")

	// Memory profile.
	rated := 0
	for _, m := range f.RAM.Modules {
		if r := diag.RatedSpeed(m.PartNumber); r > rated {
			rated = r
		}
	}
	it := BiosItem{ID: "xmp", Title: "Perfil de memória (XMP/EXPO)", Path: pathFor(v, "xmp", cpu),
		Why: "Sem o perfil, a memória roda na velocidade padrão JEDEC. Com ele, roda na velocidade anunciada do kit, o que melhora FPS médio e 1% low em jogos limitados pela CPU.",
		Caution: "O primeiro boot depois de ativar treina a memória de novo e pode levar de dezenas de segundos a alguns minutos com a tela preta: é normal, não desligue. " +
			"Depois, teste a estabilidade (ex.: TestMem5 ou OCCT memória). Se não ligar, a placa volta sozinha ao padrão após algumas tentativas (ou limpe a CMOS). " +
			"Se o perfil não funcionar: atualize a BIOS e tente a velocidade do kit (ou um degrau abaixo) manualmente, com as latências e a tensão da etiqueta, deixando o resto em Auto. Se nada estabilizar, fique na velocidade padrão: é seguro.",
		Sources: []tweaks.Source{srcAMDEXPO, srcIntelXMP}}
	if f.CPU.IsAM5() {
		it.Caution = strings.Replace(it.Caution, "pode levar de dezenas de segundos a alguns minutos com a tela preta", "no AM5 pode levar de 5 a 15 minutos com a tela preta, conforme o kit", 1)
		it.Sources = append(it.Sources, srcAM5Training)
	}
	if f.CPU.Vendor == "AMD" {
		it.Caution += " A AMD trata o EXPO como overclock: a página oficial diz que operar fora das especificações anula a garantia do processador pela AMD."
	}
	cur := f.RAM.ConfiguredMTs
	switch {
	case cur == 0:
		it.Status, it.Current = "unknown", "velocidade não informada pelo Windows"
	case diag.NoMemoryProfile(f) && f.Power.IsLaptop:
		it.Status, it.Current = "info", fmt.Sprintf("%d MT/s (notebooks raramente têm perfil de memória)", cur)
	case diag.NoMemoryProfile(f):
		it.Status, it.Current = "info", fmt.Sprintf("%d MT/s: o chipset desta placa-mãe não permite memória acima da velocidade padrão", cur)
	case rated > 0 && rated-cur >= 200:
		it.Status, it.Current = "action", fmt.Sprintf("rodando a %d MT/s; o kit é de %d MT/s", cur, rated)
	case rated > 0:
		it.Status, it.Current = "ok", fmt.Sprintf("%d MT/s (kit de %d MT/s)", cur, rated)
	case f.Power.IsLaptop:
		it.Status, it.Current = "info", fmt.Sprintf("%d MT/s (notebooks raramente têm perfil de memória)", cur)
	case f.RAM.Type == "DDR5" && cur <= 4800 || f.RAM.Type == "DDR4" && cur <= 2666:
		it.Status, it.Current = "info", fmt.Sprintf("%d MT/s: velocidade padrão JEDEC; confira na etiqueta se o kit tem perfil mais rápido", cur)
	default:
		it.Status, it.Current = "ok", fmt.Sprintf("%d MT/s", cur)
	}
	g.Items = append(g.Items, it)

	// Resizable BAR.
	it = BiosItem{ID: "rebar", Title: "Resizable BAR (Above 4G + Re-Size BAR)", Path: pathFor(v, "rebar", cpu),
		Why:     "Permite que a CPU acesse toda a memória da placa de vídeo de uma vez. Ganho varia por jogo; a NVIDIA só ativa em jogos homologados e a AMD chama de Smart Access Memory.",
		Caution: "Exige o CSM desligado (boot UEFI).",
		Sources: []tweaks.Source{srcReBARNvidia}}
	switch {
	case f.Nvidia.ReBAR == "on":
		it.Status, it.Current = "ok", fmt.Sprintf("ativo (BAR1 = %d MiB)", f.Nvidia.BAR1MiB)
	case f.Nvidia.ReBAR == "off":
		it.Status, it.Current = "action", fmt.Sprintf("desligado (BAR1 = %d MiB)", f.Nvidia.BAR1MiB)
	case f.Nvidia.ReBAR == "unsupported":
		it.Status, it.Current = "info", "esta placa de vídeo não tem Resizable BAR (a NVIDIA oferece a partir da série RTX 30)"
	case f.HasGPUVendor("AMD"):
		it.Status, it.Current = "unknown", "confira no AMD Software: Desempenho → Smart Access Memory"
	default:
		it.Status, it.Current = "unknown", "não foi possível ler (requer GPU NVIDIA com driver instalado)"
	}
	g.Items = append(g.Items, it)

	// CSM / UEFI.
	it = BiosItem{ID: "csm", Title: "Modo de boot UEFI (CSM desligado)", Path: pathFor(v, "csm", cpu),
		Why:     "Necessário para Secure Boot, Resizable BAR e para os requisitos do Windows 11 e de anti-cheats.",
		Sources: []tweaks.Source{srcMBR2GPT}}
	switch {
	case legacy:
		it.Status, it.Current = "action", "o Windows está iniciando em modo BIOS legado (CSM)"
		it.Caution = "NÃO desligue o CSM antes de converter o disco para GPT: o Windows deixaria de iniciar. Use o MBR2GPT da Microsoft (ou reinstale em UEFI) e só depois troque na BIOS."
	case strings.EqualFold(f.OS.Firmware, "UEFI"):
		it.Status, it.Current = "ok", "o Windows inicia em modo UEFI"
	default:
		it.Status, it.Current = "unknown", "modo de firmware não identificado"
	}
	g.Items = append(g.Items, it)

	// TPM.
	it = BiosItem{ID: "tpm", Title: "TPM 2.0 (fTPM/PTT)", Path: pathFor(v, "tpm", cpu),
		Why:     "Exigido pelo Windows 11 e por anti-cheats como FACEIT e Vanguard.",
		Sources: []tweaks.Source{srcTPMWin}}
	switch {
	case f.Security.TPMReady:
		it.Status, it.Current = "ok", "TPM "+f.Security.TPMVersion+" ativo"
	case f.Security.TPMPresent:
		it.Status, it.Current = "action", "TPM presente, mas não ativado"
	default:
		it.Status, it.Current = "action", "TPM não encontrado"
	}
	g.Items = append(g.Items, it)

	// Secure Boot.
	it = BiosItem{ID: "sb", Title: "Secure Boot", Path: pathFor(v, "sb", cpu),
		Why:     "Impede que código não assinado carregue antes do Windows. Exigido por FACEIT, Vanguard e EA Javelin.",
		Sources: []tweaks.Source{srcSecureBoot}}
	switch f.Security.SecureBoot {
	case "on":
		it.Status, it.Current = "ok", "ligado"
	case "off":
		it.Status, it.Current = "action", "desligado"
	default:
		it.Status, it.Current = "unknown", "não suportado neste modo de boot"
		if legacy {
			it.Caution = "Requer boot UEFI (veja o item do CSM)."
		}
	}
	g.Items = append(g.Items, it)

	// Virtualization.
	it = BiosItem{ID: "virt", Title: "Virtualização (SVM/VT-x) e IOMMU/VT-d", Path: pathFor(v, "virt", cpu),
		Why:     "Base do VBS/Integridade de Memória. O FACEIT está exigindo VBS e IOMMU em etapas; deixe ligado se você joga FACEIT ou Valorant.",
		Sources: []tweaks.Source{srcFaceit}}
	switch {
	case (f.Security.VirtualizationFW || f.Security.HypervisorPresent) && f.Security.IOMMU == "off":
		// The IOMMU is read from Windows' DMA protection (Win32_DeviceGuard).
		it.Status, it.Current = "info", "virtualização ativa; IOMMU/VT-d aparentemente desligado (o Windows informa proteção de DMA indisponível)"
		if f.RequiresVBS() {
			it.Status = "action"
		}
	case f.Security.VirtualizationFW || f.Security.HypervisorPresent:
		it.Status, it.Current = "ok", "virtualização ativa no firmware"
		if f.Security.IOMMU == "on" {
			it.Current += "; IOMMU ligado"
		}
	default:
		it.Status, it.Current = "info", "virtualização desligada no firmware"
		if f.RequiresVBS() {
			it.Status = "action"
		}
	}
	g.Items = append(g.Items, it)

	// Firmware update.
	it = BiosItem{ID: "update", Title: "Versão da BIOS", Path: g.Update,
		Why:     "Atualizações trazem correções de estabilidade, compatibilidade de memória e microcode da CPU. Atualize pela ferramenta da própria BIOS com o arquivo do site oficial, sem desligar o PC durante o processo.",
		Current: g.BIOS, Status: "info"}
	if f.CPU.RaptorDie() && f.CPU.Microcode < 0x12B {
		it.Status = "action"
		it.Current += fmt.Sprintf(" — microcode 0x%X: a Intel recomenda 0x12B ou mais novo (via BIOS)", f.CPU.Microcode)
	}
	if f.CPU.IsX3D && f.CPU.IsAM5() && !f.Board.BIOSDate.IsZero() && f.Board.BIOSDate.Year() < 2023 {
		it.Status = "action"
	}
	g.Items = append(g.Items, it)

	// AM5 memory training.
	if f.CPU.IsAM5() {
		g.Items = append(g.Items, mcrItem(f, pathFor(v, "mcr", cpu)))
	}
	return g
}

var srcFaceit = tweaks.Source{Title: "FACEIT — TPM 2.0, Secure Boot, IOMMU e VBS", URL: "https://www.faceit.com/en/news/faceit-rollout-of-tpm-secure-boot-iommu-and-vbs"}

// mcrLongPOST: on AM5 a firmware time this long usually means the memory is
// trained again at every boot (Memory Context Restore off). A heuristic:
// Fast Boot, RAID/NVMe option ROMs and USB devices also add time.
const mcrLongPOST = 25000

func mcrItem(f *facts.Facts, path string) BiosItem {
	it := BiosItem{ID: "mcr", Title: "Memory Context Restore (AM5)", Path: path, Sources: []tweaks.Source{srcAM5Training},
		Why: "O AM5 treina a memória no boot; sem o Memory Context Restore parte desse treino se repete a cada inicialização. Ligar reaproveita o último treino e encurta o boot. " +
			"Só muda o tempo de inicialização: não altera FPS nem latência em jogo.",
		Caution: "Ligue só depois de o perfil EXPO/XMP estar estável (o primeiro treino precisa ser completo) e desligue antes de trocar pentes ou mexer na memória, para forçar um treino novo. " +
			"Se aparecerem erros ou falhas de boot, desligue."}
	ms := f.Boot.FwPOSTms
	switch {
	case ms <= 0:
		it.Status, it.Current = "info", "o firmware não informou o tempo de POST ao Windows"
	case ms >= mcrLongPOST:
		// Optional: it only shortens the boot, so it is never an "ajustar".
		it.Status = "info"
		it.Current = fmt.Sprintf("último POST: %s s (\"Último tempo do BIOS\" no Gerenciador de Tarefas) — longo para AM5; o treino da memória provavelmente está se repetindo a cada boot", secs1(ms))
	default:
		it.Status = "ok"
		it.Current = fmt.Sprintf("último POST: %s s (\"Último tempo do BIOS\" no Gerenciador de Tarefas) — o treino não parece estar se repetindo", secs1(ms))
	}
	return it
}

func secs1(ms int) string {
	return strings.Replace(fmt.Sprintf("%.1f", float64(ms)/1000), ".", ",", 1)
}

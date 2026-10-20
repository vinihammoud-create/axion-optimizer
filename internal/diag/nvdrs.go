package diag

import (
	"fmt"
	"strings"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// Findings from the NVIDIA driver profile database (facts.Nvidia.DRS, read
// by NVAPI without changing anything). They only look at USER values: the
// predefined values of game profiles can come encrypted from the driver.
var (
	srcNvSettingsHDiag = tweaks.Source{Title: "NVIDIA — NVAPI SDK, NvApiDriverSettings.h: IDs e valores padrão dos ajustes do driver (V-Sync padrão = usar a configuração do aplicativo; cache de shader padrão = ligado; modo de energia padrão = ideal)", URL: "https://github.com/NVIDIA/nvapi/blob/main/NvApiDriverSettings.h"}
	srcBBGsyncDiag     = tweaks.Source{Title: "Blur Busters — G-SYNC 101 (p. 14): configurações ótimas (V-Sync no painel, limite ≥ 3 FPS abaixo da taxa, Reflex limita sozinho)", URL: "https://blurbusters.com/gsync/gsync101-input-lag-tests-and-settings/14/"}
	srcBBGsync4Diag    = tweaks.Source{Title: "Blur Busters — G-SYNC 101 (p. 4): o V-Sync tradicional acrescenta atraso de entrada (por exemplo, de 2 a 6 quadros no buffer duplo); o G-SYNC não", URL: "https://blurbusters.com/gsync/gsync101-input-lag-tests-and-settings/4/"}
	srcBBGsync9Diag    = tweaks.Source{Title: "Blur Busters — G-SYNC 101 (p. 9): para jogadores competitivos, V-SYNC desligado é o de menor atraso de entrada (com cortes na imagem)", URL: "https://blurbusters.com/gsync/gsync101-input-lag-tests-and-settings/9/"}
	srcShaderBlogDiag  = tweaks.Source{Title: "Microsoft DirectX Blog — Advanced Shader Delivery: compilar shaders de novo causa carregamento longo e travadas", URL: "https://devblogs.microsoft.com/directx/introducing-advanced-shader-delivery/"}
)

// nvResidueMin is how many optimizer settings out of the driver default
// make the residue finding (criterio do Axion).
const nvResidueMin = 3

func nvdrsFindings(f *facts.Facts, add func(Finding)) {
	d := f.Nvidia.DRS
	if !f.HasGPUVendor("NVIDIA") || !d.Read {
		return
	}
	capable, known := f.NvidiaVRR()
	nvVSyncFinding(f, capable, known, add)
	nvFRLFindings(f, capable, known, add)
	nvShaderFinding(f, add)
	nvResidueFinding(f, add)
	if capable {
		nvVRRFinding(f, add)
	}
}

// nvdrsLabel names a profile for the user (the diag does not import the
// catalog, which has its own label).
func nvdrsLabel(ref string, d facts.NvDRS) string {
	if p := d.Profile(ref); p != nil && p.Name != "" {
		if ref == sys.NVDRSBase || ref == sys.NVDRSGlobal {
			return "perfil global (" + p.Name + ")"
		}
		return "perfil " + p.Name
	}
	return ref
}

func nvVSyncFinding(f *facts.Facts, capable, known bool, add func(Finding)) {
	d := f.Nvidia.DRS
	var forced, invalid []string
	for _, ref := range append(d.AppRefs(), d.GlobalRefs()...) {
		v, ok := d.UserValue(ref, sys.NVDRSVSyncMode)
		if !ok {
			continue
		}
		item := fmt.Sprintf("%s: %s", nvdrsLabel(ref, d), sys.NVDRSValueText(sys.NVDRSVSyncMode, v))
		switch {
		case !sys.NVVSyncKnown(v):
			invalid = append(invalid, item)
		case sys.NVVSyncForcedOn(v):
			forced = append(forced, item)
		}
	}
	act := []Action{fixTweaks("Voltar o V-Sync ao padrão do driver", "fix.nvdrs.vsync")}
	src := []tweaks.Source{srcBBGsync4Diag, srcBBGsync9Diag, srcBBGsyncDiag, srcNvSettingsHDiag}
	switch {
	case len(invalid) > 0:
		add(Finding{ID: "gpu.nvdrs.vsync", Area: AreaLatencia, Severity: SevWarning, Title: "V-Sync do driver NVIDIA com um valor que o driver não conhece",
			Evidence: strings.Join(invalid, "; ") + " (valor de usuário).",
			Explanation: "O valor gravado não está na lista de valores do driver (NvApiDriverSettings.h). É o que a opção de reverter de alguns otimizadores deixa no perfil global, e o efeito de um valor desconhecido não é documentado. " +
				"Voltar ao padrão do driver (\"Usar a configuração do aplicativo 3D\") resolve sem mudar nada que o driver conheça.",
			Actions: act, Sources: src})
	case len(forced) > 0 && known && !capable:
		add(Finding{ID: "gpu.nvdrs.vsync", Area: AreaLatencia, Severity: SevWarning, Title: "V-Sync forçado no driver NVIDIA sem G-SYNC/VRR ativo",
			Evidence: strings.Join(forced, "; ") + ". O driver NVIDIA não informa VRR disponível neste monitor (sem suporte ou G-SYNC desligado no Painel NVIDIA).",
			Explanation: "Sem VRR, com o V-Sync ligado cada quadro espera o próximo ciclo do monitor, e a fila cresce quando o FPS passa da taxa. Segundo a Blur Busters, o V-Sync tradicional acrescenta atraso de entrada (G-SYNC 101, p. 4) e, para jogadores competitivos, V-Sync desligado é o de menor atraso (p. 9). " +
				"Voltar ao padrão do driver devolve a escolha ao jogo; no CS2, deixe \"Esperar pela sincronização vertical\" desligado. Se você prefere a imagem sem cortes (tearing) a menor atraso, pode manter.",
			Actions: act, Sources: src})
	case len(forced) > 0 && !known:
		add(Finding{ID: "gpu.nvdrs.vsync", Area: AreaLatencia, Severity: SevInfo, Title: "V-Sync forçado no driver NVIDIA",
			Evidence:    strings.Join(forced, "; ") + ". Não foi possível saber pelo driver se o monitor tem VRR.",
			Explanation: "Sem G-SYNC/FreeSync, o V-Sync forçado acrescenta atraso de entrada (Blur Busters, p. 4 e 9); com G-SYNC, V-Sync ligado no painel é a configuração que a Blur Busters recomenda (p. 14). Confira se o seu monitor tem VRR antes de mudar.",
			Actions:     act, Sources: src})
	}
}

func nvFRLFindings(f *facts.Facts, capable, known bool, add func(Finding)) {
	d, hz := f.Nvidia.DRS, f.PrimaryHz()
	if hz <= 0 {
		return
	}
	rates := fmt.Sprintf("monitor principal a %d Hz", hz)
	for _, disp := range f.Displays {
		if disp.Primary && disp.MaxHzAtRes > 0 && disp.MaxHzAtRes != hz {
			rates += fmt.Sprintf(" (suporta %d Hz nesta resolução)", disp.MaxHzAtRes)
		}
	}
	for _, ref := range d.AppRefs() {
		v, from, user := d.Effective(ref, sys.NVDRSFrameRateLimit)
		if v == 0 {
			continue
		}
		exe := strings.TrimPrefix(ref, sys.NVDRSAppPrefix)
		where := "no perfil do jogo"
		if from != ref {
			where = "no perfil global"
		}
		ev := fmt.Sprintf("Taxa de quadros máxima = %d FPS %s; %s.", v, where, rates)
		var act []Action
		if user && facts.NVFRLFixable(v, hz, capable, known) {
			act = []Action{fixTweaks("Tirar o limite do driver", "fix.nvdrs.frl")}
		}
		id := "gpu.nvdrs.frl." + exe
		src := []tweaks.Source{srcBBGsyncDiag}
		switch {
		case !known && int(v) < hz:
			add(Finding{ID: id, Area: AreaLatencia, Severity: SevInfo, Title: fmt.Sprintf("Limite de FPS do driver abaixo da taxa do monitor em %s", exe), Evidence: ev + " Não foi possível saber pelo driver se o monitor tem VRR.",
				Explanation: "Com G-SYNC, a Blur Busters recomenda um limite pelo menos 3 FPS abaixo da taxa (ou o Reflex, que limita sozinho); sem VRR, um limite abaixo da taxa tira a margem de quadros mais novos. Confira se o seu monitor tem VRR antes de mudar.", Sources: src})
		case capable && int(v) >= hz*90/100 && int(v) <= hz-3:
			add(Finding{ID: id, Area: AreaLatencia, Severity: SevGood, Title: fmt.Sprintf("Limite de FPS do driver em %s dentro do recomendado para G-SYNC", exe), Evidence: ev,
				Explanation: "Com G-SYNC, limitar pelo menos 3 FPS abaixo da taxa mantém o monitor no VRR sem o V-Sync assumir (Blur Busters). O Reflex do jogo também limita sozinho.", Sources: src})
		case capable && int(v) > hz-3:
			add(Finding{ID: id, Area: AreaLatencia, Severity: SevInfo, Title: fmt.Sprintf("Limite de FPS do driver em %s perto demais da taxa do monitor", exe), Evidence: ev,
				Explanation: "Muito perto da taxa (ou acima dela), o FPS encosta no teto do G-SYNC e o V-Sync pode assumir. A Blur Busters recomenda ficar pelo menos 3 FPS abaixo (ex.: 237 em 240 Hz): escolha esse limite no Painel NVIDIA ou no jogo. O Axion não muda o valor sozinho.", Sources: src})
		case known && !capable && int(v) < hz:
			add(Finding{ID: id, Area: AreaLatencia, Severity: SevWarning, Title: fmt.Sprintf("Limite de FPS do driver abaixo da taxa do monitor em %s", exe), Evidence: ev + " O driver NVIDIA não informa VRR disponível neste monitor.",
				Explanation: "Sem VRR, mais quadros por segundo que a taxa do monitor ainda deixam a imagem mostrada mais nova, e um limite abaixo da taxa tira essa margem. No CS2, o Reflex já controla a fila sem limite fixo. Se o limite é para reduzir calor ou ruído, pode manter.", Actions: act, Sources: src})
		case capable && int(v) < hz*90/100:
			add(Finding{ID: id, Area: AreaLatencia, Severity: SevInfo, Title: fmt.Sprintf("Limite de FPS do driver bem abaixo da taxa em %s", exe), Evidence: ev,
				Explanation: "O monitor pode mostrar mais quadros do que o limite deixa. Se não foi de propósito (calor, ruído), um limite poucos FPS abaixo da taxa, ou o Reflex, aproveita o monitor.", Actions: act, Sources: src})
		}
	}
}

func nvShaderFinding(f *facts.Facts, add func(Finding)) {
	d := f.Nvidia.DRS
	off := false
	for _, ref := range d.GlobalRefs() {
		if v, ok := d.UserValue(ref, sys.NVDRSShaderCache); ok && v == 0 {
			off = true
		}
	}
	if !off {
		return
	}
	add(Finding{ID: "gpu.nvdrs.shadercache", Area: AreaDesempenho, Severity: SevWarning, Title: "Cache de shader do driver NVIDIA desligado",
		Evidence:    "Cache de shader = desligado no perfil global (valor de usuário; o padrão do driver é ligado).",
		Explanation: "Sem o cache, o driver compila de novo os shaders a cada vez que o jogo abre, o que a Microsoft descreve como carregamentos longos e travadas nas primeiras partidas. O Axion só religa o cache; não apaga nada.",
		Actions:     []Action{fixTweaks("Religar o cache de shader", "fix.nvdrs.shadercache")}, Sources: []tweaks.Source{srcShaderBlogDiag, srcNvSettingsHDiag}})
}

func nvResidueFinding(f *facts.Facts, add func(Finding)) {
	d := f.Nvidia.DRS
	var items []string
	total := 0
	for _, ref := range d.GlobalRefs() {
		p := d.Profile(ref)
		if p == nil {
			continue
		}
		total += len(p.User)
		for _, id := range sys.NVDRSOptimizerIDs {
			if v, ok := d.UserValue(ref, id); ok && v != sys.NVDRSDefaults[id] {
				items = append(items, fmt.Sprintf("%s = %s (padrão do driver: %s)", sys.NVDRSName(id), sys.NVDRSValueText(id, v), sys.NVDRSValueText(id, sys.NVDRSDefaults[id])))
			}
		}
	}
	if len(items) < nvResidueMin {
		return
	}
	add(Finding{ID: "res.nvdrs", Area: AreaDesempenho, Severity: SevInfo, Title: fmt.Sprintf("%d ajustes de otimizador no perfil global do driver NVIDIA", len(items)),
		Evidence: strings.Join(items, "; ") + fmt.Sprintf(". No total, %d ajustes do perfil global têm valor de usuário.", total),
		Explanation: "É o padrão deixado por pacotes de otimização que gravam vários ajustes no perfil global (Base Profile) com o nvidiaProfileInspector (energia máxima, baixa latência Ultra, otimização em threads). " +
			"O Axion não encontrou medição pública de ganho desses valores forçados para todos os programas, e a opção de reverter de alguns desses pacotes não volta ao padrão do driver (grava um V-Sync fora da lista e o modo de energia Adaptável). " +
			"Mudanças que você mesmo fez no Painel de Controle da NVIDIA também aparecem aqui: confira a lista antes de restaurar. O limite de 3 itens é critério do Axion.",
		Actions: []Action{fixTweaks("Restaurar o padrão do driver nesses ajustes", "fix.nvdrs.residue")}, Sources: []tweaks.Source{srcNvSettingsHDiag}})
}

func nvVRRFinding(f *facts.Facts, add func(Finding)) {
	d := f.Nvidia.DRS
	refs := d.GlobalRefs()
	g := refs[len(refs)-1]
	vs, _, _ := d.Effective(g, sys.NVDRSVSyncMode)
	frl, _, _ := d.Effective(g, sys.NVDRSFrameRateLimit)
	add(Finding{ID: "gpu.nvdrs.vrr", Area: AreaLatencia, Severity: SevGood, Title: "Monitor com G-SYNC/VRR: duas configurações válidas",
		Evidence: fmt.Sprintf("O driver informa VRR %s. No perfil global: V-Sync = %s; limite de FPS = %s.", nvVRRText(d.VRR), sys.NVDRSValueText(sys.NVDRSVSyncMode, vs), sys.NVDRSValueText(sys.NVDRSFrameRateLimit, frl)),
		Explanation: "Mais fluidez, sem cortes: G-SYNC ligado, V-Sync ligado no Painel NVIDIA (desligado no jogo) e Reflex ligado no jogo, que já limita o FPS abaixo da taxa; sem Reflex, use um limite pelo menos 3 FPS abaixo da taxa (ex.: 237 em 240 Hz). " +
			"Menor atraso possível: sem limite e com V-Sync desligado, aceitando cortes na imagem acima da taxa do monitor. As duas são válidas, e o Axion não muda nenhuma sozinho.",
		Sources: []tweaks.Source{srcBBGsyncDiag}})
}

// nvVRRText: "ligado" when some monitor has VRR on, else "possível, mas
// desligado no Painel NVIDIA", plus the devices.
func nvVRRText(vrr []sys.NVVRRInfo) string {
	var devs []string
	on := false
	for _, v := range vrr {
		if !v.Read || !(v.Possible || v.Enabled) {
			continue
		}
		devs = append(devs, v.Device)
		if v.Enabled {
			on = true
		}
	}
	state := "possível, mas desligado no Painel NVIDIA"
	if on {
		state = "ligado"
	}
	if len(devs) > 0 {
		state += " (" + strings.Join(devs, ", ") + ")"
	}
	return state
}

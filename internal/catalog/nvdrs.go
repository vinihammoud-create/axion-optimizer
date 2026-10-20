package catalog

import (
	"encoding/json"
	"fmt"
	"strings"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// NVIDIA driver profiles (NVAPI DRS, 0.9). One operation per setting and
// per profile: the value before (user, predefined or absent) goes to the
// journal before the write, the write is confirmed by reading back, and
// undo puts the exact previous state back (SetSetting of the old user
// value, RestoreProfileDefaultSetting, or DeleteProfileSetting when the
// setting was not in the profile). Everything is the driver's own public
// API, the one the NVIDIA Control Panel uses; no game is touched.
//
// Seeing the state, applying and undoing call the NVAPI in the main Axion
// process (the collection runs it in the child process). That is why every
// item here requires f.Nvidia.DRS.Read first: when the collection died
// inside the NVAPI, the app does not call it again.
var (
	srcNvDRSAPI    = tweaks.Source{Title: "NVIDIA — NVAPI SDK, nvapi.h: API de perfis do driver (DRS): GetSetting, SetSetting, DeleteProfileSetting, RestoreProfileDefaultSetting, SaveSettings", URL: "https://github.com/NVIDIA/nvapi/blob/main/nvapi.h"}
	srcBBGsyncCat  = tweaks.Source{Title: "Blur Busters — G-SYNC 101 (p. 14): configurações ótimas (V-Sync no painel, limite ≥ 3 FPS abaixo da taxa, Reflex limita sozinho)", URL: "https://blurbusters.com/gsync/gsync101-input-lag-tests-and-settings/14/"}
	srcBBGsync4Cat = tweaks.Source{Title: "Blur Busters — G-SYNC 101 (p. 4): o V-Sync tradicional acrescenta atraso de entrada (por exemplo, de 2 a 6 quadros no buffer duplo); o G-SYNC não", URL: "https://blurbusters.com/gsync/gsync101-input-lag-tests-and-settings/4/"}
	srcBBGsync9Cat = tweaks.Source{Title: "Blur Busters — G-SYNC 101 (p. 9): para jogadores competitivos, V-SYNC desligado é o de menor atraso de entrada (com cortes na imagem)", URL: "https://blurbusters.com/gsync/gsync101-input-lag-tests-and-settings/9/"}
	srcNPINames    = tweaks.Source{Title: "nvidiaProfileInspector — CustomSettingNames.xml (0x10835000 = Ultra Low Latency - Enabled; fora do header público)", URL: "https://github.com/Orbmu2k/nvidiaProfileInspector/blob/master/nvidiaProfileInspector/CustomSettingNames.xml"}
)

// nvdrsSnap is the state of one setting in one profile before a change.
// Name is informative only (not compared).
type nvdrsSnap struct {
	Profile   string `json:"profile"`
	Found     bool   `json:"found"` // the profile exists
	Name      string `json:"name,omitempty"`
	InProfile bool   `json:"inProfile"`
	User      bool   `json:"user"`
	Value     uint32 `json:"value"`
}

func nvdrsProfileLabel(ref string) string {
	switch {
	case ref == sys.NVDRSBase:
		return "Base Profile (global)"
	case ref == sys.NVDRSGlobal:
		return "perfil global atual"
	}
	return "perfil de " + strings.TrimPrefix(ref, sys.NVDRSAppPrefix)
}

// nvdrsRead reads one setting of one profile (one DRS session, no
// enumeration).
func nvdrsRead(c *tweaks.Ctx, ref string, id uint32) (nvdrsSnap, error) {
	profs, err := c.P.Native.NVDRSProfiles([]string{ref}, []uint32{id}, false)
	if err != nil {
		return nvdrsSnap{}, err
	}
	if len(profs) != 1 {
		return nvdrsSnap{}, fmt.Errorf("o driver devolveu %d perfis para %s", len(profs), ref)
	}
	p := profs[0]
	s := nvdrsSnap{Profile: ref, Found: p.Found, Name: p.Name}
	if p.Found && len(p.Settings) == 1 {
		st := p.Settings[0]
		if st.Found && st.Type != sys.NVDRSTypeDWORD {
			return s, fmt.Errorf("%s não é um valor numérico (tipo %d): o Axion não altera esse ajuste", sys.NVDRSName(id), st.Type)
		}
		s.InProfile, s.User, s.Value = st.InProfile, st.User(), st.Value
	}
	return s, nil
}

// nvdrsOp writes a DWORD user value (mode "set") or removes the user value
// so the driver default applies again (mode "default") in one profile.
func nvdrsOp(ref string, id uint32, mode string, want uint32) tweaks.Op {
	ref = strings.ToLower(ref)
	name, where := sys.NVDRSName(id), nvdrsProfileLabel(ref)
	desc := fmt.Sprintf("Driver NVIDIA, %s → %s volta ao padrão do driver (NvAPI_DRS_DeleteProfileSetting + SaveSettings)", where, name)
	if mode == "set" {
		desc = fmt.Sprintf("Driver NVIDIA, %s → %s = %s (NvAPI_DRS_SetSetting + SaveSettings)", where, name, sys.NVDRSValueText(id, want))
	}
	return &tweaks.FuncOp{
		KindName: "nvdrs", ID: fmt.Sprintf("%s|%08x", ref, id), Spec: nvdrsSpec{Profile: ref, ID: id, Mode: mode, Value: want}, Desc: desc,
		SnapFn: func(c *tweaks.Ctx) (any, error) {
			s, err := nvdrsRead(c, ref, id)
			if err == nil && !s.Found {
				err = fmt.Errorf("o %s não existe no driver NVIDIA", where)
			}
			return s, err
		},
		InFn: func(c *tweaks.Ctx) (bool, error) {
			s, err := nvdrsRead(c, ref, id)
			if err != nil {
				return false, err
			}
			if !s.Found {
				return mode == "default", nil
			}
			if mode == "set" {
				return s.InProfile && s.Value == want, nil
			}
			return !s.User, nil
		},
		ApplyFn: func(c *tweaks.Ctx) error {
			if mode == "set" {
				return c.P.Native.NVDRSSet(ref, id, want)
			}
			return c.P.Native.NVDRSDelete(ref, id)
		},
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var b nvdrsSnap
			if err := json.Unmarshal(raw, &b); err != nil {
				return err
			}
			// "global" is whatever profile the NVIDIA panel has as global now:
			// if the user switched it, the old value belongs to the other one.
			if ref == sys.NVDRSGlobal && b.Name != "" {
				if cur, err := nvdrsRead(c, ref, id); err == nil && cur.Name != "" && !strings.EqualFold(cur.Name, b.Name) {
					return fmt.Errorf("o perfil global do driver NVIDIA mudou (era \"%s\", agora é \"%s\"): volte a selecionar \"%s\" no Painel de Controle da NVIDIA para desfazer", b.Name, cur.Name, b.Name)
				}
			}
			switch {
			case !b.Found:
				return nil
			case !b.InProfile:
				return c.P.Native.NVDRSDelete(ref, id)
			case !b.User:
				return c.P.Native.NVDRSRestoreDefault(ref, id)
			}
			return c.P.Native.NVDRSSet(ref, id, b.Value)
		},
		EqFn: func(a, b json.RawMessage) bool {
			var sa, sb nvdrsSnap
			if json.Unmarshal(a, &sa) != nil || json.Unmarshal(b, &sb) != nil {
				return false
			}
			if ref == sys.NVDRSGlobal && sa.Name != "" && sb.Name != "" && !strings.EqualFold(sa.Name, sb.Name) {
				return false
			}
			return sa.Found == sb.Found && sa.InProfile == sb.InProfile && sa.User == sb.User && (!sa.InProfile || sa.Value == sb.Value)
		},
		CurFn: func(c *tweaks.Ctx) string {
			s, err := nvdrsRead(c, ref, id)
			switch {
			case err != nil:
				return "erro: " + err.Error()
			case !s.Found:
				return "(perfil não existe no driver)"
			case !s.InProfile:
				return "não definido neste perfil (vale o global ou o padrão)"
			case s.User:
				return sys.NVDRSValueText(id, s.Value) + " (valor de usuário)"
			}
			return sys.NVDRSValueText(id, s.Value) + " (padrão do driver)"
		},
	}
}

// nvdrsNotRead is the reason an item is not applicable when the profile
// database was not read (checked BEFORE anything else: see the package
// comment).
func nvdrsNotRead(f *facts.Facts) (bool, string) {
	if !f.HasGPUVendor("NVIDIA") {
		return true, "sem placa de vídeo NVIDIA"
	}
	if !f.Nvidia.DRS.Read {
		why := f.Nvidia.DRS.Error
		if why == "" {
			why = "leitura não feita nesta análise"
		}
		return true, "o perfil do driver NVIDIA não foi lido: " + why
	}
	return false, ""
}

// nvdrsReadable is the common Applicable of the fixes.
func nvdrsReadable(f *facts.Facts, ops func(*facts.Facts) []tweaks.Op) (bool, string) {
	if no, why := nvdrsNotRead(f); no {
		return false, why
	}
	if len(ops(f)) == 0 {
		return false, "nada a corrigir no perfil do driver deste PC"
	}
	return true, ""
}

// nvdrsHarmfulOps: for each ref, the user value of id that bad flags gets
// one "back to the driver default" operation.
func nvdrsHarmfulOps(f *facts.Facts, refs []string, id uint32, bad func(v uint32) bool) []tweaks.Op {
	var out []tweaks.Op
	for _, ref := range refs {
		if v, ok := f.Nvidia.DRS.UserValue(ref, id); ok && bad(v) {
			out = append(out, nvdrsOp(ref, id, "default", 0))
		}
	}
	return out
}

func nvdrsAllRefs(f *facts.Facts) []string {
	d := f.Nvidia.DRS
	return append(d.GlobalRefs(), d.AppRefs()...)
}

func nvdrsFixTweaks() []*tweaks.Tweak {
	vsyncOps := func(f *facts.Facts) []tweaks.Op {
		return nvdrsHarmfulOps(f, nvdrsAllRefs(f), sys.NVDRSVSyncMode, func(v uint32) bool { return !sys.NVVSyncKnown(v) || sys.NVVSyncForcedOn(v) })
	}
	shaderOps := func(f *facts.Facts) []tweaks.Op {
		return nvdrsHarmfulOps(f, f.Nvidia.DRS.GlobalRefs(), sys.NVDRSShaderCache, func(v uint32) bool { return v == 0 })
	}
	frlOps := func(f *facts.Facts) []tweaks.Op {
		hz := f.PrimaryHz()
		capable, known := f.NvidiaVRR()
		return nvdrsHarmfulOps(f, nvdrsAllRefs(f), sys.NVDRSFrameRateLimit, func(v uint32) bool { return facts.NVFRLFixable(v, hz, capable, known) })
	}
	residueOps := func(f *facts.Facts) []tweaks.Op {
		var out []tweaks.Op
		for _, ref := range f.Nvidia.DRS.GlobalRefs() {
			for _, id := range sys.NVDRSOptimizerIDs {
				if v, ok := f.Nvidia.DRS.UserValue(ref, id); ok && v != sys.NVDRSDefaults[id] {
					out = append(out, nvdrsOp(ref, id, "default", 0))
				}
			}
		}
		return out
	}
	return []*tweaks.Tweak{
		{
			ID: "fix.nvdrs.vsync", Category: "correcoes", Title: "V-Sync do driver NVIDIA de volta ao padrão (decisão do jogo)",
			Summary: "Remove o V-Sync forçado (ou com valor inválido) que outro programa deixou no perfil do driver NVIDIA, devolvendo a escolha ao jogo.",
			Detail: "Apaga o valor de usuário de Sincronização vertical (0x00A879CF) nos perfis onde ele força o V-Sync ligado ou tem um valor que não está na lista do driver; o driver volta ao padrão \"Usar a configuração do aplicativo 3D\" (0x60925292, NvApiDriverSettings.h). " +
				"Sem monitor G-SYNC/VRR, o V-Sync faz cada quadro esperar o ciclo do monitor e, segundo a Blur Busters, acrescenta atraso de entrada (G-SYNC 101, p. 4); para jogadores competitivos, V-Sync desligado é o de menor atraso, ao custo de cortes na imagem (p. 9). Com G-SYNC, V-Sync ligado no painel é uma configuração válida: nesse caso o item não é recomendado. " +
				"Vale ao reabrir o jogo; o valor anterior fica no histórico e volta exatamente com Desfazer.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcNvSettingsH, srcBBGsync4Cat, srcBBGsync9Cat, srcBBGsyncCat, srcNvDRSAPI},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestartApp, Tags: []string{"latencia"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) { return nvdrsReadable(f, vsyncOps) },
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				invalid := false
				for _, ref := range nvdrsAllRefs(f) {
					if v, ok := f.Nvidia.DRS.UserValue(ref, sys.NVDRSVSyncMode); ok && !sys.NVVSyncKnown(v) {
						invalid = true
					}
				}
				capable, known := f.NvidiaVRR()
				switch {
				case invalid:
					return tweaks.Rec(tweaks.RecYes, "valor que o driver não conhece, deixado por outro otimizador")
				case known && !capable:
					return tweaks.Rec(tweaks.RecYes, "V-Sync forçado sem G-SYNC/VRR ativo: mais atraso de entrada")
				case capable:
					return tweaks.Rec(tweaks.RecNo, "com G-SYNC, V-Sync ligado no driver é uma das configurações recomendadas")
				}
				return tweaks.Rec(tweaks.RecNeutral, "não foi possível saber se o monitor tem VRR")
			},
			Ops: vsyncOps,
		},
		{
			ID: "fix.nvdrs.shadercache", Category: "correcoes", Title: "Religar o cache de shader do driver NVIDIA",
			Summary: "Volta o cache de shader do driver ao padrão (ligado), para os jogos não compilarem tudo de novo a cada abertura.",
			Detail: "Apaga o valor de usuário 0 (desligado) de Cache de shader (0x00198FFF) no perfil global; o padrão do driver é ligado (NvApiDriverSettings.h). " +
				"Sem o cache, os shaders são compilados de novo a cada vez que o jogo abre, o que causa carregamentos longos e travadas nas primeiras partidas, como a Microsoft descreve. Não apaga nem limpa o cache. Vale ao reabrir o jogo.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcNvSettingsH, srcShaderBlog},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestartApp, Tags: []string{"fps", "estabilidade"}, OneClick: true,
			Applicable: func(f *facts.Facts) (bool, string) { return nvdrsReadable(f, shaderOps) },
			Recommend:  yes("o cache de shader está desligado no perfil global do driver"),
			Ops:        shaderOps,
		},
		{
			ID: "fix.nvdrs.frl", Category: "correcoes", Title: "Tirar o limite de FPS do driver NVIDIA abaixo da taxa do monitor",
			Summary: "Remove o limitador de quadros do driver quando ele fica abaixo da taxa do monitor, devolvendo ao jogo o controle do FPS.",
			Detail: "Apaga o valor de usuário de Taxa de quadros máxima (0x10835002) nos perfis onde ele está acima de 0 e abaixo da taxa atual do monitor principal; o padrão do driver é desligado (0). " +
				"Sem VRR, quadros acima da taxa do monitor ainda deixam a imagem mais nova; com G-SYNC, a Blur Busters recomenda limitar pelo menos 3 FPS abaixo da taxa, ou deixar o Reflex limitar, e por isso um limite poucos FPS abaixo da taxa não entra aqui (só um limite abaixo de 90% da taxa, critério do Axion). " +
				"Se você limitou para reduzir calor ou ruído, mantenha.",
			Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcBBGsyncCat, srcNvSettingsH},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestartApp, Tags: []string{"latencia"}, OneClick: false,
			Applicable: func(f *facts.Facts) (bool, string) {
				if no, why := nvdrsNotRead(f); no {
					return false, why
				}
				if f.PrimaryHz() <= 0 {
					return false, "taxa de atualização do monitor desconhecida"
				}
				return nvdrsReadable(f, frlOps)
			},
			Recommend: func(f *facts.Facts) tweaks.Recommendation {
				if capable, known := f.NvidiaVRR(); known && !capable {
					return tweaks.Rec(tweaks.RecYes, "sem VRR, o limite abaixo da taxa tira a margem de atraso")
				}
				return tweaks.Rec(tweaks.RecNeutral, "com G-SYNC, um limite alguns FPS abaixo da taxa é o recomendado; confira se o limite foi escolha sua")
			},
			Ops: frlOps,
		},
		{
			ID: "fix.nvdrs.residue", Category: "correcoes", Title: "Ajustes de otimizadores no perfil global do driver NVIDIA de volta ao padrão",
			Summary: "Devolve ao padrão do driver os ajustes que pacotes de otimização gravam no perfil global da NVIDIA (modo de energia, filtragem, pré-renderizados, otimização em threads...).",
			Detail: "Apaga, um por um, os valores de usuário do perfil global (Base Profile) nos ajustes que pacotes de otimização gravam com o nvidiaProfileInspector, quando diferentes do padrão do driver: modo de gerenciamento de energia, máximo de quadros pré-renderizados, baixa latência Ultra, taxa de atualização preferida, filtragem de textura, otimização em threads, buffer triplo e tamanho do cache de shader. " +
				"O Axion não encontrou medição pública de ganho desses valores forçados para todos os programas; \"Preferir desempenho máximo\" no global aumenta o consumo em repouso (TheFPSReview). Mudanças que você mesmo fez no Painel NVIDIA também aparecem na lista: confira antes. Cada valor anterior fica no histórico.",
			Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcNvSettingsH, srcFPSReviewPower, srcNPINames},
			Risk: tweaks.RiskSafe, Reboot: tweaks.RebootRestartApp, Tags: []string{"gpu"}, OneClick: false,
			Applicable: func(f *facts.Facts) (bool, string) { return nvdrsReadable(f, residueOps) },
			Recommend:  neutral("confira a lista: alguns podem ter sido escolha sua no Painel NVIDIA"),
			Ops:        residueOps,
		},
	}
}

// nvPStateProfile is the per-game option "Prefer maximum performance",
// written ONLY in the game's own driver profile (never in the global one).
func nvPStateProfile(id, exe string, gameOnly func(*facts.Facts) (bool, string)) *tweaks.Tweak {
	ref := sys.NVDRSAppRef(exe)
	return &tweaks.Tweak{
		ID: id, Category: "jogos", Profile: exe, Title: "Placa NVIDIA em \"Preferir desempenho máximo\" só em " + exe,
		Summary: "Grava o modo de energia \"Preferir desempenho máximo\" apenas no perfil do driver deste jogo, para a placa não baixar o clock com ele aberto.",
		Detail: "PREFERRED_PSTATE (0x1057EB71) = 1 no perfil do driver NVIDIA do jogo (NvAPI_DRS_SetSetting + SaveSettings), nunca no perfil global. " +
			"A medição pública do TheFPSReview (RTX 2080 SUPER e 2060 SUPER) achou a experiência igual nos três modos e consumo em repouso muito maior com o modo máximo: por isso o Axion só oferece por jogo, como opção. " +
			"Pode ajudar quem vê a placa reduzir o clock em jogos leves; meça antes e depois na página Desempenho. Desfazer volta ao valor anterior do perfil do jogo.",
		Evidence: tweaks.EvLow, Sources: []tweaks.Source{srcFPSReviewPower, srcNvSettingsH, srcNvDRSAPI},
		Risk: tweaks.RiskSafe, RiskNote: "mais consumo e calor enquanto o jogo está aberto", Reboot: tweaks.RebootRestartApp, Tags: []string{"fps"}, OneClick: false,
		Applicable: func(f *facts.Facts) (bool, string) {
			if ok, why := gameOnly(f); !ok {
				return false, why
			}
			if no, why := nvdrsNotRead(f); no {
				return false, why
			}
			p := f.Nvidia.DRS.Profile(ref)
			if p == nil {
				return false, "o Axion não leu o perfil do driver deste jogo"
			}
			if !p.Found {
				return false, "o driver NVIDIA não tem um perfil para " + exe
			}
			return true, ""
		},
		Recommend: neutral("a medição pública não viu ganho de FPS; meça no seu jogo"),
		Ops: func(*facts.Facts) []tweaks.Op {
			return []tweaks.Op{nvdrsOp(ref, sys.NVDRSPowerMode, "set", sys.NVPowerPreferMax)}
		},
	}
}

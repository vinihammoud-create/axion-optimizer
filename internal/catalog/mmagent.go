package catalog

import (
	"encoding/json"

	"axion/internal/sys"
	"axion/internal/tweaks"
)

// ---------------------------------------------------------------------------
// Memory compression (memory manager agent, WMI PS_MMAgent)
// ---------------------------------------------------------------------------

var (
	srcMMAgentEnable = tweaks.Source{Title: "Microsoft Learn — Enable-MMAgent (o parâmetro MemoryCompression existe, mas a página não descreve o efeito da compressão)", URL: "https://learn.microsoft.com/powershell/module/mmagent/enable-mmagent"}
	srcMMAgentGet    = tweaks.Source{Title: "Microsoft Learn — Get-MMAgent (lê o estado dos componentes do agente de gerenciamento de memória)", URL: "https://learn.microsoft.com/powershell/module/mmagent/get-mmagent"}
	srcMemCompQA     = tweaks.Source{Title: "Microsoft Q&A (resposta de consultor independente, não é documentação) — desligar a compressão pode aumentar o uso do arquivo de paginação e causar lentidão se a RAM encher; reiniciar após mudar", URL: "https://learn.microsoft.com/answers/a/12168645"}
)

type mmagentSnap struct {
	MemoryCompression bool `json:"memoryCompression"`
}

func memoryCompressionLabel(on bool) string {
	if on {
		return "compressão de memória ligada"
	}
	return "compressão de memória desligada"
}

// memCompressionOp sets the MemoryCompression component of the memory manager
// agent through PS_MMAgent.Enable or Disable (WMI, no PowerShell). Only that
// component is passed, so prefetching, page combining and prelaunch stay as
// they are. The snapshot is the value read before, and the undo writes it back.
func memCompressionOp(want bool) tweaks.Op {
	read := func(c *tweaks.Ctx) (bool, error) {
		st, err := sys.ReadMMAgent(c.P.WMI)
		return st.MemoryCompression, err
	}
	set := func(c *tweaks.Ctx, on bool) error {
		// Skip the write when the agent already reports the value (an undo
		// after the user changed it back by hand must not write again).
		if cur, err := read(c); err == nil && cur == on {
			return nil
		}
		return sys.SetMemoryCompression(c.P.WMI, on)
	}
	desc := "PS_MMAgent.Disable(MemoryCompression) por WMI: desliga a compressão de memória"
	if want {
		desc = "PS_MMAgent.Enable(MemoryCompression) por WMI: religa a compressão de memória (padrão do Windows)"
	}
	return &tweaks.FuncOp{
		KindName: "mmagent", ID: "memorycompression", Desc: desc, Spec: mmagentSpec{Want: want},
		SnapFn: func(c *tweaks.Ctx) (any, error) {
			cur, err := read(c)
			if err != nil {
				return nil, err
			}
			return mmagentSnap{MemoryCompression: cur}, nil
		},
		InFn: func(c *tweaks.Ctx) (bool, error) {
			cur, err := read(c)
			if err != nil {
				return false, err
			}
			return cur == want, nil
		},
		ApplyFn: func(c *tweaks.Ctx) error { return set(c, want) },
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var s mmagentSnap
			if err := json.Unmarshal(raw, &s); err != nil {
				return err
			}
			return set(c, s.MemoryCompression)
		},
		CurFn: func(c *tweaks.Ctx) string {
			cur, err := read(c)
			if err != nil {
				return "erro: " + err.Error()
			}
			return memoryCompressionLabel(cur)
		},
	}
}

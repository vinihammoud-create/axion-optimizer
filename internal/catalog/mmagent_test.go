package catalog_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"axion/internal/collect"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

// readMC reads MemoryCompression from the simulated agent the way the app does.
func readMC(t *testing.T, m *mocksys.Mock) bool {
	t.Helper()
	st, err := sys.ReadMMAgent(m.W)
	if err != nil {
		t.Fatalf("ler o agente: %v", err)
	}
	return st.MemoryCompression
}

// fix.mem.compression: not offered with compression on (the Windows default),
// offered when another program turned it off, applies by WMI touching only the
// MemoryCompression component and undoes to exactly what was there.
func TestMemCompressionRoundTrip(t *testing.T) {
	m, f, e := setup(t)
	tw := e.Catalog.Get("fix.mem.compression")
	if tw == nil {
		t.Fatal("fix.mem.compression não existe")
	}
	if ok, _ := tw.IsApplicable(f); ok {
		t.Fatal("com a compressão ligada (padrão) a correção não deveria se aplicar")
	}
	if tw.OneClick || tw.Reboot != tweaks.RebootRestart || tw.Evidence != tweaks.EvLow {
		t.Errorf("sem confirmação de efeito sem reiniciar: 1 clique=%v, reboot=%v, evidência=%v", tw.OneClick, tw.Reboot, tw.Evidence)
	}

	m.SetMemoryCompression(false)
	// Other components of the agent must keep their values through apply and undo.
	if _, err := m.W.CallOut(sys.MMAgentNamespace, sys.MMAgentClass, "Enable", map[string]any{"PageCombining": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.W.CallOut(sys.MMAgentNamespace, sys.MMAgentClass, "Disable", map[string]any{"ApplicationPreLaunch": true}); err != nil {
		t.Fatal(err)
	}
	f = collect.Collect(context.Background(), m.Platform, nil)
	if ok, why := tw.IsApplicable(f); !ok {
		t.Fatalf("compressão desligada deveria se aplicar: %s", why)
	}
	before := m.World()
	res := e.Apply(context.Background(), f, []string{tw.ID}, tweaks.ApplyOpts{})
	if len(res) != 1 || res[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", res)
	}
	if !readMC(t, m) {
		t.Fatal("depois de aplicar a compressão deveria estar ligada")
	}
	rows, _ := m.W.Query(sys.MMAgentNamespace, "SELECT * FROM MMAgentComponents")
	if len(rows) != 1 || !rows[0].Bool("PageCombining") || rows[0].Bool("ApplicationPreLaunch") || !rows[0].Bool("ApplicationLaunchPrefetching") {
		t.Errorf("a correção mexeu em outro componente do agente: %+v", rows)
	}
	if d := mocksys.DiffWorld(before, m.World(), `hklm\software\axion`); len(d) != 1 || !strings.Contains(d[0], "MemoryCompression=true") {
		t.Errorf("só o MemoryCompression deveria mudar: %v", d)
	}

	f = collect.Collect(context.Background(), m.Platform, nil)
	if st := e.StateOf(context.Background(), f, tw); st.Status != "on" || !st.ByAxion {
		t.Fatalf("estado depois de aplicar: %+v", st)
	}
	rev := e.RevertTweaks(context.Background(), f, []string{tw.ID}, tweaks.RevertOpts{})
	if len(rev) != 1 || rev[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer: %+v", rev)
	}
	if readMC(t, m) {
		t.Error("desfazer deveria voltar a compressão para desligada")
	}
	if d := mocksys.DiffWorld(before, m.World()); len(d) > 0 {
		t.Errorf("desfazer não restaurou o estado exato:\n  %s", strings.Join(d, "\n  "))
	}
}

// If the agent cannot be read (no administrator rights, class missing), nothing
// is offered and a direct apply changes nothing.
func TestMemCompressionUnreadable(t *testing.T) {
	m, _, e := setup(t)
	m.SetMemoryCompression(false)
	m.W.OutHandler = func(ns, path, method string, in map[string]any) (sys.WMIRow, error) {
		return nil, errors.New("Acesso negado (0x80041003)")
	}
	f := collect.Collect(context.Background(), m.Platform, nil)
	if f.Residue.MMAgentRead {
		t.Fatal("leitura negada não pode contar como lida")
	}
	tw := e.Catalog.Get("fix.mem.compression")
	if ok, _ := tw.IsApplicable(f); ok {
		t.Fatal("estado desconhecido nunca é resíduo: a correção não deveria se aplicar")
	}
	// Forced past the applicability check (stale facts): the capture fails
	// before any write, so the journal and the machine stay untouched.
	m.W.OutHandler = nil
	m.SetMemoryCompression(false)
	stale := collect.Collect(context.Background(), m.Platform, nil)
	m.W.OutHandler = func(ns, path, method string, in map[string]any) (sys.WMIRow, error) {
		return nil, errors.New("Acesso negado (0x80041003)")
	}
	res := e.Apply(context.Background(), stale, []string{tw.ID}, tweaks.ApplyOpts{})
	if len(res) != 1 || res[0].Outcome != tweaks.OutFailed {
		t.Fatalf("sem leitura do estado a aplicação deveria falhar: %+v", res)
	}
	m.W.OutHandler = nil
	if readMC(t, m) {
		t.Error("a aplicação que falhou não pode ter ligado a compressão")
	}
}

// If Windows does not confirm the new state when read back, the apply is rolled
// back instead of being reported as done.
func TestMemCompressionNotConfirmed(t *testing.T) {
	m, _, e := setup(t)
	m.SetMemoryCompression(false)
	f := collect.Collect(context.Background(), m.Platform, nil)
	// Enable answers success but the state does not change (a restart is needed).
	m.W.OutHandler = func(ns, path, method string, in map[string]any) (sys.WMIRow, error) {
		if method == "Get" {
			return sys.WMIRow{"ReturnValue": int64(0), "cmdletOutput": sys.WMIRow{"MemoryCompression": false}}, nil
		}
		return sys.WMIRow{"ReturnValue": int64(0)}, nil
	}
	res := e.Apply(context.Background(), f, []string{"fix.mem.compression"}, tweaks.ApplyOpts{})
	if len(res) != 1 || res[0].Outcome != tweaks.OutFailed || !strings.Contains(res[0].Message, "não foi confirmada") {
		t.Fatalf("releitura sem mudança deveria falhar: %+v", res)
	}
}

// The agent returns an error code: the apply fails and says so.
func TestMemCompressionErrorCode(t *testing.T) {
	m, _, e := setup(t)
	m.SetMemoryCompression(false)
	f := collect.Collect(context.Background(), m.Platform, nil)
	m.W.OutHandler = func(ns, path, method string, in map[string]any) (sys.WMIRow, error) {
		if method == "Get" {
			return sys.WMIRow{"ReturnValue": int64(0), "cmdletOutput": sys.WMIRow{"MemoryCompression": false}}, nil
		}
		return sys.WMIRow{"ReturnValue": int64(5)}, nil
	}
	res := e.Apply(context.Background(), f, []string{"fix.mem.compression"}, tweaks.ApplyOpts{})
	if len(res) != 1 || res[0].Outcome != tweaks.OutFailed || !strings.Contains(res[0].Message, "código 5") {
		t.Fatalf("código de erro do agente deveria falhar a aplicação: %+v", res)
	}
}

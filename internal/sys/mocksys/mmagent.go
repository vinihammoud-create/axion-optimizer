package mocksys

import (
	"fmt"
	"strings"

	"axion/internal/sys"
)

// MMAgentNamespace is the WMI namespace of the simulated memory manager agent.
const MMAgentNamespace = sys.MMAgentNamespace

// The simulated agent keeps its state in one row of the MMAgentComponents
// table, so World() sees a change and a revert must restore it exactly.
const mmagentTable = "MMAgentComponents"

// mmagentComponents lists the Boolean switches of Enable and Disable, in the
// order of the class (PS_MMAgent: ApplicationLaunchPrefetching 0, OperationAPI
// 1, PageCombining 2, ApplicationPreLaunch 3, MemoryCompression 4).
var mmagentComponents = []string{"ApplicationLaunchPrefetching", "OperationAPI", "PageCombining", "ApplicationPreLaunch", "MemoryCompression"}

// SeedMMAgent registers the agent the way a clean Windows 11 install reports
// it: compression, prefetching and prelaunch on, page combining off (it is off
// by default on a client), 256 operation API files.
func SeedMMAgent(m *Mock) {
	m.W.Set(MMAgentNamespace, mmagentTable, sys.WMIRow{
		"ApplicationLaunchPrefetching": true, "ApplicationPreLaunch": true, "MaxOperationAPIFiles": int64(256),
		"MemoryCompression": true, "OperationAPI": true, "PageCombining": false,
	})
}

// SetMemoryCompression flips the simulated component (tests that need a PC
// where another optimizer turned compression off).
func (m *Mock) SetMemoryCompression(on bool) {
	m.W.mu.Lock()
	defer m.W.mu.Unlock()
	key := strings.ToLower(MMAgentNamespace) + "|" + strings.ToLower(mmagentTable)
	rows := m.W.Tables[key]
	if len(rows) == 0 {
		rows = []sys.WMIRow{{}}
		m.W.Tables[key] = rows
	}
	rows[0]["MemoryCompression"] = on
}

// mmagentCall simulates PS_MMAgent.Get, Enable and Disable. Enable and Disable
// touch only the components passed as true, like the cmdlets.
func (w *WMI) mmagentCall(method string, in map[string]any) (sys.WMIRow, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	key := strings.ToLower(MMAgentNamespace) + "|" + strings.ToLower(mmagentTable)
	rows := w.Tables[key]
	if len(rows) == 0 {
		return nil, fmt.Errorf("PS_MMAgent.%s: classe não encontrada", method)
	}
	row := rows[0]
	switch strings.ToLower(method) {
	case "get":
		// Copy: a caller must not be able to change the state by writing
		// into the answer.
		comp := sys.WMIRow{}
		for k, v := range row {
			comp[k] = v
		}
		return sys.WMIRow{"ReturnValue": int64(0), "cmdletOutput": comp}, nil
	case "enable", "disable":
		want := strings.EqualFold(method, "enable")
		for k, v := range in {
			known := false
			for _, c := range mmagentComponents {
				if strings.EqualFold(c, k) {
					known = true
					if b, _ := v.(bool); b {
						row[c] = want
					}
				}
			}
			if !known {
				return nil, fmt.Errorf("PS_MMAgent.%s: parâmetro %s desconhecido", method, k)
			}
		}
		return sys.WMIRow{"ReturnValue": int64(0)}, nil
	}
	return nil, fmt.Errorf("PS_MMAgent.%s não simulado", method)
}

package catalog

import (
	"fmt"
	"strings"
	"time"

	"axion/internal/facts"
	"axion/internal/startup"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// Startup items (0.9): turning a program on or off in the Inicialização page
// goes through the tweak engine, so the change is written to the journal
// before the machine changes, shows in the Histórico and is undone byte for
// byte. The item is built on demand from its ID,
// "startup:off:<itemID>" or "startup:on:<itemID>", like the active hours.

const startupPrefix = "startup:"

var srcStartupApproved = tweaks.Source{Title: "Microsoft Support — Configurar aplicativos para iniciar automaticamente (Gerenciador de Tarefas > Aplicativos de inicialização)", URL: "https://support.microsoft.com/windows/configure-startup-applications-in-windows-115a420a-0bff-4a6f-90e0-1934c844e473"}

// StartupID names the item that puts a startup entry in the state asked.
func StartupID(itemID string, enabled bool) string {
	st := "off"
	if enabled {
		st = "on"
	}
	return startupPrefix + st + ":" + itemID
}

// ParseStartupID reads the entry and the state out of an ID.
func ParseStartupID(id string) (itemID string, enabled, ok bool) {
	rest, found := strings.CutPrefix(id, startupPrefix)
	if !found {
		return "", false, false
	}
	st, item, found := strings.Cut(rest, ":")
	if !found || item == "" || (st != "on" && st != "off") {
		return "", false, false
	}
	return item, st == "on", true
}

// IsStartupID tells whether an ID is a startup item.
func IsStartupID(id string) bool {
	_, _, ok := ParseStartupID(id)
	return ok
}

// StartupProvider builds the item that turns a startup entry on or off.
func StartupProvider(id string, f *facts.Facts) *tweaks.Tweak {
	itemID, enabled, ok := ParseStartupID(id)
	if !ok {
		return nil
	}
	loc, err := startup.Locate(itemID)
	if err != nil {
		return nil
	}
	verb, summary := "desativar", loc.Name+" deixa de abrir sozinho quando você entra no Windows. A entrada não é apagada: dá para ligar de novo na página Inicialização ou desfazer no Histórico."
	if enabled {
		verb, summary = "ligar", loc.Name+" volta a abrir quando você entra no Windows. Dá para desfazer no Histórico."
	}
	detail := fmt.Sprintf("Grava em %s o valor \"%s\" (REG_BINARY), o mesmo que o Gerenciador de Tarefas usa: 03 seguido da data da mudança desliga, 02 liga. O valor anterior é guardado byte a byte e volta exatamente como estava ao desfazer (se não existia, é apagado). Vale a partir do próximo login.", loc.Key, loc.Value)
	if loc.Task != "" {
		detail = "Muda o estado da tarefa agendada " + loc.Task + " (gatilho ao entrar no Windows) pelo Agendador de Tarefas, como o Gerenciador de Tarefas faz; desfazer devolve o estado anterior. Vale a partir do próximo login."
	}
	return &tweaks.Tweak{
		ID: id, Category: "inicializacao", Title: "Inicialização: " + verb + " " + loc.Name,
		Summary: summary, Detail: detail,
		Evidence: tweaks.EvMedium, Sources: []tweaks.Source{srcStartupApproved},
		Risk: tweaks.RiskSafe, Reboot: tweaks.RebootSignOut, OneClick: false,
		Recommend: neutral("escolha feita por você na página Inicialização"),
		Ops:       func(*facts.Facts) []tweaks.Op { return []tweaks.Op{startupOp(loc, enabled)} },
	}
}

func startupOp(loc startup.Location, enabled bool) tweaks.Op {
	if loc.Task != "" {
		if enabled {
			return taskEnableOp(loc.Task)
		}
		return tasksOp("startup:"+strings.ToLower(loc.Task), []string{loc.Task})
	}
	// Want carries the time of this write (like Task Manager); InTarget asks
	// only on/off, so a value written at another time still counts.
	return tweaks.RegMatch(loc.Key, loc.Value, startup.ApprovedValue(enabled, time.Now()),
		func(v sys.RegValue) bool { return startup.Approved(v) == enabled })
}

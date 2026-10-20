package diag

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"axion/internal/sys"
)

// The kind of fault a blue screen code points to; each has one piece of advice.
const (
	bcDriver   = "driver"
	bcMemory   = "memória"
	bcGPU      = "vídeo"
	bcHardware = "hardware"
	bcStorage  = "armazenamento"
	bcSystem   = "sistema"
	bcPower    = "energia"
)

// bugcode describes a bug check. What the code means comes from Microsoft's
// bug check reference (one page per code, each opened and read when this
// table was written); the advice is Axion's own, by kind, and says so.
type bugcode struct {
	Name string // as Microsoft names it
	Says string // what Microsoft says it is, in plain Portuguese
	Kind string
	URL  string
	Page string // the Axion page that helps (empty: none)
}

const bcBase = "https://learn.microsoft.com/windows-hardware/drivers/debugger/"

var bugcodes = map[int64]bugcode{
	0x0A:  {"IRQL_NOT_LESS_OR_EQUAL", "um driver mexeu em memória que não podia naquele momento", bcDriver, bcBase + "bug-check-0xa--irql-not-less-or-equal", "drivers"},
	0x19:  {"BAD_POOL_HEADER", "o cabeçalho de uma área de memória do sistema está corrompido", bcMemory, bcBase + "bug-check-0x19--bad-pool-header", ""},
	0x1A:  {"MEMORY_MANAGEMENT", "um erro grave no gerenciamento de memória", bcMemory, bcBase + "bug-check-0x1a--memory-management", ""},
	0x1E:  {"KMODE_EXCEPTION_NOT_HANDLED", "um programa do núcleo gerou uma exceção que ninguém tratou", bcDriver, bcBase + "bug-check-0x1e--kmode-exception-not-handled", "drivers"},
	0x3B:  {"SYSTEM_SERVICE_EXCEPTION", "uma exceção ocorreu na passagem de código comum para o núcleo do Windows", bcDriver, bcBase + "bug-check-0x3b--system-service-exception", "drivers"},
	0x50:  {"PAGE_FAULT_IN_NONPAGED_AREA", "memória inválida do sistema foi acessada", bcMemory, bcBase + "bug-check-0x50--page-fault-in-nonpaged-area", ""},
	0x77:  {"KERNEL_STACK_INPAGE_ERROR", "o Windows não conseguiu ler dados do núcleo do arquivo de paginação", bcStorage, bcBase + "bug-check-0x77--kernel-stack-inpage-error", ""},
	0x7A:  {"KERNEL_DATA_INPAGE_ERROR", "o Windows não conseguiu ler dados do núcleo do arquivo de paginação", bcStorage, bcBase + "bug-check-0x7a--kernel-data-inpage-error", ""},
	0x7E:  {"SYSTEM_THREAD_EXCEPTION_NOT_HANDLED", "uma tarefa do sistema gerou uma exceção que ninguém tratou", bcDriver, bcBase + "bug-check-0x7e--system-thread-exception-not-handled", "drivers"},
	0x9F:  {"DRIVER_POWER_STATE_FAILURE", "um driver ficou num estado de energia inconsistente ou inválido", bcPower, bcBase + "bug-check-0x9f--driver-power-state-failure", "drivers"},
	0xC2:  {"BAD_POOL_CALLER", "uma tarefa fez um pedido de memória inválido", bcDriver, bcBase + "bug-check-0xc2--bad-pool-caller", "drivers"},
	0xD1:  {"DRIVER_IRQL_NOT_LESS_OR_EQUAL", "um driver tentou acessar memória paginável num nível de interrupção alto demais", bcDriver, bcBase + "bug-check-0xd1--driver-irql-not-less-or-equal", "drivers"},
	0xEF:  {"CRITICAL_PROCESS_DIED", "um processo crítico do sistema morreu", bcSystem, bcBase + "bug-check-0xef--critical-process-died", ""},
	0x101: {"CLOCK_WATCHDOG_TIMEOUT", "uma interrupção de relógio esperada num núcleo secundário do processador não chegou a tempo", bcHardware, bcBase + "bug-check-0x101---clock-watchdog-timeout", ""},
	0x116: {"VIDEO_TDR_FAILURE", "a tentativa de reiniciar o driver de vídeo depois de um travamento falhou", bcGPU, bcBase + "bug-check-0x116---video-tdr-failure", "drivers"},
	0x119: {"VIDEO_SCHEDULER_INTERNAL_ERROR", "o agendador de vídeo detectou uma violação fatal", bcGPU, bcBase + "bug-check-0x119---video-scheduler-internal-error", "drivers"},
	0x124: {"WHEA_UNCORRECTABLE_ERROR", "ocorreu um erro fatal de hardware", bcHardware, bcBase + "bug-check-0x124---whea-uncorrectable-error", ""},
	0x133: {"DPC_WATCHDOG_VIOLATION", "uma única rotina de driver (DPC) demorou demais, ou o sistema passou tempo demais em nível de despacho ou acima", bcDriver, bcBase + "bug-check-0x133-dpc-watchdog-violation", "dpc"},
	0x139: {"KERNEL_SECURITY_CHECK_FAILURE", "o núcleo detectou a corrupção de uma estrutura de dados crítica", bcDriver, bcBase + "bug-check-0x139--kernel-security-check-failure", "drivers"},
	0x154: {"UNEXPECTED_STORE_EXCEPTION", "o componente de armazenamento de memória do núcleo pegou uma exceção inesperada", bcMemory, bcBase + "bug-check-0x154--unexpected-store-exception", ""},
}

// bugAdvice is Axion's advice by kind of fault (not Microsoft's).
var bugAdvice = map[string]string{
	bcDriver:   "Atualize ou reinstale os drivers que mudaram pouco antes da primeira tela azul (vídeo, rede, armazenamento e chipset primeiro) e remova programas que instalam drivers, como antivírus de terceiros, anti-cheats e overlays, se o problema começou depois deles.",
	bcMemory:   "Teste a memória (Diagnóstico de Memória do Windows ou MemTest86). Se ela roda com XMP/EXPO ou overclock, volte ao padrão para ver se as telas azuis param; depois atualize os drivers.",
	bcGPU:      "Reinstale o driver de vídeo limpando o antigo, tire overclock e undervolt da placa, confira a temperatura e se a fonte dá conta da placa.",
	bcHardware: "É erro de hardware: tire overclock e undervolt do processador e da memória, volte a BIOS ao padrão, confira as temperaturas e a fonte.",
	bcStorage:  "O Windows não conseguiu ler algo do disco: confira a saúde do SSD ou HDD, o cabo e o controlador, e faça backup do que importa.",
	bcSystem:   "Pode ser arquivo do sistema corrompido ou disco com problema: rode \"sfc /scannow\" e depois \"DISM /Online /Cleanup-Image /RestoreHealth\" num prompt de comando como administrador, e confira a saúde do disco.",
	bcPower:    "O driver falhou ao entrar ou sair de suspensão ou hibernação: atualize os drivers (rede, USB, vídeo), teste com a Inicialização Rápida desligada e sem periféricos USB.",
}

var reBugcheckAtStart = regexp.MustCompile(`(?i)^\s*0x([0-9a-f]{1,8})\b`)

// BugcheckCode reads the stop code from a WER-SystemErrorReporting event 1001.
// The message reads "The bugcheck was: 0x00000116 (0x..., 0x..., 0x..., 0x...)".
// The names of the event's fields are not documented, so the code is looked
// for at the start of every value (the first parameter, whatever its name),
// and then of the rendered message.
func BugcheckCode(e sys.Event) (int64, bool) {
	keys := make([]string, 0, len(e.Data))
	for k := range e.Data {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		// param1 and the first positional field come first.
		rank := func(k string) int {
			switch strings.ToLower(k) {
			case "param1":
				return 0
			case "0":
				return 1
			}
			return 2
		}
		if rank(keys[i]) != rank(keys[j]) {
			return rank(keys[i]) < rank(keys[j])
		}
		return keys[i] < keys[j]
	})
	for _, k := range keys {
		if c, ok := parseBugcode(e.Data[k]); ok {
			return c, true
		}
	}
	return parseBugcode(e.Message)
}

func parseBugcode(s string) (int64, bool) {
	m := reBugcheckAtStart.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseInt(m[1], 16, 64)
	if err != nil || n == 0 {
		return 0, false
	}
	return n, true
}

type bugTally struct {
	code  int64
	n     int
	last  time.Time
	known bool
}

// bugTallies counts the stop codes of the blue screens, most frequent first
// (the most recent first among equals).
func bugTallies(evs []sys.Event) []bugTally {
	by := map[int64]*bugTally{}
	for _, e := range evs {
		if !strings.EqualFold(e.Provider, provWER) || e.ID != 1001 {
			continue
		}
		c, ok := BugcheckCode(e)
		if !ok {
			continue
		}
		t := by[c]
		if t == nil {
			_, known := bugcodes[c]
			t = &bugTally{code: c, known: known}
			by[c] = t
		}
		t.n++
		if e.Time.After(t.last) {
			t.last = e.Time
		}
	}
	out := make([]bugTally, 0, len(by))
	for _, t := range by {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].n != out[j].n {
			return out[i].n > out[j].n
		}
		if !out[i].last.Equal(out[j].last) {
			return out[i].last.After(out[j].last)
		}
		return out[i].code < out[j].code
	})
	return out
}

func bugLabel(c int64) string {
	if b, ok := bugcodes[c]; ok {
		return fmt.Sprintf("0x%X %s", c, b.Name)
	}
	return fmt.Sprintf("0x%X", c)
}

// bsodGuidance adds what the stop codes say to the blue screen finding: the
// codes in the evidence, one line of advice per kind of fault (at most three
// codes) and the pages that help. A code that is not in the table is listed
// without advice; no readable code leaves the finding as it was.
func bsodGuidance(x *Finding, evs []sys.Event) {
	tallies := bugTallies(evs)
	if len(tallies) == 0 {
		return
	}
	var codes []string
	for _, t := range tallies {
		s := bugLabel(t.code)
		if t.n > 1 {
			s += fmt.Sprintf(" (%d vezes)", t.n)
		}
		codes = append(codes, s)
	}
	x.Evidence += " Códigos: " + strings.Join(codes, "; ") + "."

	var lines []string
	seenKind := map[string]bool{}
	pages := map[string]bool{}
	shown := 0
	for _, t := range tallies {
		b, ok := bugcodes[t.code]
		if !ok || shown == 3 {
			continue
		}
		shown++
		line := fmt.Sprintf("%s: %s.", bugLabel(t.code), strings.ToUpper(b.Says[:1])+b.Says[1:])
		if !seenKind[b.Kind] {
			seenKind[b.Kind] = true
			line += " " + bugAdvice[b.Kind]
		}
		lines = append(lines, line)
		x.Sources = append(x.Sources, srcMS("Microsoft Learn — Bug Check "+bugLabel(t.code), b.URL))
		if b.Page != "" && !pages[b.Page] {
			pages[b.Page] = true
			label := map[string]string{"drivers": "Ver os drivers", "dpc": "Medir a latência do sistema"}[b.Page]
			x.Actions = append(x.Actions, Action{Kind: "page", Label: label, Page: b.Page})
		}
	}
	if len(lines) > 0 {
		x.Explanation += " O que cada código diz (o significado é da Microsoft; a orientação é do Axion): " + strings.Join(lines, " ")
	}
	if tallies[0].n > 1 && tallies[0].known {
		x.Explanation += fmt.Sprintf(" O código %s se repetiu %d vezes: costuma ser uma causa fixa, não um acaso.", bugLabel(tallies[0].code), tallies[0].n)
	}
}

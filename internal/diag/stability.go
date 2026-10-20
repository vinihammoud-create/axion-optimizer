package diag

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// Event providers read for the stability history.
const (
	provWHEA       = "Microsoft-Windows-WHEA-Logger"
	provKPower     = "Microsoft-Windows-Kernel-Power"
	provKGeneral   = "Microsoft-Windows-Kernel-General"
	provKBoot      = "Microsoft-Windows-Kernel-Boot"
	provWER        = "Microsoft-Windows-WER-SystemErrorReporting"
	provDisplay    = "Display"
	bootWindow     = 5 * time.Minute
	bootWindowLead = 30 * time.Second
)

var (
	srcWHEA = tweaks.Source{Title: "Microsoft Learn — Introdução ao WHEA: erros de hardware não fatais são recuperados sem tela azul", URL: "https://learn.microsoft.com/windows-hardware/drivers/whea/introduction-to-the-windows-hardware-error-architecture"}
	srcAER  = tweaks.Source{Title: "Kernel Linux — PCIe AER: erros corrigíveis não afetam o funcionamento e são recuperados sem perda de dados", URL: "https://docs.kernel.org/PCI/pcieaer-howto.html"}
)

// Stability rules use the event log (collected separately because it can be
// slow). Without the machine profile the advice is generic.
func Stability(evs []sys.Event) []Finding { return StabilityFor(evs, nil) }

// StabilityFor is Stability with the machine profile, which names the devices
// behind PCIe errors and tailors the advice (a notebook has no XMP to turn
// off).
func StabilityFor(evs []sys.Event, f *facts.Facts) []Finding { return StabilityWith(evs, f, nil) }

// StabilityWith also knows the state of the tweaks: a test the PC already
// runs (PCIe power saving off) is not offered again.
func StabilityWith(evs []sys.Event, f *facts.Facts, states map[string]tweaks.State) []Finding {
	var out []Finding
	count := func(provider string, ids ...int) (int, time.Time) {
		n := 0
		var last time.Time
		for _, e := range evs {
			if !strings.EqualFold(e.Provider, provider) {
				continue
			}
			match := len(ids) == 0
			for _, id := range ids {
				if e.ID == id {
					match = true
				}
			}
			if match {
				n++
				if e.Time.After(last) {
					last = e.Time
				}
			}
		}
		return n, last
	}
	// Symptoms decide how much a corrected hardware error weighs: the same
	// log with a blue screen or a display driver reset in it is another story.
	kp41, _ := count(provKPower, 41)
	bsod, _ := count(provWER, 1001)
	tdr, _ := count(provDisplay, 4101)
	if x := wheaFinding(evs, f, states, kp41+bsod+tdr > 0); x != nil {
		out = append(out, *x)
	}
	// After a blue screen Windows also writes a Kernel-Power 41, with the stop
	// code in BugcheckCode: that one is the blue screen (reported below), not
	// a power loss.
	n41, last41 := 0, time.Time{}
	for _, e := range evs {
		if !strings.EqualFold(e.Provider, provKPower) || e.ID != 41 {
			continue
		}
		if c := dataGet(e.Data, "BugcheckCode"); c != "" && c != "0" {
			continue
		}
		n41++
		if e.Time.After(last41) {
			last41 = e.Time
		}
	}
	if n41 > 0 {
		why := "O PC reiniciou ou desligou sem encerrar o Windows: queda de energia, fonte, temperatura ou instabilidade de overclock. Desligamentos bruscos também desgastam SSDs."
		if f != nil && f.IsLaptop() {
			why = "O notebook desligou sem encerrar o Windows: bateria que acabou, o botão de ligar segurado, temperatura ou um travamento. Desligamentos bruscos também desgastam SSDs."
		}
		out = append(out, Finding{ID: "stab.kp41", Area: AreaEstab, Severity: SevWarning, Title: plural(n41, "desligamento inesperado", "desligamentos inesperados"),
			Evidence:    "Kernel-Power 41; último em " + last41.Format("02/01/2006 15:04") + ".",
			Explanation: why})
	}
	if n, last := count(provWER, 1001); n > 0 {
		x := Finding{ID: "stab.bsod", Area: AreaEstab, Severity: SevCritical, Title: plural(n, "tela azul registrada", "telas azuis registradas"),
			Evidence: "BugCheck; último em " + last.Format("02/01/2006 15:04") + ".", Explanation: "Telas azuis indicam driver com defeito ou hardware instável. Reverta overclocks e atualize drivers de chipset e vídeo."}
		bsodGuidance(&x, evs)
		out = append(out, x)
	}
	if n, last := count(provDisplay, 4101); n > 0 {
		out = append(out, Finding{ID: "stab.tdr", Area: AreaEstab, Severity: SevWarning, Title: plural(n, "reinício do driver de vídeo", "reinícios do driver de vídeo"),
			Evidence:    "Evento 4101 (driver de vídeo parou de responder); último em " + last.Format("02/01/2006 15:04") + ".",
			Explanation: "Geralmente overclock/undervolt de GPU instável, driver corrompido (reinstale com DDU) ou temperatura alta."})
	}
	out = append(out, thermalFindings(evs, f)...)
	return out
}

// wheaEvent is one WHEA-Logger record, classified.
type wheaEvent struct {
	t      time.Time
	kind   string // pcie | cpu | mem | other
	fatal  bool
	device string // VEN_xxxx&DEV_xxxx of the reporting PCIe device
	second string // device below the port, when reported
	class  int64  // PCI class code, -1 unknown
}

var rePCIID = regexp.MustCompile(`(?i)VEN_([0-9A-F]{4})&DEV_([0-9A-F]{4})`)

// dataGet reads an EventData field, ignoring the case of its name (the
// manifests spell VendorID and VendorId).
func dataGet(d map[string]string, name string) string {
	for k, v := range d {
		if strings.EqualFold(k, name) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func hexNum(s string) (int64, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimPrefix(s, "0x"), 16, 64)
	return n, err == nil
}

// classifyWHEA sorts a record by what reported it. Corrected errors are
// logged as warnings and uncorrected ones as errors; 17 is a corrected PCI
// Express error, 18 a fatal machine check, 19 a corrected machine check and
// 47 a corrected memory error.
func classifyWHEA(e sys.Event) wheaEvent {
	w := wheaEvent{t: e.Time, kind: "other", class: -1}
	w.fatal = e.Level == 1 || e.Level == 2 || e.ID == 18
	pcie := e.ID == 17 || dataGet(e.Data, "PrimaryDeviceName") != "" ||
		(dataGet(e.Data, "VendorID") != "" && dataGet(e.Data, "DeviceID") != "" && dataGet(e.Data, "ClassCode") != "")
	switch {
	case pcie:
		w.kind = "pcie"
	case e.ID == 47:
		w.kind = "mem"
	case e.ID == 18 || e.ID == 19 || dataGet(e.Data, "MciStat") != "" || dataGet(e.Data, "MCABank") != "":
		w.kind = "cpu"
	}
	if w.kind != "pcie" {
		return w
	}
	if m := rePCIID.FindStringSubmatch(dataGet(e.Data, "PrimaryDeviceName")); m != nil {
		w.device = "VEN_" + strings.ToUpper(m[1]) + "&DEV_" + strings.ToUpper(m[2])
	} else if v, ok1 := hexNum(dataGet(e.Data, "VendorID")); ok1 {
		if d, ok2 := hexNum(dataGet(e.Data, "DeviceID")); ok2 && v > 0 && v < 0xFFFF {
			w.device = fmt.Sprintf("VEN_%04X&DEV_%04X", v, d)
		}
	}
	if m := rePCIID.FindStringSubmatch(dataGet(e.Data, "SecondaryDeviceName")); m != nil {
		w.second = "VEN_" + strings.ToUpper(m[1]) + "&DEV_" + strings.ToUpper(m[2])
	}
	if c, ok := hexNum(dataGet(e.Data, "ClassCode")); ok {
		w.class = c
	}
	return w
}

// pciVendors names the vendors most often seen behind PCIe errors.
var pciVendors = map[string]string{
	"8086": "Intel", "1022": "AMD", "10DE": "NVIDIA", "1002": "AMD", "10EC": "Realtek", "14E4": "Broadcom",
	"168C": "Qualcomm Atheros", "17CB": "Qualcomm", "14C3": "MediaTek", "144D": "Samsung", "15B7": "WD/SanDisk",
	"1987": "Phison", "1CC1": "ADATA", "126F": "Silicon Motion", "1E0F": "KIOXIA", "1C5C": "SK hynix", "2646": "Kingston",
	"1B21": "ASMedia", "1E4B": "MAXIO", "1D97": "Shenzhen Longsys", "C0A9": "Micron/Crucial",
}

// pciName names a PCI device as Device Manager does (the PCI list of the
// profile, then the GPUs, adapters and controllers it already knows), or by
// its vendor. The class code the WHEA record carries is not used for the
// name: the first notebook report logged it in a format that does not match
// the PCI specification (the GPU's own HDMI audio came out as a "network
// card"), and a public record of an Intel root port shows 0x30400 where the
// specification says 0x060400.
func pciName(f *facts.Facts, id string, class int64) string {
	if id == "" {
		return "dispositivo PCIe não identificado"
	}
	if f != nil {
		if n := f.PCIDeviceName(id); n != "" {
			return n + " (" + id + ")"
		}
		has := func(pnp string) bool { return pnp != "" && strings.Contains(strings.ToUpper(pnp), id) }
		for _, g := range f.GPUs {
			if has(g.PNPDeviceID) {
				return g.Name + " (" + id + ")"
			}
		}
		for _, a := range f.Adapters {
			if has(a.PnPDeviceID) {
				return a.Description + " (" + id + ")"
			}
		}
		for _, c := range f.Controllers {
			if has(c.PNP) {
				return c.Name + " (" + id + ")"
			}
		}
	}
	what := "dispositivo PCIe"
	if ven := strings.TrimPrefix(id, "VEN_"); len(ven) >= 4 {
		if v, ok := pciVendors[ven[:4]]; ok {
			what += " " + v
		}
	}
	return what + " (" + id + ")"
}

// bootTimes are the moments the PC started or woke up: OS start
// (Kernel-General 12), boot type (Kernel-Boot 27, also logged with Fast
// Startup), resume from sleep (Kernel-Power 107) and from Modern Standby
// (Kernel-Power 507), plus the last boot from the profile.
func bootTimes(evs []sys.Event, f *facts.Facts) []time.Time {
	var out []time.Time
	for _, e := range evs {
		switch {
		case strings.EqualFold(e.Provider, provKGeneral) && e.ID == 12,
			strings.EqualFold(e.Provider, provKBoot) && e.ID == 27,
			strings.EqualFold(e.Provider, provKPower) && (e.ID == 107 || e.ID == 507):
			out = append(out, e.Time)
		}
	}
	if f != nil && !f.OS.LastBoot.IsZero() {
		out = append(out, f.OS.LastBoot)
	}
	return out
}

func nearBoot(t time.Time, boots []time.Time) bool {
	for _, b := range boots {
		if d := t.Sub(b); d >= -bootWindowLead && d <= bootWindow {
			return true
		}
	}
	return false
}

// wheaBurst is how many corrected errors inside wheaBurstWindow, away from a
// boot or a resume, count as a burst: a link that corrects errors while the
// PC is in use, many times in minutes, is marginal even if nothing failed.
const (
	wheaBurst       = 10
	wheaBurstWindow = 5 * time.Minute
)

// burstOf tells whether the times (any order) hold wheaBurst of them inside
// one wheaBurstWindow.
func burstOf(times []time.Time) bool {
	if len(times) < wheaBurst {
		return false
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	for i := 0; i+wheaBurst-1 < len(times); i++ {
		if times[i+wheaBurst-1].Sub(times[i]) <= wheaBurstWindow {
			return true
		}
	}
	return false
}

// wheaFinding summarises the hardware errors: how many were corrected by the
// hardware and how many were fatal, which kind and device, and whether they
// happen only while the PC starts or wakes up (when PCIe links train and
// devices change power state) or during use. The severity follows the
// impact: a corrected error has none by itself (the PCIe specification
// recovers it without data loss), so with no fatal error, no symptom in the
// same period (unexpected shutdown, blue screen, display driver reset) and
// no burst it is a note; symptoms or a burst make it a warning; a fatal
// error is critical. symptoms says whether the period holds any of those.
func wheaFinding(evs []sys.Event, f *facts.Facts, states map[string]tweaks.State, symptoms bool) *Finding {
	var list []wheaEvent
	for _, e := range evs {
		// Informational records (level 4 and up) are not errors.
		if strings.EqualFold(e.Provider, provWHEA) && e.Level < 4 {
			list = append(list, classifyWHEA(e))
		}
	}
	if len(list) == 0 {
		return nil
	}
	boots := bootTimes(evs, f)
	fatal, atBoot, pcieCorrected := 0, 0, 0
	kinds := map[string]int{}
	inUse := map[string]int{}
	fatalKind := map[string]int{}
	devices := map[string]int{}
	var last time.Time
	var useTimes []time.Time
	for _, w := range list {
		if w.fatal {
			fatal++
			fatalKind[w.kind]++
		}
		// Only corrected PCIe errors are expected while links train at
		// startup or resume; a machine check (processor, memory) right after
		// a resume is as telling as one during a game (memory or fabric
		// instability, undervolt).
		if w.kind == "pcie" && !w.fatal {
			pcieCorrected++
		}
		if w.kind == "pcie" && !w.fatal && nearBoot(w.t, boots) {
			atBoot++
		} else {
			inUse[w.kind]++
			if !w.fatal {
				useTimes = append(useTimes, w.t)
			}
		}
		kinds[w.kind]++
		if w.kind == "pcie" {
			name := pciName(f, w.device, w.class)
			if w.second != "" {
				name += " → " + pciName(f, w.second, -1)
			}
			devices[name]++
		}
		if w.t.After(last) {
			last = w.t
		}
	}
	n := len(list)
	labels := map[string]string{"pcie": "PCI Express", "cpu": "processador (machine check)", "mem": "memória", "other": "outros"}
	var byKind []string
	for _, k := range []string{"pcie", "cpu", "mem", "other"} {
		if kinds[k] == 0 {
			continue
		}
		part := fmt.Sprintf("%s %d", labels[k], kinds[k])
		if k == "pcie" && len(devices) > 0 {
			names := make([]string, 0, len(devices))
			for d := range devices {
				names = append(names, d)
			}
			sort.Slice(names, func(i, j int) bool {
				if devices[names[i]] != devices[names[j]] {
					return devices[names[i]] > devices[names[j]]
				}
				return names[i] < names[j]
			})
			if len(names) > 3 {
				names = names[:3]
			}
			for i, d := range names {
				names[i] = fmt.Sprintf("%s: %d", d, devices[d])
			}
			part += " (" + strings.Join(names, "; ") + ")"
		}
		byKind = append(byKind, part)
	}
	ev := "Por tipo: " + strings.Join(byKind, ", ") + "."
	if fatal > 0 {
		ev += fmt.Sprintf(" %d não corrigido(s) pelo hardware.", fatal)
	}
	if len(boots) > 0 && pcieCorrected > 0 {
		ev += fmt.Sprintf(" %d de %d erros PCIe corrigidos até 5 minutos depois de ligar ou acordar o PC.", atBoot, pcieCorrected)
	}
	burst := burstOf(useTimes)
	if burst {
		ev += fmt.Sprintf(" Rajada: %d ou mais erros em %d minutos com o PC em uso.", wheaBurst, int(wheaBurstWindow.Minutes()))
	}
	if fatal == 0 {
		if symptoms {
			ev += " No mesmo período há desligamento inesperado, tela azul ou reinício do driver de vídeo (veja os outros itens)."
		} else {
			ev += " Nenhum desligamento inesperado, tela azul ou reinício do driver de vídeo no período."
		}
	}
	ev += " Último em " + last.Format("02/01/2006 15:04") + "."

	laptop := f != nil && f.IsLaptop()
	used := n - atBoot
	// The PCIe power-saving test is only offered when the PC does not run
	// it already (the first notebook had ASPM off on AC and the errors went
	// on). "ASPM is not the cause" is said only when errors were logged
	// after the setting was in place: the plan was already like that (not
	// applied by Axion), or the last error came after Axion applied it.
	aspmOn, aspmRuledOut := false, false
	if st, ok := states["power.pcie.aspm"]; ok && st.Status == "on" {
		aspmOn = true
		aspmRuledOut = st.AppliedAt == nil || last.After(*st.AppliedAt)
	}
	aspmNote := func() string {
		if aspmRuledOut {
			return "A economia de energia do PCIe (ASPM) já está desligada na tomada e os erros continuam: o ASPM não é a causa."
		}
		return "A economia de energia do PCIe (ASPM) já está desligada na tomada; se os erros continuarem a aparecer depois dessa mudança, o ASPM não é a causa."
	}
	// A corrected machine check or memory error, even once and away from a
	// boot, is not the noise a PCIe link makes while it trains: it stays a
	// warning on its own.
	nonPCIe := inUse["cpu"]+inUse["mem"]+inUse["other"] > 0
	x := &Finding{ID: "stab.whea", Area: AreaEstab, Evidence: ev, Sources: []tweaks.Source{srcWHEA, srcAER}}
	switch {
	case fatal > 0:
		x.Severity = SevCritical
		x.Title = plural(fatal, "erro de hardware grave (WHEA)", "erros de hardware graves (WHEA)") + " nos últimos 30 dias"
	case used > 0 && (symptoms || burst || nonPCIe):
		x.Severity = SevWarning
		x.Title = plural(n, "erro de hardware corrigido (WHEA) nos últimos 30 dias", "erros de hardware corrigidos (WHEA) nos últimos 30 dias")
		switch {
		case symptoms:
			x.Title += ", com sintomas no período"
		case burst:
			x.Title += ", em rajada"
		}
	case used > 0:
		x.Severity = SevInfo
		x.Title = plural(n, "erro de hardware corrigido pelo hardware (WHEA), sem sintoma", "erros de hardware corrigidos pelo hardware (WHEA), sem sintoma")
	default:
		x.Severity = SevInfo
		x.Title = plural(n, "erro de hardware corrigido (WHEA), só ao ligar ou acordar o PC", "erros de hardware corrigidos (WHEA), só ao ligar ou acordar o PC")
	}
	var ex []string
	if fatal > 0 {
		ex = append(ex, "Um erro grave é um erro que o hardware não conseguiu corrigir: o Windows para com tela azul ou reinício.")
	}
	switch {
	case used == 0 && fatal == 0:
		ex = append(ex, "São erros PCIe que o próprio hardware corrigiu, todos logo depois de ligar ou acordar o PC, quando os links PCIe são inicializados. "+
			"Sozinhos não indicam defeito nem afetam o jogo; passam a preocupar se aparecerem durante o uso, junto com travadas ou telas azuis. Mantenha a BIOS e o driver de chipset atualizados.")
	case x.Severity == SevInfo:
		ex = append(ex, "Erros corrigíveis são recuperados pelo próprio PCI Express, sem perda de dados e sem efeito no funcionamento; sem erro grave, sem sintoma e sem rajada, não há nada a corrigir.")
		if laptop {
			ex = append(ex, "Numa placa de vídeo de notebook, que desliga em repouso, o link é renegociado ao acordar, como no boot.")
		}
		ex = append(ex, "Vale acompanhar: se surgirem travadas, tela preta ou reinício do driver de vídeo, este item passa a pesar.")
		if inUse["pcie"] > 0 {
			if aspmOn {
				ex = append(ex, aspmNote())
			} else {
				ex = append(ex, "Se quiser testar algo, desligar a economia de energia do PCIe (ASPM) na tomada é simples e reversível.")
				x.Actions = append(x.Actions, fixTweaks("Testar sem economia de energia do PCIe", "power.pcie.aspm"))
			}
		}
	default:
		if inUse["pcie"] > 0 || fatalKind["pcie"] > 0 {
			if laptop {
				ex = append(ex, "PCI Express: atualize a BIOS e os drivers de chipset e do dispositivo indicado.")
			} else {
				ex = append(ex, "PCI Express: link instável com o dispositivo indicado. Reencaixe a placa, evite riser ou cabo de extensão e, se usa riser, fixe a geração do slot na BIOS (por exemplo, Gen 4 em vez de Auto).")
			}
			if aspmOn {
				ex = append(ex, aspmNote())
			} else {
				ex = append(ex, "Em vários relatos, desligar a economia de energia do PCIe (ASPM) faz os avisos pararem: é um teste simples e reversível.")
				x.Actions = append(x.Actions, fixTweaks("Testar sem economia de energia do PCIe", "power.pcie.aspm"))
			}
		}
		if inUse["cpu"] > 0 || fatalKind["cpu"] > 0 {
			if laptop {
				ex = append(ex, "Processador: se você usa undervolt (ThrottleStop, Intel XTU), volte ao padrão e atualize a BIOS; se aparecerem travadas ou telas azuis, procure a assistência técnica.")
			} else {
				ex = append(ex, "Processador: o mais comum é overclock ou undervolt instável (XMP/EXPO, PBO, Curve Optimizer, multiplicador): teste com tudo no padrão. Nos Intel de 13ª/14ª geração de desktop, confira também o microcódigo da BIOS.")
			}
		}
		if inUse["mem"] > 0 || fatalKind["mem"] > 0 {
			if laptop {
				ex = append(ex, "Memória: rode um teste de memória (Diagnóstico de Memória do Windows ou MemTest86).")
			} else {
				ex = append(ex, "Memória: desligue o XMP/EXPO para testar e rode um teste de memória (Diagnóstico de Memória do Windows ou MemTest86).")
			}
		}
		if inUse["other"] > 0 {
			ex = append(ex, "Os demais aparecem no Visualizador de Eventos (Logs do Windows > Sistema, origem WHEA-Logger).")
		}
		ex = append(ex, "Erros de hardware durante o uso, junto com sintomas, são causa comum de travadas e reinícios.")
	}
	x.Explanation = strings.Join(ex, " ")
	return x
}

// WHEARow is one hardware error for the exported report: enough to check
// the classification by hand (time, kind, device, corrected or not, near a
// boot or resume).
type WHEARow struct {
	Time     time.Time `json:"time"`
	ID       int       `json:"id"`
	Kind     string    `json:"kind"`
	Device   string    `json:"device,omitempty"`
	Second   string    `json:"second,omitempty"`
	Fatal    bool      `json:"fatal"`
	NearBoot bool      `json:"nearBoot"`
}

// WHEARows lists the hardware errors of the period, newest first.
func WHEARows(evs []sys.Event, f *facts.Facts) []WHEARow {
	boots := bootTimes(evs, f)
	var out []WHEARow
	for _, e := range evs {
		if !strings.EqualFold(e.Provider, provWHEA) || e.Level >= 4 {
			continue
		}
		w := classifyWHEA(e)
		out = append(out, WHEARow{Time: w.t, ID: e.ID, Kind: w.kind, Device: w.device, Second: w.second, Fatal: w.fatal, NearBoot: nearBoot(w.t, boots)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out
}

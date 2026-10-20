// Package diag turns the machine profile into findings, scores and
// recommendations. Every finding carries the evidence read from the machine,
// so the user can see why it was raised.
package diag

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"axion/internal/dpc"
	"axion/internal/facts"
	"axion/internal/netdiag"
	"axion/internal/obs"
	"axion/internal/perf"
	"axion/internal/profile"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// Severity levels.
const (
	SevCritical = "critical"
	SevWarning  = "warning"
	SevInfo     = "info"
	SevGood     = "good"
)

// Areas (score groups).
const (
	AreaDesempenho = "desempenho"
	AreaLatencia   = "latencia"
	AreaEstab      = "estabilidade"
	AreaRede       = "rede"
	AreaArmaz      = "armazenamento"
	AreaSeguranca  = "seguranca"
)

// AreaTitles for the UI.
var AreaTitles = map[string]string{
	AreaDesempenho: "Desempenho",
	AreaLatencia:   "Latência e entrada",
	AreaEstab:      "Estabilidade",
	AreaRede:       "Rede",
	AreaArmaz:      "Armazenamento",
	AreaSeguranca:  "Segurança e anti-cheat",
}

// Action links a finding to something the app can do.
type Action struct {
	Kind   string   `json:"kind"` // tweaks | page | url
	Label  string   `json:"label"`
	Tweaks []string `json:"tweaks,omitempty"`
	Page   string   `json:"page,omitempty"`
	URL    string   `json:"url,omitempty"`
	// Category selects the category of the Otimizações page the action opens.
	Category string `json:"category,omitempty"`
}

var (
	srcNicDefaults = tweaks.Source{Title: "Microsoft Learn — Reset-NetAdapterAdvancedProperty (volta as propriedades avançadas ao padrão)", URL: "https://learn.microsoft.com/powershell/module/netadapter/reset-netadapteradvancedproperty"}
	srcIntelWifi   = tweaks.Source{Title: "Intel — Configurações avançadas dos adaptadores Wi-Fi (artigo 000005585)", URL: "https://www.intel.com/content/www/us/en/support/articles/000005585/wireless/legacy-intel-wireless-products.html"}
	srcPCIeScaling = tweaks.Source{Title: "Club386 (testes do TechPowerUp) — RTX 4090: −2% em PCIe 4.0 x8, −6% em 4.0 x4 e −21% em 3.0 x4 (1080p)", URL: "https://www.club386.com/geforce-rtx-4090-pcie-scaling-test-shows-impressive-performance-even-with-low-lane-count/"}
	srcNvidiaSMI   = tweaks.Source{Title: "NVIDIA — Documentação do nvidia-smi: a largura e a geração do link PCIe podem ser reduzidas com a GPU sem uso", URL: "https://docs.nvidia.com/deploy/nvidia-smi/index.html"}
	srcEEEIntel    = tweaks.Source{Title: "Tom's Hardware — Intel desliga o EEE a 1/2,5 Gb/s para corrigir quedas das placas I225/I226", URL: "https://www.tomshardware.com/news/intel-patches-stuttering-ethernet-issues-but-its-just-a-workaround-for-now"}
	srcEEEWake     = tweaks.Source{Title: "EE News Europe — Tempo para acordar o link no EEE (16,5 µs no gigabit)", URL: "https://www.eenewseurope.com/en/powering-ethernet-part-2-optimizing-power-consumption-during-standby-operation/"}
)

// Finding is one diagnostic result.
type Finding struct {
	ID          string          `json:"id"`
	Area        string          `json:"area"`
	Severity    string          `json:"severity"`
	Title       string          `json:"title"`
	Evidence    string          `json:"evidence"`
	Explanation string          `json:"explanation"`
	Actions     []Action        `json:"actions,omitempty"`
	Sources     []tweaks.Source `json:"sources,omitempty"`
	// Dismissed: the user chose to stop being warned about it (it stays
	// listed apart and no longer lowers the score).
	Dismissed bool `json:"dismissed,omitempty"`
	// OffProfile: it only matters in a use the chosen profile does not
	// include (a gaming item on a work PC); listed apart, not scored.
	OffProfile bool `json:"offProfile,omitempty"`
}

// AreaScore is a per-area score.
type AreaScore struct {
	Area  string `json:"area"`
	Title string `json:"title"`
	Score int    `json:"score"`
	Count int    `json:"count"`
}

// Report is the diagnostic output.
type Report struct {
	Generated time.Time   `json:"generated"`
	Overall   int         `json:"overall"`
	Grade     string      `json:"grade"`
	Scores    []AreaScore `json:"scores"`
	Findings  []Finding   `json:"findings"`
	Summary   string      `json:"summary"`
	// Profile is the use profile the score was computed for.
	Profile string `json:"profile,omitempty"`
	// Unscored: nothing to score yet (the measurement reading before any
	// measurement); Overall is -1 and Grade says so.
	Unscored bool `json:"unscored,omitempty"`
}

// Unscore marks a report that has nothing to grade: a 100 with no
// measurement would read as a result.
func (r *Report) Unscore(summary string) {
	r.Unscored, r.Overall, r.Grade, r.Summary = true, -1, "Sem medição", summary
}

// Penalties per severity. Area bars use the heavier table (a single critical
// problem should visibly sink its area); the overall score subtracts the
// lighter table for every finding, so many small problems also add up.
var (
	areaPenalty    = map[string]int{SevCritical: 25, SevWarning: 9, SevInfo: 2, SevGood: 0}
	overallPenalty = map[string]int{SevCritical: 12, SevWarning: 5, SevInfo: 1, SevGood: 0}
)

// Areas in display order.
var Areas = []string{AreaDesempenho, AreaLatencia, AreaEstab, AreaRede, AreaArmaz, AreaSeguranca}

// ScoreNote explains the scoring in the UI.
const ScoreNote = "A nota geral parte de 100 e perde 12 pontos por problema importante, 5 por ponto de atenção e 1 por sugestão; com algum problema importante ela fica no máximo em 74 (Regular). As notas por área usam o mesmo princípio dentro de cada área. Avisos dispensados e pontos fora do seu perfil de uso não contam."

// Measured carries what the Axion itself measured on this PC into the
// diagnostic, so the rules speak from the user's own numbers: the last
// complete network test and the last FPS capture of the competitive game.
type Measured struct {
	Net       *netdiag.Summary `json:"net,omitempty"`       // newest test with a grade (the queue under load)
	NetLatest *netdiag.Summary `json:"netLatest,omitempty"` // newest test of all (the route to Valve), as the CS2 page uses
	Perf      *perf.Summary    `json:"perf,omitempty"`
	PerfAt    time.Time        `json:"perfAt"`
	PerfGame  string           `json:"perfGame,omitempty"`
	// Link is the PCIe link of the GPU read during the newest capture that
	// took one (any game), with the game it was read with.
	Link     *perf.GPULink `json:"link,omitempty"`
	LinkGame string        `json:"linkGame,omitempty"`
	// DPC is the newest reliable capture of the kernel's DPCs and interrupts.
	DPC      *dpc.Summary `json:"dpc,omitempty"`
	DPCAt    time.Time    `json:"dpcAt"`
	DPCLabel string       `json:"dpcLabel,omitempty"`
	// OBS is the newest stream (or recording) read from the OBS logs and what
	// it says.
	OBS         *obs.Session  `json:"obs,omitempty"`
	OBSAnalysis *obs.Analysis `json:"obsAnalysis,omitempty"`
}

// Build runs every rule. Extra findings (e.g. from the event log, collected
// separately because it is slow) are merged before scoring.
func Build(f *facts.Facts, states map[string]tweaks.State, extra ...Finding) *Report {
	return BuildWith(f, states, nil, extra...)
}

// BuildWith is Build with the measurements: they add findings of their own
// (the network under load) and temper others (the memory profile advice when
// the game is limited by the GPU).
func BuildWith(f *facts.Facts, states map[string]tweaks.State, m *Measured, extra ...Finding) *Report {
	var fs []Finding
	add := func(x Finding) { fs = append(fs, x) }
	for _, rule := range rules {
		rule(f, states, add)
	}
	fs = append(fs, extra...)
	if m != nil {
		fs = applyMeasured(fs, f, m)
	}
	return Score(fs)
}

// Dismissible tells whether the user may dismiss a finding: warnings and
// suggestions only; critical problems always count.
func Dismissible(x Finding) bool { return x.Severity == SevWarning || x.Severity == SevInfo }

// WithDismissed marks the findings the user dismissed and scores only the
// others; dismissed ones go to the end of the list.
func WithDismissed(r *Report, ids map[string]bool) *Report {
	return Finalize(r, "", ids)
}

// Finalize reads a report for a use profile and the user's dismissals: the
// findings outside the profile (a gaming item on a work PC, whatever its
// severity: a 240 Hz monitor at 60 Hz is not a problem of a spreadsheet
// PC) and the dismissed ones are marked, listed after the others and left
// out of the score. An empty profile is the default (everything counts).
// Findings of general scope (a failing SSD, blue screens, an anti-cheat
// requirement) count in every profile.
func Finalize(r *Report, profileID string, dismissed map[string]bool) *Report {
	if r == nil {
		return nil
	}
	p := profile.Get(profileID)
	var active, off, hidden []Finding
	for _, x := range r.Findings {
		switch {
		case dismissed[x.ID] && Dismissible(x):
			x.Dismissed = true
			hidden = append(hidden, x)
		case !p.FindingCounts(x.ID):
			x.OffProfile = true
			off = append(off, x)
		default:
			active = append(active, x)
		}
	}
	if len(off) == 0 && len(hidden) == 0 {
		r.Profile = p.ID
		return r
	}
	out := Score(active)
	out.Generated = r.Generated
	out.Profile = p.ID
	out.Findings = append(out.Findings, off...)
	out.Findings = append(out.Findings, hidden...)
	return out
}

// Score sorts findings and computes the scores.
func Score(fs []Finding) *Report {
	sort.SliceStable(fs, func(i, j int) bool { return sevRank(fs[i].Severity) < sevRank(fs[j].Severity) })
	r := &Report{Generated: time.Now(), Findings: fs}
	overall := 100
	crit, warn := 0, 0
	for _, x := range fs {
		overall -= overallPenalty[x.Severity]
		switch x.Severity {
		case SevCritical:
			crit++
		case SevWarning:
			warn++
		}
	}
	for _, a := range Areas {
		sc, n := 100, 0
		for _, x := range fs {
			if x.Area == a {
				sc -= areaPenalty[x.Severity]
				if x.Severity != SevGood {
					n++
				}
			}
		}
		if sc < 5 {
			sc = 5
		}
		r.Scores = append(r.Scores, AreaScore{Area: a, Title: AreaTitles[a], Score: sc, Count: n})
	}
	if overall < 5 {
		overall = 5
	}
	// A problem marked important never sits under "Bom": one critical item
	// alone (a failing SSD, blue screens) would otherwise cost only 12
	// points and still read 88.
	if crit > 0 && overall > 74 {
		overall = 74
	}
	r.Overall = overall
	switch {
	case r.Overall >= 90:
		r.Grade = "Excelente"
	case r.Overall >= 75:
		r.Grade = "Bom"
	case r.Overall >= 50:
		r.Grade = "Regular"
	default:
		r.Grade = "Precisa de atenção"
	}
	switch {
	case crit == 0 && warn == 0:
		r.Summary = "Nenhum problema importante encontrado. Veja as sugestões opcionais."
	case crit == 0:
		r.Summary = plural(warn, "ponto de atenção encontrado.", "pontos de atenção encontrados.")
	case warn == 0:
		r.Summary = plural(crit, "problema importante encontrado.", "problemas importantes encontrados.")
	default:
		r.Summary = plural(crit, "problema importante", "problemas importantes") + " e " + plural(warn, "ponto de atenção encontrados.", "pontos de atenção encontrados.")
	}
	return r
}

// win10End returns when security updates end for a Windows 10 build and
// edition (Microsoft lifecycle pages), and whether it is a long-term (LTSC)
// edition.
func win10End(build int, edition string) (time.Time, bool) {
	ed := strings.ToLower(edition)
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 23, 59, 0, 0, time.UTC) }
	if strings.Contains(ed, "enterprises") { // EnterpriseS, EnterpriseSN, IoTEnterpriseS: LTSC
		iot := strings.HasPrefix(ed, "iot")
		switch {
		case build >= 19044 && iot:
			return d(2032, time.January, 13), true // IoT Enterprise LTSC 2021
		case build >= 19044:
			return d(2027, time.January, 12), true // Enterprise LTSC 2021
		case build >= 17763:
			return d(2029, time.January, 9), true // LTSC 2019 (extended support)
		case build >= 14393:
			return d(2026, time.October, 13), true // LTSB 2016 (extended support)
		}
	}
	return d(2025, time.October, 14), false
}

// win11End returns when security updates end for a Windows 11 version
// (Home/Pro, or Enterprise/Education with 12 months more) and its name.
func win11End(build int, edition string) (time.Time, string) {
	ed := strings.ToLower(edition)
	d0 := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 23, 59, 0, 0, time.UTC) }
	// Long-term servicing (EnterpriseS / IoTEnterpriseS) has its own dates.
	// Microsoft Lifecycle: Enterprise LTSC 2024 until 09/10/2029, IoT
	// Enterprise LTSC 2024 until 10/10/2034.
	if strings.HasSuffix(ed, "enterprises") || strings.Contains(ed, "enterprises") {
		if build >= 26100 && build < 26200 {
			if strings.HasPrefix(ed, "iot") {
				return d0(2034, time.October, 10), "LTSC 2024"
			}
			return d0(2029, time.October, 9), "LTSC 2024"
		}
		return time.Time{}, ""
	}
	ent := strings.HasPrefix(ed, "enterprise") || strings.HasPrefix(ed, "education") || strings.HasPrefix(ed, "iotenterprise")
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 23, 59, 0, 0, time.UTC) }
	type ver struct {
		build     int
		name      string
		home, ent time.Time
	}
	// Descending; a build between two entries (an Insider build of the
	// series: 26220 is 25H2) belongs to the entry below it. Above the newest
	// entry only its own hundred counts: a newer version is unknown.
	table := []ver{
		// 26H2 (build 26300): Microsoft Lifecycle lists 29/09/2026 to
		// 10/10/2028 for Home and Pro; Enterprise and Education get 36 months.
		{26300, "26H2", d(2028, time.October, 10), d(2029, time.October, 9)},
		{26200, "25H2", d(2027, time.October, 12), d(2028, time.October, 10)},
		{26100, "24H2", d(2026, time.October, 13), d(2027, time.October, 12)},
		{22631, "23H2", d(2025, time.November, 11), d(2026, time.November, 10)},
		{22621, "22H2", d(2024, time.October, 8), d(2025, time.October, 14)},
		{22000, "21H2", d(2023, time.October, 10), d(2024, time.October, 8)},
	}
	for i, v := range table {
		upper := v.build + 100
		if i > 0 {
			upper = table[i-1].build
		}
		if build >= v.build && build < upper {
			if ent {
				return v.ent, v.name
			}
			return v.home, v.name
		}
	}
	return time.Time{}, "" // newer or unknown: nothing to say
}

func orText(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func onOff(s string) string {
	switch s {
	case "on":
		return "ligado"
	case "off":
		return "desligado"
	}
	return "desconhecido"
}

func sevRank(s string) int {
	switch s {
	case SevCritical:
		return 0
	case SevWarning:
		return 1
	case SevInfo:
		return 2
	}
	return 3
}

type rule func(f *facts.Facts, st map[string]tweaks.State, add func(Finding))

// activeHoursFinding reads the active hours of Windows Update (0.7): the
// window in which Windows does not restart for updates. Left at the
// default (8 to 17) with the automatic adjustment off, a restart can land
// in the evening, in the middle of a match or a call.
func activeHoursFinding(f *facts.Facts, add func(Finding)) {
	if f.OS.Build == 0 {
		return
	}
	start, end := f.OS.ActiveHoursStart, f.OS.ActiveHoursEnd
	set := start >= 0 && end >= 0
	if !set {
		start, end = 8, 17
	}
	win := fmt.Sprintf("das %02d:00 às %02d:00", start, end)
	src := []tweaks.Source{{Title: "Microsoft Learn — Política de horário ativo do Windows Update (ActiveHoursStart/End; padrão 8 às 17; janela de até 18 h)", URL: "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-update#activehoursstart"}}
	page := Action{Kind: "page", Label: "Definir o horário ativo", Page: "otimizacoes", Category: "update"}
	values := "não definidos no registro (o Windows usa 8 às 17)"
	if set {
		values = fmt.Sprintf("= %d/%d", start, end)
	}
	auto := map[int]string{1: "ligado", -1: "não gravado no registro (o Windows 11 vem com \"Automaticamente\" selecionado até alguém mudar)"}[f.OS.ActiveHoursAuto]
	if auto == "" {
		auto = "desligado (SmartActiveHoursState = " + strconv.Itoa(f.OS.ActiveHoursAuto) + ")"
	}
	switch {
	case f.OS.ActiveHoursPolicy:
		title := "Horário ativo do Windows Update fixado por política"
		if f.OS.ActiveHoursPolicyStart >= 0 && f.OS.ActiveHoursPolicyEnd >= 0 {
			title += fmt.Sprintf(" (das %02d:00 às %02d:00)", f.OS.ActiveHoursPolicyStart, f.OS.ActiveHoursPolicyEnd)
		}
		add(Finding{ID: "os.activehours", Area: AreaEstab, Severity: SevGood, Title: title,
			Evidence:    fmt.Sprintf("HKLM\\SOFTWARE\\Policies\\Microsoft\\Windows\\WindowsUpdate: SetActiveHours = 1, ActiveHoursStart/End = %d/%d.", f.OS.ActiveHoursPolicyStart, f.OS.ActiveHoursPolicyEnd),
			Explanation: "Uma política de grupo (comum em PC de empresa) define a janela; o valor das Configurações é ignorado e o Axion não o altera.", Sources: src})
	case f.OS.ActiveHoursAuto == 1:
		add(Finding{ID: "os.activehours", Area: AreaEstab, Severity: SevGood, Title: "Horário ativo do Windows Update ajustado automaticamente (" + win + " agora)",
			Evidence:    "SmartActiveHoursState = 1; ActiveHoursStart/End " + values + ".",
			Explanation: "O Windows move a janela pelas suas horas de uso e não reinicia dentro dela. Se ele reiniciar num horário em que você joga, defina a janela manualmente.", Actions: []Action{page}, Sources: src})
	case !set:
		add(Finding{ID: "os.activehours", Area: AreaEstab, Severity: SevInfo, Title: "Horário ativo do Windows Update não definido por você",
			Evidence:    "ActiveHoursStart/End " + values + "; ajuste automático " + auto + ".",
			Explanation: "Sem valores gravados, o Windows decide a janela: pelo uso, se o ajuste automático estiver selecionado nas Configurações (o padrão do Windows 11), ou das 8 às 17. Fora dela o PC pode reiniciar para instalar atualizações — à noite, no meio de uma partida ou de uma live. Se isso já aconteceu, defina a janela (até 18 h) com as horas em que você usa o PC.", Actions: []Action{page}, Sources: src})
	case start == 8 && end == 17:
		add(Finding{ID: "os.activehours", Area: AreaEstab, Severity: SevInfo, Title: "Horário ativo do Windows Update no padrão (" + win + ")",
			Evidence:    "ActiveHoursStart/End " + values + "; ajuste automático " + auto + ".",
			Explanation: "Fora da janela o Windows pode reiniciar o PC para instalar atualizações — à noite, no meio de uma partida ou de uma live. Coloque na janela (até 18 h) as horas em que você usa o PC, ou confira em Configurações se o ajuste automático acompanha o seu horário.", Actions: []Action{page}, Sources: src})
	default:
		add(Finding{ID: "os.activehours", Area: AreaEstab, Severity: SevGood, Title: "Horário ativo do Windows Update definido (" + win + ")",
			Evidence: "ActiveHoursStart/End " + values + "; ajuste automático " + auto + ".", Explanation: "O Windows não reinicia para atualizar dentro dessa janela.", Actions: []Action{page}, Sources: src})
	}
}

func tweakOff(st map[string]tweaks.State, id string) bool {
	s, ok := st[id]
	return ok && s.Applicable && s.Status != "on"
}

func fixTweaks(label string, ids ...string) Action {
	return Action{Kind: "tweaks", Label: label, Tweaks: ids}
}

func gib(b uint64) string {
	g := float64(b) / (1 << 30)
	if g >= 10 {
		return fmt.Sprintf("%.0f GB", g)
	}
	return strings.Replace(fmt.Sprintf("%.1f GB", g), ".", ",", 1)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func dateOr(t time.Time) string {
	if t.IsZero() {
		return "data desconhecida"
	}
	return t.Format("02/01/2006")
}

// ---------------------------------------------------------------------------
// RAM helpers
// ---------------------------------------------------------------------------

var ratedPatterns = []struct {
	re    *regexp.Regexp
	scale int
}{
	{regexp.MustCompile(`(?i)\bKF5(\d{2})C`), 100},                    // Kingston Fury DDR5: KF560C36 -> 6000
	{regexp.MustCompile(`(?i)\bKF4(\d{2})C`), 100},                    // Kingston Fury DDR4: KF436C17 -> 3600
	{regexp.MustCompile(`(?i)\bF5-(\d{4})`), 1},                       // G.Skill DDR5
	{regexp.MustCompile(`(?i)\bF4-(\d{4})`), 1},                       // G.Skill DDR4
	{regexp.MustCompile(`(?i)\bCM[A-Z]+\d+GX5M\d[A-Z](\d{4})`), 1},    // Corsair DDR5
	{regexp.MustCompile(`(?i)\bCM[A-Z]+\d+GX4M\d[A-Z](\d{4})`), 1},    // Corsair DDR4
	{regexp.MustCompile(`(?i)\bFF\w{3}\d{1,3}G(\d{4})HC`), 1},         // TeamGroup T-Force
	{regexp.MustCompile(`(?i)\bTF\w{2}D\w{0,3}\d{1,3}G(\d{4})HC`), 1}, // TeamGroup (alternate)
	{regexp.MustCompile(`(?i)\bBL\d+G(\d{2})C`), 100},                 // Crucial Ballistix: BL8G32C16 -> 3200
	{regexp.MustCompile(`(?i)\bCP\d+G(\d{2})C`), 100},                 // Crucial Pro: CP16G60C36 -> 6000
}

// RatedSpeed decodes the rated (XMP/EXPO) speed from common part numbers.
func RatedSpeed(part string) int {
	for _, p := range ratedPatterns {
		if m := p.re.FindStringSubmatch(part); m != nil {
			n, _ := strconv.Atoi(m[1])
			n *= p.scale
			if n >= 1600 && n <= 10000 {
				return n
			}
		}
	}
	return 0
}

var (
	reChanWord = regexp.MustCompile(`(?i)CHANNEL\s*([A-H])(?:\b|[-_ ]|$)`)
	reChanNum  = regexp.MustCompile(`(?i)CHANNEL\s*(\d)`)
	reDimmSlot = regexp.MustCompile(`(?i)DIMM[\s_-]?([A-H])[\s_-]?\d`)
	reSlotOnly = regexp.MustCompile(`(?i)^\s*([A-H])[1-4]\s*$`)
	reCtrl     = regexp.MustCompile(`(?i)CONTROLLER\s*(\d)`)
)

// MemChannel extracts the memory channel from SMBIOS bank/locator labels
// ("P0 CHANNEL A", "Controller1-ChannelA-DIMM0", "DIMM_B2", "A1").
// Returns "" when the labels carry no channel information.
func MemChannel(m facts.MemModule) string {
	s := m.Bank + " " + m.Locator
	ctrl := ""
	if x := reCtrl.FindStringSubmatch(s); x != nil {
		ctrl = "C" + x[1] + "-"
	}
	for _, re := range []*regexp.Regexp{reChanWord, reChanNum, reDimmSlot} {
		if x := re.FindStringSubmatch(s); x != nil {
			return ctrl + strings.ToUpper(x[1])
		}
	}
	if x := reSlotOnly.FindStringSubmatch(m.Locator); x != nil {
		return ctrl + strings.ToUpper(x[1])
	}
	return ""
}

// ---------------------------------------------------------------------------
// Rules
// ---------------------------------------------------------------------------

var srcMS = func(title, url string) tweaks.Source { return tweaks.Source{Title: title, URL: url} }

var (
	srcFsutilTrim  = tweaks.Source{Title: "Microsoft Learn — fsutil behavior: disabledeletenotify (TRIM ligado por padrão no NTFS, sem reiniciar)", URL: "https://learn.microsoft.com/windows-server/administration/windows-commands/fsutil-behavior"}
	srcDefragCmd   = tweaks.Source{Title: "Microsoft Learn — defrag: /L faz retrim; a tarefa agendada faz retrim do SSD uma vez por mês", URL: "https://learn.microsoft.com/windows-server/administration/windows-commands/defrag"}
	srcNVMeDocs    = tweaks.Source{Title: "Microsoft Learn — Trabalhando com unidades NVMe: o acesso ao log de saúde é feito pelo driver do Windows (StorNVMe.sys)", URL: "https://learn.microsoft.com/windows/win32/fileio/working-with-nvme-devices"}
	srcAutoMaint   = tweaks.Source{Title: "Microsoft Learn — Manutenção automática: inclui a desfragmentação/otimização, roda com o PC ocioso e para quando o usuário mexe no PC", URL: "https://learn.microsoft.com/windows/win32/w8cookbook/automatic-maintenance"}
	srcSSDStory    = tweaks.Source{Title: "Scott Hanselman (Microsoft), com a equipe de armazenamento do Windows — o Windows desfragmenta o SSD? (retrim e limite de fragmentação)", URL: "https://www.hanselman.com/blog/the-real-and-complete-story-does-windows-defragment-your-ssd"}
	srcNVMeServer  = tweaks.Source{Title: "Microsoft Tech Community — Native NVMe no Windows Server 2025 (anúncio oficial)", URL: "https://techcommunity.microsoft.com/blog/windowsservernewsandbestpractices/announcing-native-nvme-in-windows-server-2025-ushering-in-a-new-era-of-storage-p/4477353"}
	srcMemCompQA   = tweaks.Source{Title: "Microsoft Q&A (resposta de consultor independente, não é documentação) — desligar a compressão de memória pode aumentar o uso do arquivo de paginação e causar lentidão se a RAM encher", URL: "https://learn.microsoft.com/answers/a/12168645"}
	srcDjdNet      = tweaks.Source{Title: "djdallmann/GamingPCSetup — pesquisa de rede (NetworkThrottlingIndex desligado aumenta a latência de DPC do NDIS)", URL: "https://github.com/djdallmann/GamingPCSetup/blob/master/CONTENT/RESEARCH/NETWORK/README.md"}
	srcNVMeBlocked = tweaks.Source{Title: "WinBuzzer (24/03/2026) — Microsoft bloqueia o ajuste de Registro do driver NVMe nativo no Windows 11", URL: "https://winbuzzer.com/2026/03/24/microsoft-blocks-registry-hack-faster-nvme-driver-windows-11-xcxwbn/"}
)

// NoMemoryProfile reports a PC with no memory profile to turn on: notebooks
// and Intel boards whose chipset cannot run memory above the standard speed.
// The diagnostic and the BIOS guide use the same rule.
func NoMemoryProfile(f *facts.Facts) bool {
	return f.Power.IsLaptop || lockedMemoryChipset(f.Board.Product)
}

// lockedMemoryChipset reports Intel boards whose chipset does not allow
// memory above the CPU's standard speed: there is no XMP to turn on there.
func lockedMemoryChipset(product string) bool {
	p := strings.ToUpper(product)
	// Memory overclocking (XMP) on Intel boards: only Z chipsets before the
	// 500 series; from the 500 series also B and H570/H770, not H510/H610.
	// Intel ARK: B460 "Supports Memory Overclocking: No", B560 "Yes".
	for _, c := range []string{"H110", "B150", "H170", "Q150", "Q170", "B250", "H270", "Q250", "Q270",
		"H310", "B360", "B365", "H370", "Q370", "H410", "B460", "H470", "Q470", "H510", "H610"} {
		if strings.Contains(p, c) {
			return true
		}
	}
	return false
}

// jedecPart reports memory sold at the standard speed only (Kingston
// ValueRAM "KVR", Samsung "M3xx/M4xx", SK hynix "HMA/HMCG", Micron
// "MTA/MTC", Crucial basic "CT" without "BL"/"BLS"/"CP"...): no profile to
// turn on.
func jedecPart(mods []facts.MemModule) bool {
	if len(mods) == 0 {
		return false
	}
	for _, m := range mods {
		p := strings.ToUpper(strings.TrimSpace(m.PartNumber))
		switch {
		case strings.HasPrefix(p, "KVR"), strings.HasPrefix(p, "M3"), strings.HasPrefix(p, "M4"), strings.HasPrefix(p, "M47"),
			strings.HasPrefix(p, "HMA"), strings.HasPrefix(p, "HMCG"), strings.HasPrefix(p, "HMCT"), strings.HasPrefix(p, "MTA"), strings.HasPrefix(p, "MTC"):
		default:
			return false
		}
	}
	return true
}

var rules = []rule{
	// The active hours of Windows Update (0.7).
	func(f *facts.Facts, st map[string]tweaks.State, add func(Finding)) { activeHoursFinding(f, add) },
	// The NVIDIA driver profile database (0.9: V-Sync, frame limit, shader
	// cache, optimizer residue, VRR).
	func(f *facts.Facts, st map[string]tweaks.State, add func(Finding)) { nvdrsFindings(f, add) },
	// Axion elevated with another administrator account: per-user settings
	// are redirected to the console account (winsys/user.go); say so, since
	// the two hives can differ and the person may expect the admin's.
	func(f *facts.Facts, st map[string]tweaks.State, add func(Finding)) {
		if f.OS.ElevatedAs == "" || f.OS.User == "" {
			return
		}
		add(Finding{ID: "os.elevated.other", Area: AreaEstab, Severity: SevGood, // context, not a problem: no penalty
			Title:    "Axion aberto com outra conta de administrador",
			Evidence: "Conta em uso no PC: " + f.OS.User + " · Axion executando como: " + f.OS.ElevatedAs,
			Explanation: "As configurações por usuário (mouse, Game Bar, privacidade, inicialização) são lidas e aplicadas à conta que está usando o PC (" + f.OS.User + "), " +
				"não à conta de administrador informada na pergunta do Windows. As configurações do computador valem para todos os usuários."})
	},
	// Monitor refresh rate. The driver reports fractional rates rounded
	// down (59.94 Hz as 59, 239.76 as 239), so a difference of a few hertz
	// is not a finding.
	func(f *facts.Facts, st map[string]tweaks.State, add func(Finding)) {
		for _, d := range f.Displays {
			if d.CurrentHz <= 0 || d.MaxHzAtRes < d.CurrentHz+5 || float64(d.MaxHzAtRes) < float64(d.CurrentHz)*1.04 {
				continue
			}
			sev := SevWarning
			if d.MaxHzAtRes >= 120 && float64(d.MaxHzAtRes) >= float64(d.CurrentHz)*1.5 {
				sev = SevCritical // a 144/165/240 Hz monitor left at 60–100 Hz
			}
			why := fmt.Sprintf("Cada quadro fica na tela por %.1f ms em vez de %.1f ms: movimento mais fluido e imagem mais atual. É comum o Windows deixar monitores rápidos em 60 Hz depois de trocar cabo, driver ou monitor; confira também se o cabo e a entrada (DisplayPort/HDMI) suportam a taxa máxima.", 1000/float64(d.MaxHzAtRes), 1000/float64(d.CurrentHz))
			// A notebook's own screen is often lowered to 60 Hz on purpose to
			// save battery (by Windows or the maker's utility).
			if f.Power.IsLaptop && (d.Internal || f.Power.OnBattery) {
				if f.Power.OnBattery {
					sev = SevInfo
					why = fmt.Sprintf("O notebook está na bateria, e muitos baixam a tela para %d Hz de propósito para poupar energia. Para jogar, ligue o carregador: se continuar em %d Hz na tomada, ajuste em Configurações > Sistema > Tela > Avançado ou no utilitário do fabricante.", d.CurrentHz, d.CurrentHz)
				} else if sev == SevCritical {
					sev = SevWarning
					why += " Em notebook, o utilitário do fabricante às vezes deixa a tela em 60 Hz num modo de economia: confira o modo de desempenho dele também."
				}
			}
			add(Finding{ID: "display.refresh." + d.Device, Area: AreaLatencia, Severity: sev,
				Title:       fmt.Sprintf("%s está a %d Hz, mas suporta %d Hz", d.Monitor, d.CurrentHz, d.MaxHzAtRes),
				Evidence:    fmt.Sprintf("Modo atual %dx%d a %d Hz; o driver lista %d Hz nessa resolução.", d.Width, d.Height, d.CurrentHz, d.MaxHzAtRes),
				Explanation: why,
				Actions:     []Action{fixTweaks("Aplicar taxa máxima", "game.refresh")}})
		}
	},
	// RAM speed / XMP.
	func(f *facts.Facts, _ map[string]tweaks.State, add func(Finding)) {
		if len(f.RAM.Modules) == 0 || f.RAM.ConfiguredMTs == 0 {
			return
		}
		rated := 0
		for _, m := range f.RAM.Modules {
			if r := RatedSpeed(m.PartNumber); r > rated {
				rated = r
			}
		}
		cur := f.RAM.ConfiguredMTs
		ev := fmt.Sprintf("%s %s rodando a %d MT/s (módulo: %s %s).", gib(f.RAM.TotalBytes), f.RAM.Type, cur, f.RAM.Modules[0].Manufacturer, f.RAM.Modules[0].PartNumber)
		guide := Action{Kind: "page", Label: "Ver guia de BIOS", Page: "bios"}
		// Notebooks and boards whose chipset cannot run memory above the
		// standard speed have no profile to turn on: nothing to say there.
		switch {
		case NoMemoryProfile(f):
		case rated > 0 && rated-cur >= 200:
			ex := "Sem o perfil XMP/EXPO a memória roda na velocidade padrão JEDEC. Em jogos limitados pela CPU (como o CS2 em FPS alto) isso reduz FPS médio e 1% low. Ative o perfil na BIOS e verifique a estabilidade depois."
			if f.CPU.IsX3D {
				ex += " Nos Ryzen X3D o cache 3D reduz a dependência da memória, então o ganho tende a ser menor: confirme medindo o FPS antes e depois."
			}
			ex += " Se o perfil não ligar ou ficar instável: atualize a BIOS e tente a velocidade do kit (ou um degrau abaixo) manualmente, com as latências e a tensão da etiqueta. Se nada estabilizar, deixar na velocidade padrão é seguro; dispense este aviso."
			add(Finding{ID: "ram.xmp", Area: AreaDesempenho, Severity: SevWarning,
				Title:       fmt.Sprintf("Memória a %d MT/s, mas o kit é de %d MT/s: XMP/EXPO desligado", cur, rated),
				Evidence:    ev + fmt.Sprintf(" O código do módulo indica %d MT/s.", rated),
				Explanation: ex,
				Actions:     []Action{guide, {Kind: "page", Label: "Medir FPS antes e depois", Page: "desempenho"}}})
		case rated == 0 && !jedecPart(f.RAM.Modules) && f.RAM.Type != "" && cur <= facts.JEDECMax(f.RAM.Type) && (f.RAM.Type == "DDR5" && cur <= 4800 || f.RAM.Type == "DDR4" && cur <= 2666):
			// The kit's rating is unknown: only a hint.
			add(Finding{ID: "ram.xmp", Area: AreaDesempenho, Severity: SevInfo,
				Title:       fmt.Sprintf("Memória %s a %d MT/s: confira se o kit tem perfil XMP/EXPO", f.RAM.Type, cur),
				Evidence:    ev,
				Explanation: fmt.Sprintf("%d MT/s é a velocidade padrão JEDEC. Kits para jogos costumam ter um perfil mais rápido (XMP/EXPO) que precisa ser ligado na BIOS; kits comuns e de notebook não têm. Confira a velocidade na etiqueta do pente antes de mexer.", cur),
				Actions:     []Action{guide}})
		}
		// Channels: only when every module reports a parseable channel.
		chans := map[string]bool{}
		known := true
		for _, m := range f.RAM.Modules {
			c := MemChannel(m)
			if c == "" {
				known = false
				break
			}
			chans[c] = true
		}
		// LPDDR memory is soldered and multichannel by design, and notebooks
		// often report soldered memory as one module: no "single stick" there.
		soldered := strings.HasPrefix(strings.ToUpper(f.RAM.Type), "LPDDR")
		if len(f.RAM.Modules) == 1 && !soldered && !f.IsLaptop() {
			add(Finding{ID: "ram.single", Area: AreaDesempenho, Severity: SevWarning,
				Title:       "Apenas um pente de memória (canal único)",
				Evidence:    fmt.Sprintf("1 módulo de %s instalado.", gib(f.RAM.Modules[0].CapacityBytes)),
				Explanation: "Com um pente só, a CPU tem metade da largura de banda de memória. Em jogos limitados pela CPU a diferença para dois pentes (dual channel) costuma ser grande."})
		} else if known && len(chans) == 1 {
			var locs []string
			for _, m := range f.RAM.Modules {
				locs = append(locs, strings.TrimSpace(m.Bank+" "+m.Locator))
			}
			add(Finding{ID: "ram.samechannel", Area: AreaDesempenho, Severity: SevWarning,
				Title:       "Pentes de memória no mesmo canal",
				Evidence:    "Slots informados pela placa-mãe: " + strings.Join(locs, "; ") + ".",
				Explanation: "Dois pentes no mesmo canal funcionam como canal único (metade da largura de banda). Confira no manual da placa-mãe quais slots formam o dual channel (geralmente A2 e B2) e reinstale os pentes neles."})
		}
		if f.RAM.TotalBytes > 0 && f.RAM.TotalBytes <= 8<<30 {
			add(Finding{ID: "ram.low", Area: AreaDesempenho, Severity: SevWarning,
				Title:       "Pouca memória RAM para jogos atuais",
				Evidence:    gib(f.RAM.TotalBytes) + " instalados.",
				Explanation: "Jogos atuais somados a navegador e Discord passam de 8 GB, forçando o uso do disco (arquivo de paginação) e causando travadas. Para jogar, 16 GB é o mínimo pelo critério do Axion."})
		}
	},
	// Dual-CCD X3D requirements.
	func(f *facts.Facts, _ map[string]tweaks.State, add func(Finding)) {
		if !f.CPU.IsDualCCDX3D {
			if f.CPU.IsX3D {
				add(Finding{ID: "cpu.x3d.single", Area: AreaDesempenho, Severity: SevGood,
					Title:       f.CPU.Name + ": CCD único com 3D V-Cache",
					Evidence:    "Todos os núcleos têm acesso ao cache 3D.",
					Explanation: "Não depende de agendamento especial (Game Bar, plano Equilibrado) como os X3D de dois CCDs. Modos de BIOS que desligam o SMT (ex.: \"X3D Turbo Mode\" da Gigabyte) têm resultados mistos: ganham em alguns jogos e perdem em outros. Se testar, compare o FPS médio e o 1% low no seu jogo."})
			}
			return
		}
		if !f.Read(facts.StepGaming) || !f.Read(facts.StepPower) {
			return // the inputs of this check were not read
		}
		prob := []string{}
		if f.Gaming.X3DServiceState != "em execução" && f.Read(facts.PartX3D) {
			prob = append(prob, "serviço AMD 3D V-Cache Performance Optimizer "+f.Gaming.X3DServiceState)
		}
		if !f.Gaming.GameBarInstalled {
			prob = append(prob, "Xbox Game Bar não instalada")
		}
		if !f.Gaming.GameModeOn {
			prob = append(prob, "Modo de Jogo desligado")
		}
		if !strings.EqualFold(f.Power.ActiveScheme, sys.SchemeBalanced) {
			prob = append(prob, "plano de energia diferente do Equilibrado")
		}
		if len(prob) == 0 {
			ev := "Driver de chipset, Game Bar, Modo de Jogo e plano Equilibrado presentes."
			if !f.Read(facts.PartX3D) {
				ev = "Game Bar, Modo de Jogo e plano Equilibrado presentes (o serviço do driver de chipset não pôde ser verificado nesta análise)."
			}
			add(Finding{ID: "cpu.x3d.dual", Area: AreaDesempenho, Severity: SevGood,
				Title: "Agendamento do " + f.CPU.Name + " configurado corretamente", Evidence: ev})
			return
		}
		add(Finding{ID: "cpu.x3d.dual", Area: AreaDesempenho, Severity: SevCritical,
			Title:       "Configuração incompleta para o " + f.CPU.Name,
			Evidence:    strings.Join(prob, "; ") + ".",
			Explanation: "Nos X3D de dois CCDs o driver de chipset da AMD usa a detecção de jogos da Game Bar para estacionar o CCD sem 3D V-Cache. Sem isso os jogos podem rodar no CCD errado e perder bastante desempenho.",
			Actions:     []Action{fixTweaks("Corrigir no Windows", "game.mode", "power.plan.balanced"), {Kind: "url", Label: "Baixar driver de chipset AMD", URL: "https://www.amd.com/en/support/download/drivers.html"}}})
	},
	// Intel hybrid on Windows 10.
	func(f *facts.Facts, _ map[string]tweaks.State, add func(Finding)) {
		if f.CPU.IsIntelHybrid && !f.OS.IsWin11 && f.OS.Build > 0 {
			add(Finding{ID: "cpu.hybrid.win10", Area: AreaDesempenho, Severity: SevWarning,
				Title:       "CPU Intel híbrida no Windows 10",
				Evidence:    f.CPU.Name + " com P-cores e E-cores; Windows build " + strconv.Itoa(f.OS.Build) + ".",
				Explanation: "O Thread Director da Intel, que ajuda o Windows a mandar jogos para os núcleos de desempenho, é otimizado para o Windows 11 (segundo a Intel).",
				Sources:     []tweaks.Source{srcMS("Intel — Thread Director e sistemas operacionais suportados (artigo 000091284)", "https://www.intel.com/content/www/us/en/support/articles/000091284/processors.html")}})
		}
	},
	// Windows version / support.
	func(f *facts.Facts, _ map[string]tweaks.State, add func(Finding)) {
		now := time.Now()
		if f.OS.Build > 0 && f.OS.Build < 22000 {
			end, ltsc := win10End(f.OS.Build, f.OS.Edition)
			faceit := f.HasAntiCheat("FACEIT")
			switch {
			case now.Before(end) && !faceit:
				// LTSC still supported: nothing to report.
			case now.Before(end):
				add(Finding{ID: "os.win10", Area: AreaSeguranca, Severity: SevWarning, Title: "FACEIT: regra para o Windows 10 a partir de 14/10/2026",
					Evidence:    fmt.Sprintf("%s (%s), build %d; suporte até %s.", f.OS.Caption, f.OS.Edition, f.OS.Build, end.Format("02/01/2006")),
					Explanation: "Esta edição de longo prazo (LTSC) ainda recebe atualizações, mas a regra publicada pelo FACEIT pede, a partir de 14/10/2026, o Windows 10 versão 22H2 com as Atualizações de Segurança Estendidas (ESU) ativas, e o Windows 11 a partir de outubro de 2027. A página do FACEIT não cita as edições LTSC: confirme com o suporte do FACEIT antes da data.",
					Sources:     []tweaks.Source{srcMS("FACEIT — FAQ de requisitos do Windows", "https://support.faceit.com/hc/en-us/articles/23117181142556-Windows-Security-Requirements-FAQ")}})
			default:
				sev := SevWarning
				ex := "O suporte gratuito do Windows 10 terminou em 14/10/2025: sem atualizações de segurança fora do programa ESU."
				if ltsc {
					ex = fmt.Sprintf("O suporte desta edição de longo prazo (LTSC) terminou em %s.", end.Format("02/01/2006"))
				}
				if faceit {
					sev = SevCritical
					ex += " O FACEIT passa a aceitar, a partir de 14/10/2026, só o Windows 10 versão 22H2 com as Atualizações de Segurança Estendidas (ESU) ativas, e a exigir o Windows 11 em outubro de 2027."
					if f.OS.Build < 19045 && !ltsc {
						ex += fmt.Sprintf(" Este Windows (build %d) não é a versão 22H2: atualize pelo Windows Update ou migre para o Windows 11.", f.OS.Build)
					} else if !ltsc {
						ex += " Confira em Configurações > Windows Update se a inscrição no ESU está ativa."
					}
				}
				add(Finding{ID: "os.win10", Area: AreaSeguranca, Severity: sev, Title: "Windows 10 fora do suporte",
					Evidence: fmt.Sprintf("%s, build %d.", f.OS.Caption, f.OS.Build), Explanation: ex,
					Sources: []tweaks.Source{srcMS("Microsoft — Ciclo de vida do Windows 10", "https://learn.microsoft.com/lifecycle/products/windows-10-home-and-pro"),
						srcMS("FACEIT — FAQ de requisitos do Windows", "https://support.faceit.com/hc/en-us/articles/23117181142556-Windows-Security-Requirements-FAQ")}})
			}
		}
		if f.OS.Build >= 22000 {
			if end, ver := win11End(f.OS.Build, f.OS.Edition); !end.IsZero() && now.After(end) {
				add(Finding{ID: "os.win11.eos", Area: AreaSeguranca, Severity: SevWarning, Title: "Versão " + ver + " do Windows 11 sem atualizações de segurança",
					Evidence:    fmt.Sprintf("%s %s (build %d); o suporte desta versão terminou em %s.", f.OS.Caption, ver, f.OS.Build, end.Format("02/01/2006")),
					Explanation: "Cada versão anual do Windows 11 recebe atualizações por um tempo limitado. Atualize para a versão mais nova em Configurações > Windows Update (a atualização mantém arquivos e programas).",
					Actions:     []Action{{Kind: "url", Label: "Abrir o Windows Update", URL: "ms-settings:windowsupdate"}},
					Sources:     []tweaks.Source{srcMS("Microsoft — Ciclo de vida do Windows 11 Home e Pro", "https://learn.microsoft.com/lifecycle/products/windows-11-home-and-pro"), srcMS("Microsoft — Ciclo de vida do Windows 11 Enterprise e Education", "https://learn.microsoft.com/lifecycle/products/windows-11-enterprise-and-education")}})
			}
		}
		if f.OS.PendingReboot {
			add(Finding{ID: "os.reboot", Area: AreaEstab, Severity: SevWarning, Title: "Reinício pendente",
				Evidence: "O Windows Update ou a manutenção de componentes aguardam um reinício.", Explanation: "Atualizações pela metade podem causar lentidão e falhas. Reinicie antes de jogar ou otimizar."})
		}
		if f.OS.UptimeHours > 24*7 {
			title := fmt.Sprintf("PC ligado há %.0f dias sem reiniciar", f.OS.UptimeHours/24)
			ex := "Reiniciar de vez em quando libera memória presa por drivers e conclui atualizações."
			if f.Power.FastStartupOn {
				// With Fast Startup, "Desligar" hibernates the kernel: the
				// boot time does not change even if the PC is shut down daily.
				title = fmt.Sprintf("%.0f dias sem um reinício completo", f.OS.UptimeHours/24)
				ex = "Com a Inicialização Rápida ligada, \"Desligar\" não reinicia o Windows por completo (o sistema hiberna), então este tempo conta mesmo se você desliga o PC todo dia. Use \"Reiniciar\" de vez em quando: libera memória presa por drivers e conclui atualizações."
			}
			add(Finding{ID: "os.uptime", Area: AreaEstab, Severity: SevInfo, Title: title,
				Evidence: "Último boot completo: " + f.OS.LastBoot.Format("02/01/2006 15:04") + ".", Explanation: ex})
		}
	},
	// Storage.
	func(f *facts.Facts, _ map[string]tweaks.State, add func(Finding)) {
		for _, d := range f.Disks {
			name := fmt.Sprintf("%s (%s %s)", d.Model, d.BusType, d.MediaType)
			// Windows also reports "Warning" for an NVMe drive that is only
			// hot; when the health log says temperature is the sole alert,
			// the temperature finding covers it.
			tempOnly := d.NVMe != nil && d.NVMe.CriticalWarning != 0 && d.NVMe.CriticalWarning&^sys.NVMeWarnTemperature == 0
			if d.Health != "Healthy" && d.Health != "Unknown" && d.Health != "" && !tempOnly {
				add(Finding{ID: "disk.health." + strconv.Itoa(d.Number), Area: AreaArmaz, Severity: SevCritical, Title: "Disco com saúde comprometida: " + d.Model,
					Evidence:    "Estado reportado pelo Windows: " + d.Health + ".",
					Explanation: "O Windows marca a unidade assim por sinais de falha (setores ruins, área reserva esgotada, modo somente leitura). Faça backup imediatamente e planeje trocar o disco."})
			}
			num := strconv.Itoa(d.Number)
			wearEv := name + fmt.Sprintf(", %d horas ligado.", d.PowerOnHours)
			if h := d.NVMe; h != nil {
				wearEv = fmt.Sprintf("%s. Log SMART/Saúde NVMe: Percentage Used = %d%% (estimativa do fabricante), %s gravados, %s horas ligado.",
					name, h.PercentUsed, tb(h.DataWrittenBytes), thousands(h.PowerOnHours))
			}
			wearTitle := fmt.Sprintf("SSD com %d%% da vida útil consumida", d.WearPct)
			if d.WearPct >= 100 {
				wearTitle = fmt.Sprintf("SSD além da vida útil nominal (%d%%)", d.WearPct)
			}
			if d.WearPct >= 90 {
				add(Finding{ID: "disk.wear." + num, Area: AreaArmaz, Severity: SevCritical, Title: wearTitle,
					Evidence: wearEv, Explanation: "Perto do fim (ou além) da resistência nominal de gravação o SSD pode ficar lento, perder dados ou entrar em somente leitura. Faça backup e planeje a troca."})
			} else if d.WearPct >= 70 {
				add(Finding{ID: "disk.wear." + num, Area: AreaArmaz, Severity: SevWarning, Title: wearTitle,
					Evidence: wearEv, Explanation: "Ainda funciona, mas já passou da maior parte da resistência nominal de gravação. Mantenha backup e planeje a troca por um modelo com DRAM e TBW maior. Avisar a partir de 70% (e marcar como importante a partir de 90%) é critério do Axion."})
			}
			if h := d.NVMe; h != nil {
				nvmeFindings(f, d, h, name, num, add)
			}
			if d.TempC >= 70 && (d.NVMe == nil || d.NVMe.CriticalWarning&sys.NVMeWarnTemperature == 0) {
				add(Finding{ID: "disk.temp." + strconv.Itoa(d.Number), Area: AreaArmaz, Severity: SevWarning, Title: fmt.Sprintf("SSD a %d °C", d.TempC),
					Evidence: name + ".", Explanation: "Acima de ~70 °C muitos SSDs reduzem a velocidade (throttling). Um dissipador ou mais fluxo de ar resolve."})
			}
			if d.IsSystem && d.MediaType != "HDD" && d.WearPct < 0 && d.NVMe == nil {
				advice := storageDriverAdvice(f)
				ex := "Sem esse log o Axion não sabe quanto da vida útil já foi consumida. O próprio Windows mostra isso em Configurações > Sistema > Armazenamento > Discos e volumes > Propriedades (Integridade da unidade: vida útil restante estimada). Em SSDs sem DRAM e muito usados, acompanhe esse número e mantenha backup."
				if advice != "" {
					// With a vendor driver Windows reads the same (empty)
					// counters, so its health screen does not help either.
					ex = "Sem esse log o Axion não sabe quanto da vida útil já foi consumida, e com o driver do fabricante a tela de integridade do Windows também fica sem dados. " +
						"Veja a vida útil na ferramenta do fabricante do SSD" + ssdTool(d.Model) + ". Em SSDs sem DRAM e muito usados, acompanhe esse número e mantenha backup."
				}
				add(Finding{ID: "disk.health.unknown." + num, Area: AreaArmaz, Severity: SevInfo, Title: "Desgaste do SSD do sistema não pôde ser lido: " + d.Model,
					Evidence:    "O driver do disco não entregou o log de saúde (SMART) ao Axion" + healthErr(f, d.Number) + "." + storageDriverNote(f),
					Explanation: ex + advice,
					Sources:     []tweaks.Source{srcNVMeDocs}})
			}
			if d.IsSystem && d.MediaType == "HDD" {
				add(Finding{ID: "disk.hdd", Area: AreaArmaz, Severity: SevCritical, Title: "Windows instalado em HD mecânico",
					Evidence: name + ".", Explanation: "É o maior gargalo possível em carregamentos e travadas de jogos que leem dados durante a partida. Migrar para um SSD é a melhoria mais perceptível."})
			}
		}
		if v := f.SystemVolume(); v != nil && v.SizeBytes > 0 {
			pct := float64(v.FreeBytes) / float64(v.SizeBytes) * 100
			if v.FreeBytes < 15<<30 || pct < 10 {
				add(Finding{ID: "disk.free", Area: AreaArmaz, Severity: SevWarning, Title: fmt.Sprintf("Pouco espaço livre em %s (%s, %.0f%%)", v.Letter, gib(v.FreeBytes), pct),
					Evidence:    fmt.Sprintf("%s livres de %s.", gib(v.FreeBytes), gib(v.SizeBytes)),
					Explanation: "Atualizações do Windows, cache de shaders e o arquivo de paginação precisam de espaço; SSDs quase cheios também ficam mais lentos para gravar.",
					Actions:     []Action{{Kind: "page", Label: "Abrir Limpeza", Page: "limpeza"}}})
			}
		}
		if f.TempBytes > 2<<30 {
			add(Finding{ID: "disk.temp.files", Area: AreaArmaz, Severity: SevInfo, Title: "Arquivos temporários acumulados: " + gib(f.TempBytes),
				Evidence: "Pastas TEMP do usuário e do Windows.", Actions: []Action{{Kind: "page", Label: "Abrir Limpeza", Page: "limpeza"}}})
		}
		trimFindings(f, add)
	},
	// GPU driver and state.
	func(f *facts.Facts, st map[string]tweaks.State, add func(Finding)) {
		for _, g := range f.GPUs {
			if g.BasicDriver && !g.Virtual() {
				who := "do fabricante"
				if g.Vendor != "Other" {
					who = "da " + g.Vendor
				}
				add(Finding{ID: "gpu.nodriver", Area: AreaDesempenho, Severity: SevCritical, Title: "Placa de vídeo sem driver instalado",
					Evidence:    fmt.Sprintf("%s (driver genérico do Windows, %s; dispositivo %s).", g.Name, g.InfFile, g.PNPDeviceID),
					Explanation: "Com o driver básico do Windows não há aceleração 3D adequada nem as taxas de atualização altas do monitor. Instale o driver " + who + " (página Drivers). Acontece depois de usar o DDU, de uma atualização que falhou ou quando o Windows Update está impedido de instalar drivers.",
					Actions:     []Action{{Kind: "page", Label: "Abrir Drivers", Page: "drivers"}}})
				continue
			}
			if g.Integrated || g.DriverDate.IsZero() || g.Vendor == "Other" || g.Virtual() {
				continue // virtual adapters and unknown vendors have no update to offer
			}
			age := time.Since(g.DriverDate)
			if age > 180*24*time.Hour {
				ver := g.DriverVersion
				if g.VendorVersion != "" {
					ver = g.VendorVersion
				}
				where := "A página Drivers compara com a versão mais nova da NVIDIA e baixa o instalador oficial com a assinatura verificada."
				if !strings.EqualFold(g.Vendor, "NVIDIA") {
					where = "Baixe o driver mais novo no site do fabricante (" + g.Vendor + "); a página Drivers mostra o que o Windows Update oferece."
				}
				add(Finding{ID: "gpu.driver.old", Area: AreaDesempenho, Severity: SevWarning, Title: fmt.Sprintf("Driver de vídeo com %d meses", int(age.Hours()/24/30)),
					Evidence:    fmt.Sprintf("%s — driver %s de %s.", g.Name, ver, g.DriverDate.Format("02/01/2006")),
					Explanation: "Drivers novos trazem correções de desempenho e estabilidade para jogos recentes. " + where + " Avisar a partir de 6 meses é critério do Axion.",
					Actions:     []Action{{Kind: "page", Label: "Abrir Drivers", Page: "drivers"}}})
			}
		}
		pcieFinding(f, add)
		if f.Nvidia.ReBAR == "off" {
			add(Finding{ID: "gpu.rebar", Area: AreaDesempenho, Severity: SevInfo, Title: "Resizable BAR desligado",
				Evidence:    fmt.Sprintf("BAR1 = %d MiB (com ReBAR ativo ficaria do tamanho da VRAM).", f.Nvidia.BAR1MiB),
				Explanation: rebarText(f),
				Actions:     []Action{{Kind: "page", Label: "Ver guia de BIOS", Page: "bios"}}})
		}
		if tweakOff(st, "game.hags") && st["game.hags"].Rec.Level == tweaks.RecYes {
			ev := "HwSchMode ≠ 2."
			if f.Gaming.HAGSState == "off" {
				ev = "O kernel gráfico informa HAGS desligado (a GPU e o driver suportam)."
			}
			add(Finding{ID: "gpu.hags", Area: AreaDesempenho, Severity: SevWarning, Title: "Agendamento de GPU por hardware desligado",
				Evidence: ev, Explanation: st["game.hags"].Rec.Reason + ".", Actions: []Action{fixTweaks("Ativar HAGS", "game.hags")}})
		}
		if len(f.PnPProblems) > 0 {
			var names []string
			for _, p := range f.PnPProblems {
				names = append(names, fmt.Sprintf("%s (código %d)", p.Name, p.Code))
			}
			add(Finding{ID: "pnp.problems", Area: AreaEstab, Severity: SevWarning, Title: plural(len(f.PnPProblems), "dispositivo com problema de driver", "dispositivos com problema de driver"),
				Evidence: strings.Join(names, "; ") + ".", Explanation: "Dispositivos com erro podem causar travadas (DPC latency) e falhas. Instale os drivers de chipset/placa-mãe do fabricante.",
				Actions: []Action{{Kind: "url", Label: "Abrir Gerenciador de Dispositivos", URL: "tool:devmgmt"}}})
		}
	},
	// Gaming settings.
	func(f *facts.Facts, st map[string]tweaks.State, add func(Finding)) {
		if !f.Read(facts.StepGaming) {
			return
		}
		if !f.Gaming.GameModeOn {
			add(Finding{ID: "game.mode", Area: AreaDesempenho, Severity: SevWarning, Title: "Modo de Jogo desligado",
				Evidence: "AutoGameModeEnabled = 0.", Explanation: "O Modo de Jogo prioriza o jogo e bloqueia instalações de driver e avisos do Windows Update durante a partida.",
				Actions: []Action{fixTweaks("Ligar Modo de Jogo", "game.mode")}})
		}
		if f.Gaming.HistoricalRecord {
			add(Finding{ID: "game.dvr", Area: AreaDesempenho, Severity: SevWarning, Title: "Gravação contínua em segundo plano ligada",
				Evidence: "HistoricalCaptureEnabled = 1 (Xbox Game Bar).", Explanation: "A gravação \"registrar o que aconteceu\" codifica vídeo o tempo todo enquanto você joga.",
				Actions: []Action{fixTweaks("Desligar gravação contínua", "game.dvr.history")}})
		}
		if f.Gaming.MouseAccelOn {
			add(Finding{ID: "input.accel", Area: AreaLatencia, Severity: SevInfo, Title: "Aceleração do ponteiro ligada",
				Evidence: "\"Aprimorar precisão do ponteiro\" ativo (MouseSpeed = 1).", Explanation: "O CS2 usa raw input e não é afetado, mas menus, desktop e jogos sem raw input sofrem com a aceleração.",
				Actions: []Action{fixTweaks("Desligar aceleração", "input.mouse.accel")}})
		}
		if f.Boot.On(f.Boot.UsePlatformClock) {
			add(Finding{ID: "boot.hpet", Area: AreaLatencia, Severity: SevWarning, Title: "HPET forçado na inicialização (useplatformclock)",
				Evidence:    "bcdedit {current}: useplatformclock = " + f.Boot.UsePlatformClock + ".",
				Explanation: "Deixado por \"otimizadores\" antigos: força uma fonte de tempo mais lenta que o TSC, aumentando o custo de cada leitura de tempo feita pelos jogos.",
				Actions:     []Action{fixTweaks("Remover", "game.hpet.fix")}})
		}
		if f.Boot.On(f.Boot.DisableDynamicTick) || f.Boot.On(f.Boot.UsePlatformTick) {
			var set []string
			if f.Boot.On(f.Boot.DisableDynamicTick) {
				set = append(set, "disabledynamictick = "+f.Boot.DisableDynamicTick)
			}
			if f.Boot.On(f.Boot.UsePlatformTick) {
				set = append(set, "useplatformtick = "+f.Boot.UsePlatformTick)
			}
			add(Finding{ID: "boot.tick", Area: AreaLatencia, Severity: SevInfo, Title: "Parâmetros de tick não padrão na inicialização",
				Evidence: "bcdedit {current}: " + strings.Join(set, ", ") + ".", Explanation: "Sem ganho comprovado e com mais consumo de energia.",
				Actions: []Action{fixTweaks("Restaurar padrão", "game.boottick.fix")}})
		}
	},
	// Power.
	func(f *facts.Facts, st map[string]tweaks.State, add func(Finding)) {
		// (The plan-wide USB selective suspend is not a finding: Windows
		// already keeps mice and keyboards out of it unless their driver
		// opts in, and that case is reported per device, "per.suspend".)
		if f.Power.IsLaptop && f.Power.OnBattery {
			ev := "O notebook está na bateria."
			if f.Power.BatteryPercent >= 0 {
				ev = fmt.Sprintf("Bateria em %d%%.", f.Power.BatteryPercent)
			}
			add(Finding{ID: "power.battery", Area: AreaDesempenho, Severity: SevInfo, Title: "Notebook na bateria",
				Evidence: ev, Explanation: "Na bateria CPU e GPU limitam a potência. Jogue na tomada."})
		}
		if dups := duplicatePlans(f.Power.Schemes); len(dups) > 0 {
			add(Finding{ID: "power.plans.dup", Area: AreaDesempenho, Severity: SevInfo, Title: "Planos de energia repetidos na lista",
				Evidence: "Cópias do mesmo plano: " + strings.Join(dups, ", ") + ". Ativo: " + f.Power.ActiveName + ".",
				Explanation: "Scripts de otimização criam uma cópia nova a cada execução. Só o plano ativo vale: as cópias não mudam o desempenho, apenas poluem a lista. " +
					"Para limpar, em Opções de Energia escolha a cópia > Alterar configurações do plano > Excluir este plano (o plano ativo não pode ser excluído)."})
		}
	},
	// Security / anti-cheat.
	func(f *facts.Facts, st map[string]tweaks.State, add func(Finding)) {
		if !f.Read(facts.StepSecurity) || !f.Read(facts.StepGaming) {
			return // TPM/Secure Boot/VBS or the anti-cheats were not read
		}
		s := f.Security
		acs := strings.Join(f.Gaming.AntiCheats, ", ")
		if f.RequiresVBS() {
			// TPM 2.0 and Secure Boot are enforced for every FACEIT player
			// since 25/11/2025; VBS (for IOMMU) is being required in waves,
			// not from everyone: missing VBS alone is a dismissible warning.
			// What could not be read is not "missing": a WMI query that timed
			// out must not turn into "requirements not met". Vanguard asks for
			// TPM 2.0 and Secure Boot only on Windows 11.
			var hard []string
			strict := f.HasAntiCheat("FACEIT") || (f.HasAntiCheat("Vanguard") && f.OS.Build >= 22000)
			if strict && s.TPMRead && (!s.TPMReady || (s.TPMVersion != "" && !strings.HasPrefix(strings.TrimSpace(s.TPMVersion), "2"))) {
				hard = append(hard, "TPM 2.0")
			}
			if strict && (s.SecureBoot == "off" || s.SecureBoot == "unsupported") {
				hard = append(hard, "Secure Boot")
			}
			srcs := []tweaks.Source{srcMS("FACEIT — Perguntas frequentes sobre os requisitos de segurança do Windows", "https://support.faceit.com/hc/en-us/articles/23117181142556-Windows-Security-Requirements-FAQ"),
				srcMS("FACEIT — Problemas conhecidos dos requisitos do anti-cheat (Integridade de Memória em alguns casos)", "https://support.faceit.com/hc/en-us/articles/22851956652956-Known-issues-with-Anti-Cheat-Requirements"),
				srcMS("Riot — Requisitos de segurança do Vanguard", "https://support.riotgames.com/en-us/riot/performance/vanguard-security-requirements")}
			if len(hard) > 0 {
				ex := "O FACEIT exige TPM 2.0 e Secure Boot de todos os jogadores desde 25/11/2025; o Vanguard pode exigir os dois. Ative na BIOS o fTPM (AMD) ou PTT (Intel) e o Secure Boot (com o CSM desligado) — o guia de BIOS mostra onde." + mbrCaution(f)
				add(Finding{ID: "sec.anticheat", Area: AreaSeguranca, Severity: SevCritical, Title: "Requisitos do anti-cheat não atendidos: " + strings.Join(hard, ", "),
					Evidence:    fmt.Sprintf("Anti-cheat instalado: %s. TPM: %s; Secure Boot: %s.", acs, orText(s.TPMVersion, "não encontrado"), onOff(s.SecureBoot)),
					Explanation: ex,
					Actions:     []Action{{Kind: "page", Label: "Ver guia de BIOS", Page: "bios"}},
					Sources:     srcs})
			}
			if !s.VBSRunning {
				add(Finding{ID: "sec.anticheat.vbs", Area: AreaSeguranca, Severity: SevWarning, Title: "VBS desligado: o anti-cheat pode pedir para ligar",
					Evidence:    "Anti-cheat instalado: " + acs + ". Segurança baseada em virtualização (VBS) não está em execução.",
					Explanation: "O FACEIT passou a exigir VBS (para o IOMMU) em etapas, não de todos os jogadores, e em alguns casos pede a Integridade de Memória. Se o cliente do FACEIT não reclamar, pode dispensar este aviso; se pedir, ative a virtualização da CPU (SVM/VT-x) e o IOMMU na BIOS e a Isolamento de Núcleo no Windows.",
					Actions:     []Action{{Kind: "page", Label: "Ver guia de BIOS", Page: "bios"}},
					Sources:     srcs})
			}
			iommuOff := s.IOMMU == "off" && f.HasAntiCheat("FACEIT")
			if iommuOff {
				add(Finding{ID: "sec.anticheat.iommu", Area: AreaSeguranca, Severity: SevWarning, Title: "IOMMU aparentemente desligado: o FACEIT pode pedir para ligar",
					Evidence:    "Anti-cheat instalado: " + acs + ". O Windows informa que a proteção de DMA não está disponível (Win32_DeviceGuard), o que normalmente significa IOMMU/VT-d (Intel) ou AMD-Vi (AMD) desligado na BIOS.",
					Explanation: "O FACEIT passou a exigir IOMMU junto com o VBS em etapas, não de todos os jogadores. Se o cliente do FACEIT não reclamar, pode dispensar este aviso; se pedir, ligue o IOMMU/VT-d na BIOS — o guia de BIOS mostra onde.",
					Actions:     []Action{{Kind: "page", Label: "Ver guia de BIOS", Page: "bios"}},
					Sources:     srcs})
			}
			if len(hard) == 0 && s.VBSRunning && !iommuOff {
				add(Finding{ID: "sec.anticheat", Area: AreaSeguranca, Severity: SevGood, Title: "Requisitos do anti-cheat atendidos",
					Evidence:    fmt.Sprintf("%s: TPM %s, Secure Boot ligado, VBS em execução%s.", acs, s.TPMVersion, iommuNote(s)),
					Explanation: "Por isso o Axion bloqueia qualquer ajuste que desligue VBS/Integridade de Memória neste PC."})
			}
		} else if s.VBSRunning || s.HVCIRunning {
			vbsFinding(f, st, add)
		}
		// Gamers Club (Anti-Cheat 2.0, March 2026): Secure Boot for every
		// player; TPM 2.0 and VBS still in a test phase, so only Secure Boot
		// is a hard requirement here.
		if f.HasAntiCheat("Gamers Club") && !f.HasAntiCheat("FACEIT", "Vanguard") {
			srcGC := srcMS("Draft5 — Gamers Club anuncia o Anti-Cheat 2.0: Secure Boot para todos; TPM 2.0 e VBS em teste (12/03/2026)", "https://draft5.gg/noticia/gamers-club-anuncia-melhorias-no-anti-cheat-2-0")
			if s.SecureBoot != "on" {
				ex := "A Gamers Club exige o Secure Boot de todos os jogadores desde março de 2026 (TPM 2.0 e VBS ainda em teste). Ative na BIOS, com o CSM desligado — o guia de BIOS mostra onde." + mbrCaution(f)
				add(Finding{ID: "sec.anticheat.gc", Area: AreaSeguranca, Severity: SevCritical, Title: "Requisito da Gamers Club não atendido: Secure Boot",
					Evidence:    "Anti-cheat instalado: Gamers Club. Secure Boot: " + onOff(s.SecureBoot) + ".",
					Explanation: ex,
					Actions:     []Action{{Kind: "page", Label: "Ver guia de BIOS", Page: "bios"}}, Sources: []tweaks.Source{srcGC}})
			} else {
				add(Finding{ID: "sec.anticheat.gc", Area: AreaSeguranca, Severity: SevGood, Title: "Requisito da Gamers Club atendido",
					Evidence:    fmt.Sprintf("Gamers Club: Secure Boot ligado; TPM %s; VBS %s.", orText(s.TPMVersion, "não encontrado"), map[bool]string{true: "em execução", false: "desligado"}[s.VBSRunning]),
					Explanation: "Secure Boot é a exigência atual da Gamers Club para todos; TPM 2.0 e VBS estão em fase de teste e podem passar a ser pedidos.", Sources: []tweaks.Source{srcGC}})
			}
		}
		if s.SecureBoot == "off" && f.OS.IsWin11 && !f.RequiresVBS() && !f.HasAntiCheat("Gamers Club") {
			add(Finding{ID: "sec.secureboot", Area: AreaSeguranca, Severity: SevInfo, Title: "Secure Boot desligado",
				Evidence: "UEFISecureBootEnabled = 0.", Explanation: "Vários anti-cheats (Vanguard, EA Javelin, FACEIT) exigem Secure Boot. Ative na BIOS se for jogar esses títulos."})
		}
		// With another antivirus registered, Defender steps aside on purpose
		// (passive mode): nothing to report.
		if s.DefenderRealtime == "off" && f.Health.Defender.ThirdParty == "" && !strings.EqualFold(f.Health.Defender.Mode, "Passive") {
			add(Finding{ID: "sec.defender", Area: AreaSeguranca, Severity: SevInfo, Title: "Proteção em tempo real do Defender desligada",
				Evidence: "RealTimeProtectionEnabled = falso (ou outro antivírus ativo).", Explanation: "Se você não usa outro antivírus, religue. Para jogos, uma exclusão de pasta é a alternativa segura."})
		}
	},
	// Known vulnerable kernel drivers left by other programs (read only).
	func(f *facts.Facts, st map[string]tweaks.State, add func(Finding)) {
		if !f.Read(facts.StepVulnDrivers) || !f.VulnDrivers.Read || len(f.VulnDrivers.Found) == 0 {
			return // not read, or nothing found: silent
		}
		vulnDriverFinding(f.VulnDrivers.Found, add)
	},
	// Network.
	func(f *facts.Facts, st map[string]tweaks.State, add func(Finding)) {
		if tweakOff(st, "net.tcp.defaults") {
			add(Finding{ID: "net.tcp", Area: AreaRede, Severity: SevWarning, Title: "Configuração TCP alterada (autotuning/RSS)",
				Evidence:    fmt.Sprintf("autotuning=%s, RSS=%s.", f.Net.TCPAutoTuning, f.Net.RSS),
				Explanation: "Provavelmente alterado por um \"otimizador de ping\". Limita downloads e não reduz o ping.", Actions: []Action{fixTweaks("Restaurar padrão", "net.tcp.defaults")}})
		}
		// Driver settings on every adapter, connected or not: a fixed speed
		// on a cable unplugged today matters the day it is plugged in.
		speedFinding(f, add)
		nicDefaultsFinding(f, add)
		a := f.ActiveAdapter()
		if a == nil {
			return
		}
		if a.Kind == "Wi-Fi" {
			add(Finding{ID: "net.wifi", Area: AreaRede, Severity: SevInfo, Title: "Jogando pelo Wi-Fi",
				Evidence:    a.Description + fmt.Sprintf(" a %d Mb/s.", a.LinkSpeedMbps),
				Explanation: "O Wi-Fi adiciona variação de latência (jitter) e perda de pacotes por interferência. Um cabo de rede é a melhoria de rede mais efetiva; se não for possível, use 5/6 GHz perto do roteador."})
		}
		if a.Kind == "Ethernet" && a.LinkSpeedMbps > 0 && a.LinkSpeedMbps < 1000 && a.GigabitCapable() {
			add(Finding{ID: "net.link100", Area: AreaRede, Severity: SevWarning, Title: fmt.Sprintf("Placa gigabit conectada a %d Mb/s", a.LinkSpeedMbps),
				Evidence: a.Description + ".", Explanation: "O gigabit usa os quatro pares do cabo: com um par ruim, um cabo antigo (Cat5) ou um conector mal crimpado, o link cai para 100 Mb/s. " +
					"Troque por um cabo Cat5e/Cat6 e teste outra porta. Se a porta do roteador ou do switch for de 100 Mb/s, o limite é dela."})
		}
		if kw, ok := a.Keywords["*EEE"]; ok && kw.Current == "1" {
			add(Finding{ID: "net.eee", Area: AreaRede, Severity: SevInfo, Title: "Economia de energia da placa de rede (EEE) ligada",
				Evidence:    a.Description + ": Energy-Efficient Ethernet (*EEE) = 1.",
				Explanation: "O EEE (IEEE 802.3az) desliga o link entre um pacote e outro. Acordar leva cerca de 16,5 µs no gigabit, nada perto da latência da internet; o motivo para desligar é compatibilidade: a própria Intel desativou o EEE nas placas I225/I226 para acabar com quedas e travadas. Num desktop, desligar não tem desvantagem além de um consumo um pouco maior com a rede parada.",
				Actions:     []Action{fixTweaks("Desligar EEE", "net.nic.eee")}, Sources: []tweaks.Source{srcEEEIntel, srcEEEWake}})
		}
	},
	// Background processes.
	func(f *facts.Facts, _ map[string]tweaks.State, add func(Finding)) {
		var heavy []string
		cats := map[string]bool{}
		for _, p := range f.Processes {
			m := p.Mem()
			if p.Category == "rgb" || (p.Category == "browser" && m > 1<<30) || p.Category == "overlay" && m > 300<<20 {
				heavy = append(heavy, fmt.Sprintf("%s (%s)", p.Name, gib(m)))
				cats[p.Category] = true
			}
		}
		if len(heavy) > 0 {
			var why []string
			if cats["rgb"] {
				why = append(why, "Programas de RGB e periféricos consultam o hardware o tempo todo; alguns já foram ligados a micro-travadas, e o efeito varia por versão: se notar engasgos, feche e compare.")
			}
			if cats["browser"] {
				why = append(why, "Navegadores abertos disputam CPU e memória com o jogo, principalmente com vídeos ou muitas abas.")
			}
			if cats["overlay"] {
				why = append(why, "Overlays e gravação usam a GPU em segundo plano.")
			}
			why = append(why, "Feche o que não usa antes de jogar (página Processos). A memória mostrada é a exclusiva de cada programa.")
			add(Finding{ID: "proc.background", Area: AreaDesempenho, Severity: SevInfo, Title: "Programas pesados em segundo plano",
				Evidence: strings.Join(heavy, "; ") + ".", Explanation: strings.Join(why, " "),
				Actions: []Action{{Kind: "page", Label: "Ver processos", Page: "processos"}}})
		}
		if f.Startup.Enabled > 8 {
			add(Finding{ID: "startup.many", Area: AreaDesempenho, Severity: SevInfo, Title: fmt.Sprintf("%d programas iniciam com o Windows", f.Startup.Enabled),
				Evidence:    "Chaves Run do usuário e da máquina.",
				Explanation: "Cada programa da inicialização deixa o Windows mais lento para ficar pronto e muitos continuam abertos em segundo plano. A página Inicialização mostra quais dá para desligar sem perder nada. Avisar a partir de 9 é critério do Axion.",
				Actions:     []Action{{Kind: "page", Label: "Gerenciar inicialização", Page: "inicializacao"}}})
		}
	},
	// Firmware issues with vendor-confirmed fixes.
	func(f *facts.Facts, _ map[string]tweaks.State, add func(Finding)) {
		if f.CPU.RaptorDie() {
			mc := f.CPU.Microcode
			sig := ""
			if g := f.CPU.Signature(); g != "" {
				sig = " (CPUID " + g + ")"
			}
			switch {
			case mc < 0x12B:
				add(Finding{ID: "cpu.intel.vmin", Area: AreaEstab, Severity: SevCritical, Title: fmt.Sprintf("Microcode 0x%X: atualize a BIOS (instabilidade Intel 13ª/14ª geração)", mc),
					Evidence:    fmt.Sprintf("%s%s com microcode 0x%X; BIOS %s de %s.", f.CPU.Name, sig, mc, f.Board.BIOSVersion, dateOr(f.Board.BIOSDate)),
					Explanation: "A Intel confirmou que processadores desktop de 13ª e 14ª geração sofrem degradação (Vmin shift) por tensão elevada, e que a correção é o microcode 0x12B ou mais novo, entregue por atualização de BIOS. Atualize a BIOS pelo site da fabricante da placa-mãe; se o PC já trava ou dá tela azul sem motivo, a atualização pode não bastar e vale acionar a garantia do processador.",
					Actions:     []Action{{Kind: "page", Label: "Ver guia de BIOS", Page: "bios"}},
					Sources:     []tweaks.Source{srcMS("Intel — Causa raiz da instabilidade da 13ª/14ª geração (0x12B)", "https://community.intel.com/t5/Blogs/Tech-Innovation/Client/Intel-Core-13th-and-14th-Gen-Desktop-Instability-Root-Cause/post/1633239")}})
			case mc >= 0x12B && mc < 0x12F:
				add(Finding{ID: "cpu.intel.vmin", Area: AreaEstab, Severity: SevInfo, Title: fmt.Sprintf("Microcode 0x%X: existe revisão mais nova (0x12F)", mc),
					Evidence:    f.CPU.Name + sig + ".",
					Explanation: "Você já tem a correção principal (0x12B). A Intel publicou depois o 0x12F para casos adicionais de Vmin shift; vem em BIOS mais novas.",
					Sources:     []tweaks.Source{srcMS("Intel — Atualização de microcode 0x12F", "https://community.intel.com/t5/Mobile-and-Desktop-Processors/Intel-Core-13th-and-14th-Gen-Vmin-Shift-Instabilty-Update-New/m-p/1686948")}})
			}
		}
		if f.CPU.IsAM5() && !f.Board.BIOSDate.IsZero() && f.Board.BIOSDate.Before(time.Date(2023, 5, 1, 0, 0, 0, 0, time.UTC)) {
			sev := SevWarning
			if f.CPU.IsX3D {
				sev = SevCritical // the damaged processors reported were X3D with EXPO
			}
			add(Finding{ID: "cpu.am5.soc", Area: AreaEstab, Severity: sev, Title: "BIOS anterior ao limite de tensão SoC do Ryzen 7000",
				Evidence:    fmt.Sprintf("%s; BIOS %s de %s.", f.CPU.Name, f.Board.BIOSVersion, dateOr(f.Board.BIOSDate)),
				Explanation: "Em abril de 2023, depois de casos de processadores danificados com EXPO (principalmente X3D), a AMD passou a limitar por BIOS a tensão SoC dos Ryzen 7000 a 1,3 V. Atualize a BIOS antes de usar perfis de memória.",
				Actions:     []Action{{Kind: "page", Label: "Ver guia de BIOS", Page: "bios"}},
				Sources:     []tweaks.Source{srcMS("Tom's Hardware (27/04/2023) — AMD limita a tensão SoC a 1,3 V", "https://www.tomshardware.com/news/amd-issues-follow-up-statement-on-ryzen-burnout-issues-limits-soc-voltages")}})
		}
	},
	// Residue left by other optimizers / debloat scripts.
	func(f *facts.Facts, st map[string]tweaks.State, add func(Finding)) {
		r := f.Residue
		acs := strings.Join(f.Gaming.AntiCheats, ", ")
		if f.Boot.Readable && (f.Boot.On(f.Boot.TestSigning) || f.Boot.On(f.Boot.NoIntegrityChecks)) {
			sev, ex := SevWarning, "Isso costuma ser deixado por ferramentas que instalam drivers não assinados."
			if f.Boot.On(f.Boot.TestSigning) {
				ex = "No modo de teste, drivers sem assinatura válida podem carregar. " + ex
			} else {
				ex = "Pela Microsoft o nointegritychecks é ignorado desde o Windows 7/8, mas anti-cheats como o EasyAntiCheat recusam o boot com ele marcado. " + ex
			}
			if acs != "" {
				sev = SevCritical
				ex = "Anti-cheats (" + acs + ") recusam rodar com esses parâmetros de boot: o jogo não abre ou você é desconectado. " + ex
			}
			var ids []string
			var what []string
			if f.Boot.On(f.Boot.TestSigning) {
				ids = append(ids, "fix.bcd.testsigning")
				what = append(what, "testsigning = "+f.Boot.TestSigning)
			}
			if f.Boot.On(f.Boot.NoIntegrityChecks) {
				ids = append(ids, "fix.bcd.nointegrity")
				what = append(what, "nointegritychecks = "+f.Boot.NoIntegrityChecks)
			}
			add(Finding{ID: "res.dse", Area: AreaSeguranca, Severity: sev, Title: "Verificação de assinatura de drivers desligada",
				Evidence: "bcdedit {current}: " + strings.Join(what, ", ") + ".", Explanation: ex,
				Actions: []Action{fixTweaks("Corrigir", ids...)}})
		}
		if f.Boot.Readable && strings.EqualFold(strings.TrimSpace(f.Boot.NX), "AlwaysOff") {
			add(Finding{ID: "res.dep", Area: AreaSeguranca, Severity: SevWarning, Title: "Prevenção de Execução de Dados (DEP) desligada",
				Evidence: "bcdedit {current}: nx = AlwaysOff.", Explanation: "Proteção básica de memória, exigida por anti-cheats. Não há ganho de desempenho em desligá-la.",
				Actions: []Action{fixTweaks("Religar DEP", "fix.bcd.nx")}})
		}
		if !r.Readable {
			return
		}
		if r.UACOff {
			add(Finding{ID: "res.uac", Area: AreaSeguranca, Severity: SevWarning, Title: "Controle de Conta de Usuário (UAC) desligado",
				Evidence: "EnableLUA = 0.", Explanation: "Todo programa roda como administrador e apps da Microsoft Store (inclusive o app Xbox) não abrem. Não há ganho de desempenho.",
				Actions: []Action{fixTweaks("Religar UAC", "fix.uac")}})
		}
		if r.WindowScalingOff() {
			add(Finding{ID: "res.tcp1323", Area: AreaRede, Severity: SevWarning, Title: "Escalonamento de janela TCP desligado",
				Evidence: fmt.Sprintf("Tcp1323Opts = %d.", r.Tcp1323Opts), Explanation: "Limita a velocidade de downloads (Steam, atualizações). Não muda o ping de jogos.",
				Actions: []Action{fixTweaks("Restaurar padrão", "fix.tcp.windowscaling")}})
		}
		if r.TaskOffloadOff {
			add(Finding{ID: "res.offload", Area: AreaRede, Severity: SevInfo, Title: "Offloads de rede desligados",
				Evidence: "DisableTaskOffload = 1.", Explanation: "A CPU passa a fazer trabalho da placa de rede. Valor documentado pela Microsoft para depuração de drivers.",
				Actions: []Action{fixTweaks("Restaurar padrão", "fix.tcp.taskoffload")}})
		}
		if r.TdrOff() {
			add(Finding{ID: "res.tdr", Area: AreaEstab, Severity: SevWarning, Title: "Recuperação do driver de vídeo desligada",
				Evidence: "TdrLevel = 0.", Explanation: "Se a GPU travar, a tela congela até reiniciar o PC em vez de o Windows recuperar o driver.",
				Actions: []Action{fixTweaks("Religar", "fix.gpu.tdr")}})
		}
		if r.LargeSystemCache {
			add(Finding{ID: "res.largecache", Area: AreaDesempenho, Severity: SevInfo, Title: "Cache de sistema em modo servidor",
				Evidence: "LargeSystemCache = 1.", Explanation: "Favorece o cache de arquivos em vez da memória dos programas e jogos.",
				Actions: []Action{fixTweaks("Restaurar padrão", "fix.mem.largecache")}})
		}
		if r.NetworkThrottlingOff() {
			add(Finding{ID: "res.net.throttling", Area: AreaRede, Severity: SevInfo, Title: "Limitação de rede do MMCSS desligada",
				Evidence:    "NetworkThrottlingIndex = 0xFFFFFFFF (padrão do Windows: 10).",
				Explanation: "Valor gravado por otimizadores para \"liberar a rede\". Não há medição pública de ping menor com ele, e a única medição encontrada (pesquisa de rede do djdallmann) viu a latência de DPC da placa de rede aumentar.",
				Actions:     []Action{fixTweaks("Voltar ao padrão", "fix.net.throttling")},
				Sources:     []tweaks.Source{srcDjdNet}})
		}
		if r.MemoryCompressionOff() {
			add(Finding{ID: "res.mem.compression", Area: AreaDesempenho, Severity: SevInfo, Title: "Compressão de memória desligada",
				Evidence:    "PS_MMAgent (WMI): MemoryCompression = False (padrão do Windows: ligada).",
				Explanation: "Alguns otimizadores desligam a compressão com Disable-MMAgent -MemoryCompression. A única fonte encontrada é uma resposta de consultor independente no Microsoft Q&A: sem a compressão o arquivo de paginação pode ser mais usado, com risco de lentidão se a RAM encher. Não há medição pública de FPS nos dois casos, e com RAM sobrando a diferença tende a ser nenhuma. Se você desligou de propósito, pode manter.",
				Actions:     []Action{fixTweaks("Religar", "fix.mem.compression")},
				Sources:     []tweaks.Source{srcMemCompQA}})
		}
		switch {
		case r.PageFile == "none":
			add(Finding{ID: "res.pagefile", Area: AreaEstab, Severity: SevWarning, Title: "Sem arquivo de paginação",
				Evidence: "PagingFiles vazio.", Explanation: "O limite de memória virtual fica igual à RAM: jogos e navegadores podem fechar por \"falta de memória\" mesmo com RAM livre, e o Windows não grava relatórios de tela azul.",
				Actions: []Action{fixTweaks("Deixar o Windows gerenciar", "fix.pagefile")}})
		case r.PageFile == "custom" && r.PageFileMaxMB > 0 && r.PageFileMaxMB < 8192:
			add(Finding{ID: "res.pagefile", Area: AreaEstab, Severity: SevInfo, Title: fmt.Sprintf("Arquivo de paginação fixo em %d MB", r.PageFileMaxMB),
				Evidence: "PagingFiles: " + strings.Join(r.PagingFilesRaw, "; ") + ".", Explanation: "Tamanho fixo pequeno limita a memória virtual em jogos pesados. O gerenciamento automático é o recomendado pela Microsoft.",
				Actions: []Action{fixTweaks("Deixar o Windows gerenciar", "fix.pagefile")}})
		}
		if len(r.UpdateBlocks) > 0 {
			seen := map[string]bool{}
			var why []string
			for _, b := range r.UpdateBlocks {
				if !seen[b.Why] {
					seen[b.Why] = true
					why = append(why, b.Why)
				}
			}
			add(Finding{ID: "res.wu", Area: AreaSeguranca, Severity: SevWarning, Title: "Windows Update bloqueado",
				Evidence: strings.Join(why, "; ") + ".", Explanation: "Sem atualizações o PC fica sem correções de segurança e de estabilidade. Para evitar drivers automáticos existe uma opção específica, sem bloquear o resto.",
				Actions: []Action{fixTweaks("Desbloquear", "fix.update.policies")}})
		}
		var sysSvc, appSvc []string
		for _, s := range r.DisabledServices {
			item := s.Display + " (" + s.Impact + ")"
			if s.Group == "system" {
				sysSvc = append(sysSvc, item)
			} else {
				appSvc = append(appSvc, item)
			}
		}
		if len(sysSvc) > 0 {
			add(Finding{ID: "res.svc.system", Area: AreaEstab, Severity: SevWarning, Title: plural(len(sysSvc), "serviço essencial desativado", "serviços essenciais desativados"),
				Evidence: strings.Join(sysSvc, "; ") + ".", Explanation: "Desativados por scripts de \"otimização\". Esses serviços não custam FPS e desativá-los quebra funções do Windows.",
				Actions: []Action{fixTweaks("Reativar", "fix.services.system")}})
		}
		if len(appSvc) > 0 {
			add(Finding{ID: "res.svc.apps", Area: AreaEstab, Severity: SevInfo, Title: plural(len(appSvc), "serviço opcional desativado", "serviços opcionais desativados"),
				Evidence: strings.Join(appSvc, "; ") + ".", Explanation: "Não são essenciais ao Windows, mas também não custam desempenho: em Manual só iniciam quando algo precisa. Desativados, quebram funções e causam erros difíceis de diagnosticar.",
				Actions: []Action{fixTweaks("Reativar", "fix.services.apps")}})
		}
		if r.HostsEntries > 30 {
			add(Finding{ID: "res.hosts", Area: AreaRede, Severity: SevInfo, Title: fmt.Sprintf("Arquivo hosts com %d bloqueios", r.HostsEntries),
				Evidence: "Exemplos: " + strings.Join(r.HostsSample, ", ") + ".", Explanation: "Listas de bloqueio no hosts (comuns em \"anti-telemetria\") podem quebrar Windows Update, Microsoft Store, login do Xbox e launchers. Malware também altera esse arquivo. Revise o conteúdo. Avisar a partir de 31 linhas é critério do Axion."})
		}
		if len(r.NVMeOverrides) > 0 {
			add(Finding{ID: "res.nvme.override", Area: AreaArmaz, Severity: SevWarning, Title: "Driver NVMe de servidor forçado por um ajuste não suportado",
				Evidence: "FeatureManagement\\Overrides: " + strings.Join(r.NVMeOverrides, ", ") + " = 1 (ativam o nvmedisk.sys do Windows Server 2025).",
				Explanation: "A Microsoft só suporta esse driver no Windows Server 2025; em março de 2026 as builds Insider do Windows 11 deixaram de aceitar esses valores, e o bloqueio pode chegar às estáveis. " +
					"Quem usou relatou Samsung Magician e WD Dashboard sem reconhecer o SSD, pedido da chave de recuperação do BitLocker e risco de o Windows não iniciar. Não acelera SSDs SATA.",
				Actions: []Action{fixTweaks("Remover", "fix.nvme.override")},
				Sources: []tweaks.Source{srcNVMeServer, srcNVMeBlocked, srcMS("HotHardware (24/03/2026) — bloqueio nas builds Insider e IDs novos", "https://hothardware.com/news/microsoft-kills-nvme-registry-trick-but-theres-a-workaround")}})
		}
		if r.SpectreOverride && !st["adv.spectre"].ByAxion {
			add(Finding{ID: "res.spectre", Area: AreaSeguranca, Severity: SevInfo, Title: "Mitigações Spectre/Meltdown desligadas por outro programa",
				Evidence: "FeatureSettingsOverride = 3 em Memory Management.", Explanation: "Em CPUs atuais o ganho de desligar é mínimo e a exposição a ataques continua.",
				Actions: []Action{fixTweaks("Religar", "fix.spectre")}})
		}
	},
	// Axion changes undone by Windows (updates, other tools).
	func(f *facts.Facts, st map[string]tweaks.State, add func(Finding)) {
		var ids []string
		for id, s := range st {
			if s.ByAxion && s.Drifted {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			return
		}
		sort.Strings(ids)
		add(Finding{ID: "axion.drift", Area: AreaDesempenho, Severity: SevWarning, Title: plural(len(ids), "otimização desfeita desde a aplicação", "otimizações desfeitas desde a aplicação"),
			Evidence:    "Alteradas depois de aplicadas: " + strings.Join(ids, ", ") + ".",
			Explanation: "Atualizações grandes do Windows e outros programas às vezes restauram configurações. O backup original continua guardado; você pode reaplicar.",
			Actions:     []Action{fixTweaks("Reaplicar", ids...)}})
	},
}

// adapterLabel names an adapter in evidence, saying when it is unplugged.
func adapterLabel(a facts.Adapter) string {
	if !a.Up {
		return a.Description + " (desconectada)"
	}
	return a.Description
}

// speedFinding reports wired adapters with a fixed speed and duplex.
func speedFinding(f *facts.Facts, add func(Finding)) {
	var parts []string
	slow, slowIdle := false, false
	for _, a := range f.PhysicalAdapters() {
		kw, ok := a.Keywords["*SpeedDuplex"]
		if !ok || a.Kind != "Ethernet" || kw.Current == "" || kw.Current == "0" {
			continue
		}
		v := kw.Current
		if kw.Label(v) != v {
			v += " (" + kw.Label(v) + ")"
		}
		parts = append(parts, adapterLabel(a)+": velocidade e duplex (*SpeedDuplex) = "+v)
		if len(kw.Current) == 1 && kw.Current >= "1" && kw.Current <= "4" {
			if a.Up {
				slow = true
			} else {
				slowIdle = true
			}
		}
	}
	if len(parts) == 0 {
		return
	}
	sev, ex := SevInfo, "Fixar a velocidade não reduz o ping: a norma do gigabit por cabo (1000BASE-T) exige autonegociação, então fixar só tira a adaptação do link — com um cabo ruim a conexão cai em vez de continuar a 100 Mb/s. O padrão do driver é o automático."
	switch {
	case slow:
		sev, ex = SevWarning, "Fixada em 10 ou 100 Mb/s, a placa limita a velocidade da internet e dos downloads. Volte para o automático."
	case slowIdle:
		ex = "Fixada em 10 ou 100 Mb/s, a placa vai limitar a velocidade da internet quando o cabo for usado. Volte para o automático."
	}
	add(Finding{ID: "net.speed", Area: AreaRede, Severity: sev, Title: "Velocidade da placa de rede fixada (em vez de automática)",
		Evidence:    strings.Join(parts, "; ") + " (0 = automático).",
		Explanation: ex, Actions: []Action{fixTweaks("Voltar ao automático", "fix.nic.speed")}})
}

// nicDefaultsFinding lists the advanced adapter settings changed from the
// driver's defaults (by hand or by an optimizer).
func nicDefaultsFinding(f *facts.Facts, add func(Finding)) {
	var parts, notes []string
	n := 0
	seen := map[string]bool{}
	wifiPower := false // Intel Wi-Fi power/roaming keywords (a latency-for-battery trade, not an offload)
	for _, a := range f.PhysicalAdapters() {
		ch := a.NonDefaults()
		if len(ch) == 0 {
			continue
		}
		n += len(ch)
		var items []string
		for _, c := range ch {
			items = append(items, fmt.Sprintf("%s = %s (padrão: %s)", c.Label(), c.Current, c.Default))
			if c.Note != "" && !seen[c.Name] {
				seen[c.Name] = true
				notes = append(notes, c.Name+": "+c.Note)
			}
			switch c.Keyword {
			case "MIMOPowerSaveMode", "RoamAggressiveness", "RoamingPreferredBandType", "*PacketCoalescing":
				wifiPower = true
			}
		}
		parts = append(parts, adapterLabel(a)+" — "+strings.Join(items, "; "))
	}
	if n == 0 {
		return
	}
	ex := "Opções mudadas à mão, por um otimizador ou já na instalação de fábrica. Nenhuma tem ganho de ping medido; as de descarga (offload) passam para a CPU trabalho que a placa faria, e outras mudam como o Wi-Fi escolhe o ponto de acesso. "
	if wifiPower {
		ex += "As de energia e roaming do Wi-Fi (economia MIMO/SMPS, agressividade de roaming, banda preferida, agrupamento de pacotes) trocam bateria por antenas sempre ativas e menos varreduras de rede, que são a causa comum de picos de latência no Wi-Fi: fazem sentido para jogar na tomada, e o padrão da Intel é o melhor na bateria. "
	}
	ex += "Se a conexão cai, oscila ou ficou mais lenta, volte ao padrão do fabricante (dá para desfazer). Se você mudou alguma de propósito e está tudo bem, pode dispensar este aviso."
	if len(notes) > 0 {
		ex += " Sobre as opções encontradas — " + strings.Join(notes, "; ") + "."
	}
	add(Finding{ID: "net.nic.defaults", Area: AreaRede, Severity: SevInfo,
		Title:       plural(n, "opção avançada de placa de rede fora do padrão do driver", "opções avançadas de placas de rede fora do padrão do driver"),
		Evidence:    strings.Join(parts, " | ") + ".",
		Explanation: ex,
		Actions:     []Action{fixTweaks("Voltar ao padrão do driver", "fix.nic.defaults")},
		Sources:     []tweaks.Source{srcNicDefaults, srcIntelWifi}})
}

// mbrCaution warns a legacy-BIOS install (MBR disk) before any advice to turn
// the CSM off: without the GPT conversion the PC stops booting.
func mbrCaution(f *facts.Facts) string {
	if strings.EqualFold(f.OS.Firmware, "BIOS") {
		return " Atenção: este Windows foi instalado no modo BIOS antigo (disco MBR). Converta o disco para GPT com o MBR2GPT da Microsoft antes de desligar o CSM, senão o PC não inicia."
	}
	return ""
}

// iommuNote adds the IOMMU state Windows reports (DMA protection) to an
// anti-cheat evidence line.
func iommuNote(s facts.SecurityInfo) string {
	switch s.IOMMU {
	case "on":
		return ", IOMMU ligado (proteção de DMA disponível)"
	case "off":
		return ", IOMMU desligado ou ausente (proteção de DMA indisponível)"
	}
	return ""
}

// vbsFinding describes what runs on the hypervisor when no anti-cheat needs
// it, and says so when a "VBS off" already in the registry is not honoured.
func vbsFinding(f *facts.Facts, st map[string]tweaks.State, add func(Finding)) {
	s := f.Security
	title := "VBS ligado (Integridade de Memória desligada)"
	if s.HVCIRunning {
		title = "VBS e Integridade de Memória ligados"
	}
	ev := "Segurança baseada em virtualização (VBS) em execução"
	if svc := s.RunningServices(); len(svc) > 0 {
		ev += "; recursos que a usam: " + strings.Join(svc, ", ")
	} else {
		ev += "; o Windows não lista nenhum recurso usando o VBS (Integridade de Memória, Credential Guard…)"
	}
	if len(s.HypervisorFeatures) > 0 {
		ev += ". O hipervisor também é iniciado por: " + strings.Join(s.HypervisorFeatures, ", ")
	}
	ev += ". Nenhum anti-cheat que exija VBS foi encontrado."
	const warn = "Atenção: FACEIT e Vanguard exigem ou podem exigir VBS."
	ex := "A Microsoft cita a Integridade de Memória e a Plataforma de Máquina Virtual como possível custo de desempenho em jogos e sugere desligar só enquanto joga. " + warn
	if !s.HVCIRunning {
		ex = "A Integridade de Memória, o recurso de VBS que a Microsoft cita como possível custo em jogos, já está desligada. " + warn
	}
	actions := []Action{fixTweaks("Ver opção (avançado)", "adv.vbs.off")}
	if vs, ok := st["adv.vbs.off"]; ok && vs.Status == "on" {
		switch vs.Effect {
		case "ignored":
			title = "VBS continua ligado, embora o registro peça para desligar"
			if vs.AppliedAt != nil {
				// Written by Axion before the last boot: Windows had its restart.
				ex = "Os valores que desligam o VBS já estão no registro e o PC já reiniciou depois deles, mas o Windows o mantém em execução: " + strings.Join(s.VBSHolders(), "; ") + ". " +
					"Por isso o ajuste aparece como aplicado sem estar valendo. Se você quer o VBS ligado (FACEIT/Vanguard), não há nada a fazer; se quer desligado, resolva o motivo acima."
			} else {
				// Written by someone else (another optimizer, by hand): when,
				// nobody knows, so a restart comes before any conclusion.
				ex = "Os valores que desligam o VBS estão no registro (não foram gravados pelo Axion), mas o Windows o mantém em execução. Reinicie o PC antes de concluir que ele os ignora; se depois do reinício o VBS continuar ligado, o motivo está aqui: " + strings.Join(s.VBSHolders(), "; ") + ". " +
					"Se você quer o VBS ligado (FACEIT/Vanguard), não há nada a fazer."
			}
			actions = nil
		case "reboot":
			title += " (reinício pendente)"
			ex = "O Axion já gravou o pedido para desligar o VBS: ele para depois de reiniciar o PC. Depois do reinício, rode a análise de novo para confirmar."
			actions = nil
		}
	}
	add(Finding{ID: "sec.vbs", Area: AreaDesempenho, Severity: SevInfo, Title: title, Evidence: ev, Explanation: ex, Actions: actions})
}

// pcieFinding reports a GPU link narrower than the card supports. NVIDIA
// documents that the link width and generation "may be reduced when the GPU
// is not in use": only a reading with the link at its maximum generation
// shows the width it trained to. In a notebook the lanes wired to the GPU are
// the manufacturer's design: there is no slot to change.
func pcieFinding(f *facts.Facts, add func(Finding)) {
	n := f.Nvidia
	if !n.SMI || n.PCIeWidthMax <= 0 || n.PCIeWidth <= 0 || n.PCIeWidth >= n.PCIeWidthMax {
		return
	}
	ev := fmt.Sprintf("nvidia-smi: link PCIe x%d (a placa suporta x%d)", n.PCIeWidth, n.PCIeWidthMax)
	if n.PCIeGenCur > 0 && n.PCIeGenMax > 0 {
		ev += fmt.Sprintf(", geração %d de %d", n.PCIeGenCur, n.PCIeGenMax)
	}
	if n.PState != "" {
		ev += ", estado " + n.PState
	}
	if n.UtilPct >= 0 && (n.PState != "" || n.PCIeGenCur > 0) {
		ev += fmt.Sprintf(", %d%% de uso", n.UtilPct)
	}
	ev += " no momento da leitura."
	src := []tweaks.Source{srcNvidiaSMI}
	switch {
	case f.IsLaptop():
		add(Finding{ID: "gpu.pcie.width", Area: AreaDesempenho, Severity: SevGood,
			Title:       fmt.Sprintf("GPU do notebook ligada em PCIe x%d", n.PCIeWidth),
			Evidence:    ev,
			Explanation: fmt.Sprintf("Num notebook, quantas linhas PCIe chegam à GPU é decisão do fabricante na placa-mãe: não há slot para trocar nem nada a corrigir. O \"x%d\" é o máximo que o chip aceita; o notebook pode ter sido projetado com menos linhas, e com a GPU sem uso o link também pode ser reduzido.", n.PCIeWidthMax),
			Sources:     src})
	case n.LinkAtFullSpeed():
		add(Finding{ID: "gpu.pcie.width", Area: AreaDesempenho, Severity: SevWarning,
			Title:       fmt.Sprintf("GPU trabalhando em PCIe x%d (suporta x%d)", n.PCIeWidth, n.PCIeWidthMax),
			Evidence:    ev,
			Explanation: "Lido com o link na geração máxima, então essa é a largura real. A placa pode estar no slot errado (use o slot PCIe superior, ligado à CPU), mal encaixada, num riser ou dividindo linhas com um SSD M.2 (veja o manual da placa-mãe). Para ter uma ideia do tamanho: numa RTX 4090 em 1080p, o TechPowerUp mediu cerca de 2% a menos em PCIe 4.0 x8, 6% em 4.0 x4 e 21% em 3.0 x4.",
			Sources:     append(src, srcPCIeScaling)})
	default:
		title := fmt.Sprintf("GPU lida em PCIe x%d com a placa em repouso (suporta x%d)", n.PCIeWidth, n.PCIeWidthMax)
		if n.PCIeGenCur <= 0 {
			title = fmt.Sprintf("GPU lida em PCIe x%d sem confirmar se estava em uso (suporta x%d)", n.PCIeWidth, n.PCIeWidthMax)
		}
		add(Finding{ID: "gpu.pcie.width", Area: AreaDesempenho, Severity: SevInfo,
			Title:       title,
			Evidence:    ev,
			Explanation: "Sem uso a NVIDIA pode reduzir a largura e a geração do link para economizar energia, então esta leitura não confirma um problema. Para conferir, deixe um jogo aberto e veja a interface de barramento no GPU-Z (ou rode a análise de novo com o jogo aberto). Se continuar abaixo do máximo com a GPU em uso: slot errado, placa mal encaixada, riser ou linhas divididas com um SSD M.2.",
			Sources:     src})
	}
}

// nvmeFindings turns the drive's own health log into findings.
func nvmeFindings(f *facts.Facts, d facts.Disk, h *sys.NVMeHealth, name, num string, add func(Finding)) {
	serious := h.CriticalWarning &^ sys.NVMeWarnTemperature
	if serious != 0 {
		w := sys.NVMeHealth{CriticalWarning: serious}.Warnings()
		add(Finding{ID: "disk.nvme.alert." + num, Area: AreaArmaz, Severity: SevCritical, Title: "O SSD emitiu um alerta crítico: " + d.Model,
			Evidence:    fmt.Sprintf("%s. Log SMART/Saúde NVMe: Critical Warning = 0x%02X (%s); reserva disponível %d%% (limite do fabricante %d%%).", name, h.CriticalWarning, strings.Join(w, "; "), h.SparePct, h.SpareThreshold),
			Explanation: "É o próprio controlador do disco avisando que a confiabilidade caiu. Faça backup agora e troque o disco; não use ele para o sistema."})
	}
	if h.CriticalWarning&sys.NVMeWarnTemperature != 0 {
		add(Finding{ID: "disk.nvme.temp." + num, Area: AreaArmaz, Severity: SevWarning, Title: "SSD fora do limite de temperatura: " + d.Model,
			Evidence:    fmt.Sprintf("%s. Critical Warning bit 1 ligado; %d °C agora; %d min acima do limite de aviso e %d min acima do crítico desde a fabricação.", name, h.TempC, h.WarnTempMinutes, h.CritTempMinutes),
			Explanation: "Acima do limite o SSD reduz a velocidade para se proteger e, em uso prolongado, a vida útil cai. Um dissipador no M.2 ou mais fluxo de ar resolve."})
	}
	if h.MediaErrors > 0 && serious == 0 {
		add(Finding{ID: "disk.nvme.media." + num, Area: AreaArmaz, Severity: SevWarning, Title: "SSD com " + thousands(h.MediaErrors) + map[bool]string{true: " erro", false: " erros"}[h.MediaErrors == 1] + " de integridade de dados",
			Evidence:    fmt.Sprintf("%s. Log SMART/Saúde NVMe: Media and Data Integrity Errors = %s.", name, thousands(h.MediaErrors)),
			Explanation: "São leituras que o controlador não conseguiu corrigir. Mesmo poucas já indicam células falhando: mantenha backup e acompanhe; se o número subir, troque o disco."})
	}
	if h.UnsafeShutdowns >= 20 && h.PowerCycles > 0 && h.UnsafeShutdowns*5 >= h.PowerCycles {
		add(Finding{ID: "disk.nvme.unsafe." + num, Area: AreaEstab, Severity: SevInfo, Title: fmt.Sprintf("%s desligamentos sem aviso registrados pelo SSD", thousands(h.UnsafeShutdowns)),
			Evidence:    fmt.Sprintf("%s. %s de %s ciclos de energia (%d%%) terminaram sem o Windows avisar o disco.", name, thousands(h.UnsafeShutdowns), thousands(h.PowerCycles), h.UnsafeShutdowns*100/h.PowerCycles),
			Explanation: unsafeShutdownText(f)})
	}
}

// tb formats bytes as decimal terabytes (the unit SSD makers use for TBW).
func tb(b uint64) string {
	if b >= 1e12 {
		return strings.Replace(fmt.Sprintf("%.1f TB", float64(b)/1e12), ".", ",", 1)
	}
	return fmt.Sprintf("%d GB", b/1e9)
}

// trimFindings checks TRIM and the drive optimizer when the PC has an SSD.
func trimFindings(f *facts.Facts, add func(Finding)) {
	if !f.HasSSD() {
		return
	}
	t := f.Trim
	ssdPage := Action{Kind: "page", Label: "Abrir manutenção do SSD", Page: "limpeza"}
	if t.Readable && t.NTFS == 1 {
		add(Finding{ID: "ssd.trim.off", Area: AreaArmaz, Severity: SevWarning, Title: "TRIM desligado no Windows",
			Evidence:    "fsutil behavior query disabledeletenotify: NTFS DisableDeleteNotify = 1 (padrão do Windows: 0).",
			Explanation: "Sem TRIM o SSD não fica sabendo quais blocos foram apagados: continua movendo dados que já não existem na coleta de lixo, as gravações ficam mais lentas com o tempo e o desgaste aumenta. Religar vale na hora, sem reiniciar; depois, um retrim avisa o SSD de todo o espaço que foi liberado enquanto estava desligado.",
			Actions:     []Action{fixTweaks("Religar TRIM", "fix.trim.on"), ssdPage}, Sources: []tweaks.Source{srcFsutilTrim}})
	}
	if t.TaskRead {
		switch {
		case !t.TaskExists:
			add(Finding{ID: "ssd.optimize.missing", Area: AreaArmaz, Severity: SevWarning, Title: "Tarefa de otimização de unidades removida",
				Evidence:    "A tarefa " + sys.DefragTask + " não existe neste Windows.",
				Explanation: "É ela que faz o retrim mensal do SSD (reenvia o TRIM de todo o espaço livre, cobrindo avisos que se perdem no dia a dia) e desfragmenta HDs. Costuma ser apagada por scripts que acham que o Windows \"desfragmenta o SSD\". Sem ela, use o botão de retrim do Axion uma vez por mês.",
				Actions:     []Action{ssdPage}, Sources: []tweaks.Source{srcDefragCmd, srcSSDStory}})
		case !t.TaskOn:
			add(Finding{ID: "ssd.optimize.off", Area: AreaArmaz, Severity: SevWarning, Title: "Otimização automática de unidades desativada",
				Evidence:    "A tarefa " + sys.DefragTask + " está desativada.",
				Explanation: "Em SSD essa tarefa não faz desfragmentação semanal: ela faz o retrim uma vez por mês (reenvia o TRIM de todo o espaço livre, cobrindo avisos que se perdem sob carga) e, segundo a equipe de armazenamento do Windows (2014), só desfragmenta o SSD quando há instantâneos de volume (Restauração do Sistema), porque a cópia-na-gravação fica lenta num volume muito fragmentado. Desativá-la não protege o SSD.",
				Actions:     []Action{fixTweaks("Reativar", "fix.defrag.task"), ssdPage}, Sources: []tweaks.Source{srcDefragCmd, srcSSDStory}})
		}
	}
	// A stale retrim is reported only when the log shows an old one and nothing
	// newer (no events = no finding: the log may have been cleared).
	for _, v := range f.Volumes {
		last, ok := t.LastRetrim[strings.ToUpper(v.Letter)]
		if !v.SSD || !ok || !t.TaskOn || (t.Readable && t.NTFS == 1) {
			continue
		}
		if days := int(time.Since(last).Hours() / 24); days > 45 {
			add(Finding{ID: "ssd.retrim.old." + strings.TrimSuffix(strings.ToUpper(v.Letter), ":"), Area: AreaArmaz, Severity: SevInfo,
				Title:       fmt.Sprintf("Último retrim de %s há %d dias", v.Letter, days),
				Evidence:    "Log de Aplicativo, evento 258 do otimizador de unidades: último retrim em " + last.Format("02/01/2006") + ".",
				Explanation: "O Windows faz o retrim do SSD uma vez por mês na manutenção automática, que roda com o PC ocioso e para quando você mexe nele. Em PCs que quase nunca ficam ociosos ligados, ela atrasa; um retrim manual resolve.",
				Actions:     []Action{ssdPage}, Sources: []tweaks.Source{srcDefragCmd, srcAutoMaint}})
		}
	}
}

// storageDriverNote names the physical storage controllers and their
// drivers. Only stornvme (the inbox NVMe driver) is documented to pass the
// NVMe health log through; RAID and vendor drivers usually refuse it.
func storageDriverNote(f *facts.Facts) string {
	var inbox, other []string
	for _, c := range f.Controllers {
		if !strings.HasPrefix(strings.ToUpper(c.PNP), "PCI\\") || c.Driver == "" {
			continue
		}
		d := strings.ToLower(c.Driver)
		label := c.Name + " (" + c.Driver + ")"
		if n, ok := facts.StorageDriverNames[d]; ok && c.Name == "" {
			label = n
		}
		if d == "stornvme" || d == "storahci" {
			inbox = append(inbox, label)
		} else {
			other = append(other, label)
		}
	}
	switch {
	case len(other) > 0:
		return " Controladora com driver que não é o padrão do Windows: " + strings.Join(other, ", ") + ". Drivers de RAID ou de fabricante costumam não repassar o comando de saúde do NVMe; ferramentas como o CrystalDiskInfo podem ter o mesmo problema."
	case len(inbox) > 0:
		return " Controladora: " + strings.Join(inbox, ", ") + ", o driver padrão do Windows. Se o Windows também não mostra a integridade da unidade em Configurações, a limitação é do próprio SSD ou de um filtro de disco (criptografia ou backup)."
	}
	return ""
}

// storageDriverAdvice explains the way back to the inbox driver when an NVMe
// controller runs a vendor miniport (not RAID, whose change needs more care).
func storageDriverAdvice(f *facts.Facts) string {
	for _, c := range f.Controllers {
		d := strings.ToLower(c.Driver)
		if !strings.HasPrefix(strings.ToUpper(c.PNP), "PCI\\") || d == "" || d == "stornvme" || d == "storahci" {
			continue
		}
		if d == "rcbottom" || d == "rcraid" || strings.HasPrefix(d, "iastor") {
			return " A controladora está em modo RAID: mudar para AHCI/NVMe exige preparar o Windows antes e não vale a pena só por isso."
		}
		return " Opcional: trocar para o driver da Microsoft devolve essa leitura (Gerenciador de Dispositivos → Controladores de armazenamento → " + c.Name +
			" → Atualizar driver → Procurar no computador → Permitir que eu escolha → controlador NVM Express padrão). É o disco do sistema: crie um ponto de restauração antes e, se usar BitLocker, tenha a chave de recuperação à mão."
	}
	return ""
}

// ssdTool names the vendor's SSD utility for a model, when known.
func ssdTool(model string) string {
	m := strings.ToUpper(model)
	switch {
	case strings.HasPrefix(m, "WD") || strings.Contains(m, "WESTERN DIGITAL") || strings.Contains(m, "SANDISK"):
		return " (WD Dashboard / SanDisk Dashboard)"
	case strings.Contains(m, "SAMSUNG"):
		return " (Samsung Magician)"
	case strings.Contains(m, "KINGSTON"):
		return " (Kingston SSD Manager)"
	case strings.Contains(m, "CRUCIAL") || strings.HasPrefix(m, "CT"):
		return " (Crucial Storage Executive)"
	case strings.Contains(m, "ADATA") || strings.Contains(m, "XPG"):
		return " (ADATA SSD ToolBox)"
	case strings.Contains(m, "SEAGATE"):
		return " (SeaTools)"
	}
	return ""
}

// healthErr returns the collection error about a disk's health log, if any.
func healthErr(f *facts.Facts, number int) string {
	tag := fmt.Sprintf("disco %d:", number)
	for _, e := range f.Errors {
		if strings.Contains(e, tag) && strings.Contains(strings.ToLower(e), "nvme") {
			if i := strings.LastIndex(e, ": "); i >= 0 && i+2 < len(e) {
				return " (resposta: " + strings.TrimSuffix(e[i+2:], ".") + ")"
			}
		}
	}
	return ""
}

// thousands formats n with Brazilian thousands separators.
func thousands(n uint64) string {
	s := strconv.FormatUint(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	return s
}

// duplicatePlans lists power plan names that appear more than once, as
// "3× Name".
func duplicatePlans(schemes []sys.PowerScheme) []string {
	count := map[string]int{}
	var order []string
	for _, sc := range schemes {
		k := strings.ToLower(strings.TrimSpace(sc.Name))
		if k == "" {
			continue
		}
		if count[k] == 0 {
			order = append(order, sc.Name)
		}
		count[k]++
	}
	var out []string
	for _, name := range order {
		if n := count[strings.ToLower(strings.TrimSpace(name))]; n > 1 {
			out = append(out, fmt.Sprintf("%d× %s", n, strings.TrimSpace(name)))
		}
	}
	return out
}

// rebarText explains Resizable BAR; on a notebook the BIOS rarely has the
// option and the maker decides.
func rebarText(f *facts.Facts) string {
	if f.IsLaptop() {
		return "Em notebook o Resizable BAR depende do fabricante: quando ele oferece, vem numa atualização de BIOS ou já ligado, e raramente há opção para mudar. Confira se há BIOS nova no site do fabricante. A NVIDIA só ativa em jogos homologados; o ganho varia por jogo."
	}
	return "Com Above 4G Decoding + Re-Size BAR ligados na BIOS (CSM desligado), a CPU acessa toda a VRAM de uma vez. A NVIDIA só ativa em jogos homologados; o ganho varia por jogo."
}

// unsafeShutdownText explains the SSD's unsafe shutdown count by form factor.
func unsafeShutdownText(f *facts.Facts) string {
	if f.IsLaptop() {
		return "Acontece quando a bateria acaba, o botão de ligar é segurado, há um travamento ou um reset. Cada um interrompe gravações pendentes e pode corromper arquivos. Desligue pelo Windows e não deixe a bateria zerar."
	}
	return "Acontece em queda de energia, botão de desligar segurado, travamento ou reset. Cada um interrompe gravações pendentes e pode corromper arquivos. Desligue pelo Windows; se a energia cai com frequência, um nobreak resolve."
}

// Package health judges the Windows health checks: the facts read in the
// "Saúde do Windows" step become a list of checks, each one OK, needing
// attention, or not read. A check that could not be read is never reported as
// a problem (and never as healthy either): it says it was not read.
//
// The limits are Axion's own and say so; nothing here is a Microsoft number.
package health

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"axion/internal/facts"
	"axion/internal/tweaks"
)

// Status of a check.
type Status string

// Check statuses.
const (
	OK        Status = "ok"
	Attention Status = "attention"
	Unread    Status = "unread"
)

// Severities of a check that needs attention (the diagnostic's own).
const (
	SevWarning = "warning"
	SevInfo    = "info"
)

// Groups of the page, in display order.
const (
	GroupProtection = "Proteção"
	GroupUpdates    = "Atualizações e disco"
	GroupPower      = "Energia"
	GroupBoot       = "Inicialização"
)

// Groups lists the groups in display order.
var Groups = []string{GroupProtection, GroupUpdates, GroupPower, GroupBoot}

// Axion's limits.
const (
	SigAgeDays     = 7  // Defender signatures this old or older
	UpdateOldDays  = 90 // no quality update for this long
	BatteryWearPct = 40 // battery this worn or more
	BootSlowMs     = 60000
	BootMinSamples = 3
	BootWindow     = 5 // boots the median is taken over
)

// Link is where a check sends the user.
type Link struct {
	Label string `json:"label"`
	Page  string `json:"page,omitempty"`
	URL   string `json:"url,omitempty"`
}

// Check is one health check.
type Check struct {
	ID    string `json:"id"`
	Group string `json:"group"`
	// Title names the check; Headline says what is wrong (only with Attention).
	Title    string `json:"title"`
	Headline string `json:"headline,omitempty"`
	Status   Status `json:"status"`
	Severity string `json:"severity,omitempty"` // SevWarning | SevInfo, with Attention
	// Value is what was read ("assinaturas de 2 dias"); Detail explains an
	// attention or a check not read.
	Value     string `json:"value"`
	Detail    string `json:"detail,omitempty"`
	Criterion string `json:"criterion,omitempty"` // the limit, said as Axion's
	// Covered is the diagnostic finding that already reports this check (the
	// diagnostic does not repeat it).
	Covered string          `json:"covered,omitempty"`
	Links   []Link          `json:"links,omitempty"`
	Sources []tweaks.Source `json:"sources,omitempty"`
}

// Evaluate runs every check. The result keeps a fixed order (group, then the
// order below); checks that do not apply to this machine are left out.
func Evaluate(f *facts.Facts, now time.Time) []Check {
	var out []Check
	out = append(out, defenderChecks(f, now)...)
	out = append(out, updateChecks(f, now)...)
	out = append(out, diskChecks(f)...)
	out = append(out, restartChecks(f)...)
	out = append(out, batteryChecks(f)...)
	out = append(out, bootChecks(f)...)
	sort.SliceStable(out, func(i, j int) bool { return groupIndex(out[i].Group) < groupIndex(out[j].Group) })
	return out
}

func groupIndex(g string) int {
	for i, x := range Groups {
		if x == g {
			return i
		}
	}
	return len(Groups)
}

// Summary counts the checks by status.
type Summary struct {
	OK        int `json:"ok"`
	Attention int `json:"attention"`
	Unread    int `json:"unread"`
}

// Summarize counts the checks.
func Summarize(cs []Check) Summary {
	var s Summary
	for _, c := range cs {
		switch c.Status {
		case OK:
			s.OK++
		case Attention:
			s.Attention++
		default:
			s.Unread++
		}
	}
	return s
}

func dec1(x float64) string { return strings.Replace(fmt.Sprintf("%.1f", x), ".", ",", 1) }

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func unread(c Check, why string) Check {
	c.Status, c.Value = Unread, "não lido"
	c.Detail = why
	return c
}

// ---------------------------------------------------------------------------
// Protection
// ---------------------------------------------------------------------------

// defenderQuiet tells that the Defender is not the antivirus doing the work
// (passive mode beside another antivirus), so its own settings say nothing.
func defenderQuiet(d facts.DefenderHealth) bool {
	m := strings.ToLower(d.Mode)
	return strings.Contains(m, "passive") || strings.Contains(m, "edr") || (!d.RealTime && d.ThirdParty != "")
}

func defenderChecks(f *facts.Facts, now time.Time) []Check {
	d := f.Health.Defender
	var out []Check

	rt := Check{ID: "defender.realtime", Group: GroupProtection, Title: "Proteção em tempo real", Covered: "sec.defender"}
	switch f.Security.DefenderRealtime {
	case "on":
		rt.Status, rt.Value = OK, "ligada"
	case "off":
		rt.Status, rt.Severity, rt.Value = Attention, SevInfo, "desligada"
		rt.Headline = "Proteção em tempo real do Defender desligada"
		rt.Detail = "Se você não usa outro antivírus, religue. Para jogos, uma exclusão de pasta é a alternativa segura."
		if d.ThirdParty != "" {
			rt.Status, rt.Value = OK, "desligada, com outro antivírus registrado: "+d.ThirdParty
		}
	default:
		rt = unread(rt, "O Windows não respondeu sobre a proteção em tempo real.")
	}
	out = append(out, rt)

	sig := Check{ID: "defender.signatures", Group: GroupProtection, Title: "Assinaturas do Defender",
		Criterion: fmt.Sprintf("Critério do Axion: %d dias ou mais sem atualizar. Com o PC ligado e na internet, as assinaturas se atualizam sozinhas, numa cadência programada.", SigAgeDays),
		Sources:   []tweaks.Source{SrcDefenderUpdates}}
	switch {
	case !d.Read || !f.Read(facts.StepHealth):
		sig = unread(sig, "O Windows não informou o estado do Defender.")
	case defenderQuiet(d):
		sig.Status, sig.Value = OK, "o Defender está em segundo plano ("+orDefault(d.ThirdParty, "outro produto de segurança")+" é quem protege)"
	case d.SigAgeDays < 0:
		sig = unread(sig, "O Windows não informou a idade das assinaturas.")
	case d.SigAgeDays >= SigAgeDays:
		sig.Status, sig.Severity = Attention, SevWarning
		sig.Value = "assinaturas de " + plural(d.SigAgeDays, "dia", "dias")
		sig.Headline = "Defender com assinaturas de " + plural(d.SigAgeDays, "dia", "dias")
		sig.Detail = "O Defender só reconhece as ameaças que estão nas suas assinaturas; com elas velhas, vírus novos passam. Abra a Segurança do Windows e peça para verificar atualizações (ou rode o Windows Update). Se a atualização falhar sempre, veja o item de falhas do Windows Update."
		sig.Links = []Link{{Label: "Abrir a Segurança do Windows", URL: "windowsdefender:"}, {Label: "Abrir o Windows Update", URL: "ms-settings:windowsupdate"}}
	default:
		sig.Status = OK
		sig.Value = "atualizadas há " + ageText(d.SigAgeDays)
	}
	out = append(out, sig)

	tamper := Check{ID: "defender.tamper", Group: GroupProtection, Title: "Proteção contra violação",
		Sources: []tweaks.Source{SrcTamper}}
	switch {
	case !d.Read || !f.Read(facts.StepHealth):
		tamper = unread(tamper, "O Windows não informou o estado do Defender.")
	case defenderQuiet(d) || !d.Enabled:
		tamper.Status, tamper.Value = OK, "não se aplica (o Defender não é o antivírus ativo)"
	case d.Tamper:
		tamper.Status, tamper.Value = OK, "ligada"
	default:
		tamper.Status, tamper.Severity, tamper.Value = Attention, SevInfo, "desligada"
		tamper.Headline = "Proteção contra violação do Defender desligada"
		tamper.Detail = "Com ela ligada, programas (e vírus) não conseguem desligar o Defender nem mudar as configurações de segurança por baixo dos panos. Ligue em Segurança do Windows > Proteção contra vírus e ameaças > Gerenciar configurações."
		tamper.Links = []Link{{Label: "Abrir a Segurança do Windows", URL: "windowsdefender:"}}
	}
	out = append(out, tamper)

	excl := Check{ID: "defender.exclusions", Group: GroupProtection, Title: "Exclusões do Defender",
		Criterion: "Critério do Axion: só as exclusões mais amplas da lista que a Microsoft manda nunca excluir (disco inteiro, Users, Temp e Windows, tipos como .exe, .dll e .ps1, programas como cmd, PowerShell e java). A pasta de um jogo específico não é marcada, embora a Microsoft peça cautela com qualquer exclusão.",
		Sources:   []tweaks.Source{SrcExclusionMistakes}}
	switch {
	case !d.Read || !f.Read(facts.StepHealth):
		excl = unread(excl, "O Windows não informou o estado do Defender.")
	case !d.ExclusionsRead:
		excl = unread(excl, "As exclusões só aparecem para um administrador e o Windows não as listou.")
	default:
		broad := BroadExclusions(d)
		total := len(d.ExclPaths) + len(d.ExclExts) + len(d.ExclProcs)
		switch {
		case len(broad) > 0:
			excl.Status, excl.Severity = Attention, SevWarning
			excl.Value = strings.Join(broad, "; ")
			excl.Headline = "Defender com exclusões amplas demais"
			excl.Detail = "Tudo o que está numa exclusão deixa de ser verificado. Exclusões amplas (um disco inteiro, a pasta do usuário, tipos como .exe ou programas de script) são o primeiro lugar onde um vírus se esconde e foram a causa de vários ataques. Se foi um otimizador ou um instalador que colocou, remova em Segurança do Windows > Proteção contra vírus e ameaças > Gerenciar configurações > Exclusões. Para jogos, exclua só a pasta do jogo."
			excl.Links = []Link{{Label: "Abrir a Segurança do Windows", URL: "windowsdefender:"}}
		case total == 0:
			excl.Status, excl.Value = OK, "nenhuma exclusão"
		default:
			excl.Status, excl.Value = OK, plural(total, "exclusão, específica", "exclusões, todas específicas")
		}
	}
	out = append(out, excl)
	return out
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func ageText(days int) string {
	switch days {
	case 0:
		return "menos de 1 dia"
	case 1:
		return "1 dia"
	}
	return fmt.Sprintf("%d dias", days)
}

// Exclusions that are too broad. Paths are compared without case, with
// backslashes and without a final one.
var (
	broadPathRe = regexp.MustCompile(`^(?:[a-z]:|[a-z]:\\(?:\*|temp|users(?:\\\*)?|windows|windows\\system32|windows\\temp|windows\\prefetch|program files|program files \(x86\)|programdata)|[a-z]:\\users\\[^\\]+(?:\\(?:downloads|desktop|documents|appdata|appdata\\local|appdata\\roaming|appdata\\local\\temp|appdata\\locallow\\temp))?|\*)$`)
	broadEnvRe  = regexp.MustCompile(`^%(?:systemdrive|userprofile|windir|systemroot|programfiles|programfiles\(x86\)|programdata|temp|tmp|appdata|localappdata|public|homedrive)%$`)
	// File types and programs of Microsoft's "do not exclude" list that
	// attackers abuse (executables, scripts, installers and script hosts).
	badExt = map[string]bool{"exe": true, "dll": true, "scr": true, "bat": true, "cmd": true, "ps1": true, "vbs": true, "vbe": true, "js": true, "wsf": true,
		"msi": true, "com": true, "hta": true, "jar": true, "sys": true, "bin": true, "tmp": true, "url": true, "reg": true, "inf": true, "cpl": true,
		"ocx": true, "py": true, "*": true}
	badProc = map[string]bool{"powershell.exe": true, "cmd.exe": true, "wscript.exe": true, "cscript.exe": true, "mshta.exe": true, "svchost.exe": true,
		"bash.exe": true, "bitsadmin.exe": true, "schtasks.exe": true, "wmic.exe": true, "psexec.exe": true, "msbuild.exe": true, "dotnet.exe": true,
		"java.exe": true, "wuauclt.exe": true}
)

// BroadExclusions lists the Defender exclusions that leave too much unscanned,
// as the user wrote them.
func BroadExclusions(d facts.DefenderHealth) []string {
	var out []string
	for _, p := range d.ExclPaths {
		n := strings.ToLower(strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(p), "/", `\`), `\`))
		if broadPathRe.MatchString(n) || broadEnvRe.MatchString(n) {
			out = append(out, "pasta "+p)
		}
	}
	for _, e := range d.ExclExts {
		if badExt[strings.TrimPrefix(strings.ToLower(strings.TrimSpace(e)), ".")] {
			out = append(out, "tipo "+e)
		}
	}
	for _, p := range d.ExclProcs {
		q := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(p, "/", `\`)))
		base := q
		if i := strings.LastIndex(q, `\`); i >= 0 {
			base = q[i+1:]
		}
		if badProc[base] {
			out = append(out, "programa "+p)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Windows Update and disk
// ---------------------------------------------------------------------------

func updateChecks(f *facts.Facts, now time.Time) []Check {
	u := f.Health.Updates
	ok := u.Read && f.Read(facts.StepHealth)
	var out []Check

	rec := Check{ID: "update.recent", Group: GroupUpdates, Title: "Atualização do Windows em dia",
		Criterion: fmt.Sprintf("Critério do Axion: %d dias ou mais sem instalar nenhuma atualização de qualidade. A Microsoft publica uma por mês.", UpdateOldDays),
		Sources:   []tweaks.Source{SrcWUTroubleshoot}}
	switch {
	case !ok:
		rec = unread(rec, "O histórico do Windows Update não pôde ser lido.")
	case u.Last.IsZero():
		rec = unread(rec, "Não achei nenhuma atualização no histórico dos últimos 180 dias (o registro pode ter sido limpo), então não dá para dizer se o PC está em dia.")
	default:
		days := int(now.Sub(u.Last).Hours() / 24)
		if days < 0 {
			days = 0
		}
		when := "KB" + u.LastKB + " instalada há " + ageText(days)
		if days >= UpdateOldDays {
			rec.Status, rec.Severity, rec.Value = Attention, SevWarning, when
			rec.Headline = "Sem atualização do Windows há " + plural(days, "dia", "dias")
			rec.Detail = "As atualizações mensais corrigem falhas de segurança já exploradas por vírus. Com tanto tempo sem instalar, o PC está exposto a elas. Abra o Windows Update e instale o que estiver pendente."
			rec.Links = []Link{{Label: "Abrir o Windows Update", URL: "ms-settings:windowsupdate"}}
		} else {
			rec.Status, rec.Value = OK, when
		}
	}
	out = append(out, rec)

	fail := Check{ID: "update.failing", Group: GroupUpdates, Title: "Falhas do Windows Update",
		Sources: []tweaks.Source{SrcWUTroubleshoot}}
	switch {
	case !ok:
		fail = unread(fail, "O histórico do Windows Update não pôde ser lido.")
	case !u.Fail.IsZero() && u.Fail.After(u.Last):
		code := orDefault(u.FailCode, "sem código")
		days := int(now.Sub(u.Fail).Hours() / 24)
		fail.Status, fail.Severity = Attention, SevWarning
		fail.Value = fmt.Sprintf("KB%s falhou há %s (erro %s) e não foi instalada depois", u.FailKB, ageText(days), code)
		fail.Headline = "A última atualização do Windows falhou e não foi refeita"
		fail.Detail = "Reinicie o PC e tente de novo em Windows Update. Se repetir, rode a solução de problemas do Windows Update (Configurações > Sistema > Solução de problemas) e procure o código do erro no site da Microsoft."
		fail.Links = []Link{{Label: "Abrir o Windows Update", URL: "ms-settings:windowsupdate"}, {Label: "Abrir as soluções de problemas", URL: "ms-settings:troubleshoot"}}
	case !u.Fail.IsZero():
		fail.Status, fail.Value = OK, "a falha mais recente já foi resolvida por uma instalação posterior"
	default:
		fail.Status, fail.Value = OK, "nenhuma falha nos últimos 180 dias"
	}
	out = append(out, fail)
	return out
}

func diskChecks(f *facts.Facts) []Check {
	h := f.Health
	c := Check{ID: "disk.dirty", Group: GroupUpdates, Title: "Volumes marcados para verificação",
		Sources: []tweaks.Source{SrcFsutilDirty}}
	switch {
	case !h.DirtyRead || !f.Read(facts.StepHealth):
		return []Check{unread(c, "O Windows não informou o estado dos volumes.")}
	case len(h.Dirty) > 0:
		c.Status, c.Severity = Attention, SevWarning
		c.Value = "volume " + strings.Join(h.Dirty, ", ") + " marcado como sujo"
		c.Headline = "Volume " + strings.Join(h.Dirty, ", ") + " marcado para verificação de disco"
		c.Detail = "O Windows marca o volume como sujo quando ele não foi fechado direito (queda de energia, travamento) ou quando encontrou erros no sistema de arquivos. No próximo boot ele verifica o disco sozinho: reinicie e deixe terminar. Se o aviso voltar, confira a saúde do disco (a análise do Axion mostra o SMART) e faça backup."
	default:
		c.Status, c.Value = OK, "nenhum volume marcado"
	}
	return []Check{c}
}

func restartChecks(f *facts.Facts) []Check {
	var out []Check
	if f.Read(facts.StepOS) {
		rb := Check{ID: "restart.pending", Group: GroupUpdates, Title: "Reinício pendente", Covered: "os.reboot"}
		if f.OS.PendingReboot {
			rb.Status, rb.Severity, rb.Value = Attention, SevWarning, "o Windows aguarda um reinício"
			rb.Headline = "Reinício pendente"
			rb.Detail = "Atualizações pela metade podem causar lentidão e falhas. Reinicie antes de jogar ou otimizar."
		} else {
			rb.Status, rb.Value = OK, "nenhum"
		}
		out = append(out, rb)
		if !f.OS.LastBoot.IsZero() {
			up := Check{ID: "restart.uptime", Group: GroupUpdates, Title: "Tempo sem reiniciar", Covered: "os.uptime"}
			days := int(math.Round(f.OS.UptimeHours / 24)) // one number for the title and the value
			if f.OS.UptimeHours > 24*7 {
				up.Status, up.Severity = Attention, SevInfo
				up.Headline = fmt.Sprintf("%d dias sem um reinício completo", days)
				up.Detail = "Reiniciar de vez em quando libera memória presa por drivers e conclui atualizações."
			} else {
				up.Status = OK
			}
			up.Value = "último boot completo há " + ageText(days)
			out = append(out, up)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Battery
// ---------------------------------------------------------------------------

func batteryChecks(f *facts.Facts) []Check {
	if !f.Power.IsLaptop {
		return nil
	}
	b := f.Health.Battery
	c := Check{ID: "battery.wear", Group: GroupPower, Title: "Desgaste da bateria",
		Criterion: fmt.Sprintf("Critério do Axion: %d%% ou mais da capacidade original perdida (a bateria dura visivelmente menos).", BatteryWearPct),
		Sources:   []tweaks.Source{SrcBatteryReport}}
	if !b.Read || !f.Read(facts.StepHealth) {
		return []Check{unread(c, "O relatório de energia do Windows não trouxe a capacidade da bateria.")}
	}
	val := fmt.Sprintf("%s Wh de %s Wh originais (desgaste de %d%%", dec1(float64(b.FullMWh)/1000), dec1(float64(b.DesignMWh)/1000), b.WearPct)
	if b.Cycles > 0 {
		val += ", " + plural(b.Cycles, "ciclo", "ciclos")
	}
	val += ")"
	c.Value = val
	if b.WearPct >= BatteryWearPct {
		c.Status, c.Severity = Attention, SevWarning
		c.Headline = fmt.Sprintf("Bateria com %d%% de desgaste", b.WearPct)
		c.Detail = "A bateria já guarda bem menos carga do que guardava nova, então o notebook desliga mais cedo longe da tomada. Não afeta o desempenho na tomada. A troca é feita pelo fabricante ou por uma assistência; enquanto isso, jogar sempre na tomada poupa a bateria."
	} else {
		c.Status = OK
	}
	return []Check{c}
}

// ---------------------------------------------------------------------------
// Boot
// ---------------------------------------------------------------------------

// MedianBoot returns the median time until the desktop was usable over the
// newest boots (at most BootWindow), the boots counted and how many of them
// Windows itself flagged as slower than usual.
func MedianBoot(b facts.BootHealth) (medianMs, n, degraded int) {
	s := b.Samples
	if len(s) > BootWindow {
		s = s[:BootWindow]
	}
	if len(s) == 0 {
		return 0, 0, 0
	}
	v := make([]int, len(s))
	for i, x := range s {
		v[i] = x.MainMs
		if x.Degraded {
			degraded++
		}
	}
	sort.Ints(v)
	return v[len(v)/2], len(s), degraded
}

func bootChecks(f *facts.Facts) []Check {
	b := f.Health.Boot
	t := Check{ID: "boot.time", Group: GroupBoot, Title: "Tempo de inicialização",
		Criterion: fmt.Sprintf("Critério do Axion: %d s ou mais entre o logotipo do Windows e a área de trabalho (ou a tela de login), na mediana dos últimos boots. É o tempo que o próprio Windows registra em cada boot.", BootSlowMs/1000),
		Sources:   []tweaks.Source{SrcBootPerf}}
	apps := Check{ID: "boot.apps", Group: GroupBoot, Title: "Programas que atrasam a inicialização",
		Criterion: fmt.Sprintf("Critério do Axion: um programa que o Windows mediu com %d s ou mais de atraso em pelo menos %d boots.", facts.BootSlowAppMs/1000, facts.BootSlowAppBoots),
		Sources:   []tweaks.Source{SrcBootPerf}}
	if !b.Read || !f.Read(facts.StepHealth) {
		return []Check{unread(t, "O registro de desempenho de inicialização do Windows não pôde ser lido."),
			unread(apps, "O registro de desempenho de inicialização do Windows não pôde ser lido.")}
	}
	med, n, degraded := MedianBoot(b)
	switch {
	case n < BootMinSamples:
		t = unread(t, fmt.Sprintf("Só %s no registro; preciso de pelo menos %d para tirar uma média.", plural(n, "boot", "boots"), BootMinSamples))
	case med >= BootSlowMs:
		t.Status, t.Severity = Attention, SevInfo
		t.Value = fmt.Sprintf("%s s até o Windows ficar pronto (mediana de %d boots)", dec1(float64(med)/1000), n)
		t.Headline = fmt.Sprintf("Inicialização lenta: %s s até o Windows ficar pronto", dec1(float64(med)/1000))
		t.Detail = "Em disco rígido (HDD) isso é comum; trocar por um SSD é a melhora que mais se sente. Em SSD, costuma ser excesso de programas na inicialização: veja a página de Inicialização."
		t.Links = []Link{{Label: "Ver programas da inicialização", Page: "inicializacao"}}
	default:
		t.Status = OK
		t.Value = fmt.Sprintf("%s s até o Windows ficar pronto (mediana de %d boots)", dec1(float64(med)/1000), n)
	}
	if t.Status != Unread && degraded > 0 {
		t.Value += fmt.Sprintf("; o Windows marcou %d como mais lento que o normal", degraded)
	}
	switch {
	case n < BootMinSamples:
		apps = unread(apps, "Poucos boots no registro para dizer qual programa atrasa.")
	case len(b.Slow) > 0:
		var names []string
		for _, a := range b.Slow {
			names = append(names, fmt.Sprintf("%s (atrasou %d boots, até %s s)", a.Name, a.Boots, dec1(float64(a.WorstMs)/1000)))
		}
		apps.Status, apps.Severity = Attention, SevInfo
		apps.Value = strings.Join(names, "; ")
		apps.Headline = "Programas que atrasam a inicialização: " + shortNames(b.Slow)
		apps.Detail = "O Windows mediu estes programas demorando para iniciar durante o boot, em vários boots seguidos. Se não precisa deles já ao ligar, desative-os na inicialização."
		apps.Links = []Link{{Label: "Ver programas da inicialização", Page: "inicializacao"}}
	default:
		apps.Status, apps.Value = OK, "nenhum programa se destacou"
	}
	return []Check{t, apps}
}

func shortNames(a []facts.BootApp) string {
	var n []string
	for i, x := range a {
		if i == 3 {
			n = append(n, fmt.Sprintf("e mais %d", len(a)-3))
			break
		}
		n = append(n, x.Name)
	}
	return strings.Join(n, ", ")
}

package app

import (
	"bytes"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"time"

	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/netdiag"
	"axion/internal/perf"
	"axion/internal/profile"
	"axion/internal/tweaks"
)

// The HTML report (0.7): the diagnostic as a page anyone can open, to send
// to a friend or a technician, with the same evidence, explanations and
// sources the interface shows. Self-contained (inline styles, no scripts),
// anonymous unless the full version is asked for.

type htmlReport struct {
	Version   string
	Generated string
	Anonymous bool
	Profile   profile.Info
	Machine   []htmlKV
	Eval      *Evaluation
	Pillars   []htmlPillar
	Areas     []htmlArea
	Off       []diag.Finding
	Dismissed []diag.Finding
	Applied   []htmlApplied
	Kit       []htmlKit
	Net       *netdiag.Summary
	NetWhen   string
	Perf      *perf.Summary
	PerfWhen  string
	PerfGame  string
	Missing   []string
	Note      string
}

type htmlKV struct{ K, V string }

type htmlPillar struct {
	Title    string
	Overall  int
	Grade    string
	Summary  string
	Findings []diag.Finding
}

type htmlArea struct {
	Title    string
	Score    int
	Findings []diag.Finding
}

type htmlApplied struct {
	Title, Category, When, Status string
}

// htmlKit is one program of the saved selection of "Instalar programas".
type htmlKit struct {
	Name, Package, State string
}

var htmlFuncs = template.FuncMap{
	"sev": func(s string) string {
		switch s {
		case diag.SevCritical:
			return "Importante"
		case diag.SevWarning:
			return "Atenção"
		case diag.SevInfo:
			return "Sugestão"
		case diag.SevGood:
			return "Ok"
		}
		return s
	},
	"limiter": func(l string) string {
		switch l {
		case "gpu":
			return "a placa de vídeo"
		case "cpu":
			return "o lado da CPU (processador ou limite do jogo)"
		case "limite":
			return "o limite de FPS do jogo"
		case "misto":
			return "ora a CPU, ora a GPU"
		}
		return "não classificado"
	},
	"grade": func(n int) string {
		switch {
		case n < 0:
			return "sc-none"
		case n >= 90:
			return "sc-ok"
		case n >= 75:
			return "sc-info"
		case n >= 50:
			return "sc-warn"
		}
		return "sc-bad"
	},
}

var htmlTmpl = template.Must(template.New("r").Funcs(htmlFuncs).Parse(`<!doctype html>
<html lang="pt-BR"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Axion — relatório de {{.Generated}}</title>
<style>
body{font:15px/1.5 -apple-system,"Segoe UI",Roboto,sans-serif;color:#1b1f2a;background:#f5f6fa;margin:0;padding:24px}
main{max-width:980px;margin:0 auto}
h1{font-size:24px;margin:0 0 4px}h2{font-size:18px;margin:28px 0 10px;border-bottom:1px solid #d9dce6;padding-bottom:6px}h3{font-size:15px;margin:18px 0 8px}
.muted{color:#5b6274}.small{font-size:13px}
.card{background:#fff;border:1px solid #e1e4ee;border-radius:10px;padding:14px 16px;margin:10px 0}
.kv{display:grid;grid-template-columns:180px 1fr;gap:4px 12px}.kv b{font-weight:600}
.scores{display:grid;grid-template-columns:repeat(3,1fr);gap:12px}
.score{text-align:center}.score .n{font-size:40px;font-weight:700;line-height:1}.score .g{font-weight:600;margin-top:4px}
.sc-ok{color:#0f8a5f}.sc-info{color:#2f5fd0}.sc-warn{color:#b26a00}.sc-bad{color:#c62828}.sc-none{color:#8a90a3}
.f{border-left:4px solid #c9cedc;padding:8px 12px;margin:8px 0;background:#fff;border-radius:0 8px 8px 0}
.f.critical{border-color:#c62828}.f.warning{border-color:#e0a000}.f.info{border-color:#2f5fd0}.f.good{border-color:#0f8a5f}
.f .t{font-weight:600}.f .tag{display:inline-block;font-size:11px;padding:1px 7px;border-radius:9px;background:#eef0f6;margin-left:6px;vertical-align:middle}
.f .e{color:#39404f;font-size:13.5px;margin-top:2px}.f .x{font-size:13.5px;margin-top:4px}
.f .s a{font-size:12.5px;margin-right:10px}
table{border-collapse:collapse;width:100%;font-size:13.5px}td,th{text-align:left;padding:6px 8px;border-bottom:1px solid #e6e8f0}th{font-weight:600}
details summary{cursor:pointer;color:#2f5fd0}
footer{margin-top:30px;font-size:12.5px;color:#5b6274}
@media print{body{background:#fff;padding:0}.card{break-inside:avoid}}
</style></head><body><main>
<h1>Axion Optimizer — relatório do PC</h1>
<div class="muted">Gerado em {{.Generated}} pela versão {{.Version}} · perfil de uso: {{.Profile.Title}}{{if .Anonymous}} · relatório anônimo (nome do PC, conta, endereços e números de série removidos){{end}}</div>

<h2>O PC</h2>
<div class="card kv">{{range .Machine}}<b>{{.K}}</b><span>{{.V}}</span>{{end}}</div>

{{if .Eval}}<h2>As três notas</h2>
<div class="card"><div class="scores">{{range .Pillars}}<div class="score"><div class="n {{grade .Overall}}">{{if lt .Overall 0}}—{{else}}{{.Overall}}{{end}}</div><div class="g">{{.Title}} · {{.Grade}}</div><div class="small muted">{{.Summary}}</div></div>{{end}}</div>
<p class="small muted">{{.Note}}</p>{{if .Missing}}<p class="small"><b>Ainda não medido:</b> {{range $i, $m := .Missing}}{{if $i}}, {{end}}{{$m}}{{end}}.</p>{{end}}</div>{{end}}

<h2>Diagnóstico (configuração)</h2>
{{range .Areas}}<h3>{{.Title}} <span class="muted small">nota {{.Score}}</span></h3>
{{range .Findings}}<div class="f {{.Severity}}"><div class="t">{{.Title}}<span class="tag">{{sev .Severity}}</span></div>{{if .Evidence}}<div class="e">{{.Evidence}}</div>{{end}}{{if .Explanation}}<div class="x">{{.Explanation}}</div>{{end}}{{if .Sources}}<div class="s">{{range .Sources}}<a href="{{.URL}}">{{.Title}}</a>{{end}}</div>{{end}}</div>{{end}}{{end}}
{{if .Off}}<details><summary>Fora do perfil {{.Profile.Title}} ({{len .Off}}) — não contam na nota neste perfil</summary>{{range .Off}}<div class="f {{.Severity}}"><div class="t">{{.Title}}<span class="tag">{{sev .Severity}}</span></div>{{if .Evidence}}<div class="e">{{.Evidence}}</div>{{end}}{{if .Explanation}}<div class="x">{{.Explanation}}</div>{{end}}</div>{{end}}</details>{{end}}
{{if .Dismissed}}<details><summary>Dispensados pelo usuário ({{len .Dismissed}}) — não contam na nota</summary>{{range .Dismissed}}<div class="f {{.Severity}}"><div class="t">{{.Title}}<span class="tag">{{sev .Severity}}</span></div>{{if .Evidence}}<div class="e">{{.Evidence}}</div>{{end}}</div>{{end}}</details>{{end}}

{{range $p := .Pillars}}{{if ne $p.Title "Configuração"}}<h2>{{$p.Title}} <span class="muted small">{{if lt $p.Overall 0}}{{$p.Grade}}{{else}}nota {{$p.Overall}} · {{$p.Grade}}{{end}}</span></h2>{{if lt $p.Overall 0}}<p class="small muted">{{$p.Summary}}</p>{{end}}
{{range $p.Findings}}<div class="f {{.Severity}}"><div class="t">{{.Title}}<span class="tag">{{sev .Severity}}</span></div>{{if .Evidence}}<div class="e">{{.Evidence}}</div>{{end}}{{if .Explanation}}<div class="x">{{.Explanation}}</div>{{end}}{{if .Sources}}<div class="s">{{range .Sources}}<a href="{{.URL}}">{{.Title}}</a>{{end}}</div>{{end}}</div>{{end}}{{end}}{{end}}

{{if .Net}}<h2>Último teste de rede</h2><div class="card kv"><b>Quando</b><span>{{.NetWhen}}</span>{{if .Net.Grade}}<b>Bufferbloat</b><span>nota {{.Net.Grade}} (+{{printf "%.0f" .Net.AddedMs}} ms com a conexão cheia); {{printf "%.0f" .Net.DownMbps}} Mb/s de download, {{printf "%.0f" .Net.UpMbps}} Mb/s de upload</span>{{end}}{{if ge .Net.NetMs 0.0}}<b>Internet (1.1.1.1)</b><span>{{printf "%.1f" .Net.NetMs}} ms, jitter {{printf "%.1f" .Net.NetJitter}} ms, perda {{printf "%.1f" .Net.NetLoss}}%</span>{{end}}{{if ge .Net.ValveMs 0.0}}<b>Relay da Valve</b><span>{{.Net.ValvePOP}}: {{printf "%.1f" .Net.ValveMs}} ms, jitter {{printf "%.1f" .Net.ValveJit}} ms, perda {{printf "%.1f" .Net.ValveLoss}}%</span>{{end}}</div>{{end}}

{{if .Perf}}<h2>Última medição de FPS{{if .PerfGame}} ({{.PerfGame}}){{end}}</h2><div class="card kv"><b>Quando</b><span>{{.PerfWhen}}</span><b>FPS</b><span>{{printf "%.0f" .Perf.AvgFPS}} médio, 1% low {{printf "%.0f" .Perf.Low1FPS}}, 0,1% low {{printf "%.0f" .Perf.Low01FPS}}</span><b>Quadros</b><span>{{.Perf.Frames}} em {{printf "%.0f" .Perf.Seconds}} s; tempo de quadro médio {{printf "%.2f" .Perf.FTAvg}} ms, P99 {{printf "%.2f" .Perf.FTP99}} ms</span><b>Quem limitou</b><span>{{limiter .Perf.Limiter}}{{if ge .Perf.GPUBoundPct 0.0}} (GPU em {{printf "%.0f" .Perf.GPUBoundPct}}% dos quadros, CPU em {{printf "%.0f" .Perf.CPUBoundPct}}%){{end}}</span></div>{{end}}

{{if .Applied}}<h2>Otimizações aplicadas pelo Axion</h2><div class="card"><table><tr><th>Otimização</th><th>Categoria</th><th>Quando</th><th>Estado</th></tr>{{range .Applied}}<tr><td>{{.Title}}</td><td>{{.Category}}</td><td>{{.When}}</td><td>{{.Status}}</td></tr>{{end}}</table></div>{{end}}

{{if .Kit}}<h2>Kit de programas (Instalar programas)</h2><div class="card"><p class="small muted">A seleção salva na página Instalar programas, para ter a lista em mãos depois de formatar. Cada linha é um identificador do winget (<code>winget install --id &lt;identificador&gt; -e</code>) ou um produto da Microsoft Store.</p><table><tr><th>Programa</th><th>Pacote</th><th>Neste PC</th></tr>{{range .Kit}}<tr><td>{{.Name}}</td><td><code>{{.Package}}</code></td><td>{{.State}}</td></tr>{{end}}</table></div>{{end}}

<footer>Relatório gerado pelo Axion Optimizer. Cada ponto mostra a evidência lida do PC e as fontes; nenhuma otimização é aplicada sem confirmação e todas ficam no histórico com o valor anterior. A nota parte de 100 e perde 12 pontos por problema importante, 5 por ponto de atenção e 1 por sugestão.</footer>
</main></body></html>`))

// ExportHTML renders the report page. Unless full, the strings go through
// the anonymizer of the JSON export.
func (a *App) ExportHTML(full bool) ([]byte, error) {
	f := a.Facts()
	rep := a.Report()
	if f == nil || rep == nil {
		return nil, ErrNoScan
	}
	host, users := a.anonNames()
	an := newAnonymizer(host, users...)
	scrub := func(s string) string {
		if full {
			return s
		}
		return an.text(s, false)
	}
	scrubFinding := func(x diag.Finding) diag.Finding {
		x.Title, x.Evidence, x.Explanation = scrub(x.Title), scrub(x.Evidence), scrub(x.Explanation)
		return x
	}
	prof := a.profile()
	r := htmlReport{Version: a.Version, Generated: time.Now().Format("02/01/2006 15:04"), Anonymous: !full, Profile: prof, Note: evalNote}
	r.Machine = machineRows(f, full, an)
	if ev, err := a.Evaluation(); err == nil {
		r.Eval = ev
		r.Missing = ev.Missing
		for _, p := range []struct {
			title string
			rep   *diag.Report
		}{{"Configuração", ev.Config}, {"Capacidade", ev.Capacity}, {"Medição", ev.Measurement}} {
			if p.rep == nil {
				continue
			}
			hp := htmlPillar{Title: p.title, Overall: p.rep.Overall, Grade: p.rep.Grade, Summary: p.rep.Summary}
			if p.title != "Configuração" {
				for _, x := range p.rep.Findings {
					hp.Findings = append(hp.Findings, scrubFinding(x))
				}
			}
			r.Pillars = append(r.Pillars, hp)
		}
	}
	byArea := map[string][]diag.Finding{}
	for _, x := range rep.Findings {
		switch {
		case x.Dismissed:
			r.Dismissed = append(r.Dismissed, scrubFinding(x))
		case x.OffProfile:
			r.Off = append(r.Off, scrubFinding(x))
		default:
			byArea[x.Area] = append(byArea[x.Area], scrubFinding(x))
		}
	}
	for _, s := range rep.Scores {
		if len(byArea[s.Area]) == 0 {
			continue
		}
		r.Areas = append(r.Areas, htmlArea{Title: s.Title, Score: s.Score, Findings: byArea[s.Area]})
	}
	// Applied items come from the history, so the on-demand ones (per-game
	// profiles, the active hours) are listed with the catalog's.
	states := a.States()
	seen := map[string]bool{}
	for _, e := range a.Engine.J.Snapshot() { // newest first: one line per item
		if !e.Active() || seen[e.TweakID] {
			continue
		}
		seen[e.TweakID] = true
		title, cat := e.TweakTitle, CategoryTitle(e.Category)
		if title == "" {
			title = e.TweakID
		}
		status := "ativa"
		if st, ok := states[e.TweakID]; ok && st.EntryID == e.ID {
			switch {
			case st.Drifted:
				status = "desfeita pelo Windows ou por outro programa"
			case st.Effect == "reboot":
				status = "vale após reiniciar"
			case st.Effect == "ignored":
				status = "não está valendo"
			}
		} else if e.Status == tweaks.StatusFailed || e.Status == tweaks.StatusPartialRev {
			status = "com pendência (desfazer de novo)"
		}
		r.Applied = append(r.Applied, htmlApplied{Title: scrub(title), Category: cat, When: e.Time.Local().Format("02/01/2006 15:04"), Status: status})
	}
	sort.Slice(r.Applied, func(i, j int) bool { return r.Applied[i].Title < r.Applied[j].Title })
	if sel := a.Settings().Apps; len(sel) > 0 {
		by := map[string]AppRow{}
		for _, row := range a.Apps().Items {
			by[row.ID] = row
		}
		for _, id := range sel {
			row, ok := by[id]
			if !ok {
				continue
			}
			pkg := row.Winget
			if pkg == "" {
				pkg = row.Store + " (Microsoft Store)"
			}
			state := "não instalado"
			if row.Installed {
				state = "instalado"
				if row.Version != "" {
					state += " " + row.Version
				}
			}
			r.Kit = append(r.Kit, htmlKit{Name: row.Name, Package: pkg, State: state})
		}
	}
	if m := a.measured(); m != nil {
		if n := m.NetLatest; n != nil {
			if m.Net != nil && m.Net.Grade != "" {
				n = m.Net
			}
			r.Net, r.NetWhen = n, n.At.Local().Format("02/01/2006 15:04")
			if n.Label != "" {
				r.NetWhen += " (" + scrub(n.Label) + ")"
			}
		}
		if m.Perf != nil {
			r.Perf, r.PerfWhen, r.PerfGame = m.Perf, m.PerfAt.Local().Format("02/01/2006 15:04"), m.PerfGame
		}
	}
	var buf bytes.Buffer
	if err := htmlTmpl.Execute(&buf, r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// machineRows summarises the hardware for the page header.
func machineRows(f *facts.Facts, full bool, an *anonymizer) []htmlKV {
	var rows []htmlKV
	add := func(k, v string) {
		if v = strings.TrimSpace(v); v != "" {
			rows = append(rows, htmlKV{k, v})
		}
	}
	if full {
		add("Nome do PC", f.Hostname)
	}
	add("Windows", strings.TrimSpace(fmt.Sprintf("%s %s (build %d.%d)", f.OS.Caption, f.OS.DisplayVersion, f.OS.Build, f.OS.UBR)))
	add("Processador", fmt.Sprintf("%s — %d núcleos / %d threads", f.CPU.Name, f.CPU.Cores, f.CPU.Threads))
	if g := f.PrimaryGPU(); g != nil {
		v := g.Name
		if g.VRAMMiB > 0 {
			v += fmt.Sprintf(", %.0f GB", float64(g.VRAMMiB)/1024)
		}
		if g.VendorVersion != "" {
			v += ", driver " + g.VendorVersion
		} else if g.DriverVersion != "" {
			v += ", driver " + g.DriverVersion
		}
		add("Placa de vídeo", v)
	}
	if f.RAM.TotalBytes > 0 {
		add("Memória", fmt.Sprintf("%.0f GB %s a %d MT/s (%d pentes)", float64(f.RAM.TotalBytes)/(1<<30), f.RAM.Type, f.RAM.ConfiguredMTs, len(f.RAM.Modules)))
	}
	if b := f.Board; b.Product != "" || b.SystemModel != "" {
		v := strings.TrimSpace(b.Manufacturer + " " + b.Product)
		if b.SystemModel != "" && !strings.EqualFold(b.SystemModel, b.Product) {
			v = strings.TrimSpace(b.SystemMaker+" "+b.SystemModel) + " (" + v + ")"
		}
		if b.BIOSVersion != "" {
			v += ", BIOS " + b.BIOSVersion
		}
		add("Placa-mãe", v)
	}
	for _, d := range f.Disks {
		if d.IsSystem {
			add("Disco do sistema", fmt.Sprintf("%s (%s %s, %.0f GB)", d.Model, d.BusType, d.MediaType, float64(d.SizeBytes)/1e9))
		}
	}
	for _, d := range f.Displays {
		if d.Primary {
			add("Monitor", fmt.Sprintf("%s — %dx%d a %d Hz (máximo %d Hz)", d.Monitor, d.Width, d.Height, d.CurrentHz, d.MaxHzAtRes))
		}
	}
	for _, ad := range f.Adapters {
		if ad.Up && ad.HasGateway {
			v := ad.Kind + ": " + ad.Description
			if ad.LinkSpeedMbps > 0 {
				v += fmt.Sprintf(", %d Mb/s", ad.LinkSpeedMbps)
			}
			add("Rede", v)
			break
		}
	}
	sec := f.Security
	var s []string
	if sec.TPMReady {
		s = append(s, "TPM "+sec.TPMVersion)
	}
	s = append(s, "Secure Boot "+sec.SecureBoot)
	if sec.VBSRunning {
		s = append(s, "VBS ligado")
	}
	if len(f.Gaming.AntiCheats) > 0 {
		s = append(s, "anti-cheat: "+strings.Join(f.Gaming.AntiCheats, ", "))
	}
	add("Segurança", strings.Join(s, " · "))
	if f.Power.IsLaptop {
		add("Tipo", "notebook")
	}
	if !full {
		for i := range rows {
			rows[i].V = an.text(rows[i].V, false)
		}
	}
	return rows
}

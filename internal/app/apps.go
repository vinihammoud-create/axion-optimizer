package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"axion/internal/apps"
	"axion/internal/programs"
	"axion/internal/sys"
)

// The "Instalar programas" page (0.8): the catalog of apps installed
// through winget. The Axion never carries an installer; it runs the
// package manager that ships with Windows, with the newest version of
// each package, and keeps a log of what it installed.

// appsState caches what winget reported (reading it takes seconds).
type appsState struct {
	mu        sync.Mutex
	at        time.Time
	installed map[string]apps.Installed // lower-case id -> installed (with available version)
	regNames  []string                  // installed programs by name (registry), for what winget does not know
	err       string
}

// AppRow is one catalog item with its state on this PC.
type AppRow struct {
	apps.Item
	Preset    string `json:"preset"`              // marcado | sugerido | ""
	Installed bool   `json:"installed"`           //
	Version   string `json:"version,omitempty"`   // installed version (winget)
	Available string `json:"available,omitempty"` // newer version (winget)
	Source    string `json:"source,omitempty"`    // winget | registro
	Selected  bool   `json:"selected"`            // part of the saved selection
}

// AppsView is the page model.
type AppsView struct {
	Winget      WingetInfo        `json:"winget"`
	Profile     string            `json:"profile"`
	Categories  []apps.Category   `json:"categories"`
	Items       []AppRow          `json:"items"`
	Scanned     bool              `json:"scanned"`
	ScannedAt   time.Time         `json:"scannedAt"`
	ScanError   string            `json:"scanError,omitempty"`
	Log         []AppLogEntry     `json:"log"`
	Warning     string            `json:"warning,omitempty"`
	Honest      string            `json:"honest"`
	Sources     []sourceLink      `json:"sources"`
	Upgrades    int               `json:"upgrades"`              // packages winget can update (any, not only the catalog)
	UpgradeList []apps.Installed  `json:"upgradeList,omitempty"` // those packages, with versions (what "Atualizar tudo" changes)
	Selection   []string          `json:"selection"`
	HasSel      bool              `json:"hasSelection"`
	Detected    map[string]string `json:"detected,omitempty"` // why an item came checked (hardware)
}

type sourceLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// WingetInfo says whether the package manager is usable.
type WingetInfo struct {
	Available bool   `json:"available"`
	Path      string `json:"path,omitempty"`
	Why       string `json:"why,omitempty"`
}

// AppLogEntry is one install the Axion ran.
type AppLogEntry struct {
	At      time.Time `json:"at"`
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Package string    `json:"package"`
	Action  string    `json:"action"` // install | upgrade | upgrade-all
	Outcome string    `json:"outcome"`
	OK      bool      `json:"ok"`
	Code    uint32    `json:"code,omitempty"`
}

var appsSources = []sourceLink{
	{Title: "Microsoft Learn — winget: o gerenciador de pacotes do Windows", URL: "https://learn.microsoft.com/windows/package-manager/winget/"},
	{Title: "Microsoft Learn — winget install (opções --id, -e, --silent, aceite de termos)", URL: "https://learn.microsoft.com/windows/package-manager/winget/install"},
	{Title: "Microsoft Learn — winget upgrade (--all)", URL: "https://learn.microsoft.com/windows/package-manager/winget/upgrade"},
	{Title: "winget-cli (GitHub) — códigos de retorno", URL: "https://github.com/microsoft/winget-cli/blob/master/doc/windows/package-manager/winget/returnCodes.md"},
	{Title: "winstall.app — espelho do catálogo winget-pkgs, onde os identificadores foram conferidos", URL: "https://winstall.app/"},
}

const appsHonest = "O Axion não traz instalador nenhum dentro dele: cada item é um identificador do catálogo do winget (o gerenciador de pacotes da Microsoft que já vem no Windows 10 e 11), um produto da Microsoft Store ou o link oficial do fabricante. O winget baixa o instalador do próprio fabricante, confere o hash SHA-256 dele com o que está no catálogo e instala em silêncio (os itens da Store passam pela própria Store). Ao rodar o winget, o Axion aceita por você os termos da fonte (winget e Store) e a licença de cada pacote, como a linha de comando faria com --accept-source-agreements e --accept-package-agreements. \"Instalar marcados\" só instala o que você marcou; \"Atualizar tudo\" atualiza todo programa que o winget reconhece neste PC, do catálogo ou não, e a lista aparece antes de confirmar. Os apps pagos exigem a sua licença. Ler a lista de instalados e instalar usam a internet (o winget atualiza o índice do catálogo e baixa de cada fabricante)."

func (a *App) appsDir() string { return filepath.Join(a.P.DataDir, "apps") }

// wingetPath finds winget.exe in the App Installer package folder (under
// Program Files\WindowsApps, which only TrustedInstaller writes), x64 or
// ARM64, with its Microsoft signature checked. The app execution alias in
// the user's WindowsApps folder is never used: that folder is writable
// without administrator rights and the alias's target is not checked, so
// it could send an elevated Axion to a program planted by anyone.
func (a *App) wingetPath() (string, error) {
	if a.P.IsMock {
		return "winget.exe", nil
	}
	paths := a.P.Native.Paths()
	if paths.ProgramFiles != "" {
		var m []string
		for _, arch := range []string{"x64", "arm64"} {
			found, _ := filepath.Glob(filepath.Join(paths.ProgramFiles, "WindowsApps", "Microsoft.DesktopAppInstaller_*_"+arch+"__8wekyb3d8bbwe", "winget.exe"))
			m = append(m, found...)
		}
		if len(m) > 0 {
			sort.SliceStable(m, func(i, j int) bool { return versionLess(packageVersion(m[j]), packageVersion(m[i])) }) // newest first
			for _, p := range m {
				st, err := os.Lstat(p)
				if err != nil || !st.Mode().IsRegular() {
					continue
				}
				if sig, err := a.P.Native.VerifySignature(p); err == nil && strings.Contains(strings.ToLower(sig.Signer), "microsoft") && (sig.Trusted || sig.RevocationOffline) {
					return p, nil
				}
			}
		}
	}
	return "", errors.New("o winget (Instalador de Aplicativo) não foi encontrado neste Windows: instale \"Instalador de Aplicativo\" pela Microsoft Store (aka.ms/getwinget) ou abra o Windows Update; num Windows recém-instalado ele aparece alguns minutos depois do primeiro login")
}

// packageVersion reads the version out of an App Installer package folder
// name (Microsoft.DesktopAppInstaller_1.26.430.0_x64__8wekyb3d8bbwe).
func packageVersion(p string) []int {
	dir := filepath.Base(filepath.Dir(p))
	_, rest, ok := strings.Cut(dir, "_")
	if !ok {
		return nil
	}
	ver, _, _ := strings.Cut(rest, "_")
	var out []int
	for _, part := range strings.Split(ver, ".") {
		n, _ := strconv.Atoi(part)
		out = append(out, n)
	}
	return out
}

// versionLess compares dotted versions numerically.
func versionLess(a, b []int) bool {
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return x < y
		}
	}
	return false
}

// winget runs the package manager with a log of its lines.
func (a *App) winget(ctx context.Context, args []string, timeout time.Duration, onLine func(string)) (sys.Result, error) {
	exe, err := a.wingetPath()
	if err != nil {
		return sys.Result{ExitCode: -1}, err
	}
	return a.P.Run.Run(ctx, exe, args, sys.RunOpts{Timeout: timeout, OnLine: func(l string) {
		l = strings.TrimSpace(l)
		if l == "" || isSpinner(l) {
			return
		}
		if onLine != nil {
			onLine(l)
		}
	}})
}

// isSpinner drops the progress lines winget redraws (spinner and bar).
func isSpinner(l string) bool {
	switch l {
	case "-", "\\", "|", "/":
		return true
	}
	return strings.HasPrefix(l, "█") || strings.HasPrefix(l, "▒")
}

// readInstalled asks winget what is installed and what has a newer
// version, plus the installed programs of the registry for the items the
// package manager does not know.
func (a *App) readInstalled(ctx context.Context, c *JobCtl) error {
	st := &a.appsSt
	if err := os.MkdirAll(a.appsDir(), 0o755); err != nil {
		return err
	}
	out := filepath.Join(a.appsDir(), "export.json")
	_ = os.Remove(out)
	logf := func(l string) {
		if c != nil {
			c.Logf("winget: %s", l)
		}
	}
	if c != nil {
		c.Progress(0.1, "Lendo o que o winget tem instalado")
	}
	res, err := a.winget(ctx, apps.ExportArgs(out), 4*time.Minute, logf)
	if err != nil {
		return err
	}
	installed := map[string]apps.Installed{}
	if b, rerr := os.ReadFile(out); rerr == nil {
		if m, perr := apps.ParseExport(b); perr == nil {
			installed = m
		} else {
			return fmt.Errorf("a lista do winget não pôde ser lida: %v", perr)
		}
	} else if res.ExitCode != 0 {
		return fmt.Errorf("winget export: %s", apps.Classify(res.ExitCode, apps.LastLine(res.Stdout+"\n"+res.Stderr)).Text)
	}
	_ = os.Remove(out)
	if c != nil {
		c.Progress(0.6, "Conferindo atualizações disponíveis")
	}
	if res, err := a.winget(ctx, apps.UpgradeListArgs(), 4*time.Minute, logf); err == nil {
		for id, up := range apps.ParseUpgradeTable(res.Stdout) {
			cur, ok := installed[id]
			if !ok {
				cur = apps.Installed{ID: up.ID}
			}
			if cur.Version == "" {
				cur.Version = up.Version
			}
			cur.Available = up.Available
			installed[id] = cur
		}
	} else if c != nil {
		c.Logf("aviso: lista de atualizações: %v", err)
	}
	var names []string
	for _, p := range programs.List(a.P.Reg) {
		names = append(names, p.Name)
	}
	st.mu.Lock()
	st.installed, st.regNames, st.at, st.err = installed, names, time.Now(), ""
	st.mu.Unlock()
	return nil
}

// StartAppsRefresh reads the installed packages in a job.
func (a *App) StartAppsRefresh() (Job, error) {
	if _, err := a.wingetPath(); err != nil {
		return Job{}, fmt.Errorf("%w: %v", errBadRequest, err)
	}
	return a.Jobs.Start("apps-refresh", "Lendo os programas instalados", true, func(ctx context.Context, c *JobCtl) (any, error) {
		if err := a.readInstalled(ctx, c); err != nil {
			a.appsSt.mu.Lock()
			a.appsSt.err = err.Error()
			a.appsSt.mu.Unlock()
			return nil, err
		}
		v := a.Apps()
		c.Logf("%d itens do catálogo instalados, %d com atualização", countInstalled(v.Items), v.Upgrades)
		return map[string]int{"installed": countInstalled(v.Items), "upgrades": v.Upgrades}, nil
	})
}

func countInstalled(rows []AppRow) int {
	n := 0
	for _, r := range rows {
		if r.Installed {
			n++
		}
	}
	return n
}

// Apps builds the page model from the catalog, the profile, the hardware
// and what winget reported.
func (a *App) Apps() AppsView {
	v := AppsView{Categories: apps.Categories, Profile: a.profile().ID, Honest: appsHonest, Sources: appsSources, Log: a.appsLog(), Detected: map[string]string{}}
	if p, err := a.wingetPath(); err != nil {
		v.Winget = WingetInfo{Why: err.Error()}
	} else {
		v.Winget = WingetInfo{Available: true, Path: p}
	}
	f := a.Facts()
	var periphs []apps.Periph
	laptopMaker, gpuVendor := "", ""
	if f != nil {
		for _, d := range f.Peripherals.Devices {
			if d.VID != 0 {
				periphs = append(periphs, apps.Periph{VID: d.VID, PID: d.PID, Name: d.Name})
			}
		}
		if f.IsLaptop() {
			laptopMaker = strings.ToLower(f.Board.SystemMaker + " " + f.Board.Manufacturer)
		}
		if g := f.PrimaryGPU(); g != nil {
			gpuVendor = g.Vendor
		}
		if f.OS.ElevatedAs != "" {
			v.Warning = "O Axion está elevado com outra conta de administrador (" + f.OS.ElevatedAs + "): programas instalados \"só para este usuário\" (Discord, Spotify, VS Code) iriam para essa conta, não para a sua. Abra o Axion com a sua própria conta de administrador para instalar."
		}
	}
	sel := a.Settings().Apps
	selected := map[string]bool{}
	for _, id := range sel {
		selected[id] = true
	}
	v.Selection, v.HasSel = sel, len(sel) > 0
	st := &a.appsSt
	st.mu.Lock()
	installed, names, at, serr := st.installed, st.regNames, st.at, st.err
	st.mu.Unlock()
	v.Scanned, v.ScannedAt, v.ScanError = installed != nil, at, serr
	for id, x := range installed {
		if x.Available != "" && id != "" {
			v.Upgrades++
			v.UpgradeList = append(v.UpgradeList, x)
		}
	}
	sort.Slice(v.UpgradeList, func(i, j int) bool {
		return strings.ToLower(v.UpgradeList[i].ID) < strings.ToLower(v.UpgradeList[j].ID)
	})
	for _, it := range apps.All {
		row := AppRow{Item: it, Preset: it.Preset(v.Profile, periphs, laptopMaker, gpuVendor), Selected: selected[it.ID]}
		for _, p := range periphs {
			if it.Matches(p) {
				v.Detected[it.ID] = fmt.Sprintf("%s (USB %04X:%04X) detectado", strings.TrimSpace(p.Name), p.VID, p.PID)
				break
			}
		}
		if it.LaptopMatch(laptopMaker) {
			v.Detected[it.ID] = "notebook " + strings.TrimSpace(f.Board.SystemMaker) + " detectado"
		}
		if (it.ID == "nvidia" && gpuVendor == "NVIDIA") || (it.ID == "amd" && gpuVendor == "AMD") {
			v.Detected[it.ID] = "placa de vídeo " + gpuVendor + " detectada"
		}
		for _, key := range []string{it.Winget, it.Store} {
			if key == "" {
				continue
			}
			if x, ok := installed[strings.ToLower(key)]; ok {
				row.Installed, row.Version, row.Available, row.Source = true, x.Version, x.Available, "winget"
			}
		}
		if !row.Installed {
			for _, n := range names {
				if it.NameMatch(n) {
					row.Installed, row.Source = true, "registro"
					break
				}
			}
		}
		if row.Installed && row.Available == "" {
			row.Selected = false // nothing to do for it
		}
		v.Items = append(v.Items, row)
	}
	return v
}

// appsExport is the "apps" part of the JSON report: the saved kit (ids and
// packages, so the list survives a format) and the installs the Axion ran.
func (a *App) appsExport() map[string]any {
	sel := a.Settings().Apps
	var kit []map[string]string
	for _, id := range sel {
		it, ok := apps.Get(id)
		if !ok {
			continue
		}
		kit = append(kit, map[string]string{"id": it.ID, "name": it.Name, "winget": it.Winget, "store": it.Store})
	}
	return map[string]any{"selection": kit, "log": a.appsLog()}
}

// AppsSelectRequest saves the user's selection (the kit for the next
// format).
type AppsSelectRequest struct {
	IDs []string `json:"ids"`
}

// SaveAppsSelection stores the selection.
func (a *App) SaveAppsSelection(req AppsSelectRequest) error {
	ids := uniqueIDs(req.IDs)
	sort.Strings(ids)
	return a.UpdateSettings(func(s *Settings) { s.Apps = ids })
}

// uniqueIDs keeps the catalog ids of a list, once each, in order.
func uniqueIDs(list []string) []string {
	var ids []string
	seen := map[string]bool{}
	for _, id := range list {
		if _, ok := apps.Get(id); ok && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// AppsInstallRequest names the catalog items to install (or update).
type AppsInstallRequest struct {
	IDs []string `json:"ids"`
}

// AppInstallResult is the outcome of one item.
type AppInstallResult struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Action  string `json:"action"` // install | upgrade | skipped | manual
	Outcome string `json:"outcome"`
	OK      bool   `json:"ok"`
	Reboot  bool   `json:"reboot"`
	Partial bool   `json:"partial,omitempty"` // upgrade --all: some packages failed
	Code    uint32 `json:"code,omitempty"`
}

// AppsInstallSummary is the job result.
type AppsInstallSummary struct {
	Results []AppInstallResult `json:"results"`
	OK      int                `json:"ok"`
	Failed  int                `json:"failed"`
	Skipped int                `json:"skipped"`
	Reboot  bool               `json:"reboot"`
	Seconds float64            `json:"seconds"`
}

// perItemTimeout bounds one installer; heavy packages (Office, with a
// download of gigabytes) get longer. When it expires the Axion stops
// waiting for winget, but the installer winget started may keep running.
const (
	perItemTimeout      = 45 * time.Minute
	perHeavyItemTimeout = 2 * time.Hour
)

// StartAppsInstall installs (or updates) the items in a job, one at a
// time. Cancelling never interrupts an installer (killing one half-way
// leaves a broken program): the current item finishes and the job stops
// before the next, with the results so far.
func (a *App) StartAppsInstall(req AppsInstallRequest) (Job, error) {
	if err := a.requireAdmin("instalar programas"); err != nil {
		return Job{}, err
	}
	if _, err := a.wingetPath(); err != nil {
		return Job{}, fmt.Errorf("%w: %v", errBadRequest, err)
	}
	var items []apps.Item
	for _, id := range req.IDs {
		if _, ok := apps.Get(id); !ok {
			return Job{}, fmt.Errorf("%w: item desconhecido: %s", errBadRequest, id)
		}
	}
	for _, id := range uniqueIDs(req.IDs) {
		it, _ := apps.Get(id)
		if it.Winget == "" && it.Store == "" {
			continue // manual link: nothing to run
		}
		items = append(items, it)
	}
	if len(items) == 0 {
		return Job{}, fmt.Errorf("%w: nenhum item instalável marcado (os itens com link oficial são instalados pelo site do fabricante)", errBadRequest)
	}
	if f := a.Facts(); f != nil && f.OS.ElevatedAs != "" {
		return Job{}, fmt.Errorf("%w: o Axion está elevado com outra conta (%s); abra-o com a sua própria conta de administrador para instalar programas", errBadRequest, f.OS.ElevatedAs)
	}
	return a.Jobs.Start("apps-install", "Instalando programas", true, func(ctx context.Context, c *JobCtl) (any, error) {
		start := time.Now()
		sum := &AppsInstallSummary{}
		a.appsSt.mu.Lock()
		installed := a.appsSt.installed
		a.appsSt.mu.Unlock()
		stop := context.AfterFunc(ctx, func() {
			c.Step("cancelamento pedido: o instalador atual termina e a tarefa para antes do próximo")
		})
		defer stop()
		for i, it := range items {
			if ctx.Err() != nil {
				c.Logf("cancelado antes de %s", it.Name)
				break
			}
			key := strings.ToLower(it.Winget)
			if it.Store != "" {
				key = strings.ToLower(it.Store)
			}
			pkg := it.Winget
			if pkg == "" {
				pkg = it.Store
			}
			action, args := "install", apps.InstallArgs(it)
			if cur, ok := installed[key]; ok {
				if cur.Available == "" {
					sum.Results = append(sum.Results, AppInstallResult{ID: it.ID, Name: it.Name, Action: "skipped", Outcome: "já instalado", OK: true})
					sum.Skipped++
					c.Logf("%s: já instalado", it.Name)
					continue
				}
				action, args = "upgrade", apps.UpgradeArgs(it)
			}
			c.Progress(float64(i)/float64(len(items)), fmt.Sprintf("%s (%d de %d): %s", map[string]string{"install": "Instalando", "upgrade": "Atualizando"}[action], i+1, len(items), it.Name))
			c.Logf("winget %s", strings.Join(args, " "))
			// The entry goes to the log before the installer starts, so a
			// closed Axion still leaves a trace of what was running.
			pending := AppLogEntry{At: time.Now(), ID: it.ID, Name: it.Name, Package: pkg, Action: action, Outcome: "em andamento (o Axion foi fechado antes de terminar?)"}
			a.appendAppsLog(pending)
			timeout := perItemTimeout
			if it.Heavy {
				timeout = perHeavyItemTimeout
			}
			res, err := a.winget(context.WithoutCancel(ctx), args, timeout, func(l string) { c.Logf("  %s", l) })
			r := AppInstallResult{ID: it.ID, Name: it.Name, Action: action}
			switch {
			case errors.Is(err, sys.ErrTimeout):
				r.Outcome = fmt.Sprintf("o winget não terminou em %s; o instalador de %s pode ainda estar rodando, confira na página Programas daqui a pouco", timeout, it.Name)
			case err != nil:
				r.Outcome = err.Error()
			default:
				o := apps.Classify(res.ExitCode, apps.LastLine(res.Stdout+"\n"+res.Stderr))
				r.Outcome, r.OK, r.Reboot, r.Code = o.Text, o.OK, o.Reboot, o.Code
				if o.Skipped {
					r.Action = "skipped"
				}
			}
			switch {
			case r.Action == "skipped":
				sum.Skipped++
			case r.OK:
				sum.OK++
			default:
				sum.Failed++
			}
			if r.Reboot {
				sum.Reboot = true
			}
			c.Logf("%s: %s", it.Name, r.Outcome)
			sum.Results = append(sum.Results, r)
			a.updateAppsLog(pending, AppLogEntry{At: pending.At, ID: it.ID, Name: it.Name, Package: pkg, Action: r.Action, Outcome: r.Outcome, OK: r.OK, Code: r.Code})
		}
		sum.Seconds = time.Since(start).Seconds()
		c.Progress(0.97, "Relendo os programas instalados")
		if err := a.readInstalled(context.WithoutCancel(ctx), nil); err != nil {
			c.Logf("aviso: releitura dos instalados: %v", err)
		}
		if ctx.Err() != nil {
			return sum, ctx.Err()
		}
		return sum, nil
	})
}

// StartAppsUpgradeAll updates every package winget knows a newer version of.
func (a *App) StartAppsUpgradeAll() (Job, error) {
	if err := a.requireAdmin("atualizar programas"); err != nil {
		return Job{}, err
	}
	if _, err := a.wingetPath(); err != nil {
		return Job{}, fmt.Errorf("%w: %v", errBadRequest, err)
	}
	if f := a.Facts(); f != nil && f.OS.ElevatedAs != "" {
		return Job{}, fmt.Errorf("%w: o Axion está elevado com outra conta (%s); abra-o com a sua própria conta de administrador para atualizar programas", errBadRequest, f.OS.ElevatedAs)
	}
	return a.Jobs.Start("apps-upgrade", "Atualizando todos os programas", true, func(ctx context.Context, c *JobCtl) (any, error) {
		start := time.Now()
		c.Progress(0.05, "winget upgrade --all: cada programa na versão mais nova do catálogo")
		c.NoCancel("aguardando os instaladores terminarem")
		pending := AppLogEntry{At: time.Now(), ID: "upgrade-all", Name: "Atualizar tudo", Package: "--all", Action: "upgrade-all", Outcome: "em andamento (o Axion foi fechado antes de terminar?)"}
		a.appendAppsLog(pending)
		res, err := a.winget(context.WithoutCancel(ctx), apps.UpgradeAllArgs(), 2*time.Hour, func(l string) {
			c.Logf("  %s", l)
			if strings.HasPrefix(l, "Found ") || strings.HasPrefix(l, "Encontrado") {
				c.Step(l)
			}
		})
		c.NoCancel("")
		sum := &AppsInstallSummary{Seconds: time.Since(start).Seconds()}
		r := AppInstallResult{ID: "upgrade-all", Name: "Atualizar tudo", Action: "upgrade-all"}
		if errors.Is(err, sys.ErrTimeout) {
			r.Outcome = "o winget não terminou em 2 horas; um instalador pode ainda estar rodando"
			sum.Failed = 1
		} else if err != nil {
			r.Outcome = err.Error()
			sum.Failed = 1
		} else {
			o := apps.Classify(res.ExitCode, apps.LastLine(res.Stdout+"\n"+res.Stderr))
			r.Outcome, r.OK, r.Reboot, r.Code, r.Partial = o.Text, o.OK, o.Reboot, o.Code, o.Partial
			switch {
			case r.OK:
				sum.OK = 1
			case r.Partial:
				sum.OK, sum.Failed = 1, 1
			default:
				sum.Failed = 1
			}
			sum.Reboot = o.Reboot
		}
		sum.Results = []AppInstallResult{r}
		c.Logf("atualizar tudo: %s", r.Outcome)
		a.updateAppsLog(pending, AppLogEntry{At: pending.At, ID: "upgrade-all", Name: "Atualizar tudo", Package: "--all", Action: "upgrade-all", Outcome: r.Outcome, OK: r.OK || r.Partial, Code: r.Code})
		c.Progress(0.97, "Relendo os programas instalados")
		if err := a.readInstalled(context.WithoutCancel(ctx), nil); err != nil {
			c.Logf("aviso: releitura dos instalados: %v", err)
		}
		return sum, nil
	})
}

func (a *App) appsLogPath() string { return filepath.Join(a.appsDir(), "instalados.json") }

// appsLog returns what the Axion installed, newest first.
func (a *App) appsLog() []AppLogEntry {
	b, err := os.ReadFile(a.appsLogPath())
	if err != nil {
		return []AppLogEntry{}
	}
	var out []AppLogEntry
	if json.Unmarshal(b, &out) != nil {
		return []AppLogEntry{}
	}
	return out
}

const appsLogKeep = 200

// updateAppsLog replaces the pending entry of an item with its outcome.
func (a *App) updateAppsLog(pending, final AppLogEntry) {
	list := a.appsLog()
	found := false
	for i := range list {
		if list[i].ID == pending.ID && list[i].At.Equal(pending.At) {
			list[i], found = final, true
			break
		}
	}
	if !found {
		list = append([]AppLogEntry{final}, list...)
	}
	a.saveAppsLog(list)
}

func (a *App) appendAppsLog(e AppLogEntry) {
	a.saveAppsLog(append([]AppLogEntry{e}, a.appsLog()...))
}

func (a *App) saveAppsLog(list []AppLogEntry) {
	if len(list) > appsLogKeep {
		list = list[:appsLogKeep]
	}
	_ = os.MkdirAll(a.appsDir(), 0o755)
	if err := saveFile(a.appsLogPath(), func() ([]byte, error) { return json.MarshalIndent(list, "", " ") }); err != nil {
		log.Printf("registro de instalações não salvo: %v", err)
	}
}

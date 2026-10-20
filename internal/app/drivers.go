package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"axion/internal/drivers"
	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

var (
	srcWUASpecific = tweaks.Source{Title: "Microsoft Learn — Windows Update Agent: procurar, baixar e instalar atualizações específicas", URL: "https://learn.microsoft.com/windows/win32/wua_sdk/searching--downloading--and-installing-specific-updates"}
	srcWUASearch   = tweaks.Source{Title: "Microsoft Learn — IUpdateSearcher::Search (critérios Type='Driver', IsHidden, UpdateID)", URL: "https://learn.microsoft.com/windows/win32/api/wuapi/nf-wuapi-iupdatesearcher-search"}
	srcDriverRank  = tweaks.Source{Title: "Microsoft Learn — Como o Windows classifica drivers (assinatura e correspondência de ID)", URL: "https://learn.microsoft.com/windows-hardware/drivers/install/how-setup-ranks-drivers--windows-vista-and-later-"}
	srcDriverPick  = tweaks.Source{Title: "Microsoft Learn — Como o Windows escolhe drivers (data e versão desempatam)", URL: "https://learn.microsoft.com/windows-hardware/drivers/install/how-windows-selects-drivers"}
	srcAuthentic   = tweaks.Source{Title: "Microsoft Learn — WinVerifyTrust (verificação de assinatura Authenticode)", URL: "https://learn.microsoft.com/windows/win32/api/wintrust/nf-wintrust-winverifytrust"}
	srcNvidiaDrv   = tweaks.Source{Title: "NVIDIA — Download de drivers (mesma busca usada pelo Axion)", URL: "https://www.nvidia.com/en-us/drivers/"}
	srcKudu        = tweaks.Source{Title: "Kudu (MIT) — projeto aberto analisado como referência de interface", URL: "https://github.com/adventdevinc/kudu"}
)

// Vendor pages for GPUs without an automatic check.
var gpuVendorLinks = map[string][]Link{
	"AMD":   {{Label: "AMD Software: Adrenalin (detecção automática)", URL: "https://www.amd.com/en/support/download/drivers.html"}},
	"Intel": {{Label: "Intel Driver & Support Assistant", URL: "https://www.intel.com/content/www/us/en/support/intel-driver-support-assistant.html"}},
}

// Link is a labelled external page.
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// OfferStatus remembers what happened to an offer in this session.
type OfferStatus struct {
	Status string    `json:"status"` // baixado, instalado, falhou
	Detail string    `json:"detail,omitempty"`
	At     time.Time `json:"at"`
}

// GPUCheck is the vendor check of the main graphics card.
type GPUCheck struct {
	Vendor           string                `json:"vendor"`
	Name             string                `json:"name"`
	InstalledVersion string                `json:"installedVersion"`
	InstalledDate    time.Time             `json:"installedDate"`
	CheckedAt        time.Time             `json:"checkedAt"`
	Latest           *drivers.NvidiaDriver `json:"latest,omitempty"`
	// UpToDate is nil when the newest version is unknown (no check yet, or
	// the vendor lookup failed): neither "in date" nor "outdated".
	UpToDate    *bool  `json:"upToDate,omitempty"`
	Error       string `json:"error,omitempty"`
	File        string `json:"file,omitempty"` // verified installer ready to run
	FileVersion string `json:"fileVersion,omitempty"`
	Signer      string `json:"signer,omitempty"`
	Links       []Link `json:"links,omitempty"`
}

// driverState is persisted in drivers.json.
type driverState struct {
	SearchedAt  time.Time              `json:"searchedAt"`
	SearchError string                 `json:"searchError,omitempty"`
	Updates     []sys.DriverUpdate     `json:"updates"`
	Status      map[string]OfferStatus `json:"status"`
	GPU         *GPUCheck              `json:"gpu,omitempty"`
	Reboot      bool                   `json:"reboot"`
}

// BoardInfo points to the motherboard maker for chipset drivers and BIOS.
type BoardInfo struct {
	Maker       string    `json:"maker"`
	Model       string    `json:"model"`
	BIOSVersion string    `json:"biosVersion"`
	BIOSDate    time.Time `json:"biosDate"`
	Support     *Link     `json:"support,omitempty"`
	Chipset     *Link     `json:"chipset,omitempty"`
}

// DriversView is the Drivers page model.
type DriversView struct {
	Elevated       bool                `json:"elevated"`
	SearchedAt     time.Time           `json:"searchedAt"`
	SearchError    string              `json:"searchError,omitempty"`
	Offers         []drivers.Offer     `json:"offers"`
	Hidden         []drivers.Offer     `json:"hidden"`
	Components     []drivers.Component `json:"components"`
	InventoryError string              `json:"inventoryError,omitempty"`
	GPU            *GPUCheck           `json:"gpu,omitempty"`
	Board          BoardInfo           `json:"board"`
	Reboot         bool                `json:"reboot"`
	Notes          []string            `json:"notes"`
	Sources        []tweaks.Source     `json:"sources"`
}

// driverStore keeps the Drivers page state in memory and on disk.
type driverStore struct {
	mu     sync.Mutex
	loaded bool
	st     driverState
	inv    []drivers.Installed
	invAt  time.Time
}

// NvidiaEndpoints are NVIDIA's endpoints and the HTTP client (replaceable in tests).
type NvidiaEndpoints struct {
	ProductsURL string
	LookupURL   string
	Client      *http.Client
}

func (a *App) nvidia() NvidiaEndpoints {
	n := a.NvidiaAPI
	if n.ProductsURL == "" {
		n.ProductsURL = drivers.NvidiaProductsURL
	}
	if n.LookupURL == "" {
		n.LookupURL = drivers.NvidiaLookupURL
	}
	if n.Client == nil {
		n.Client = &http.Client{Timeout: 30 * time.Second}
	}
	return n
}

func (a *App) driversPath() string { return filepath.Join(a.P.DataDir, "drivers.json") }

func (a *App) driverStateCopy() driverState {
	a.drv.mu.Lock()
	defer a.drv.mu.Unlock()
	a.loadDriverStateLocked()
	st := a.drv.st
	st.Updates = append([]sys.DriverUpdate(nil), st.Updates...)
	st.Status = make(map[string]OfferStatus, len(a.drv.st.Status))
	for k, v := range a.drv.st.Status {
		st.Status[k] = v
	}
	if a.drv.st.GPU != nil {
		g := *a.drv.st.GPU
		st.GPU = &g
	}
	return st
}

func (a *App) loadDriverStateLocked() {
	if a.drv.loaded {
		return
	}
	a.drv.loaded = true
	if b, err := os.ReadFile(a.driversPath()); err == nil {
		_ = json.Unmarshal(b, &a.drv.st)
	}
	if a.drv.st.Status == nil {
		a.drv.st.Status = map[string]OfferStatus{}
	}
}

// updateDriverState changes the state under the lock and saves it.
func (a *App) updateDriverState(fn func(st *driverState)) {
	a.drv.mu.Lock()
	a.loadDriverStateLocked()
	fn(&a.drv.st)
	a.drv.mu.Unlock()
	if err := saveFile(a.driversPath(), func() ([]byte, error) {
		a.drv.mu.Lock()
		defer a.drv.mu.Unlock()
		return json.MarshalIndent(a.drv.st, "", " ")
	}); err != nil {
		log.Printf("estado dos drivers não foi salvo: %v", err)
	}
}

// inventory reads the installed drivers (cached for a few minutes).
func (a *App) inventory(force bool) ([]drivers.Installed, error) {
	a.drv.mu.Lock()
	if !force && a.drv.inv != nil && time.Since(a.drv.invAt) < 10*time.Minute {
		inv := a.drv.inv
		a.drv.mu.Unlock()
		return inv, nil
	}
	a.drv.mu.Unlock()
	rows, err := a.P.WMI.Query(`root\cimv2`, drivers.InventoryWQL)
	if err != nil {
		return nil, err
	}
	inv := drivers.FromWMI(rows)
	a.drv.mu.Lock()
	a.drv.inv, a.drv.invAt = inv, time.Now()
	a.drv.mu.Unlock()
	return inv, nil
}

// Drivers builds the page.
func (a *App) Drivers() (DriversView, error) {
	f := a.Facts()
	if f == nil {
		return DriversView{}, ErrNoScan
	}
	st := a.driverStateCopy()
	v := DriversView{Elevated: a.P.Native.IsElevated(), SearchedAt: st.SearchedAt, SearchError: st.SearchError, GPU: st.GPU, Reboot: st.Reboot,
		Sources: []tweaks.Source{srcWUASpecific, srcWUASearch, srcDriverRank, srcDriverPick, srcAuthentic, srcNvidiaDrv, srcKudu}}
	inv, err := a.inventory(false)
	if err != nil {
		v.InventoryError = "não foi possível ler os drivers instalados: " + err.Error()
	}
	v.Components = drivers.Group(inv)
	for _, o := range drivers.Evaluate(st.Updates, inv) {
		if s, ok := st.Status[o.UpdateID]; ok {
			o.Status, o.StatusDetail = s.Status, s.Detail
			if s.Status == "baixado" {
				o.Downloaded = true
			}
			if s.Status == "instalado" {
				// The reasons judged the old driver: say what happened instead.
				o.Select, o.Verdict = false, drivers.VerdictInstalled
				o.Reasons = []string{"Instalado pelo Axion em " + s.At.Format("02/01/2006 15:04") + "."}
				if st.Reboot {
					o.Reasons = append(o.Reasons, "Reinicie o PC para concluir a troca do driver.")
				}
			}
		}
		if o.Hidden {
			v.Hidden = append(v.Hidden, o)
		} else {
			v.Offers = append(v.Offers, o)
		}
	}
	sort.SliceStable(v.Offers, func(i, j int) bool {
		return v.Offers[i].Verdict != drivers.VerdictInstalled && v.Offers[j].Verdict == drivers.VerdictInstalled
	})
	if v.GPU == nil {
		v.GPU = a.gpuBaseline(f)
	}
	v.Board = boardInfo(f)
	v.Notes = []string{
		"Nada é baixado nem instalado sem você escolher. Os drivers do Windows Update são os mesmos que aparecem em Configurações > Windows Update > Atualizações opcionais, assinados e distribuídos pela Microsoft para o seu hardware.",
		"Antes de instalar, o Axion cria um ponto de restauração. Se um driver novo der problema, use Gerenciador de Dispositivos > o dispositivo > Propriedades > Driver > Reverter driver.",
		"Conexões com a internet só acontecem quando você clica: a busca usa o próprio Windows Update; a checagem da placa NVIDIA consulta o serviço público da NVIDIA enviando só o modelo da placa e a versão do Windows.",
	}
	return v, nil
}

// gpuBaseline describes the main GPU before any vendor check.
func (a *App) gpuBaseline(f *facts.Facts) *GPUCheck {
	g := f.PrimaryGPU()
	if g == nil {
		return nil
	}
	c := &GPUCheck{Vendor: g.Vendor, Name: g.Name, InstalledVersion: g.DriverVersion, InstalledDate: g.DriverDate, Links: gpuVendorLinks[g.Vendor]}
	if g.VendorVersion != "" {
		c.InstalledVersion = g.VendorVersion
	}
	return c
}

func boardInfo(f *facts.Facts) BoardInfo {
	b := BoardInfo{Maker: f.Board.Manufacturer, Model: f.Board.Product, BIOSVersion: f.Board.BIOSVersion, BIOSDate: f.Board.BIOSDate}
	for maker, url := range supportPages {
		if strings.Contains(strings.ToLower(f.Board.Manufacturer), strings.ToLower(maker)) {
			b.Support = &Link{Label: "Suporte " + maker + " (BIOS e drivers da placa-mãe)", URL: url}
		}
	}
	switch f.CPU.Vendor {
	case "AMD":
		b.Chipset = &Link{Label: "Drivers de chipset AMD (Ryzen)", URL: "https://www.amd.com/en/support/download/drivers.html"}
	case "Intel":
		b.Chipset = &Link{Label: "Intel Driver & Support Assistant (chipset)", URL: "https://www.intel.com/content/www/us/en/support/intel-driver-support-assistant.html"}
	}
	return b
}

// StartDriverScan looks for driver updates: installed inventory, Windows
// Update search and the NVIDIA check.
func (a *App) StartDriverScan() (Job, error) {
	f := a.Facts()
	if f == nil {
		return Job{}, ErrNoScan
	}
	return a.Jobs.Start("drivers-scan", "Procurando drivers", true, func(ctx context.Context, c *JobCtl) (any, error) {
		c.Progress(0.05, "Lendo os drivers instalados")
		if _, err := a.inventory(true); err != nil {
			c.Logf("aviso: inventário de drivers: %v", err)
		}
		c.Progress(0.15, "Consultando o Windows Update (pode levar alguns minutos)")
		c.NoCancel(wuNoCancel)
		ups, err := a.P.Updates.SearchDrivers(ctx)
		c.NoCancel("")
		searchErr := ""
		if err != nil {
			searchErr = err.Error()
			c.Logf("Windows Update: %v", err)
		} else {
			c.Logf("Windows Update ofereceu %s", driversN(len(ups)))
		}
		a.updateDriverState(func(st *driverState) {
			st.SearchedAt, st.SearchError = time.Now(), searchErr
			if err == nil {
				st.Updates = ups
				keep := map[string]OfferStatus{}
				for _, u := range ups {
					if s, ok := st.Status[u.UpdateID]; ok && s.Status != "instalado" {
						keep[u.UpdateID] = s
					}
				}
				st.Status = keep
			}
		})
		c.Progress(0.8, "Checando o driver da placa de vídeo")
		g := a.checkGPU(ctx, f, c)
		a.updateDriverState(func(st *driverState) {
			// Keep a verified download of the same version.
			if st.GPU != nil && st.GPU.File != "" && g.Latest != nil && st.GPU.FileVersion == g.Latest.Version {
				g.File, g.FileVersion, g.Signer = st.GPU.File, st.GPU.FileVersion, st.GPU.Signer
			}
			st.GPU = g
		})
		if err != nil && g.Error != "" {
			return nil, fmt.Errorf("Windows Update: %v; NVIDIA: %s", err, g.Error)
		}
		v, _ := a.Drivers()
		return map[string]any{"offers": len(v.Offers), "hidden": len(v.Hidden), "gpuUpToDate": g.UpToDate, "searchError": searchErr}, nil
	})
}

// checkGPU asks the vendor for the newest driver (NVIDIA only; AMD and
// Intel publish no lookup service, their pages are linked).
func (a *App) checkGPU(ctx context.Context, f *facts.Facts, c *JobCtl) *GPUCheck {
	g := a.gpuBaseline(f)
	if g == nil {
		return &GPUCheck{Error: "nenhuma placa de vídeo encontrada"}
	}
	g.CheckedAt = time.Now()
	if g.Vendor != "NVIDIA" {
		return g
	}
	g.Links = []Link{{Label: "Página de drivers da NVIDIA", URL: "https://www.nvidia.com/en-us/drivers/"}}
	latest, err := a.nvidiaLatest(ctx, g.Name, f.OS.IsWin11)
	if err != nil {
		g.Error = err.Error()
		c.Logf("NVIDIA: %v", err)
		return g
	}
	g.Latest = latest
	upToDate := drivers.CompareVersions(g.InstalledVersion, latest.Version) >= 0
	g.UpToDate = &upToDate
	if upToDate {
		c.Logf("NVIDIA: driver instalado %s é o mais recente (%s)", g.InstalledVersion, latest.Version)
	} else {
		c.Logf("NVIDIA: disponível %s (instalado %s)", latest.Version, g.InstalledVersion)
	}
	return g
}

func (a *App) httpGet(ctx context.Context, url string, limit int64) ([]byte, error) {
	n := a.nvidia()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AxionOptimizer/"+a.Version)
	resp, err := n.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sem resposta: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("resposta HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("resposta grande demais")
	}
	return b, nil
}

// nvidiaProducts returns the product list, cached for 30 days.
func (a *App) nvidiaProducts(ctx context.Context) (map[string]int, error) {
	cache := filepath.Join(a.P.DataDir, "nvidia-produtos.xml")
	if st, err := os.Stat(cache); err == nil && time.Since(st.ModTime()) < 30*24*time.Hour {
		if b, err := os.ReadFile(cache); err == nil {
			if m, err := drivers.ParseProducts(b); err == nil {
				return m, nil
			}
		}
	}
	b, err := a.httpGet(ctx, a.nvidia().ProductsURL, 8<<20)
	if err != nil {
		return nil, fmt.Errorf("lista de produtos da NVIDIA: %w", err)
	}
	m, err := drivers.ParseProducts(b)
	if err != nil {
		return nil, err
	}
	_ = os.WriteFile(cache, b, 0o644)
	return m, nil
}

func (a *App) nvidiaLatest(ctx context.Context, gpuName string, win11 bool) (*drivers.NvidiaDriver, error) {
	products, err := a.nvidiaProducts(ctx)
	if err != nil {
		return nil, err
	}
	pfid, ok := drivers.ProductID(products, gpuName)
	if !ok {
		return nil, fmt.Errorf("o modelo \"%s\" não está na lista de produtos da NVIDIA", gpuName)
	}
	b, err := a.httpGet(ctx, drivers.LookupQuery(a.nvidia().LookupURL, pfid, drivers.NvidiaOSID(win11)), 4<<20)
	if err != nil {
		return nil, fmt.Errorf("busca de drivers da NVIDIA: %w", err)
	}
	list, err := drivers.ParseLookup(b)
	if err != nil {
		return nil, err
	}
	d := drivers.PickGameReady(list)
	if d == nil {
		return nil, errors.New("a NVIDIA não retornou um driver Game Ready para esta placa")
	}
	return d, nil
}

// DriverIDsRequest selects Windows Update offers.
type DriverIDsRequest struct {
	IDs []string `json:"ids"`
}

func (a *App) offerIDs(ids []string, need func(o drivers.Offer) error) ([]string, error) {
	v, err := a.Drivers()
	if err != nil {
		return nil, err
	}
	by := map[string]drivers.Offer{}
	for _, o := range append(append([]drivers.Offer(nil), v.Offers...), v.Hidden...) {
		by[o.UpdateID] = o
	}
	var out []string
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		o, ok := by[id]
		if !ok || seen[id] {
			continue
		}
		if need != nil {
			if err := need(o); err != nil {
				return nil, fmt.Errorf("%w: %v", errBadRequest, err)
			}
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: nenhuma atualização válida escolhida; procure de novo", errBadRequest)
	}
	return out, nil
}

func (a *App) requireAdmin(what string) error {
	if !a.P.Native.IsElevated() {
		return fmt.Errorf("%w: %s exige executar o Axion como administrador", errBadRequest, what)
	}
	return nil
}

func (a *App) setOfferStatus(res []sys.UpdateResult, okStatus string) (ok, failed int) {
	a.updateDriverState(func(st *driverState) {
		for _, r := range res {
			s := OfferStatus{Status: okStatus, At: time.Now()}
			if r.OK() {
				ok++
				if r.RebootRequired {
					s.Detail = "reinicie para concluir"
				}
			} else {
				failed++
				s.Status = "falhou"
				s.Detail = sys.WUError(uint32(r.HResult))
				if r.HResult == 0 {
					s.Detail = fmt.Sprintf("código %d do Windows Update", r.Code)
				}
			}
			st.Status[r.UpdateID] = s
		}
	})
	return ok, failed
}

// StartDriverDownload downloads the chosen offers through Windows Update.
func (a *App) StartDriverDownload(req DriverIDsRequest) (Job, error) {
	if err := a.requireAdmin("baixar drivers"); err != nil {
		return Job{}, err
	}
	ids, err := a.offerIDs(req.IDs, nil)
	if err != nil {
		return Job{}, err
	}
	return a.Jobs.Start("drivers-download", "Baixando drivers", true, func(ctx context.Context, c *JobCtl) (any, error) {
		c.Progress(0.1, "Baixando "+driversN(len(ids))+" pelo Windows Update")
		c.NoCancel(wuNoCancel)
		res, err := a.P.Updates.DownloadDrivers(ctx, ids)
		if err != nil {
			return nil, err
		}
		ok, failed := a.setOfferStatus(res, "baixado")
		for _, r := range res {
			if r.OK() {
				c.Logf("baixado: %s", r.Title)
			} else {
				c.Logf("falhou: %s — %s", r.Title, sys.WUError(uint32(r.HResult)))
			}
		}
		return map[string]int{"ok": ok, "failed": failed}, nil
	})
}

// StartDriverInstall creates a restore point and installs downloaded offers.
func (a *App) StartDriverInstall(req DriverIDsRequest) (Job, error) {
	if err := a.requireAdmin("instalar drivers"); err != nil {
		return Job{}, err
	}
	ids, err := a.offerIDs(req.IDs, func(o drivers.Offer) error {
		if !o.Downloaded {
			return fmt.Errorf("\"%s\" ainda não foi baixada", o.Title)
		}
		return nil
	})
	if err != nil {
		return Job{}, err
	}
	return a.Jobs.Start("drivers-install", "Instalando drivers", true, func(ctx context.Context, c *JobCtl) (any, error) {
		c.Progress(0.05, "Criando ponto de restauração do Windows")
		rp := "created"
		if err := a.P.Native.CreateRestorePoint("Axion Optimizer — antes de instalar drivers"); err != nil {
			rp = "failed: " + err.Error()
			c.Logf("aviso: ponto de restauração não criado: %v (ainda é possível reverter cada driver pelo Gerenciador de Dispositivos)", err)
		}
		c.Progress(0.25, "Instalando "+driversN(len(ids))+" pelo Windows Update")
		c.NoCancel(wuNoCancel)
		res, reboot, err := a.P.Updates.InstallDrivers(ctx, ids)
		if err != nil {
			return nil, err
		}
		ok, failed := a.setOfferStatus(res, "instalado")
		for _, r := range res {
			if r.OK() {
				c.Logf("instalado: %s", r.Title)
			} else {
				c.Logf("falhou: %s — %s", r.Title, sys.WUError(uint32(r.HResult)))
			}
		}
		if reboot {
			a.updateDriverState(func(st *driverState) { st.Reboot = true })
			c.Logf("reinicie o PC para concluir a instalação")
		}
		a.drv.mu.Lock()
		a.drv.inv = nil // versions changed
		a.drv.mu.Unlock()
		if ok > 0 {
			// Driver versions feed the diagnostic: read the machine again.
			c.NoCancel("")
			c.Progress(0.95, "Atualizando o diagnóstico")
			a.refreshAfterChange(ctx)
		}
		return map[string]any{"ok": ok, "failed": failed, "reboot": reboot, "restorePoint": rp}, nil
	})
}

// HideDriverRequest hides or shows an offer.
type HideDriverRequest struct {
	ID     string `json:"id"`
	Hidden bool   `json:"hidden"`
}

// HideDriver hides (or shows) an offer in Windows Update.
func (a *App) HideDriver(ctx context.Context, req HideDriverRequest) error {
	if err := a.requireAdmin("ocultar atualizações"); err != nil {
		return err
	}
	ids, err := a.offerIDs([]string{req.ID}, nil)
	if err != nil {
		return err
	}
	if err := a.P.Updates.SetHidden(ctx, ids[0], req.Hidden); err != nil {
		return err
	}
	a.updateDriverState(func(st *driverState) {
		for i := range st.Updates {
			if st.Updates[i].UpdateID == ids[0] {
				st.Updates[i].Hidden = req.Hidden
			}
		}
	})
	return nil
}

func (a *App) gpuDownloadDir() string { return filepath.Join(a.P.DataDir, "downloads", "nvidia") }

// progressWriter reports download progress.
type progressWriter struct {
	w     io.Writer
	done  int64
	total int64
	last  time.Time
	fn    func(done, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.done += int64(n)
	if time.Since(p.last) > 250*time.Millisecond {
		p.last = time.Now()
		p.fn(p.done, p.total)
	}
	return n, err
}

// verifyNvidia checks that an installer is signed by NVIDIA.
// errRevocationUnknown: the file is signed by NVIDIA but the revocation of
// the certificate could not be checked now; the file is kept and nothing runs
// until a later check succeeds.
var errRevocationUnknown = errors.New("não foi possível checar agora se o certificado da NVIDIA foi revogado (servidor de certificados inacessível); o arquivo foi mantido, tente de novo mais tarde")

func (a *App) verifyNvidia(path string) (sys.Signature, error) {
	sig, err := a.P.Native.VerifySignature(path)
	if err != nil {
		return sig, fmt.Errorf("verificar assinatura: %w", err)
	}
	if !sig.Trusted && sig.RevocationOffline && strings.EqualFold(strings.TrimSpace(sig.Signer), drivers.NvidiaSigner) {
		return sig, errRevocationUnknown
	}
	if !sig.Trusted {
		return sig, fmt.Errorf("assinatura digital inválida: %s", sig.Error)
	}
	if !strings.EqualFold(strings.TrimSpace(sig.Signer), drivers.NvidiaSigner) {
		return sig, fmt.Errorf("assinado por \"%s\", não pela NVIDIA", sig.Signer)
	}
	return sig, nil
}

// StartGPUDownload downloads the newest NVIDIA driver found by the last
// check and verifies its Authenticode signature.
func (a *App) StartGPUDownload() (Job, error) {
	st := a.driverStateCopy()
	if st.GPU == nil || st.GPU.Latest == nil {
		return Job{}, fmt.Errorf("%w: procure atualizações primeiro", errBadRequest)
	}
	latest := *st.GPU.Latest
	name, err := drivers.DownloadTarget(latest.DownloadURL)
	if err != nil {
		return Job{}, fmt.Errorf("%w: %v", errBadRequest, err)
	}
	return a.Jobs.Start("gpu-download", "Baixando driver NVIDIA "+latest.Version, true, func(ctx context.Context, c *JobCtl) (any, error) {
		dir := a.gpuDownloadDir()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		// Only one installer is kept (they are ~1 GB each).
		if old, _ := os.ReadDir(dir); old != nil {
			for _, e := range old {
				if e.Name() != name {
					_ = os.RemoveAll(filepath.Join(dir, e.Name()))
				}
			}
		}
		dst := filepath.Join(dir, name)
		if _, err := os.Stat(dst); err == nil {
			sig, err := a.verifyNvidia(dst)
			switch {
			case err == nil:
				c.Logf("instalador já baixado e verificado (%s)", sig.Signer)
				a.updateDriverState(func(s *driverState) { setGPUFile(s, dst, latest.Version, sig.Signer) })
				return map[string]string{"file": dst, "signer": sig.Signer}, nil
			case errors.Is(err, errRevocationUnknown):
				return nil, err
			}
			_ = os.Remove(dst)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, latest.DownloadURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "AxionOptimizer/"+a.Version)
		client := *a.nvidia().Client
		client.Timeout = 0 // large file: rely on the job context
		client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
			if len(via) > 5 || r.URL.Scheme != drivers.AllowedScheme || !drivers.AllowedDownloadHost(r.URL.Hostname()) {
				return errors.New("redirecionamento para fora dos servidores da NVIDIA")
			}
			return nil
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("download: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("download: resposta HTTP %d", resp.StatusCode)
		}
		const maxSize = 3 << 30
		if resp.ContentLength > maxSize {
			return nil, errors.New("arquivo maior que o esperado para um driver")
		}
		part := dst + ".part"
		out, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		pw := &progressWriter{w: out, total: resp.ContentLength, fn: func(done, total int64) {
			if total > 0 {
				c.Progress(0.02+0.9*float64(done)/float64(total), fmt.Sprintf("Baixando %s de %s", mib(done), mib(total)))
			} else {
				c.Progress(0.5, "Baixando "+mib(done))
			}
		}}
		_, err = io.Copy(pw, io.LimitReader(resp.Body, maxSize+1))
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err == nil && pw.done > maxSize {
			err = errors.New("arquivo maior que o esperado para um driver")
		}
		if err == nil && resp.ContentLength > 0 && pw.done != resp.ContentLength {
			err = fmt.Errorf("download incompleto (%s de %s)", mib(pw.done), mib(resp.ContentLength))
		}
		if err != nil {
			_ = os.Remove(part)
			return nil, err
		}
		if err := os.Rename(part, dst); err != nil {
			_ = os.Remove(part)
			return nil, err
		}
		c.Progress(0.95, "Verificando a assinatura digital")
		sig, err := a.verifyNvidia(dst)
		if errors.Is(err, errRevocationUnknown) {
			return nil, err // kept for a later check, never marked as verified
		}
		if err != nil {
			_ = os.Remove(dst)
			return nil, fmt.Errorf("o arquivo baixado foi descartado: %w", err)
		}
		c.Logf("assinatura válida: %s", sig.Signer)
		a.updateDriverState(func(s *driverState) { setGPUFile(s, dst, latest.Version, sig.Signer) })
		return map[string]string{"file": dst, "signer": sig.Signer}, nil
	})
}

func setGPUFile(s *driverState, file, version, signer string) {
	if s.GPU == nil {
		s.GPU = &GPUCheck{}
	}
	s.GPU.File, s.GPU.FileVersion, s.GPU.Signer = file, version, signer
}

func mib(n int64) string {
	return strings.Replace(fmt.Sprintf("%.0f MB", float64(n)/(1<<20)), ".", ",", 1)
}

// InstallGPUDriver re-checks the signature of the downloaded installer and
// opens it. The vendor's installer takes over from there.
func (a *App) InstallGPUDriver() error {
	if err := a.requireAdmin("instalar o driver de vídeo"); err != nil {
		return err
	}
	st := a.driverStateCopy()
	if st.GPU == nil || st.GPU.File == "" {
		return fmt.Errorf("%w: baixe o driver primeiro", errBadRequest)
	}
	file := st.GPU.File
	if filepath.Dir(file) != a.gpuDownloadDir() {
		return fmt.Errorf("%w: instalador fora da pasta do Axion", errBadRequest)
	}
	if _, err := a.verifyNvidia(file); err != nil {
		if errors.Is(err, errRevocationUnknown) {
			return err
		}
		_ = os.Remove(file)
		a.updateDriverState(func(s *driverState) { setGPUFile(s, "", "", "") })
		return fmt.Errorf("instalador descartado: %w", err)
	}
	return a.P.Native.StartProgram(file, "")
}

// DeleteGPUDownload removes the downloaded installer.
func (a *App) DeleteGPUDownload() error {
	st := a.driverStateCopy()
	if st.GPU != nil && st.GPU.File != "" && filepath.Dir(st.GPU.File) == a.gpuDownloadDir() {
		if err := os.Remove(st.GPU.File); err != nil && !os.IsNotExist(err) {
			// Keep the record: the file is still on disk (the installer may
			// be open, or an antivirus is scanning it).
			return fmt.Errorf("não foi possível apagar o instalador (%v); feche o instalador da NVIDIA e tente de novo", err)
		}
	}
	a.updateDriverState(func(s *driverState) { setGPUFile(s, "", "", "") })
	return nil
}

func driversN(n int) string {
	if n == 1 {
		return "1 driver"
	}
	return strconv.Itoa(n) + " drivers"
}

const wuNoCancel = "O Windows Update não permite interromper esta etapa no meio; ela termina sozinha."

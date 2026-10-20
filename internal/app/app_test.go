package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"axion/internal/drivers"
	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

type client struct {
	t    *testing.T
	s    *Server
	host string
}

func (c *client) do(method, path string, body any, hdr map[string]string) (int, []byte) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, "http://"+c.s.Host+path, rd)
	req.Header.Set("X-Axion-Token", c.s.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	if c.host != "" {
		req.Host = c.host
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

func (c *client) json(method, path string, body any, out any) {
	c.t.Helper()
	code, b := c.do(method, path, body, nil)
	if code != 200 {
		c.t.Fatalf("%s %s: %d %s", method, path, code, b)
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			c.t.Fatalf("%s %s: %v", method, path, err)
		}
	}
}

func (c *client) wait(jobID string) Job {
	c.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		var j Job
		c.json("GET", "/api/jobs/"+jobID, nil, &j)
		if j.Status != JobRunning {
			return j
		}
		time.Sleep(20 * time.Millisecond)
	}
	c.t.Fatal("tarefa não terminou")
	return Job{}
}

func setupServer(t *testing.T) (*mocksys.Mock, *client) {
	t.Helper()
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(m.Platform, "test")
	if err != nil {
		t.Fatal(err)
	}
	ui := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>Axion</title>")}}
	s, err := NewServer(a, ui)
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve()
	t.Cleanup(func() { s.Close() })
	return m, &client{t: t, s: s}
}

func TestSecurityGuards(t *testing.T) {
	_, c := setupServer(t)
	if code, _ := c.do("GET", "/api/state", nil, map[string]string{"X-Axion-Token": ""}); code != 401 {
		t.Errorf("sem token: %d", code)
	}
	if code, _ := c.do("GET", "/api/state", nil, map[string]string{"X-Axion-Token": "errado"}); code != 401 {
		t.Errorf("token errado: %d", code)
	}
	if code, _ := c.do("GET", "/api/state", nil, map[string]string{"Origin": "https://evil.example"}); code != 403 {
		t.Errorf("origem estranha: %d", code)
	}
	if code, _ := c.do("GET", "/api/state", nil, map[string]string{"Sec-Fetch-Site": "cross-site"}); code != 403 {
		t.Errorf("cross-site: %d", code)
	}
	if code, _ := c.do("POST", "/api/scan", nil, map[string]string{"Content-Type": "text/plain"}); code != 415 {
		t.Errorf("POST sem JSON: %d", code)
	}
	c.host = "attacker.example:80"
	if code, _ := c.do("GET", "/api/state", nil, nil); code != 421 {
		t.Errorf("DNS rebinding (Host errado): %d", code)
	}
	c.host = ""
	if code, b := c.do("GET", "/", nil, map[string]string{"X-Axion-Token": ""}); code != 200 || strings.Contains(string(b), c.s.Token) {
		t.Errorf("index sem token deveria abrir e não conter o token: %d", code)
	}
	var st StateResp
	c.json("GET", "/api/state", nil, &st)
	if !st.Mock || st.Scanned {
		t.Errorf("estado inicial: %+v", st)
	}
	if code, _ := c.do("POST", "/api/open", map[string]string{"url": "file:///C:/Windows/System32/cmd.exe"}, nil); code != 400 {
		t.Errorf("URL perigosa deveria ser recusada: %d", code)
	}
}

func TestEndToEnd(t *testing.T) {
	m, c := setupServer(t)
	before := m.World()

	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise falhou: %+v", j)
	}
	// A second exclusive job while one runs is refused; here nothing runs.
	var rep ReportResp
	c.json("GET", "/api/report", nil, &rep)
	if len(rep.Findings) < 10 || rep.Overall <= 0 {
		t.Fatalf("relatório vazio: %+v", rep.Report)
	}
	var plan Plan
	c.json("GET", "/api/plan?tier=recomendado", nil, &plan)
	if len(plan.Items) < 5 {
		t.Fatalf("plano recomendado pequeno demais: %d", len(plan.Items))
	}
	ids := []string{}
	for _, it := range plan.Items {
		if it.Optional {
			t.Errorf("recomendado com item opcional: %s", it.ID)
		}
		ids = append(ids, it.ID)
	}
	var full Plan
	c.json("GET", "/api/plan?tier=completo", nil, &full)
	if len(full.Items) <= len(plan.Items) {
		t.Errorf("completo deveria incluir opcionais: %d <= %d", len(full.Items), len(plan.Items))
	}
	var cs2 Plan
	c.json("GET", "/api/plan?tier=cs2", nil, &cs2)
	hasProfile := false
	for _, it := range cs2.Items {
		if strings.HasPrefix(it.ID, "profile:cs2.exe:") {
			hasProfile = true
		}
	}
	if !hasProfile {
		t.Errorf("plano CS2 sem perfil do cs2.exe: %+v", cs2.Items)
	}

	c.json("POST", "/api/apply", ApplyRequest{IDs: ids, RestorePoint: true}, &j)
	j = c.wait(j.ID)
	if j.Status != JobDone {
		t.Fatalf("aplicação falhou: %+v", j)
	}
	b, _ := json.Marshal(j.Result)
	var br BatchResult
	_ = json.Unmarshal(b, &br)
	if br.Counts["applied"] == 0 || br.Counts["failed"] > 0 || br.RestorePoint != "created" {
		t.Fatalf("resultado: %+v", br.Counts)
	}
	var after Plan
	c.json("GET", "/api/plan?tier=recomendado", nil, &after)
	if len(after.Items) != 0 {
		var left []string
		for _, it := range after.Items {
			left = append(left, it.ID)
		}
		t.Errorf("após aplicar, ainda restam recomendados: %v", left)
	}
	var st StateResp
	c.json("GET", "/api/state", nil, &st)
	if st.Applied == 0 || !st.RebootPending {
		t.Errorf("estado após aplicar: applied=%d reboot=%v", st.Applied, st.RebootPending)
	}
	var hist struct {
		Entries []HistoryEntry `json:"entries"`
	}
	c.json("GET", "/api/history", nil, &hist)
	if len(hist.Entries) == 0 {
		t.Fatal("histórico vazio")
	}

	c.json("POST", "/api/revert", RevertRequest{All: true}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("reversão falhou: %+v", j)
	}
	if d := mocksys.DiffWorld(before, m.World(), "hklm\\software\\axion", "power|0000", "hklm\\software\\microsoft\\windows nt\\currentversion\\systemrestore"); len(d) > 0 {
		t.Errorf("reverter tudo não restaurou o PC:\n  %s", strings.Join(d, "\n  "))
	}
	for _, path := range []string{"/api/tweaks", "/api/games", "/api/cs2", "/api/bios", "/api/excluded", "/api/facts", "/api/export"} {
		if code, b := c.do("GET", path, nil, nil); code != 200 || len(b) < 10 {
			t.Errorf("%s: %d", path, code)
		}
	}
}

func TestPeripheralsPage(t *testing.T) {
	m, c := setupServer(t)

	// Before a scan the page still lists devices from a quick inventory.
	var pv PeripheralsView
	c.json("GET", "/api/peripherals", nil, &pv)
	if pv.Scanned || len(pv.Devices) != 2 || !pv.TraceAvailable {
		t.Fatalf("página antes da análise: scanned=%v devices=%d trace=%v", pv.Scanned, len(pv.Devices), pv.TraceAvailable)
	}

	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise falhou: %+v", j)
	}
	c.json("GET", "/api/peripherals", nil, &pv)
	if !pv.Scanned || len(pv.Devices) != 2 {
		t.Fatalf("página após análise: %+v", pv)
	}
	kbd := pv.Devices[1]
	if kbd.Role != "Teclado" || !strings.Contains(kbd.Rate, "125 Hz") || !strings.Contains(kbd.Port, "1 hub externo") || len(kbd.Findings) != 3 {
		t.Errorf("teclado: role=%q rate=%q port=%q findings=%d", kbd.Role, kbd.Rate, kbd.Port, len(kbd.Findings))
	}
	if pv.Devices[0].Role != "Receptor sem fio (mouse e teclado)" || !strings.Contains(pv.Devices[0].Rate, "1000 Hz") {
		t.Errorf("receptor: %q %q", pv.Devices[0].Role, pv.Devices[0].Rate)
	}
	if pv.Settings.MouseQueue != 20 || len(pv.Findings) == 0 {
		t.Errorf("configurações: %+v findings=%d", pv.Settings, len(pv.Findings))
	}

	// Polling test through the job API.
	c.json("POST", "/api/peripherals/measure", MeasureRequest{Seconds: 3, USBTrace: true}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("medição falhou: %+v", j)
	}
	if m.N.Captures != 1 {
		t.Errorf("captura não executada: %d", m.N.Captures)
	}
	c.json("GET", "/api/peripherals", nil, &pv)
	if pv.Measurement == nil || len(pv.Measurement.Devices) != 1 || !pv.Measurement.USBTraced {
		t.Fatalf("medição não aparece na página: %+v", pv.Measurement)
	}
	got := pv.Devices[0].Measured
	if got == nil || got.Snapped != 1000 || got.Verdict != "ok" {
		t.Fatalf("resultado do receptor: %+v", got)
	}
	// Persisted for the next start and included in the exported report.
	a2, err := New(m.Platform, "test")
	if err != nil {
		t.Fatal(err)
	}
	if h := a2.Measurements(); len(h) != 1 || h[0].Devices[0].Snapped != 1000 {
		t.Errorf("medições não foram salvas: %+v", h)
	}
	if _, ok := a2.exportRaw()["peripherals"]; !ok {
		t.Errorf("relatório exportado sem medições")
	}
	// Bounds: a request for 60 s is clamped.
	c.json("POST", "/api/peripherals/measure", MeasureRequest{Seconds: 60}, &j)
	j = c.wait(j.ID)
	b, _ := json.Marshal(j.Result)
	var res struct {
		Seconds float64 `json:"seconds"`
	}
	_ = json.Unmarshal(b, &res)
	if res.Seconds != 15 {
		t.Errorf("duração deveria ser limitada a 15 s, obtido %v", res.Seconds)
	}
}

// fakeNvidia serves NVIDIA's product list, the driver lookup and the
// installer download.
func fakeNvidia(t *testing.T, installer []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/products", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<LookupValueSearch><LookupValues><LookupValue ParentID="120"><Name>GeForce RTX 3060</Name><Value>942</Value></LookupValue></LookupValues></LookupValueSearch>`)
	})
	var srv *httptest.Server
	mux.HandleFunc("/lookup", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pfid") != "942" || r.URL.Query().Get("osID") != "135" {
			http.Error(w, "bad query", 400)
			return
		}
		fmt.Fprintf(w, `{"Success":"1","IDS":[{"downloadInfo":{"ID":"279803","Name":"GeForce%%20Game%%20Ready%%20Driver","Version":"617.14","IsBeta":"0","IsWHQL":"1","IsCRD":"0","ReleaseDateTime":"Tue Sep 22, 2026","DownloadURLFileSize":"990.85 MB","DetailsURL":"https://www.nvidia.com/en-us/drivers/details/279803/","DownloadURL":"%s/Windows/617.14/617.14-desktop-win10-win11-64bit-international-dch-whql.exe"}}]}`, srv.URL)
	})
	mux.HandleFunc("/Windows/617.14/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(installer)))
		w.Write(installer)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestDriversPage(t *testing.T) {
	m, c := setupServer(t)
	srv := fakeNvidia(t, []byte("MZ-SIGNED:NVIDIA Corporation\nbinário do instalador"))
	c.s.App.NvidiaAPI = NvidiaEndpoints{ProductsURL: srv.URL + "/products", LookupURL: srv.URL + "/lookup", Client: srv.Client()}
	oldHost, oldScheme := drivers.AllowedDownloadHost, drivers.AllowedScheme
	drivers.AllowedDownloadHost, drivers.AllowedScheme = func(h string) bool { return h == "127.0.0.1" }, "http"
	t.Cleanup(func() { drivers.AllowedDownloadHost, drivers.AllowedScheme = oldHost, oldScheme })

	if code, _ := c.do("GET", "/api/drivers", nil, nil); code != http.StatusConflict {
		t.Fatalf("antes da análise deveria pedir a análise, veio %d", code)
	}
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	c.wait(j.ID)

	var v DriversView
	c.json("GET", "/api/drivers", nil, &v)
	if len(v.Components) == 0 || v.Components[0].Kind != "gpu" || len(v.Offers) != 0 || v.GPU == nil || v.GPU.InstalledVersion != "566.36" {
		t.Fatalf("página inicial: %+v", v)
	}
	c.json("POST", "/api/drivers/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("busca falhou: %+v", j)
	}
	c.json("GET", "/api/drivers", nil, &v)
	if len(v.Offers) != 3 || len(v.Hidden) != 1 || v.SearchedAt.IsZero() {
		t.Fatalf("ofertas: %d ocultas: %d", len(v.Offers), len(v.Hidden))
	}
	var recommended []string
	for _, o := range v.Offers {
		if o.Select {
			recommended = append(recommended, o.UpdateID)
		}
		if o.Class == "Display" && (o.Verdict != "nao" || o.Select) {
			t.Errorf("driver de vídeo antigo do Windows Update deveria ser desaconselhado: %+v", o)
		}
	}
	if len(recommended) != 2 {
		t.Fatalf("recomendadas: %v", recommended)
	}
	g := v.GPU
	if g.Latest == nil || g.Latest.Version != "617.14" || g.UpToDate == nil || *g.UpToDate || g.Error != "" {
		t.Fatalf("checagem NVIDIA: %+v", g)
	}

	// Install before download is refused.
	if code, _ := c.do("POST", "/api/drivers/install", DriverIDsRequest{IDs: recommended}, nil); code != http.StatusBadRequest {
		t.Fatalf("instalar sem baixar deveria falhar, veio %d", code)
	}
	noNet := uint32(0x8024402C) // WU_E_PT_WINHTTP_NAME_NOT_RESOLVED
	m.U.Fail[recommended[1]] = int32(noNet)
	c.json("POST", "/api/drivers/download", DriverIDsRequest{IDs: append(recommended, "desconhecido")}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("download falhou: %+v", j)
	}
	c.json("GET", "/api/drivers", nil, &v)
	st := map[string]string{}
	for _, o := range v.Offers {
		st[o.UpdateID] = o.Status + "|" + o.StatusDetail
	}
	if st[recommended[0]] != "baixado|" || !strings.HasPrefix(st[recommended[1]], "falhou|sem conexão") {
		t.Fatalf("estados após download: %v", st)
	}
	delete(m.U.Fail, recommended[1])
	c.json("POST", "/api/drivers/install", DriverIDsRequest{IDs: recommended[:1]}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("instalação falhou: %+v", j)
	}
	if len(m.N.RestorePoints) == 0 || !strings.Contains(m.N.RestorePoints[len(m.N.RestorePoints)-1], "drivers") {
		t.Errorf("ponto de restauração não criado antes dos drivers: %v", m.N.RestorePoints)
	}
	if !strings.Contains(strings.Join(m.U.Log, ","), "install "+recommended[0]) {
		t.Errorf("instalação não chegou ao Windows Update: %v", m.U.Log)
	}

	// Hide and show again.
	c.json("POST", "/api/drivers/hide", HideDriverRequest{ID: recommended[1], Hidden: true}, nil)
	c.json("GET", "/api/drivers", nil, &v)
	if len(v.Hidden) != 2 {
		t.Fatalf("ocultar: %d ocultas", len(v.Hidden))
	}

	// NVIDIA installer: download, signature check, launch.
	c.json("POST", "/api/drivers/gpu/download", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("download do driver NVIDIA falhou: %+v", j)
	}
	c.json("GET", "/api/drivers", nil, &v)
	if v.GPU.File == "" || v.GPU.Signer != "NVIDIA Corporation" || v.GPU.FileVersion != "617.14" {
		t.Fatalf("instalador não registrado: %+v", v.GPU)
	}
	c.json("POST", "/api/drivers/gpu/install", map[string]any{}, nil)
	if len(m.N.Started) != 1 || m.N.Started[0] != v.GPU.File {
		t.Fatalf("instalador não foi aberto: %v", m.N.Started)
	}
	// A file swapped after the download is refused and discarded.
	if err := os.WriteFile(v.GPU.File, []byte("MZ-SIGNED:Outra Empresa\nx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _ := c.do("POST", "/api/drivers/gpu/install", map[string]any{}, nil); code == http.StatusOK {
		t.Fatal("instalador adulterado foi aberto")
	}
	if _, err := os.Stat(v.GPU.File); !os.IsNotExist(err) {
		t.Error("instalador adulterado não foi apagado")
	}
	// Exported report carries the state without the local path.
	b, _ := json.Marshal(c.s.App.exportRaw()["drivers"])
	if !strings.Contains(string(b), "617.14") || strings.Contains(string(b), m.DataDir) {
		t.Errorf("exportação de drivers: %s", b)
	}
}

func TestGPUDownloadRejectsWrongSigner(t *testing.T) {
	_, c := setupServer(t)
	srv := fakeNvidia(t, []byte("MZ-SIGNED:Empresa Falsa\nbinário"))
	c.s.App.NvidiaAPI = NvidiaEndpoints{ProductsURL: srv.URL + "/products", LookupURL: srv.URL + "/lookup", Client: srv.Client()}
	oldHost, oldScheme := drivers.AllowedDownloadHost, drivers.AllowedScheme
	drivers.AllowedDownloadHost, drivers.AllowedScheme = func(h string) bool { return h == "127.0.0.1" }, "http"
	t.Cleanup(func() { drivers.AllowedDownloadHost, drivers.AllowedScheme = oldHost, oldScheme })
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	c.wait(j.ID)
	c.json("POST", "/api/drivers/scan", map[string]any{}, &j)
	c.wait(j.ID)
	c.json("POST", "/api/drivers/gpu/download", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobFailed || !strings.Contains(j.Error, "não pela NVIDIA") {
		t.Fatalf("assinatura de outro fabricante aceita: %+v", j)
	}
	files, _ := os.ReadDir(c.s.App.gpuDownloadDir())
	if len(files) != 0 {
		t.Errorf("arquivo recusado ficou no disco: %v", files)
	}
}

// Revocation servers unreachable: the NVIDIA-signed file is kept but never
// marked as verified nor run; a later check (online) accepts it without a
// new download.
func TestGPUDownloadRevocationOffline(t *testing.T) {
	m, c := setupServer(t)
	srv := fakeNvidia(t, []byte("MZ-SIGNED:NVIDIA Corporation\nbinário"))
	c.s.App.NvidiaAPI = NvidiaEndpoints{ProductsURL: srv.URL + "/products", LookupURL: srv.URL + "/lookup", Client: srv.Client()}
	oldHost, oldScheme := drivers.AllowedDownloadHost, drivers.AllowedScheme
	drivers.AllowedDownloadHost, drivers.AllowedScheme = func(h string) bool { return h == "127.0.0.1" }, "http"
	t.Cleanup(func() { drivers.AllowedDownloadHost, drivers.AllowedScheme = oldHost, oldScheme })
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	c.wait(j.ID)
	c.json("POST", "/api/drivers/scan", map[string]any{}, &j)
	c.wait(j.ID)
	m.N.RevocationOffline = true
	c.json("POST", "/api/drivers/gpu/download", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobFailed || !strings.Contains(j.Error, "revogado") {
		t.Fatalf("sem checar a revogação o download não pode ser aceito: %+v", j)
	}
	files, _ := os.ReadDir(c.s.App.gpuDownloadDir())
	if len(files) != 1 {
		t.Fatalf("o arquivo da NVIDIA deveria ficar para uma nova checagem: %v", files)
	}
	if st := c.s.App.driverStateCopy(); st.GPU != nil && st.GPU.File != "" {
		t.Fatalf("arquivo marcado como verificado: %+v", st.GPU)
	}
	if err := c.s.App.InstallGPUDriver(); err == nil || len(m.N.Started) != 0 {
		t.Fatalf("instalador executado sem verificação: %v %v", err, m.N.Started)
	}
	m.N.RevocationOffline = false
	c.json("POST", "/api/drivers/gpu/download", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone || !strings.Contains(strings.Join(j.Log, "\n"), "já baixado e verificado") {
		t.Fatalf("segunda checagem: %+v", j)
	}
}

func TestCleanupPage(t *testing.T) {
	m, c := setupServer(t)
	recycle := int64(m.N.RecycleBytes)
	var v CleanupView
	c.json("GET", "/api/cleanup", nil, &v)
	by := map[string]CleanupItem{}
	for _, it := range v.Items {
		by[it.ID] = it
	}
	if u := by["temp.user"]; u.Size.Files != 3 || u.Size.Kept != 1 || u.Size.Bytes != (480<<20)+(2<<10)+(95<<20) || !u.Default {
		t.Fatalf("temp do usuário: %+v", u.Size)
	}
	if o := by["windows.old"]; o.Kind != "link" || o.Size.Bytes != 18<<30 || o.Link == "" {
		t.Errorf("Windows.old: %+v", o)
	}
	if r := by["recycle"]; r.Size.Bytes != recycle || recycle == 0 || r.Default {
		t.Errorf("lixeira: %+v", r)
	}
	if len(v.NotDone) == 0 || v.Free == 0 {
		t.Errorf("página incompleta: %+v", v)
	}
	var j Job
	c.json("POST", "/api/cleanup/run", CleanupRequest{IDs: []string{"temp.user", "temp.windows", "wer", "recycle", "winsxs", "windows.old"}}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("limpeza falhou: %+v", j)
	}
	b, _ := json.Marshal(j.Result)
	var rep CleanupReport
	_ = json.Unmarshal(b, &rep)
	if len(rep.Results) != 5 { // windows.old is only linked, never deleted by Axion
		t.Fatalf("resultados: %+v", rep.Results)
	}
	want := int64(480<<20) + (2 << 10) + (95 << 20) + (2300 << 20) + (180 << 10) + (24 << 10) + recycle + (1700 << 20)
	if rep.Freed != want {
		t.Errorf("liberado %d, esperado %d", rep.Freed, want)
	}
	p := m.N.Paths()
	for _, kept := range []string{filepath.Join(p.UserTemp, "discord_update.log"), filepath.Join(p.LocalAppData, "NVIDIA", "DXCache", "d3d_0001.nvph"), filepath.Join(p.WindowsDir, "MEMORY.DMP")} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s não deveria ser apagado: %v", kept, err)
		}
	}
	if _, err := os.Stat(filepath.Join(p.UserTemp, "7zS8C1E")); !os.IsNotExist(err) {
		t.Error("pasta temporária antiga vazia deveria sair")
	}
	c.json("GET", "/api/cleanup", nil, &v)
	if v.Last == nil || v.Last.Freed != want {
		t.Errorf("último resultado não aparece: %+v", v.Last)
	}
	if code, _ := c.do("POST", "/api/cleanup/run", CleanupRequest{IDs: []string{"windows.old"}}, nil); code != http.StatusBadRequest {
		t.Errorf("só alvos com link: esperado 400, veio %d", code)
	}
}

func TestStartupPage(t *testing.T) {
	m, c := setupServer(t)
	var v StartupView
	c.json("GET", "/api/startup", nil, &v)
	if v.Enabled != 10 || v.Suggest != 5 || len(v.Items) != 11 { // 8 seeded + Contoso's run value and task
		t.Fatalf("ligados=%d sugeridos=%d itens=%d", v.Enabled, v.Suggest, len(v.Items))
	}
	var ids []string
	for _, it := range v.Items {
		if it.Enabled && it.Verdict == "desativar" {
			ids = append(ids, it.ID)
		}
	}
	c.json("POST", "/api/startup/set", StartupRequest{IDs: ids, Enabled: false}, &v)
	if v.Enabled != 5 || v.Suggest != 0 {
		t.Fatalf("após desativar: ligados=%d sugeridos=%d", v.Enabled, v.Suggest)
	}
	if tk, _ := m.T.Get(`\GoogleUpdateTaskMachineUA`); tk.Enabled {
		t.Error("tarefa de logon continua ligada")
	}
	// Each change went through the engine: one history entry per item.
	act := c.s.App.Engine.J.Active()
	if len(act) != 5 {
		t.Fatalf("entradas no histórico: %d", len(act))
	}
	for _, en := range act {
		if !strings.HasPrefix(en.TweakID, "startup:off:") || en.BatchLabel != "Inicialização" {
			t.Errorf("entrada inesperada: %s (%s)", en.TweakID, en.BatchLabel)
		}
	}
	if code, _ := c.do("POST", "/api/startup/set", StartupRequest{IDs: []string{"run-user|Inexistente"}}, nil); code == http.StatusOK {
		t.Error("item inexistente aceito")
	}
}

func TestProcessesPage(t *testing.T) {
	m, c := setupServer(t)
	var v ProcessesView
	c.json("GET", "/api/processes", nil, &v)
	by := map[string]ProcGroup{}
	for _, g := range v.Groups {
		by[strings.ToLower(g.Name)] = g
	}
	ch := by["chrome.exe"]
	if ch.Count != 3 || !ch.User || ch.Protected || ch.CPU <= 0 || ch.RAM != (610+290+210)<<20 || ch.Label != "Google Chrome" {
		t.Fatalf("chrome: %+v", ch)
	}
	if !by["svchost.exe"].Protected || by["svchost.exe"].User || !by["explorer.exe"].Protected || !by["msmpeng.exe"].Protected {
		t.Errorf("processos do sistema deveriam ser protegidos: %+v %+v", by["svchost.exe"], by["explorer.exe"])
	}
	if v.Groups[0].User != true || v.CPUTotal <= 0 || v.RAMTotal == 0 {
		t.Errorf("ordem/totais: %+v", v.Groups[0])
	}
	var res map[string]int
	c.json("POST", "/api/processes/close", CloseRequest{Name: "chrome.exe", PIDs: append(ch.PIDs, 2211)}, &res)
	if res["closed"] != 3 {
		t.Fatalf("fechados: %v", res)
	}
	for _, p := range m.N.Procs {
		if strings.EqualFold(p.Name, "chrome.exe") {
			t.Error("chrome continua aberto")
		}
		if p.PID == 2211 && !strings.EqualFold(p.Name, "Discord.exe") {
			t.Error("PID de outro programa foi afetado")
		}
	}
	if len(m.N.Procs) != 8 {
		t.Errorf("Discord (PID de outro programa na lista) não deveria fechar: %d processos", len(m.N.Procs))
	}
	for _, name := range []string{"svchost.exe", "explorer.exe", "vgc.exe"} {
		if code, _ := c.do("POST", "/api/processes/close", CloseRequest{Name: name, PIDs: []uint32{880, 1200}}, nil); code == http.StatusOK {
			t.Errorf("%s protegido foi aceito", name)
		}
	}
}

func TestUninstallFlow(t *testing.T) {
	m, c := setupServer(t)
	var v ProgramsView
	c.json("GET", "/api/programs", nil, &v)
	if len(v.Programs) != 3 {
		t.Fatalf("programas: %+v", v.Programs)
	}
	var j Job
	c.json("POST", "/api/programs/uninstall", UninstallRequest{ID: mocksys.ContosoKey}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("desinstalação falhou: %+v", j)
	}
	b, _ := json.Marshal(j.Result)
	var res UninstallResult
	_ = json.Unmarshal(b, &res)
	if !res.Removed || len(res.Leftovers) != 6 {
		t.Fatalf("resultado: %+v", res)
	}
	if len(m.N.Visible) != 1 || !strings.Contains(m.N.Visible[0], "unins000.exe") {
		t.Errorf("desinstalador oficial não foi aberto: %v", m.N.Visible)
	}
	var ids []string
	for _, l := range res.Leftovers {
		ids = append(ids, l.ID)
	}
	// A forged item (not found by Axion for this program) is ignored.
	var batch struct {
		ID    string `json:"id"`
		Items []any  `json:"items"`
	}
	c.json("POST", "/api/programs/quarantine", QuarantineRequest{ID: mocksys.ContosoKey, Items: append(ids, "dir|"+m.N.Paths().WindowsDir)}, &batch)
	if len(batch.Items) != 6 {
		t.Fatalf("quarentena: %+v", batch)
	}
	if _, err := os.Stat(m.N.Paths().WindowsDir); err != nil {
		t.Fatal("item forjado foi movido")
	}
	c.json("GET", "/api/programs", nil, &v)
	if len(v.Programs) != 2 || len(v.Quarantine) != 1 {
		t.Fatalf("após quarentena: %d programas, %d lotes", len(v.Programs), len(v.Quarantine))
	}
	// The same leftovers cannot be quarantined twice.
	if code, _ := c.do("POST", "/api/programs/quarantine", QuarantineRequest{ID: mocksys.ContosoKey, Items: ids}, nil); code != http.StatusBadRequest {
		t.Errorf("segunda quarentena: %d", code)
	}
	c.json("POST", "/api/programs/restore", BatchRequest{Batch: batch.ID}, nil)
	c.json("GET", "/api/programs", nil, &v)
	if len(v.Quarantine) != 0 {
		t.Errorf("lote restaurado continua na quarentena")
	}
	if ok, _ := m.R.KeyExists(`HKCU\Software\Contoso\Contoso Game Booster`); !ok {
		t.Error("configurações não voltaram")
	}

	// Per-user program: the uninstaller runs through a task, never elevated.
	var sp string
	for _, p := range v.Programs {
		if p.Name == "Spotify" {
			sp = p.ID
		}
	}
	c.json("POST", "/api/programs/uninstall", UninstallRequest{ID: sp}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("desinstalação por usuário falhou: %+v", j)
	}
	if len(m.N.Visible) != 1 {
		t.Errorf("desinstalador do usuário não deveria rodar elevado: %v", m.N.Visible)
	}
	if tasks, _ := m.T.List(`\Axion`, true); len(tasks) != 0 {
		t.Errorf("tarefa temporária não foi apagada: %+v", tasks)
	}

	// An uninstaller that never finishes: the wait can be cancelled.
	m.N.RunHook = func(string, string) int { return 0 }
	c.json("POST", "/api/programs/uninstall", UninstallRequest{ID: `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\{23170F69-40C1-2702-2301-000001000000}`}, &j)
	time.Sleep(100 * time.Millisecond)
	c.json("POST", "/api/jobs/"+j.ID+"/cancel", map[string]any{}, nil)
	if j = c.wait(j.ID); j.Status != JobCancelled {
		t.Fatalf("espera não foi cancelada: %+v", j)
	}
	if code, _ := c.do("POST", "/api/programs/leftovers", UninstallRequest{ID: `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\{23170F69-40C1-2702-2301-000001000000}`}, nil); code == http.StatusOK {
		t.Error("sobras de programa ainda instalado não deveriam ser procuradas")
	}
}

// With two desktops (fast user switching) the session is never guessed from
// someone else's explorer.exe; Axion's own process decides.
func TestOwnSession(t *testing.T) {
	two := []sys.ProcessInfo{{PID: 10, Name: "explorer.exe", Session: 1}, {PID: 20, Name: "explorer.exe", Session: 2}}
	if s := ownSession(two); s != 0 {
		t.Errorf("duas sessões: não deveria adivinhar, veio %d", s)
	}
	self := append(two, sys.ProcessInfo{PID: uint32(os.Getpid()), Name: "axion.exe", Session: 2})
	if s := ownSession(self); s != 2 {
		t.Errorf("sessão do próprio Axion: %d", s)
	}
	if s := ownSession(two[:1]); s != 1 {
		t.Errorf("uma área de trabalho: %d", s)
	}
}

func TestSSDMaintenance(t *testing.T) {
	m, c := setupServer(t)
	// Before an analysis the volumes are unknown and retrim is refused.
	var v SSDView
	c.json("GET", "/api/ssd", nil, &v)
	if v.Scanned || v.Trim.NTFS != 0 || !v.TaskOn || v.TaskLast == nil {
		t.Fatalf("antes da análise: %+v", v)
	}
	if code, _ := c.do("POST", "/api/ssd/retrim", RetrimRequest{}, nil); code != http.StatusConflict && code != http.StatusBadRequest {
		t.Errorf("retrim sem análise: esperado erro, veio %d", code)
	}

	// A guide turned TRIM and the optimizer off; the analysis finds both.
	_, _ = m.Platform.Run.Run(context.Background(), "fsutil.exe", []string{"behavior", "set", "disabledeletenotify", "1"}, sys.RunOpts{})
	_ = m.T.SetEnabled(sys.DefragTask, false)
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	c.json("GET", "/api/ssd", nil, &v)
	if !v.Scanned || !v.HasSSD || v.Trim.NTFS != 1 || v.TaskOn || !v.FixTrim || !v.FixTask || len(v.Volumes) != 2 {
		t.Fatalf("depois da análise: %+v", v)
	}
	var rep struct {
		Findings []struct{ ID string } `json:"findings"`
	}
	c.json("GET", "/api/report", nil, &rep)
	seen := map[string]bool{}
	for _, f := range rep.Findings {
		seen[f.ID] = true
	}
	if !seen["ssd.trim.off"] || !seen["ssd.optimize.off"] {
		t.Errorf("achados de SSD ausentes: %v", seen)
	}

	// Fix both, then retrim every SSD volume.
	c.json("POST", "/api/apply", map[string]any{"ids": []string{"fix.trim.on", "fix.defrag.task"}}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("correções: %+v", j)
	}
	c.json("GET", "/api/ssd", nil, &v)
	if v.Trim.NTFS != 0 || !v.TaskOn || v.FixTrim || v.FixTask {
		t.Fatalf("após corrigir: %+v", v)
	}
	before := time.Now()
	c.json("POST", "/api/ssd/retrim", RetrimRequest{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("retrim: %+v", j)
	}
	b, _ := json.Marshal(j.Result)
	var res RetrimResult
	_ = json.Unmarshal(b, &res)
	if strings.Join(res.Done, ",") != "C:,D:" || len(res.Failed) != 0 {
		t.Fatalf("resultado: %+v", res)
	}
	calls := 0
	for _, call := range m.X.Calls {
		if strings.EqualFold(call.Exe, "defrag.exe") {
			calls++
			if len(call.Args) != 3 || call.Args[1] != "/L" || call.Args[2] != "/H" {
				t.Errorf("defrag chamado com %v (só retrim é permitido)", call.Args)
			}
		}
	}
	if calls != 2 {
		t.Errorf("defrag chamado %d vezes", calls)
	}
	c.json("GET", "/api/ssd", nil, &v)
	for _, vol := range v.Volumes {
		if vol.LastRetrim == nil || vol.LastRetrim.Before(before.Add(-time.Second)) {
			t.Errorf("%s: data do retrim não atualizada: %v", vol.Letter, vol.LastRetrim)
		}
	}
	// Only letters of SSD volumes are accepted.
	if code, _ := c.do("POST", "/api/ssd/retrim", RetrimRequest{Letters: []string{"Z:"}}, nil); code != http.StatusBadRequest {
		t.Errorf("letra desconhecida: esperado 400, veio %d", code)
	}
}

func TestPerfCapture(t *testing.T) {
	m, c := setupServer(t)
	var v PerfView
	c.json("GET", "/api/perf", nil, &v)
	if !v.Ready || v.Version != "2.6.0" || len(v.Processes) == 0 || v.Processes[0].Exe != "cs2.exe" || len(v.Sources) == 0 {
		t.Fatalf("página: %+v", v)
	}
	capture := func(req PerfCaptureRequest) Job {
		t.Helper()
		var j Job
		c.json("POST", "/api/perf/capture", req, &j)
		return c.wait(j.ID)
	}
	result := func(j Job) perfRunResult {
		t.Helper()
		b, _ := json.Marshal(j.Result)
		var r perfRunResult
		_ = json.Unmarshal(b, &r)
		return r
	}
	j := capture(PerfCaptureRequest{Process: "cs2.exe", Seconds: 20, Label: "antes"})
	if j.Status != JobDone {
		t.Fatalf("medição: %+v", j)
	}
	r := result(j)
	s := r.Summary
	if r.Label != "antes" || r.Process != "cs2.exe" || r.Swapchains != 2 || s.Frames < 7000 || s.AvgFPS < 380 || s.AvgFPS > 460 ||
		s.Limiter != "gpu" || s.PresentMode != "Hardware: Independent Flip" || s.Low1FPS <= 0 || s.Low1FPS >= s.AvgFPS ||
		s.PCLatencyAvg < 7 || s.PCLatencyAvg > 10 || s.InputLatencyAvg < 6 || s.InputLatencyAvg > 9 || len(r.Series) == 0 || len(r.Series) > 1200 {
		t.Fatalf("resultado: %+v", r)
	}
	// PresentMon got the documented options, hotkey mode by default.
	var pmArgs []string
	for _, call := range m.X.Calls {
		if strings.EqualFold(call.Exe, presentMonName) {
			pmArgs = call.Args
		}
	}
	joined := strings.Join(pmArgs, " ")
	for _, want := range []string{"--process_name cs2.exe", "--timed 20", "--terminate_after_timed", "--hotkey SCROLLLOCK", "--session_name AxionPresentMon", "--stop_existing_session", "--no_console_stats"} {
		if !strings.Contains(joined, want) {
			t.Errorf("argumento ausente %q em %s", want, joined)
		}
	}
	if _, err := os.Stat(filepath.Join(m.Platform.DataDir, "desempenho", r.ID+".csv")); err != nil {
		t.Errorf("CSV não guardado com a medição: %v", err)
	}
	// More runs: one more "antes" (countdown mode) and two "depois".
	if j := capture(PerfCaptureRequest{Process: "cs2", Seconds: 20, Mode: "delay", Delay: 5, Label: "antes"}); j.Status != JobDone {
		t.Fatalf("contagem regressiva: %+v", j)
	}
	var after []string
	for i := 0; i < 2; i++ {
		j := capture(PerfCaptureRequest{Process: "cs2.exe", Seconds: 20, Label: "depois"})
		if j.Status != JobDone {
			t.Fatalf("depois: %+v", j)
		}
		after = append(after, result(j).ID)
	}
	c.json("GET", "/api/perf", nil, &v)
	if len(v.Runs) != 4 || strings.Join(v.Labels, ",") != "depois,antes" {
		t.Fatalf("lista: %d medições, rótulos %v", len(v.Runs), v.Labels)
	}
	var before []string
	for _, run := range v.Runs {
		if run.Label == "antes" {
			before = append(before, run.ID)
		}
		if run.Series != nil {
			t.Error("a lista não deveria trazer a série do gráfico")
		}
	}
	var cmp PerfComparison
	c.json("POST", "/api/perf/compare", PerfCompareRequest{A: before, B: after, LabelA: "antes", LabelB: "depois"}, &cmp)
	if !cmp.Enough || len(cmp.Rows) < 5 || cmp.Rows[0].Key != "avgFps" || cmp.Rows[0].A.N != 2 || cmp.Rows[0].P < 0 || len(cmp.RunsA) != 2 {
		t.Fatalf("comparação: %+v", cmp)
	}
	if code, _ := c.do("POST", "/api/perf/compare", PerfCompareRequest{A: before, B: before}, nil); code != http.StatusBadRequest {
		t.Errorf("mesmo grupo dos dois lados: %d", code)
	}
	// One run with the chart series; rename; delete.
	var full perfRunResult
	c.json("GET", "/api/perf/run/"+after[0], nil, &full)
	if len(full.Series) == 0 {
		t.Error("série do gráfico ausente")
	}
	var renamed perfRunResult
	c.json("POST", "/api/perf/label", PerfIDRequest{ID: after[0], Label: " depois do EXPO "}, &renamed)
	if renamed.Label != "depois do EXPO" {
		t.Errorf("rótulo: %q", renamed.Label)
	}
	c.json("POST", "/api/perf/delete", PerfIDRequest{ID: after[1]}, nil)
	if _, err := os.Stat(filepath.Join(m.Platform.DataDir, "desempenho", after[1]+".csv")); !os.IsNotExist(err) {
		t.Error("CSV da medição apagada ficou")
	}
	for _, bad := range []PerfCaptureRequest{{Process: "cs2.exe --etl_file x"}, {Process: "cs2.exe", Seconds: 5}, {Process: "cs2.exe", Hotkey: "F12"}, {Process: "cs2.exe", Mode: "delay", Delay: 99}} {
		if code, _ := c.do("POST", "/api/perf/capture", bad, nil); code != http.StatusBadRequest {
			t.Errorf("%+v: esperado 400, veio %d", bad, code)
		}
	}
	if code, _ := c.do("GET", "/api/perf/run/abc", nil, nil); code != http.StatusBadRequest {
		t.Errorf("ID inválido: %d", code)
	}
	// Game not running, and PresentMon failing.
	m.PMNoGame = true
	if j := capture(PerfCaptureRequest{Process: "cs2.exe", Seconds: 10}); j.Status != JobFailed || !strings.Contains(j.Error, "nenhum quadro") {
		t.Errorf("sem jogo: %+v", j)
	}
	m.PMNoGame, m.PMFail = false, "error: failed to start trace session: access denied."
	if j := capture(PerfCaptureRequest{Process: "cs2.exe", Seconds: 10}); j.Status != JobFailed || !strings.Contains(j.Error, "access denied") {
		t.Errorf("falha do PresentMon: %+v", j)
	}
	if left, _ := filepath.Glob(filepath.Join(m.Platform.DataDir, "desempenho", "*.csv")); len(left) != 3 {
		t.Errorf("CSVs esperados: 3 medições, achados %v", left)
	}
}

type perfRunResult struct {
	ID         string        `json:"id"`
	Label      string        `json:"label"`
	Process    string        `json:"process"`
	Swapchains int           `json:"swapchains"`
	Summary    perfSummaryJS `json:"summary"`
	Series     []float32     `json:"series"`
}

type perfSummaryJS struct {
	Frames          int     `json:"frames"`
	AvgFPS          float64 `json:"avgFps"`
	Low1FPS         float64 `json:"low1Fps"`
	Limiter         string  `json:"limiter"`
	PresentMode     string  `json:"presentMode"`
	PCLatencyAvg    float64 `json:"pcLatencyAvg"`
	InputLatencyAvg float64 `json:"inputLatencyAvg"`
}

func TestValidExe(t *testing.T) {
	for _, ok := range []string{"cs2.exe", "Among Us.exe", "VALORANT-Win64-Shipping.exe", "r5apex.exe", "FortniteClient-Win64-Shipping.exe", "Game (x64).exe"} {
		if !validExe(ok) {
			t.Errorf("%q deveria valer", ok)
		}
	}
	for _, bad := range []string{"--etl_file.exe", "cs2.exe x.exe", "a  b.exe", `..\x.exe`, "x/y.exe", "cs2", "cs2.exe --x", "a -b.exe", strings.Repeat("a", 70) + ".exe"} {
		if validExe(bad) {
			t.Errorf("%q deveria ser recusado", bad)
		}
	}
}

// Memory Context Restore is judged from the firmware time of the last boot.
func TestBiosMCRFromPOSTTime(t *testing.T) {
	f := &facts.Facts{CPU: facts.CPUInfo{Name: "AMD Ryzen 7 7800X3D 8-Core Processor", Vendor: "AMD", IsX3D: true},
		Board: facts.BoardInfo{Manufacturer: "Gigabyte Technology Co., Ltd.", Product: "B650M H", BIOSVersion: "F6"},
		RAM:   facts.RAMInfo{Type: "DDR5", ConfiguredMTs: 4800, Modules: []facts.MemModule{{PartNumber: "KF556C40-16"}}}}
	item := func(ms int) BiosItem {
		f.Boot.FwPOSTms = ms
		for _, it := range buildBiosGuide(f).Items {
			if it.ID == "mcr" {
				return it
			}
		}
		t.Fatal("item do Memory Context Restore ausente em AM5")
		return BiosItem{}
	}
	if it := item(38200); it.Status != "info" || !strings.Contains(it.Current, "38,2 s") || !strings.Contains(it.Current, "repetindo") || !strings.Contains(it.Path, "Tweaker") {
		t.Errorf("POST longo: %+v", it)
	}
	if it := item(13900); it.Status != "ok" || !strings.Contains(it.Current, "13,9 s") {
		t.Errorf("POST curto: %+v", it)
	}
	if it := item(-1); it.Status != "info" {
		t.Errorf("sem tempo de POST: %+v", it)
	}
	for _, it := range buildBiosGuide(f).Items {
		if it.ID == "xmp" && (it.Status != "action" || !strings.Contains(it.Caution, "primeiro boot")) {
			t.Errorf("EXPO: %+v", it)
		}
	}
}

// A warning the user cannot act on (e.g. EXPO that does not boot) can be
// dismissed: it stays listed apart and stops lowering the score.
func TestDismissFinding(t *testing.T) {
	_, c := setupServer(t)
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise: %+v", j)
	}
	type rep struct {
		Overall  int `json:"overall"`
		Findings []struct {
			ID        string `json:"id"`
			Severity  string `json:"severity"`
			Dismissed bool   `json:"dismissed"`
		} `json:"findings"`
	}
	var before, after rep
	c.json("GET", "/api/report", nil, &before)
	var st1, st2 struct {
		Warning int `json:"warning"`
	}
	c.json("GET", "/api/state", nil, &st1)
	c.json("POST", "/api/findings/dismiss", DismissRequest{ID: "ram.xmp", Dismiss: true}, &after)
	last := after.Findings[len(after.Findings)-1]
	if last.ID != "ram.xmp" || !last.Dismissed || after.Overall <= before.Overall {
		t.Fatalf("dispensar: nota %d -> %d, último %+v", before.Overall, after.Overall, last)
	}
	c.json("GET", "/api/state", nil, &st2)
	if st2.Warning != st1.Warning-1 {
		t.Errorf("contagem de avisos: %d -> %d", st1.Warning, st2.Warning)
	}
	// Survives a new analysis, and can be restored.
	c.json("POST", "/api/scan", map[string]any{}, &j)
	c.wait(j.ID)
	c.json("GET", "/api/report", nil, &after)
	if f := after.Findings[len(after.Findings)-1]; f.ID != "ram.xmp" || !f.Dismissed {
		t.Errorf("dispensa perdida na nova análise: %+v", f)
	}
	c.json("POST", "/api/findings/dismiss", DismissRequest{ID: "ram.xmp", Dismiss: false}, &after)
	if after.Overall != before.Overall {
		t.Errorf("restaurar: nota %d, esperado %d", after.Overall, before.Overall)
	}
	for _, f := range before.Findings {
		if f.Severity == "critical" {
			if code, _ := c.do("POST", "/api/findings/dismiss", DismissRequest{ID: f.ID, Dismiss: true}, nil); code != http.StatusBadRequest {
				t.Errorf("crítico %s dispensado (código %d)", f.ID, code)
			}
			break
		}
	}
	if code, _ := c.do("POST", "/api/findings/dismiss", DismissRequest{ID: "nao.existe", Dismiss: true}, nil); code != http.StatusBadRequest {
		t.Errorf("ID inexistente: %d", code)
	}
}

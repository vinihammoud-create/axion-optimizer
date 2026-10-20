package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"axion/internal/catalog"
	"axion/internal/diag"
	"axion/internal/dpc"
	"axion/internal/facts"
	"axion/internal/games"
	"axion/internal/netdiag"
	"axion/internal/perf"
	"axion/internal/profile"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// Server exposes the API and the embedded interface on 127.0.0.1.
//
// Security model: the listener is loopback-only on a random port; every API
// request must carry the per-run token in the X-Axion-Token header. The
// desktop window injects the token; the browser fallback opens the page with
// a one-time code (BootCode) that the page trades for the token, so the
// address kept in the browser history holds nothing reusable. The Host
// header must match the listener (blocks DNS rebinding) and cross-site
// requests are refused (custom header + Origin/Sec-Fetch-Site checks).
// Mutating endpoints only run catalog operations; there is no endpoint that
// runs arbitrary commands.
type Server struct {
	App   *App
	Token string
	Host  string // 127.0.0.1:port
	UI    fs.FS
	srv   *http.Server
	ln    net.Listener

	bootMu sync.Mutex
	boot   map[string]time.Time // one-time codes -> expiry
}

// bootTTL is how long a browser code stays valid (the page uses it at once).
const bootTTL = 2 * time.Minute

// NewServer listens on a random loopback port.
func NewServer(a *App, ui fs.FS) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		ln.Close()
		return nil, err
	}
	s := &Server{App: a, Token: hex.EncodeToString(b[:]), Host: ln.Addr().String(), UI: ui, ln: ln, boot: map[string]time.Time{}}
	// No WriteTimeout: a file picker (POST /api/games/add) waits for the user.
	s.srv = &http.Server{Handler: s.routes(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 2 * time.Minute}
	return s, nil
}

// BootCode returns a new one-time code for opening the interface in a
// browser (http://host/#b=code). It expires in bootTTL.
func (s *Server) BootCode() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	code := hex.EncodeToString(b[:])
	now := time.Now()
	s.bootMu.Lock()
	defer s.bootMu.Unlock()
	for c, exp := range s.boot {
		if now.After(exp) {
			delete(s.boot, c)
		}
	}
	s.boot[code] = now.Add(bootTTL)
	return code, nil
}

// redeemBoot consumes a one-time code (constant-time comparison).
func (s *Server) redeemBoot(code string) bool {
	now := time.Now()
	s.bootMu.Lock()
	defer s.bootMu.Unlock()
	found := ""
	for c, exp := range s.boot {
		if subtle.ConstantTimeCompare([]byte(c), []byte(code)) == 1 && !now.After(exp) {
			found = c
		}
	}
	if found == "" {
		return false
	}
	delete(s.boot, found)
	return true
}

// hBoot trades a one-time code for the token (browser fallback only). It is
// the one API route without the token: the code is the credential, and only
// a same-origin page can send it.
func (s *Server) hBoot(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != "http://"+s.Host {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "origem não permitida"})
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := readJSON(r, &req); err != nil || !s.redeemBoot(req.Code) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "este link de acesso expirou ou já foi usado"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": s.Token})
}

// URL is the base address of the interface.
func (s *Server) URL() string { return "http://" + s.Host + "/" }

// Serve blocks serving requests.
func (s *Server) Serve() error {
	err := s.srv.Serve(s.ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Close stops the server.
func (s *Server) Close() error { return s.srv.Close() }

// Shutdown stops accepting requests and waits (up to max) for the ones in
// progress, such as a quick change running through RunExclusive.
func (s *Server) Shutdown(max time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), max)
	defer cancel()
	return s.srv.Shutdown(ctx)
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	api := func(pattern string, h func(w http.ResponseWriter, r *http.Request) (any, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			v, err := h(w, r)
			if err != nil {
				code := http.StatusInternalServerError
				switch {
				case errors.Is(err, ErrNoScan):
					code = http.StatusConflict
				case errors.Is(err, ErrBusy):
					code = http.StatusConflict
				case errors.Is(err, errBadRequest):
					code = http.StatusBadRequest
				case errors.Is(err, errNotFound):
					code = http.StatusNotFound
				}
				writeJSON(w, code, map[string]string{"error": strings.TrimPrefix(err.Error(), errBadRequest.Error()+": ")})
				return
			}
			if v == nil {
				v = map[string]bool{"ok": true}
			}
			writeJSON(w, http.StatusOK, v)
		})
	}

	api("GET /api/state", s.hState)
	api("POST /api/scan", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.StartScan() })
	api("GET /api/jobs/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		j, ok := s.App.Jobs.Get(r.PathValue("id"))
		if !ok {
			return nil, errNotFound
		}
		return j, nil
	})
	api("POST /api/jobs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]bool{"cancelled": s.App.Jobs.Cancel(r.PathValue("id"))}, nil
	})
	api("GET /api/facts", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if f := s.App.Facts(); f != nil {
			return f, nil
		}
		return nil, ErrNoScan
	})
	api("GET /api/report", s.hReport)
	api("POST /api/findings/dismiss", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req DismissRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		rep, err := s.App.DismissFinding(req)
		if err != nil {
			return nil, err
		}
		return ReportResp{Report: rep, ScoreNote: diag.ScoreNote}, nil
	})
	api("GET /api/tweaks", s.hTweaks)
	api("GET /api/plan", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.App.BuildPlan(r.URL.Query().Get("tier"), r.URL.Query().Get("category"))
	})
	api("POST /api/apply", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req ApplyRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartApply(req)
	})
	api("POST /api/revert", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req RevertRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartRevert(req)
	})
	api("POST /api/restorepoint", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.StartRestorePoint() })
	api("GET /api/history", s.hHistory)
	api("GET /api/excluded", func(w http.ResponseWriter, r *http.Request) (any, error) { return catalog.Excluded(), nil })
	api("GET /api/games", func(w http.ResponseWriter, r *http.Request) (any, error) {
		gl, cs2, err := s.App.GameViews()
		if err != nil {
			return nil, err
		}
		return map[string]any{"games": gl, "cs2": cs2, "unused": s.App.UnusedGames(), "unusedDays": games.UnusedDays}, nil
	})
	api("POST /api/games/rescan", func(w http.ResponseWriter, r *http.Request) (any, error) {
		s.App.rescanGames()
		return nil, nil
	})
	api("POST /api/games/add", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req struct {
			Exe  string `json:"exe"`
			Pick bool   `json:"pick"`
		}
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		if req.Pick {
			p, err := s.App.P.Native.PickFile("Escolha o executável do jogo", "Programas (*.exe)", "*.exe")
			if err != nil {
				return nil, err
			}
			if p == "" {
				return map[string]bool{"cancelled": true}, nil
			}
			req.Exe = p
		}
		if err := s.App.AddManualGame(req.Exe); err != nil {
			return nil, fmt.Errorf("%w: %v", errBadRequest, err)
		}
		return map[string]string{"exe": req.Exe}, nil
	})
	api("POST /api/games/remove", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req struct {
			Exe string `json:"exe"`
		}
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return nil, s.App.RemoveManualGame(req.Exe)
	})
	api("GET /api/cs2", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.CS2Guide() })
	api("POST /api/cs2/preset", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req struct {
			IDs []string `json:"ids"`
		}
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartCS2Preset(req.IDs)
	})
	api("GET /api/bios", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.BiosGuide() })
	api("GET /api/peripherals", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.Peripherals(), nil })
	api("POST /api/peripherals/measure", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req MeasureRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartMeasure(req)
	})
	api("GET /api/drivers", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.Drivers() })
	api("GET /api/drivers/old", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.App.OldDriverPackages(r.Context()), nil
	})
	api("POST /api/drivers/old/delete", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req OldPackagesDeleteRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartOldPackagesDelete(req)
	})
	api("POST /api/drivers/scan", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.StartDriverScan() })
	api("POST /api/drivers/download", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req DriverIDsRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartDriverDownload(req)
	})
	api("POST /api/drivers/install", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req DriverIDsRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartDriverInstall(req)
	})
	api("POST /api/drivers/hide", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req HideDriverRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return nil, s.App.Jobs.RunExclusive(func() error { return s.App.HideDriver(r.Context(), req) })
	})
	api("POST /api/drivers/gpu/download", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.StartGPUDownload() })
	api("POST /api/drivers/gpu/install", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return nil, s.App.Jobs.RunExclusive(s.App.InstallGPUDriver)
	})
	api("POST /api/drivers/gpu/delete", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return nil, s.App.Jobs.RunExclusive(s.App.DeleteGPUDownload)
	})
	api("GET /api/programs", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.Programs(), nil })
	api("POST /api/programs/uninstall", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req UninstallRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartUninstall(req)
	})
	api("POST /api/programs/leftovers", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req UninstallRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.RescanLeftovers(req)
	})
	api("POST /api/programs/quarantine", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req QuarantineRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		var b any
		err := s.App.Jobs.RunExclusive(func() (err error) { b, err = s.App.QuarantineLeftovers(req); return err })
		return b, err
	})
	api("POST /api/programs/restore", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req BatchRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		var b any
		err := s.App.Jobs.RunExclusive(func() (err error) { b, err = s.App.RestoreBatch(req); return err })
		return b, err
	})
	api("POST /api/programs/purge", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req BatchRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return nil, s.App.Jobs.RunExclusive(func() error { return s.App.DeleteBatch(req) })
	})
	api("GET /api/processes", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.Processes() })
	api("POST /api/processes/close", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req CloseRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.CloseProgram(req)
	})
	api("GET /api/startup", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.Startup(), nil })
	api("POST /api/startup/set", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req StartupRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		var v any
		err := s.App.Jobs.RunExclusive(func() (err error) { v, err = s.App.SetStartup(req); return err })
		return v, err
	})
	api("GET /api/perf", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.Perf(), nil })
	api("GET /api/perf/run/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.PerfRun(r.PathValue("id")) })
	api("POST /api/perf/capture", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req PerfCaptureRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartPerfCapture(req)
	})
	api("POST /api/perf/delete", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req PerfIDRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return nil, s.App.DeletePerfRun(req)
	})
	api("POST /api/perf/label", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req PerfIDRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.LabelPerfRun(req)
	})
	api("POST /api/perf/compare", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req PerfCompareRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.ComparePerf(req)
	})
	api("GET /api/sessions", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.Sessions(), nil })
	api("GET /api/sessions/run/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.App.SessionReport(r.PathValue("id"))
	})
	// The match report only changes the settings of Axion, not the PC: it
	// does not need RunExclusive (like SetProfile).
	api("POST /api/sessions/auto", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req AutoReportRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.SetAutoReport(req)
	})
	api("POST /api/sessions/seen", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req PerfIDRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return nil, s.App.MarkSessionSeen(req)
	})
	api("POST /api/sessions/delete", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req PerfIDRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return nil, s.App.DeleteSession(req)
	})
	api("GET /api/trial", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.Trial() })
	api("POST /api/trial/start", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req TrialRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartTrial(req)
	})
	api("POST /api/trial/undo", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req TrialIDRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartTrialUndo(req)
	})
	api("POST /api/trial/keep", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req TrialIDRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return nil, s.App.KeepTrial(req)
	})
	api("GET /api/latency", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.Latency() })
	api("GET /api/net", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.Net(), nil })
	api("GET /api/net/run/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.NetRun(r.PathValue("id")) })
	api("POST /api/net/test", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req NetTestRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartNetTest(req)
	})
	api("POST /api/net/delete", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req NetIDRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return nil, s.App.DeleteNetRun(req)
	})
	api("POST /api/net/label", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req NetIDRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.LabelNetRun(req)
	})
	api("POST /api/net/compare", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req NetCompareRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.CompareNet(req)
	})
	api("GET /api/obs", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.OBS(), nil })
	api("GET /api/dpc", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.DPC(), nil })
	api("GET /api/health", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.Health(), nil })
	api("GET /api/dpc/run/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.DPCRun(r.PathValue("id")) })
	api("POST /api/dpc/test", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req DPCRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartDPC(req)
	})
	api("POST /api/dpc/delete", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req DPCIDRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return nil, s.App.DeleteDPCRun(req)
	})
	api("POST /api/dpc/label", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req DPCIDRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.LabelDPCRun(req)
	})
	api("POST /api/dpc/compare", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req DPCCompareRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.CompareDPC(req)
	})
	api("GET /api/ssd", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.SSD(r.Context()), nil })
	api("POST /api/ssd/retrim", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req RetrimRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartRetrim(req)
	})
	api("GET /api/cleanup", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.App.Cleanup(r.URL.Query().Get("fresh") == "1"), nil
	})
	api("POST /api/cleanup/run", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req CleanupRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartCleanup(req)
	})
	api("GET /api/repair", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.Repair(), nil })
	api("POST /api/repair/run", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req RepairRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartRepair(req)
	})
	api("POST /api/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req struct {
			RestorePoint *bool   `json:"restorePoint"`
			Accepted     *bool   `json:"accepted"`
			Profile      *string `json:"profile"`
			Simple       *bool   `json:"simple"`
		}
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		if req.Profile != nil {
			if err := s.App.SetProfile(*req.Profile); err != nil {
				return nil, err
			}
		}
		err := s.App.UpdateSettings(func(st *Settings) {
			if req.RestorePoint != nil {
				st.RestorePoint = *req.RestorePoint
			}
			if req.Accepted != nil {
				st.Accepted = *req.Accepted
			}
			if req.Simple != nil {
				st.Simple = *req.Simple
			}
		})
		return s.App.Settings(), err
	})
	api("GET /api/apps", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.Apps(), nil })
	api("POST /api/apps/refresh", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.StartAppsRefresh() })
	api("POST /api/apps/install", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req AppsInstallRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartAppsInstall(req)
	})
	api("POST /api/apps/upgrade", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.StartAppsUpgradeAll() })
	api("POST /api/apps/select", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req AppsSelectRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return nil, s.App.SaveAppsSelection(req)
	})
	api("GET /api/evaluation", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.App.Evaluation() })
	api("POST /api/evaluate", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req EvalRequest
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		return s.App.StartEvaluation(req)
	})
	api("GET /api/profiles", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]any{"profiles": profile.All, "current": s.App.profile().ID, "chosen": s.App.Settings().Profile != ""}, nil
	})
	api("POST /api/open", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req struct {
			URL string `json:"url"`
		}
		if err := readJSON(r, &req); err != nil {
			return nil, err
		}
		if !s.allowedURL(req.URL) {
			return nil, fmt.Errorf("%w: endereço não permitido", errBadRequest)
		}
		return nil, s.App.P.Native.OpenURL(req.URL)
	})
	mux.HandleFunc("GET /api/export/html", func(w http.ResponseWriter, r *http.Request) {
		b, err := s.App.ExportHTML(r.URL.Query().Get("full") == "1")
		if err != nil {
			code := http.StatusInternalServerError
			if errors.Is(err, ErrNoScan) {
				code = http.StatusConflict
			}
			writeJSON(w, code, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="axion-relatorio-%s.html"`, time.Now().Format("20060102-1504")))
		_, _ = w.Write(b)
	})
	api("GET /api/export", func(w http.ResponseWriter, r *http.Request) (any, error) {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="axion-relatorio-%s.json"`, time.Now().Format("20060102-1504")))
		return s.App.Export(r.URL.Query().Get("full") == "1"), nil
	})

	static := http.FileServer(http.FS(s.UI))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// Served from memory: never let the WebView store copies on disk.
		w.Header().Set("Cache-Control", "no-store")
		static.ServeHTTP(w, r)
	})
	return s.guard(mux)
}

var (
	errBadRequest = errors.New("requisição inválida")
	errNotFound   = errors.New("não encontrado")
)

const csp = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'; object-src 'none'"

func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", csp)
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		if r.Host != s.Host {
			http.Error(w, "host inválido", http.StatusMisdirectedRequest)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+s.Host {
			http.Error(w, "origem não permitida", http.StatusForbidden)
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
			http.Error(w, "requisição entre sites bloqueada", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
			if r.URL.Path == "/api/boot" {
				if r.Method != http.MethodPost || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
					writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "use POST com application/json"})
					return
				}
				s.hBoot(w, r)
				return
			}
			tok := r.Header.Get("X-Axion-Token")
			if subtle.ConstantTimeCompare([]byte(tok), []byte(s.Token)) != 1 {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token ausente ou inválido"})
				return
			}
			if r.Method == http.MethodPost && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "use application/json"})
				return
			}
			// The trail of what the interface asked for (the polling calls
			// are left out): after a crash it shows the last action.
			if !(r.Method == http.MethodGet && (r.URL.Path == "/api/state" || strings.HasPrefix(r.URL.Path, "/api/jobs/"))) {
				log.Printf("api: %s %s", r.Method, r.URL.Path)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) allowedURL(u string) bool {
	switch {
	case strings.HasPrefix(u, "https://") && !strings.ContainsAny(u, " \"'<>\\"):
		return true
	case strings.HasPrefix(u, "ms-settings:") && !strings.ContainsAny(u, " \"'<>\\/"):
		return true
	case u == "tool:devmgmt" || u == "tool:rstrui" || u == "tool:sysprot" || u == "tool:eventvwr":
		return true
	case sys.IsSteamUninstallURL(u):
		return true
	case u == "folder:"+s.App.P.DataDir:
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

func readJSON(r *http.Request, v any) error {
	b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%w: %v", errBadRequest, err)
	}
	if len(b) == 0 {
		b = []byte("{}")
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%w: JSON inválido", errBadRequest)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// StateResp is the header/status model.
type StateResp struct {
	Version            string    `json:"version"`
	Mock               bool      `json:"mock"`
	Elevated           bool      `json:"elevated"`
	Scanned            bool      `json:"scanned"`
	LastScan           time.Time `json:"lastScan"`
	Job                *Job      `json:"job,omitempty"`
	Settings           Settings  `json:"settings"`
	RebootPending      bool      `json:"rebootPending"`
	PendingRecommended int       `json:"pendingRecommended"`
	Applied            int       `json:"applied"`
	Drifted            int       `json:"drifted"`
	Overall            int       `json:"overall"`
	Grade              string    `json:"grade"`
	Critical           int       `json:"critical"`
	Warning            int       `json:"warning"`
	Machine            Machine   `json:"machine"`
	DataDir            string    `json:"dataDir"`
	// Session is the game session being recorded for the match report;
	// Unseen are the match reports not opened yet.
	Session *SessionLive    `json:"session,omitempty"`
	Unseen  []SessionNotice `json:"unseen,omitempty"`
}

// Machine is a one-line summary of the hardware.
type Machine struct {
	Host string `json:"host"`
	OS   string `json:"os"`
	CPU  string `json:"cpu"`
	GPU  string `json:"gpu"`
	RAM  string `json:"ram"`
}

func machineOf(f *facts.Facts) Machine {
	m := Machine{Host: f.Hostname, OS: strings.TrimSpace(fmt.Sprintf("%s %s (build %d.%d)", f.OS.Caption, f.OS.DisplayVersion, f.OS.Build, f.OS.UBR)), CPU: f.CPU.Name}
	if g := f.PrimaryGPU(); g != nil {
		m.GPU = g.Name
	}
	if f.RAM.TotalBytes > 0 {
		m.RAM = fmt.Sprintf("%.0f GB %s %d MT/s", float64(f.RAM.TotalBytes)/(1<<30), f.RAM.Type, f.RAM.ConfiguredMTs)
	}
	return m
}

func (s *Server) hState(w http.ResponseWriter, r *http.Request) (any, error) {
	a := s.App
	st := StateResp{Version: a.Version, Mock: a.P.IsMock, Elevated: a.P.Native.IsElevated(), Settings: a.Settings(), DataDir: a.P.DataDir}
	if j, ok := a.Jobs.Running(); ok {
		j.Log = nil
		j.Result = nil
		st.Job = &j
	}
	st.Session, st.Unseen = a.sessionLive(), a.sessionNotices()
	if f := a.Facts(); f != nil {
		st.Scanned, st.LastScan, st.Machine = true, a.LastScan(), machineOf(f)
		st.RebootPending = a.RebootPending()
		st.PendingRecommended = a.PendingRecommended()
		if rep := a.Report(); rep != nil {
			st.Overall, st.Grade = rep.Overall, rep.Grade
			for _, x := range rep.Findings {
				if x.Dismissed || x.OffProfile {
					continue
				}
				switch x.Severity {
				case diag.SevCritical:
					st.Critical++
				case diag.SevWarning:
					st.Warning++
				}
			}
		}
		for _, v := range a.States() {
			if v.ByAxion {
				st.Applied++
				if v.Drifted {
					st.Drifted++
				}
			}
		}
	}
	return st, nil
}

// ReportResp adds UI metadata to the diagnostic.
type ReportResp struct {
	*diag.Report
	ScoreNote string `json:"scoreNote"`
}

func (s *Server) hReport(w http.ResponseWriter, r *http.Request) (any, error) {
	rep := s.App.Report()
	if rep == nil {
		return nil, ErrNoScan
	}
	return ReportResp{Report: rep, ScoreNote: diag.ScoreNote}, nil
}

func (s *Server) hTweaks(w http.ResponseWriter, r *http.Request) (any, error) {
	list, err := s.App.TweakViews(r.URL.Query().Get("refresh") == "1")
	if err != nil {
		return nil, err
	}
	return map[string]any{"categories": tweaks.Categories, "tweaks": list}, nil
}

// HistoryOp is an operation formatted for display.
type HistoryOp struct {
	Kind     string `json:"kind"`
	Describe string `json:"describe"`
	Before   string `json:"before"`
	Skipped  bool   `json:"skipped"`
	Verified bool   `json:"verified"`
	Error    string `json:"error,omitempty"`
}

// HistoryEntry is a journal entry formatted for display.
type HistoryEntry struct {
	ID         string     `json:"id"`
	Batch      string     `json:"batch"`
	BatchLabel string     `json:"batchLabel,omitempty"`
	TweakID    string     `json:"tweakId"`
	Title      string     `json:"title"`
	Category   string     `json:"category"`
	Time       time.Time  `json:"time"`
	Status     string     `json:"status"`
	Error      string     `json:"error,omitempty"`
	RevertedAt *time.Time `json:"revertedAt,omitempty"`
	RevertNote string     `json:"revertNote,omitempty"`
	Reboot     string     `json:"reboot"`
	Changed    bool       `json:"changed"`
	Drifted    bool       `json:"drifted"`
	// Active: changes may still be on the machine (Desfazer is offered).
	// Pending: a failed apply or undo left changes; undoing again retries.
	Active  bool `json:"active"`
	Pending bool `json:"pending,omitempty"`
	// Legacy: written before 0.3 (or edited outside Axion); undo is limited.
	Legacy bool        `json:"legacy,omitempty"`
	Ops    []HistoryOp `json:"ops"`
}

func beforeText(o tweaks.OpRecord) string {
	if len(o.Before) == 0 {
		return ""
	}
	if o.Kind == "registry" {
		if v, ok := tweaks.SnapshotValue(o.Before); ok {
			return v.Display()
		}
	}
	s := string(o.Before)
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

func (s *Server) hHistory(w http.ResponseWriter, r *http.Request) (any, error) {
	states := s.App.States()
	var out []HistoryEntry
	for _, e := range s.App.Engine.J.Snapshot() {
		he := HistoryEntry{ID: e.ID, Batch: e.Batch, BatchLabel: e.BatchLabel, TweakID: e.TweakID, Title: e.TweakTitle, Category: e.Category, Time: e.Time, Status: e.Status,
			Error: e.Error, RevertedAt: e.RevertedAt, RevertNote: e.RevertNote, Reboot: e.Reboot, Changed: e.Changed(), Legacy: !e.Verified(),
			Active: e.Active()}
		he.Pending = he.Active && (e.Status == tweaks.StatusFailed || e.Status == tweaks.StatusPartialRev)
		if st, ok := states[e.TweakID]; ok && st.EntryID == e.ID {
			he.Drifted = st.Drifted
		}
		for _, o := range e.Ops {
			he.Ops = append(he.Ops, HistoryOp{Kind: o.Kind, Describe: o.Describe, Before: beforeText(o), Skipped: o.Skipped, Verified: o.Verified, Error: o.Error})
		}
		out = append(out, he)
	}
	return map[string]any{"entries": out}, nil
}

// Export bundles everything for support/troubleshooting: the CS2 page's
// verdicts and the hardware errors (time, kind, device, corrected, near a
// boot) go too, so a reviewer can check the classification without the PC.
// Unless full is asked for, what identifies the PC, the person and the
// network is replaced (see anon.go); the "anonymous" key says which one
// the file is.
func (a *App) Export(full bool) any {
	raw := a.exportRaw()
	raw["anonymous"] = !full
	if full {
		return raw
	}
	b, err := json.Marshal(raw)
	if err != nil {
		raw["anonymous"] = false
		return raw
	}
	var tree any
	if err := json.Unmarshal(b, &tree); err != nil {
		raw["anonymous"] = false
		return raw
	}
	// Before the first analysis the names are unknown; the patterns still
	// scrub the tests, captures and history kept from earlier sessions.
	host, users := a.anonNames()
	return newAnonymizer(host, users...).Apply(tree)
}

func (a *App) exportRaw() map[string]any {
	gl, cs2 := a.Games()
	a.mu.RLock()
	f, evs := a.facts, a.events
	a.mu.RUnlock()
	var cs2Page any
	if g, err := a.CS2Guide(); err == nil {
		cs2Page = map[string]any{"settings": g.Settings, "tips": g.Tips}
	}
	var whea []diag.WHEARow
	if evs != nil {
		whea = diag.WHEARows(evs, f)
	}
	var eval any
	if ev, err := a.Evaluation(); err == nil {
		eval = ev
	}
	return map[string]any{
		"generated":   time.Now(),
		"version":     a.Version,
		"profile":     a.profile().ID,
		"evaluation":  eval,
		"apps":        a.appsExport(),
		"facts":       f,
		"report":      a.Report(),
		"states":      a.States(),
		"history":     a.Engine.J.Snapshot(),
		"games":       gl,
		"cs2":         cs2,
		"cs2Page":     cs2Page,
		"whea":        whea,
		"measured":    a.measured(),
		"peripherals": a.Measurements(),
		"drivers":     a.driverExport(),
		"performance": perf.List(a.perfDir()),
		"sessions":    perf.List(a.sessionDir()),
		"network":     netdiag.List(a.netDir()),
		"dpc":         dpc.List(a.dpcDir()),
		"obs":         a.OBS().Rows,
		"dismissed":   a.Settings().Dismissed,
		"batches":     a.Batches(),
	}
}

// driverExport is the Drivers state without local file paths.
func (a *App) driverExport() any {
	st := a.driverStateCopy()
	if st.GPU != nil && st.GPU.File != "" {
		st.GPU.File = filepath.Base(st.GPU.File) // the name only: the path has the user's folder
	}
	return st
}

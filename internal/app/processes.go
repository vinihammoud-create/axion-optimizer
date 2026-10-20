package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"axion/internal/collect"
	"axion/internal/sys"
)

// Processes that must never be closed from the page: Windows itself,
// security, anti-cheats and Axion.
var protectedProcs = map[string]bool{
	"system": true, "registry": true, "idle": true, "system idle process": true, "memory compression": true, "secure system": true,
	"smss.exe": true, "csrss.exe": true, "wininit.exe": true, "winlogon.exe": true, "services.exe": true, "lsass.exe": true, "lsaiso.exe": true,
	"svchost.exe": true, "fontdrvhost.exe": true, "dwm.exe": true, "explorer.exe": true, "sihost.exe": true, "ctfmon.exe": true, "spoolsv.exe": true,
	"audiodg.exe": true, "msmpeng.exe": true, "nissrv.exe": true, "securityhealthservice.exe": true, "securityhealthsystray.exe": true,
	"taskhostw.exe": true, "wudfhost.exe": true, "conhost.exe": true, "dllhost.exe": true, "startmenuexperiencehost.exe": true,
	"shellexperiencehost.exe": true, "searchhost.exe": true, "textinputhost.exe": true, "applicationframehost.exe": true, "lockapp.exe": true,
	"vgc.exe": true, "vgtray.exe": true, "faceitservice.exe": true, "faceit.exe": true, "easyanticheat.exe": true, "easyanticheat_eos.exe": true,
	"beservice.exe": true, "eaanticheat.gameservice.exe": true, "axion.exe": true, "msedgewebview2.exe": true,
}

// protectedWhy explains protections that are not about Windows itself.
var protectedWhy = map[string]string{
	"msedgewebview2.exe": "mostra a interface do Axion (e de outros apps); fechar derrubaria esta janela",
}

// ownSession is the session Axion runs in, which is the user's desktop
// session. It comes from Axion's own process; only when that is not in the
// list (tests) the single desktop shell is used, and with more than one
// desktop (fast user switching, remote sessions) nothing is guessed.
func ownSession(list []sys.ProcessInfo) uint32 {
	self := uint32(os.Getpid())
	for _, p := range list {
		if p.PID == self {
			return p.Session
		}
	}
	var s uint32
	for _, p := range list {
		if strings.EqualFold(p.Name, "explorer.exe") && p.Session != 0 {
			if s != 0 && s != p.Session {
				return 0
			}
			s = p.Session
		}
	}
	return s
}

// ProcGroup is one program with all its processes.
type ProcGroup struct {
	Name      string   `json:"name"`
	Label     string   `json:"label"`
	Category  string   `json:"category"`
	Path      string   `json:"path,omitempty"`
	Count     int      `json:"count"`
	PIDs      []uint32 `json:"pids"`
	RAM       uint64   `json:"ram"`  // private memory
	CPU       float64  `json:"cpu"`  // % of the whole CPU
	User      bool     `json:"user"` // runs in the user's session
	Protected bool     `json:"protected"`
	Why       string   `json:"why,omitempty"`
}

// ProcessesView is the Processos page.
type ProcessesView struct {
	At       time.Time   `json:"at"`
	Groups   []ProcGroup `json:"groups"`
	RAMUsed  uint64      `json:"ramUsed"`
	RAMTotal uint64      `json:"ramTotal"`
	CPUTotal float64     `json:"cpuTotal"`
	Heavy    []string    `json:"heavy"`
	Honest   string      `json:"honest"`
}

// Processes samples CPU use over a short interval and groups the processes.
func (a *App) Processes() (ProcessesView, error) {
	first, err := a.P.Native.Processes()
	if err != nil {
		return ProcessesView{}, err
	}
	i1, k1, u1, _ := a.P.Native.CPUTimes()
	time.Sleep(700 * time.Millisecond)
	second, err := a.P.Native.Processes()
	if err != nil {
		return ProcessesView{}, err
	}
	i2, k2, u2, _ := a.P.Native.CPUTimes()
	total := float64((k2 + u2) - (k1 + u1))
	prev := map[uint32]sys.ProcessInfo{}
	for _, p := range first {
		prev[p.PID] = p
	}
	self := strings.ToLower(filepath.Base(os.Args[0]))
	userSession := ownSession(second)
	groups := map[string]*ProcGroup{}
	for _, p := range second {
		name := strings.ToLower(p.Name)
		if p.PID == 0 || name == "" {
			continue
		}
		g := groups[name]
		if g == nil {
			g = &ProcGroup{Name: p.Name, Label: p.Name, Path: p.Path}
			if cat, label, ok := collect.ProcCategory(p.Name); ok {
				g.Category, g.Label = cat, label
			}
			g.Protected = protectedProcs[name] || name == self || p.PID == 4
			groups[name] = g
		}
		g.Count++
		g.PIDs = append(g.PIDs, p.PID)
		g.RAM += p.PrivateMem
		if q, ok := prev[p.PID]; ok && total > 0 && p.CPUTime >= q.CPUTime {
			g.CPU += float64(p.CPUTime-q.CPUTime) / total * 100
		}
		if userSession != 0 && p.Session == userSession {
			g.User = true
		}
		if g.Path == "" {
			g.Path = p.Path
		}
	}
	v := ProcessesView{At: time.Now(), Honest: "Fechar programas em segundo plano libera memória e CPU para o jogo; não aumenta o FPS se o PC já não estava no limite. " +
		"Fechar um programa pelo Axion é como \"Finalizar tarefa\": alterações não salvas nele se perdem. Processos do Windows, do antivírus e dos anti-cheats não podem ser fechados aqui."}
	if total > 0 {
		v.CPUTotal = 100 - float64(i2-i1)/total*100
	}
	if ml, err := a.P.Native.MemoryLists(); err == nil {
		v.RAMTotal, v.RAMUsed = ml.Total, ml.Total-ml.Avail
	}
	for _, g := range groups {
		if !g.User {
			g.Protected = true
			g.Why = "processo do sistema ou serviço"
		} else if g.Protected {
			g.Why = "essencial ao Windows, à segurança ou a um anti-cheat"
			if w := protectedWhy[strings.ToLower(g.Name)]; w != "" {
				g.Why = w
			}
		}
		if g.Category == "rgb" || g.Category == "browser" || g.RAM > 1<<30 {
			v.Heavy = append(v.Heavy, g.Label)
		}
		v.Groups = append(v.Groups, *g)
	}
	sort.Slice(v.Groups, func(i, j int) bool {
		if v.Groups[i].User != v.Groups[j].User {
			return v.Groups[i].User
		}
		return v.Groups[i].RAM > v.Groups[j].RAM
	})
	sort.Strings(v.Heavy)
	return v, nil
}

// CloseRequest asks to close a program (all its processes).
type CloseRequest struct {
	Name string   `json:"name"`
	PIDs []uint32 `json:"pids"`
}

// CloseProgram ends the processes of a program in the user's session. The
// name is checked again for every PID so a reused PID is never touched.
// CloseResult reports how many processes were closed and why the others
// were not (a copy running as a Windows service, outside the session).
type CloseResult struct {
	Closed int      `json:"closed"`
	Failed []string `json:"failed,omitempty"`
}

func (a *App) CloseProgram(req CloseRequest) (CloseResult, error) {
	name := strings.ToLower(strings.TrimSpace(req.Name))
	if name == "" || len(req.PIDs) == 0 {
		return CloseResult{}, fmt.Errorf("%w: programa não informado", errBadRequest)
	}
	if protectedProcs[name] || name == strings.ToLower(filepath.Base(os.Args[0])) {
		return CloseResult{}, fmt.Errorf("%w: %s é protegido", errBadRequest, req.Name)
	}
	list, err := a.P.Native.Processes()
	if err != nil {
		return CloseResult{}, err
	}
	userSession := ownSession(list)
	byPID := map[uint32]sys.ProcessInfo{}
	for _, p := range list {
		byPID[p.PID] = p
	}
	closed := 0
	var errs []string
	for _, pid := range req.PIDs {
		p, ok := byPID[pid]
		if !ok || !strings.EqualFold(p.Name, name) {
			continue // already gone or the PID now belongs to another program
		}
		if userSession == 0 || p.Session != userSession {
			errs = append(errs, fmt.Sprintf("processo %d está em outra sessão do Windows (serviço ou outro usuário)", pid))
			continue
		}
		if err := a.P.Native.KillProcess(pid, p.Name); err != nil {
			errs = append(errs, fmt.Sprintf("processo %d: %v", pid, err))
			continue
		}
		closed++
	}
	if closed > 0 {
		a.refreshBackground() // the diagnostic lists heavy background programs
	}
	if closed == 0 && len(errs) > 0 {
		return CloseResult{Failed: errs}, fmt.Errorf("não foi possível fechar: %s", strings.Join(errs, "; "))
	}
	return CloseResult{Closed: closed, Failed: errs}, nil
}

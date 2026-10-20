package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"axion/internal/collect"
	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

var (
	srcTrimFsutil = tweaks.Source{Title: "Microsoft Learn — fsutil behavior: disabledeletenotify (TRIM ligado por padrão no NTFS, sem reiniciar)", URL: "https://learn.microsoft.com/windows-server/administration/windows-commands/fsutil-behavior"}
	srcTrimDefrag = tweaks.Source{Title: "Microsoft Learn — defrag: /L faz retrim; a tarefa agendada faz retrim do SSD uma vez por mês", URL: "https://learn.microsoft.com/windows-server/administration/windows-commands/defrag"}
	srcTrimStory  = tweaks.Source{Title: "Scott Hanselman (Microsoft), com a equipe de armazenamento do Windows — o Windows desfragmenta o SSD? (retrim e limite de fragmentação)", URL: "https://www.hanselman.com/blog/the-real-and-complete-story-does-windows-defragment-your-ssd"}
)

// SSDVolume is one fixed volume on the SSD maintenance card.
type SSDVolume struct {
	Letter     string     `json:"letter"`
	Label      string     `json:"label"`
	FS         string     `json:"fs"`
	Disk       string     `json:"disk"` // model of the physical disk
	SSD        bool       `json:"ssd"`
	LastRetrim *time.Time `json:"lastRetrim,omitempty"` // from the Application log (event 258)
}

// SSDView is the "Manutenção do SSD" card of the Limpeza page. TRIM, the
// optimizer task and the retrim dates are read live; the volume list comes
// from the last analysis.
type SSDView struct {
	Scanned    bool            `json:"scanned"`
	HasSSD     bool            `json:"hasSSD"`
	Trim       sys.TrimState   `json:"trim"`
	TrimErr    string          `json:"trimErr,omitempty"`
	TaskRead   bool            `json:"taskRead"`
	TaskExists bool            `json:"taskExists"`
	TaskOn     bool            `json:"taskOn"`
	TaskLast   *time.Time      `json:"taskLast,omitempty"`
	TaskResult int             `json:"taskResult"`
	TaskErr    string          `json:"taskErr,omitempty"`
	Volumes    []SSDVolume     `json:"volumes"`
	FixTrim    bool            `json:"fixTrim"` // fix.trim.on is applicable (last analysis)
	FixTask    bool            `json:"fixTask"` // fix.defrag.task is applicable
	Elevated   bool            `json:"elevated"`
	Honest     string          `json:"honest"`
	Sources    []tweaks.Source `json:"sources"`
}

// SSD builds the SSD maintenance view.
func (a *App) SSD(ctx context.Context) SSDView {
	v := SSDView{Elevated: a.P.Native.IsElevated(), Sources: []tweaks.Source{srcTrimFsutil, srcTrimDefrag, srcTrimStory},
		Honest: "O TRIM avisa o SSD, a cada arquivo apagado, quais blocos ficaram livres; o Windows liga isso por padrão. " +
			"Uma vez por mês a otimização automática faz o retrim: reenvia o aviso de todo o espaço livre, cobrindo avisos que se perdem com o disco ocupado. " +
			"Fazer retrim toda hora não acelera nada; ele serve depois de religar o TRIM, depois de apagar muita coisa ou quando a manutenção automática está atrasada."}
	if st, err := sys.QueryTrim(ctx, a.P); err != nil {
		v.Trim, v.TrimErr = sys.TrimState{NTFS: -1, ReFS: -1}, err.Error()
	} else {
		v.Trim = st
	}
	if t, err := a.P.Tasks.Get(sys.DefragTask); err != nil {
		v.TaskErr = err.Error()
	} else {
		v.TaskRead, v.TaskExists, v.TaskOn, v.TaskResult = true, t.Exists, t.Exists && t.Enabled, t.LastResult
		if !t.LastRun.IsZero() {
			last := t.LastRun
			v.TaskLast = &last
		}
	}
	last := collect.LastRetrims(a.P)
	f := a.Facts()
	if f == nil {
		return v
	}
	v.Scanned, v.HasSSD = true, f.HasSSD()
	models := map[int]string{}
	for _, d := range f.Disks {
		models[d.Number] = d.Model
	}
	for _, vol := range f.Volumes {
		sv := SSDVolume{Letter: strings.ToUpper(vol.Letter), Label: vol.Label, FS: vol.FS, SSD: vol.SSD}
		if vol.Disk >= 0 {
			sv.Disk = models[vol.Disk]
		}
		if t, ok := last[sv.Letter]; ok {
			sv.LastRetrim = &t
		}
		v.Volumes = append(v.Volumes, sv)
	}
	st := a.States()
	fixable := func(id string) bool { s, ok := st[id]; return ok && s.Applicable && s.Status != "on" }
	v.FixTrim, v.FixTask = fixable("fix.trim.on"), fixable("fix.defrag.task")
	return v
}

// RetrimRequest optionally limits the retrim to some drive letters.
type RetrimRequest struct {
	Letters []string `json:"letters"`
}

// RetrimResult is returned by the retrim job.
type RetrimResult struct {
	Done   []string `json:"done"`
	Failed []string `json:"failed"`
}

// retrimTargets lists the SSD volumes (NTFS/ReFS) of the last analysis.
func (a *App) retrimTargets(letters []string) ([]string, error) {
	f := a.Facts()
	if f == nil {
		return nil, ErrNoScan
	}
	want := map[string]bool{}
	for _, l := range letters {
		l = strings.ToUpper(strings.TrimSpace(l))
		if !strings.HasSuffix(l, ":") {
			l += ":"
		}
		want[l] = true
	}
	var out []string
	for _, v := range f.Volumes {
		l := strings.ToUpper(v.Letter)
		fs := strings.ToUpper(v.FS)
		if !v.SSD || (fs != "NTFS" && fs != "REFS") || len(l) != 2 || l[1] != ':' || l[0] < 'A' || l[0] > 'Z' {
			continue
		}
		if len(want) > 0 && !want[l] {
			continue
		}
		out = append(out, l)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: nenhum volume NTFS em SSD encontrado na última análise", errBadRequest)
	}
	return out, nil
}

// StartRetrim runs "defrag <volume> /L" (retrim, Microsoft Learn) on the SSD
// volumes. It only sends TRIM for free space; no data is moved.
func (a *App) StartRetrim(req RetrimRequest) (Job, error) {
	if err := a.requireAdmin("enviar TRIM ao SSD"); err != nil {
		return Job{}, err
	}
	targets, err := a.retrimTargets(req.Letters)
	if err != nil {
		return Job{}, err
	}
	return a.Jobs.Start("retrim", "Retrim do SSD", true, func(ctx context.Context, c *JobCtl) (any, error) {
		var res RetrimResult
		for i, l := range targets {
			c.Progress(float64(i)/float64(len(targets)), "Retrim em "+l+": avisando o SSD de todo o espaço livre")
			c.Logf("defrag %s /L /H", l)
			out, err := a.P.Run.Run(ctx, "defrag.exe", []string{l, "/L", "/H"}, sys.RunOpts{Timeout: 30 * time.Minute})
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastLine := ""
			for _, line := range strings.Split(strings.ReplaceAll(out.Stdout+"\n"+out.Stderr, "\r", "\n"), "\n") {
				if s := strings.TrimSpace(line); s != "" {
					c.Logf("%s", s)
					lastLine = s
				}
			}
			switch {
			case err != nil:
				res.Failed = append(res.Failed, l+" ("+err.Error()+")")
			case out.ExitCode != 0:
				why := fmt.Sprintf("código %d", out.ExitCode)
				if lastLine != "" {
					why += ": " + lastLine
				}
				res.Failed = append(res.Failed, l+" ("+why+")")
			default:
				res.Done = append(res.Done, l)
			}
		}
		c.Progress(1, "Concluído")
		if len(res.Done) == 0 {
			return res, fmt.Errorf("o retrim não foi concluído: %s", strings.Join(res.Failed, "; "))
		}
		// The diagnostic warns about late retrims: re-read the history.
		a.patchFacts(func(f *facts.Facts) { collect.RefreshRetrims(a.P, &f.Trim) })
		return res, nil
	})
}

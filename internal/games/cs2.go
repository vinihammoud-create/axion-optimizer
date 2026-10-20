package games

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// CS2Video holds the settings Counter-Strike 2 stores in cs2_video.txt.
type CS2Video struct {
	Path       string            `json:"path"`
	Modified   time.Time         `json:"modified"`
	Width      int               `json:"width"`
	Height     int               `json:"height"`
	RefreshHz  float64           `json:"refreshHz"` // 0 = not set in the file
	Fullscreen bool              `json:"fullscreen"`
	NoBorder   bool              `json:"noBorder"`
	VSync      int               `json:"vsync"`      // -1 unknown
	LowLatency int               `json:"lowLatency"` // NVIDIA Reflex: 0 off, 1 on, 2 on + boost, -1 unknown
	Raw        map[string]string `json:"raw"`
}

// LaunchNote comments one launch option.
type LaunchNote struct {
	Option string `json:"option"`
	Level  string `json:"level"` // info | warning
	Text   string `json:"text"`
}

// CS2Info is what the CS2 page shows.
type CS2Info struct {
	Installed bool      `json:"installed"`
	Game      *Game     `json:"game,omitempty"`
	AccountID string    `json:"accountId,omitempty"`
	Video     *CS2Video `json:"video,omitempty"`
	// VideoCloud: whether the account's Steam Cloud list (remotecache.vdf)
	// has cs2_video.txt: "sim", "não" or "" (list not read).
	VideoCloud    string       `json:"videoCloud,omitempty"`
	LaunchOptions string       `json:"launchOptions"`
	LaunchNotes   []LaunchNote `json:"launchNotes"`
	// Convars are the saved console variables of the account (lower-case
	// names), from cs2_user_convars_0_slot0.vcfg and cs2_machine_convars.vcfg.
	// A setting left at its default is not written there.
	Convars     map[string]string `json:"convars,omitempty"`
	ConvarFiles []string          `json:"convarFiles,omitempty"`
	// Steam is the client's download settings (0.7).
	Steam *SteamClient `json:"steam,omitempty"`
}

// CS2ConvarFiles are the files where CS2 saves settings per account.
var CS2ConvarFiles = []string{"cs2_user_convars_0_slot0.vcfg", "cs2_machine_convars.vcfg"}

// ParseConvars reads the "config" > "convars" block of a CS2 .vcfg file.
func ParseConvars(src string) (map[string]string, bool) {
	v, err := ParseVDF(src)
	if err != nil {
		return nil, false
	}
	block := v.Path("config", "convars")
	if block == nil {
		return nil, false
	}
	out := map[string]string{}
	for k, val := range block {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	return out, true
}

// ConvarInt returns an integer convar, or def when absent or not a number.
func (c CS2Info) ConvarInt(name string, def int) int {
	v, ok := c.Convars[strings.ToLower(name)]
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return def
	}
	return int(f)
}

// steamAccounts lists userdata account folders, most relevant first:
// the active/most recent user, then by cs2_video.txt modification time.
func (s *Scanner) steamAccounts(root string) []string {
	ud := winJoin(root, "userdata")
	entries, err := s.FS.ReadDir(ud)
	if err != nil {
		return nil
	}
	type acc struct {
		id  string
		mod time.Time
	}
	var list []acc
	for _, e := range entries {
		if !e.IsDir() || atoi64(e.Name()) <= 0 {
			continue
		}
		a := acc{id: e.Name()}
		if st, err := s.FS.Stat(winJoin(ud, e.Name(), "730", "local", "cfg", "cs2_video.txt")); err == nil {
			a.mod = st.ModTime()
		}
		list = append(list, a)
	}
	preferred := ""
	if s.Reg != nil {
		if v, err := s.Reg.GetValue(`HKCU\Software\Valve\Steam\ActiveProcess`, "ActiveUser"); err == nil && v.Exists && v.Int > 0 {
			preferred = strconv.FormatUint(v.Int, 10)
		}
	}
	if preferred == "" {
		preferred = s.mostRecentLogin(root)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if (list[i].id == preferred) != (list[j].id == preferred) {
			return list[i].id == preferred
		}
		return list[i].mod.After(list[j].mod)
	})
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.id
	}
	return out
}

// mostRecentLogin reads config\loginusers.vdf and converts the SteamID64
// marked MostRecent into the userdata account id.
func (s *Scanner) mostRecentLogin(root string) string {
	b, err := s.FS.ReadFile(winJoin(root, "config", "loginusers.vdf"))
	if err != nil {
		return ""
	}
	v, err := ParseVDF(string(b))
	if err != nil {
		return ""
	}
	users := v.Sub("users")
	for id64, raw := range users {
		u, ok := raw.(VDF)
		if !ok || u.Str("MostRecent") != "1" {
			continue
		}
		n, err := strconv.ParseUint(id64, 10, 64)
		if err != nil || n < 76561197960265728 {
			continue
		}
		return strconv.FormatUint(n-76561197960265728, 10)
	}
	return ""
}

// ParseCS2Video interprets cs2_video.txt.
func ParseCS2Video(src string) (*CS2Video, bool) {
	v, err := ParseVDF(src)
	if err != nil {
		return nil, false
	}
	cfg := v.Sub("video.cfg")
	if cfg == nil {
		return nil, false
	}
	out := &CS2Video{VSync: -1, LowLatency: -1, Raw: map[string]string{}}
	for k, val := range cfg {
		if s, ok := val.(string); ok {
			out.Raw[k] = s
		}
	}
	num := func(k string) (int, bool) {
		s, ok := out.Raw[k]
		if !ok {
			return 0, false
		}
		n, err := strconv.Atoi(strings.TrimSpace(s))
		return n, err == nil
	}
	out.Width, _ = num("setting.defaultres")
	out.Height, _ = num("setting.defaultresheight")
	if n, ok := num("setting.refreshrate_numerator"); ok && n > 0 {
		d, ok := num("setting.refreshrate_denominator")
		if !ok || d <= 0 {
			d = 1
		}
		out.RefreshHz = float64(n) / float64(d)
	}
	if n, ok := num("setting.fullscreen"); ok {
		out.Fullscreen = n == 1
	}
	if n, ok := num("setting.nowindowborder"); ok {
		out.NoBorder = n == 1
	}
	if n, ok := num("setting.mat_vsync"); ok {
		out.VSync = n
	}
	if n, ok := num("setting.r_low_latency"); ok {
		out.LowLatency = n
	}
	return out, true
}

// launch option notes: only claims with a clear mechanism.
var launchRules = []struct {
	match func(opt string) bool
	level string
	text  string
}{
	{func(o string) bool { return o == "-threads" }, "warning", "O CS2 dimensiona as threads sozinho. Forçar um número não tem ganho documentado e um valor errado limita o uso dos núcleos. Remova."},
	{func(o string) bool { return o == "-d3d9ex" || o == "-dxlevel" }, "warning", "Opção do CS:GO (DirectX 9). O CS2 usa DirectX 11/Vulkan e ignora isso."},
	{func(o string) bool { return o == "-high" }, "info", "Inicia o jogo com prioridade Alta. Só ajuda quando outro programa disputa a CPU; o perfil de prioridade do Axion faz o mesmo de forma persistente."},
	{func(o string) bool { return o == "-tickrate" }, "info", "Só vale para servidores locais (treino com bots). Partidas online usam o sistema sub-tick dos servidores da Valve."},
	{func(o string) bool { return o == "-allow_third_party_software" }, "warning", "Permite que programas de terceiros injetem código no jogo; a Valve informou (Trusted Mode) que isso pode reduzir seu Trust Factor. " +
		"O motivo mais comum é a Captura de Jogo do OBS, que o Trusted Mode bloqueia no CS2. Alternativa sem a opção: Captura de Janela ou de Tela no OBS, com o jogo em tela cheia sem bordas."},
	{func(o string) bool { return o == "-vulkan" }, "info", "Troca o DirectX 11 pelo Vulkan. O desempenho varia por GPU/driver; o padrão é DirectX 11. Meça antes e depois."},
	{func(o string) bool {
		return o == "-nojoy" || o == "-novid" || o == "-softparticlesdefaultoff" || o == "+cl_forcepreload"
	}, "info", "Herdado de listas do CS:GO. Sem efeito mensurável em FPS ou latência no CS2."},
}

// LaunchValue returns the number given to a launch option ("-refresh 165",
// "-freq 144"), the first of names found.
func LaunchValue(opts string, names ...string) (int, string, bool) {
	f := strings.Fields(strings.ToLower(opts))
	for i, o := range f {
		for _, n := range names {
			if o != n || i+1 >= len(f) {
				continue
			}
			if v, err := strconv.Atoi(f[i+1]); err == nil && v > 0 {
				return v, n, true
			}
		}
	}
	return 0, "", false
}

// AnalyzeLaunchOptions comments each recognised option.
func AnalyzeLaunchOptions(opts string) []LaunchNote {
	var out []LaunchNote
	seen := map[string]bool{}
	for _, f := range strings.Fields(strings.ToLower(opts)) {
		if !strings.HasPrefix(f, "-") && !strings.HasPrefix(f, "+") {
			continue
		}
		for _, r := range launchRules {
			if r.match(f) && !seen[f] {
				seen[f] = true
				out = append(out, LaunchNote{Option: f, Level: r.level, Text: r.text})
			}
		}
	}
	return out
}

// CS2 gathers the CS2 installation, video settings and launch options.
func (s *Scanner) CS2(games []Game) CS2Info {
	info := CS2Info{}
	for i := range games {
		if games[i].ID == "steam:730" {
			g := games[i]
			info.Game, info.Installed = &g, true
		}
	}
	root := s.SteamRoot()
	if root == "" {
		return info
	}
	info.Steam = s.steamClient(root)
	for _, acc := range s.steamAccounts(root) {
		p := winJoin(root, "userdata", acc, "730", "local", "cfg", "cs2_video.txt")
		if b, err := s.FS.ReadFile(p); err == nil && info.Video == nil {
			if v, ok := ParseCS2Video(string(b)); ok {
				v.Path = p
				if st, err := s.FS.Stat(p); err == nil {
					v.Modified = st.ModTime()
				}
				info.Video = v
				info.AccountID = acc
			}
		}
		if b, err := s.FS.ReadFile(winJoin(root, "userdata", acc, "config", "localconfig.vdf")); err == nil {
			if v, err := ParseVDF(string(b)); err == nil {
				apps := v.Path("UserLocalConfigStore", "Software", "Valve", "Steam", "apps")
				if app := apps.Sub("730"); app != nil {
					if info.AccountID == "" || info.AccountID == acc {
						info.LaunchOptions = app.Str("LaunchOptions")
						info.AccountID = acc
					}
				}
			}
		}
		if info.Video != nil {
			break
		}
	}
	if info.Video != nil && info.AccountID != "" {
		if in, ok := s.CS2VideoInCloud(root, info.AccountID); ok {
			info.VideoCloud = "não"
			if in {
				info.VideoCloud = "sim"
			}
		}
	}
	if info.AccountID != "" {
		for _, name := range CS2ConvarFiles {
			p := winJoin(root, "userdata", info.AccountID, "730", "local", "cfg", name)
			b, err := s.FS.ReadFile(p)
			if err != nil {
				continue
			}
			if cv, ok := ParseConvars(string(b)); ok {
				if info.Convars == nil {
					info.Convars = map[string]string{}
				}
				for k, v := range cv {
					if _, dup := info.Convars[k]; !dup {
						info.Convars[k] = v
					}
				}
				info.ConvarFiles = append(info.ConvarFiles, p)
			}
		}
	}
	info.LaunchNotes = AnalyzeLaunchOptions(info.LaunchOptions)
	return info
}

package collect

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"axion/internal/facts"
	"axion/internal/sys"
)

// CriticalService is a service that "debloat"/optimizer scripts commonly
// disable although Windows or popular apps need it. Default is the start type
// of a clean Windows 10/11 install (trigger-started services are Manual).
type CriticalService struct {
	Name    string
	Display string
	Impact  string
	Group   string // system | apps
	Default sys.ServiceStart
	// OnlyIf restricts the check (e.g. Wi-Fi service only with a Wi-Fi adapter).
	OnlyIf func(f *facts.Facts) bool
}

// CriticalServices is the list checked by the scan.
var CriticalServices = []CriticalService{
	{Name: "wuauserv", Display: "Windows Update", Impact: "o Windows deixa de receber atualizações de segurança e de drivers", Group: "system", Default: sys.StartManual},
	{Name: "UsoSvc", Display: "Update Orchestrator", Impact: "as atualizações não são agendadas nem concluídas", Group: "system", Default: sys.StartAutoDelayed},
	{Name: "BITS", Display: "Serviço de Transferência Inteligente em Segundo Plano", Impact: "downloads do Windows Update, da Store e de vários apps falham", Group: "system", Default: sys.StartManual},
	{Name: "CryptSvc", Display: "Serviços de Criptografia", Impact: "verificação de assinaturas, atualizações e instalação de drivers falham", Group: "system", Default: sys.StartAuto},
	{Name: "TrustedInstaller", Display: "Instalador de Módulos do Windows", Impact: "atualizações e recursos opcionais do Windows não instalam", Group: "system", Default: sys.StartManual},
	{Name: "Dhcp", Display: "Cliente DHCP", Impact: "a rede deixa de receber endereço IP automaticamente", Group: "system", Default: sys.StartAuto},
	{Name: "AudioSrv", Display: "Áudio do Windows", Impact: "o PC fica sem som", Group: "system", Default: sys.StartAuto},
	{Name: "AudioEndpointBuilder", Display: "Construtor de Pontos de Extremidade de Áudio", Impact: "dispositivos de áudio deixam de aparecer", Group: "system", Default: sys.StartAuto},
	{Name: "EventLog", Display: "Log de Eventos do Windows", Impact: "sem registro de eventos, diagnósticos e várias tarefas agendadas falham", Group: "system", Default: sys.StartAuto},
	{Name: "Winmgmt", Display: "Instrumentação de Gerenciamento do Windows (WMI)", Impact: "drivers, diagnósticos e muitos programas (inclusive launchers) falham", Group: "system", Default: sys.StartAuto},
	{Name: "BFE", Display: "Mecanismo de Filtragem Básica", Impact: "Firewall, IPsec e algumas VPNs e jogos deixam de funcionar", Group: "system", Default: sys.StartAuto},
	{Name: "mpssvc", Display: "Firewall do Windows Defender", Impact: "o PC fica sem firewall", Group: "system", Default: sys.StartAuto},
	{Name: "Appinfo", Display: "Informações de Aplicativo", Impact: "não é possível executar programas como administrador", Group: "system", Default: sys.StartManual},
	{Name: "FontCache", Display: "Cache de Fontes do Windows", Impact: "apps abrem mais devagar", Group: "system", Default: sys.StartAuto},
	{Name: "WerSvc", Display: "Relatório de Erros do Windows", Impact: "programas e o Windows deixam de gerar relatórios de falha, o que dificulta descobrir a causa de fechamentos e travamentos", Group: "apps", Default: sys.StartManual},
	{Name: "W32Time", Display: "Horário do Windows", Impact: "o relógio sai de sincronia (falhas de login, certificados e anti-cheat)", Group: "system", Default: sys.StartManual},
	{Name: "WlanSvc", Display: "Configuração Automática de WLAN", Impact: "o Wi-Fi para de funcionar", Group: "system", Default: sys.StartAuto,
		OnlyIf: func(f *facts.Facts) bool {
			for _, a := range f.Adapters {
				if a.Kind == "Wi-Fi" {
					return true
				}
			}
			return false
		}},
	{Name: "iphlpsvc", Display: "Auxiliar IP", Impact: "IPv6/Teredo usados pelo chat e multiplayer do Xbox deixam de funcionar", Group: "apps", Default: sys.StartAuto},
	{Name: "AppXSvc", Display: "Serviço de Implantação AppX", Impact: "apps da Microsoft Store e do Xbox não instalam nem atualizam", Group: "apps", Default: sys.StartManual},
	{Name: "ClipSVC", Display: "Serviço de Licença do Cliente", Impact: "apps da Microsoft Store e jogos do Game Pass não abrem", Group: "apps", Default: sys.StartManual},
	{Name: "TokenBroker", Display: "Gerenciador de Contas Web", Impact: "login com conta Microsoft em apps (Xbox, Store, Office) falha", Group: "apps", Default: sys.StartManual},
	{Name: "XblAuthManager", Display: "Gerenciador de Autenticação Xbox Live", Impact: "login no app Xbox e jogos do Game Pass falha", Group: "apps", Default: sys.StartManual},
	{Name: "XboxNetApiSvc", Display: "Serviço de Rede Xbox Live", Impact: "multiplayer de jogos Xbox/Game Pass pode falhar", Group: "apps", Default: sys.StartManual},
	{Name: "XboxGipSvc", Display: "Gerenciamento de Acessórios Xbox", Impact: "controles e acessórios Xbox não atualizam firmware nem são configurados", Group: "apps", Default: sys.StartManual},
	{Name: "XblGameSave", Display: "Salvamento de Jogos do Xbox Live", Impact: "os jogos salvos do Xbox/Game Pass deixam de ser enviados e baixados da nuvem", Group: "apps", Default: sys.StartManual},
	{Name: "TabletInputService", Display: "Serviço de Teclado Virtual e Painel de Manuscrito", Impact: "o teclado virtual (toque) e a caneta deixam de funcionar", Group: "apps", Default: sys.StartManual},
	{Name: "camsvc", Display: "Gerenciador de Acesso a Recursos", Impact: "apps não conseguem usar microfone e câmera (ex.: Discord)", Group: "apps", Default: sys.StartManual},
	{Name: "bthserv", Display: "Suporte de Bluetooth", Impact: "dispositivos Bluetooth deixam de funcionar", Group: "apps", Default: sys.StartManual},
	{Name: "hidserv", Display: "Serviço de Dispositivo de Interface Humana", Impact: "teclas de mídia e botões especiais de teclados e controles param", Group: "apps", Default: sys.StartManual},
}

// HostsPath is the hosts file (overridable in tests); the Windows folder
// comes from the platform, not from %SystemRoot%.
var HostsPath = func(paths sys.Paths) string {
	root := paths.WindowsDir
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "drivers", "etc", "hosts")
}

// ParsePagingFiles interprets the PagingFiles REG_MULTI_SZ value.
// A system-managed file anywhere means Windows can still grow the page file,
// so it wins over custom ones; maxMB is the total of the custom maxima.
func ParsePagingFiles(vals []string) (mode string, maxMB int) {
	auto, custom, system := false, false, false
	for _, v := range vals {
		fs := strings.Fields(v)
		if len(fs) == 0 {
			continue
		}
		if strings.HasPrefix(fs[0], "?:") {
			auto = true
			continue
		}
		if len(fs) >= 3 {
			mn, _ := strconv.Atoi(fs[1])
			mx, _ := strconv.Atoi(fs[2])
			if mn == 0 && mx == 0 {
				system = true
				continue
			}
			custom = true
			maxMB += mx
			continue
		}
		system = true
	}
	switch {
	case auto:
		return "auto", 0
	case system:
		return "system", 0
	case custom:
		return "custom", maxMB
	}
	return "none", 0
}

// CountHosts counts active entries of a hosts file, ignoring comments and
// the localhost mappings Windows ships.
func CountHosts(content string) (n int, sample []string) {
	sc := bufio.NewScanner(strings.NewReader(content))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fs := strings.Fields(line)
		if len(fs) < 2 {
			continue
		}
		local := true
		for _, h := range fs[1:] {
			switch strings.ToLower(h) {
			case "localhost", "localhost.localdomain", "ip6-localhost", "ip6-loopback":
			default:
				local = false
			}
		}
		if local {
			continue
		}
		n++
		if len(sample) < 5 {
			sample = append(sample, fs[1])
		}
	}
	return n, sample
}

func (c *collector) regVal(path, name string) sys.RegValue {
	v, err := c.p.Reg.GetValue(path, name)
	if err != nil {
		return sys.RegValue{}
	}
	return v
}

const (
	keyWUPolicy   = `HKLM\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate`
	keyWUPolicyAU = keyWUPolicy + `\AU`
	keyWUUX       = `HKLM\SOFTWARE\Microsoft\WindowsUpdate\UX\Settings`
	keyMemMgmt    = `HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Memory Management`
	keyTcpipParm  = `HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters`
)

func (c *collector) residue() {
	r := &c.f.Residue
	r.Readable = true
	r.UACOff = c.regInt(`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`, "EnableLUA", 1) == 0
	r.Tcp1323Opts = int(c.regInt(keyTcpipParm, "Tcp1323Opts", -1))
	r.TaskOffloadOff = c.regInt(keyTcpipParm, "DisableTaskOffload", 0) == 1
	r.TdrLevel = int(c.regInt(`HKLM\SYSTEM\CurrentControlSet\Control\GraphicsDrivers`, "TdrLevel", -1))
	r.LargeSystemCache = c.regInt(keyMemMgmt, "LargeSystemCache", 0) == 1
	r.PowerThrottlingOff = c.regInt(`HKLM\SYSTEM\CurrentControlSet\Control\Power\PowerThrottling`, "PowerThrottlingOff", 0) == 1
	r.PrioritySeparation = int(c.regInt(`HKLM\SYSTEM\CurrentControlSet\Control\PriorityControl`, "Win32PrioritySeparation", -1))
	r.NetworkThrottling = c.regInt(facts.KeySystemProfile, "NetworkThrottlingIndex", -1)
	// Memory compression: the state lives in the memory manager agent, read
	// through its WMI class (the one Get-MMAgent wraps). An answer that fails
	// leaves MMAgentRead false and the state unknown.
	if st, err := sys.ReadMMAgent(c.p.WMI); err == nil {
		r.MMAgentRead, r.MemoryCompression = true, st.MemoryCompression
	}
	pf := c.regVal(keyMemMgmt, "PagingFiles")
	switch {
	case !pf.Exists:
		r.PageFile = "none"
	case pf.Kind == sys.RegMultiSZ:
		r.PagingFilesRaw = pf.Strs
		r.PageFile, r.PageFileMaxMB = ParsePagingFiles(pf.Strs)
	default:
		r.PagingFilesRaw = []string{pf.Str}
		r.PageFile, r.PageFileMaxMB = ParsePagingFiles([]string{pf.Str})
	}
	if fo := c.regInt(keyMemMgmt, "FeatureSettingsOverride", -1); fo >= 0 && fo&3 == 3 && c.regInt(keyMemMgmt, "FeatureSettingsOverrideMask", -1) >= 0 {
		r.SpectreOverride = true
	}
	for _, id := range facts.NVMeOverrideIDs {
		if v := c.regVal(facts.KeyFeatureOverrides, id); v.Exists && v.Kind == sys.RegDWord && v.Int == 1 {
			r.NVMeOverrides = append(r.NVMeOverrides, id)
		}
	}
	seenDrv := map[string]bool{}
	for _, ctl := range c.f.Controllers {
		d := strings.ToLower(ctl.Driver)
		if d == "" || seenDrv[d] || !strings.HasPrefix(strings.ToUpper(ctl.PNP), "PCI\\") {
			continue
		}
		seenDrv[d] = true
		r.StorageDrivers = append(r.StorageDrivers, d)
	}
	sort.Strings(r.StorageDrivers)

	// Windows Update blocked by policies or by the "pause until 2077" trick.
	add := func(path, name, why string) {
		v := c.regVal(path, name)
		r.UpdateBlocks = append(r.UpdateBlocks, facts.RegRef{Path: path, Name: name, Display: v.Display(), Why: why})
	}
	for _, x := range []struct{ path, name, why string }{
		{keyWUPolicyAU, "NoAutoUpdate", "atualizações automáticas desativadas por política"},
		{keyWUPolicy, "DisableWindowsUpdateAccess", "acesso ao Windows Update bloqueado por política"},
		{keyWUPolicy, "SetDisableUXWUAccess", "tela do Windows Update escondida por política"},
		{keyWUPolicy, "DoNotConnectToWindowsUpdateInternetLocations", "conexão aos servidores do Windows Update bloqueada por política"},
	} {
		if c.regInt(x.path, x.name, 0) == 1 {
			add(x.path, x.name, x.why)
		}
	}
	if c.regInt(keyWUPolicyAU, "UseWUServer", 0) == 1 {
		srv := strings.ToLower(strings.TrimSpace(c.regStr(keyWUPolicy, "WUServer")))
		if srv == "" || strings.Contains(srv, "127.0.0.1") || strings.Contains(srv, "localhost") || strings.Contains(srv, "0.0.0.0") || strings.Contains(srv, "[::1]") {
			add(keyWUPolicyAU, "UseWUServer", "servidor de atualização falso configurado ("+firstNonEmpty(srv, "vazio")+")")
			if c.regVal(keyWUPolicy, "WUServer").Exists {
				add(keyWUPolicy, "WUServer", "endereço do servidor de atualização falso")
			}
		}
	}
	if d := c.regInt(keyWUUX, "FlightSettingsMaxPauseDays", -1); d > 35 {
		add(keyWUUX, "FlightSettingsMaxPauseDays", "limite de pausa estendido para "+strconv.FormatInt(d, 10)+" dias")
	}
	if exp := c.regStr(keyWUUX, "PauseUpdatesExpiryTime"); exp != "" {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(exp)); err == nil && time.Until(t) > 400*24*time.Hour {
			for _, n := range []string{"PauseUpdatesExpiryTime", "PauseUpdatesStartTime", "PauseFeatureUpdatesStartTime", "PauseFeatureUpdatesEndTime", "PauseQualityUpdatesStartTime", "PauseQualityUpdatesEndTime"} {
				if c.regVal(keyWUUX, n).Exists {
					add(keyWUUX, n, "atualizações pausadas até "+t.Format("02/01/2006"))
				}
			}
		}
	}

	// Services disabled by debloat scripts.
	for _, s := range CriticalServices {
		if s.OnlyIf != nil && !s.OnlyIf(c.f) {
			continue
		}
		info, err := c.p.Svc.Query(s.Name)
		if err != nil || !info.Exists || info.Start != sys.StartDisabled {
			continue
		}
		r.DisabledServices = append(r.DisabledServices, facts.DisabledService{Name: s.Name, Display: s.Display, Impact: s.Impact, Group: s.Group, Default: s.Default})
	}

	// Hosts file.
	r.HostsEntries = -1
	if c.p.IsMock {
		r.HostsEntries = 0
		return
	}
	if b, err := os.ReadFile(HostsPath(c.p.Native.Paths())); err == nil {
		r.HostsEntries, r.HostsSample = CountHosts(string(b))
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

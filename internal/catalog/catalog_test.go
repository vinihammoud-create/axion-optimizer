package catalog_test

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"axion/internal/catalog"
	"axion/internal/collect"
	"axion/internal/diag"
	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
	"axion/internal/tweaks"
)

func diagHas(f *facts.Facts, id string) bool {
	for _, x := range diag.Build(f, nil).Findings {
		if x.ID == id {
			return true
		}
	}
	return false
}

func setup(t *testing.T) (*mocksys.Mock, *facts.Facts, *tweaks.Engine) {
	t.Helper()
	m, err := mocksys.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := collect.Collect(context.Background(), m.Platform, nil)
	cat, err := tweaks.NewCatalog(catalog.All())
	if err != nil {
		t.Fatal(err)
	}
	j, err := tweaks.OpenJournal(m.DataDir, m.Key)
	if err != nil {
		t.Fatal(err)
	}
	e := tweaks.NewEngine(m.Platform, cat, j)
	e.Providers = append(e.Providers, catalog.ProfileProvider)
	return m, f, e
}

func TestCollectMockProfile(t *testing.T) {
	_, f, _ := setup(t)
	if len(f.Errors) > 0 {
		t.Fatalf("erros na coleta: %v", f.Errors)
	}
	checks := map[string]bool{
		"CPU X3D":            f.CPU.IsX3D && !f.CPU.IsDualCCDX3D && f.CPU.Vendor == "AMD" && f.CPU.Threads == 16,
		"GPU NVIDIA 12GB":    f.PrimaryGPU() != nil && f.PrimaryGPU().Vendor == "NVIDIA" && f.PrimaryGPU().VRAMMiB == 12288,
		"NVIDIA 566.36":      f.PrimaryGPU() != nil && f.PrimaryGPU().VendorVersion == "566.36",
		"RAM 32GB DDR5":      f.RAM.TotalBytes == 32<<30 && f.RAM.Type == "DDR5" && f.RAM.ConfiguredMTs == 4800,
		"disco de sistema":   f.SystemDisk() != nil && f.SystemDisk().WearPct == 81 && f.SystemDisk().BusType == "NVMe",
		"monitor 60/240":     len(f.Displays) == 1 && f.Displays[0].CurrentHz == 60 && f.Displays[0].MaxHzAtRes == 240,
		"ethernet com EEE":   f.ActiveAdapter() != nil && f.ActiveAdapter().Keywords["*EEE"].Current == "1",
		"VBS/HVCI":           f.Security.VBSRunning && f.Security.HVCIRunning,
		"FACEIT":             f.HasAntiCheat("FACEIT") && f.RequiresVBS(),
		"useplatformclock":   f.Boot.UsePlatformClock != "",
		"ReBAR off":          f.Nvidia.ReBAR == "off" && f.Nvidia.BAR1MiB == 256,
		"TCP normal":         f.Net.TCPAutoTuning == "normal" && f.Net.RSS == "enabled",
		"Win11 24H2":         f.OS.IsWin11 && f.OS.Build == 26100,
		"aceleração ligada":  f.Gaming.MouseAccelOn,
		"Game Bar instalada": f.Gaming.GameBarInstalled,
	}
	for name, ok := range checks {
		if !ok {
			t.Errorf("coleta incorreta: %s", name)
		}
	}
}

func TestCatalogMetadata(t *testing.T) {
	cats := map[string]bool{}
	for _, c := range tweaks.Categories {
		cats[c.ID] = true
	}
	seen := map[string]bool{}
	for _, tw := range catalog.All() {
		if seen[tw.ID] {
			t.Errorf("ID duplicado %s", tw.ID)
		}
		seen[tw.ID] = true
		if !cats[tw.Category] {
			t.Errorf("%s: categoria inválida %q", tw.ID, tw.Category)
		}
		if tw.Title == "" || len(tw.Summary) < 40 || len(tw.Detail) < 60 {
			t.Errorf("%s: texto insuficiente", tw.ID)
		}
		if tw.Evidence == 0 || tw.Risk == 0 {
			t.Errorf("%s: evidência/risco não definidos", tw.ID)
		}
		if len(tw.Sources) == 0 {
			t.Errorf("%s: sem fontes", tw.ID)
		}
		for _, s := range tw.Sources {
			if u, err := url.Parse(s.URL); err != nil || u.Scheme != "https" || s.Title == "" {
				t.Errorf("%s: fonte inválida %+v", tw.ID, s)
			}
		}
		if tw.Risk != tweaks.RiskSafe && tw.OneClick {
			t.Errorf("%s: só ajustes sem risco entram no 1 clique (risco %s)", tw.ID, tw.Risk)
		}
		if tw.Category == "avancado" && tw.OneClick {
			t.Errorf("%s: a categoria Avançado nunca entra no 1 clique", tw.ID)
		}
	}
	for _, ex := range catalog.Excluded() {
		if ex.Name == "" || ex.Reason == "" || ex.Verdict == "" {
			t.Errorf("exclusão incompleta: %+v", ex)
		}
	}
}

// No two tweaks may write the same resource unless they share a group.
func TestNoOverlappingTargets(t *testing.T) {
	_, f, _ := setup(t)
	owner := map[string]*tweaks.Tweak{}
	for _, tw := range catalog.All() {
		for _, op := range tw.Ops(f) {
			if prev, ok := owner[op.Target()]; ok && (prev.Group == "" || prev.Group != tw.Group) {
				t.Errorf("%s e %s alteram o mesmo recurso %s", prev.ID, tw.ID, op.Target())
			}
			owner[op.Target()] = tw
		}
	}
}

// Every applicable tweak must apply, verify, and revert to the exact
// previous machine state.
func TestApplyVerifyRevertEveryTweak(t *testing.T) {
	for _, tw := range catalog.All() {
		tw := tw
		t.Run(tw.ID, func(t *testing.T) {
			m, f, e := setup(t)
			roundTrip(t, m, f, e, tw)
		})
	}
}

// A machine broken by other optimizers: every correction must apply and
// revert exactly.
func TestCorrectionsOnBrokenMachine(t *testing.T) {
	ids := []string{"fix.bcd.testsigning", "fix.bcd.nointegrity", "fix.bcd.nx", "fix.uac", "fix.tcp.windowscaling", "fix.gpu.tdr", "fix.mem.largecache", "fix.services.system", "game.boottick.fix", "net.tcp.defaults",
		"fix.a11y.filterkeys", "fix.input.queue", "fix.kernel.threaddpc", "fix.bcd.limits", "input.usb.power", "fix.nvme.override", "fix.trim.on", "fix.defrag.task",
		"fix.bcd.hypervisor", "fix.power.throttling", "fix.priosep", "fix.net.throttling", "fix.mem.compression", "fix.nvdrs.vsync", "fix.nvdrs.shadercache"}
	for _, id := range ids {
		id := id
		t.Run(id, func(t *testing.T) {
			m, _, e := setup(t)
			breakMachine(m)
			f := collect.Collect(context.Background(), m.Platform, nil)
			tw := e.Catalog.Get(id)
			if tw == nil {
				t.Fatalf("otimização %s não existe", id)
			}
			if ok, why := tw.IsApplicable(f); !ok {
				t.Fatalf("deveria se aplicar ao PC quebrado: %s", why)
			}
			if tw.Recommendation(f).Level != tweaks.RecYes {
				t.Errorf("correção deveria ser recomendada: %+v", tw.Recommendation(f))
			}
			roundTrip(t, m, f, e, tw)
		})
	}
}

func breakMachine(m *mocksys.Mock) {
	m.BCD["testsigning"] = "Yes"
	m.BCD["nointegritychecks"] = "Yes"
	m.BCD["nx"] = "AlwaysOff"
	m.BCD["disabledynamictick"] = "Yes"
	_ = m.R.SetValue(`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`, "EnableLUA", mocksysDW(0))
	_ = m.R.SetValue(`HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters`, "Tcp1323Opts", mocksysDW(0))
	_ = m.R.SetValue(`HKLM\SYSTEM\CurrentControlSet\Control\GraphicsDrivers`, "TdrLevel", mocksysDW(0))
	_ = m.R.SetValue(`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Memory Management`, "LargeSystemCache", mocksysDW(1))
	_ = m.S.SetStart("wuauserv", sysStartDisabled)
	_ = m.S.SetStart("AudioSrv", sysStartDisabled)
	// A "ping optimizer" disabled autotuning.
	_, _ = m.Platform.Run.Run(context.Background(), "netsh.exe", []string{"int", "tcp", "set", "global", "autotuninglevel=disabled"}, sysRunOpts)
	// Input "latency" guides: Filter Keys switched on by the Shift hotkey,
	// threaded DPCs off, msconfig limits on processors and memory.
	_ = m.R.SetValue(`HKCU\Control Panel\Accessibility\Keyboard Response`, "Flags", mocksysSZ("127"))
	_ = m.R.SetValue(`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\kernel`, "ThreadDpcEnable", mocksysDW(0))
	m.BCD["numproc"] = "2"
	m.BCD["truncatememory"] = "0x40000000"
	// The 2026 "native NVMe" registry hack (and WER disabled by a "debloat").
	for _, id := range facts.NVMeOverrideIDs {
		_ = m.R.SetValue(facts.KeyFeatureOverrides, id, mocksysDW(1))
	}
	_ = m.S.SetStart("WerSvc", sysStartDisabled)
	// An "SSD optimization" guide: TRIM and the drive optimizer turned off.
	_, _ = m.Platform.Run.Run(context.Background(), "fsutil.exe", []string{"behavior", "set", "disabledeletenotify", "1"}, sysRunOpts)
	_ = m.T.SetEnabled(sys.DefragTask, false)
	// "FPS boost" packs: hypervisor off, EcoQoS off, background-services
	// priority separation.
	m.BCD["hypervisorlaunchtype"] = "Off"
	_ = m.R.SetValue(`HKLM\SYSTEM\CurrentControlSet\Control\Power\PowerThrottling`, "PowerThrottlingOff", mocksysDW(1))
	_ = m.R.SetValue(`HKLM\SYSTEM\CurrentControlSet\Control\PriorityControl`, "Win32PrioritySeparation", mocksysDW(0x1A))
	// A "competitive" optimizer pack: MMCSS network throttling off.
	_ = m.R.SetValue(facts.KeySystemProfile, "NetworkThrottlingIndex", mocksysDW(0xFFFFFFFF))
	// An optimizer pack: Disable-MMAgent -MemoryCompression (a WMI state in the mock).
	m.SetMemoryCompression(false)
	// An optimizer pack left in the NVIDIA Base Profile, then its "revert"
	// (V-Sync with a value the driver does not have, power mode Adaptive).
	d := m.N.DRS
	d.SetUser(sys.NVDRSBase, sys.NVDRSVSyncMode, 0x60925619)
	d.SetUser(sys.NVDRSBase, sys.NVDRSShaderCache, 0)
	d.SetUser(sys.NVDRSBase, sys.NVDRSPowerMode, sys.NVPowerAdaptive)
	d.SetUser(sys.NVDRSBase, sys.NVDRSTextureQuality, 0x14)
	d.SetUser(sys.NVDRSBase, sys.NVDRSOGLThreads, 1)
	d.SetUser(sys.NVDRSBase, sys.NVDRSUltraLowLatency, 1)
	d.SetUser(sys.NVDRSBase, sys.NVDRSPrerenderLimit, 1)
}

// Two tweaks own different bits of the same Flags value: each one must be
// undone without touching the other's bit.
func TestFlagBitsAreIndependent(t *testing.T) {
	m, _, e := setup(t)
	const key = `HKCU\Control Panel\Accessibility\Keyboard Response`
	_ = m.R.SetValue(key, "Flags", mocksysSZ("127")) // Filter Keys on + hotkey armed
	f := collect.Collect(context.Background(), m.Platform, nil)
	flags := func() string { v, _ := m.R.GetValue(key, "Flags"); return v.Str }
	for _, id := range []string{"fix.a11y.filterkeys", "input.a11y.hotkeys"} {
		if r := e.Apply(context.Background(), f, []string{id}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
			t.Fatalf("%s: %+v", id, r[0])
		}
	}
	if flags() != "122" {
		t.Fatalf("Flags após os dois ajustes = %s, esperado 122", flags())
	}
	f = collect.Collect(context.Background(), m.Platform, nil)
	if r := e.RevertTweaks(context.Background(), f, []string{"fix.a11y.filterkeys"}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("reverter Teclas de Filtro: %+v", r[0])
	}
	if flags() != "123" {
		t.Fatalf("reverter as Teclas de Filtro deveria religar só o bit 0x1: Flags = %s", flags())
	}
	if r := e.RevertTweaks(context.Background(), f, []string{"input.a11y.hotkeys"}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("reverter atalhos: %+v", r[0])
	}
	if flags() != "127" {
		t.Fatalf("estado original não restaurado: Flags = %s", flags())
	}
}

// The USB power tweak touches only the input devices where suspension is on.
func TestUSBPowerTouchesOnlyInputDevices(t *testing.T) {
	m, f, e := setup(t)
	tw := e.Catalog.Get("input.usb.power")
	ops := tw.Ops(f)
	if len(ops) != 2 {
		t.Fatalf("esperava 2 operações (WMI + SelectiveSuspendEnabled do teclado), obtido %d", len(ops))
	}
	for _, op := range ops {
		if !strings.Contains(strings.ToUpper(op.Target()), "VID_1A2C&PID_2124") {
			t.Errorf("operação fora do teclado com suspensão: %s", op.Target())
		}
	}
	if r := e.Apply(context.Background(), f, []string{"input.usb.power"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	rows, _ := m.W.Query(`root\WMI`, "SELECT * FROM MSPower_DeviceEnable")
	for _, r := range rows {
		inst := r.Str("InstanceName")
		want := !strings.Contains(inst, "VID_1A2C&PID_2124")
		if r.Bool("Enable") != want {
			t.Errorf("%s: Enable = %v, esperado %v (hubs não devem mudar)", inst, r.Bool("Enable"), want)
		}
	}
	v, _ := m.R.GetValue(`HKLM\SYSTEM\CurrentControlSet\Enum\`+mocksys.MockKbdUSB+`\Device Parameters`, "SelectiveSuspendEnabled")
	if v.Kind != sys.RegBinary || len(v.Bin) != 1 || v.Bin[0] != 0 {
		t.Errorf("SelectiveSuspendEnabled deveria virar binário 00: %+v", v)
	}
	f2 := collect.Collect(context.Background(), m.Platform, nil)
	if diagHas(f2, "per.suspend.1A2C-2124") {
		t.Errorf("achado de suspensão deveria sumir após aplicar")
	}
}

func roundTrip(t *testing.T, m *mocksys.Mock, f *facts.Facts, e *tweaks.Engine, tw *tweaks.Tweak) {
	t.Helper()
	ok, why := tw.IsApplicable(f)
	if !ok {
		t.Skipf("não se aplica ao PC simulado: %s", why)
	}
	before := m.World()
	keysBefore := m.R.KeyCount()
	res := e.Apply(context.Background(), f, []string{tw.ID}, tweaks.ApplyOpts{AllowBlocked: true})
	if len(res) != 1 {
		t.Fatalf("resultados inesperados: %+v", res)
	}
	if res[0].Outcome != tweaks.OutApplied && res[0].Outcome != tweaks.OutAlready {
		t.Fatalf("falhou ao aplicar (%s): %s", res[0].Outcome, res[0].Message)
	}
	st := e.StateOf(context.Background(), f, tw)
	if st.Status != "on" {
		t.Fatalf("estado após aplicar = %s, ops=%+v", st.Status, st.Ops)
	}
	if res[0].Outcome == tweaks.OutAlready {
		return
	}
	// The app re-reads the machine after applying: state and undo must work
	// with the new facts (the condition that made the tweak applicable may
	// be gone, e.g. the monitor is now at its max refresh rate).
	f = collect.Collect(context.Background(), m.Platform, nil)
	st = e.StateOf(context.Background(), f, tw)
	if st.Status != "on" || !st.ByAxion {
		t.Fatalf("com os fatos atualizados, estado = %s (byAxion=%v)", st.Status, st.ByAxion)
	}
	if !st.ByAxion {
		t.Errorf("estado não registra aplicação pelo Axion")
	}
	after := m.World()
	if len(mocksys.DiffWorld(before, after, "hklm\\software\\axion")) == 0 {
		t.Errorf("aplicar não mudou nada no sistema")
	}
	rev := e.RevertTweaks(context.Background(), f, []string{tw.ID}, tweaks.RevertOpts{})
	if len(rev) != 1 || rev[0].Outcome != tweaks.OutReverted {
		t.Fatalf("falhou ao reverter: %+v", rev)
	}
	restored := m.World()
	if d := mocksys.DiffWorld(before, restored); len(d) > 0 {
		t.Errorf("reversão não restaurou o estado original:\n  %s", strings.Join(d, "\n  "))
	}
	if kc := m.R.KeyCount() - m.R.KeyCountUnder(`HKLM\SOFTWARE\Axion`); kc > keysBefore {
		t.Errorf("reversão deixou chaves vazias: %d -> %d", keysBefore, kc)
	}
}

func TestBlockedTweakIsRefused(t *testing.T) {
	_, f, e := setup(t)
	res := e.Apply(context.Background(), f, []string{"adv.vbs.off"}, tweaks.ApplyOpts{})
	if res[0].Outcome != tweaks.OutBlocked {
		t.Fatalf("VBS com FACEIT instalado deveria ser bloqueado, obtido %s", res[0].Outcome)
	}
}

func TestConflictDetection(t *testing.T) {
	m, f, e := setup(t)
	res := e.Apply(context.Background(), f, []string{"input.mouse.accel"}, tweaks.ApplyOpts{})
	if res[0].Outcome != tweaks.OutApplied {
		t.Fatal(res[0].Message)
	}
	// Another program changes the value afterwards.
	_ = m.R.SetValue(`HKCU\Control Panel\Mouse`, "MouseSpeed", mocksysSZ("2"))
	rev := e.RevertTweaks(context.Background(), f, []string{"input.mouse.accel"}, tweaks.RevertOpts{})
	if rev[0].Outcome != tweaks.OutConflict {
		t.Fatalf("esperava conflito, obtido %s", rev[0].Outcome)
	}
	rev = e.RevertTweaks(context.Background(), f, []string{"input.mouse.accel"}, tweaks.RevertOpts{Force: true})
	if rev[0].Outcome != tweaks.OutReverted {
		t.Fatalf("revert forçado falhou: %+v", rev[0])
	}
}

func TestGroupSwitchRevertsPreviousPlan(t *testing.T) {
	m, f, e := setup(t)
	orig := m.World()["power|active"]
	if r := e.Apply(context.Background(), f, []string{"power.plan.high"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0].Message)
	}
	if r := e.Apply(context.Background(), f, []string{"power.plan.ultimate"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatal(r[0].Message)
	}
	if len(e.J.Active()) != 1 {
		t.Fatalf("apenas um plano deveria estar ativo no histórico, há %d", len(e.J.Active()))
	}
	e.RevertAll(context.Background(), f, tweaks.RevertOpts{})
	if got := m.World()["power|active"]; got != orig {
		t.Fatalf("plano original não restaurado: %s != %s", got, orig)
	}
}

func TestProfileProvider(t *testing.T) {
	m, f, e := setup(t)
	catalog.GamePath = func(exe string) string {
		if exe == "cs2.exe" {
			return `C:\Program Files (x86)\Steam\steamapps\common\Counter-Strike Global Offensive\game\bin\win64\cs2.exe`
		}
		return ""
	}
	defer func() { catalog.GamePath = func(string) string { return "" } }()
	before := m.World()
	ids := []string{catalog.ProfileID("cs2.exe", "priority"), catalog.ProfileID("cs2.exe", "fso")}
	for _, r := range e.Apply(context.Background(), f, ids, tweaks.ApplyOpts{}) {
		if r.Outcome != tweaks.OutApplied {
			t.Fatalf("%s: %s %s", r.TweakID, r.Outcome, r.Message)
		}
	}
	v, _ := m.R.GetValue(`HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options\cs2.exe\PerfOptions`, "CpuPriorityClass")
	if !v.Exists || v.Int != 3 {
		t.Fatalf("IFEO não gravado: %+v", v)
	}
	e.RevertAll(context.Background(), f, tweaks.RevertOpts{})
	if d := mocksys.DiffWorld(before, m.World()); len(d) > 0 {
		t.Fatalf("perfil não revertido: %v", d)
	}
	if n := m.R.KeyCount(); n <= 0 {
		t.Fatal("contagem de chaves inválida")
	}
}

// Every custom operation must be rebuildable from the journal alone.
func TestOpsAreRebuildable(t *testing.T) {
	m, f, e := setup(t)
	breakMachine(m)
	fb := collect.Collect(context.Background(), m.Platform, nil)
	catalog.GamePath = func(exe string) string { return `C:\Games\` + exe }
	defer func() { catalog.GamePath = func(string) string { return "" } }()
	var all []*tweaks.Tweak
	all = append(all, catalog.All()...)
	for _, feat := range catalog.ProfileFeatures {
		if tw := e.Resolve(catalog.ProfileID("cs2.exe", feat), f); tw != nil {
			all = append(all, tw)
		}
	}
	// Startup items of a logon task use the existing task ops; they must be
	// rebuildable from the journal as well. The providers are called directly:
	// this setup only registers the profile provider.
	for _, on := range []bool{true, false} {
		id := catalog.StartupID(`task|\GoogleUpdateTaskMachineUA`, on)
		tw := catalog.StartupProvider(id, f)
		if tw == nil {
			t.Fatalf("o provedor não montou %s", id)
		}
		all = append(all, tw)
	}
	for _, tw := range all {
		for _, facts := range []*facts.Facts{f, fb} {
			for _, op := range tw.Ops(facts) {
				fo, ok := op.(*tweaks.FuncOp)
				if !ok {
					continue
				}
				spec := fo.SpecJSON()
				if len(spec) == 0 {
					t.Errorf("%s: operação %s sem Spec", tw.ID, op.Target())
					continue
				}
				rec := &tweaks.OpRecord{Kind: op.Kind(), Target: op.Target(), Spec: spec}
				if rb := tweaks.Reconstruct(rec); rb == nil || rb.Target() != op.Target() {
					t.Errorf("%s: operação %s não pode ser reconstruída", tw.ID, op.Target())
				}
			}
		}
	}
}

// "No" on a tick/clock element is equivalent to the default: the fixes must
// not offer to "restore" it (real report, v0.1.0).
func TestBootFixesIgnoreNo(t *testing.T) {
	m, _, e := setup(t)
	m.BCD["disabledynamictick"] = "No"
	m.BCD["useplatformtick"] = "No"
	m.BCD["useplatformclock"] = "No"
	f := collect.Collect(context.Background(), m.Platform, nil)
	for _, id := range []string{"game.boottick.fix", "game.hpet.fix"} {
		if ok, why := e.Catalog.Get(id).IsApplicable(f); ok || !strings.Contains(why, "padrão") {
			t.Errorf("%s: aplicável=%v (%s)", id, ok, why)
		}
	}
}

// Undoing the telemetry tasks after Windows removed one of them re-enables
// the others (it used to stop at the missing task and leave them disabled).
func TestTasksRestoreSkipsRemovedTask(t *testing.T) {
	m, f, e := setup(t)
	const gone = `\Microsoft\Windows\Application Experience\ProgramDataUpdater`
	for _, p := range []string{`\Microsoft\Windows\Application Experience\Microsoft Compatibility Appraiser`, gone,
		`\Microsoft\Windows\Customer Experience Improvement Program\Consolidator`} {
		m.T.Add(p, true, nil)
	}
	if r := e.Apply(context.Background(), f, []string{"privacy.telemetry.tasks"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	m.T.Remove(gone) // a feature update removes one task
	res := e.RevertTweaks(context.Background(), f, []string{"privacy.telemetry.tasks"}, tweaks.RevertOpts{})
	if res[0].Outcome != tweaks.OutReverted {
		t.Fatalf("reverter: %+v", res[0])
	}
	for _, p := range []string{`\Microsoft\Windows\Application Experience\Microsoft Compatibility Appraiser`,
		`\Microsoft\Windows\Customer Experience Improvement Program\Consolidator`} {
		if ti, _ := m.T.Get(p); !ti.Enabled {
			t.Errorf("%s continuou desativada", p)
		}
	}
}

// A history record edited outside Axion cannot make the undo enable a task
// that the tweak does not manage.
func TestTasksRestoreIgnoresForeignPaths(t *testing.T) {
	m, f, e := setup(t)
	const evil = `\Evil\Persist`
	m.T.Add(evil, false, nil)
	m.T.Add(`\Microsoft\Windows\Customer Experience Improvement Program\Consolidator`, true, nil)
	if r := e.Apply(context.Background(), f, []string{"privacy.telemetry.tasks"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	path := filepath.Join(m.DataDir, "journal.json")
	raw, _ := os.ReadFile(path)
	var doc map[string]any
	_ = json.Unmarshal(raw, &doc)
	for _, x := range doc["entries"].([]any) {
		en := x.(map[string]any)
		for _, o := range en["ops"].([]any) {
			op := o.(map[string]any)
			var snap []map[string]any
			b, _ := json.Marshal(op["before"])
			var s string
			if json.Unmarshal(b, &s) == nil {
				b = []byte(s)
			}
			_ = json.Unmarshal(b, &snap)
			snap = append(snap, map[string]any{"path": evil, "exists": true, "enabled": true})
			nb, _ := json.Marshal(snap)
			op["before"] = json.RawMessage(nb)
		}
	}
	raw, _ = json.Marshal(doc)
	_ = os.WriteFile(path, raw, 0o644)
	j, _ := tweaks.OpenJournal(m.DataDir, m.Key)
	e2 := tweaks.NewEngine(m.Platform, e.Catalog, j)
	e2.RevertAll(context.Background(), f, tweaks.RevertOpts{Force: true})
	if ti, _ := m.T.Get(evil); ti.Enabled {
		t.Fatal("tarefa fora do ajuste foi ativada a partir de um registro adulterado")
	}
}

// Undoing "game.windowed" removes only its own option from the shared
// DirectX string: an option Settings added afterwards (Auto HDR) stays.
func TestDXPrefRestoreKeepsOtherOptions(t *testing.T) {
	m, f, e := setup(t)
	const key = `HKCU\Software\Microsoft\DirectX\UserGpuPreferences`
	_ = m.R.SetValue(key, "DirectXUserGlobalSettings", sys.SZ("VRROptimizeEnable=0;"))
	if r := e.Apply(context.Background(), f, []string{"game.windowed"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	v, _ := m.R.GetValue(key, "DirectXUserGlobalSettings")
	_ = m.R.SetValue(key, "DirectXUserGlobalSettings", sys.SZ(v.Str+"AutoHDREnable=1;"))
	res := e.RevertTweaks(context.Background(), f, []string{"game.windowed"}, tweaks.RevertOpts{})
	if res[0].Outcome != tweaks.OutReverted {
		t.Fatalf("reverter sem conflito: %+v", res[0])
	}
	v, _ = m.R.GetValue(key, "DirectXUserGlobalSettings")
	if v.Str != "VRROptimizeEnable=0;AutoHDREnable=1;" {
		t.Fatalf("valor depois de desfazer: %q", v.Str)
	}
}

// Undoing the fullscreen-optimizations flag keeps a compatibility mode the
// user set afterwards in the .exe properties.
func TestLayerFlagRestoreKeepsOtherFlags(t *testing.T) {
	m, f, e := setup(t)
	const key = `HKCU\Software\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Layers`
	exe := `C:\Jogos\teste\jogo.exe`
	tw := &tweaks.Tweak{ID: "t.layer", Category: "jogos", Title: "FSO", Evidence: tweaks.EvLow, Risk: tweaks.RiskSafe,
		Ops: func(*facts.Facts) []tweaks.Op {
			return []tweaks.Op{catalog.LayerFlagOp(exe, "DISABLEDXMAXIMIZEDWINDOWEDMODE")}
		}}
	cat, _ := tweaks.NewCatalog([]*tweaks.Tweak{tw})
	e2 := tweaks.NewEngine(m.Platform, cat, e.J)
	if r := e2.Apply(context.Background(), f, []string{"t.layer"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	v, _ := m.R.GetValue(key, exe)
	_ = m.R.SetValue(key, exe, sys.SZ(v.Str+" HIGHDPIAWARE"))
	if r := e2.RevertTweaks(context.Background(), f, []string{"t.layer"}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("reverter: %+v", r[0])
	}
	v, _ = m.R.GetValue(key, exe)
	if v.Str != "~ HIGHDPIAWARE" {
		t.Fatalf("valor depois de desfazer: %q", v.Str)
	}
	// Nothing else there: the value goes away as it was absent before.
	_ = m.R.SetValue(key, exe, sys.SZ("~ DISABLEDXMAXIMIZEDWINDOWEDMODE"))
	_ = m.R.DeleteValue(key, exe)
	if r := e2.Apply(context.Background(), f, []string{"t.layer"}, tweaks.ApplyOpts{}); r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar de novo: %+v", r[0])
	}
	if r := e2.RevertTweaks(context.Background(), f, []string{"t.layer"}, tweaks.RevertOpts{}); r[0].Outcome != tweaks.OutReverted {
		t.Fatalf("reverter de novo: %+v", r[0])
	}
	if v, _ := m.R.GetValue(key, exe); v.Exists {
		t.Fatalf("valor deveria ser removido: %q", v.Str)
	}
}

// Removing testsigning with BitLocker on suspends the protection for one
// restart (manage-bde), so the boot change does not ask for the recovery key.
func TestBCDFixSuspendsBitLocker(t *testing.T) {
	m, _, e := setup(t)
	m.BCD["testsigning"] = "Yes"
	f := collect.Collect(context.Background(), m.Platform, nil)
	f.Security.BitLockerOS = "on"
	r := e.Apply(context.Background(), f, []string{"fix.bcd.testsigning"}, tweaks.ApplyOpts{})
	if r[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", r[0])
	}
	found := false
	for _, c := range m.X.Calls {
		if strings.EqualFold(c.Exe, "manage-bde.exe") && strings.Join(c.Args, " ") == "-protectors -disable C: -RebootCount 1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("BitLocker não foi suspenso: %+v", m.X.Calls)
	}
}

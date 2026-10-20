package collect

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"axion/internal/facts"
	"axion/internal/sys"
)

const (
	chanDiagPerf = "Microsoft-Windows-Diagnostics-Performance/Operational"
	// windowsUpdateXPath: installations (19) and failures (20) of the last
	// 180 days (timediff counts milliseconds).
	windowsUpdateXPath = "*[System[Provider[@Name='Microsoft-Windows-WindowsUpdateClient'] and (EventID=19 or EventID=20) and TimeCreated[timediff(@SystemTime) <= 15552000000]]]"
	bootSamples        = 10
)

// notQualityKB lists the KB numbers that are not Windows quality updates:
// the Defender definitions and platform updates and the Malicious Software
// Removal Tool arrive several times a month and would hide a PC that stopped
// receiving the real updates.
var notQualityKB = map[string]bool{"2267602": true, "4052623": true, "890830": true}

var reKB = regexp.MustCompile(`(?i)\bKB(\d{4,8})\b`)

// health reads the Windows health checks. Each group is read on its own: one
// that fails is left unread and the rest still run.
func (c *collector) health() {
	h := &c.f.Health
	c.defender(&h.Defender)
	c.dirtyBit(h)
	c.updateHistory(&h.Updates)
	if c.f.Power.IsLaptop {
		c.batteryWear(&h.Battery)
	}
	c.bootHistory(&h.Boot)
}

func (c *collector) defender(d *facts.DefenderHealth) {
	d.SigAgeDays = -1
	rows, err := c.p.WMI.Query(`root\Microsoft\Windows\Defender`, "SELECT AntivirusEnabled, RealTimeProtectionEnabled, IsTamperProtected, AMRunningMode, AntivirusSignatureAge, AntivirusSignatureLastUpdated FROM MSFT_MpComputerStatus")
	if err != nil {
		c.errf("WMI MSFT_MpComputerStatus: %v", err)
	} else if len(rows) > 0 {
		r := rows[0]
		d.Read = true
		d.Enabled = r.Bool("AntivirusEnabled")
		d.RealTime = r.Bool("RealTimeProtectionEnabled")
		d.Tamper = r.Bool("IsTamperProtected")
		d.Mode = strings.TrimSpace(r.Str("AMRunningMode"))
		d.SigUpdated = r.Time("AntivirusSignatureLastUpdated")
		if _, ok := r["AntivirusSignatureAge"]; ok {
			d.SigAgeDays = int(r.Int("AntivirusSignatureAge"))
		}
	}
	// Another antivirus registered with Windows Security (the Defender entry
	// itself is not "another").
	if av, err := c.p.WMI.Query(`root\SecurityCenter2`, "SELECT displayName FROM AntiVirusProduct"); err == nil {
		for _, r := range av {
			n := strings.TrimSpace(r.Str("displayName"))
			if n != "" && !strings.Contains(strings.ToLower(n), "defender") {
				d.ThirdParty = n
				break
			}
		}
	}
	// The exclusions are only listed to an administrator: anyone else is
	// answered "N/A: Must be an administrator to view exclusions", which
	// must not be read as an exclusion.
	if rows, err := c.p.WMI.Query(`root\Microsoft\Windows\Defender`, "SELECT ExclusionPath, ExclusionExtension, ExclusionProcess FROM MSFT_MpPreference"); err == nil && len(rows) > 0 {
		paths, exts, procs := rows[0].Strings("ExclusionPath"), rows[0].Strings("ExclusionExtension"), rows[0].Strings("ExclusionProcess")
		if !hidden(paths) && !hidden(exts) && !hidden(procs) {
			d.ExclusionsRead = true
			d.ExclPaths, d.ExclExts, d.ExclProcs = clean(paths), clean(exts), clean(procs)
		}
	}
}

func hidden(list []string) bool {
	for _, s := range list {
		if strings.HasPrefix(strings.TrimSpace(s), "N/A") {
			return true
		}
	}
	return false
}

func clean(list []string) []string {
	var out []string
	for _, s := range list {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// dirtyBit lists the fixed volumes whose NTFS "dirty" bit is set.
func (c *collector) dirtyBit(h *facts.HealthInfo) {
	rows, err := c.p.WMI.Query(`root\cimv2`, "SELECT DriveLetter, DriveType, DirtyBitSet FROM Win32_Volume")
	if err != nil {
		c.errf("WMI Win32_Volume: %v", err)
		return
	}
	for _, r := range rows {
		letter := strings.ToUpper(strings.TrimSpace(r.Str("DriveLetter")))
		if letter == "" || r.Int("DriveType") != 3 {
			continue
		}
		h.DirtyRead = true
		if r.Bool("DirtyBitSet") {
			h.Dirty = append(h.Dirty, letter)
		}
	}
	sort.Strings(h.Dirty)
}

// updateHistory finds the newest quality update installed and the newest one
// that failed, from the events of the Windows Update client.
func (c *collector) updateHistory(u *facts.UpdateHealth) {
	evs, err := c.p.Events.Query("System", windowsUpdateXPath, 600)
	if err != nil {
		c.errf("Windows Update (eventos): %v", err)
		return
	}
	u.Read = true
	for _, e := range evs { // newest first
		kb := qualityKB(e.Data["updateTitle"])
		if kb == "" {
			continue
		}
		switch e.ID {
		case 19:
			if e.Time.After(u.Last) {
				u.Last, u.LastKB = e.Time, kb
			}
		case 20:
			if e.Time.After(u.Fail) {
				u.Fail, u.FailKB, u.FailCode = e.Time, kb, strings.TrimSpace(e.Data["errorCode"])
			}
		}
	}
}

// qualityKB returns the KB number of a Windows quality update title, or ""
// for anything else (apps, drivers, definitions).
func qualityKB(title string) string {
	m := reKB.FindStringSubmatch(title)
	if m == nil || notQualityKB[m[1]] {
		return ""
	}
	return m[1]
}

// batteryWear reads the capacity the battery still holds from the Windows
// power report (powercfg /batteryreport /xml): the XML is structured and not
// translated, unlike the program's text output.
func (c *collector) batteryWear(b *facts.BatteryHealth) {
	b.Cycles = -1
	dir := c.p.DataDir
	if dir == "" {
		dir = os.TempDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	out := filepath.Join(dir, "battery-report.xml")
	_ = os.Remove(out)
	defer os.Remove(out)
	ctx, cancel := context.WithTimeout(c.ctxOrBG(), 40*time.Second)
	defer cancel()
	res, err := c.p.Run.Run(ctx, "powercfg.exe", []string{"/batteryreport", "/xml", "/output", out, "/duration", "1"}, sys.RunOpts{Timeout: 40 * time.Second})
	if err != nil || res.ExitCode != 0 {
		return
	}
	data, err := os.ReadFile(out)
	if err != nil {
		return
	}
	if got, ok := ParseBatteryReport(data); ok {
		*b = got
	}
}

type batteryXML struct {
	Batteries []struct {
		ID         string `xml:"Id"`
		Relative   int    `xml:"RelativeCapacity"`
		Design     int    `xml:"DesignCapacity"`
		Full       int    `xml:"FullChargeCapacity"`
		CycleCount int    `xml:"CycleCount"`
	} `xml:"Batteries>Battery"`
}

// ParseBatteryReport reads the batteries of a powercfg battery report. With
// more than one battery the capacities add up. A report in relative units
// (RelativeCapacity = 1) or without a design capacity says nothing about wear.
func ParseBatteryReport(data []byte) (facts.BatteryHealth, bool) {
	var r batteryXML
	if err := xml.Unmarshal(data, &r); err != nil {
		return facts.BatteryHealth{}, false
	}
	b := facts.BatteryHealth{Cycles: -1}
	var names []string
	for _, x := range r.Batteries {
		if x.Relative == 1 || x.Design <= 0 || x.Full <= 0 {
			continue
		}
		b.DesignMWh += x.Design
		b.FullMWh += x.Full
		if x.CycleCount > 0 {
			if b.Cycles < 0 {
				b.Cycles = 0
			}
			b.Cycles += x.CycleCount
		}
		if n := strings.TrimSpace(x.ID); n != "" {
			names = append(names, n)
		}
	}
	if b.DesignMWh <= 0 {
		return facts.BatteryHealth{}, false
	}
	b.Name = strings.Join(names, " + ")
	// A new battery often holds slightly more than its label: no negative wear.
	b.WearPct = 100 - int((float64(b.FullMWh)*100/float64(b.DesignMWh))+0.5)
	if b.WearPct < 0 {
		b.WearPct = 0
	}
	b.Read = true
	return b, true
}

// bootHistory reads the boots Windows timed (event 100) and the programs it
// timed as slow to start (event 101).
func (c *collector) bootHistory(b *facts.BootHealth) {
	evs, err := c.p.Events.Query(chanDiagPerf, "*[System[(EventID=100)]]", bootSamples)
	if err != nil {
		c.errf("Diagnostics-Performance (boot): %v", err)
		return
	}
	for _, e := range evs {
		if e.ID != 100 {
			continue
		}
		total, main := atoiData(e.Data["BootTime"]), atoiData(e.Data["MainPathBootTime"])
		if total <= 0 || main <= 0 {
			continue
		}
		b.Samples = append(b.Samples, facts.BootSample{At: e.Time, TotalMs: total, MainMs: main,
			Degraded: strings.EqualFold(e.Data["BootIsDegradation"], "true")})
	}
	b.Read = true
	if len(b.Samples) == 0 {
		return
	}
	// Programs slow to start: one event per program per boot, the boot named by
	// its StartTime. Only the window of the boots read counts.
	oldest := b.Samples[len(b.Samples)-1].At.Add(-time.Hour)
	apps, err := c.p.Events.Query(chanDiagPerf, "*[System[(EventID=101)]]", 300)
	if err != nil {
		return
	}
	type acc struct {
		boots map[string]bool
		worst int
	}
	by := map[string]*acc{}
	for _, e := range apps {
		if e.ID != 101 || e.Time.Before(oldest) {
			continue
		}
		name := strings.TrimSpace(e.Data["Name"])
		delay := atoiData(e.Data["DegradationTime"])
		if name == "" || delay < facts.BootSlowAppMs {
			continue
		}
		a := by[name]
		if a == nil {
			a = &acc{boots: map[string]bool{}}
			by[name] = a
		}
		a.boots[e.Data["StartTime"]] = true
		if delay > a.worst {
			a.worst = delay
		}
	}
	for name, a := range by {
		if len(a.boots) >= facts.BootSlowAppBoots {
			b.Slow = append(b.Slow, facts.BootApp{Name: name, Boots: len(a.boots), WorstMs: a.worst})
		}
	}
	sort.Slice(b.Slow, func(i, j int) bool {
		if b.Slow[i].Boots != b.Slow[j].Boots {
			return b.Slow[i].Boots > b.Slow[j].Boots
		}
		return b.Slow[i].Name < b.Slow[j].Name
	})
}

func atoiData(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

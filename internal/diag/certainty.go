package diag

import "strings"

// Certainty says how firmly the reading backs the conclusion of a finding,
// apart from how much it weighs (Severity, which alone drives the score).
const (
	// CertConfirmed: the evidence is a direct reading that states the
	// problem (a registry, WMI or API value, a number measured against a
	// published limit, an entry of the event log).
	CertConfirmed = "confirmado"
	// CertPossible: the rule infers or uses a heuristic (a criterion of the
	// Axion as the only basis, a correlation, a suggestion that depends on
	// the use of the PC). Also the default: a rule never overclaims.
	CertPossible = "possivel"
	// CertInsufficient: the finding exists to say something could not be read.
	CertInsufficient = "insuficiente"
	// CertNormal: nothing to fix (every SevGood finding).
	CertNormal = "normal"
)

// CertaintyLabels name the certainties for the user.
var CertaintyLabels = map[string]string{
	CertConfirmed:    "Problema confirmado",
	CertPossible:     "Possível problema",
	CertInsufficient: "Informação insuficiente",
	CertNormal:       "Condição normal",
}

// certRules classifies every rule by its finding ID, read rule by rule. A key
// is the ID, or a prefix ending in ".*" for the IDs that carry a device or a
// disk ("disk.wear.*"); "ID:severity" refines one severity of the ID. When the
// same ID and severity mean different things, the finding sets Certainty
// itself (the cases marked "by the rule" below). SevGood is always
// CertNormal, whatever the table says; an ID not listed is CertPossible.
var certRules = map[string]string{
	// Windows and its updates.
	"os.activehours":    CertPossible, // a suggestion that depends on when the PC is used
	"os.elevated.other": CertNormal,
	"os.win10":          CertConfirmed, // build against the Microsoft lifecycle; the LTSC + FACEIT case is possivel by the rule
	"os.win11.eos":      CertConfirmed,
	"os.reboot":         CertConfirmed,
	"os.uptime":         CertPossible, // 7 days is the Axion's line
	"axion.drift":       CertConfirmed,

	// Monitor, memory, processor.
	"display.refresh.*":      CertConfirmed, // the driver lists the faster mode
	"display.refresh.*:info": CertPossible,  // on battery the notebook may lower it on purpose
	"ram.xmp:warning":        CertConfirmed, // the part number states the rated speed
	"ram.xmp":                CertPossible,  // the rating is unknown: only a hint
	"ram.single":             CertConfirmed,
	"ram.samechannel":        CertPossible, // read from the slot labels, which boards fill in their own way
	"ram.low":                CertPossible, // 16 GB is the Axion's criterion
	"cpu.x3d.single":         CertNormal,
	"cpu.x3d.dual":           CertConfirmed,
	"cpu.hybrid.win10":       CertConfirmed,
	"cpu.intel.vmin":         CertConfirmed, // microcode against Intel's published fix
	"cpu.am5.soc":            CertPossible,  // inferred from the BIOS date
	"power.battery":          CertConfirmed,
	"power.plans.dup":        CertConfirmed,
	"proc.background":        CertPossible,
	"startup.many":           CertPossible, // 9 programs is the Axion's line

	// Storage.
	"disk.health.*":         CertConfirmed,
	"disk.health.unknown.*": CertInsufficient,
	"disk.wear.*":           CertPossible, // 70% and 90% are the Axion's lines; beyond 100% is confirmado by the rule
	"disk.temp.*":           CertPossible, // ~70 °C is general, not the drive's own limit
	"disk.temp.files":       CertPossible,
	"disk.hdd":              CertConfirmed,
	"disk.free":             CertPossible,
	"disk.nvme.alert.*":     CertConfirmed,
	"disk.nvme.temp.*":      CertConfirmed,
	"disk.nvme.media.*":     CertConfirmed,
	"disk.nvme.unsafe.*":    CertConfirmed,
	"ssd.trim.off":          CertConfirmed,
	"ssd.optimize.missing":  CertConfirmed,
	"ssd.optimize.off":      CertConfirmed,
	"ssd.retrim.old.*":      CertPossible, // the log may have lost newer entries

	// Video card.
	"gpu.nodriver":           CertConfirmed,
	"gpu.driver.old":         CertPossible, // 6 months is the Axion's line
	"gpu.rebar":              CertPossible, // the gain varies by game
	"gpu.hags":               CertConfirmed,
	"gpu.pcie.width:warning": CertConfirmed, // read with the link at full speed or under load
	"gpu.pcie.width":         CertPossible,  // read at idle: does not confirm a problem
	"gpu.nvdrs.vsync":        CertPossible,  // forced V-Sync is a choice; an unknown value is confirmado by the rule
	"gpu.nvdrs.frl.*":        CertPossible,
	"gpu.nvdrs.shadercache":  CertConfirmed,
	"gpu.nvdrs.vrr":          CertNormal,
	"res.nvdrs":              CertPossible, // 3 items is the Axion's line
	"pnp.problems":           CertConfirmed,

	// Games, input and latency.
	"game.mode":        CertConfirmed,
	"game.dvr":         CertConfirmed,
	"input.accel":      CertPossible, // games with raw input are not affected
	"boot.hpet":        CertConfirmed,
	"boot.tick":        CertConfirmed,
	"input.filterkeys": CertConfirmed,
	"res.inputqueue":   CertConfirmed,
	"res.threaddpc":    CertConfirmed,
	"per.bt.*":         CertConfirmed,
	"per.lowspeed.*":   CertConfirmed,
	"per.rate.*":       CertConfirmed,
	"per.hub.*":        CertPossible,  // a maker's advice, not measured here
	"per.suspend.*":    CertPossible,  // the device may sleep, not seen sleeping
	"dpc.slow":         CertConfirmed, // routines over 1 ms; only a share of the processor is possivel by the rule

	// Security and anti-cheat.
	"sec.anticheat":         CertConfirmed,
	"sec.anticheat.vbs":     CertPossible, // required in waves, not from everyone
	"sec.anticheat.iommu":   CertPossible,
	"sec.anticheat.gc":      CertConfirmed,
	"sec.secureboot":        CertPossible, // depends on the games played
	"sec.defender":          CertPossible, // another antivirus may be active
	"sec.vbs":               CertPossible,
	"sec.driver.vulneravel": CertPossible, // matched by file and service name
	"res.dse":               CertConfirmed,
	"res.dep":               CertConfirmed,
	"res.uac":               CertConfirmed,
	"res.wu":                CertConfirmed,
	"res.spectre":           CertConfirmed,

	// Network.
	"net.tcp":            CertPossible, // "provavelmente" changed by an optimizer
	"net.wifi":           CertPossible,
	"net.link100":        CertConfirmed,
	"net.eee":            CertPossible,
	"net.speed:warning":  CertConfirmed, // fixed at 10/100 Mb/s on a connected adapter
	"net.speed":          CertPossible,
	"net.nic.defaults":   CertPossible,
	"net.bloat":          CertConfirmed,
	"net.valve:warning":  CertConfirmed, // measured loss
	"net.valve":          CertPossible,  // jitter alone asks for a second test
	"net.calls":          CertConfirmed, // measured against Microsoft's targets
	"res.tcp1323":        CertConfirmed,
	"res.offload":        CertConfirmed,
	"res.net.throttling": CertPossible,
	"res.hosts":          CertPossible, // 31 lines is the Axion's line

	// Settings left by other optimizers.
	"res.tdr":                 CertConfirmed,
	"res.largecache":          CertConfirmed,
	"res.mem.compression":     CertPossible,
	"res.pagefile:warning":    CertConfirmed,
	"res.pagefile":            CertPossible, // 8 GB is the Axion's line
	"res.svc.system":          CertConfirmed,
	"res.svc.apps":            CertConfirmed,
	"res.nvme.override":       CertConfirmed,
	"res.bootlimits":          CertPossible, // a limit that has no effect
	"res.bootlimits:critical": CertConfirmed,

	// Stability, temperature and the event log.
	"stab.kp41":          CertConfirmed,
	"stab.bsod":          CertConfirmed,
	"stab.tdr":           CertConfirmed,
	"stab.whea:critical": CertConfirmed,
	"stab.whea:warning":  CertConfirmed,
	"stab.whea":          CertPossible, // corrected errors without a symptom
	"therm.shutdown":     CertConfirmed,
	"therm.throttle":     CertConfirmed,
	"therm.now":          CertConfirmed, // a thermal zone says so; the frequency limit alone is possivel by the rule
	"therm.firmware":     CertPossible,  // the event does not say the cause
	"obs.*":              CertConfirmed, // read from the OBS log

	// "Saúde do Windows".
	"health.*":                   CertPossible,
	"health.defender.signatures": CertPossible, // Axion's criterion
	"health.defender.tamper":     CertConfirmed,
	"health.defender.exclusions": CertPossible, // Axion's criterion
	"health.update.recent":       CertPossible, // Axion's criterion
	"health.update.failing":      CertConfirmed,
	"health.disk.dirty":          CertConfirmed,
	"health.battery.wear":        CertPossible, // Axion's criterion
	"health.boot.time":           CertPossible, // Axion's criterion
	"health.boot.apps":           CertPossible, // Axion's criterion

	// Capacity (published minimums) and measurement. What was not read is
	// insuficiente by the rule.
	"cap.cpu":          CertPossible,
	"cap.cpu:critical": CertConfirmed,
	"cap.ram":          CertPossible, // below the editors' minimum is confirmado by the rule
	"cap.ram:critical": CertConfirmed,
	"cap.gpu":          CertPossible, // below the editors' minimum is confirmado by the rule
	"cap.gpu:critical": CertConfirmed,
	"cap.os":           CertConfirmed,
	"cap.display":      CertPossible,
	"cap.disk":         CertConfirmed,
	"meas.fps":         CertConfirmed,
	"meas.mouse":       CertConfirmed,
}

// validCertainty reports one of the four values.
func validCertainty(c string) bool {
	_, ok := CertaintyLabels[c]
	return ok
}

// ruleCertainty looks an ID up in certRules: the ID itself, then each of its
// prefixes as "prefix.*", the severity first at every step.
func ruleCertainty(id, sev string) (string, bool) {
	try := func(k string) (string, bool) {
		if c, ok := certRules[k+":"+sev]; ok {
			return c, true
		}
		c, ok := certRules[k]
		return c, ok
	}
	if c, ok := try(id); ok {
		return c, true
	}
	for i := strings.LastIndexByte(id, '.'); i > 0; i = strings.LastIndexByte(id[:i], '.') {
		if c, ok := try(id[:i] + ".*"); ok {
			return c, true
		}
	}
	return "", false
}

// certaintyOf is the certainty of a finding: CertNormal for SevGood, the one
// its rule set, the table's, and CertPossible otherwise. A finding that is not
// good is never "normal".
func certaintyOf(x Finding) string {
	if x.Severity == SevGood {
		return CertNormal
	}
	if validCertainty(x.Certainty) && x.Certainty != CertNormal {
		return x.Certainty
	}
	if c, ok := ruleCertainty(x.ID, x.Severity); ok && c != CertNormal {
		return c
	}
	return CertPossible
}

package facts

import "time"

// StepHealth is the collection step of the Windows health checks.
const StepHealth = "Saúde do Windows"

// A program is named as slow to start only when Windows timed it at least
// BootSlowAppMs late in at least BootSlowAppBoots of the boots read.
const (
	BootSlowAppMs    = 5000
	BootSlowAppBoots = 3
)

// HealthInfo is what the health checks read. Every group carries its own Read
// flag: a group that could not be read stays silent (a zero value is not
// "healthy" and not "broken").
type HealthInfo struct {
	Defender DefenderHealth `json:"defender"`
	// Dirty lists the fixed volumes with the NTFS "dirty" bit set (Windows
	// runs a disk check at the next boot); DirtyRead tells the volumes were read.
	DirtyRead bool          `json:"dirtyRead"`
	Dirty     []string      `json:"dirty,omitempty"`
	Updates   UpdateHealth  `json:"updates"`
	Battery   BatteryHealth `json:"battery"`
	Boot      BootHealth    `json:"boot"`
}

// DefenderHealth is Microsoft Defender as the Windows Security provider
// reports it.
type DefenderHealth struct {
	Read       bool      `json:"read"`
	Enabled    bool      `json:"enabled"`
	RealTime   bool      `json:"realTime"`
	Tamper     bool      `json:"tamper"`
	Mode       string    `json:"mode,omitempty"` // AMRunningMode: Normal, Passive, EDR Block Mode...
	SigAgeDays int       `json:"sigAgeDays"`     // -1 unknown
	SigUpdated time.Time `json:"sigUpdated"`
	// ThirdParty names another antivirus registered with Windows Security.
	ThirdParty string `json:"thirdParty,omitempty"`
	// Exclusions are only listed to an administrator; ExclusionsRead tells
	// they were.
	ExclusionsRead bool     `json:"exclusionsRead"`
	ExclPaths      []string `json:"exclPaths,omitempty"`
	ExclExts       []string `json:"exclExts,omitempty"`
	ExclProcs      []string `json:"exclProcs,omitempty"`
}

// UpdateHealth is the Windows Update history read from the System log (the
// WindowsUpdateClient events 19 and 20).
type UpdateHealth struct {
	Read bool `json:"read"`
	// Last is the newest quality or cumulative update (a KB) installed.
	Last   time.Time `json:"last"`
	LastKB string    `json:"lastKB,omitempty"`
	// Fail is the newest failed installation of a KB update.
	Fail     time.Time `json:"fail"`
	FailKB   string    `json:"failKB,omitempty"`
	FailCode string    `json:"failCode,omitempty"`
}

// BatteryHealth is the battery wear from the power report: how much of the
// designed capacity the battery still holds.
type BatteryHealth struct {
	Read      bool   `json:"read"`
	Name      string `json:"name,omitempty"`
	DesignMWh int    `json:"designMWh"`
	FullMWh   int    `json:"fullMWh"`
	Cycles    int    `json:"cycles"` // -1 unknown
	WearPct   int    `json:"wearPct"`
}

// BootSample is one boot as the Diagnostics-Performance log timed it.
type BootSample struct {
	At       time.Time `json:"at"`
	TotalMs  int       `json:"totalMs"`  // BootTime
	MainMs   int       `json:"mainMs"`   // MainPathBootTime: until the desktop is usable
	Degraded bool      `json:"degraded"` // Windows itself flagged the boot as slower than usual
}

// BootApp is a program Windows timed as slow to start during boot.
type BootApp struct {
	Name    string `json:"name"`
	Boots   int    `json:"boots"`   // boots (of the ones read) it slowed down
	WorstMs int    `json:"worstMs"` // largest delay it caused
}

// BootHealth is the boot-time history, newest first.
type BootHealth struct {
	Read    bool         `json:"read"`
	Samples []BootSample `json:"samples,omitempty"`
	Slow    []BootApp    `json:"slow,omitempty"`
}

package drivers

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"axion/internal/sys"
)

// DisplayClassGUID is the setup class of display adapters.
const DisplayClassGUID = "{4d36e968-e325-11ce-bfc1-08002be10318}"

// Package is a driver package of the Windows driver store, as
// "pnputil /enum-drivers /devices /format xml" lists it.
type Package struct {
	Published string    `json:"published"` // oem148.inf
	Original  string    `json:"original"`  // nvdmi.inf
	Provider  string    `json:"provider"`
	Class     string    `json:"class"`
	ClassGUID string    `json:"classGuid"`
	Version   string    `json:"version"` // 32.0.16.1714
	Date      time.Time `json:"date"`
	Devices   int       `json:"devices"` // devices the package is installed on
	Started   int       `json:"started"` // of those, the ones running now
}

type pnpXML struct {
	Drivers []struct {
		Name      string `xml:"DriverName,attr"`
		Original  string `xml:"OriginalName"`
		Provider  string `xml:"ProviderName"`
		Class     string `xml:"ClassName"`
		ClassGUID string `xml:"ClassGuid"`
		Version   string `xml:"DriverVersion"`
		Devices   []struct {
			Status string `xml:"Status"`
		} `xml:"Devices>Device"`
	} `xml:"Driver"`
}

// ParsePnpUtil reads the XML of "pnputil /enum-drivers /devices /format xml".
// The elements are not translated, unlike the text output. The date comes
// first in "DriverVersion" as month/day/year, whatever the language.
func ParsePnpUtil(stdout string) ([]Package, error) {
	var x pnpXML
	// The tool writes UTF-8; text read through another code page can leave
	// invalid bytes in device names, which the XML decoder refuses.
	if err := xml.Unmarshal([]byte(strings.ToValidUTF8(stdout, "?")), &x); err != nil {
		return nil, err
	}
	var out []Package
	for _, d := range x.Drivers {
		p := Package{Published: strings.TrimSpace(d.Name), Original: strings.TrimSpace(d.Original), Provider: strings.TrimSpace(d.Provider),
			Class: strings.TrimSpace(d.Class), ClassGUID: strings.ToLower(strings.TrimSpace(d.ClassGUID)), Devices: len(d.Devices)}
		date, ver, _ := strings.Cut(strings.TrimSpace(d.Version), " ")
		if t, err := time.Parse("01/02/2006", date); err == nil {
			p.Date = t
		}
		p.Version = strings.TrimSpace(ver)
		for _, dev := range d.Devices {
			if strings.EqualFold(strings.TrimSpace(dev.Status), "Started") {
				p.Started++
			}
		}
		if p.Published != "" {
			out = append(out, p)
		}
	}
	return out, nil
}

// ListPackages asks Windows for the driver packages. ok is false when the tool
// is missing or does not know /format (Windows before 11 22H2 or so): nothing
// is then said about old packages.
func ListPackages(ctx context.Context, p *sys.Platform) ([]Package, bool) {
	res, err := p.Run.Run(ctx, "pnputil.exe", []string{"/enum-drivers", "/devices", "/format", "xml"}, sys.RunOpts{Timeout: 60 * time.Second})
	if err != nil || res.ExitCode != 0 {
		return nil, false
	}
	pkgs, err := ParsePnpUtil(res.Stdout)
	if err != nil || len(pkgs) == 0 {
		return nil, false
	}
	return pkgs, true
}

// OldPackage is a display driver package that a newer one has replaced.
type OldPackage struct {
	Package
	// Newest is the version of the package that replaced it.
	Newest string `json:"newest"`
}

// OldDisplayPackages finds the display driver packages that stay in the driver
// store after an update: same original INF and provider, older than the
// newest, and installed on no device at all (even a stopped one keeps its package). Only display adapters are
// looked at: their packages are large and an old copy is pure waste, while the
// same INF name on other classes (firmware, extensions) often stands for
// different components, not for older versions. Nothing is removed here.
func OldDisplayPackages(pkgs []Package) []OldPackage {
	groups := map[string][]Package{}
	var keys []string
	for _, p := range pkgs {
		if p.ClassGUID != DisplayClassGUID || p.Original == "" {
			continue
		}
		k := strings.ToLower(p.Original) + "|" + strings.ToLower(p.Provider)
		if _, ok := groups[k]; !ok {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], p)
	}
	sort.Strings(keys)
	var out []OldPackage
	for _, k := range keys {
		g := groups[k]
		if len(g) < 2 {
			continue
		}
		sort.SliceStable(g, func(i, j int) bool { return newer(g[i], g[j]) })
		for _, p := range g[1:] {
			if p.Devices == 0 {
				out = append(out, OldPackage{Package: p, Newest: g[0].Version})
			}
		}
	}
	return out
}

// newer orders by version, then by date, then by the number of the published
// name (a later installation has a higher oemNN).
func newer(a, b Package) bool {
	if c := cmpVersion(a.Version, b.Version); c != 0 {
		return c > 0
	}
	if !a.Date.Equal(b.Date) {
		return a.Date.After(b.Date)
	}
	return oemNumber(a.Published) > oemNumber(b.Published)
}

// cmpVersion compares dotted numbers part by part (32.0.9 < 32.0.16).
func cmpVersion(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(strings.TrimSpace(pa[i]))
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(strings.TrimSpace(pb[i]))
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

func oemNumber(name string) int {
	n, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(name), "oem"), ".inf"))
	return n
}

var reOEMName = regexp.MustCompile(`(?i)^oem[0-9]+\.inf$`)

// RemovePackage deletes one driver package from the driver store
// ("pnputil /delete-driver oemNN.inf"). It never passes /uninstall or /force:
// without them pnputil itself refuses a package that a device still uses,
// which is the last safety net if the list the user confirmed is out of date.
func RemovePackage(ctx context.Context, p *sys.Platform, published string) error {
	if !reOEMName.MatchString(published) {
		return fmt.Errorf("nome de pacote inválido: %q", published)
	}
	res, err := p.Run.Run(ctx, "pnputil.exe", []string{"/delete-driver", published}, sys.RunOpts{Timeout: 3 * time.Minute})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		if msg := strings.TrimSpace(res.Stdout); msg != "" {
			return errors.New(msg)
		}
		return fmt.Errorf("o pnputil terminou com o código %d", res.ExitCode)
	}
	return nil
}

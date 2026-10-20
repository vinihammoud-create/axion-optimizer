// Package drivers holds the platform-independent logic of the Drivers page:
// the NVIDIA driver lookup, the driver inventory grouped by component and
// the evaluation of what Windows Update offers.
package drivers

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// NVIDIA's public driver search, the same service behind nvidia.com/drivers
// (used by open-source checkers such as TinyNvidiaUpdateChecker):
//   - lookupValueSearch.aspx?TypeID=3 lists every product with its product
//     family ID (pfid);
//   - AjaxDriverService DriverManualLookup returns the drivers of a pfid.
const (
	NvidiaProductsURL = "https://www.nvidia.com/Download/API/lookupValueSearch.aspx?TypeID=3"
	NvidiaLookupURL   = "https://gfwsl.geforce.com/services_toolkit/services/com/nvidia/services/AjaxDriverService.php"
	NvidiaSigner      = "NVIDIA Corporation"

	nvidiaOSWin11    = 135 // "Windows 11" in lookupValueSearch TypeID=4
	nvidiaOSWin10x64 = 57  // "Windows 10 64-bit"
)

// NvidiaOSID returns NVIDIA's operating system ID.
func NvidiaOSID(win11 bool) int {
	if win11 {
		return nvidiaOSWin11
	}
	return nvidiaOSWin10x64
}

type lookupValue struct {
	ParentID string `xml:"ParentID,attr"`
	Name     string `xml:"Name"`
	Value    string `xml:"Value"`
}

// ParseProducts reads the product list into lower-case name → pfid. Every
// <LookupValue> element is read, wherever it sits in the document.
func ParseProducts(b []byte) (map[string]int, error) {
	dec := xml.NewDecoder(strings.NewReader(string(b)))
	out := map[string]int{}
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "LookupValue" {
			continue
		}
		var v lookupValue
		if err := dec.DecodeElement(&v, &se); err != nil {
			return nil, fmt.Errorf("lista de produtos da NVIDIA: %w", err)
		}
		id, err := strconv.Atoi(strings.TrimSpace(v.Value))
		name := normName(v.Name)
		if err != nil || id <= 0 || name == "" {
			continue
		}
		if _, dup := out[name]; !dup {
			out[name] = id
		}
	}
	if len(out) == 0 {
		return nil, errors.New("lista de produtos da NVIDIA vazia ou em formato desconhecido")
	}
	return out, nil
}

var spaces = regexp.MustCompile(`\s+`)

func normName(s string) string {
	s = strings.ToLower(strings.TrimSpace(spaces.ReplaceAllString(s, " ")))
	return strings.TrimPrefix(s, "nvidia ")
}

// reMemSize matches a memory size Windows adds to some GPU names
// ("RTX 3050 6GB Laptop GPU", "GTX 1060 6GB") that NVIDIA's list leaves out.
var reMemSize = regexp.MustCompile(`\s\d{1,2}\s?gb\b`)

// ProductID finds the pfid of a GPU as Windows names it
// ("NVIDIA GeForce RTX 3060", "NVIDIA GeForce RTX 4060 Laptop GPU").
func ProductID(products map[string]int, gpuName string) (int, bool) {
	n := normName(gpuName)
	if id, ok := products[n]; ok {
		return id, true
	}
	// Some names carry a memory size or a vendor suffix Windows adds; the
	// first real report had "NVIDIA GeForce RTX 3050 6GB Laptop GPU", listed
	// by NVIDIA as "GeForce RTX 3050 Laptop GPU".
	try := func(k string) (int, bool) {
		id, ok := products[strings.TrimSpace(spaces.ReplaceAllString(k, " "))]
		return id, ok
	}
	cands := []string{n}
	if m := reMemSize.ReplaceAllString(n, ""); m != n {
		cands = append(cands, m)
	}
	for i, c := range cands {
		if i > 0 {
			if id, ok := try(c); ok {
				return id, true
			}
		}
		for _, trim := range []string{" (tm)", "(r)", " with max-q design"} {
			if id, ok := try(strings.ReplaceAll(c, trim, "")); ok {
				return id, true
			}
		}
	}
	return 0, false
}

// LookupQuery builds the DriverManualLookup URL for a product family.
func LookupQuery(base string, pfid, osID int) string {
	q := url.Values{}
	q.Set("func", "DriverManualLookup")
	q.Set("pfid", strconv.Itoa(pfid))
	q.Set("osID", strconv.Itoa(osID))
	q.Set("dch", "1")
	q.Set("numberOfResults", "10")
	q.Set("languageCode", "1033")
	return base + "?" + q.Encode()
}

// NvidiaDriver is one driver release returned by the lookup.
type NvidiaDriver struct {
	ID          string    `json:"id"`
	Version     string    `json:"version"`
	Name        string    `json:"name"`
	Released    time.Time `json:"released"`
	DownloadURL string    `json:"downloadUrl"`
	SizeLabel   string    `json:"sizeLabel"`
	DetailsURL  string    `json:"detailsUrl"`
	Beta        bool      `json:"beta"`
	WHQL        bool      `json:"whql"`
	Studio      bool      `json:"studio"`
}

type lookupResponse struct {
	Success json.RawMessage `json:"Success"`
	IDS     []struct {
		DownloadInfo map[string]json.RawMessage `json:"downloadInfo"`
	} `json:"IDS"`
}

func rawString(m map[string]json.RawMessage, k string) string {
	v, ok := m[k]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		if u, err := url.QueryUnescape(s); err == nil {
			s = u
		}
		return strings.TrimSpace(s)
	}
	var n json.Number
	if json.Unmarshal(v, &n) == nil {
		return n.String()
	}
	return ""
}

// ParseLookup decodes the DriverManualLookup answer.
func ParseLookup(b []byte) ([]NvidiaDriver, error) {
	var r lookupResponse
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("resposta da NVIDIA: %w", err)
	}
	var out []NvidiaDriver
	for _, e := range r.IDS {
		m := e.DownloadInfo
		d := NvidiaDriver{
			ID: rawString(m, "ID"), Version: rawString(m, "Version"), Name: rawString(m, "Name"),
			DownloadURL: rawString(m, "DownloadURL"), SizeLabel: rawString(m, "DownloadURLFileSize"), DetailsURL: rawString(m, "DetailsURL"),
			Beta: rawString(m, "IsBeta") == "1", WHQL: rawString(m, "IsWHQL") == "1", Studio: rawString(m, "IsCRD") == "1",
		}
		if t, err := time.Parse("Mon Jan 2, 2006", rawString(m, "ReleaseDateTime")); err == nil {
			d.Released = t
		}
		if d.Version == "" || d.DownloadURL == "" {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

// PickGameReady returns the newest Game Ready driver that is not a beta,
// preferring WHQL-certified releases.
func PickGameReady(list []NvidiaDriver) *NvidiaDriver {
	var best *NvidiaDriver
	for i := range list {
		d := &list[i]
		if d.Studio || d.Beta {
			continue
		}
		switch {
		case best == nil:
			best = d
		case d.WHQL && !best.WHQL:
			best = d
		case d.WHQL == best.WHQL && CompareVersions(d.Version, best.Version) > 0:
			best = d
		}
	}
	return best
}

// CompareVersions compares dotted numeric versions ("617.14" vs "616.92").
func CompareVersions(a, b string) int {
	pa, pb := strings.Split(strings.TrimSpace(a), "."), strings.Split(strings.TrimSpace(b), ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// AllowedDownloadHost limits NVIDIA downloads to NVIDIA's download servers.
// It is a variable only so tests can point it at a local server.
var AllowedDownloadHost = func(host string) bool {
	host = strings.ToLower(host)
	return host == "download.nvidia.com" || strings.HasSuffix(host, ".download.nvidia.com")
}

// AllowedScheme is "https" (tests may use "http" with a local server).
var AllowedScheme = "https"

var safeFile = regexp.MustCompile(`^[A-Za-z0-9._-]+\.exe$`)

// DownloadTarget validates an NVIDIA download URL and returns a safe file name.
func DownloadTarget(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("endereço de download inválido: %w", err)
	}
	if u.Scheme != AllowedScheme || !AllowedDownloadHost(u.Hostname()) || u.User != nil {
		return "", fmt.Errorf("endereço de download fora dos servidores da NVIDIA: %s", u.Host)
	}
	name := path.Base(u.Path)
	if !safeFile.MatchString(name) {
		return "", fmt.Errorf("nome de arquivo inesperado: %s", name)
	}
	return name, nil
}

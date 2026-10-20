package mocksys

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"axion/internal/sys"
)

// Updates is a fake Windows Update Agent with a few driver offers.
type Updates struct {
	mu       sync.Mutex
	Offers   []sys.DriverUpdate
	Fail     map[string]int32 // UpdateID → HRESULT for downloads/installs that fail
	SearchFn func() error     // optional: error from SearchDrivers
	Searches int
	Log      []string
}

// NewUpdates returns the default offers of the simulated PC.
func NewUpdates() *Updates {
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	return &Updates{Fail: map[string]int32{}, Offers: []sys.DriverUpdate{
		{UpdateID: "0c3f1e6a-6f0b-4b7e-9a51-2f7d3c1b8e01", Revision: 1, Title: "Realtek Semiconductor Corp. - Net - 1125.21.903.2025",
			Class: "Net", HardwareID: `PCI\VEN_10EC&DEV_8125&SUBSYS_E0001458&REV_05`, Manufacturer: "Realtek", Model: "Realtek PCIe 2.5GbE Family Controller",
			Provider: "Realtek", VerDate: day(2025, 9, 3), SizeBytes: 1_310_720, BrowseOnly: true, RebootBehavior: 2},
		{UpdateID: "5a2d9b77-1c44-4f0e-8f7a-9b0e1d2c3f02", Revision: 1, Title: "Advanced Micro Devices, Inc. - System - 7.6.2.123",
			Class: "System", HardwareID: `PCI\VEN_1022&DEV_790B&SUBSYS_50011458`, Manufacturer: "Advanced Micro Devices, Inc", Model: "AMD SMBus",
			Provider: "Advanced Micro Devices, Inc.", VerDate: day(2025, 6, 10), SizeBytes: 409_600, BrowseOnly: true},
		{UpdateID: "9e8f7a6b-5c4d-4e3f-8a2b-1c0d9e8f7a03", Revision: 2, Title: "NVIDIA - Display - 32.0.15.6094",
			Class: "Display", HardwareID: `PCI\VEN_10DE&DEV_2504&SUBSYS_397D1462`, Manufacturer: "NVIDIA", Model: "NVIDIA GeForce RTX 3060",
			Provider: "NVIDIA", VerDate: day(2024, 8, 14), SizeBytes: 812_000_000, BrowseOnly: true, RebootBehavior: 2},
		{UpdateID: "3b1a0c9d-8e7f-4a6b-9c5d-4e3f2a1b0c04", Revision: 1, Title: "Intel Corporation - Bluetooth - 23.100.0.3",
			Class: "Bluetooth", HardwareID: `USB\VID_8087&PID_0032`, Manufacturer: "Intel Corporation", Model: "Intel(R) Wireless Bluetooth(R)",
			Provider: "Intel", VerDate: day(2025, 2, 20), SizeBytes: 15_728_640, BrowseOnly: true, Hidden: true},
	}}
}

func (u *Updates) find(id string) int {
	for i := range u.Offers {
		if strings.EqualFold(u.Offers[i].UpdateID, id) {
			return i
		}
	}
	return -1
}

// SearchDrivers implements sys.Updates.
func (u *Updates) SearchDrivers(ctx context.Context) ([]sys.DriverUpdate, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.Searches++
	if u.SearchFn != nil {
		if err := u.SearchFn(); err != nil {
			return nil, err
		}
	}
	return append([]sys.DriverUpdate(nil), u.Offers...), nil
}

// DownloadDrivers implements sys.Updates.
func (u *Updates) DownloadDrivers(ctx context.Context, ids []string) ([]sys.UpdateResult, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	var out []sys.UpdateResult
	for _, id := range ids {
		i := u.find(id)
		if i < 0 {
			continue
		}
		r := sys.UpdateResult{UpdateID: u.Offers[i].UpdateID, Title: u.Offers[i].Title, Code: sys.WUSucceeded}
		if hr, bad := u.Fail[u.Offers[i].UpdateID]; bad {
			r.Code, r.HResult = sys.WUFailed, hr
		} else {
			u.Offers[i].Downloaded = true
		}
		u.Log = append(u.Log, "download "+u.Offers[i].UpdateID)
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, errors.New("as atualizações escolhidas não são mais oferecidas pelo Windows Update; procure de novo")
	}
	return out, nil
}

// InstallDrivers implements sys.Updates.
func (u *Updates) InstallDrivers(ctx context.Context, ids []string) ([]sys.UpdateResult, bool, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	var out []sys.UpdateResult
	reboot := false
	var keep []sys.DriverUpdate
	installed := map[string]bool{}
	for _, id := range ids {
		i := u.find(id)
		if i < 0 {
			continue
		}
		o := u.Offers[i]
		if !o.Downloaded {
			return nil, false, errors.New("\"" + o.Title + "\" ainda não foi baixada")
		}
		r := sys.UpdateResult{UpdateID: o.UpdateID, Title: o.Title, Code: sys.WUSucceeded, RebootRequired: o.RebootBehavior == 1}
		if hr, bad := u.Fail[o.UpdateID]; bad {
			r.Code, r.HResult = sys.WUFailed, hr
		} else {
			installed[o.UpdateID] = true
			reboot = reboot || r.RebootRequired
		}
		u.Log = append(u.Log, "install "+o.UpdateID)
		out = append(out, r)
	}
	for _, o := range u.Offers {
		if !installed[o.UpdateID] {
			keep = append(keep, o)
		}
	}
	u.Offers = keep
	if len(out) == 0 {
		return nil, false, errors.New("as atualizações escolhidas não são mais oferecidas pelo Windows Update; procure de novo")
	}
	return out, reboot, nil
}

// SetHidden implements sys.Updates.
func (u *Updates) SetHidden(ctx context.Context, id string, hidden bool) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	i := u.find(id)
	if i < 0 {
		return errors.New("atualização não encontrada")
	}
	u.Offers[i].Hidden = hidden
	u.Log = append(u.Log, "hide "+id)
	return nil
}

// VerifySignature implements sys.Native: files whose content starts with
// "MZ-SIGNED:<signer>\n" are trusted and signed by <signer>; while
// RevocationOffline is set, the check fails as if the certificate servers
// were unreachable.
func (n *Native) VerifySignature(path string) (sys.Signature, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return sys.Signature{}, err
	}
	const tag = "MZ-SIGNED:"
	s := string(b)
	if !strings.HasPrefix(s, tag) {
		return sys.Signature{Error: "arquivo sem assinatura digital"}, nil
	}
	signer, _, _ := strings.Cut(s[len(tag):], "\n")
	n.mu.Lock()
	offline := n.RevocationOffline
	n.mu.Unlock()
	if offline {
		return sys.Signature{Signer: signer, RevocationOffline: true, Error: "não foi possível checar a revogação do certificado"}, nil
	}
	return sys.Signature{Trusted: true, Signer: signer}, nil
}

// StartProgram implements sys.Native.
func (n *Native) StartProgram(path, args string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	n.mu.Lock()
	n.Started = append(n.Started, path)
	n.mu.Unlock()
	return nil
}

var _ sys.Updates = (*Updates)(nil)

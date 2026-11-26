package catalog_test

import (
	"context"
	"testing"

	"axion/internal/collect"
	"axion/internal/sys"
	"axion/internal/sys/mocksys"
)

// Items the default simulated PC does not qualify for: each one gets the PC it
// needs, then goes through the same apply/verify/revert comparison of the whole
// simulated machine as every other item.
func TestRoundTripPreparedTweaks(t *testing.T) {
	disk := func(n int, name string, media int64) sys.WMIRow {
		return sys.WMIRow{"DeviceId": string(rune('0' + n)), "ObjectId": mocksys.MockDiskObjectID(n), "FriendlyName": name, "MediaType": media, "BusType": int64(11), "Size": uint64(1000204886016), "HealthStatus": int64(0)}
	}
	cases := []struct {
		id      string
		prepare func(m *mocksys.Mock)
	}{
		{"adv.vmp.off", func(m *mocksys.Mock) {
			m.W.Set(`root\cimv2`, "Win32_OptionalFeature", sys.WMIRow{"Name": "VirtualMachinePlatform", "InstallState": int64(1)})
		}},
		{"services.sysmain.hdd", func(m *mocksys.Mock) {
			m.W.Set(`root\Microsoft\Windows\Storage`, "MSFT_PhysicalDisk", disk(0, "WD Blue 1TB", 3), disk(1, "Crucial MX500 1TB", 4))
		}},
		{"adv.spectre", func(m *mocksys.Mock) {
			m.W.Set(`root\cimv2`, "Win32_Processor", sys.WMIRow{"Name": "Intel(R) Core(TM) i7-4790K CPU @ 4.00GHz", "Manufacturer": "GenuineIntel", "NumberOfCores": int64(4), "NumberOfLogicalProcessors": int64(8), "MaxClockSpeed": int64(4000)})
		}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.id, func(t *testing.T) {
			m, _, e := setup(t)
			c.prepare(m)
			f := collect.Collect(context.Background(), m.Platform, nil)
			tw := e.Catalog.Get(c.id)
			if tw == nil {
				t.Fatalf("otimização %s não existe", c.id)
			}
			if ok, why := tw.IsApplicable(f); !ok {
				t.Fatalf("o PC simulado preparado deveria qualificar para %s: %s", c.id, why)
			}
			roundTrip(t, m, f, e, tw)
		})
	}
}

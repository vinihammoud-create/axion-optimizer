package diag

import (
	"testing"

	"axion/internal/facts"
	"axion/internal/profile"
)

// 0.8.1 (report of the G15): the SSD reading cites the Visual Studio page
// only in the programming profile, where that page is the requirement.
func TestCapacityDiskSource(t *testing.T) {
	f := &facts.Facts{Disks: []facts.Disk{{Model: "ADATA 512GB", MediaType: "SSD", BusType: "NVMe", IsSystem: true}}}
	for _, tc := range []struct {
		prof string
		want int
	}{{profile.Competitive, 0}, {profile.Work, 0}, {profile.Dev, 1}} {
		for _, x := range Capacity(f, tc.prof) {
			if x.ID == "cap.disk" && len(x.Sources) != tc.want {
				t.Errorf("%s: %d fontes no cap.disk (%v)", tc.prof, len(x.Sources), x.Sources)
			}
		}
	}
}

// With nothing measured, the measurement reading has no score.
func TestUnscore(t *testing.T) {
	r := Score(nil)
	if r.Overall != 100 {
		t.Fatalf("relatório vazio: %d", r.Overall)
	}
	r.Unscore("nada medido")
	if !r.Unscored || r.Overall != -1 || r.Grade != "Sem medição" {
		t.Errorf("%+v", r)
	}
}

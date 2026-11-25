package diag

import (
	"strings"
	"testing"

	"axion/internal/facts"
	"axion/internal/profile"
)

func capFindings(f *facts.Facts, prof, id string) []Finding {
	var out []Finding
	for _, x := range Capacity(f, prof) {
		if x.ID == id {
			out = append(out, x)
		}
	}
	return out
}

func TestCapacityGPUVRAMUnreadIsNotGood(t *testing.T) {
	f := &facts.Facts{GPUs: []facts.GPUInfo{{Name: "GeForce RTX 3060", Vendor: "NVIDIA", VRAMMiB: 0}}}
	for _, prof := range []string{profile.Competitive, profile.Creation} {
		for _, x := range capFindings(f, prof, "cap.gpu") {
			if x.Severity == SevGood || x.Severity == SevWarning || x.Severity == SevCritical {
				t.Errorf("%s: VRAM não lida virou %v: %q", prof, x.Severity, x.Title)
			}
		}
	}
}

func TestCapacityNoGPUReadIsNotAlarm(t *testing.T) {
	f := &facts.Facts{}
	for _, prof := range []string{profile.Competitive, profile.Creation} {
		for _, x := range capFindings(f, prof, "cap.gpu") {
			if x.Severity == SevGood || x.Severity == SevWarning || x.Severity == SevCritical {
				t.Errorf("%s: GPU não lida virou %v: %q", prof, x.Severity, x.Title)
			}
			if txt := strings.ToLower(x.Title + x.Evidence + x.Explanation); strings.Contains(txt, "driver") || strings.Contains(txt, "básico") {
				t.Errorf("%s: GPU não lida cita driver: %q", prof, x.Title)
			}
		}
	}
}

func TestCapacityBasicDriverStillCritical(t *testing.T) {
	f := &facts.Facts{GPUs: []facts.GPUInfo{{Name: "Microsoft Basic Display Adapter", BasicDriver: true}}}
	for _, prof := range []string{profile.Competitive, profile.Creation} {
		got := capFindings(f, prof, "cap.gpu")
		if len(got) != 1 || got[0].Severity != SevCritical {
			t.Errorf("%s: driver básico lido deve seguir crítico: %+v", prof, got)
		}
	}
}

func TestCapacityCoresUnreadIsNotGood(t *testing.T) {
	f := &facts.Facts{CPU: facts.CPUInfo{Name: "CPU X", Cores: 0, Threads: 8}}
	for _, prof := range []string{profile.Competitive, profile.Creation, profile.Dev, profile.Work} {
		for _, x := range capFindings(f, prof, "cap.cpu") {
			if txt := x.Title + " " + x.Evidence; strings.Contains(txt, "0 núcleos") {
				t.Errorf("%s: cap.cpu diz 0 núcleos: %q / %q", prof, x.Title, x.Evidence)
			}
		}
	}
	for _, x := range capFindings(f, profile.Work, "cap.cpu") {
		t.Errorf("work: núcleos não lidos geraram achado %v: %q", x.Severity, x.Title)
	}
}

package mocksys

import (
	"os"
	"path/filepath"
	"strings"
)

// cs2SampleKV is cs2_video.txt as CS2 writes it, in the order of a real
// install, with the values of the demo profile (mockCS2 in internal/app).
var cs2SampleKV = [][2]string{
	{"Version", "16"}, {"VendorID", "4318"}, {"DeviceID", "9708"},
	{"setting.cpu_level", "3"}, {"setting.gpu_mem_level", "3"}, {"setting.gpu_level", "3"}, {"setting.knowndevice", "0"},
	{"setting.defaultres", "1280"}, {"setting.defaultresheight", "960"},
	{"setting.refreshrate_numerator", "60"}, {"setting.refreshrate_denominator", "1"},
	{"setting.fullscreen", "1"}, {"setting.coop_fullscreen", "0"}, {"setting.nowindowborder", "0"},
	{"setting.mat_vsync", "0"}, {"setting.fullscreen_min_on_focus_loss", "1"}, {"setting.high_dpi", "0"},
	{"Autoconfig", "2"}, {"setting.shaderquality", "0"}, {"setting.r_texturefilteringquality", "0"},
	{"setting.msaa_samples", "2"}, {"setting.r_csgo_cmaa_enable", "0"},
	{"setting.videocfg_shadow_quality", "2"}, {"setting.videocfg_dynamic_shadows", "1"},
	{"setting.videocfg_texture_detail", "0"}, {"setting.videocfg_particle_detail", "0"},
	{"setting.videocfg_ao_detail", "0"}, {"setting.videocfg_hdr_detail", "-1"}, {"setting.videocfg_fsr_detail", "0"},
	{"setting.monitor_index", "0"}, {"setting.r_low_latency", "0"}, {"setting.aspectratiomode", "0"},
}

// CS2VideoSample is cs2_video.txt as CS2 writes it (CRLF, one TAB before the
// key and two between key and value, no BOM), with the values of the demo
// profile.
var CS2VideoSample = func() string {
	var sb strings.Builder
	sb.WriteString("\"video.cfg\"\r\n{\r\n")
	for _, p := range cs2SampleKV {
		sb.WriteString("\t\"" + p[0] + "\"\t\t\"" + p[1] + "\"\r\n")
	}
	sb.WriteString("}\r\n")
	return sb.String()
}()

// CS2VideoPath is where the simulated Steam keeps it: outside mockfs, so the
// leftovers scan of Program Files never sees a fake Steam folder.
func CS2VideoPath(dataDir string) string {
	return filepath.Join(dataDir, "cs2sim", "Steam", "userdata", "123", "730", "local", "cfg", "cs2_video.txt")
}

// SeedCS2Video writes the file and watches its folder in World(), so a
// changed byte or a forgotten temporary file shows in DiffWorld.
func (m *Mock) SeedCS2Video(content string) (string, error) {
	p := CS2VideoPath(m.DataDir)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return "", err
	}
	m.WatchDir(filepath.Dir(p))
	return p, nil
}

// WatchDir makes World() list the regular files of dir (name -> SHA-256).
func (m *Mock) WatchDir(dir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.watch {
		if d == dir {
			return
		}
	}
	m.watch = append(m.watch, dir)
}

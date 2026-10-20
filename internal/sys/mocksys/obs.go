package mocksys

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"axion/internal/sys"
)

// MockOBSLog is the log of a stream of two hours and ten minutes on the
// simulated PC: NVENC at 8 Mb/s, 1440p composed down to 1080p at 60 fps. The
// card was a little late composing frames (1,8%) and the connection dropped
// 3,1% of them; the encoder kept up. The lines follow the messages in OBS'
// source (libobs/obs.c, libobs/obs-output.c, libobs-d3d11) and the logs users
// post on the OBS forum.
func MockOBSLog(start time.Time) string {
	at := func(d time.Duration) string { return start.Add(d).Format("15:04:05.000") }
	var b strings.Builder
	w := func(d time.Duration, f string, a ...any) { fmt.Fprintf(&b, "%s: %s\n", at(d), fmt.Sprintf(f, a...)) }
	w(0, "OBS 30.2.3 (64-bit, windows)")
	w(0, "CPU Name: AMD Ryzen 7 7800X3D 8-Core Processor")
	w(time.Second, "Loading up D3D11 on adapter NVIDIA GeForce RTX 3060 (0)")
	w(2*time.Second, "video settings reset:")
	w(2*time.Second, "\tbase resolution:   2560x1440")
	w(2*time.Second, "\toutput resolution: 1920x1080")
	w(2*time.Second, "\tdownscale filter:  Bicubic")
	w(2*time.Second, "\tfps:               60/1")
	w(10*time.Minute, "==== Streaming Start ===============================================")
	w(10*time.Minute+time.Second, "[NVENC encoder: 'streaming_h264'] settings:")
	w(10*time.Minute+time.Second, "\trate_control: CBR")
	w(10*time.Minute+time.Second, "\tbitrate:      8000")
	w(10*time.Minute+time.Second, "\tkeyint:       120")
	stop := 2*time.Hour + 10*time.Minute
	w(stop, "==== Streaming Stop ================================================")
	w(stop+100*time.Millisecond, "Output 'adv_stream': stopping")
	w(stop+200*time.Millisecond, "Output 'adv_stream': Total frames output: 433800 (447600 attempted)")
	w(stop+200*time.Millisecond, "Output 'adv_stream': Total drawn frames: 439200 (447600 attempted)")
	w(stop+200*time.Millisecond, "Output 'adv_stream': Number of lagged frames due to rendering lag/stalls: 8100 (1.8%%)")
	w(stop+200*time.Millisecond, "Output 'adv_stream': Number of dropped frames due to insufficient bandwidth/connection stalls: 13800 (3.1%%)")
	w(stop+300*time.Millisecond, "Video stopped, number of skipped frames due to encoding lag: 870/447600 (0.2%%)")
	return b.String()
}

// seedOBS writes a log of OBS Studio into the simulated profile.
func seedOBS(p sys.Paths) {
	dir := filepath.Join(p.AppData, "obs-studio", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	start := time.Now().Add(-26 * time.Hour)
	_ = os.WriteFile(filepath.Join(dir, start.Format("2006-01-02 15-04-05")+".txt"), []byte(MockOBSLog(start)), 0o644)
}

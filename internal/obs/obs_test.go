package obs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var logStart = time.Date(2026, 10, 5, 20, 0, 0, 0, time.Local)

// The end of a stream exactly as a user posted it on the OBS forum (thread
// "Dropped Frames (Network)"), plus the lines OBS prints before it.
const forumExcerpt = `20:27:54.013: Output 'rtmp multitrack video': Total frames output: 37446 (107334 attempted)
20:27:54.013: Output 'rtmp multitrack video': Total drawn frames: 57716 (57828 attempted)
20:27:54.013: Output 'rtmp multitrack video': Number of lagged frames due to rendering lag/stalls: 112 (0.2%)
20:27:54.013: Output 'rtmp multitrack video': Number of dropped frames due to insufficient bandwidth/connection stalls: 69888 (65.1%)
20:27:54.030: [rtmp stream: 'rtmp multitrack video'] Freeing 59229 remaining packets
20:27:54.042: Video stopped, number of skipped frames due to encoding lag: 20074/50845 (39.5%)
`

func TestParseTheForumExcerpt(t *testing.T) {
	ss := ParseLog(strings.NewReader(forumExcerpt), "2026-10-05 20-00-00.txt", logStart)
	if len(ss) != 1 || len(ss[0].Stats) != 1 {
		t.Fatalf("sessões: %+v", ss)
	}
	s := ss[0].Stats[0]
	if s.Output != "rtmp multitrack video" || s.Kind != KindStream || s.Frames != 37446 || s.Attempted != 107334 || s.Drawn != 57716 ||
		s.Lagged != 112 || s.LaggedPct != 0.2 || s.Dropped != 69888 || s.DropPct != 65.1 || s.Skipped != 20074 || s.SkippedOf != 50845 || s.SkippedPct != 39.5 {
		t.Errorf("%+v", s)
	}
	if got := ss[0].At; got.Hour() != 20 || got.Minute() != 27 || got.Second() != 54 {
		t.Errorf("horário da parada: %v", got)
	}
	if ss[0].Stream() == nil || ss[0].Stream().Output != "rtmp multitrack video" {
		t.Error("Stream()")
	}
}

// A complete log of a stream with the header, the encoder and the video settings.
func sampleLog(bitrate int, skipped, lagged, dropped string, stamped bool) string {
	cont := func(ts, s string) string {
		if stamped {
			return ts + ": " + s
		}
		return s
	}
	var b strings.Builder
	w := func(s string) { b.WriteString(s + "\n") }
	w("20:00:01.100: OBS 30.2.3 (64-bit, windows)")
	w("20:00:01.200: CPU Name: AMD Ryzen 7 7800X3D 8-Core Processor")
	w("20:00:02.300: Loading up D3D11 on adapter NVIDIA GeForce RTX 3060 (0)")
	w("20:00:03.400: video settings reset:")
	w(cont("20:00:03.400", "\tbase resolution:   2560x1440"))
	w(cont("20:00:03.400", "\toutput resolution: 1920x1080"))
	w(cont("20:00:03.400", "\tdownscale filter:  Bicubic"))
	w(cont("20:00:03.400", "\tfps:               60/1"))
	w("20:10:00.000: ==== Streaming Start ===============================================")
	w("20:10:01.000: [NVENC encoder: 'streaming_h264'] settings:")
	w(cont("20:10:01.000", "\trate_control: CBR"))
	w(cont("20:10:01.000", fmt.Sprintf("\tbitrate:      %d", bitrate)))
	w(cont("20:10:01.000", "\tkeyint:       120"))
	w("21:10:00.000: ==== Streaming Stop ================================================")
	w("21:10:00.100: Output 'adv_stream': stopping")
	w("21:10:00.200: Output 'adv_stream': Total frames output: 215000 (216000 attempted)")
	w("21:10:00.200: Output 'adv_stream': Total drawn frames: 215900")
	w("21:10:00.200: Output 'adv_stream': Number of lagged frames due to rendering lag/stalls: " + lagged)
	w("21:10:00.200: Output 'adv_stream': Number of dropped frames due to insufficient bandwidth/connection stalls: " + dropped)
	w("21:10:00.300: Video stopped, number of skipped frames due to encoding lag: " + skipped)
	return b.String()
}

func TestParseCompleteLogWithContext(t *testing.T) {
	for _, stamped := range []bool{true, false} {
		ss := ParseLog(strings.NewReader(sampleLog(6000, "130/216000 (0.1%)", "3 (0.0%)", "1000 (0.5%)", stamped)), "2026-10-05 20-00-00.txt", logStart)
		if len(ss) != 1 {
			t.Fatalf("stamped=%v: %+v", stamped, ss)
		}
		c := ss[0].Context
		if c.GPU != "NVIDIA GeForce RTX 3060" || c.Encoder != "NVENC" || c.RateControl != "CBR" || c.BitrateKbps != 6000 ||
			c.BaseRes != "2560x1440" || c.OutputRes != "1920x1080" || c.FPS != 60 || c.OBSVersion != "30.2.3" {
			t.Errorf("stamped=%v contexto: %+v", stamped, c)
		}
		if ss[0].Seconds < 3590 || ss[0].Seconds > 3610 { // from "Streaming Start" to the stop
			t.Errorf("duração: %v", ss[0].Seconds)
		}
		st := ss[0].Stats[0]
		if st.Frames != 215000 || st.Attempted != 216000 || st.Drawn != 215900 || st.Dropped != 1000 || st.DropPct != 0.5 || st.Skipped != 130 || st.SkippedPct != 0.1 {
			t.Errorf("stamped=%v stat: %+v", stamped, st)
		}
	}
}

func TestParseX264AndOtherBlocks(t *testing.T) {
	log := `20:00:01.000: [x264 encoder: 'streaming_h264'] preset: veryfast
20:00:01.000: [x264 encoder: 'streaming_h264'] settings:
20:00:01.000: 	rate_control: CBR
20:00:01.000: 	bitrate:      4500
20:00:01.000: 	buffer size:  4500
20:00:02.000: [some plugin] settings:
20:00:02.000: 	color: red
20:00:30.000: Output 'simple_stream': Total frames output: 100
20:00:30.000: Video stopped, number of skipped frames due to encoding lag: 5/100 (5.0%)
`
	ss := ParseLog(strings.NewReader(log), "x", logStart)
	if len(ss) != 1 {
		t.Fatalf("%+v", ss)
	}
	if c := ss[0].Context; c.Encoder != "x264" || c.BitrateKbps != 4500 || c.RateControl != "CBR" {
		t.Errorf("x264: %+v", c)
	}
	// A "settings:" block of another plugin does not replace the encoder's.
	if st := ss[0].Stats[0]; st.Frames != 100 || st.Attempted != 100 || st.Skipped != 5 || st.Lagged != -1 || st.Dropped != -1 {
		t.Errorf("campos não informados ficam em -1: %+v", st)
	}
}

func TestStreamAndRecordingTogether(t *testing.T) {
	log := `20:00:30.000: Output 'adv_stream': Total frames output: 1000
20:00:30.000: Output 'adv_stream': Number of dropped frames due to insufficient bandwidth/connection stalls: 10 (1.0%)
20:00:30.100: Output 'adv_file_output': Total frames output: 1000
20:00:30.100: Output 'adv_file_output': Number of lagged frames due to rendering lag/stalls: 40 (4.0%)
20:00:30.200: Video stopped, number of skipped frames due to encoding lag: 3/1000 (0.3%)
`
	ss := ParseLog(strings.NewReader(log), "x", logStart)
	if len(ss) != 1 || len(ss[0].Stats) != 2 {
		t.Fatalf("%+v", ss)
	}
	by := map[string]Stat{}
	for _, s := range ss[0].Stats {
		by[s.Output] = s
	}
	if by["adv_stream"].Kind != KindStream || by["adv_file_output"].Kind != KindRecording || by["adv_stream"].DropPct != 1 || by["adv_file_output"].LaggedPct != 4 {
		t.Errorf("%+v", by)
	}
	// The encoder's count is video-wide: both outputs get it.
	if by["adv_stream"].Skipped != 3 || by["adv_file_output"].Skipped != 3 {
		t.Errorf("quadros pulados: %+v", by)
	}
}

func TestSeveralSessionsMidnightAndACrash(t *testing.T) {
	log := `23:50:00.000: Output 'simple_stream': Total frames output: 500
23:50:00.100: Video stopped, number of skipped frames due to encoding lag: 0/500 (0.0%)
00:15:00.000: Output 'simple_stream': Total frames output: 900
00:15:00.100: Video stopped, number of skipped frames due to encoding lag: 9/900 (1.0%)
00:40:00.000: Output 'simple_stream': Total frames output: 300
00:40:00.000: Output 'simple_stream': Number of dropped frames due to insufficient bandwidth/connection stalls: 30 (10.0%)
`
	start := time.Date(2026, 10, 5, 23, 45, 0, 0, time.Local)
	ss := ParseLog(strings.NewReader(log), "x", start)
	if len(ss) != 3 {
		t.Fatalf("sessões: %d", len(ss))
	}
	if ss[1].At.Day() != 6 || ss[1].At.Hour() != 0 || ss[1].At.Minute() != 15 || ss[0].At.Day() != 5 {
		t.Errorf("a meia-noite: %v e %v", ss[0].At, ss[1].At)
	}
	// The last one ended without "Video stopped": kept, with the encoder count unknown.
	if last := ss[2].Stats[0]; last.Skipped != -1 || last.SkippedPct != -1 || last.DropPct != 10 || ss[2].At.Day() != 6 {
		t.Errorf("log sem a parada do vídeo: %+v at=%v", last, ss[2].At)
	}
}

func TestParseIgnoresNoiseAndCommaDecimals(t *testing.T) {
	log := "garbage line\n" + strings.Repeat("20:00:00.000: nothing here\n", 50) +
		"20:00:30.000: Output 'adv_stream': Number of lagged frames due to rendering lag/stalls: 5 (0,5%)\n" +
		"20:00:30.100: Video stopped, number of skipped frames due to encoding lag: 1/100 (1,0%)\n"
	ss := ParseLog(strings.NewReader(log), "x", logStart)
	if len(ss) != 1 || ss[0].Stats[0].LaggedPct != 0.5 || ss[0].Stats[0].SkippedPct != 1 {
		t.Errorf("%+v", ss)
	}
	if len(ParseLog(strings.NewReader(""), "x", logStart)) != 0 || len(ParseLog(strings.NewReader("Video stopped, number of skipped frames due to encoding lag: 1/2 (50.0%)\n"), "x", logStart)) != 0 {
		t.Error("sem nenhuma saída não há sessão")
	}
}

func TestKindOf(t *testing.T) {
	for name, want := range map[string]string{
		"adv_stream": KindStream, "simple_stream": KindStream, "rtmp multitrack video": KindStream,
		"adv_file_output": KindRecording, "simple_file_output": KindRecording,
		"simple_replay_buffer": KindReplay, "adv_replay_buffer": KindReplay, "virtualcam_output": KindOther,
	} {
		if got := kindOf(name); got != want {
			t.Errorf("%s: %s", name, got)
		}
	}
}

func TestReadFolder(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("2026-10-05 20-00-00.txt", sampleLog(6000, "10/1000 (1.0%)", "0 (0.0%)", "0 (0.0%)", true))
	write("2026-10-04 18-00-00.txt", sampleLog(4000, "0/1000 (0.0%)", "0 (0.0%)", "0 (0.0%)", true))
	write("2026-07-01 18-00-00.txt", sampleLog(4000, "0/1000 (0.0%)", "0 (0.0%)", "0 (0.0%)", true)) // too old
	write("notes.txt", "not a log")
	write("2026-10-03 10-00-00.log", "wrong extension")
	if err := os.Mkdir(filepath.Join(dir, "2026-10-02 10-00-00.txt"), 0o755); err != nil { // a folder
		t.Fatal(err)
	}
	ss, found := Read(dir, now)
	if !found || len(ss) != 2 || !ss[0].At.After(ss[1].At) || ss[0].Log != "2026-10-05 20-00-00.txt" || ss[0].Stats[0].Skipped != 10 {
		t.Fatalf("sessões: %v %+v", found, ss)
	}
	if _, found := Read(filepath.Join(dir, "does-not-exist"), now); found {
		t.Error("pasta inexistente: OBS nunca foi usado")
	}
	if ss, found := Read(t.TempDir(), now); !found || len(ss) != 0 {
		t.Error("pasta vazia existe mas sem sessões")
	}
	// Only the newest logs are read.
	many := t.TempDir()
	for i := 0; i < MaxLogs+5; i++ {
		d := time.Date(2026, 10, 1, 10, 0, 0, 0, time.Local).Add(time.Duration(i) * time.Hour)
		name := d.Format("2006-01-02 15-04-05") + ".txt"
		if err := os.WriteFile(filepath.Join(many, name), []byte(forumExcerpt), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if ss, _ := Read(many, now); len(ss) != MaxLogs {
		t.Errorf("logs lidos: %d, esperado %d", len(ss), MaxLogs)
	}
}

func finding(a Analysis, id string) *Finding {
	for i := range a.Findings {
		if a.Findings[i].ID == id {
			return &a.Findings[i]
		}
	}
	return nil
}

func analyzeLog(bitrate int, skipped, lagged, dropped string, upload float64) Analysis {
	ss := ParseLog(strings.NewReader(sampleLog(bitrate, skipped, lagged, dropped, true)), "x", logStart)
	return Analyze(Pick(ss), upload)
}

func TestAnalyzeAFineStream(t *testing.T) {
	a := analyzeLog(6000, "100/216000 (0.0%)", "50 (0.0%)", "100 (0.0%)", 40)
	if a.Level != LevelOK || len(a.Findings) != 1 || a.Findings[0].ID != "ok" || !strings.Contains(a.Findings[0].Text, "Nada relevante") {
		t.Errorf("%+v", a)
	}
}

func TestAnalyzeThresholds(t *testing.T) {
	for _, c := range []struct {
		skipped, lagged, dropped string
		id, level                string
	}{
		{"500/100000 (0.4%)", "0 (0.0%)", "0 (0.0%)", "encoding", ""}, // below 0,5%
		{"500/100000 (0.5%)", "0 (0.0%)", "0 (0.0%)", "encoding", LevelAttention},
		{"2000/100000 (2.0%)", "0 (0.0%)", "0 (0.0%)", "encoding", LevelImportant},
		{"0/100000 (0.0%)", "900 (0.9%)", "0 (0.0%)", "rendering", ""},
		{"0/100000 (0.0%)", "1000 (1.0%)", "0 (0.0%)", "rendering", LevelAttention},
		{"0/100000 (0.0%)", "5000 (5.0%)", "0 (0.0%)", "rendering", LevelImportant},
		{"0/100000 (0.0%)", "0 (0.0%)", "400 (0.4%)", "network", ""},
		{"0/100000 (0.0%)", "0 (0.0%)", "500 (0.5%)", "network", LevelAttention},
		{"0/100000 (0.0%)", "0 (0.0%)", "2000 (2.0%)", "network", LevelImportant},
	} {
		a := analyzeLog(6000, c.skipped, c.lagged, c.dropped, 40)
		f := finding(a, c.id)
		switch {
		case c.level == "" && f != nil:
			t.Errorf("%s/%s/%s: não devia alertar %s", c.skipped, c.lagged, c.dropped, c.id)
		case c.level != "" && (f == nil || f.Level != c.level):
			t.Errorf("%s/%s/%s: %s esperado %s, veio %+v", c.skipped, c.lagged, c.dropped, c.id, c.level, f)
		}
		if c.level != "" && a.Level != c.level {
			t.Errorf("nível geral: %s", a.Level)
		}
	}
}

func TestAnalyzeEncodingAdviceFollowsTheEncoder(t *testing.T) {
	skipped := "3000/100000 (3.0%)"
	nv := finding(analyzeLog(6000, skipped, "0 (0.0%)", "0 (0.0%)", 40), "encoding")
	if nv == nil || !strings.Contains(nv.Text, "codificador da placa de vídeo") || !strings.Contains(nv.Text, "HAGS") || nv.Page != "otimizacoes" {
		t.Errorf("NVENC: %+v", nv)
	}
	x264 := strings.NewReplacer("NVENC encoder", "x264 encoder").Replace(sampleLog(6000, skipped, "0 (0.0%)", "0 (0.0%)", true))
	ss := ParseLog(strings.NewReader(x264), "x", logStart)
	f := finding(Analyze(Pick(ss), 40), "encoding")
	if f == nil || !strings.Contains(f.Text, "x264") || !strings.Contains(f.Text, "NVENC") || strings.Contains(f.Text, "HAGS") {
		t.Errorf("x264: %+v", f)
	}
}

func TestAnalyzeNetworkChecksTheBitrateAgainstTheUpload(t *testing.T) {
	over := finding(analyzeLog(8000, "0/100000 (0.0%)", "0 (0.0%)", "3000 (3.0%)", 10), "network") // 8 Mb/s of 10
	if over == nil || !strings.Contains(over.Text, "passa de 75%") || !strings.Contains(over.Text, "8,0 Mb/s") || !strings.Contains(over.Text, "10 Mb/s") {
		t.Errorf("acima de 75%%: %+v", over)
	}
	within := finding(analyzeLog(4000, "0/100000 (0.0%)", "0 (0.0%)", "3000 (3.0%)", 40), "network") // 4 Mb/s of 40
	if within == nil || !strings.Contains(within.Text, "cabe em 75%") || !strings.Contains(within.Text, "cabo") || strings.Contains(within.Text, "passa de 75%") {
		t.Errorf("dentro de 75%%: %+v", within)
	}
	unknown := finding(analyzeLog(4000, "0/100000 (0.0%)", "0 (0.0%)", "3000 (3.0%)", 0), "network")
	if unknown == nil || !strings.Contains(unknown.Text, "Meça o upload") || unknown.Page != "rede" {
		t.Errorf("upload desconhecido: %+v", unknown)
	}
	for _, f := range []*Finding{over, within, unknown} {
		if len(f.Sources) == 0 || f.Sources[0].URL != "https://obsproject.com/kb/stream-connection-troubleshooting" {
			t.Errorf("fonte oficial: %+v", f.Sources)
		}
	}
}

func TestAnalyzeRecordingAndEdgeCases(t *testing.T) {
	if a := Analyze(nil, 40); len(a.Findings) != 0 {
		t.Error("sem sessão")
	}
	if Pick(nil) != nil {
		t.Error("Pick(nil)")
	}
	rec := `20:00:30.000: Output 'adv_file_output': Total frames output: 5000
20:00:30.000: Output 'adv_file_output': Number of lagged frames due to rendering lag/stalls: 300 (6.0%)
20:00:30.100: Video stopped, number of skipped frames due to encoding lag: 0/5000 (0.0%)
`
	ss := ParseLog(strings.NewReader(rec), "x", logStart)
	a := Analyze(Pick(ss), 40)
	if f := finding(a, "rendering"); f == nil || f.Level != LevelImportant || !strings.Contains(a.Findings[0].Title, "6,0%") {
		t.Errorf("gravação: %+v", a)
	}
	// A very short one is flagged as such, since a few frames weigh a lot.
	short := `20:00:30.000: Output 'adv_stream': Total frames output: 600
20:00:30.000: Output 'adv_stream': Number of dropped frames due to insufficient bandwidth/connection stalls: 60 (10.0%)
20:00:30.100: Video stopped, number of skipped frames due to encoding lag: 0/600 (0.0%)
`
	if a := Analyze(Pick(ParseLog(strings.NewReader(short), "x", logStart)), 40); finding(a, "short") == nil || finding(a, "network") == nil {
		t.Errorf("sessão curta: %+v", a)
	}
	// With a stream and a recording the stream is the one read.
	both := `20:00:30.000: Output 'adv_file_output': Total frames output: 5000
20:00:30.000: Output 'adv_file_output': Number of lagged frames due to rendering lag/stalls: 300 (6.0%)
20:00:30.000: Output 'adv_stream': Total frames output: 5000
20:00:30.000: Output 'adv_stream': Number of lagged frames due to rendering lag/stalls: 0 (0.0%)
20:00:30.100: Video stopped, number of skipped frames due to encoding lag: 0/5000 (0.0%)
`
	if a := Analyze(Pick(ParseLog(strings.NewReader(both), "x", logStart)), 40); a.Level != LevelOK {
		t.Errorf("lê a transmissão: %+v", a)
	}
	// Texts never promise.
	for _, f := range analyzeLog(8000, "9000/100000 (9.0%)", "9000 (9.0%)", "9000 (9.0%)", 10).Findings {
		for _, bad := range []string{"garante", "elimina", "resolve "} {
			if strings.Contains(strings.ToLower(f.Text), bad) {
				t.Errorf("%s promete: %q", f.ID, f.Text)
			}
		}
	}
}

func TestSessionLogNeverKeepsRawLines(t *testing.T) {
	// A log line with something secret-looking: nothing of it can reach the session.
	log := `20:00:01.000: [rtmp stream: 'adv_stream'] Connecting to RTMP URL rtmp://live.twitch.tv/app/live_123456_SECRETKEY...
` + forumExcerpt
	b := fmt.Sprintf("%+v", ParseLog(strings.NewReader(log), "x", logStart))
	if strings.Contains(b, "SECRETKEY") || strings.Contains(b, "twitch") {
		t.Errorf("a sessão guardou texto do log: %s", b)
	}
}

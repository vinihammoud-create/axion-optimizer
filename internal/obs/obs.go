// Package obs reads the log files of OBS Studio to tell, with the numbers OBS
// itself prints when a stream or a recording ends, why a live stream
// stuttered: the encoder could not keep up (skipped frames), the GPU was too
// busy to compose the frames (lagged frames) or the connection could not carry
// the bitrate (dropped frames).
//
// Only the log files are read (%APPDATA%\obs-studio\logs\*.txt), never the
// settings: the stream key lives in the service settings and is none of this
// package's business. Only numbers and the names of the encoder and the GPU
// are kept; no log line is stored.
//
// The messages come from OBS' source (libobs/obs-output.c, libobs/obs.c,
// libobs-d3d11 and the encoder plugins) and from real logs posted on the OBS
// forum:
//
//	Output 'adv_stream': Total frames output: 37446 (107334 attempted)
//	Output 'adv_stream': Total drawn frames: 57716 (57828 attempted)
//	Output 'adv_stream': Number of lagged frames due to rendering lag/stalls: 112 (0.2%)
//	Output 'adv_stream': Number of dropped frames due to insufficient bandwidth/connection stalls: 69888 (65.1%)
//	Video stopped, number of skipped frames due to encoding lag: 20074/50845 (39.5%)
package obs

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Kinds of output.
const (
	KindStream    = "stream"
	KindRecording = "recording"
	KindReplay    = "replay"
	KindOther     = "other"
)

// Stat is what OBS reported for one output when it stopped. -1 = not reported.
type Stat struct {
	Output    string  `json:"output"` // 'adv_stream', 'simple_file_output'...
	Kind      string  `json:"kind"`
	Frames    int64   `json:"frames"`    // frames the output delivered
	Attempted int64   `json:"attempted"` // frames it tried to deliver (the difference was dropped)
	Drawn     int64   `json:"drawn"`
	Lagged    int64   `json:"lagged"` // frames not composed in time (rendering lag: the GPU)
	LaggedPct float64 `json:"laggedPct"`
	Dropped   int64   `json:"dropped"` // frames dropped by the network (streams only)
	DropPct   float64 `json:"droppedPct"`
	// Skipped is the video-wide count of frames the encoder could not take in
	// time (encoding lag): OBS prints it once, when the video stops.
	Skipped    int64   `json:"skipped"`
	SkippedOf  int64   `json:"skippedOf"`
	SkippedPct float64 `json:"skippedPct"`
}

// Context is how OBS was set up in the session.
type Context struct {
	GPU         string  `json:"gpu,omitempty"`
	Encoder     string  `json:"encoder,omitempty"`     // x264, NVENC, AMF, QSV or ""
	RateControl string  `json:"rateControl,omitempty"` // CBR, VBR, CQP, CRF...
	BitrateKbps int     `json:"bitrateKbps"`           // 0 unknown
	BaseRes     string  `json:"baseRes,omitempty"`
	OutputRes   string  `json:"outputRes,omitempty"`
	FPS         float64 `json:"fps"`
	OBSVersion  string  `json:"obsVersion,omitempty"`
}

// Session is one stop of the video (a stream, a recording or both together).
type Session struct {
	Log     string    `json:"log"` // file name of the log
	At      time.Time `json:"at"`  // when it stopped
	Seconds float64   `json:"seconds"`
	Stats   []Stat    `json:"stats"`
	Context Context   `json:"context"`
}

// Stream returns the stream output of the session (nil without one).
func (s *Session) Stream() *Stat {
	for i := range s.Stats {
		if s.Stats[i].Kind == KindStream {
			return &s.Stats[i]
		}
	}
	return nil
}

var (
	reLine     = regexp.MustCompile(`^(\d\d):(\d\d):(\d\d)\.(\d{3}): (.*)$`)
	reFrames   = regexp.MustCompile(`^Output '([^']+)': Total frames output: (\d+)(?: \((\d+) attempted\))?\s*$`)
	reDrawn    = regexp.MustCompile(`^Output '([^']+)': Total drawn frames: (\d+)(?: \((\d+) attempted\))?\s*$`)
	reLagged   = regexp.MustCompile(`^Output '([^']+)': Number of lagged frames due to rendering lag/stalls: (\d+) \((\d+(?:[.,]\d+)?)%\)\s*$`)
	reDropped  = regexp.MustCompile(`^Output '([^']+)': Number of dropped frames due to insufficient bandwidth/connection stalls: (\d+) \((\d+(?:[.,]\d+)?)%\)\s*$`)
	reSkipped  = regexp.MustCompile(`^Video stopped, number of skipped frames due to encoding lag: (\d+)/(\d+) \((\d+(?:[.,]\d+)?)%\)\s*$`)
	reAdapter  = regexp.MustCompile(`^Loading up D3D11 on adapter (.+) \((\d+)\)\s*$`)
	reVersion  = regexp.MustCompile(`^OBS (\d+(?:\.\d+)+\S*) \(`)
	reBase     = regexp.MustCompile(`base resolution:\s+(\d+x\d+)`)
	reOutRes   = regexp.MustCompile(`output resolution:\s+(\d+x\d+)`)
	reFPS      = regexp.MustCompile(`fps:\s+(\d+)/(\d+)`)
	reRate     = regexp.MustCompile(`^\s*rate_control:\s+(\S+)`)
	reBitrate  = regexp.MustCompile(`^\s*bitrate:\s+(\d+)`)
	reStartEnd = regexp.MustCompile(`={3,}\s*(Streaming|Recording)\s+(Start|Stop)\s*={3,}`)
	reLogName  = regexp.MustCompile(`^(\d{4})-(\d\d)-(\d\d) (\d\d)-(\d\d)-(\d\d)`)
)

func pct(s string) float64 {
	v, _ := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64)
	return v
}

func num(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func kindOf(output string) string {
	n := strings.ToLower(output)
	switch {
	case strings.Contains(n, "replay"):
		return KindReplay
	case strings.Contains(n, "stream") || strings.Contains(n, "multitrack"):
		return KindStream
	case strings.Contains(n, "file") || strings.Contains(n, "record"):
		return KindRecording
	}
	return KindOther
}

func familyOf(header string) string {
	h := strings.ToLower(header)
	switch {
	case strings.Contains(h, "x264"):
		return "x264"
	case strings.Contains(h, "nvenc"):
		return "NVENC"
	case strings.Contains(h, "amf"):
		return "AMF"
	case strings.Contains(h, "qsv"):
		return "QSV"
	}
	return ""
}

// encBlock is the "settings:" block of an encoder.
type encBlock struct {
	family, rate string
	bitrate      int
	streaming    bool
	seen         bool // it showed a rate control or a bitrate, so it is an encoder's
}

// ParseLog reads one OBS log. at is the time the log started (it is in the
// file name); the lines only carry the time of day. Sessions come in the order
// of the log.
func ParseLog(r io.Reader, name string, at time.Time) []Session {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)

	var (
		out       []Session
		pending   []*Stat
		byName    = map[string]*Stat{}
		ctx       Context
		video     Context // base/output resolution and fps of the last "video settings reset"
		lastEnc   *encBlock
		lastStrm  *encBlock
		lastTS    = at
		started   time.Time
		block     string // header of the indented block being read: "video" or "enc"
		cur       *encBlock
		midnight  = time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, at.Location())
		prevTOD   = at.Sub(midnight) // the log starts at its file's time
		dayOffset int
	)
	// remember records an encoder block once it shows a rate control or a bitrate:
	// other plugins print "settings:" blocks that are not an encoder's.
	remember := func(b *encBlock) {
		if b.seen {
			return
		}
		b.seen = true
		lastEnc = b
		if b.streaming {
			lastStrm = b
		}
	}
	stat := func(output string) *Stat {
		s := byName[output]
		if s == nil {
			s = &Stat{Output: output, Kind: kindOf(output), Frames: -1, Attempted: -1, Drawn: -1, Lagged: -1, LaggedPct: -1, Dropped: -1, DropPct: -1, Skipped: -1, SkippedOf: -1, SkippedPct: -1}
			byName[output] = s
			pending = append(pending, s)
		}
		return s
	}
	finish := func(stop time.Time) {
		if len(pending) == 0 {
			return
		}
		c := ctx
		c.BaseRes, c.OutputRes, c.FPS = video.BaseRes, video.OutputRes, video.FPS
		enc := lastEnc
		for _, s := range pending {
			if s.Kind == KindStream && lastStrm != nil {
				enc = lastStrm
			}
		}
		if enc != nil {
			c.Encoder, c.RateControl, c.BitrateKbps = enc.family, enc.rate, enc.bitrate
		}
		sess := Session{Log: name, At: stop, Context: c}
		if !started.IsZero() && stop.After(started) && stop.Sub(started) < 24*time.Hour {
			sess.Seconds = stop.Sub(started).Seconds()
		}
		for _, s := range pending {
			sess.Stats = append(sess.Stats, *s)
		}
		out = append(out, sess)
		pending, byName = nil, map[string]*Stat{}
	}

	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		msg := line
		if m := reLine.FindStringSubmatch(line); m != nil {
			tod := time.Duration(num(m[1]))*time.Hour + time.Duration(num(m[2]))*time.Minute + time.Duration(num(m[3]))*time.Second + time.Duration(num(m[4]))*time.Millisecond
			if tod < prevTOD-time.Hour { // the time of day went back: the log went past midnight
				dayOffset++
			}
			prevTOD = tod
			lastTS, msg = midnight.AddDate(0, 0, dayOffset).Add(tod), m[5]
		}
		// The lines of a block are indented, with or without their own time stamp.
		if strings.HasPrefix(msg, "\t") || strings.HasPrefix(msg, " ") {
			switch block {
			case "video":
				if m := reBase.FindStringSubmatch(msg); m != nil {
					video.BaseRes = m[1]
				} else if m := reOutRes.FindStringSubmatch(msg); m != nil {
					video.OutputRes = m[1]
				} else if m := reFPS.FindStringSubmatch(msg); m != nil {
					if d := num(m[2]); d > 0 {
						video.FPS = float64(num(m[1])) / float64(d)
					}
				}
			case "enc":
				if m := reRate.FindStringSubmatch(msg); m != nil && cur != nil {
					cur.rate = m[1]
					remember(cur)
				} else if m := reBitrate.FindStringSubmatch(msg); m != nil && cur != nil {
					cur.bitrate = int(num(m[1]))
					remember(cur)
				}
			}
			continue
		}
		block = ""
		switch {
		case strings.EqualFold(strings.TrimSpace(msg), "video settings reset:"):
			block = "video"
			video = Context{}
		case strings.HasSuffix(strings.TrimSpace(msg), "settings:"):
			cur = &encBlock{family: familyOf(msg), streaming: strings.Contains(strings.ToLower(msg), "stream")}
			block = "enc"
		case reAdapter.MatchString(msg):
			ctx.GPU = strings.TrimSpace(reAdapter.FindStringSubmatch(msg)[1])
		case reVersion.MatchString(msg):
			ctx.OBSVersion = reVersion.FindStringSubmatch(msg)[1]
		case reStartEnd.MatchString(msg):
			if m := reStartEnd.FindStringSubmatch(msg); m[2] == "Start" {
				started = lastTS
			}
		case reFrames.MatchString(msg):
			m := reFrames.FindStringSubmatch(msg)
			s := stat(m[1])
			s.Frames, s.Attempted = num(m[2]), num(m[2])
			if m[3] != "" {
				s.Attempted = num(m[3])
			}
		case reDrawn.MatchString(msg):
			m := reDrawn.FindStringSubmatch(msg)
			stat(m[1]).Drawn = num(m[2])
		case reLagged.MatchString(msg):
			m := reLagged.FindStringSubmatch(msg)
			s := stat(m[1])
			s.Lagged, s.LaggedPct = num(m[2]), pct(m[3])
		case reDropped.MatchString(msg):
			m := reDropped.FindStringSubmatch(msg)
			s := stat(m[1])
			s.Dropped, s.DropPct = num(m[2]), pct(m[3])
		case reSkipped.MatchString(msg):
			m := reSkipped.FindStringSubmatch(msg)
			for _, s := range pending {
				s.Skipped, s.SkippedOf, s.SkippedPct = num(m[1]), num(m[2]), pct(m[3])
			}
			finish(lastTS)
		}
	}
	// A log that ends without "Video stopped" (OBS crashed or was killed): what
	// was read is kept, without the encoder's count.
	finish(lastTS)
	return out
}

// logTime reads the start of a log from its file name ("2026-10-05 12-34-56.txt").
func logTime(name string, fallback time.Time) time.Time {
	m := reLogName.FindStringSubmatch(name)
	if m == nil {
		return fallback
	}
	return time.Date(int(num(m[1])), time.Month(num(m[2])), int(num(m[3])), int(num(m[4])), int(num(m[5])), int(num(m[6])), 0, time.Local)
}

// Limits of what is read.
const (
	MaxLogs     = 10                  // newest logs read
	MaxAge      = 60 * 24 * time.Hour // older logs say nothing about this PC today
	MaxLogBytes = 64 << 20            // a log bigger than this is skipped
	MaxSessions = 12
)

// Read reads the newest logs of dir and returns the sessions, newest first.
// found says whether the folder exists (OBS has been used on this PC).
func Read(dir string, now time.Time) (sessions []Session, found bool) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	found = true
	type lf struct {
		name string
		at   time.Time
	}
	var logs []lf
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(n), ".txt") || !reLogName.MatchString(n) {
			continue
		}
		fi, err := e.Info()
		if err != nil || fi.Size() > MaxLogBytes {
			continue
		}
		at := logTime(n, fi.ModTime())
		if now.Sub(at) > MaxAge {
			continue
		}
		logs = append(logs, lf{n, at})
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].at.After(logs[j].at) })
	if len(logs) > MaxLogs {
		logs = logs[:MaxLogs]
	}
	for _, l := range logs {
		f, err := os.Open(filepath.Join(dir, l.name))
		if err != nil {
			continue
		}
		sessions = append(sessions, ParseLog(f, l.name, l.at)...)
		f.Close()
	}
	sort.SliceStable(sessions, func(i, j int) bool { return sessions[i].At.After(sessions[j].At) })
	if len(sessions) > MaxSessions {
		sessions = sessions[:MaxSessions]
	}
	return sessions, found
}

package obs

import (
	"strings"
	"testing"
)

func FuzzParseLog(f *testing.F) {
	f.Add(forumExcerpt)
	f.Add(sampleLog(6000, "130/216000 (0.1%)", "3 (0.0%)", "1000 (0.5%)", true))
	f.Add(sampleLog(6000, "130/216000 (0.1%)", "3 (0.0%)", "1000 (0.5%)", false))
	f.Add(`20:00:01.000: [x264 encoder: 'streaming_h264'] settings:
20:00:01.000:   rate_control: CBR
20:00:01.000:   bitrate:      4500
20:00:02.000: [some plugin] settings:
20:00:02.000:   color: red
20:00:30.000: Output 'simple_stream': Total frames output: 100
20:00:30.000: Video stopped, number of skipped frames due to encoding lag: 5/100 (5.0%)
`)
	f.Add("")
	f.Fuzz(func(t *testing.T, log string) {
		ParseLog(strings.NewReader(log), "x", logStart)
	})
}

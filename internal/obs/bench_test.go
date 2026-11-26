package obs

import (
	"fmt"
	"strings"
	"testing"
)

// Input shapes for BenchmarkParseLogGrowth. Each builds a log of about n bytes.
var growthShapes = []struct {
	name  string
	build func(n int) string
}{
	{"repeated-stat-lines", func(n int) string {
		return strings.Repeat("20:00:01.000: Output 'adv_stream': Total frames output: 100 (120 attempted)\n", n/74+1)
	}},
	{"distinct-outputs", func(n int) string {
		var b strings.Builder
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "20:00:01.000: Output 'o%d': Total frames output: 100\n", i)
		}
		return b.String()
	}},
	{"distinct-outputs-then-skipped", func(n int) string {
		var b strings.Builder
		for i := 0; b.Len() < n; i++ {
			fmt.Fprintf(&b, "20:00:01.000: Output 'o%d': Total frames output: 100\n", i)
		}
		b.WriteString("20:00:02.000: Video stopped, number of skipped frames due to encoding lag: 1/100 (1.0%)\n")
		return b.String()
	}},
	{"many-sessions", func(n int) string {
		const one = "20:00:01.000: Output 'a': Total frames output: 100\n20:00:02.000: Video stopped, number of skipped frames due to encoding lag: 1/100 (1.0%)\n"
		return strings.Repeat(one, n/len(one)+1)
	}},
	{"many-short-lines", func(n int) string { return strings.Repeat("a\n", n/2) }},
	{"many-empty-lines", func(n int) string { return strings.Repeat("\n", n) }},
	{"many-timestamped-lines", func(n int) string { return strings.Repeat("20:00:01.000: x\n", n/16+1) }},
	{"indented-block-lines", func(n int) string {
		return "video settings reset:\n" + strings.Repeat("\tfps: 60/1\n", n/10+1)
	}},
	{"encoder-blocks", func(n int) string {
		return strings.Repeat("[x264 encoder: 'streaming_h264'] settings:\n\trate_control: CBR\n\tbitrate: 4500\n", n/70+1)
	}},
	{"one-long-line", func(n int) string { return strings.Repeat("a", n) }},
	{"one-long-equals-line", func(n int) string { return strings.Repeat("=", n) }},
	{"one-long-spaces-line", func(n int) string { return strings.Repeat(" ", n) }},
	{"one-long-cr-line", func(n int) string { return "x" + strings.Repeat("\r", n) }},
	{"one-long-timestamped-line", func(n int) string { return "20:00:01.000: " + strings.Repeat("Output '", n/8) }},
	{"one-long-output-name", func(n int) string {
		return "20:00:01.000: Output '" + strings.Repeat("a", n) + "': Total frames output: 1\n"
	}},
	{"equals-lines-of-16KiB", func(n int) string { return strings.Repeat(strings.Repeat("=", 16<<10)+"\n", n/(16<<10)) }},
	{"one-long-settings-line", func(n int) string { return strings.Repeat("a", n) + " settings:\n" }},
}

// BenchmarkParseLogGrowth times ParseLog for inputs that grow by 4x per step.
// Linear code takes about 4x longer per step; quadratic code about 16x.
func BenchmarkParseLogGrowth(b *testing.B) {
	for _, sh := range growthShapes {
		for _, n := range []int{64 << 10, 256 << 10, 1 << 20, 4 << 20} {
			log := sh.build(n)
			b.Run(fmt.Sprintf("%s/%dKiB", sh.name, len(log)>>10), func(b *testing.B) {
				b.SetBytes(int64(len(log)))
				for b.Loop() {
					ParseLog(strings.NewReader(log), "x", logStart)
				}
			})
		}
	}
}

// BenchmarkParseLogSmall times ParseLog for a few hundred bytes: what is left
// is the cost that does not depend on the input.
func BenchmarkParseLogSmall(b *testing.B) {
	b.SetBytes(int64(len(forumExcerpt)))
	for b.Loop() {
		ParseLog(strings.NewReader(forumExcerpt), "x", logStart)
	}
}

func BenchmarkParseLogEmpty(b *testing.B) {
	for b.Loop() {
		ParseLog(strings.NewReader(""), "x", logStart)
	}
}

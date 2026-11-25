package perf

import (
	"strings"
	"testing"
)

func FuzzParseCSV(f *testing.F) {
	var sb strings.Builder
	sb.WriteString(header26)
	sb.WriteString(row("cs2.exe", "0xA", 0, 0, 0, 0, 0, "Hardware: Independent Flip", "NA"))
	tm := 0.0
	for i := 0; i < 5; i++ {
		tm += 2.5
		sb.WriteString(row("cs2.exe", "0xA", tm, 2.5, 1.0, 1.4, 2.4, "Hardware: Independent Flip", "NA"))
	}
	sb.WriteString(row("cs2.exe", "0xB", 16, 16.6, 1, 15, 1, "Composed: Flip", "NA"))
	sb.WriteString(row("Discord.exe", "0xC", 16, 16.6, 1, 15, 1, "Composed: Flip", "NA"))
	sb.WriteString("cs2.exe,4242,0xA,DXGI,0,512,1,Hardware: Indep")
	f.Add(sb.String(), "CS2")
	f.Add(header26, "cs2.exe")
	f.Add("a,b,c\n1,2,3\n", "")
	f.Fuzz(func(t *testing.T, src, process string) {
		ParseCSV(strings.NewReader(src), process)
	})
}

package sys

import "testing"

func TestSameProcessImage(t *testing.T) {
	cases := []struct {
		image, expect string
		want          bool
	}{
		{`C:\Program Files\Steam\steam.exe`, "steam.exe", true},
		{`C:\Program Files\Steam\steam.exe`, "STEAM.EXE", true},
		{`C:\Program Files\Steam\steamwebhelper.exe`, "steam.exe", false},
		{`C:\ProgramData\Axion\bin\PresentMon.exe`, `c:\programdata\axion\bin\presentmon.exe`, true},
		{`C:\Users\x\Downloads\PresentMon.exe`, `C:\ProgramData\Axion\bin\PresentMon.exe`, false},
		{`C:\x\a.exe`, "", false},
		{"", "a.exe", false},
		{"a.exe", "a.exe", true},
	}
	for _, c := range cases {
		if got := SameProcessImage(c.image, c.expect); got != c.want {
			t.Errorf("SameProcessImage(%q, %q) = %v", c.image, c.expect, got)
		}
	}
}

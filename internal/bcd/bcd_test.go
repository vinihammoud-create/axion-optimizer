package bcd

import "testing"

func TestNormalizeFor(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"testsigning", "Sim", "true"},
		{"testsigning", "Не", "Не"}, // unknown word: kept
		{"testsigning", "Нет", "false"},
		{"nointegritychecks", "Tak", "true"},
		{"hypervisorlaunchtype", "Off", "Off"}, // enum value, not a boolean
		{"hypervisorlaunchtype", "Auto", "Auto"},
		{"nx", "OptIn", "OptIn"},
		{"numproc", "0", "0"},
	}
	for _, c := range cases {
		if got := NormalizeFor(c.name, c.in); got != c.want {
			t.Errorf("NormalizeFor(%q, %q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

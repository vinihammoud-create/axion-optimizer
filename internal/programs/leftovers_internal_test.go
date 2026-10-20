package programs

import "testing"

func TestInstallDirDefault(t *testing.T) {
	p := Program{Name: "Tiny Tool 2.1", Publisher: "Tiny Soft", SizeBytes: 100 << 10}
	cases := []struct {
		dir   string
		size  int64
		size0 bool
		want  bool
	}{
		{`D:\Games`, 3 << 20, false, false},                     // generic/shared folder
		{`C:\Program Files\Tiny Tool`, 2 << 20, false, true},    // own folder, small
		{`C:\Program Files\Tiny Soft`, 1 << 20, false, true},    // maker's folder
		{`C:\Program Files\Tiny Tool`, 900 << 20, false, false}, // far larger than declared
		{`C:\Program Files\Tiny Tool`, 60 << 20, true, false},   // no declared size, large
		{`C:\Program Files\Tiny Tool`, 10 << 20, true, true},    // no declared size, small
	}
	for _, c := range cases {
		q := p
		q.Location = c.dir // as on Windows: the install folder is the one checked
		if c.size0 {
			q.SizeBytes = 0
		}
		if got, why := installDirDefault(q, c.dir, c.size); got != c.want {
			t.Errorf("%s (%d B, size0=%v): got %v (%s), want %v", c.dir, c.size, c.size0, got, why, c.want)
		}
	}
}

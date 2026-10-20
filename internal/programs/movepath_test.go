package programs

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// A move never goes through a link or junction in a parent folder. The test
// makes a junction (no privilege needed) with mklink; elsewhere, a symlink.
func TestMovePathRefusesLinkedParent(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(filepath.Join(real, "Vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, real).CombinedOutput(); err != nil {
			t.Skipf("mklink /J: %v %s", err, out)
		}
	} else if err := os.Symlink(real, link); err != nil {
		t.Skipf("sem permissão para criar link: %v", err)
	}
	if err := movePath(filepath.Join(link, "Vendor"), filepath.Join(base, "q", "Vendor")); err == nil {
		t.Fatal("moveu através de um link")
	}
	if _, err := os.Stat(filepath.Join(real, "Vendor")); err != nil {
		t.Fatal("a pasta real saiu do lugar")
	}
	if err := movePath(filepath.Join(real, "Vendor"), filepath.Join(base, "q", "Vendor")); err != nil {
		t.Fatalf("caminho sem link: %v", err)
	}
}

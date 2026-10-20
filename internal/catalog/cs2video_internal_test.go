package catalog

import (
	"io/fs"
	"strings"
	"testing"
)

// A link is never replaced and a read-only file is never unlocked. Tested on
// the mode bits, because creating a link needs a privilege the test machine
// may not have.
func TestCS2ModeRefusal(t *testing.T) {
	if err := cs2ModeRefusal(0o644); err != nil {
		t.Errorf("arquivo comum: %v", err)
	}
	if err := cs2ModeRefusal(fs.ModeSymlink | 0o777); err == nil || !strings.Contains(err.Error(), "link") {
		t.Errorf("link: %v", err)
	}
	if err := cs2ModeRefusal(0o444); err == nil || !strings.Contains(err.Error(), "somente leitura") {
		t.Errorf("somente leitura: %v", err)
	}
}

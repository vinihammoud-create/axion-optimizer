//go:build windows

package winsys

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"testing"
	"time"

	"axion/internal/dpc"
)

// realDPCOut turns on the check against the real kernel: it needs an elevated
// process and writes what it read to that file. Run it by hand:
//
//	go test -c -o axion-dpc-check.test.exe ./internal/sys/winsys
//	axion-dpc-check.test.exe -test.run TestRealDPCCapture -real-dpc-out result.json   (as administrator)
var realDPCOut = flag.String("real-dpc-out", "", "grava o resultado de uma captura real de DPC/ISR neste arquivo (precisa de administrador)")

func TestRealDPCCapture(t *testing.T) {
	if *realDPCOut == "" {
		t.Skip("verificação com o kernel real: informe -real-dpc-out (como administrador)")
	}
	n := &Native{}
	capt, err := n.CaptureDPC(context.Background(), 10*time.Second)
	out := map[string]any{"error": "", "capture": capt, "summary": dpc.Summarize(capt)}
	if err != nil {
		out["error"] = err.Error()
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	if werr := os.WriteFile(*realDPCOut, b, 0o644); werr != nil {
		t.Fatal(werr)
	}
	if err != nil {
		t.Fatalf("captura real: %v", err)
	}
	t.Logf("eventos=%d descartados=%d perdidos=%d módulos? %v", capt.Events, capt.Discarded, capt.LostEvents, !capt.NoModules)
}

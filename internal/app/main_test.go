package app

import (
	"io"
	"log"
	"os"
	"testing"
)

// The app logs every request, job and analysis step; the tests keep their
// output to the failures (go test -v with AXION_TEST_LOG=1 shows the log).
func TestMain(m *testing.M) {
	if os.Getenv("AXION_TEST_LOG") == "" {
		log.SetOutput(io.Discard)
	}
	os.Exit(m.Run())
}

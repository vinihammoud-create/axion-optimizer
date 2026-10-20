package netprobe

import (
	"net/http"
	"testing"
	"time"
)

func TestStatusErrorWait(t *testing.T) {
	for v, want := range map[string]time.Duration{"120": 2 * time.Minute, " 30 ": 30 * time.Second, "": 0, "abc": 0, "-5": 0, "999999": 24 * time.Hour} {
		if got := (&StatusError{Code: 429, RetryAfter: v}).Wait(); got != want {
			t.Errorf("Retry-After %q: %v, esperado %v", v, got, want)
		}
	}
	at := time.Now().Add(10 * time.Minute).UTC().Format(http.TimeFormat)
	if got := (&StatusError{Code: 429, RetryAfter: at}).Wait(); got < 9*time.Minute || got > 10*time.Minute {
		t.Errorf("Retry-After em data: %v", got)
	}
	past := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)
	if got := (&StatusError{Code: 429, RetryAfter: past}).Wait(); got != 0 {
		t.Errorf("data no passado: %v", got)
	}
}

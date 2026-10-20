package sys

import (
	"fmt"
	"testing"
)

// Shapes of go-ole's errors: *OleError (Code) wrapping an EXCEPINFO (SCODE).
type fakeOle struct {
	hr  uintptr
	sub error
}

func (e *fakeOle) Error() string   { return "Exceção." }
func (e *fakeOle) Code() uintptr   { return e.hr }
func (e *fakeOle) Unwrap() error   { return e.sub }
func (e *fakeOle) SubError() error { return e.sub }

type fakeExcep struct{ scode uint32 }

func (e fakeExcep) Error() string { return "<nil>" }
func (e fakeExcep) SCODE() uint32 { return e.scode }

func TestHResult(t *testing.T) {
	missing := &fakeOle{hr: 0x80020009, sub: fakeExcep{scode: 0x80070002}}
	if got := HResult(fmt.Errorf("GetTask: %w", missing)); got != 0x80070002 {
		t.Errorf("tarefa inexistente: 0x%08X", got)
	}
	if got := HResult(&fakeOle{hr: 0x80070005}); got != 0x80070005 {
		t.Errorf("sem EXCEPINFO: 0x%08X", got)
	}
	if got := HResult(&fakeOle{hr: 0x80020009, sub: fakeExcep{}}); got != 0x80020009 {
		t.Errorf("EXCEPINFO sem código: 0x%08X", got)
	}
	if got := HResult(fmt.Errorf("comum")); got != 0 {
		t.Errorf("erro comum: 0x%08X", got)
	}
}

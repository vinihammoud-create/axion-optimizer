package main

import (
	"strings"
	"testing"
)

// The report Axion 0.5.2 printed to its standard error when the scan died
// inside QueryServiceConfig (misaligned buffer), captured on the user's PC.
const report052 = `Exception 0xc0000005 0x0 0xffffffffffffffff 0x7ff83737cba2
PC=0x7ff83737cba2

runtime.cgocall(0xd280e0, 0xc000088360)
        runtime/cgocall.go:167 +0x3e fp=0xc000450d28 sp=0xc000450cc0 pc=0xd1c07e
runtime.syscall_syscalln(0x0?, 0x0?, {0xc000450da0?, 0x1647190?, 0xc000450d58?})
        runtime/syscall_windows.go:521 +0x4e fp=0xc000450d48 sp=0xc000450d28 pc=0xd0aa2e
syscall.SyscallN(0x1647180?, {0xc000450da0?, 0x6f0?, 0xc000450ea8?})
        runtime/syscall_windows.go:500 +0x1e fp=0xc000450d80 sp=0xc000450d48 pc=0xd2297e
golang.org/x/sys/windows.QueryServiceConfig(0x20981fb6150, 0xcc471e?, 0x400, 0xc0002061c0?)
        golang.org/x/sys@v0.36.0/windows/zsyscall_windows.go:1111 +0x9d fp=0xc000450de8 sp=0xc000450d80 pc=0xedf09d
axion/internal/sys/winsys.Services.Query.func1(0x20981fb6150)
        axion/internal/sys/winsys/services.go:50 +0xb2 fp=0xc000451270 sp=0xc000450de8 pc=0x10adcb2
axion/internal/sys/winsys.withService({0x1220b68, 0x9}, 0x5, 0xc000451388)
        axion/internal/sys/winsys/services.go:34 +0x106 fp=0xc0004512e8 sp=0xc000451270 pc=0x10ad7c6
runtime.goexit({})
        runtime/asm_amd64.s:1700 +0x1 fp=0xc000451fe8 sp=0xc000451fe0 pc=0xd268e1
created by axion/internal/app.(*Jobs).Start in goroutine 38
        axion/internal/app/jobs.go:128 +0x725

goroutine 1 gp=0xc0000021c0 m=0 mp=0x1661660 [syscall, locked to thread]:
main.main()
        axion/cmd/axion/main_windows.go:214 +0x1238 fp=0xc00015df50 sp=0xc00015db40 pc=0x10ccc38
rax     0x450ec000000000
gs      0x2b
`

func TestCrashSummaryException(t *testing.T) {
	got := crashSummary(report052)
	want := "exceção 0xc0000005 do Windows (leitura em 0xffffffffffffffff) dentro de uma chamada ao sistema — em axion/internal/sys/winsys.Services.Query.func1(0x20981fb6150) (axion/internal/sys/winsys/services.go:50)"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestCrashSummaryPanic(t *testing.T) {
	r := "panic: runtime error: invalid memory address or nil pointer dereference\n[signal 0xc0000005 code=0x0 addr=0x2 pc=0x328f58]\n\ngoroutine 1 [running, locked to thread]:\ngolang.org/x/sys/windows.ACLFromEntries(...)\n        golang.org/x/sys@v0.36.0/windows/security_windows.go:1494 +0x98\naxion/internal/sys/winsys.resetContents({0xc000023400, 0x14}, 0x1)\n        axion/internal/sys/winsys/secure.go:289 +0x37\nmain.main()\n        axion/cmd/axion/main_windows.go:54 +0xbd\n"
	got := crashSummary(r)
	if !strings.HasPrefix(got, "panic: runtime error: invalid memory address") || !strings.Contains(got, "winsys.resetContents") || !strings.Contains(got, "(axion/internal/sys/winsys/secure.go:289)") {
		t.Fatalf("got %q", got)
	}
}

func TestCrashSummaryPlainText(t *testing.T) {
	if got := crashSummary("plataforma: acesso negado\n"); got != "plataforma: acesso negado" {
		t.Fatalf("got %q", got)
	}
	if got := crashSummary("  \n"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestHasCrashReport(t *testing.T) {
	if !hasCrashReport(report052) {
		t.Fatal("exception report not recognized")
	}
	if !hasCrashReport("panic: boom\n\ngoroutine 1 [running]:\nmain.main()\n") {
		t.Fatal("panic not recognized")
	}
	noise := "[0926/200836.770:ERROR:ui\\gfx\\win\\window_impl.cc:124] Failed to unregister class Chrome_WidgetWin_0. Error = 1411\n"
	if hasCrashReport(noise) || hasCrashReport("") {
		t.Fatal("WebView2 log line taken as a crash")
	}
}

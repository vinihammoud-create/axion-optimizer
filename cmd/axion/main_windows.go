//go:build windows

// Command axion is the Windows desktop application: a WebView2 window over the
// local, token-protected API. When the WebView2 runtime is unavailable the
// interface opens in the default browser instead.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"

	"axion/internal/app"
	"axion/internal/sys/winsys"
	"axion/internal/web"
)

const (
	windowTitle = "Axion Optimizer"
	mutexName   = `Global\AxionOptimizer.SingleInstance`
	// finishingName exists while a closed Axion finishes a task that was
	// running: a new launch is told to wait instead of offered to end it.
	finishingName = `Global\AxionOptimizer.Finishing`
	// reopenName exists while the interface runs in the browser: a new
	// launch signals it and the running Axion opens a new tab. It lives in
	// the session's own namespace, so a launch by another user signed in on
	// the same PC never opens a tab on this desktop.
	reopenName = `Local\AxionOptimizer.Reopen`
)

var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW          = user32.NewProc("FindWindowW")
	procSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	procShowWindow           = user32.NewProc("ShowWindow")
	procIsIconic             = user32.NewProc("IsIconic")
	procGetDpiForSystem      = user32.NewProc("GetDpiForSystem")
	procSystemParametersInfo = user32.NewProc("SystemParametersInfoW")
	procMessageBoxW          = user32.NewProc("MessageBoxW")
	dwmapi                   = windows.NewLazySystemDLL("dwmapi.dll")
	procDwmSetWindowAttr     = dwmapi.NewProc("DwmSetWindowAttribute")
)

// hostileEnvPrefixes are environment variables that make a program load
// code from where they point: the WebView2 loader takes its browser folder
// and extra arguments from WEBVIEW2_*, and the .NET runtime (MMC snap-ins
// opened from Axion run elevated) loads profilers named by COR_*, COMPlus_*
// and DOTNET_*. Axion runs as administrator and inherits the user's
// environment, which any program of that user can change, so they go before
// anything starts.
var hostileEnvPrefixes = []string{"WEBVIEW2_", "COR_", "COMPLUS_", "DOTNET_", "CORECLR_"}

func scrubEnvironment() {
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		up := strings.ToUpper(k)
		for _, p := range hostileEnvPrefixes {
			if strings.HasPrefix(up, p) {
				_ = os.Unsetenv(k)
				break
			}
		}
	}
}

func main() {
	scrubEnvironment()
	// The collection process (see collect_windows.go) does nothing else.
	if len(os.Args) >= 2 && os.Args[1] == collectChildFlag {
		os.Exit(runCollectChild(os.Args[2:]))
	}
	dataDir := filepath.Join(programData(), "Axion")
	// Nothing may fail silently: a panic anywhere in main shows a message
	// (fatal runtime errors, which skip recover, go to crash.log below).
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC: %v\n%s", r, debug.Stack())
			messageBox(fmt.Sprintf("O Axion encontrou um erro inesperado e precisa fechar.\n\n%v\n\nDetalhes em:\n%s", r, filepath.Join(dataDir, "axion.log")), 0x10)
			os.Exit(1)
		}
	}()
	// Protect the folder before anything is written to it (the log included):
	// a junction or a folder planted there by another account must not
	// redirect or feed privileged writes. Without a safe folder Axion does not
	// run at all.
	hardenWarn, hardenErr := winsys.PrepareDataDir(dataDir)
	if hardenErr != nil {
		messageBox("O Axion não conseguiu preparar a pasta de dados com segurança e vai fechar.\n\n"+hardenErr.Error()+
			"\n\nFeche outros programas que estejam usando essa pasta (antivírus, backup) e abra o Axion de novo.", 0x10)
		os.Exit(1)
	}
	setupLog(dataDir)
	lastCrash, lastCrashFile := setupCrashOutput(dataDir)
	log.Printf("Axion %s: pasta de dados pronta", Version)
	if hardenWarn != nil {
		log.Printf("aviso: permissões de itens da pasta de dados: %v", hardenWarn)
	}
	if lastCrash != "" {
		log.Printf("a execução anterior terminou com erro (%s)", lastCrashFile)
		messageBox("Na última vez o Axion fechou por causa de um erro. O relatório foi salvo em:\n"+lastCrashFile+
			"\n\nInício do relatório:\n"+lastCrash, 0x30)
	}

	if !singleInstance() {
		return
	}
	log.Printf("Axion %s iniciando", Version)
	installNativeCrashLog()
	webview2.OnWindowEvent = func(ev string) { log.Printf("janela: %s", ev) }

	p, err := winsys.New(dataDir)
	if err != nil {
		fatal("Não foi possível iniciar: %v", err)
	}
	log.Printf("plataforma pronta")
	a, err := app.New(p, Version)
	if err != nil {
		fatal("Não foi possível iniciar: %v", err)
	}
	// The analysis runs in a separate Axion process: a Windows component that
	// fails inside a probe ends that process, not this window.
	if exe, err := os.Executable(); err == nil {
		a.Collector = childCollector(exe, dataDir)
	} else {
		log.Printf("coleta no próprio processo (%v)", err)
	}
	srv, err := app.NewServer(a, web.FS())
	if err != nil {
		fatal("Não foi possível abrir a interface local: %v", err)
	}
	go func() {
		if err := srv.Serve(); err != nil {
			log.Printf("servidor: %v", err)
		}
	}()
	defer srv.Close()

	log.Printf("interface local em %s", srv.URL())
	// The browser session runs at most once (the 45 s fallback below and a
	// missing WebView2 runtime may both ask for it); a second caller waits
	// for it to end. The same holds for the end of the session.
	var browserOnce, endOnce sync.Once
	browser := func(why string) { browserOnce.Do(func() { browserSession(srv, why) }) }
	end := func() { endOnce.Do(func() { shutdown(a, srv) }) }
	if err := prepareWebView2Loader(dataDir); err != nil {
		log.Printf("janela própria indisponível (%v): abrindo no navegador", err)
		browser("o WebView2Loader.dll não pôde ser usado (" + err.Error() + ")")
		end()
		return
	}
	log.Printf("WebView2Loader carregado; criando a janela")
	// If the WebView2 runtime never answers, the interface still opens in the
	// browser instead of leaving the user with nothing.
	windowReady := make(chan struct{})
	var windowUp atomic.Bool
	go func() {
		select {
		case <-windowReady:
			return
		case <-time.After(45 * time.Second):
		}
		log.Printf("a janela não abriu em 45 s: abrindo no navegador")
		browser("a janela do Axion não abriu a tempo")
		// The notice was closed. A window that opened in the meantime keeps
		// Axion running (closing it ends the program); otherwise Axion ends
		// here, after the running task.
		if windowUp.Load() {
			return
		}
		end()
		os.Exit(0)
	}()
	scale := systemScale()
	width, height := fitToWorkArea(int(1360*scale), int(880*scale))
	bg := &edge.COREWEBVIEW2_COLOR{A: 255, R: 9, G: 11, B: 16}
	var userClosed bool
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus:       true,
		DataPath:        filepath.Join(localAppData(), "Axion", "WebView2"),
		BackgroundColor: bg,
		WindowOptions:   webview2.WindowOptions{Title: windowTitle, Width: uint(width), Height: uint(height), IconId: 1, Center: true},
		Closed:          &userClosed,
	})
	if w != nil {
		windowUp.Store(true)
	}
	close(windowReady)
	if w == nil && userClosed {
		// Closed while WebView2 was still starting: the user wants out, not
		// the browser fallback. If the fallback already opened the browser
		// (WebView2 took more than 45 s), that session ends first, when the
		// user closes its notice.
		log.Printf("janela fechada antes de terminar de abrir")
		browserOnce.Do(func() {})
		end()
		return
	}
	if w == nil {
		log.Printf("WebView2 indisponível: abrindo no navegador")
		browser("o Microsoft Edge WebView2 Runtime não está disponível neste Windows")
		end()
		return
	}
	log.Printf("janela criada")
	// A short trail of life signs: if Axion is ended from outside (no error
	// of its own), the log still shows until when it was running.
	go func() {
		for i := 1; i <= 5; i++ {
			time.Sleep(3 * time.Second)
			log.Printf("em execução (%d s)", 3*i)
		}
	}()
	defer w.Destroy()
	hwnd := uintptr(w.Window())
	winsys.OwnerWindow = hwnd
	darkTitleBar(hwnd)
	// The minimum size never exceeds the usable screen (a 1366×768 laptop
	// at 125 % or a 1080p screen at 150 % is shorter than 680 × scale).
	minW, minH := fitToWorkArea(int(1024*scale), int(680*scale))
	w.SetSize(minW, minH, webview2.HintMin)
	// The token only exists inside this window: it is injected at document
	// creation and only for the Axion origin.
	w.Init(fmt.Sprintf(`if (location.origin === %q) { window.__AXION__ = Object.freeze({ token: %q }); }`, "http://"+srv.Host, srv.Token))
	w.Navigate(srv.URL() + "#/painel")
	w.Run()
	log.Printf("janela fechada")
	end()
	log.Printf("Axion encerrado")
}

// webView2LoaderSHA256 is the hash of the WebView2Loader.dll (WebView2 SDK,
// x64, signed by Microsoft Corporation) shipped next to Axion.exe.
const webView2LoaderSHA256 = "c4674acf95f0800793a4a6d61132adf5dfa694c218e482d86093494c4e84100a"

func fileSHA256(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// prepareWebView2Loader loads the WebView2 loader from the protected data
// folder (only administrators can write there): the copy shipped next to
// Axion.exe is checked by hash and copied there first. A DLL planted in the
// download folder is therefore never loaded into this elevated process, and
// nothing is loaded from memory.
func prepareWebView2Loader(dataDir string) error {
	dst := filepath.Join(dataDir, "bin", "WebView2Loader.dll")
	if fileSHA256(dst) != webView2LoaderSHA256 {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		src := filepath.Join(filepath.Dir(exe), "WebView2Loader.dll")
		b, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("WebView2Loader.dll não está junto do Axion.exe: %w", err)
		}
		if sum := sha256.Sum256(b); hex.EncodeToString(sum[:]) != webView2LoaderSHA256 {
			return errors.New("o WebView2Loader.dll junto do Axion.exe não é o publicado com esta versão")
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		tmp := dst + ".tmp"
		if err := os.WriteFile(tmp, b, 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, dst); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		if fileSHA256(dst) != webView2LoaderSHA256 {
			return errors.New("a cópia do WebView2Loader.dll não confere")
		}
	}
	_, err := windows.LoadLibraryEx(dst, 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH)
	return err
}

// shutdown ends the session when the interface closes. The local API stops
// taking requests first, so nothing new starts and a quick change already
// inside a request finishes. Tasks that only read, measure or download are
// cancelled; tasks that change the machine finish first, so their history
// entry is saved (up to 5 minutes). Meanwhile a new launch is told to wait
// instead of being offered to end this copy, and a notice explains the wait
// if it takes more than a moment.
func shutdown(a *app.App, srv *app.Server) {
	if name, err := windows.UTF16PtrFromString(finishingName); err == nil {
		if h, err := createNamed(func(sa *windows.SecurityAttributes) (windows.Handle, error) {
			return windows.CreateEvent(sa, 1, 0, name)
		}); h != 0 {
			defer windows.CloseHandle(h)
		} else {
			log.Printf("aviso de encerramento: %v", err)
		}
	}
	if err := srv.Shutdown(15 * time.Second); err != nil {
		log.Printf("encerrando a interface local: %v", err)
	}
	// The match report ends with the window: what was recorded is saved. No
	// system dialog tells the user; the report is waiting at the next start.
	if !a.StopSessions(30 * time.Second) {
		log.Printf("relatório da partida: a gravação não terminou em 30 s; encerrando mesmo assim")
	}
	j, running := a.Jobs.Running()
	if !running && !a.Jobs.Held() {
		return
	}
	if running {
		log.Printf("interface fechada com uma tarefa em andamento: %s (%.0f%%)", j.Title, j.Progress*100)
	} else {
		log.Printf("interface fechada durante uma alteração rápida")
	}
	done := a.Shutdown(5*time.Minute, func(title string) {
		// The notice closes with the process when the task ends.
		go messageBox("O Axion está terminando uma tarefa que estava em andamento ("+title+") e fecha sozinho em seguida.\n\n"+
			"Não desligue o PC até ele fechar.", 0x40)
	})
	if done {
		log.Printf("tarefa terminada; encerrando")
	} else {
		log.Printf("a tarefa não terminou em 5 minutos; encerrando mesmo assim")
	}
}

// browserSession runs the interface in the default browser until the user
// closes the notice. Launching Axion again meanwhile opens a new tab.
func browserSession(srv *app.Server, why string) {
	stop := watchReopen(srv)
	defer stop()
	if err := openInBrowser(srv); err != nil {
		fatal("Não foi possível abrir a interface: %v", err)
	}
	messageBox("O Axion abriu no seu navegador porque "+why+".\n\n"+
		"Mantenha esta mensagem aberta enquanto usa o Axion. Clique em OK para encerrar o programa. "+
		"Se fechar a aba, abra o Axion de novo pelo atalho: ele abre outra aba.\n\n"+
		"Para usar a janela própria do Axion, mantenha o WebView2Loader.dll junto do Axion.exe e o WebView2 Runtime instalado: https://developer.microsoft.com/microsoft-edge/webview2/", 0x40)
}

// watchReopen opens a new browser tab each time another launch of Axion
// signals the reopen event. The returned function stops watching.
func watchReopen(srv *app.Server) (stop func()) {
	name, err := windows.UTF16PtrFromString(reopenName)
	if err != nil {
		return func() {}
	}
	ev, err := createNamed(func(sa *windows.SecurityAttributes) (windows.Handle, error) {
		return windows.CreateEvent(sa, 0, 0, name)
	}) // auto-reset
	if ev == 0 {
		log.Printf("evento de reabertura: %v", err)
		return func() {}
	}
	if err == windows.ERROR_ALREADY_EXISTS && !trustedObject(ev) {
		// Created by a program without administrator rights: not ours.
		log.Printf("evento de reabertura criado por outro programa: ignorado")
		windows.CloseHandle(ev)
		return func() {}
	}
	quit, err := windows.CreateEvent(nil, 1, 0, nil)
	if quit == 0 {
		log.Printf("evento de reabertura: %v", err)
		windows.CloseHandle(ev)
		return func() {}
	}
	go func() {
		defer windows.CloseHandle(ev)
		defer windows.CloseHandle(quit)
		for {
			r, err := windows.WaitForMultipleObjects([]windows.Handle{ev, quit}, false, windows.INFINITE)
			if err != nil || r != windows.WAIT_OBJECT_0 {
				return
			}
			log.Printf("o Axion foi aberto de novo: abrindo outra aba no navegador")
			if err := openInBrowser(srv); err != nil {
				log.Printf("reabrir no navegador: %v", err)
			}
		}
	}()
	return func() { _ = windows.SetEvent(quit) }
}

// openInBrowser opens the interface in the default browser with a one-time
// code (in the URL fragment, which is never sent to the server): the page
// trades it for the token, so the address saved in the browser history
// holds nothing reusable.
func openInBrowser(srv *app.Server) error {
	code, err := srv.BootCode()
	if err != nil {
		return err
	}
	win, err := windows.GetWindowsDirectory() // not %SystemRoot% (user-settable)
	if err != nil {
		return err
	}
	return exec.Command(filepath.Join(win, "explorer.exe"), srv.URL()+"#b="+code).Start()
}

// signalReopen asks a copy of Axion running in the browser to open a new
// tab. It reports whether such a copy exists.
func signalReopen() bool {
	name, _ := windows.UTF16PtrFromString(reopenName)
	h, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE|windows.READ_CONTROL, false, name)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	if !trustedObject(h) {
		log.Printf("evento de reabertura criado por outro programa: ignorado")
		return false
	}
	return windows.SetEvent(h) == nil
}

// namedEventExists reports whether another copy of Axion holds the named
// event (one created by a program without administrator rights does not count).
func namedEventExists(n string) bool {
	name, _ := windows.UTF16PtrFromString(n)
	h, err := windows.OpenEvent(windows.SYNCHRONIZE|windows.READ_CONTROL, false, name)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	return trustedObject(h)
}

// crashFile keeps crash.log open for the life of the process: it is the
// process's standard error handle, where the Go runtime prints every fatal
// report.
var crashFile *os.File

// setupCrashOutput sends fatal runtime errors (which bypass recover) to
// crash.log in the protected data folder. The file becomes the process's
// standard error handle: that is where the Go runtime prints every fatal
// report, including an exception raised inside a Windows DLL called by Axion
// (what ended 0.5.0–0.5.2 during the scan), which debug.SetCrashOutput does
// not receive because the runtime never marks such an exception as a panic. A
// report left by the previous run is renamed and summarized, to be shown once.
func setupCrashOutput(dir string) (prev, prevFile string) {
	path := filepath.Join(dir, "crash.log")
	// Only a real report is kept and announced: WebView2 also writes its own
	// log lines to the standard error (at shutdown, for instance), and those
	// are discarded with the truncation below.
	if b, err := os.ReadFile(path); err == nil && hasCrashReport(string(b)) {
		prevFile = filepath.Join(dir, "crash-"+time.Now().Format("20060102-150405")+".log")
		if os.Rename(path, prevFile) != nil {
			prevFile = path
		}
		prev = crashSummary(string(b))
		if len(b) > 1200 {
			b = b[:1200]
		}
		prev += "\n\n" + strings.TrimSpace(string(b))
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		log.Printf("crash.log: %v", err)
		return prev, prevFile
	}
	if err := windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd())); err != nil {
		// Without the handle switch, only Go panics can be captured.
		log.Printf("crash.log: %v", err)
		if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
			log.Printf("crash.log: %v", err)
		}
		_ = f.Close()
		return prev, prevFile
	}
	os.Stderr = f
	crashFile = f
	return prev, prevFile
}

// programData is the machine-wide data folder, from the known-folder API
// (not %ProgramData%: user environment variables, which a program without
// administrator rights can set, are inherited by the elevated Axion and
// could point its protected data folder somewhere the user controls).
func programData() string {
	if v, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0); err == nil && v != "" {
		return v
	}
	if win, err := windows.GetWindowsDirectory(); err == nil {
		return filepath.VolumeName(win) + `\ProgramData`
	}
	return `C:\ProgramData`
}

func localAppData() string {
	if v, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0); err == nil && v != "" {
		return v
	}
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return v
	}
	return os.TempDir()
}

func setupLog(dir string) {
	path := filepath.Join(dir, "axion.log")
	if st, err := os.Stat(path); err == nil && st.Size() > 4<<20 {
		_ = os.Remove(path + ".1")
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(f)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
}

func fatal(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	messageBox(msg, 0x10)
	os.Exit(1)
}

// askYesNo shows a Yes/No question (MB_YESNO | MB_ICONQUESTION).
func askYesNo(text string) bool {
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString(windowTitle)
	r, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x4|0x20)
	return r == 6 // IDYES
}

// lockHolder finds the copy of Axion that holds the single-instance lock:
// the oldest other process of this same executable (later launches exit or
// only show a message). It also tells whether that copy runs in this Windows
// session. Only one Axion runs per machine, since it changes machine-wide
// settings, but a copy in another session (another user signed in on the
// same PC) cannot be shown or reopened from here and is never ended from here.
func lockHolder() (pid uint32, sameSession, ok bool) {
	self := uint32(os.Getpid())
	exe, err := os.Executable()
	if err != nil {
		return 0, false, false
	}
	var selfSession uint32
	if windows.ProcessIdToSessionId(self, &selfSession) != nil {
		return 0, false, false
	}
	base := strings.ToLower(filepath.Base(exe))
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, false, false
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	var oldest int64
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if pe.ProcessID == self || strings.ToLower(windows.UTF16ToString(pe.ExeFile[:])) != base {
			continue
		}
		ph, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pe.ProcessID)
		if err != nil {
			continue
		}
		var created, exited, kernel, user windows.Filetime
		terr := windows.GetProcessTimes(ph, &created, &exited, &kernel, &user)
		windows.CloseHandle(ph)
		if terr != nil {
			continue
		}
		if t := created.Nanoseconds(); !ok || t < oldest {
			var sess uint32
			if windows.ProcessIdToSessionId(pe.ProcessID, &sess) != nil {
				continue
			}
			pid, sameSession, ok, oldest = pe.ProcessID, sess == selfSession, true, t
		}
	}
	return pid, sameSession, ok
}

func messageBox(text string, flags uintptr) {
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString(windowTitle)
	_, _, _ = procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), flags)
}

// lockUnavailable explains that the single-instance lock could not be used.
// Only another program (without administrator rights) creating an object
// with the same name leads here; running without the lock could let two
// copies change the machine at once, so Axion does not open.
func lockUnavailable(err error) {
	log.Printf("trava de cópia única indisponível: %v", err)
	messageBox("O Axion não conseguiu usar a trava que impede duas cópias abertas ao mesmo tempo ("+mutexName+").\n\n"+
		"Se uma versão anterior do Axion estiver aberta, feche-a e abra de novo. Senão, outro programa sem permissão de administrador criou um objeto com esse nome: "+
		"reinicie o PC e, se a mensagem voltar, verifique o PC com o antivírus.", 0x30)
}

// singleInstance returns false (after focusing the running window) when
// Axion is already open.
func singleInstance() bool {
	name, _ := windows.UTF16PtrFromString(mutexName)
	h, err := createNamed(func(sa *windows.SecurityAttributes) (windows.Handle, error) {
		return windows.CreateMutex(sa, false, name)
	})
	if h == 0 {
		lockUnavailable(err)
		return false
	}
	if err != windows.ERROR_ALREADY_EXISTS {
		// The handle stays open for the life of the process.
		return true
	}
	trusted := trustedObject(h)
	windows.CloseHandle(h)
	if !trusted {
		// A lock not owned by an administrator was planted by another
		// program: never mistake it for a running copy.
		lockUnavailable(errors.New("dono do objeto não é administrador"))
		return false
	}
	cls, _ := windows.UTF16PtrFromString("webview")
	title, _ := windows.UTF16PtrFromString(windowTitle)
	if hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title))); hwnd != 0 {
		if iconic, _, _ := procIsIconic.Call(hwnd); iconic != 0 {
			_, _, _ = procShowWindow.Call(hwnd, 9) // SW_RESTORE
		}
		_, _, _ = procSetForegroundWindow.Call(hwnd)
		return false
	}
	// The lock is machine-wide: the copy may belong to another user signed
	// in on this PC. Its window, browser tab and tasks are out of reach from
	// here, and it must not be ended from this session.
	pid, mine, found := lockHolder()
	if found && !mine {
		log.Printf("o Axion já está aberto em outra sessão do Windows (processo %d)", pid)
		messageBox("O Axion já está aberto em outra sessão de usuário deste PC.\n\n"+
			"Só uma cópia pode rodar por vez, porque ele muda configurações do computador inteiro. Feche o Axion na outra sessão e abra de novo aqui.", 0x40)
		return false
	}
	// No window: the interface may be open in the browser (a new tab is
	// opened there), or the copy is finishing a task after its window closed
	// (it must not be ended in the middle).
	if signalReopen() {
		log.Printf("o Axion já está aberto no navegador: pedida uma nova aba")
		return false
	}
	if namedEventExists(finishingName) {
		messageBox("O Axion está terminando uma tarefa que estava em andamento quando a janela foi fechada.\n\n"+
			"Aguarde ele fechar sozinho (alguns segundos ou minutos) e abra de novo.", 0x40)
		return false
	}
	// The copy holds the lock but has no window: it is still starting or got
	// stuck. Never exit silently.
	log.Printf("outra cópia do Axion sem janela: processo %d (encontrada: %v)", pid, found)
	if !found {
		messageBox("Outra cópia do Axion está terminando de abrir ou de fechar. Aguarde alguns segundos e abra de novo.", 0x40)
		return false
	}
	if !askYesNo("O Axion já está em execução, mas sem janela (pode ter travado ao abrir).\n\nEncerrar essa cópia e abrir de novo?") {
		return false
	}
	if ph, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid); err == nil {
		_ = windows.TerminateProcess(ph, 1)
		_, _ = windows.WaitForSingleObject(ph, 5000)
		windows.CloseHandle(ph)
	}
	h, err = createNamed(func(sa *windows.SecurityAttributes) (windows.Handle, error) {
		return windows.CreateMutex(sa, false, name)
	})
	switch {
	case h == 0:
		lockUnavailable(err)
		return false
	case err == windows.ERROR_ALREADY_EXISTS:
		windows.CloseHandle(h)
		messageBox("Não foi possível encerrar a outra cópia do Axion. Feche-a pelo Gerenciador de Tarefas (Axion.exe) e abra de novo.", 0x30)
		return false
	}
	return true
}

func systemScale() float64 {
	if procGetDpiForSystem.Find() == nil {
		if dpi, _, _ := procGetDpiForSystem.Call(); dpi >= 96 {
			return float64(dpi) / 96
		}
	}
	return 1
}

type rect struct{ Left, Top, Right, Bottom int32 }

func fitToWorkArea(w, h int) (int, int) {
	var r rect
	const spiGetWorkArea = 0x0030
	if ok, _, _ := procSystemParametersInfo.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&r)), 0); ok != 0 {
		maxW, maxH := int(float64(r.Right-r.Left)*0.94), int(float64(r.Bottom-r.Top)*0.94)
		if w > maxW {
			w = maxW
		}
		if h > maxH {
			h = maxH
		}
	}
	return w, h
}

// darkTitleBar asks DWM for the dark caption (Windows 10 20H1+ / 11).
func darkTitleBar(hwnd uintptr) {
	if procDwmSetWindowAttr.Find() != nil {
		return
	}
	on := int32(1)
	for _, attr := range []uintptr{20, 19} { // DWMWA_USE_IMMERSIVE_DARK_MODE (and pre-20H1 value)
		if r, _, _ := procDwmSetWindowAttr.Call(hwnd, attr, uintptr(unsafe.Pointer(&on)), 4); r == 0 {
			return
		}
	}
}

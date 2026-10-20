//go:build !windows

// On non-Windows hosts the command runs the interface over the simulated
// machine (mocksys) for development, screenshots and demos. Nothing real is
// changed.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"axion/internal/app"
	"axion/internal/drivers"
	"axion/internal/sys/mocksys"
	"axion/internal/web"
)

func main() {
	dir := flag.String("data", "", "pasta de dados (padrão: temporária)")
	failDown := flag.String("net-fail-down", "", "simula a falha do download no teste de rede (mensagem)")
	flag.Parse()
	if *dir == "" {
		d, err := os.MkdirTemp("", "axion-preview-")
		if err != nil {
			log.Fatal(err)
		}
		*dir = d
	}
	m, err := mocksys.New(*dir)
	if err != nil {
		log.Fatal(err)
	}
	m.N.Net.FailDown = *failDown
	a, err := app.New(m.Platform, Version+"-preview")
	if err != nil {
		log.Fatal(err)
	}
	a.NvidiaAPI = fakeNvidia()
	srv, err := app.NewServer(a, web.FS())
	if err != nil {
		log.Fatal(err)
	}
	// Line 1: one-time link (valid for 2 minutes), as the Windows browser
	// fallback opens. Line 2: the token, for tools that inject it the way
	// the desktop window does (window.__AXION__).
	code, err := srv.BootCode()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s#b=%s\n", srv.URL(), code)
	fmt.Printf("token=%s\n", srv.Token)
	go func() {
		if err := srv.Serve(); err != nil {
			log.Fatal(err)
		}
	}()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
	_ = a.StopSessions(30 * time.Second)
	_ = srv.Close()
}

// fakeNvidia answers the NVIDIA driver lookup locally, so the preview never
// touches the network. The "installer" is a small blob the mock treats as
// signed by NVIDIA.
func fakeNvidia() app.NvidiaEndpoints {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	blob := []byte("MZ-SIGNED:NVIDIA Corporation\n" + strings.Repeat("x", 24<<20))
	mux := http.NewServeMux()
	mux.HandleFunc("/products", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<LookupValueSearch><LookupValues><LookupValue ParentID="120"><Name>GeForce RTX 3060</Name><Value>942</Value></LookupValue></LookupValues></LookupValueSearch>`)
	})
	mux.HandleFunc("/lookup", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"Success":"1","IDS":[{"downloadInfo":{"ID":"279803","Name":"GeForce%%20Game%%20Ready%%20Driver","Version":"617.14","IsBeta":"0","IsWHQL":"1","IsCRD":"0","ReleaseDateTime":"Tue Sep 22, 2026","DownloadURLFileSize":"990.85 MB","DetailsURL":"https://www.nvidia.com/en-us/drivers/details/279803/","DownloadURL":"%s/Windows/617.14/617.14-desktop-win10-win11-64bit-international-dch-whql.exe"}}]}`, base)
	})
	mux.HandleFunc("/Windows/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(blob)))
		_, _ = w.Write(blob)
	})
	go func() { _ = http.Serve(ln, mux) }()
	drivers.AllowedDownloadHost = func(h string) bool { return h == "127.0.0.1" }
	drivers.AllowedScheme = "http"
	return app.NvidiaEndpoints{ProductsURL: base + "/products", LookupURL: base + "/lookup"}
}

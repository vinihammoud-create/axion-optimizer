// Command axion-preview serves the interface over the simulated machine
// (mocksys) on any host, Windows included. It is a development tool for
// working on and screenshotting the interface: nothing real is read or
// changed. cmd/axion does the same on non-Windows hosts.
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

	"axion/internal/app"
	"axion/internal/drivers"
	"axion/internal/sys/mocksys"
	"axion/internal/web"
)

// Version labels the preview build.
var Version = "dev"

func main() {
	dir := flag.String("data", "", "pasta de dados (padrão: temporária)")
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
	// The CS2 page shows the preset card only with a cs2_video.txt: seed the
	// simulated one (inside the preview's own data folder, never a real file).
	if _, err := m.SeedCS2Video(mocksys.CS2VideoSample); err != nil {
		log.Fatal(err)
	}
	a, err := app.New(m.Platform, Version+"-preview")
	if err != nil {
		log.Fatal(err)
	}
	a.NvidiaAPI = fakeNvidia()
	srv, err := app.NewServer(a, web.FS())
	if err != nil {
		log.Fatal(err)
	}
	// A second loopback listener hands out a fresh one-time link per request,
	// so a script can open the page several times (each browser profile needs
	// its own code).
	devLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/code", func(w http.ResponseWriter, r *http.Request) {
		code, err := srv.BootCode()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "%s#b=%s", srv.URL(), code)
	})
	go func() { _ = http.Serve(devLn, mux) }()
	fmt.Printf("dev=http://%s/code\n", devLn.Addr())
	go func() {
		if err := srv.Serve(); err != nil {
			log.Fatal(err)
		}
	}()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
	_ = srv.Close()
}

// fakeNvidia answers the NVIDIA driver lookup locally, so the preview never
// touches the network.
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

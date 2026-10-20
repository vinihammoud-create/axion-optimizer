package netprobe

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"axion/internal/sys"
)

func TestBuildAndParseDNS(t *testing.T) {
	q, err := BuildQuery(0xBEEF, "api.steampowered.com.")
	if err != nil {
		t.Fatal(err)
	}
	want := "\x03api\x0csteampowered\x03com\x00\x00\x01\x00\x01"
	if string(q[12:]) != want || binary.BigEndian.Uint16(q) != 0xBEEF || q[2] != 0x01 {
		t.Fatalf("consulta: %q", q)
	}
	if _, err := BuildQuery(1, "a..b"); err == nil {
		t.Error("rótulo vazio deveria falhar")
	}
	if _, _, ok := ParseReply(q); ok {
		t.Error("uma consulta não é resposta")
	}
	r := append([]byte(nil), q...)
	r[2] |= 0x80 // QR
	r[3] = 0x83  // RA + NXDOMAIN
	if id, rc, ok := ParseReply(r); !ok || id != 0xBEEF || rc != 3 {
		t.Errorf("resposta: %v %d %v", id, rc, ok)
	}
}

func TestDNSQueryLocal(t *testing.T) {
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	go func() {
		buf := make([]byte, 512)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			// A stray datagram with another ID first: must be ignored.
			stray := append([]byte(nil), buf[:n]...)
			stray[0] ^= 0xFF
			stray[2] |= 0x80
			_, _ = pc.WriteTo(stray, addr)
			resp := append([]byte(nil), buf[:n]...)
			resp[2] |= 0x80
			_, _ = pc.WriteTo(resp, addr)
		}
	}()
	p := New(nil)
	p.DNSPort = pc.LocalAddr().(*net.UDPAddr).Port
	// A loopback round trip can read 0 on the coarse default clock of Windows:
	// a fake clock that ticks 1 ms per reading keeps the check deterministic.
	var tick time.Duration
	p.Clock = func() time.Duration { tick += time.Millisecond; return tick }
	rtt, rc, err := p.DNSQuery(context.Background(), "127.0.0.1", "example.com", 2*time.Second)
	if err != nil || rc != 0 || rtt <= 0 {
		t.Fatalf("DNS local: %v %d %v", rtt, rc, err)
	}
	if _, _, err := p.DNSQuery(context.Background(), "não-é-ip", "example.com", time.Second); err == nil {
		t.Error("endereço inválido")
	}
}

func TestLoadAndSDR(t *testing.T) {
	var ups atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/__down", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("bytes"))
		w.Header().Set("Content-Length", strconv.Itoa(n))
		buf := make([]byte, 64<<10)
		for n > 0 && r.Context().Err() == nil {
			k := len(buf)
			if k > n {
				k = n
			}
			if _, err := w.Write(buf[:k]); err != nil {
				return
			}
			n -= k
		}
	})
	mux.HandleFunc("/__up", func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		ups.Add(n)
	})
	mux.HandleFunc("/sdr", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("appid") != "730" {
			http.Error(w, "x", 400)
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"pops":{}}`))
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	p := New(nil)
	p.EP = Endpoints{SDRConfig: srv.URL + "/sdr?appid=%d", Down: srv.URL + "/__down?bytes=%d", Up: srv.URL + "/__up"}
	p.TLSConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig
	b, err := p.SDRConfig(context.Background(), 730)
	if err != nil || string(b) != `{"success":true,"pops":{}}` {
		t.Fatalf("SDR: %s %v", b, err)
	}
	var seen atomic.Int64
	st, err := p.Load(context.Background(), sys.LoadDownload, 3, 400*time.Millisecond, func(n int64) { seen.Add(n) })
	if err != nil || st.Bytes <= 0 || st.Bytes != seen.Load() || st.Streams != 3 {
		t.Fatalf("download: %+v %v (visto %d)", st, err, seen.Load())
	}
	if st.Elapsed > 3*time.Second {
		t.Errorf("a fase deveria terminar no prazo: %v", st.Elapsed)
	}
	st, err = p.Load(context.Background(), sys.LoadUpload, 2, 400*time.Millisecond, nil)
	if err != nil || st.Bytes <= 0 {
		t.Fatalf("upload: %+v %v", st, err)
	}
	// A dead server reports the error instead of a zero speed.
	p.EP.Down = "https://127.0.0.1:1/__down?bytes=%d"
	if _, err := p.Load(context.Background(), sys.LoadDownload, 1, 300*time.Millisecond, nil); err == nil {
		t.Error("sem servidor deveria falhar")
	}
}

// A server that refuses (429) ends the phase at once with the reason, instead
// of every stream retrying against a limit already reached.
func TestLoadRefusedByServer(t *testing.T) {
	var hits atomic.Int64
	var served atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/__down", func(w http.ResponseWriter, r *http.Request) {
		// The first request goes through; the next ones hit the limit.
		if served.Add(1) == 1 {
			_, _ = w.Write(make([]byte, 1<<20))
			return
		}
		hits.Add(1)
		w.Header().Set("Retry-After", "300")
		http.Error(w, "too many", http.StatusTooManyRequests)
	})
	mux.HandleFunc("/__up", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "no", http.StatusForbidden)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	p := New(nil)
	p.EP = Endpoints{Down: srv.URL + "/__down?bytes=%d", Up: srv.URL + "/__up"}
	p.TLSConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig

	start := time.Now()
	st, err := p.Load(context.Background(), sys.LoadDownload, 1, 3*time.Second, nil)
	var se *StatusError
	if err == nil || !errors.As(err, &se) || se.Code != 429 {
		t.Fatalf("429 precisa virar erro mesmo com dados já recebidos: %+v %v", st, err)
	}
	if st.Bytes == 0 {
		t.Errorf("os bytes recebidos antes da recusa continuam contados: %+v", st)
	}
	if !strings.Contains(err.Error(), "limite de testes seguidos") || !strings.Contains(err.Error(), "5 minutos") {
		t.Errorf("mensagem: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("a recusa deveria encerrar a fase na hora: %v", time.Since(start))
	}
	hits.Store(0)
	if _, err := p.Load(context.Background(), sys.LoadUpload, 4, 3*time.Second, nil); err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("403: %v", err)
	}
	if n := hits.Load(); n > 4 {
		t.Errorf("cada conexão deveria desistir na primeira recusa: %d pedidos", n)
	}
}

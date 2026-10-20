// Package netprobe is the internet side of the Rede page: Valve's relay list,
// link saturation for the bufferbloat test and DNS timing. It only runs when
// the user starts a network test.
package netprobe

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"axion/internal/sys"
)

// Endpoints are the public services the probe talks to.
type Endpoints struct {
	SDRConfig string // Steam Web API, "%d" = app id
	Down      string // Cloudflare speed test download, "%d" = bytes
	Up        string // Cloudflare speed test upload (POST)
}

// DefaultEndpoints: the Steam Web API (relay list) and Cloudflare's public
// speed test (the same endpoints speed.cloudflare.com uses).
var DefaultEndpoints = Endpoints{
	SDRConfig: "https://api.steampowered.com/ISteamApps/GetSDRConfig/v1/?appid=%d",
	Down:      "https://speed.cloudflare.com/__down?bytes=%d",
	Up:        "https://speed.cloudflare.com/__up",
}

// Probe implements sys.NetProbe with the Go network stack.
type Probe struct {
	EP    Endpoints
	Clock func() time.Duration // high-resolution clock (DNS timing)
	// TLSConfig and DNSPort are only set by tests (local servers).
	TLSConfig *tls.Config
	DNSPort   int
}

// New returns a probe using the default endpoints and the given clock.
func New(clock func() time.Duration) *Probe {
	return &Probe{EP: DefaultEndpoints, Clock: clock}
}

var _ sys.NetProbe = (*Probe)(nil)

const userAgent = "AxionOptimizer (teste de rede)"

// client builds an HTTP/1.1 client with one connection per stream (HTTP/2
// would multiplex every stream over a single TCP connection, which does not
// load the link the way parallel downloads do) and no proxy: the test
// measures the direct path.
func (p *Probe) client(conns int, timeout time.Duration) *http.Client {
	tr := &http.Transport{
		Proxy:               nil,
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		MaxConnsPerHost:     conns,
		MaxIdleConnsPerHost: conns,
		ForceAttemptHTTP2:   false,
		TLSNextProto:        map[string]func(string, *tls.Conn) http.RoundTripper{}, // HTTP/1.1 only
		DisableCompression:  true,
		TLSClientConfig:     p.TLSConfig,
	}
	return &http.Client{Transport: tr, Timeout: timeout}
}

// SDRConfig implements sys.NetProbe.
func (p *Probe) SDRConfig(ctx context.Context, appID int) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(p.EP.SDRConfig, appID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	c := p.client(1, 15*time.Second)
	defer c.CloseIdleConnections()
	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Steam Web API respondeu %s", res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 4<<20))
}

// StatusError is an answer other than 200 from the speed-test server.
type StatusError struct {
	Dir        string // download, upload
	Code       int
	Status     string // "429 Too Many Requests"
	RetryAfter string // Retry-After header, when sent
}

func (e *StatusError) Error() string {
	msg := fmt.Sprintf("o servidor de teste (Cloudflare) recusou o %s com HTTP %s", e.Dir, e.Status)
	switch e.Code {
	case http.StatusTooManyRequests:
		// RFC 6585: the client sent too many requests in a given time.
		msg += ": limite de testes seguidos atingido; "
		if s, err := strconv.Atoi(strings.TrimSpace(e.RetryAfter)); err == nil && s > 0 {
			if s >= 120 {
				msg += fmt.Sprintf("tente de novo em %d minutos", (s+59)/60)
			} else {
				msg += fmt.Sprintf("tente de novo em %d s", s)
			}
		} else {
			msg += "espere alguns minutos e repita o teste"
		}
	case http.StatusForbidden:
		msg += ": o serviço bloqueou o pedido; repita mais tarde"
	}
	return msg
}

// Refused reports whether the server turned the request down (4xx): trying
// again at once only adds requests to a limit already reached.
func (e *StatusError) Refused() bool { return e.Code >= 400 && e.Code < 500 }

// Wait is how long the server asked to wait (Retry-After, in seconds or as
// an HTTP date; RFC 9110), or 0 when it did not say.
func (e *StatusError) Wait() time.Duration {
	v := strings.TrimSpace(e.RetryAfter)
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(v); err == nil {
		if s <= 0 {
			return 0
		}
		if s > 24*3600 {
			s = 24 * 3600
		}
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 && d <= 24*time.Hour {
			return d
		}
	}
	return 0
}

func statusErr(dir string, res *http.Response) error {
	return &StatusError{Dir: dir, Code: res.StatusCode, Status: res.Status, RetryAfter: res.Header.Get("Retry-After")}
}

// Load implements sys.NetProbe. Each stream repeats requests until the time
// is up; the byte count excludes headers. A refusal from the server (4xx)
// ends the whole phase at once and is returned even when some data moved,
// because the link was not full for the time the measurement needs.
func (p *Probe) Load(ctx context.Context, dir sys.LoadDir, streams int, d time.Duration, onBytes func(n int64)) (sys.LoadStats, error) {
	if streams < 1 {
		streams = 1
	}
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	c := p.client(streams, 0)
	defer c.CloseIdleConnections()
	var total atomic.Int64
	count := func(n int64) {
		total.Add(n)
		if onBytes != nil {
			onBytes(n)
		}
	}
	start := time.Now()
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr, refused error
	for i := 0; i < streams; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				var err error
				if dir == sys.LoadDownload {
					err = p.download(ctx, c, count)
				} else {
					err = p.upload(ctx, c, count)
				}
				if err != nil && ctx.Err() == nil {
					var se *StatusError
					if errors.As(err, &se) && se.Refused() {
						errMu.Lock()
						if refused == nil {
							refused = err
						}
						errMu.Unlock()
						cancel() // stop every stream: the server said no
						return
					}
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
					select { // back off a little before retrying
					case <-ctx.Done():
					case <-time.After(300 * time.Millisecond):
					}
				}
			}
		}()
	}
	wg.Wait()
	st := sys.LoadStats{Bytes: total.Load(), Elapsed: time.Since(start), Streams: streams, Server: "speed.cloudflare.com"}
	if refused != nil {
		return st, refused
	}
	if st.Bytes == 0 {
		if firstErr != nil {
			return st, firstErr
		}
		return st, errors.New("nenhum dado transferido")
	}
	return st, nil
}

// chunk is the size of each download request: large enough to keep the
// connection busy, small enough to adapt to slow links.
const chunk = 25_000_000

func (p *Probe) download(ctx context.Context, c *http.Client, count func(int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(p.EP.Down, chunk), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return statusErr("download", res)
	}
	buf := make([]byte, 64<<10)
	for {
		n, err := res.Body.Read(buf)
		if n > 0 {
			count(int64(n))
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func (p *Probe) upload(ctx context.Context, c *http.Client, count func(int64)) error {
	body := &uploadBody{ctx: ctx, left: chunk, count: count}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.EP.Up, body)
	if err != nil {
		return err
	}
	req.ContentLength = chunk
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("User-Agent", userAgent)
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return statusErr("upload", res)
	}
	return nil
}

// uploadBody streams incompressible bytes and counts what the connection
// accepted. It ends early (with an error) when the phase is over.
type uploadBody struct {
	ctx   context.Context
	left  int64
	count func(int64)
}

var noise = func() []byte {
	b := make([]byte, 64<<10)
	_, _ = rand.Read(b)
	return b
}()

func (u *uploadBody) Read(p []byte) (int, error) {
	if err := u.ctx.Err(); err != nil {
		return 0, err
	}
	if u.left <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > len(noise) {
		n = len(noise)
	}
	if int64(n) > u.left {
		n = int(u.left)
	}
	copy(p, noise[:n])
	u.left -= int64(n)
	// Counted when handed to the connection: the socket buffer is small
	// next to the seconds of transfer, so the error is negligible.
	u.count(int64(n))
	return n, nil
}

// DNSQuery implements sys.NetProbe.
func (p *Probe) DNSQuery(ctx context.Context, server, name string, timeout time.Duration) (time.Duration, int, error) {
	ip := net.ParseIP(server)
	if ip == nil {
		return 0, -1, fmt.Errorf("servidor DNS inválido: %s", server)
	}
	var idb [2]byte
	_, _ = rand.Read(idb[:])
	id := binary.BigEndian.Uint16(idb[:])
	q, err := BuildQuery(id, name)
	if err != nil {
		return 0, -1, err
	}
	d := net.Dialer{Timeout: timeout}
	port := 53
	if p.DNSPort > 0 {
		port = p.DNSPort
	}
	conn, err := d.DialContext(ctx, "udp", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	if err != nil {
		return 0, -1, err
	}
	defer conn.Close()
	deadline := time.Now().Add(timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)
	clock := p.Clock
	if clock == nil {
		start := time.Now()
		clock = func() time.Duration { return time.Since(start) }
	}
	t0 := clock()
	if _, err := conn.Write(q); err != nil {
		return 0, -1, err
	}
	buf := make([]byte, 1500)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return 0, -1, err
		}
		rid, rcode, ok := ParseReply(buf[:n])
		if !ok || rid != id {
			continue // stray or spoofed datagram: keep waiting for ours
		}
		return clock() - t0, rcode, nil
	}
}

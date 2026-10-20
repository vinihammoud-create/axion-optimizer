package mocksys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"axion/internal/sys"
)

// NetSim is the simulated network of the demo machine: a wired PC behind a
// home router, a provider that uses CGNAT, Cloudflare about 9 ms away and
// Valve relays by city. While a load phase runs, latency to the internet
// grows (bufferbloat), as on a real congested link. Relay addresses use the
// documentation ranges (RFC 5737), never real servers.
type NetSim struct {
	mu      sync.Mutex
	loading bool
	dir     sys.LoadDir
	seq     int

	DownMbps, UpMbps   float64
	BloatDown, BloatUp float64 // extra ms under load
	Offline            bool    // the internet side fails (SDR, load, DNS)
	// FailDown and FailUp make one direction of the load test fail with
	// this message (a speed-test server that refuses, say).
	FailDown, FailUp string
	// Loads counts the saturation phases run.
	Loads int
	// DNSDown lists resolvers that never answer (to provoke dns.fail).
	DNSDown map[string]bool
}

// NewNetSim returns the default demo network.
func NewNetSim() *NetSim {
	return &NetSim{DownMbps: 480, UpMbps: 240, BloatDown: 22, BloatUp: 48}
}

// Route of the demo machine: router, CGNAT gateway, provider core, IX.
var mockHops = []string{"192.168.0.1", "100.64.12.1", "187.16.216.10", "200.160.0.10"}

// mockPOPs are the demo relays: key, city, base RTT (ms), relay addresses.
var mockPOPs = []struct {
	key, desc string
	lon, lat  float64
	ms        float64
	ips       []string
}{
	{"gru", "Sao Paulo (Brazil)", -46.6, -23.5, 12, []string{"203.0.113.10", "203.0.113.11"}},
	{"eze", "Buenos Aires (Argentina)", -58.4, -34.6, 38, []string{"203.0.113.20"}},
	{"scl", "Santiago (Chile)", -70.7, -33.4, 46, []string{"203.0.113.30", "203.0.113.31"}},
	{"lim", "Lima (Peru)", -77.0, -12.0, 71, []string{"203.0.113.40"}},
	{"bog", "Bogota (Colombia)", -74.1, 4.7, 84, []string{"203.0.113.50"}},
	{"atl", "Atlanta (Georgia)", -84.4, 33.7, 118, []string{"198.51.100.10"}},
	{"iad", "Sterling (Virginia)", -77.4, 39.0, 126, []string{"198.51.100.20", "198.51.100.21"}},
	{"dfw", "Dallas (Texas)", -96.8, 32.8, 139, []string{"198.51.100.30"}},
	{"lax", "Los Angeles (California)", -118.2, 34.1, 171, []string{"198.51.100.40"}},
	{"mad", "Madrid (Spain)", -3.7, 40.4, 196, []string{"192.0.2.10"}},
	{"fra", "Frankfurt (Germany)", 8.7, 50.1, 205, []string{"192.0.2.20"}},
	{"sgp", "Singapore", 103.8, 1.35, 330, []string{"192.0.2.30"}},
}

// profile returns base RTT (ms), jitter amplitude (ms), loss probability and
// whether the destination is on the internet (subject to bufferbloat).
func (s *NetSim) profile(ip string) (base, jit, loss float64, internet bool) {
	switch {
	case ip == "192.168.0.1" || strings.HasPrefix(ip, "192.168."):
		return 0.38, 0.12, 0, false
	case ip == "1.1.1.1" || ip == "1.0.0.1":
		return 8.6, 0.9, 0, true
	case ip == "8.8.8.8":
		return 11.2, 1.1, 0, true
	}
	for _, p := range mockPOPs {
		for _, a := range p.ips {
			if a == ip {
				l := 0.0
				if p.key == "gru" {
					l = 0.004 // 0.4%: a little loss on the path to the nearest relay
				}
				return p.ms, 0.8 + p.ms/60, l, true
			}
		}
	}
	return 60, 3, 0.01, true
}

// rnd is a small deterministic generator (0..1) from a sequence number.
func rnd(seq int, salt uint32) float64 {
	x := uint32(seq)*2654435761 ^ salt
	x ^= x >> 15
	x *= 2246822519
	x ^= x >> 13
	return float64(x%100000) / 100000
}

// Ping implements sys.Native over the simulated network.
func (n *Native) Ping(ip string, ttl int, size int, df bool, timeout time.Duration) (sys.PingReply, error) {
	s := n.Net
	s.mu.Lock()
	s.seq++
	seq, loading, dir := s.seq, s.loading, s.dir
	bloat := s.BloatDown
	if dir == sys.LoadUpload {
		bloat = s.BloatUp
	}
	s.mu.Unlock()
	if df && size > 1472 {
		return sys.PingReply{TooBig: true, Status: 11009, From: mockHops[0]}, nil
	}
	base, jit, loss, internet := s.profile(ip)
	dist := len(mockHops) + 1
	if !internet {
		dist = 1
	}
	if ttl > 0 && ttl < dist {
		// Routers answer "TTL expired" from their control plane: slower,
		// jittery and rate limited, which is why a hop that looks bad is
		// only a problem when the destination shows it too.
		hopMs := []float64{0.4, 2.8, 4.1, 6.0}[ttl-1]
		if ttl > 1 && rnd(seq, 7) < 0.06 {
			return sys.PingReply{Timeout: true, Status: 11010}, nil
		}
		extra := 0.0
		if ttl > 1 {
			extra = rnd(seq, 11) * 7
		}
		return sys.PingReply{TTLHop: true, From: mockHops[ttl-1], RTT: ms(hopMs + extra), Status: 11013}, nil
	}
	if rnd(seq, 3) < loss {
		return sys.PingReply{Timeout: true, Status: 11010}, nil
	}
	rtt := base + (rnd(seq, 5)-0.5)*2*jit
	if loading && internet {
		rtt += bloat * (0.55 + 0.9*rnd(seq, 9))
	}
	if loading && !internet {
		rtt += 0.3 * rnd(seq, 13)
	}
	if rtt < 0.05 {
		rtt = 0.05
	}
	if timeout > 0 && ms(rtt) > timeout {
		// Like IcmpSendEcho: a reply later than the timeout is a timeout.
		return sys.PingReply{Timeout: true, Status: 11010}, nil
	}
	return sys.PingReply{OK: true, From: ip, RTT: ms(rtt)}, nil
}

func ms(v float64) time.Duration { return time.Duration(v * float64(time.Millisecond)) }

// Monotonic implements sys.Native.
func (n *Native) Monotonic() time.Duration { return time.Since(n.start) }

// SDRConfig implements sys.NetProbe with the demo relays.
func (s *NetSim) SDRConfig(ctx context.Context, appID int) ([]byte, error) {
	s.mu.Lock()
	off := s.Offline
	s.mu.Unlock()
	if off {
		return nil, errors.New("sem conexão (simulado)")
	}
	var b strings.Builder
	b.WriteString(`{"revision":1,"success":true,"pops":{`)
	for i, p := range mockPOPs {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `%q:{"desc":%q,"geo":[%g,%g],"relays":[`, p.key, p.desc, p.lon, p.lat)
		for j, ip := range p.ips {
			if j > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `{"ipv4":%q,"port_range":[27015,27060]}`, ip)
		}
		b.WriteString("]}")
	}
	// A POP without relays (only reachable through partners) is ignored.
	b.WriteString(`,"xyz":{"desc":"Parceiro (sem relays)","geo":[0,0]}}}`)
	return []byte(b.String()), nil
}

// Load implements sys.NetProbe: it reports the simulated speed and makes
// Ping see the congestion while it runs.
func (s *NetSim) Load(ctx context.Context, dir sys.LoadDir, streams int, d time.Duration, onBytes func(n int64)) (sys.LoadStats, error) {
	s.mu.Lock()
	if s.Offline {
		s.mu.Unlock()
		return sys.LoadStats{}, errors.New("sem conexão (simulado)")
	}
	s.Loads++
	if fail := map[sys.LoadDir]string{sys.LoadDownload: s.FailDown, sys.LoadUpload: s.FailUp}[dir]; fail != "" {
		s.mu.Unlock()
		return sys.LoadStats{Streams: streams, Server: "simulado"}, errors.New(fail)
	}
	s.loading, s.dir = true, dir
	mbps := s.DownMbps
	if dir == sys.LoadUpload {
		mbps = s.UpMbps
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.loading = false
		s.mu.Unlock()
	}()
	const tick = 25 * time.Millisecond
	start := time.Now()
	var total int64
	for time.Since(start) < d {
		select {
		case <-ctx.Done():
			return sys.LoadStats{Bytes: total, Elapsed: time.Since(start), Streams: streams, Server: "simulado"}, nil
		case <-time.After(tick):
		}
		frac := 1.0
		if time.Since(start) < d/5 { // TCP ramp-up
			frac = 0.6
		}
		n := int64(mbps * 1e6 / 8 * tick.Seconds() * frac)
		total += n
		if onBytes != nil {
			onBytes(n)
		}
	}
	return sys.LoadStats{Bytes: total, Elapsed: time.Since(start), Streams: streams, Server: "simulado"}, nil
}

// mockDNS is the simulated response time (ms) per resolver.
var mockDNS = map[string]float64{"192.168.0.1": 21, "1.1.1.1": 9.5, "1.0.0.1": 10, "8.8.8.8": 24, "8.8.4.4": 25, "9.9.9.9": 31, "208.67.222.222": 47}

// DNSQuery implements sys.NetProbe with simulated resolvers.
func (s *NetSim) DNSQuery(ctx context.Context, server, name string, timeout time.Duration) (time.Duration, int, error) {
	s.mu.Lock()
	s.seq++
	seq, off, down := s.seq, s.Offline, s.DNSDown[server]
	s.mu.Unlock()
	if off {
		return 0, -1, errors.New("sem conexão (simulado)")
	}
	if down {
		return 0, -1, errors.New("tempo esgotado (simulado)")
	}
	base, ok := mockDNS[server]
	if !ok {
		base = 40
	}
	// The first lookup of a name misses the resolver's cache sometimes.
	if rnd(seq, 17) < 0.2 {
		base *= 3.5
	}
	if server == "9.9.9.9" && rnd(seq, 19) < 0.03 {
		return 0, -1, errors.New("tempo esgotado (simulado)")
	}
	return ms(base * (0.85 + 0.3*rnd(seq, 23))), 0, nil
}

var _ sys.NetProbe = (*NetSim)(nil)

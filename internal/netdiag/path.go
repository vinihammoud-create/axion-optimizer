package netdiag

import (
	"net"
)

// Address kinds.
const (
	KindPrivate   = "privado" // RFC 1918 (home and company networks)
	KindCGNAT     = "cgnat"   // RFC 6598 shared address space (carrier NAT)
	KindPublic    = "público" // routable on the internet
	KindLinkLocal = "local"   // 169.254/16, loopback
	KindUnknown   = "sem resposta"
)

var (
	rfc1918 = mustNets("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16")
	homeNet = mustNets("172.16.0.0/12", "192.168.0.0/16") // ranges home routers hand out
	cgnat   = mustNets("100.64.0.0/10")
	special = mustNets("127.0.0.0/8", "169.254.0.0/16", "0.0.0.0/8")
)

func mustNets(cidrs ...string) []*net.IPNet {
	var out []*net.IPNet
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}

func inAny(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ClassifyIP tells where an IPv4 address lives.
func ClassifyIP(s string) string {
	ip := net.ParseIP(s).To4()
	switch {
	case ip == nil:
		return KindUnknown
	case inAny(ip, special):
		return KindLinkLocal
	case inAny(ip, rfc1918):
		return KindPrivate
	case inAny(ip, cgnat):
		return KindCGNAT
	}
	return KindPublic
}

// IsPrivate reports whether an address is not routable on the internet.
func IsPrivate(s string) bool { k := ClassifyIP(s); return k != KindPublic && k != KindUnknown }

// Hop is one step of the path to the internet (TTL-limited probes).
type Hop struct {
	TTL  int     `json:"ttl"`
	Addr string  `json:"addr,omitempty"` // "" = no reply
	RTT  float64 `json:"rtt,omitempty"`  // ms (control-plane reply: only indicative)
	Kind string  `json:"kind"`
	Dest bool    `json:"dest,omitempty"` // reached the destination
}

// Path is what the route to the internet reveals.
type Path struct {
	Hops        []Hop  `json:"hops"`
	Gateway     string `json:"gateway,omitempty"`
	FirstPublic string `json:"firstPublic,omitempty"`
	PublicTTL   int    `json:"publicTtl,omitempty"`
	// HomeRouters counts hops in home-router ranges (192.168/16,
	// 172.16/12) before the internet: 2 or more means two routers doing
	// NAT in cascade (for example the provider's modem in router mode plus
	// your own router). 10/8 is left out: providers use it inside their
	// own network.
	HomeRouters int  `json:"homeRouters"`
	DoubleNAT   bool `json:"doubleNat"`
	// CGNAT: the provider shares one public address among customers
	// (RFC 6598 range on the path).
	CGNAT bool `json:"cgnat"`
}

// AnalyzePath reads the hops (ordered by TTL) of a route to a public host.
func AnalyzePath(hops []Hop) Path {
	p := Path{Hops: hops}
	for i := range p.Hops {
		h := &p.Hops[i]
		h.Kind = ClassifyIP(h.Addr)
		if h.Addr == "" {
			h.Kind = KindUnknown
		}
	}
	for _, h := range p.Hops {
		if h.Addr == "" {
			continue
		}
		if p.Gateway == "" && h.TTL == 1 {
			p.Gateway = h.Addr
		}
		switch h.Kind {
		case KindPublic:
			if p.FirstPublic == "" {
				p.FirstPublic, p.PublicTTL = h.Addr, h.TTL
			}
		case KindCGNAT:
			if p.FirstPublic == "" {
				p.CGNAT = true
			}
		case KindPrivate:
			if p.FirstPublic == "" && inAny(net.ParseIP(h.Addr).To4(), homeNet) {
				p.HomeRouters++
			}
		}
	}
	p.DoubleNAT = p.HomeRouters >= 2
	return p
}

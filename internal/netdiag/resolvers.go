package netdiag

import (
	"net"
	"regexp"
	"strings"
)

// Resolver is a public DNS service the test compares with the configured one
// and the Rede page can offer when the configured one fails or is slow.
type Resolver struct {
	Key  string // id used in the item "dns:{GUID}:<key>"
	Name string
	Of   string // "da Cloudflare", "do Google": for sentences in Portuguese
	V4   []string
	V6   []string
	// Source is the page of the service that lists these addresses.
	Source Source
}

// PublicResolvers is the one table of resolvers: the benchmark queries the
// first IPv4 address of each, and the DNS item writes the full lists. The
// addresses are those on the pages of the services (opened and read).
var PublicResolvers = []Resolver{
	{Key: "cloudflare", Name: "Cloudflare", Of: "da Cloudflare", V4: []string{"1.1.1.1", "1.0.0.1"}, V6: []string{"2606:4700:4700::1111", "2606:4700:4700::1001"},
		Source: Source{"Cloudflare — Endereços do 1.1.1.1 (IPv4 e IPv6)", "https://developers.cloudflare.com/1.1.1.1/ip-addresses/"}},
	{Key: "google", Name: "Google", Of: "do Google", V4: []string{"8.8.8.8", "8.8.4.4"}, V6: []string{"2001:4860:4860::8888", "2001:4860:4860::8844"},
		Source: Source{"Google Public DNS — Como usar (endereços IPv4 e IPv6)", "https://developers.google.com/speed/public-dns/docs/using"}},
	// The Quad9 page gives 2620:fe::9 as the second IPv6 address (the list of
	// known DoH servers of Microsoft Learn writes 2620:fe::fe:9).
	{Key: "quad9", Name: "Quad9", Of: "do Quad9", V4: []string{"9.9.9.9", "149.112.112.112"}, V6: []string{"2620:fe::fe", "2620:fe::9"},
		Source: Source{"Quad9 — Endereços do serviço (bloqueio de malware, DNSSEC)", "https://quad9.net/service/service-addresses-and-features/"}},
}

// ResolverByKey finds a resolver by its key.
func ResolverByKey(key string) (Resolver, bool) {
	for _, r := range PublicResolvers {
		if r.Key == key {
			return r, true
		}
	}
	return Resolver{}, false
}

// ResolverByAddr finds the resolver that owns an address (IPv4 or IPv6).
func ResolverByAddr(addr string) (Resolver, bool) {
	ip := net.ParseIP(strings.TrimSpace(addr))
	if ip == nil {
		return Resolver{}, false
	}
	for _, r := range PublicResolvers {
		for _, a := range append(append([]string(nil), r.V4...), r.V6...) {
			if ip.Equal(net.ParseIP(a)) {
				return r, true
			}
		}
	}
	return Resolver{}, false
}

var reGUID = regexp.MustCompile(`^\{[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\}$`)

// DNSTweakID names the item that sets the DNS of an adapter to a resolver:
// "dns:{GUID}:<key>" (IPv4 and IPv6) or "dns:{GUID}:<key>:v4" (IPv4 only, for
// a PC without IPv6 DNS: writing IPv6 servers there would make the next test
// blame the DNS for a path that does not exist).
func DNSTweakID(guid, key string, v4Only bool) string {
	id := "dns:" + guid + ":" + key
	if v4Only {
		id += ":v4"
	}
	return id
}

// ParseDNSTweakID reads the adapter and the resolver out of an item ID.
func ParseDNSTweakID(id string) (guid, key string, v4Only, ok bool) {
	rest, found := strings.CutPrefix(id, "dns:")
	if !found {
		return "", "", false, false
	}
	parts := strings.Split(rest, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return "", "", false, false
	}
	if len(parts) == 3 {
		if parts[2] != "v4" {
			return "", "", false, false
		}
		v4Only = true
	}
	guid, key = parts[0], parts[1]
	if !reGUID.MatchString(guid) {
		return "", "", false, false
	}
	if _, ok := ResolverByKey(key); !ok {
		return "", "", false, false
	}
	return guid, key, v4Only, true
}

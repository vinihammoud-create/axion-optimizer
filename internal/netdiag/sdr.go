package netdiag

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// POP is a Valve data center / relay site of the Steam Datagram Relay.
type POP struct {
	Key    string   `json:"key"`
	Desc   string   `json:"desc"`
	Lat    float64  `json:"lat"`
	Lon    float64  `json:"lon"`
	Relays []string `json:"relays"` // IPv4 addresses
}

// ParseSDR reads the Steam Web API answer of ISteamApps/GetSDRConfig: POPs
// without relays (reached only through partners) are left out.
func ParseSDR(b []byte) ([]POP, error) {
	var raw struct {
		Success bool `json:"success"`
		Pops    map[string]struct {
			Desc   string    `json:"desc"`
			Geo    []float64 `json:"geo"` // [lon, lat]
			Relays []struct {
				IPv4 string `json:"ipv4"`
			} `json:"relays"`
		} `json:"pops"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, errors.New("lista de relays da Valve em formato inesperado")
	}
	var out []POP
	for key, p := range raw.Pops {
		pop := POP{Key: key, Desc: strings.TrimSpace(p.Desc)}
		if len(p.Geo) == 2 {
			pop.Lon, pop.Lat = p.Geo[0], p.Geo[1]
		}
		for _, r := range p.Relays {
			if ClassifyIP(r.IPv4) == KindPublic {
				pop.Relays = append(pop.Relays, r.IPv4)
			}
		}
		if len(pop.Relays) > 0 {
			out = append(out, pop)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("a lista de relays da Valve veio vazia")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// City returns the readable part of a POP description ("Sao Paulo
// (Brazil)" -> "São Paulo") in Portuguese when known.
func (p POP) City() string {
	d := p.Desc
	if i := strings.Index(d, "("); i > 0 {
		d = strings.TrimSpace(d[:i])
	}
	if pt, ok := cityPT[d]; ok {
		return pt
	}
	if d == "" {
		return strings.ToUpper(p.Key)
	}
	return d
}

var cityPT = map[string]string{
	"Sao Paulo": "São Paulo", "Buenos Aires": "Buenos Aires", "Santiago": "Santiago", "Lima": "Lima", "Bogota": "Bogotá",
	"Sterling": "Sterling (EUA)", "Atlanta": "Atlanta (EUA)", "Dallas": "Dallas (EUA)", "Los Angeles": "Los Angeles (EUA)",
	"Seattle": "Seattle (EUA)", "Chicago": "Chicago (EUA)", "New York": "Nova York (EUA)", "Frankfurt": "Frankfurt",
	"Madrid": "Madri", "London": "Londres", "Paris": "Paris", "Amsterdam": "Amsterdã", "Stockholm": "Estocolmo",
	"Vienna": "Viena", "Warsaw": "Varsóvia", "Helsinki": "Helsinque", "Singapore": "Singapura", "Tokyo": "Tóquio",
	"Hong Kong": "Hong Kong", "Sydney": "Sydney", "Mumbai": "Mumbai", "Chennai": "Chennai", "Dubai": "Dubai",
	"Johannesburg": "Joanesburgo", "Seoul": "Seul",
}

// RelayProbe is the quick first look at every POP.
type RelayProbe struct {
	Key    string  `json:"key"`
	City   string  `json:"city"`
	Addr   string  `json:"addr"`
	BestMs float64 `json:"bestMs"` // best of the quick probes; Lost if none replied
}

// Nearest returns the n POPs with the lowest round trip among those that
// replied, in order.
func Nearest(probes []RelayProbe, n int) []RelayProbe {
	var ok []RelayProbe
	for _, p := range probes {
		if p.BestMs >= 0 {
			ok = append(ok, p)
		}
	}
	sort.SliceStable(ok, func(i, j int) bool { return ok[i].BestMs < ok[j].BestMs })
	if len(ok) > n {
		ok = ok[:n]
	}
	return ok
}

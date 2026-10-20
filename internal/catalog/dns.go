package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"axion/internal/facts"
	"axion/internal/netdiag"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// DNS of an adapter (0.9): the Rede page offers a public resolver only when
// the network test measured the configured DNS failing or slow. It is an
// execution item, built on demand from its ID ("dns:{GUID}:<resolver>", or
// "...:v4" for IPv4 only), so the choice is applied and undone by the engine:
// the DNS that was there (a static list, or automatic) is saved before the
// write and comes back exactly. It is never part of the one-click plan.
// A DNS does not change the ping (RFC 1034; see excluded.go).

// KeyDNSPolicy is where a group policy fixes the DNS servers of the PC.
const KeyDNSPolicy = `HKLM\SOFTWARE\Policies\Microsoft\Windows NT\DNSClient`

var (
	srcSetDNS    = tweaks.Source{Title: "Microsoft Learn — SetInterfaceDnsSettings (netioapi.h): grava os servidores DNS de um adaptador (Windows 10 build 19041 ou mais novo)", URL: "https://learn.microsoft.com/windows/win32/api/netioapi/nf-netioapi-setinterfacednssettings"}
	srcIpconfig  = tweaks.Source{Title: "Microsoft Learn — ipconfig: /flushdns limpa o cache do resolvedor de DNS do cliente", URL: "https://learn.microsoft.com/windows-server/administration/windows-commands/ipconfig"}
	dnsMinBuild  = 19041
	dnsFlushWait = 15 * time.Second
)

// dnsSnap is the DNS of an adapter before the change.
type dnsSnap struct {
	Present bool     `json:"present"`
	V4      []string `json:"v4"`
	V6      []string `json:"v6"`
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func containsAny(list, want []string) bool {
	for _, a := range list {
		for _, b := range want {
			if ia, ib := net.ParseIP(a), net.ParseIP(b); ia != nil && ib != nil && ia.Equal(ib) {
				return true
			}
		}
	}
	return false
}

func sameServers(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !net.ParseIP(a[i]).Equal(net.ParseIP(b[i])) {
			return false
		}
	}
	return true
}

func listText(s []string) string {
	if len(s) == 0 {
		return "automático"
	}
	return strings.Join(s, ", ")
}

func readDNS(c *tweaks.Ctx, guid string) (dnsSnap, sys.DNSConfig, error) {
	cfg, err := c.P.Native.DNSConfig(guid)
	if errors.Is(err, sys.ErrNotFound) {
		return dnsSnap{}, cfg, nil
	}
	if err != nil {
		return dnsSnap{}, cfg, err
	}
	return dnsSnap{Present: true, V4: orEmpty(cfg.V4), V6: orEmpty(cfg.V6)}, cfg, nil
}

// dnsOp sets the static DNS servers of an adapter (an empty list = automatic).
func dnsOp(guid string, v4, v6 []string) tweaks.Op {
	return &tweaks.FuncOp{
		KindName: "dns", ID: strings.ToLower(guid), Spec: dnsSpec{GUID: guid, V4: v4, V6: v6},
		Desc: fmt.Sprintf("Adaptador %s → DNS %s (IPv4) e %s (IPv6) (SetInterfaceDnsSettings); o DNS anterior fica guardado", guid, listText(v4), listText(v6)),
		SnapFn: func(c *tweaks.Ctx) (any, error) {
			s, _, err := readDNS(c, guid)
			return s, err
		},
		InFn: func(c *tweaks.Ctx) (bool, error) {
			s, _, err := readDNS(c, guid)
			if err != nil {
				return false, err
			}
			if !s.Present {
				return false, nil // ApplyFn says the adapter was not found
			}
			return sameServers(s.V4, v4) && sameServers(s.V6, v6), nil
		},
		ApplyFn: func(c *tweaks.Ctx) error {
			s, cfg, err := readDNS(c, guid)
			if err != nil {
				return err
			}
			if !s.Present {
				return fmt.Errorf("o adaptador %s não foi encontrado", guid)
			}
			if cfg.DoH {
				return errors.New("este adaptador usa DNS criptografado (DNS over HTTPS) configurado por você; o Axion não troca")
			}
			if v, err := c.P.Reg.GetValue(KeyDNSPolicy, "NameServer"); err == nil && v.Exists && strings.TrimSpace(v.Str) != "" {
				return errors.New("uma política de grupo fixa o DNS deste PC; o DNS do adaptador seria ignorado")
			}
			return c.P.Native.SetDNSConfig(guid, v4, v6)
		},
		RestoreFn: func(c *tweaks.Ctx, raw json.RawMessage) error {
			var s dnsSnap
			if err := json.Unmarshal(raw, &s); err != nil {
				return err
			}
			if !s.Present {
				return nil
			}
			cur, _, err := readDNS(c, guid)
			if err != nil {
				return err
			}
			if !cur.Present {
				if c.Log != nil {
					c.Log("adaptador %s não existe mais: nada a restaurar no DNS", guid)
				}
				return nil
			}
			// Nothing to write when the adapter already is as before (an apply
			// that was refused before writing, for example).
			if sameServers(cur.V4, s.V4) && sameServers(cur.V6, s.V6) {
				return nil
			}
			return c.P.Native.SetDNSConfig(guid, s.V4, s.V6) // the exact previous lists; empty = automatic
		},
		EqFn: func(a, b json.RawMessage) bool {
			var sa, sb dnsSnap
			if json.Unmarshal(a, &sa) != nil || json.Unmarshal(b, &sb) != nil {
				return false
			}
			if !sa.Present || !sb.Present {
				return true // adapter gone: nothing to compare
			}
			return sameServers(sa.V4, sb.V4) && sameServers(sa.V6, sb.V6)
		},
		CurFn: func(c *tweaks.Ctx) string {
			s, cfg, err := readDNS(c, guid)
			switch {
			case err != nil:
				return "erro: " + err.Error()
			case !s.Present:
				return "(adaptador não encontrado)"
			}
			out := "IPv4: " + listText(s.V4) + "; IPv6: " + listText(s.V6)
			if cfg.DoH {
				out += " (DNS criptografado configurado)"
			}
			return out
		},
	}
}

// IsDNSID tells whether an ID is a DNS item.
func IsDNSID(id string) bool { _, _, _, ok := netdiag.ParseDNSTweakID(id); return ok }

// DNSProvider builds the item that sets the DNS of an adapter on demand.
func DNSProvider(id string, f *facts.Facts) *tweaks.Tweak {
	guid, key, v4Only, ok := netdiag.ParseDNSTweakID(id)
	if !ok {
		return nil
	}
	rs, _ := netdiag.ResolverByKey(key)
	name := guid
	if f != nil {
		for _, a := range f.Adapters {
			if strings.EqualFold(a.InterfaceGUID, guid) && a.Name != "" {
				name = a.Name
			}
		}
	}
	v4, v6 := rs.V4, rs.V6
	servers := strings.Join(v4, ", ")
	both := "IPv4"
	if v4Only {
		v6 = []string{}
	} else {
		servers += " e " + strings.Join(v6, ", ")
		both = "IPv4 e IPv6"
	}
	detail := fmt.Sprintf("Usa SetInterfaceDnsSettings (API do Windows) para gravar %s como servidores fixos do adaptador e limpa o cache de nomes do Windows (ipconfig /flushdns). ", servers) +
		"Antes, guarda o DNS que estava (a lista fixa, ou o modo automático recebido do roteador), e o desfazer volta exatamente a ele. " +
		"Só é oferecido quando o teste de rede do Axion mediu o DNS atual falhando ou lento neste PC. O DNS só traduz nomes em endereços antes de conectar, então o ping durante a partida não muda. " +
		"Não mexe em adaptador com DNS criptografado (DNS over HTTPS) configurado por você nem quando uma política de grupo fixa o DNS."
	if v4Only {
		detail += " O IPv6 não é gravado porque o adaptador não tem DNS IPv6 próprio."
	}
	if key == "quad9" {
		detail += " O Quad9 bloqueia domínios que ele classifica como maliciosos (segundo o próprio serviço)."
	}
	return &tweaks.Tweak{
		ID: id, Category: "rede", Title: "DNS " + rs.Of + " em " + name, Group: "dns:" + strings.ToLower(guid),
		Summary:  "Troca o DNS do adaptador " + name + " pelo " + rs.Of + " (" + both + "). Sites e o login abrem mais rápido quando o DNS atual falha ou demora; o ping das partidas não muda.",
		Detail:   detail,
		Evidence: tweaks.EvMedium,
		Sources:  []tweaks.Source{srcSetDNS, {Title: rs.Source.Title, URL: rs.Source.URL}, srcRFC1034, srcIpconfig},
		Risk:     tweaks.RiskModerate,
		RiskNote: "Nomes da rede local ou da empresa (impressora, NAS, intranet), filtros de controle dos pais da operadora ou do roteador e páginas de login de Wi-Fi público dependem do DNS da rede; com um DNS público eles podem parar de abrir. Desfazer volta ao DNS anterior.",
		Reboot:   tweaks.RebootNone, OneClick: false, Tags: []string{"rede"},
		MinBuild:  dnsMinBuild,
		Recommend: neutral("oferecido só quando o teste de rede mostra o DNS atual falhando ou lento"),
		Ops:       func(*facts.Facts) []tweaks.Op { return []tweaks.Op{dnsOp(guid, v4, v6)} },
		Post: func(c *tweaks.Ctx, reverted bool) error {
			res, err := c.P.Run.Run(c.Context, "ipconfig.exe", []string{"/flushdns"}, sys.RunOpts{Timeout: dnsFlushWait})
			if err == nil && res.ExitCode != 0 {
				err = fmt.Errorf("código %d", res.ExitCode)
			}
			if err != nil {
				return fmt.Errorf("o cache de nomes do Windows não foi limpo (ipconfig /flushdns: %v); ele se renova sozinho em poucos minutos", err)
			}
			return nil
		},
		Effect: func(f *facts.Facts) (bool, string) {
			for _, a := range f.Adapters {
				if strings.EqualFold(a.InterfaceGUID, guid) && len(a.DNSServers) > 0 && !containsAny(a.DNSServers, append(append([]string(nil), rs.V4...), rs.V6...)) {
					return false, "o Windows não está usando este DNS: uma VPN, política ou programa de DNS pode estar por cima"
				}
			}
			return true, ""
		},
	}
}

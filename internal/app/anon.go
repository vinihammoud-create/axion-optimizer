package app

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// The exported report is anonymous by default (0.7): a report sent to a
// forum, a friend or a technician should not carry the name of the PC,
// the Windows account, the Steam account, the addresses of the network or
// device serial numbers. The anonymization runs on the JSON tree, so every
// page of the export (facts, tests, history, batches) gets the same
// treatment: keys known to carry identity are replaced, and the free text
// (evidence lines, paths, command lines) is scrubbed with patterns.

// anonKeys are JSON keys whose string values identify the PC, the person
// or the devices, replaced wherever they appear (lists of strings too;
// objects under them are still walked).
var anonKeys = map[string]string{
	"hostname": "<pc>", "host": "<pc>", "user": "<usuário>", "elevatedAs": "<usuário>", "userProfile": "<pasta do usuário>",
	"mac": "<mac>", "bssid": "<bssid>", "serial": "<série>", "serialNumber": "<série>", "uuid": "<uuid>",
	"accountId": "<conta>", "ipv4": "<ip>", "ipv6": "<ip>", "gateways": "<ip>", "dns": "<dns>", "dnsServers": "<dns>", "addr": "<ip>",
}

// versionKeys hold version strings that look like addresses (a NIC driver
// 12.19.1.37): the address patterns skip them.
var versionKeys = map[string]bool{"driverVersion": true, "vendorVersion": true, "version": true, "biosVersion": true, "firmware": true, "driver": true, "presentMon": true}

// anonConvars are CS2 settings that name the player.
var anonConvars = map[string]bool{"name": true, "cl_clanid": true}

var (
	reUsers   = regexp.MustCompile(`(?i)([\\/]Users[\\/])([^\\/"']+)`)
	reUserdat = regexp.MustCompile(`(?i)(userdata[\\/])(\d+)`)
	reMAC     = regexp.MustCompile(`\b(?:[0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2}\b`)
	reIPv4    = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	reIPv6    = regexp.MustCompile(`\b(?:[0-9A-Fa-f]{1,4}:){3,7}[0-9A-Fa-f]{1,4}\b|\b[0-9A-Fa-f]{1,4}(?::[0-9A-Fa-f]{1,4}){0,6}::(?:[0-9A-Fa-f]{1,4}(?::[0-9A-Fa-f]{1,4}){0,6})?\b`)
	reSteam64 = regexp.MustCompile(`\b7656119\d{10}\b`)
	// Bluetooth instance IDs carry the device's address as 12 hex digits.
	reBTAddr = regexp.MustCompile(`(?i)(BTHENUM\\[^"'\s]*&0&|BTHLE\\Dev_|BTH\\Dev_)([0-9A-F]{12})`)
	// The last segment of a USB/HID/storage instance path is the device's
	// own serial number when Windows did not generate it (generated ones
	// carry '&': 6&1f6a8a2&0&4).
	rePnPSerial = regexp.MustCompile(`(?i)\b((?:USB|USBSTOR|HID|SCSI|NVME)\\[^\s"']*\\)([^\\\s"'&]{6,})(&0)?\b`)
)

// keptUsers are profile folders that name nobody.
var keptUsers = map[string]bool{"public": true, "default": true, "default user": true, "all users": true, "defaultuser0": true}

// keptIPs are the public addresses of the test targets (they identify the
// test, not the PC); every other address is masked.
var keptIPs = map[string]bool{"1.1.1.1": true, "1.0.0.1": true, "8.8.8.8": true, "8.8.4.4": true, "9.9.9.9": true, "149.112.112.112": true, "208.67.222.222": true, "208.67.220.220": true, "127.0.0.1": true, "0.0.0.0": true}

// commonNames are PC or account names too generic to scrub from free text
// (a PC called "PC" would turn every "PC" of the report into a marker;
// an account called "Dell" would mangle the model). Their keyed fields are
// still replaced.
var commonNames = map[string]bool{"pc": true, "user": true, "usuario": true, "usuário": true, "admin": true, "administrador": true, "administrator": true, "gamer": true, "casa": true, "home": true, "note": true, "notebook": true, "desktop": true,
	"dell": true, "hp": true, "asus": true, "acer": true, "lenovo": true, "samsung": true, "msi": true, "gigabyte": true, "intel": true, "amd": true, "nvidia": true, "windows": true, "microsoft": true, "steam": true, "axion": true}

// anonymizer replaces what identifies this PC in the export.
type anonymizer struct {
	names []nameRep // PC name, account (with and without the domain), profile folder
}

type nameRep struct{ name, rep string }

// newAnonymizer learns the names to replace. Accounts come as
// DOMAIN\account on a real Windows: both forms are scrubbed.
func newAnonymizer(host string, users ...string) *anonymizer {
	a := &anonymizer{}
	add := func(n, rep string) {
		n = strings.TrimSpace(n)
		if len(n) < 3 || commonNames[strings.ToLower(n)] {
			return
		}
		for _, x := range a.names {
			if strings.EqualFold(x.name, n) {
				return
			}
		}
		a.names = append(a.names, nameRep{n, rep})
	}
	add(host, "<pc>")
	for _, u := range users {
		add(u, "<usuário>")
		if i := strings.LastIndexAny(u, `\/`); i >= 0 {
			add(u[i+1:], "<usuário>")
		}
	}
	// Longest first, so "Joao Silva" goes before "Joao".
	for i := 1; i < len(a.names); i++ {
		for j := i; j > 0 && len(a.names[j].name) > len(a.names[j-1].name); j-- {
			a.names[j], a.names[j-1] = a.names[j-1], a.names[j]
		}
	}
	return a
}

// Apply scrubs a decoded JSON tree in place and returns it.
func (a *anonymizer) Apply(v any) any {
	return a.apply(v, "")
}

func (a *anonymizer) apply(v any, key string) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if rep, ok := anonKeys[k]; ok {
				x[k] = a.replaceKey(val, rep)
			}
			if k == "convars" {
				if m, ok := val.(map[string]any); ok {
					for ck := range m {
						if anonConvars[strings.ToLower(ck)] {
							m[ck] = "<removido>"
						}
					}
				}
			}
			x[k] = a.apply(x[k], k)
		}
		return x
	case []any:
		for i := range x {
			x[i] = a.apply(x[i], key)
		}
		return x
	case string:
		return a.text(x, versionKeys[key])
	}
	return v
}

// replaceKey replaces a string (or the strings of a list) while keeping
// the shape; empty stays empty; objects are left for the walk.
func (a *anonymizer) replaceKey(v any, rep string) any {
	switch x := v.(type) {
	case string:
		if x == "" {
			return x
		}
		return rep
	case []any:
		for i := range x {
			if s, ok := x[i].(string); ok && s != "" {
				x[i] = rep
			}
		}
		return x
	}
	return v
}

// text scrubs free text: the names learned from the facts, profile
// folders, Steam accounts, device addresses and serials, MAC and IP
// addresses. version: the string is a version number, which the address
// patterns would mistake for one.
func (a *anonymizer) text(s string, version bool) string {
	if s == "" {
		return s
	}
	for _, n := range a.names {
		s = replaceFold(s, n.name, n.rep)
	}
	s = reUsers.ReplaceAllStringFunc(s, func(m string) string {
		g := reUsers.FindStringSubmatch(m)
		if keptUsers[strings.ToLower(g[2])] {
			return m
		}
		return g[1] + "<usuário>"
	})
	s = reUserdat.ReplaceAllString(s, "${1}<conta>")
	s = reSteam64.ReplaceAllString(s, "<steamid>")
	s = reBTAddr.ReplaceAllString(s, "${1}<mac>")
	s = rePnPSerial.ReplaceAllString(s, "${1}<série>${3}")
	s = reMAC.ReplaceAllString(s, "<mac>")
	if version {
		return s
	}
	s = replaceIPv4(s)
	s = reIPv6.ReplaceAllStringFunc(s, func(m string) string {
		if m == "::1" || m == "::" {
			return m
		}
		return "<ip>"
	})
	return s
}

// replaceIPv4 masks dotted addresses: four numbers up to 255, not a test
// target, and not a version number announced as one ("driver 2.1.4.3",
// "versão 12.19.1.37").
func replaceIPv4(s string) string {
	locs := reIPv4.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, loc := range locs {
		m := s[loc[0]:loc[1]]
		ok := !keptIPs[m]
		for _, part := range strings.Split(m, ".") {
			if n, err := strconv.Atoi(part); err != nil || n > 255 {
				ok = false
			}
		}
		if ok {
			lead := strings.ToLower(s[maxInt(0, loc[0]-12):loc[0]])
			for _, w := range []string{"driver ", "versão ", "versao ", "version ", "vers. ", "firmware ", "bios ", " v", "(v"} {
				if strings.HasSuffix(lead, w) {
					ok = false
				}
			}
		}
		b.WriteString(s[last:loc[0]])
		if ok {
			b.WriteString("<ip>")
		} else {
			b.WriteString(m)
		}
		last = loc[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// asciiLower lowers the ASCII letters only, so the byte offsets of the
// result match the original (a lower-cased İ or K would change length).
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// replaceFold replaces every occurrence of old, ignoring ASCII case, as a
// whole word (a user called "Ana" must not mangle "Analisador").
func replaceFold(s, old, rep string) string {
	ls, lo := asciiLower(s), asciiLower(old)
	if lo == "" || !strings.Contains(ls, lo) {
		return s
	}
	var b strings.Builder
	i := 0
	for {
		j := strings.Index(ls[i:], lo)
		if j < 0 {
			b.WriteString(s[i:])
			return b.String()
		}
		start, end := i+j, i+j+len(lo)
		if wordChar(ls, start-1) || wordChar(ls, end) {
			b.WriteString(s[i:end])
			i = end
			continue
		}
		b.WriteString(s[i:start])
		b.WriteString(rep)
		i = end
	}
}

func wordChar(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	c := s[i]
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

// anonNames are the names of this PC the export must not carry: the host,
// the console account and the elevated account (DOMAIN\name) and the
// profile folder.
func (a *App) anonNames() (host string, users []string) {
	f := a.Facts()
	if f != nil {
		host = f.Hostname
		users = append(users, f.OS.User, f.OS.ElevatedAs)
	}
	if p := a.P.Native.Paths(); p.UserProfile != "" {
		users = append(users, filepath.Base(strings.ReplaceAll(p.UserProfile, `\`, "/")))
	}
	return host, users
}

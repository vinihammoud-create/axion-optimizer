package netdiag

import (
	"strconv"
	"strings"
)

// ParseWlanInterfaces reads the output of "netsh.exe wlan show interfaces"
// (English or Brazilian Portuguese) and returns the signal (0-100) and the
// band of the interface with this GUID. With several Wi-Fi adapters the
// command lists all of them, one block each; only the block whose GUID
// matches is used. Band comes back with a decimal point ("2.4 GHz") whatever
// the Windows language wrote. An unknown GUID gives 0 and "".
func ParseWlanInterfaces(out, guid string) (signal int, band string) {
	want := normGUID(guid)
	if want == "" {
		return 0, ""
	}
	var curGUID, curBand string
	var curSig int
	flush := func() {
		if curGUID != "" && curGUID == want {
			signal, band = curSig, curBand
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		switch k {
		case "guid":
			flush()
			curGUID, curSig, curBand = normGUID(v), 0, ""
		case "signal", "sinal":
			// "99%" or "99 %".
			n, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(v, "%")))
			if err != nil || n < 0 || n > 100 {
				n = 0
			}
			curSig = n
		case "band", "banda":
			curBand = strings.ReplaceAll(v, ",", ".")
		}
	}
	flush()
	return signal, band
}

func normGUID(g string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(g), "{}"))
}

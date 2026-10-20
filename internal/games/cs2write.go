package games

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrCS2VideoFormat: the file is not the flat "video.cfg" { "k" "v" } block
// CS2 writes.
var ErrCS2VideoFormat = errors.New("o cs2_video.txt tem um formato que o Axion não reconhece; nada foi alterado")

// videoSpan is where the value of one key sits (only the bytes between the
// quotes of the value).
type videoSpan struct {
	valStart, valEnd int
	value            string
}

var reVideoLine = regexp.MustCompile(`^([ \t]*)"([^"\r\n]*)"([ \t]+)"([^"\r\n]*)"([ \t]*)(\r?)$`)

const utf8BOM = "\xEF\xBB\xBF"

// findVideoKey locates key (case-insensitive, as ParseVDF) in the top-level
// "video.cfg" block. Only the bytes between the value's quotes are reported.
func findVideoKey(src []byte, key string) (videoSpan, bool, error) {
	want := strings.ToLower(key)
	state, n := 0, 0 // 0 before header, 1 header, 2 inside, 3 closed
	var sp videoSpan
	for off := 0; off < len(src); {
		lineEnd, next := len(src), len(src)
		if i := bytes.IndexByte(src[off:], '\n'); i >= 0 {
			lineEnd, next = off+i, off+i+1
		}
		line := src[off:lineEnd] // keeps a trailing \r (CRLF files)
		t := strings.TrimSpace(string(line))
		if state == 0 {
			t = strings.TrimSpace(strings.TrimPrefix(string(line), utf8BOM))
		}
		switch state {
		case 0:
			if strings.EqualFold(t, `"video.cfg"`) {
				state = 1
			} else if t != "" {
				return videoSpan{}, false, ErrCS2VideoFormat
			}
		case 1:
			if t == "{" {
				state = 2
			} else if t != "" {
				return videoSpan{}, false, ErrCS2VideoFormat
			}
		case 2:
			if t == "}" {
				state = 3
				break
			}
			if t == "" {
				break
			}
			m := reVideoLine.FindSubmatchIndex(line)
			if m == nil {
				if strings.ContainsAny(t, "{}") || strings.Contains(strings.ToLower(t), `"`+want+`"`) {
					return videoSpan{}, false, ErrCS2VideoFormat
				}
				break // a line about something else stays as is
			}
			if strings.ToLower(string(line[m[4]:m[5]])) == want {
				n++
				sp = videoSpan{valStart: off + m[8], valEnd: off + m[9], value: string(line[m[8]:m[9]])}
			}
		case 3:
			if t != "" {
				return videoSpan{}, false, ErrCS2VideoFormat
			}
		}
		off = next
	}
	if state != 3 {
		return videoSpan{}, false, ErrCS2VideoFormat
	}
	if n > 1 {
		return videoSpan{}, false, fmt.Errorf("%w (a opção %s aparece %d vezes)", ErrCS2VideoFormat, key, n)
	}
	return sp, n == 1, nil
}

// CS2VideoValue returns the value of key in cs2_video.txt.
func CS2VideoValue(src []byte, key string) (string, bool, error) {
	sp, ok, err := findVideoKey(src, key)
	return sp.value, ok, err
}

// SetCS2VideoValue replaces only the value between quotes; the rest of the
// file (order, tabs, CRLF, other keys) is byte-identical. Never inserts a key.
func SetCS2VideoValue(src []byte, key, value string) ([]byte, error) {
	if len(value) > 64 || strings.ContainsAny(value, "\"\r\n\\") {
		return nil, fmt.Errorf("valor inválido para %s", key)
	}
	sp, ok, err := findVideoKey(src, key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("a opção %s não está no cs2_video.txt", key)
	}
	out := make([]byte, 0, len(src)-len(sp.value)+len(value))
	out = append(out, src[:sp.valStart]...)
	out = append(out, value...)
	return append(out, src[sp.valEnd:]...), nil
}

// CS2VideoInCloud reads userdata\<acc>\730\remotecache.vdf (the files Steam
// Cloud keeps for CS2 on this account) and tells whether cs2_video.txt is one.
// read=false: the list could not be read (nothing can be said).
func (s *Scanner) CS2VideoInCloud(root, acc string) (inCloud, read bool) {
	b, err := s.FS.ReadFile(winJoin(root, "userdata", acc, "730", "remotecache.vdf"))
	if err != nil {
		return false, false
	}
	v, err := ParseVDF(string(b))
	if err != nil {
		return false, false
	}
	app := v.Sub("730")
	if app == nil {
		return false, false
	}
	for k, val := range app {
		if _, isBlock := val.(VDF); isBlock && (k == "cs2_video.txt" || strings.HasSuffix(k, "/cs2_video.txt") || strings.HasSuffix(k, `\cs2_video.txt`)) {
			return true, true
		}
	}
	return false, true
}

package games

import (
	"errors"
	"strings"
)

// VDF is a parsed Valve KeyValues text document (libraryfolders.vdf,
// appmanifest_*.acf, localconfig.vdf, cs2_video.txt). Keys are stored
// lower-cased; values are either string or VDF.
type VDF map[string]any

// Str returns a string value (case-insensitive key).
func (v VDF) Str(key string) string {
	if v == nil {
		return ""
	}
	s, _ := v[strings.ToLower(key)].(string)
	return s
}

// Sub returns a nested block (case-insensitive key).
func (v VDF) Sub(key string) VDF {
	if v == nil {
		return nil
	}
	m, _ := v[strings.ToLower(key)].(VDF)
	return m
}

// Path follows nested blocks.
func (v VDF) Path(keys ...string) VDF {
	cur := v
	for _, k := range keys {
		cur = cur.Sub(k)
		if cur == nil {
			return nil
		}
	}
	return cur
}

var errVDF = errors.New("vdf inválido")

// vdfMaxDepth and vdfMaxBytes bound what ParseVDF accepts: these files are
// writable by the user, and a deep enough nesting would overflow the stack
// (a fatal error in Go, which no recover catches) at every scan.
const (
	vdfMaxDepth = 64
	vdfMaxBytes = 32 << 20
)

// ParseVDF parses the text KeyValues format. It is tolerant: unknown
// constructs (#include, conditionals like [$WIN32]) are skipped.
func ParseVDF(src string) (VDF, error) {
	if len(src) > vdfMaxBytes {
		return nil, errVDF
	}
	toks, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	pos := 0
	var parse func(depth int) (VDF, error)
	parse = func(depth int) (VDF, error) {
		out := VDF{}
		for pos < len(toks) {
			t := toks[pos]
			if t.kind == '}' {
				if depth == 0 {
					return nil, errVDF
				}
				pos++
				return out, nil
			}
			if t.kind != 's' {
				return nil, errVDF
			}
			key := strings.ToLower(t.text)
			pos++
			if pos >= len(toks) {
				return nil, errVDF
			}
			switch toks[pos].kind {
			case '{':
				pos++
				if depth+1 > vdfMaxDepth {
					return nil, errVDF
				}
				sub, err := parse(depth + 1)
				if err != nil {
					return nil, err
				}
				if prev, ok := out[key].(VDF); ok {
					for k, v := range sub {
						prev[k] = v
					}
				} else {
					out[key] = sub
				}
			case 's':
				if _, exists := out[key]; !exists {
					out[key] = toks[pos].text
				}
				pos++
			default:
				return nil, errVDF
			}
		}
		if depth > 0 {
			return nil, errVDF
		}
		return out, nil
	}
	return parse(0)
}

type token struct {
	kind byte // 's' string, '{', '}'
	text string
}

func tokenize(src string) ([]token, error) {
	var toks []token
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == 0xEF || c == 0xBB || c == 0xBF:
			i++
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '{' || c == '}':
			toks = append(toks, token{kind: c})
			i++
		case c == '[':
			// conditional such as [$WIN32]: skip
			for i < len(src) && src[i] != ']' {
				i++
			}
			i++
		case c == '"':
			i++
			var sb strings.Builder
			for {
				if i >= len(src) {
					return nil, errVDF
				}
				if src[i] == '\\' && i+1 < len(src) {
					switch src[i+1] {
					case '\\':
						sb.WriteByte('\\')
					case '"':
						sb.WriteByte('"')
					case 'n':
						sb.WriteByte('\n')
					case 't':
						sb.WriteByte('\t')
					default:
						sb.WriteByte('\\')
						sb.WriteByte(src[i+1])
					}
					i += 2
					continue
				}
				if src[i] == '"' {
					i++
					break
				}
				sb.WriteByte(src[i])
				i++
			}
			toks = append(toks, token{kind: 's', text: sb.String()})
		default:
			// unquoted token (rare): read until whitespace or brace
			j := i
			for j < len(src) && !strings.ContainsRune(" \t\r\n{}\"", rune(src[j])) {
				j++
			}
			if j == i {
				i++
				continue
			}
			toks = append(toks, token{kind: 's', text: src[i:j]})
			i = j
		}
	}
	return toks, nil
}

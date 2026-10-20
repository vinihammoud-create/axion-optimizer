package winsys

import "unicode/utf16"

// splitMultiSZ splits UTF-16 REG_MULTI_SZ data at each NUL, keeping empty
// strings in the middle and dropping the empty ones at the end (the
// terminator, and padding some writers leave).
func splitMultiSZ(u []uint16) []string {
	var out []string
	start := 0
	for i, c := range u {
		if c == 0 {
			out = append(out, string(utf16.Decode(u[start:i])))
			start = i + 1
		}
	}
	if start < len(u) { // no final NUL
		out = append(out, string(utf16.Decode(u[start:])))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

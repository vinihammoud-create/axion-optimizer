package netprobe

import (
	"encoding/binary"
	"errors"
	"strings"
)

// BuildQuery encodes a standard recursive query for the A record of name
// (RFC 1035, section 4.1).
func BuildQuery(id uint16, name string) ([]byte, error) {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".")
	if name == "" || len(name) > 253 {
		return nil, errors.New("nome inválido")
	}
	b := make([]byte, 12, 12+len(name)+6)
	binary.BigEndian.PutUint16(b[0:], id)
	binary.BigEndian.PutUint16(b[2:], 0x0100) // RD
	binary.BigEndian.PutUint16(b[4:], 1)      // QDCOUNT
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 {
			return nil, errors.New("nome inválido")
		}
		b = append(b, byte(len(label)))
		b = append(b, label...)
	}
	b = append(b, 0, 0, 1, 0, 1) // root, QTYPE A, QCLASS IN
	return b, nil
}

// ParseReply reads the ID and the response code of a DNS message; ok is
// false when it is not a response.
func ParseReply(b []byte) (id uint16, rcode int, ok bool) {
	if len(b) < 12 {
		return 0, 0, false
	}
	flags := binary.BigEndian.Uint16(b[2:])
	if flags&0x8000 == 0 { // QR
		return 0, 0, false
	}
	return binary.BigEndian.Uint16(b[0:]), int(flags & 0x000F), true
}

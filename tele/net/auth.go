package telenet

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/temoto/vender/tele"
)

var errAuthSecretWeak = fmt.Errorf("secret must be >= 8 bytes")

// Returns first uint64 of HMAC-SHA256 over concatenated:
// p.Seq p.Time p.VmId p.AuthId
// All numbers use big-endian encoding.
func Auth1(p *tele.Packet, secret []byte) (uint64, error) {
	if len(secret) < 8 {
		return 0, errAuthSecretWeak
	}
	var b [sha256.Size]byte
	binary.BigEndian.PutUint32(b[0:], p.Seq)
	binary.BigEndian.PutUint64(b[4:], uint64(p.Time))
	binary.BigEndian.PutUint32(b[12:], uint32(p.VmId))
	h := hmac.New(sha256.New, secret)
	if _, err := h.Write(b[:16]); err != nil {
		return 0, err
	}
	if _, err := h.Write([]byte(p.AuthId)); err != nil {
		return 0, err
	}
	prefix := binary.BigEndian.Uint64(h.Sum(b[:0]))
	return prefix, nil
}

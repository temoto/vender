package telenet

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/temoto/vender/tele"
)

var errAuthSecretWeak = fmt.Errorf("secret must be >= 8 bytes")

type GetSecretFunc = func(authid string, f *tele.Frame) ([]byte, error)

// Returns big-endian first uint64 of HMAC-SHA256.
// `secret` must be at least 8 bytes.
func Auth1(data, secret []byte) (uint64, error) {
	if len(secret) < 8 {
		return 0, errAuthSecretWeak
	}
	var b [sha256.Size]byte
	h := hmac.New(sha256.New, secret)
	if _, err := h.Write(data); err != nil {
		return 0, err
	}
	prefix := binary.BigEndian.Uint64(h.Sum(b[:0]))
	return prefix, nil
}

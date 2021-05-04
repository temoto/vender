package spq

import (
	"bytes"
	"encoding"
	"encoding/binary"
)

const keyPrefixLen = 4
const keyLen = keyPrefixLen + 8

var itemKeyPrefix = [keyPrefixLen]byte{'p', 'q', 'i', '1'}
var itemKeyLimit = [keyPrefixLen]byte{'p', 'q', 'i', '2'}

type Box struct {
	err   error
	key   [keyLen]byte
	value []byte
}

func (b *Box) Bytes() []byte { return b.value }
func (b *Box) Unmarshal(x encoding.BinaryUnmarshaler) error {
	return x.UnmarshalBinary(b.value)
}

func (b *Box) empty() bool {
	return (b.value == nil) && (b.err == nil)
}

func encodeKey(key []byte, id uint64) {
	copy(key, itemKeyPrefix[:])
	binary.BigEndian.PutUint64(key[keyPrefixLen:], id)
}

func unkey(key []byte) (uint64, error) {
	if len(key) != keyLen {
		return 0, ErrInvalidKey
	}
	if !bytes.Equal(key[:keyPrefixLen], itemKeyPrefix[:]) {
		return 0, ErrInvalidKey
	}
	return binary.BigEndian.Uint64(key[keyPrefixLen:]), nil
}

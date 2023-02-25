package spq

import (
	"fmt"

	ldberrors "github.com/syndtr/goleveldb/leveldb/errors"
)

var (
	ErrClosed     = fmt.Errorf("pq is closed")
	ErrInvalidKey = fmt.Errorf("pq storage key invalid")
)

func IsCorrupted(err error) bool {
	return ldberrors.IsCorrupted(err)
}

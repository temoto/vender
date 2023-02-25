package extremofile

import (
	"errors"
)

var (
	errCorrupt   = errors.New("data corrupted")
	errMetaParse = errors.New("metadata error")
)

type critical struct{ E error }

func (f critical) Error() string { return f.E.Error() }

func wrapCritical(e error) error {
	if e == nil {
		return nil
	}
	if _, ok := e.(critical); ok {
		return e
	}
	return critical{E: e}
}

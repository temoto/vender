package gpio

import (
	"errors"
	"io"
	"time"
)

var ErrClosed = errors.New("already closed")
var ErrTimeout error = errTimeout{}

type errTimeout struct{}

func (errTimeout) Error() string   { return "timeout" }
func (errTimeout) IsTimeout() bool { return true }
func (errTimeout) Timeout() bool   { return true }
func (errTimeout) Temporary() bool { return true }

// Please use this to check whether gpio.*.Close() was already called.
func IsClosed(err error) bool {
	return err == ErrClosed
}

// Or use any timeout error check you know, and please report if some doesn't work.
func IsTimeout(err error) bool {
	if err == nil { // redundant but hopefully useful shortcut
		return false
	}
	if t, ok := err.(interface{ Timeout() bool }); ok {
		return t.Timeout()
	}
	return false
}

type Chiper interface {
	io.Closer
	Info() ChipInfo
	LineInfo(line uint32) (LineInfo, error)
	OpenLines(flag RequestFlag, consumerLabel string, lines ...uint32) (Lineser, error)
	GetLineEvent(line uint32, flag RequestFlag, events EventFlag, consumerLabel string) (Eventer, error)
}

type LineSetFunc func(value byte)

type Lineser interface {
	io.Closer
	SetFunc(line uint32) LineSetFunc
	LineOffsets() []uint32
	Read() (HandleData, error)
	Flush() error
	SetBulk(bs ...byte)
}

type Eventer interface {
	io.Closer
	Read() (byte, error)
	Wait(timeout time.Duration) (EventData, error)
}

// compile-time interface check
var _ Chiper = &chip{}

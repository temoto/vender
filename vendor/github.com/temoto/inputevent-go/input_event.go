package inputevent

import (
	"syscall"
	"unsafe"
)

type InputEvent struct {
	Time  syscall.Timeval // when event occurred
	Type  uint16          // one of EV_*, defines meaning of following fields
	Code  uint16          // EV_KEY: key scan code
	Value int32           // EV_KEY: KeyEventState
}

const EventSizeof = int(unsafe.Sizeof(InputEvent{}))

// InputEvent.Value
type KeyEventState int32

const (
	KeyStateUp   KeyEventState = 0
	KeyStateDown KeyEventState = 1
	KeyStateHold KeyEventState = 2
)

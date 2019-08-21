package ui

import (
	"fmt"

	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/head/money"
)

//go:generate stringer -type=EventKind -trimprefix=Event
type EventKind uint8

const (
	EventInvalid EventKind = iota
	EventInput
	EventMoney
	EventTime
	EventService
	EventCommand // TODO
	EventStop
)

type Event struct { //nolint:maligned
	Kind  EventKind
	Input input.Event
	Money money.Event
}

func (e *Event) String() string {
	return fmt.Sprintf("ui.Event(%s input=%v money=%v)", e.Kind.String(), e.Input, e.Money)
}

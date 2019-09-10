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
	EventLock
	EventService
	EventStop
)

type Event struct { //nolint:maligned
	Kind  EventKind
	Input input.Event
	Money money.Event
}

func (e *Event) String() string {
	inner := ""
	switch e.Kind {
	case EventInput:
		inner = fmt.Sprintf(" source=%s key=%v up=%t", e.Input.Source, e.Input.Key, e.Input.Up)
	case EventMoney:
		// TODO
		inner = fmt.Sprintf(" %#v", e.Money)
	}
	return fmt.Sprintf("ui.Event(%s%s)", e.Kind.String(), inner)
}

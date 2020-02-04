package types

import (
	"fmt"

	"github.com/temoto/vender/currency"
)

//go:generate stringer -type=EventKind -trimprefix=Event
type EventKind uint8

const (
	EventInvalid EventKind = iota
	EventInput
	EventMoneyCredit
	EventTime
	EventLock
	EventService
	EventStop
)

type Event struct {
	Input  InputEvent
	Amount currency.Amount
	Kind   EventKind
}

func (e *Event) String() string {
	inner := ""
	switch e.Kind {
	case EventInput:
		inner = fmt.Sprintf(" source=%s key=%v up=%t", e.Input.Source, e.Input.Key, e.Input.Up)
	case EventMoneyCredit:
		// inner = fmt.Sprintf(" amount=%s err=%v", e.Amount.Format100I(), e.Money.Err)
		inner = fmt.Sprintf(" amount=%d", e.Amount)
	}
	return fmt.Sprintf("Event(%s%s)", e.Kind.String(), inner)
}

type InputKey uint16

type InputEvent struct {
	Source string
	Key    InputKey
	Up     bool
}

func (e *InputEvent) IsZero() bool  { return e.Key == 0 }
func (e *InputEvent) IsDigit() bool { return e.Key >= '0' && e.Key <= '9' }

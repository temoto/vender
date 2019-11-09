package money

import (
	"fmt"
	"time"

	"github.com/temoto/vender/currency"
)

//go:generate stringer -type=EventKind -trimprefix=Event
type EventKind uint8

const (
	EventInvalid EventKind = iota
	EventAbort
	EventCredit
)

type Event struct {
	Created time.Time
	Err     error
	Amount  currency.Amount
	Kind    EventKind
}

func (e *Event) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}
func (e *Event) String() string {
	return fmt.Sprintf("money.Event(kind=%s err='%s' created=%s amount=%s)", e.Kind.String(), e.Error(), e.Created.Format(time.RFC3339Nano), e.Amount.Format100I())
}

package kitchen

import (
	"fmt"
	"time"

	"github.com/juju/errors"
)

const (
	EventAbort = "abort" // unable to serve
	EventDone  = "done"  // serving done
	EventPing  = "ping"
)

type Event struct {
	created time.Time
	name    string
	item    string
	err     error
}

func (e *Event) Time() time.Time { return e.created }
func (e *Event) Name() string    { return e.name }
func (e *Event) Item() string    { return e.item }
func (e *Event) Err() error      { return e.err }
func (e *Event) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}
func (e *Event) String() string {
	return fmt.Sprintf("kitchen.Event<name=%s err='%s' created=%s item=%s>", e.name, e.Error(), e.created.Format(time.RFC3339Nano), e.item)
}

func (self *KitchenSystem) Events() <-chan Event {
	return self.events
}

var (
	ErrTODO = errors.New("TODO")
)

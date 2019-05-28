package money

import (
	"testing"
	"time"
)

func TestEvents01(t *testing.T) {
	ms := MoneySystem{}
	received := make(chan Event, 1)
	ms.EventSubscribe(func(ev Event) {
		received <- ev
	})
	ms.EventFire(Event{name: EventPing})
	select {
	case e := <-received:
		if e.Name() != EventPing {
			t.Fatalf("Invalid event received: %s", e.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("Event receive timeout")
	}
}

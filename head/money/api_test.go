package money

import (
	"testing"
	"time"
)

func TestEvents01(t *testing.T) {
	ms := MoneySystem{events: make(chan Event, 1)}
	go func() { ms.events <- Event{name: EventPing} }()
	select {
	case e := <-ms.Events():
		if e.Name() != EventPing {
			t.Fatalf("Invalid event received: %s", e.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("Event receive timeout")
	}
}

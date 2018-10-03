package money

import (
	"testing"
	"time"
)

func TestEvents01(t *testing.T) {
	es := Global.events
	go func() { es <- Event{name: EventPing} }()
	select {
	case e := <-es:
		if e.Name() != EventPing {
			t.Fatalf("Invalid event received: %s", e.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("Event receive timeout")
	}
}

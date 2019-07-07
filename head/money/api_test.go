package money

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/state"
)

func TestEvents01(t *testing.T) {
	t.Parallel()

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

func TestAbort(t *testing.T) {
	t.Parallel()

	ctx, g := state.NewTestContext(t, "money{scale=100}")
	mock := mdb.MockFromContext(ctx)
	defer mock.Close()
	mock.ExpectMap(map[string]string{
		"09":           "021643640200170102050a0a1900000000000000000000",
		"0f00":         "434f47303030303030303030303030463030313230303120202020029000000003",
		"0f0100000002": "",
		"0f05":         "01000600",
		"0a":           "0000110008",
		"0b":           "",
		"":             "",
	})

	ms := MoneySystem{}
	require.NoError(t, ms.Start(ctx))
	mock.ExpectMap(nil)

	ms.dirty += g.Config().ScaleU(11)
	go mock.Expect([]mdb.MockR{
		{"0b", ""},
		{"0f020b", ""},
		{"0f04", "00"},
		{"0f04", ""},
		{"0f03", "0b00"},
	})
	time.Sleep(10 * time.Millisecond) // let coin Run() make POLL
	require.NoError(t, ms.Abort(ctx))

	mock.ExpectMap(map[string]string{
		"0b":         "",
		"0c0000ffff": "",
		"":           "",
	})
	require.NoError(t, ms.Stop(ctx))
}

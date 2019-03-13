package evend

import (
	"testing"

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
)

func TestConveyor(t *testing.T) {
	t.Parallel()
	init := func(t testing.TB, reqCh <-chan mdb.Packet, respCh chan<- mdb.Packet) {
		mdb.TestChanTx(t, reqCh, respCh, "d8", "")
		mdb.TestChanTx(t, reqCh, respCh, "d9", "011810000a0000c8001fff01050a32640000000000000000000000")
	}
	reply := func(t testing.TB, reqCh <-chan mdb.Packet, respCh chan<- mdb.Packet) {
		mdb.TestChanTx(t, reqCh, respCh, "db", "04")
		mdb.TestChanTx(t, reqCh, respCh, "da011806", "")
		mdb.TestChanTx(t, reqCh, respCh, "db", "50")
		mdb.TestChanTx(t, reqCh, respCh, "db", "54")
		mdb.TestChanTx(t, reqCh, respCh, "db", "")

		mdb.TestChanTx(t, reqCh, respCh, "db", "")
		mdb.TestChanTx(t, reqCh, respCh, "da016707", "")
		mdb.TestChanTx(t, reqCh, respCh, "db", "54")
		mdb.TestChanTx(t, reqCh, respCh, "db", "50")
		mdb.TestChanTx(t, reqCh, respCh, "db", "")
	}
	ctx := testMake(t, init, reply)
	e := engine.ContextValueEngine(ctx, engine.ContextKey)
	d := new(DeviceConveyor)
	// TODO make small delay default in tests
	d.dev.DelayErr = 1
	d.dev.DelayNext = 1
	d.dev.DelayReset = 1
	err := d.Init(ctx)
	if err != nil {
		t.Fatalf("Init err=%v", err)
	}
	err = e.Resolve("mdb.evend.conveyor_move_cup").Do(ctx)
	if err != nil {
		t.Fatalf("Move err=%v", err)
	}
	err = e.Resolve("mdb.evend.conveyor_move_elevator").Do(ctx)
	if err != nil {
		t.Fatalf("Move err=%v", err)
	}
}

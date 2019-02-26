package evend

import (
	"testing"
	"time"

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
)

func TestValve(t *testing.T) {
	init := func(t testing.TB, reqCh <-chan mdb.Packet, respCh chan<- mdb.Packet) {
		mdb.TestChanTx(t, reqCh, respCh, "c0", "")
		mdb.TestChanTx(t, reqCh, respCh, "c1", "011810000a0000c8001fff01050a32640000000000000000000000")
	}
	reply := func(t testing.TB, reqCh <-chan mdb.Packet, respCh chan<- mdb.Packet) {
		mdb.TestChanTx(t, reqCh, respCh, "c3", "44")
		mdb.TestChanTx(t, reqCh, respCh, "c3", "04")
		mdb.TestChanTx(t, reqCh, respCh, "c3", "")
		mdb.TestChanTx(t, reqCh, respCh, "c2014e", "")
		mdb.TestChanTx(t, reqCh, respCh, "c3", "14")
		mdb.TestChanTx(t, reqCh, respCh, "c3", "14")
		mdb.TestChanTx(t, reqCh, respCh, "c3", "10")
		mdb.TestChanTx(t, reqCh, respCh, "c3", "")
	}
	ctx := testMake(t, init, reply)
	e := engine.ContextValueEngine(ctx, engine.ContextKey)
	d := new(DeviceValve)
	err := d.Init(ctx)
	// TODO make small delay default in tests
	d.dev.DelayNext = 0 * time.Millisecond
	d.dev.DelayErr = 0 * time.Millisecond
	if err != nil {
		t.Fatalf("Init err=%v", err)
	}
	err = e.Resolve("mdb.evend.valve_pour_hot(120)").Do(ctx)
	if err != nil {
		t.Fatalf("pour_hot err=%v", err)
	}
}

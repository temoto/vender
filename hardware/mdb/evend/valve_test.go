package evend

import (
	"math/rand"
	"testing"

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/helpers"
)

func TestValve(t *testing.T) {
	t.Parallel()

	init := func(t testing.TB, reqCh <-chan mdb.Packet, respCh chan<- mdb.Packet) {
		mdb.TestChanTx(t, reqCh, respCh, "c0", "")
		mdb.TestChanTx(t, reqCh, respCh, "c1", "011810000a0000c8001fff01050a32640000000000000000000000")
	}
	reply := func(t testing.TB, reqCh <-chan mdb.Packet, respCh chan<- mdb.Packet) {
		mdb.TestChanTx(t, reqCh, respCh, "c411", "17")
		mdb.TestChanTx(t, reqCh, respCh, "c51049", "")

		mdb.TestChanTx(t, reqCh, respCh, "c3", "44")
		mdb.TestChanTx(t, reqCh, respCh, "c3", "04")
		mdb.TestChanTx(t, reqCh, respCh, "c3", "")
		mdb.TestChanTx(t, reqCh, respCh, "c2014e", "")
		mdb.TestChanTx(t, reqCh, respCh, "c3", "10")
		mdb.TestChanTx(t, reqCh, respCh, "c3", "")
	}
	ctx := testMake(t, init, reply)
	// config := state.GetConfig(ctx)
	e := engine.ContextValueEngine(ctx, engine.ContextKey)
	d := new(DeviceValve)
	// TODO make small delay default in tests
	d.dev.DelayIdle = 1
	d.dev.DelayNext = 1
	d.dev.DelayReset = 1
	err := d.Init(ctx)
	if err != nil {
		t.Fatalf("Init err=%v", err)
	}

	engine.DoCheckError(t, e.Resolve("mdb.evend.valve_get_temp_hot"), ctx)
	helpers.AssertEqual(t, d.tempHot, uint8(23))

	engine.DoCheckError(t, d.NewSetTempHot().(engine.ArgApplier).Apply(73), ctx)

	water := d.waterStock.Min() + rand.Int31() + 120
	d.waterStock.Set(water)
	engine.DoCheckError(t, e.Resolve("mdb.evend.valve_pour_hot(120)"), ctx)
	helpers.AssertEqual(t, d.waterStock.Value(), water-120)
}

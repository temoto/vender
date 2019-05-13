package evend

import (
	"math/rand"
	"testing"

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

func TestValve(t *testing.T) {
	t.Parallel()

	ctx := state.NewTestContext(t, "")
	mock := mdb.MockFromContext(ctx)
	defer mock.Close()
	go mock.Expect([]mdb.MockR{
		{"c0", ""},
		{"c1", "011810000a0000c8001fff01050a32640000000000000000000000"},

		{"c411", "17"},
		{"c51049", ""},

		{"c3", "44"},
		{"c3", "04"},
		{"c3", ""},
		{"c2013a", ""},
		{"c3", "10"},
		{"c3", ""},
	})
	e := engine.GetEngine(ctx)
	d := new(DeviceValve)
	// TODO make small delay default in tests
	d.dev.DelayIdle = 1
	d.dev.DelayNext = 1
	d.dev.DelayReset = 1
	err := d.Init(ctx)
	if err != nil {
		t.Fatalf("Init err=%v", err)
	}

	engine.TestDo(t, ctx, "mdb.evend.valve_get_temp_hot")
	helpers.AssertEqual(t, d.tempHot, uint8(23))

	engine.DoCheckError(t, engine.ArgApply(d.NewSetTempHot(), 73), ctx)

	water := d.waterStock.Min() + rand.Int31() + 90
	t.Logf("water before=%d", water)
	d.waterStock.Set(water)
	engine.DoCheckError(t, e.Resolve("mdb.evend.valve_pour_hot(90)"), ctx)
	helpers.AssertEqual(t, d.waterStock.Value(), water-90)
}

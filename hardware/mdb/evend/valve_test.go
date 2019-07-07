package evend

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/state"
)

func TestValve(t *testing.T) {
	t.Parallel()

	ctx, g := state.NewTestContext(t, "")
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
	d := new(DeviceValve)
	// TODO make small delay default in tests
	d.dev.DelayIdle = 1
	d.dev.DelayNext = 1
	d.dev.DelayReset = 1
	err := d.Init(ctx)
	if err != nil {
		t.Fatalf("Init err=%v", err)
	}

	g.Engine.TestDo(t, ctx, "mdb.evend.valve_get_temp_hot")
	assert.Equal(t, uint8(23), uint8(d.tempHot.Get()))

	g.Engine.TestDo(t, ctx, "mdb.evend.valve_set_temp_hot(73)")

	water := d.waterStock.Min() + rand.Int31() + 90
	t.Logf("water before=%d", water)
	d.waterStock.Set(water)
	g.Engine.TestDo(t, ctx, "mdb.evend.valve_pour_hot(90)")
	assert.Equal(t, water-90, d.waterStock.Value())
}

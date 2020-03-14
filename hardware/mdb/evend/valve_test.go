package evend

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/hardware/mdb"
	state_new "github.com/temoto/vender/internal/state/new"
)

func TestValve(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, "", `
engine { inventory {
	stock "water" { check=false hw_rate = 0.6 min = 500 }
}}
hardware { device "evend.valve" {} }`)
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
		{"c20136", ""},
		{"c3", "10"},
		{"c3", ""},

		{"c411", "00"}, // when hot temp sensor is broken
		{"c51000", ""}, // must disable boiler
	})
	require.NoError(t, Enum(ctx))

	g.Engine.TestDo(t, ctx, "evend.valve.get_temp_hot")
	dev, err := g.GetDevice("evend.valve")
	require.NoError(t, err)
	assert.Equal(t, uint8(23), uint8(dev.(*DeviceValve).tempHot.Get()))

	g.Engine.TestDo(t, ctx, "evend.valve.set_temp_hot(73)")

	water, err := g.Inventory.Get("water")
	require.NoError(t, err)
	initial := (rand.Float32() - 0.5) * 10000 // check=false may go negative, not error
	t.Logf("water before=%f", initial)
	water.Set(initial)
	g.Engine.TestDo(t, ctx, "add.water_hot(90)")
	assert.Equal(t, initial-90, water.Value())

	{
		getTemp := g.Engine.Resolve("evend.valve.get_temp_hot")
		require.NotNil(t, getTemp)
		err := getTemp.Do(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sensor problem")
	}
}

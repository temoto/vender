package evend

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/state"
)

func TestEspresso(t *testing.T) {
	t.Parallel()

	ctx, g := state.NewTestContext(t, `
engine { inventory {
	stock "espresso" { register_add="ignore(?) mdb.evend.espresso_grind" spend_rate=7 }
}}`)
	mock := mdb.MockFromContext(ctx)
	defer mock.Close()
	go mock.Expect([]mdb.MockR{
		{"e8", ""},
		{"e9", "0800010100010e03d7070e0000000201"},
		{"eb", ""},
		{"ea01", ""},
		{"eb", ""},
	})
	d := new(DeviceEspresso)
	// TODO make small delay default in tests
	d.dev.DelayIdle = 1
	d.dev.DelayNext = 1
	d.dev.DelayReset = 1
	err := d.Init(ctx)
	if err != nil {
		t.Fatalf("Init err=%v", err)
	}

	stock, err := g.Inventory.Get("espresso")
	require.NoError(t, err)
	stock.Set(7)
	g.Engine.TestDo(t, ctx, "add.espresso(1)")
}

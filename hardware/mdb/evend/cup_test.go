package evend

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/state"
)

func TestCup(t *testing.T) {
	t.Parallel()

	ctx, g := state.NewTestContext(t, `
engine { inventory {
	stock "cup" { rate = 1 }
}}`)
	mock := mdb.MockFromContext(ctx)
	defer mock.Close()
	go mock.Expect([]mdb.MockR{
		{"e0", ""},
		{"e1", "06000b0100010a06d807362800000701"},
		{"e3", ""},
		{"e204", ""},
		{"e3", ""},
		{"e3", ""},
		{"e201", ""},
		{"e3", "50"},
		{"e3", ""},
	})
	d := new(DeviceCup)
	// TODO make small delay default in tests
	d.dev.DelayIdle = 1
	d.dev.DelayNext = 1
	d.dev.DelayReset = 1
	err := d.Init(ctx)
	if err != nil {
		t.Fatalf("Init err=%v", err)
	}

	source, err := g.Inventory.GetSource("cup")
	require.NoError(t, err)
	source.Set(7)
	g.Engine.TestDo(t, ctx, "@add.cup(1)")
}

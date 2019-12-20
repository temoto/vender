package evend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/hardware/mdb"
	state_new "github.com/temoto/vender/state/new"
)

func TestCup(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, "", `
engine {
inventory { stock "cup" { } }
alias "add.cup" { scenario = "mdb.evend.cup_dispense stock.cup.spend1" }
}
hardware { device "mdb.evend.cup" {} }`)
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
	require.NoError(t, Enum(ctx))

	stock, err := g.Inventory.Get("cup")
	require.NoError(t, err)
	stock.Set(7)
	g.Engine.TestDo(t, ctx, "add.cup")
	assert.Equal(t, float32(6), stock.Value())
}

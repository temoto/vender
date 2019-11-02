package evend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/hardware/mdb"
	state_new "github.com/temoto/vender/state/new"
)

func TestConveyor(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, "")
	mock := mdb.MockFromContext(ctx)
	defer mock.Close()
	go mock.Expect([]mdb.MockR{
		{"d8", ""},
		{"d9", "011810000a0000c8001fff01050a32640000000000000000000000"},

		// calibrate
		{"db", ""},
		{"da010000", ""},
		{"db", ""},
		// cup
		{"db", "04"},
		{"db", "04"},
		{"db", ""},
		{"da011806", ""},
		{"db", "50"},
		{"db", "50"},
		{"db", ""},

		{"db", ""},
		{"da016707", ""},
		{"db", "50"},
		{"db", ""},

		{"db", ""},
		{"da030400", ""},
		{"db", "50"},
		{"db", ""},

		// TODO test + handle it too
		// {"db", ""},
		// {"da016707", ""},
		// {"db", "54"}, // oops
	})
	d := new(DeviceConveyor)
	require.NoError(t, d.Init(ctx))

	assert.NoError(t, g.Engine.RegisterParse("conveyor_move_cup", "mdb.evend.conveyor_move(1560)"))
	assert.NoError(t, g.Engine.RegisterParse("conveyor_move_elevator", "mdb.evend.conveyor_move(1895)"))
	g.Engine.TestDo(t, ctx, "conveyor_move_cup")
	g.Engine.TestDo(t, ctx, "conveyor_move_elevator")
	g.Engine.TestDo(t, ctx, "mdb.evend.conveyor_shake(4)")
}

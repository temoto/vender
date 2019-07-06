package evend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/state"
)

func TestRegister(t *testing.T) {
	t.Parallel()

	ctx, g := state.NewTestContext(t, "")
	mock := mdb.MockFromContext(ctx)
	defer mock.Close()
	mock.ExpectMap(map[string]string{
		// relevant
		"40": "", "41": "", "d8": "", "d9": "",
		// irrelevant, only to reduce test log noise
		"48": "", "49": "", "50": "", "51": "", "58": "", "59": "", "60": "", "61": "",
		"68": "", "69": "", "70": "", "71": "", "78": "", "79": "",
		"c0": "", "c1": "", "c8": "", "c9": "", "d0": "", "d1": "", "e0": "", "e1": "", "e8": "", "e9": "",
	})
	Enum(ctx, enumIgnore)

	mock.ExpectMap(nil)
	go mock.Expect([]mdb.MockR{
		{"db", ""}, {"da010000", ""}, {"db", ""}, // conveyor calibrate / conveyor_move(0)
		{"db", ""}, {"da01fa00", ""}, {"db", ""}, // conveyor move to hopper
		{"43", ""}, {"420a", ""}, {"43", ""}, // hopper run
	})

	assert.NoError(t, g.Engine.RegisterParse("@hopper1(?)", "mdb.evend.conveyor_move(250) mdb.evend.hopper1_run(?)"))
	assert.NoError(t, g.Engine.RegisterParse("@conveyor_move_cup", "mdb.evend.conveyor_move(1560)"))
	assert.NoError(t, g.Engine.RegisterParse("@conveyor_move_elevator", "mdb.evend.conveyor_move(1895)"))

	g.Engine.TestDo(t, ctx, "@hopper1(10)")
}

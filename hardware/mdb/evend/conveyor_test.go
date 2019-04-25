package evend

import (
	"testing"

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/state"
)

func TestConveyor(t *testing.T) {
	t.Parallel()

	ctx := state.NewTestContext(t, "")
	mock := mdb.MockFromContext(ctx)
	defer mock.Close()
	go mock.Expect([]mdb.MockR{
		{"d8", ""},
		{"d9", "011810000a0000c8001fff01050a32640000000000000000000000"},

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

		// TODO test + handle it too
		// {"db", ""},
		// {"da016707", ""},
		// {"db", "54"}, // oops
	})
	d := new(DeviceConveyor)
	// TODO make small delay default in tests
	d.dev.DelayIdle = 1
	d.dev.DelayNext = 1
	d.dev.DelayReset = 1
	err := d.Init(ctx)
	if err != nil {
		t.Fatalf("Init err=%v", err)
	}

	engine.TestDo(t, ctx, "mdb.evend.conveyor_move_cup")
	engine.TestDo(t, ctx, "mdb.evend.conveyor_move_elevator")
}

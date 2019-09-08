package evend

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/hardware/mdb"
	state_new "github.com/temoto/vender/state/new"
)

func TestElevator(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, "")
	mock := mdb.MockFromContext(ctx)
	defer mock.Close()
	go mock.Expect([]mdb.MockR{
		{"d0", ""},
		{"d1", "04000b0100011805de07020000000a01"},

		// calibrate ok
		{"d3", ""},
		{"d2030000", ""},
		{"d3", "0d00"},

		// move(100) ok
		{"d3", ""},
		{"d2036400", ""},
		{"d3", "0d00"},

		// move(50) error before
		{"d3", "0427"},
		{"d0", ""},       // reset
		{"d3", ""},       // calibrate/wait-ready
		{"d2030000", ""}, // calibrate/move
		{"d3", "0d00"},   // calibrate/wait-done
		{"d3", ""},       // continue normal
		{"d2033200", ""},
		{"d3", "0d00"},

		// move(70) error after
		{"d3", ""},
		{"d2034600", ""},
		{"d3", ""},
		{"d3", "0427"},
		{"d0", ""},       // reset
		{"d3", ""},       // calibrate/wait-ready
		{"d2030000", ""}, // calibrate/move
		{"d3", "0d00"},   // calibrate/wait-done
		{"d3", ""},       // continue normal
		{"d2034600", ""},
		{"d3", "0d00"},
	})

	d := new(DeviceElevator)
	d.dev.XXX_FIXME_SetAllDelays(1) // TODO make small delay default in tests
	require.Nil(t, d.Init(ctx))

	g.Engine.TestDo(t, ctx, "mdb.evend.elevator_move(100)")
	g.Engine.TestDo(t, ctx, "mdb.evend.elevator_move(50)")
	g.Engine.TestDo(t, ctx, "mdb.evend.elevator_move(70)")
}

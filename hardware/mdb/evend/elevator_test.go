package evend

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/hardware/mdb"
	state_new "github.com/temoto/vender/internal/state/new"
)

func TestElevator(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, "", `hardware { device "evend.elevator" {} }`)
	mock := mdb.MockFromContext(ctx)
	defer mock.Close()
	go mock.Expect([]mdb.MockR{
		{"d0", ""},
		{"d1", "04000b0100011805de07020000000a01"},

		// move(100) ok
		{"d3", ""},
		{"d2036400", ""},
		{"d3", "0d00"},

		// move(50) requires cal0, ok
		{"d3", ""},
		{"d2030000", ""},
		{"d3", "0d00"},

		// move(50) error before
		{"d3", "0427"},
		{"d0", ""},       // reset
		{"d3", ""},       // calibrate/wait-ready
		{"d2030000", ""}, // calibrate/move
		{"d3", "0d00"},   // calibrate/wait-done
		{"d3", ""},       // calibrate/wait-ready
		{"d2036400", ""}, // calibrate/move
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
		{"d3", ""},       // calibrate/wait-ready
		{"d2036400", ""}, // calibrate/move
		{"d3", "0d00"},   // calibrate/wait-done
		{"d3", ""},       // continue normal
		{"d2034600", ""},
		{"d3", "0d00"},
	})
	require.NoError(t, Enum(ctx))

	g.Engine.TestDo(t, ctx, "evend.elevator.move(100)")
	g.Engine.TestDo(t, ctx, "evend.elevator.move(50)")
	g.Engine.TestDo(t, ctx, "evend.elevator.move(70)")
}

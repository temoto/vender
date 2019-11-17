package money

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/hardware"
	"github.com/temoto/vender/hardware/mdb"
	state_new "github.com/temoto/vender/state/new"
)

func TestAbort(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, `hardware{device "mdb.coin" {}} money{scale=100}`)
	mock := mdb.MockFromContext(ctx)
	defer mock.Close()
	mock.ExpectMap(map[string]string{
		"08":           "",
		"09":           "021643640200170102050a0a1900000000000000000000",
		"0f00":         "434f47303030303030303030303030463030313230303120202020029000000003",
		"0f0100000002": "",
		"0f05":         "01000600",
		"0a":           "0000110008",
		"0b":           "",
		"":             "",
	})

	require.NoError(t, hardware.Enum(ctx))
	ms := MoneySystem{}
	require.NoError(t, ms.Start(ctx))
	mock.ExpectMap(nil)

	ms.dirty += g.Config.ScaleU(11)
	go mock.Expect([]mdb.MockR{
		{"0f020b", ""},
		{"0f04", "00"},
		{"0f04", ""},
		{"0f03", "0b00"},
	})
	require.NoError(t, ms.Abort(ctx))

	mock.ExpectMap(map[string]string{
		"0c0000ffff": "",
	})
	require.NoError(t, ms.Stop(ctx))
}

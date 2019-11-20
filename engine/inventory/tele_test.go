package inventory_test

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	tele_api "github.com/temoto/vender/head/tele/api"
	state_new "github.com/temoto/vender/state/new"
)

func TestInventoryTele(t *testing.T) {
	t.Parallel()

	_, g := state_new.NewTestContext(t, `engine {
inventory {
	tele_add_name = true
	stock "drink" { code=7 }
  stock "snack" { min=1 }
  stock "water" { min=.5 }
}
}`)
	if water, err := g.Inventory.Get("water"); err != nil {
		require.NoError(t, err)
	} else {
		water.Set(1.8)
	}
	pb := g.Inventory.Tele()
	const expect1 = `stocks:<name:"snack" > stocks:<value:1 name:"water" valuef:1.8 > stocks:<code:7 name:"drink" > `
	require.Equal(t, expect1, proto.CompactTextString(pb))

	new := &tele_api.Inventory{}
	var err error

	// at least one unknown stock; expect: error, state not changed
	require.NoError(t, proto.UnmarshalText(`stocks:<name:"missing" >`, new))
	pb, err = g.Inventory.SetTele(new)
	require.Error(t, err)
	require.Equal(t, "stock name=missing code=0 not found", err.Error())
	require.Equal(t, expect1, proto.CompactTextString(pb))
	pb = g.Inventory.Tele()
	require.Equal(t, expect1, proto.CompactTextString(pb))

	// stock referenced by code; expect: OK, modified
	const expect2 = `stocks:<name:"snack" > stocks:<value:1 name:"water" valuef:1.8 > stocks:<code:7 value:2 name:"drink" valuef:2 > `
	require.NoError(t, proto.UnmarshalText(`stocks:<code:7 valuef:2 >`, new))
	pb, err = g.Inventory.SetTele(new)
	require.NoError(t, err)
	require.Equal(t, expect2, proto.CompactTextString(pb))
	pb = g.Inventory.Tele()
	require.Equal(t, expect2, proto.CompactTextString(pb))

	// stock value omited (zero); expect: OK, modified
	const expect3 = `stocks:<value:4 name:"snack" valuef:4.2 > stocks:<name:"water" > stocks:<code:7 value:2 name:"drink" valuef:2 > `
	require.NoError(t, proto.UnmarshalText(`stocks:<name:"snack" valuef:4.2 > stocks:<name:"water" >`, new))
	pb, err = g.Inventory.SetTele(new)
	require.NoError(t, err)
	require.Equal(t, expect3, proto.CompactTextString(pb))
	pb = g.Inventory.Tele()
	require.Equal(t, expect3, proto.CompactTextString(pb))
}

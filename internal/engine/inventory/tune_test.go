package inventory_test

import (
	"context"
	math "math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/internal/engine"
	state_new "github.com/temoto/vender/internal/state/new"
)

func TestTuneDrink(t *testing.T) {
	t.Parallel()

	rand := helpers.RandUnix()
	ctx, g := state_new.NewTestContext(t, "", `
engine {
inventory {
	stock "sugar" { register_add="sugar.drop(?)" spend_rate=0.98 }
	stock "tea" { register_add="tea.drop(?)" spend_rate=0.33 }
}
menu {
  item "1" { scenario="add.tea(15) add.sugar(4) add.tea(10)" }
}
}`)
	sugarStock, err := g.Inventory.Get("sugar")
	require.NoError(t, err)
	teaStock, err := g.Inventory.Get("tea")
	require.NoError(t, err)

	sugarInitial := rand.Float32() * (1 << 20)
	teaInitial := rand.Float32() * (1 << 20)
	sugarStock.Set(sugarInitial)
	teaStock.Set(teaInitial)
	hwSugar := make(chan engine.Arg, 2)
	hwTea := make(chan engine.Arg, 2)
	g.Engine.Register("sugar.drop(?)", engine.FuncArg{
		F: func(ctx context.Context, arg engine.Arg) error {
			hwSugar <- arg
			return nil
		}})
	g.Engine.Register("tea.drop(?)", engine.FuncArg{
		F: func(ctx context.Context, arg engine.Arg) error {
			hwTea <- arg
			return nil
		}})
	ctx, err = g.Inventory.WithTuning(ctx, "tea", 1.25)
	require.NoError(t, err)
	ctx, err = g.Inventory.WithTuning(ctx, "sugar", 0.25)
	require.NoError(t, err)

	menuDo := g.Config.Engine.Menu.Items[0].Doer
	// TODO make sure tuning survives transformations like Force
	require.NotNil(t, menuDo)
	require.NoError(t, menuDo.Validate())
	require.NoError(t, menuDo.Do(ctx))
	hwSugar1 := <-hwSugar
	hwTea1 := <-hwTea
	hwTea2 := <-hwTea
	sugarSpent := float32(math.Round(float64(hwSugar1) * 0.98))
	teaSpent := float32(math.Round(float64(hwTea1)*0.33) + math.Round(float64(hwTea2)*0.33))
	assert.Equal(t, engine.Arg(math.Round(4*0.25)), hwSugar1)
	assert.Equal(t, engine.Arg(math.Round(15*1.25)), hwTea1)
	assert.Equal(t, engine.Arg(math.Round(10*1.25)), hwTea2)
	assert.Equal(t, sugarInitial-sugarSpent, sugarStock.Value())
	assert.NotEqual(t, teaInitial-float32(math.Round(float64(hwTea1+hwTea2)*0.33)), teaStock.Value()) // without precision
	assert.Equal(t, teaInitial-teaSpent, teaStock.Value())                                            // with precision
}

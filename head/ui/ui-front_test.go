package ui

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/state"
)

func TestFrontSimple(t *testing.T) {
	t.Parallel()

	ctx, g := state.NewTestContext(t, `
engine {
	menu {
		item "1" { scenario = "" }
	}
}`)
	mock := mdb.MockFromContext(ctx)
	defer mock.Close()
	mock.ExpectMap(map[string]string{
		"": "",
	})
	moneysys := new(money.MoneySystem)
	err := moneysys.Start(ctx)
	require.NoError(t, err)
	env := &tenv{ctx: ctx, g: g}
	g.Config.UI.Front.MsgStateIntro = "hello simple"
	uiTestSetup(t, env, StateFrontBegin, StateFrontEnd)
	go env.ui.Loop(ctx)

	steps := []step{
		{expect: env._T("hello simple", " "), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyReject}},
		{expect: "", inev: input.Event{}},
	}
	uiTestWait(t, env, steps)
}

func TestFrontTune(t *testing.T) {
	t.Parallel()

	ctx, g := state.NewTestContext(t, `
engine {
	inventory {
		stock "cream" { register_add="ignore(?)" }
		stock "sugar" { register_add="ignore(?)" }
	}
	menu {
		item "1" { scenario = "add.cream(10) add.sugar(10)" }
	}
}
ui {
	front { reset_sec = 5 }
}`)
	mock := mdb.MockFromContext(ctx)
	defer mock.Close()
	mock.ExpectMap(map[string]string{
		"": "",
	})
	moneysys := new(money.MoneySystem)
	err := moneysys.Start(ctx)
	require.NoError(t, err)
	env := &tenv{ctx: ctx, g: g}
	g.Config.UI.Front.MsgStateIntro = "hello tune"
	uiTestSetup(t, env, StateFrontBegin, StateFrontEnd)
	go env.ui.Loop(ctx)
	creamStock := g.Inventory.MustGet(t, "cream")
	creamStock.Set(100)
	sugarStock := g.Inventory.MustGet(t, "sugar")
	sugarStock.Set(200)

	steps := []step{
		{expect: env._T("hello tune", " "), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyCreamMore}},
		{expect: env._T(fmt.Sprintf("%s  /5", msgCream), "   - \x97\x97\x97\x97\x97\x94 +   "), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeySugarLess}},
		{expect: env._T(fmt.Sprintf("%s  /3", msgSugar), "   - \x97\x97\x95\x94\x94\x94 +   "), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeySugarLess}},
		{expect: env._T(fmt.Sprintf("%s  /2", msgSugar), "   - \x97\x96\x94\x94\x94\x94 +   "), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: '1'}},
		{expect: env._T(fmt.Sprintf("%s0", msgCredit), fmt.Sprintf(msgInputCode, "1")), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyAccept}},
		{expect: env._T(msgMaking1, msgMaking2), inev: input.Event{}},
		{expect: "", inev: input.Event{}},
	}
	uiTestWait(t, env, steps)
	assert.Equal(t, float32(100-13), creamStock.Value())
	assert.Equal(t, float32(200-5), sugarStock.Value())
}

// This test ensures particular behavior of currently operated coffee machine tuning.
func TestScaleTuneRate(t *testing.T) {
	t.Run("cream", func(t *testing.T) {
		f := func(v uint8) float32 { return scaleTuneRate(v, MaxCream, DefaultCream) }
		cases := []float32{0, 0.25, 0.5, 0.75, 1, 1.25, 1.5}
		for input, expect := range cases {
			assert.Equal(t, expect, f(uint8(input)))
		}
		for i := uint8(0); i < 10; i++ {
			assert.Equal(t, cases[len(cases)-1], f(uint8(len(cases))+i))
		}
	})
	t.Run("sugar", func(t *testing.T) {
		f := func(v uint8) float32 { return scaleTuneRate(v, MaxSugar, DefaultSugar) }
		cases := []float32{0, 0.25, 0.5, 0.75, 1, 1.25, 1.5, 1.75, 2}
		for input, expect := range cases {
			assert.Equal(t, expect, f(uint8(input)))
		}
		for i := uint8(0); i < 10; i++ {
			assert.Equal(t, cases[len(cases)-1], f(uint8(len(cases))+i))
		}
	})
}

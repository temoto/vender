package ui_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/internal/money"
	state_new "github.com/temoto/vender/internal/state/new"
	"github.com/temoto/vender/internal/types"
	"github.com/temoto/vender/internal/ui"
	tele_api "github.com/temoto/vender/tele"
)

func TestFrontTune(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, "", `
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
	moneysys := new(money.MoneySystem)
	require.NoError(t, moneysys.Start(ctx))
	env := &tenv{ctx: ctx, g: g}
	g.Config.UI.Front.MsgStateIntro = "hello tune"
	uiTestSetup(t, env, ui.StateFrontBegin, ui.StateFrontEnd)
	go env.ui.Loop(ctx)
	creamStock := g.Inventory.MustGet(t, "cream")
	creamStock.Set(100)
	sugarStock := g.Inventory.MustGet(t, "sugar")
	sugarStock.Set(200)

	steps := []step{
		{expect: env._T("hello tune", " "), inev: env._Key(input.EvendKeyCreamMore)},
		{expect: env._T(fmt.Sprintf("%s  /5", ui.MsgCream), "   - \x97\x97\x97\x97\x97\x94 +   "), inev: env._Key(input.EvendKeySugarLess)},
		{expect: env._T(fmt.Sprintf("%s  /3", ui.MsgSugar), "   - \x97\x97\x95\x94\x94\x94 +   "), inev: env._Key(input.EvendKeySugarLess)},
		{expect: env._T(fmt.Sprintf("%s  /2", ui.MsgSugar), "   - \x97\x96\x94\x94\x94\x94 +   "), inev: env._Key('1')},
		{expect: env._T(fmt.Sprintf("%s0", ui.MsgCredit), fmt.Sprintf(ui.MsgInputCode, "1")), inev: env._KeyAccept},
		{expect: env._T(ui.MsgMaking1, ui.MsgMaking2), inev: env._Timeout},
		{},
	}
	uiTestWait(t, env, steps)
	assert.Equal(t, float32(100-13), creamStock.Value())
	assert.Equal(t, float32(200-5), sugarStock.Value())
}

func TestFrontMoneyAbort(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, "", `
engine {
	inventory {
		stock "cream" { register_add="ignore(?)" }
	}
	menu {
		item "1" { price=7 scenario = "add.cream(10)" }
	}
}
ui {
	front { reset_sec = 5 }
}`)
	moneysys := new(money.MoneySystem)
	require.NoError(t, moneysys.Start(ctx))
	env := &tenv{ctx: ctx, g: g}
	g.Config.UI.Front.MsgStateIntro = "money-abort"
	uiTestSetup(t, env, ui.StateFrontBegin, ui.StateFrontEnd)
	go env.ui.Loop(ctx)
	creamStock := g.Inventory.MustGet(t, "cream")
	creamStock.Set(100)

	steps := []step{
		{expect: env._T("money-abort", ""), inev: env._Key(input.EvendKeyCreamMore)},
		{expect: env._T(fmt.Sprintf("%s  /5", ui.MsgCream), "   - \x97\x97\x97\x97\x97\x94 +   "), inev: env._Key('1')},
		{expect: env._T(fmt.Sprintf("%s0", ui.MsgCredit), fmt.Sprintf(ui.MsgInputCode, "1")), inev: env._MoneyAbort},
		{}, // MoneyKeyAbort -> ui.StateFrontEnd
	}
	uiTestWait(t, env, steps)
}

func TestFrontLock(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, "", `
engine {
	menu {
		item "1" { price=7 scenario = "" }
	}
}
ui {
	front {
		msg_intro = "hello,world"
		msg_locked = "locked,wait"
		reset_sec = 5
	}
}`)
	moneysys := new(money.MoneySystem)
	require.NoError(t, moneysys.Start(ctx))
	env := &tenv{ctx: ctx, g: g}
	uiTestSetup(t, env, ui.StateFrontBegin, ui.StateFrontAccept)
	go env.ui.Loop(ctx)

	steps := []step{
		{expect: env._T("hello,world", ""), inev: types.Event{Kind: types.EventTime}},
		{fun: func() { assert.True(t, env.ui.LockWait(tele_api.Priority_Now)) }},
		{expect: env._T("locked,wait", ""), inev: types.Event{Kind: types.EventTime}},
		{fun: func() { env.ui.LockEnd() }},
		{expect: env._T("hello,world", ""), inev: types.Event{Kind: types.EventStop}},
		{},
	}
	uiTestWait(t, env, steps)
}

func TestFrontPay(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, "", `
engine {
	menu {
		item "1" { price=7 scenario = "" }
	}
}
money {
	credit_max = 50
	scale = 100
}
ui {
	front {
		msg_intro = "please buy"
		reset_sec = 5
	}
}`)
	moneysys := new(money.MoneySystem)
	require.NoError(t, moneysys.Start(ctx))
	env := &tenv{ctx: ctx, g: g, uiState: make(chan ui.State, 1)}
	uiTestSetup(t, env, ui.StateFrontBegin, ui.StateFrontEnd)
	go env.ui.Loop(ctx)

	env.requireState(t, ui.StateFrontSelect)
	env.requireDisplay(t, "please buy", "")
	require.NoError(t, moneysys.XXX_InjectCoin(500))
	env.g.Hardware.Input.Emit(env._Key('1').Input)
	env.requireDisplay(t, ui.MsgCredit+"5", fmt.Sprintf(ui.MsgInputCode, "1"))
	env.g.Hardware.Input.Emit(env._KeyAccept.Input)
	env.requireDisplay(t, "", ui.MsgMenuInsufficientCredit)
	require.NoError(t, moneysys.XXX_InjectCoin(200))
	env.g.Hardware.Input.Emit(env._KeyAccept.Input)
	env.requireState(t, ui.StateFrontAccept)
	env.requireDisplay(t, ui.MsgMaking1, ui.MsgMaking2)
	env.requireState(t, ui.StateFrontEnd)
	if assert.False(t, env.g.Alive.IsRunning(), "ui still running") {
		env.g.Alive.Wait()
	}
}

// This test ensures particular behavior of currently operated coffee machine tuning.
func TestScaleTuneRate(t *testing.T) {
	t.Run("cream", func(t *testing.T) {
		f := func(v uint8) float32 { return ui.ScaleTuneRate(v, ui.MaxCream, ui.DefaultCream) }
		cases := []float32{0, 0.25, 0.5, 0.75, 1, 1.25, 1.5}
		for input, expect := range cases {
			assert.Equal(t, expect, f(uint8(input)))
		}
		for i := uint8(0); i < 10; i++ {
			assert.Equal(t, cases[len(cases)-1], f(uint8(len(cases))+i))
		}
	})
	t.Run("sugar", func(t *testing.T) {
		f := func(v uint8) float32 { return ui.ScaleTuneRate(v, ui.MaxSugar, ui.DefaultSugar) }
		cases := []float32{0, 0.25, 0.5, 0.75, 1, 1.25, 1.5, 1.75, 2}
		for input, expect := range cases {
			assert.Equal(t, expect, f(uint8(input)))
		}
		for i := uint8(0); i < 10; i++ {
			assert.Equal(t, cases[len(cases)-1], f(uint8(len(cases))+i))
		}
	})
}

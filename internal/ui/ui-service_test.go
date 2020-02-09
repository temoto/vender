package ui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/internal/money"
	state_new "github.com/temoto/vender/internal/state/new"
	"github.com/temoto/vender/internal/ui"
)

func TestServiceAuth(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, "", "")
	env := &tenv{ctx: ctx, g: g}
	g.Config.UI.Service.Auth.Enable = true
	g.Config.UI.Service.Auth.Passwords = []string{"lemz1g"}
	uiTestSetup(t, env, ui.StateServiceBegin, ui.StateServiceEnd)
	env.ui.Service.SecretSalt = []byte("test")
	go env.ui.Loop(ctx)

	steps := []step{
		{expect: env._T(" ", "\x8d fflcrq\x00"), inev: env._Key('1')},
		{expect: env._T(" ", "\x8d qtky0g\x00"), inev: env._Key('9')},
		{expect: env._T(" ", "\x8d nfiinw\x00"), inev: env._Key('7')},
		{expect: env._T(" ", "\x8d 2grymg\x00"), inev: env._Key('0')},
		{expect: env._T(" ", "\x8d lemz1g\x00"), inev: env._KeyAccept},
		{expect: env._T("Menu", "1 inventory"), inev: env._KeyReject},
		{},
	}
	uiTestWait(t, env, steps)
}

func TestServiceMenu(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, "", `
ui { service {
	test "first" { scenario="" }
}}`)
	env := &tenv{ctx: ctx, g: g}
	g.Config.UI.Service.Auth.Enable = false
	uiTestSetup(t, env, ui.StateServiceBegin, ui.StateServiceEnd)
	go env.ui.Loop(ctx)

	steps := []step{
		{expect: env._T("Menu", "1 inventory"), inev: env._KeyNext},
		{expect: env._T("Menu", "2 test"), inev: env._KeyNext},
		{expect: env._T("Menu", "3 reboot"), inev: env._KeyPrev},
		{expect: env._T("Menu", "2 test"), inev: env._KeyNext},
		{expect: env._T("Menu", "3 reboot"), inev: env._KeyNext},
		{expect: env._T("Menu", "4 network"), inev: env._KeyNext},
		{expect: env._T("Menu", "5 money-load"), inev: env._KeyNext},
		{expect: env._T("Menu", "6 report"), inev: env._KeyNext},
		{expect: env._T("Menu", "1 inventory"), inev: env._KeyReject},
		{},
	}
	uiTestWait(t, env, steps)
}

func TestServiceInventory(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, "", `
engine { inventory {
	stock "cup" { code=3 }
	stock "water" { code=4 }
}}`)
	env := &tenv{ctx: ctx, g: g}
	g.Config.UI.Service.Auth.Enable = false
	uiTestSetup(t, env, ui.StateServiceBegin, ui.StateServiceEnd)
	go env.ui.Loop(ctx)

	steps := []step{
		{expect: env._T("Menu", "1 inventory"), inev: env._KeyAccept},
		{expect: env._T("I3 cup", "0.0 \x00"), inev: env._Key('3')},
		{expect: env._T("I3 cup", "0.0 3\x00"), inev: env._Key('2')},
		{expect: env._T("I3 cup", "0.0 32\x00"), inev: env._KeyAccept},
		{expect: env._T("I3 cup", "32.0 \x00"), inev: env._KeyNext},
		{expect: env._T("I4 water", "0.0 \x00"), inev: env._Key('1')},
		{expect: env._T("I4 water", "0.0 1\x00"), inev: env._Key('7')},
		{expect: env._T("I4 water", "0.0 17\x00"), inev: env._Key('.')},
		{expect: env._T("I4 water", "0.0 17.\x00"), inev: env._Key('5')},
		{expect: env._T("I4 water", "0.0 17.5\x00"), inev: env._KeyAccept},
		{expect: env._T("I4 water", "17.5 \x00"), inev: env._KeyNext},
		{expect: env._T("I3 cup", "32.0 \x00"), inev: env._KeyReject},
		{expect: env._T("Menu", "1 inventory"), inev: env._KeyReject},
		{},
	}
	uiTestWait(t, env, steps)
}

func TestServiceTest(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, "", `
ui { service {
	test "first" { scenario="" }
	test "second" { scenario="" }
}}`)
	env := &tenv{ctx: ctx, g: g}
	g.Config.UI.Service.Auth.Enable = false
	uiTestSetup(t, env, ui.StateServiceBegin, ui.StateServiceEnd)
	go env.ui.Loop(ctx)

	steps := []step{
		{expect: env._T("Menu", "1 inventory"), inev: env._KeyNext},
		{expect: env._T("Menu", "2 test"), inev: env._KeyAccept},
		{expect: env._T("T1 first", " "), inev: env._KeyNext},
		{expect: env._T("T2 second", " "), inev: env._KeyAccept},
		{expect: env._T("T2 second", "in progress")},
		{expect: env._T("T2 second", "OK"), inev: env._KeyReject},
		{expect: env._T("Menu", "2 test"), inev: env._KeyReject},
		{},
	}
	uiTestWait(t, env, steps)
}

func TestServiceReboot(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, "", `engine {}`)
	env := &tenv{ctx: ctx, g: g, uiState: make(chan ui.State, 1)}
	g.Config.UI.Service.Auth.Enable = false
	uiTestSetup(t, env, ui.StateServiceBegin, ui.StateStop)
	go env.ui.Loop(ctx)

	env.requireState(t, ui.StateServiceAuth)
	env.requireState(t, ui.StateServiceMenu)
	env.requireDisplay(t, "Menu", "1 inventory")
	env.g.Hardware.Input.Emit(env._KeyNext.Input)
	env.requireState(t, ui.StateServiceMenu)
	env.requireDisplay(t, "Menu", "2 test")
	env.g.Hardware.Input.Emit(env._KeyNext.Input)
	env.requireState(t, ui.StateServiceMenu)
	env.requireDisplay(t, "Menu", "3 reboot")
	env.g.Hardware.Input.Emit(env._KeyAccept.Input)
	env.requireState(t, ui.StateServiceReboot)
	env.requireDisplay(t, "for reboot", "press 1")
	env.g.Hardware.Input.Emit(env._Key('1').Input)
	// can't requireState because g.Stop may have stopped ui.Loop
	env.requireDisplay(t, "reboot", "in progress")
	env.g.Alive.Wait()
}

func TestServiceMoneyLoad(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, "", `engine {} money { credit_max=5000 }`)
	moneysys := new(money.MoneySystem)
	require.NoError(t, moneysys.Start(ctx))
	env := &tenv{ctx: ctx, g: g, uiState: make(chan ui.State, 1)}
	g.Config.UI.Service.Auth.Enable = false
	uiTestSetup(t, env, ui.StateServiceMoneyLoad, ui.StateServiceEnd)
	go env.ui.Loop(ctx)

	env.requireDisplay(t, "money-load", "0")
	require.NoError(t, moneysys.XXX_InjectCoin(200))
	env.g.Hardware.Input.Emit(env._Key('.').Input) // FIXME XXX_InjectCoin must emit EventMoneyCredit
	env.requireDisplay(t, "money-load", "2")
	env.g.Hardware.Input.Emit(env._KeyReject.Input)
	env.requireState(t, ui.StateServiceMenu)
	env.requireDisplay(t, "Menu", "1 inventory")
	assert.Equal(t, currency.Amount(0), moneysys.Credit(ctx))
	env.g.Hardware.Input.Emit(env._KeyReject.Input)
	env.requireState(t, ui.StateServiceEnd)
	env.g.Alive.Wait()
}

func TestServiceReport(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, "", `engine {}`)
	env := &tenv{ctx: ctx, g: g}
	g.Config.UI.Service.Auth.Enable = false
	uiTestSetup(t, env, ui.StateServiceBegin, ui.StateServiceEnd)
	go env.ui.Loop(ctx)

	steps := []step{
		{expect: env._T("Menu", "1 inventory"), inev: env._KeyNext},
		{expect: env._T("Menu", "2 test"), inev: env._KeyNext},
		{expect: env._T("Menu", "3 reboot"), inev: env._KeyNext},
		{expect: env._T("Menu", "4 network"), inev: env._KeyNext},
		{expect: env._T("Menu", "5 money-load"), inev: env._KeyNext},
		{expect: env._T("Menu", "6 report"), inev: env._KeyAccept},
		{expect: env._T("Menu", "6 report"), inev: env._KeyReject},
		{},
	}
	uiTestWait(t, env, steps)
}

func TestVisualHash(t *testing.T) {
	t.Parallel()

	type Case struct {
		input  string
		salt   string
		expect string
	}
	testSalt := "\xfe"
	cases := []Case{
		{"", testSalt, "03fjzq"},
		{"1111", testSalt, "7my0oq"},
		{"1234", testSalt, "oxvktq"},
	}
	for _, c := range cases {
		c := c
		t.Run("input="+c.input, func(t *testing.T) {
			result := ui.VisualHash([]byte(c.input), []byte(c.salt))
			assert.Equal(t, c.expect, result)
		})
	}
}

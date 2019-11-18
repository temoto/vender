package ui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/temoto/vender/head/ui"
	state_new "github.com/temoto/vender/state/new"
)

func TestServiceAuth(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, "")
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

func TestServiceInventory(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, `
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

	ctx, g := state_new.NewTestContext(t, `
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
		{expect: env._T("T2 second", "in progress"), inev: ui.Event{}},
		{expect: env._T("T2 second", "OK"), inev: env._KeyReject},
		{expect: env._T("Menu", "2 test"), inev: env._KeyReject},
		{},
	}
	uiTestWait(t, env, steps)
}

func TestServiceReboot(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, `engine {}`)
	env := &tenv{ctx: ctx, g: g}
	g.Config.UI.Service.Auth.Enable = false
	uiTestSetup(t, env, ui.StateServiceBegin, ui.StateServiceEnd)
	go env.ui.Loop(ctx)

	steps := []step{
		{expect: env._T("Menu", "1 inventory"), inev: env._KeyNext},
		{expect: env._T("Menu", "2 test"), inev: env._KeyNext},
		{expect: env._T("Menu", "3 reboot"), inev: env._KeyAccept},
		{expect: env._T("for reboot", "press 1"), inev: env._Key('1')},
		{expect: env._T("reboot", "in progress"), inev: ui.Event{}},
	}
	uiTestWait(t, env, steps)
}

func TestServiceReport(t *testing.T) {
	t.Parallel()

	ctx, g := state_new.NewTestContext(t, `engine {}`)
	env := &tenv{ctx: ctx, g: g}
	g.Config.UI.Service.Auth.Enable = false
	uiTestSetup(t, env, ui.StateServiceBegin, ui.StateServiceEnd)
	go env.ui.Loop(ctx)

	steps := []step{
		{expect: env._T("Menu", "1 inventory"), inev: env._KeyNext},
		{expect: env._T("Menu", "2 test"), inev: env._KeyNext},
		{expect: env._T("Menu", "3 reboot"), inev: env._KeyNext},
		{expect: env._T("Menu", "4 network"), inev: env._KeyNext},
		{expect: env._T("Menu", "5 report"), inev: env._KeyAccept},
		{expect: env._T("Menu", "5 report"), inev: env._KeyReject},
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

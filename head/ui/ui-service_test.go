package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/state"
)

func TestServiceAuth(t *testing.T) {
	t.Parallel()

	ctx, g := state.NewTestContext(t, "")
	env := &tenv{ctx: ctx, g: g}
	g.Config.UI.Service.Auth.Enable = true
	g.Config.UI.Service.Auth.Passwords = []string{"lemz1g"}
	uiTestSetup(t, env, StateServiceBegin, StateServiceEnd)
	env.ui.service.secretSalt = []byte("test")
	go env.ui.Loop(ctx)

	steps := []step{
		{expect: env._T(" ", "\x8d fflcrq\x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: '1'}},
		{expect: env._T(" ", "\x8d qtky0g\x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: '9'}},
		{expect: env._T(" ", "\x8d nfiinw\x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: '7'}},
		{expect: env._T(" ", "\x8d 2grymg\x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: '0'}},
		{expect: env._T(" ", "\x8d lemz1g\x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyAccept}},
		{expect: env._T("Menu", "1 inventory"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyReject}},
		{expect: "", inev: input.Event{}},
	}
	uiTestWait(t, env, steps)
}

func TestServiceInventory(t *testing.T) {
	t.Parallel()

	ctx, g := state.NewTestContext(t, `
engine { inventory {
	stock "cup" { }
	stock "water" { }
}}`)
	env := &tenv{ctx: ctx, g: g}
	g.Config.UI.Service.Auth.Enable = false
	uiTestSetup(t, env, StateServiceBegin, StateServiceEnd)
	go env.ui.Loop(ctx)

	steps := []step{
		{expect: env._T("Menu", "1 inventory"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyAccept}},
		{expect: env._T("I1 cup", "0 \x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: '3'}},
		{expect: env._T("I1 cup", "0 3\x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: '2'}},
		{expect: env._T("I1 cup", "0 32\x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyAccept}},
		{expect: env._T("I1 cup", "32 \x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyCreamMore}},
		{expect: env._T("I2 water", "0 \x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: '7'}},
		{expect: env._T("I2 water", "0 7\x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: '5'}},
		{expect: env._T("I2 water", "0 75\x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: '0'}},
		{expect: env._T("I2 water", "0 750\x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyAccept}},
		{expect: env._T("I2 water", "750 \x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyCreamMore}},
		{expect: env._T("I1 cup", "32 \x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyReject}},
		{expect: env._T("Menu", "1 inventory"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyReject}},
		{expect: "", inev: input.Event{}},
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
			result := visualHash([]byte(c.input), []byte(c.salt))
			assert.Equal(t, c.expect, result)
		})
	}
}

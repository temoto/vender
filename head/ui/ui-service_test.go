package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/alive"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/state"
)

func TestServiceInventory(t *testing.T) {
	t.Parallel()

	const width = 16
	ctx, g := state.NewTestContext(t, "")
	g.Config().UI.Service.Auth.Enable = false
	display, displayMock := lcd.NewMockTextDisplay(width, "", 0)
	g.Hardware.HD44780.Display = display
	g.Inventory.Register("water", 1)
	g.Inventory.Register("cup", 1)
	ui := NewUIService(ctx)
	a := alive.NewAlive()
	displayUpdated := make(chan struct{})
	display.SetUpdateChan(displayUpdated)
	stopch := a.StopChan()
	go ui.Run(a)

	type Step struct {
		expect string
		inev   input.Event
	}
	_T := func(l1, l2 string) string {
		return fmt.Sprintf("%s\n%s", display.Translate(l1), display.Translate(l2))
	}
	steps := []Step{
		{expect: _T("Menu", "1 inventory"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyAccept}},
		{expect: _T("I1 cup", "0 \x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: '3'}},
		{expect: _T("I1 cup", "0 3\x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyAccept}},
		{expect: _T("I1 cup", "3 \x00"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyReject}},
		{expect: _T("Menu", "1 inventory"), inev: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyReject}},
		{expect: "", inev: input.Event{}},
	}
expectLoop:
	for _, step := range steps {
		select {
		case <-displayUpdated:
		case <-stopch:
			if !(step.expect == "" && step.inev.IsZero()) {
				t.Error("ui stopped before end of test")
			}
			break expectLoop
		}
		current := displayMock.String()
		t.Logf("display:\n%s\n%s\ninput=%v", current, strings.Repeat("-", width), step.inev)
		require.Equal(t, step.expect, current)
		g.Hardware.Input.Emit(step.inev)
	}
	if a.IsRunning() {
		t.Logf("display:\n%s", displayMock.String())
		t.Error("ui still running")
	}
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

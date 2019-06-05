package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/temoto/alive"
	keyboard "github.com/temoto/vender/hardware/evend-keyboard"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

func TestServiceInventory(t *testing.T) {
	t.Parallel()

	const width = 16
	ctx, g := state.NewTestContext(t, "")
	g.Config().UI.Service.Auth.Enable = false
	kb := keyboard.NewMockKeyboard(0)
	g.Hardware.Keyboard.Device = kb
	display, displayMock := lcd.NewMockTextDisplay(width, "", 0)
	g.Hardware.HD44780.Display = display
	g.Inventory.Register("water", 1)
	g.Inventory.Register("cup", 1)
	ui := NewUIService(ctx)
	a := alive.NewAlive()
	go ui.Run(a)
	stopch := a.StopChan()
	displayUpdated := make(chan struct{})
	display.SetUpdateChan(displayUpdated)

	type Step struct {
		expect string
		input  keyboard.Key
	}
	_T := func(l1, l2 string) string {
		return fmt.Sprintf("%s\n%s", display.Translate(l1), display.Translate(l2))
	}
	steps := []Step{
		{expect: _T("Menu", "1 inventory"), input: keyboard.KeyAccept},
		{expect: _T("I1 cup", "0 \x00"), input: '3'},
		{expect: _T("I1 cup", "0 3\x00"), input: keyboard.KeyAccept},
		{expect: _T("I1 cup", "3 \x00"), input: keyboard.KeyReject},
		{expect: _T("Menu", "1 inventory"), input: keyboard.KeyReject},
		{expect: "", input: 0},
	}
expectLoop:
	for _, step := range steps {
		select {
		case <-displayUpdated:
		case <-stopch:
			if !(step.expect == "" && step.input == 0) {
				t.Error("ui stopped before end of test")
			}
			break expectLoop
		}
		current := displayMock.String()
		t.Logf("display:\n%s\n%s\ninput=%d", current, strings.Repeat("-", width), step.input)
		helpers.AssertEqual(t, current, step.expect)
		kb.C <- step.input
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
			helpers.AssertEqual(t, result, c.expect)
		})
	}
}

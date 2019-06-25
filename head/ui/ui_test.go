package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/temoto/alive"
	"github.com/temoto/errors"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

func TestFrontUILoop(t *testing.T) {
	t.Parallel()

	ctx, g := state.NewTestContext(t, "")
	g.Config().Engine.Menu.ResetTimeoutSec = 1
	const width = 16
	display, _ := lcd.NewMockTextDisplay(width, "", 0)
	g.Hardware.HD44780.Display = display

	menuMap := make(Menu)
	if err := menuMap.Init(ctx); err != nil {
		t.Fatalf("menuMap.Init err=%v", errors.ErrorStack(err))
	}
	counter := 0
	uiFront := NewUIFront(ctx, menuMap)
	uiFrontRunner := &state.FuncRunner{Name: "ui-front", F: func(a *alive.Alive) {
		time.AfterFunc(300*time.Millisecond, a.Stop)
		frontResult := uiFront.Run(a)
		t.Logf("uiFront result=%#v", frontResult)
		counter++
	}}

	g.UINext(uiFrontRunner)
	// g.UINext(uiFrontRunner)
	// require.Equal(t, counter, 2)
}

func TestFormatScale(t *testing.T) {
	t.Parallel()

	type Case struct {
		value  uint8
		min    uint8
		max    uint8
		expect string
	}
	alpha := []byte{'0', '1', '2', '3'}
	cases := []Case{
		{0, 0, 0, "000000"},
		{1, 0, 7, "300000"},
		{2, 0, 7, "320000"},
		{3, 0, 7, "332000"},
		{4, 0, 7, "333100"},
		{5, 0, 7, "333310"},
		{6, 0, 7, "333330"},
		{7, 0, 7, "333333"},
	}

	for _, c := range cases {
		c := c
		t.Run(fmt.Sprintf("scale:%d[%d..%d]", c.value, c.min, c.max), func(t *testing.T) {
			result := string(formatScale(c.value, c.min, c.max, alpha))
			helpers.AssertEqual(t, c.expect, result)
		})
	}
}

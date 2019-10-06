package ui_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/head/ui"
	"github.com/temoto/vender/state"
)

const testDisplayWidth = 16

type tenv struct {
	ctx context.Context
	g   *state.Global
	ui  *ui.UI

	display        *lcd.TextDisplay
	displayUpdated chan lcd.State
	_T             func(l1, l2 string) string
	_Key           func(input.Key) ui.Event
	_KeyAccept     ui.Event
	_KeyReject     ui.Event
	_KeyNext       ui.Event
	_KeyPrev       ui.Event
	_KeyService    ui.Event
	_MoneyAbort    ui.Event
	_Timeout       ui.Event
}

type step struct {
	expect string
	inev   ui.Event
	fun    func()
}

func uiTestSetup(t testing.TB, env *tenv, initState, endState ui.State) {
	env.display = lcd.NewMockTextDisplay(&lcd.TextDisplayConfig{Width: testDisplayWidth})
	env.g.Hardware.HD44780.Display = env.display
	env.ui = &ui.UI{
		XXX_testHook: func(s ui.State) {
			t.Logf("testHook %s", s.String())
			switch s {
			case endState: // success path
				env.g.Alive.Stop()
			case ui.StateInvalid:
				t.Fatalf("ui switch state=invalid")
				env.g.Alive.Stop()
			}
		},
	}
	err := env.ui.Init(env.ctx)
	require.NoError(t, err)
	env.ui.State = initState
	env.displayUpdated = make(chan lcd.State)
	env.display.SetUpdateChan(env.displayUpdated)
	env._T = func(l1, l2 string) string {
		return fmt.Sprintf("%s\n%s",
			lcd.PadSpace(env.display.Translate(l1), testDisplayWidth),
			lcd.PadSpace(env.display.Translate(l2), testDisplayWidth),
		)
	}
	env._Key = func(k input.Key) ui.Event {
		return ui.Event{Kind: ui.EventInput, Input: input.Event{Source: input.EvendKeyboardSourceTag, Key: k}}
	}
	env._KeyAccept = ui.Event{Kind: ui.EventInput, Input: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyAccept}}
	env._KeyReject = ui.Event{Kind: ui.EventInput, Input: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyReject}}
	env._KeyNext = ui.Event{Kind: ui.EventInput, Input: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyCreamMore}}
	env._KeyPrev = ui.Event{Kind: ui.EventInput, Input: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyCreamLess}}
	env._KeyService = ui.Event{Kind: ui.EventInput, Input: input.Event{Source: input.DevInputEventTag}}
	env._MoneyAbort = ui.Event{Kind: ui.EventInput, Input: input.Event{Source: input.MoneySourceTag, Key: input.MoneyKeyAbort}}
	env._Timeout = ui.Event{Kind: ui.EventTime}
}

func uiTestWait(t testing.TB, env *tenv, steps []step) {
	waitch := env.g.Alive.WaitChan()

	for _, step := range steps {
		if step.fun != nil {
			step.fun()
			continue
		}

		select {
		case current := <-env.displayUpdated:
			t.Logf("display:\n%s\n%s\nevent=%s", current, strings.Repeat("-", testDisplayWidth), step.inev.String())
			require.Equal(t, step.expect, current.Format(testDisplayWidth))
			switch step.inev.Kind {
			case ui.EventInput:
				env.g.Hardware.Input.Emit(step.inev.Input)

			case ui.EventStop:
				env.g.Log.Debugf("emit ui.EventStop")
				env.g.Alive.Stop()
				env.g.Alive.Wait()
				return
			}

		case <-waitch:
			if !(step.expect == "" && step.inev.Kind == ui.EventInvalid) {
				t.Error("ui stopped before end of test")
			}
			return
		}
	}
	if env.g.Alive.IsRunning() {
		t.Logf("display:\n%s", env.display.State().Format(testDisplayWidth))
		t.Error("ui still running")
	}
}

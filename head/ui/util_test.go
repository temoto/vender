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
	"github.com/temoto/vender/internal/types"
	"github.com/temoto/vender/state"
)

const testDisplayWidth = 16

type tenv struct {
	ctx context.Context
	g   *state.Global
	ui  *ui.UI

	display        *lcd.TextDisplay
	displayUpdated chan lcd.State
	uiState        chan ui.State
	_T             func(l1, l2 string) string
	_Key           func(types.InputKey) types.Event
	_KeyAccept     types.Event
	_KeyReject     types.Event
	_KeyNext       types.Event
	_KeyPrev       types.Event
	_KeyService    types.Event
	_MoneyAbort    types.Event
	_Timeout       types.Event
}

type step struct {
	expect string
	inev   types.Event
	fun    func()
}

func uiTestSetup(t testing.TB, env *tenv, initState, endState ui.State) {
	env.display = lcd.NewMockTextDisplay(&lcd.TextDisplayConfig{Width: testDisplayWidth})
	env.g.Hardware.HD44780.Display = env.display
	env.ui = &ui.UI{
		XXX_testHook: func(s ui.State) {
			t.Logf("testHook %s", s.String())
			if env.uiState != nil {
				select {
				case env.uiState <- s:
				default:
					t.Fatalf("add requireState(%s)", s.String())
				}
			}
			switch s {
			case endState: // success path
				env.g.Alive.Stop()
			case ui.StateDefault:
				t.Fatalf("ui switch state=default")
				env.g.Alive.Stop()
			}
		},
	}
	err := env.ui.Init(env.ctx)
	require.NoError(t, err)
	env.ui.XXX_testSetState(initState)
	env.displayUpdated = make(chan lcd.State)
	env.display.SetUpdateChan(env.displayUpdated)
	env._T = func(l1, l2 string) string {
		return fmt.Sprintf("%s\n%s",
			lcd.PadSpace(env.display.Translate(l1), testDisplayWidth),
			lcd.PadSpace(env.display.Translate(l2), testDisplayWidth),
		)
	}
	env._Key = func(k types.InputKey) types.Event {
		return types.Event{Kind: types.EventInput, Input: types.InputEvent{Source: input.EvendKeyboardSourceTag, Key: k}}
	}
	env._KeyAccept = types.Event{Kind: types.EventInput, Input: types.InputEvent{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyAccept}}
	env._KeyReject = types.Event{Kind: types.EventInput, Input: types.InputEvent{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyReject}}
	env._KeyNext = types.Event{Kind: types.EventInput, Input: types.InputEvent{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyCreamMore}}
	env._KeyPrev = types.Event{Kind: types.EventInput, Input: types.InputEvent{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyCreamLess}}
	env._KeyService = types.Event{Kind: types.EventInput, Input: types.InputEvent{Source: input.DevInputEventTag}}
	env._MoneyAbort = types.Event{Kind: types.EventInput, Input: types.InputEvent{Source: input.MoneySourceTag, Key: input.MoneyKeyAbort}}
	env._Timeout = types.Event{Kind: types.EventTime}
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
			case types.EventInvalid:

			case types.EventInput:
				env.g.Hardware.Input.Emit(step.inev.Input)

			case types.EventStop:
				env.g.Log.Debugf("emit types.EventStop")
				env.g.Alive.Stop()
				env.g.Alive.Wait()
				return

			case types.EventTime: // TODO

			default:
				t.Fatalf("test code error not supported event=%s", step.inev.String())
			}

		case <-waitch:
			if !(step.expect == "" && step.inev.Kind == types.EventInvalid) {
				t.Error("ui stopped before end of test")
			}
			return
		}
	}
	if env.g.Alive.IsRunning() {
		t.Logf("display:\n%s", env.display.State().Format(testDisplayWidth))
		t.Error("ui still running")
	}
	env.g.Alive.Wait()
}

func (env *tenv) requireDisplay(t testing.TB, line1, line2 string) {
	expect := env._T(line1, line2)
	current := <-env.displayUpdated
	t.Logf("display:\n%s\n%s", current, strings.Repeat("-", testDisplayWidth))
	require.Equal(t, expect, current.Format(testDisplayWidth))
}

func (env *tenv) requireState(t testing.TB, expect ui.State) {
	require.NotNil(t, env.uiState)
	current := <-env.uiState
	require.Equal(t, expect.String(), current.String())
}

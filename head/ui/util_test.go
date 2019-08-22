package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/state"
)

const testDisplayWidth = 16

type tenv struct {
	ctx context.Context
	g   *state.Global
	ui  *UI

	display        *lcd.TextDisplay
	displayUpdated chan lcd.State
	_T             func(l1, l2 string) string
	_Key           func(input.Key) Event
	_KeyAccept     Event
	_KeyReject     Event
	_KeyNext       Event
	_KeyPrev       Event
	_KeyService    Event
}

type step struct {
	expect string
	inev   Event
}

func uiTestSetup(t testing.TB, env *tenv, initState, endState State) {
	env.display = lcd.NewMockTextDisplay(&lcd.TextDisplayConfig{Width: testDisplayWidth})
	env.g.Hardware.HD44780.Display.Store(env.display)
	env.ui = &UI{
		testHook: func(s State) {
			t.Logf("testHook %s", s.String())
			switch s {
			case endState: // success path
				env.g.Alive.Stop()
			case StateInvalid:
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
	env._Key = func(k input.Key) Event {
		return Event{Kind: EventInput, Input: input.Event{Source: input.EvendKeyboardSourceTag, Key: k}}
	}
	env._KeyAccept = Event{Kind: EventInput, Input: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyAccept}}
	env._KeyReject = Event{Kind: EventInput, Input: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyReject}}
	env._KeyNext = Event{Kind: EventInput, Input: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyCreamMore}}
	env._KeyPrev = Event{Kind: EventInput, Input: input.Event{Source: input.EvendKeyboardSourceTag, Key: input.EvendKeyCreamLess}}
	env._KeyService = Event{Kind: EventInput, Input: input.Event{Source: input.DevInputEventTag}}
}

func uiTestWait(t testing.TB, env *tenv, steps []step) {
	for _, step := range steps {
		select {
		case current := <-env.displayUpdated:
			t.Logf("display:\n%s\n%s\nevent=%s", current, strings.Repeat("-", testDisplayWidth), step.inev.String())
			require.Equal(t, step.expect, current.Format(testDisplayWidth))
			if step.inev.Kind == EventInput {
				env.g.Hardware.Input.Emit(step.inev.Input)
			}
		case <-env.g.Alive.WaitChan():
			if !(step.expect == "" && step.inev.Kind == EventInvalid) {
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

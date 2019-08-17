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
	displayMock    fmt.Stringer
	displayUpdated chan struct{}
	_T             func(l1, l2 string) string
}

type step struct {
	expect string
	inev   input.Event
}

func uiTestSetup(t testing.TB, env *tenv, initState, endState State) {
	env.display, env.displayMock = lcd.NewMockTextDisplay(&lcd.TextDisplayConfig{Width: testDisplayWidth})
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
	env.displayUpdated = make(chan struct{})
	env.display.SetUpdateChan(env.displayUpdated)
	env._T = func(l1, l2 string) string {
		return fmt.Sprintf("%s\n%s", env.display.Translate(l1), env.display.Translate(l2))
	}
}

func uiTestWait(t testing.TB, env *tenv, steps []step) {
	for _, step := range steps {
		select {
		case <-env.displayUpdated:
		case <-env.g.Alive.WaitChan():
			if !(step.expect == "" && step.inev.IsZero()) {
				t.Error("ui stopped before end of test")
			}
			return
		}
		current := env.displayMock.String()
		t.Logf("display:\n%s\n%s\ninput=%v", current, strings.Repeat("-", testDisplayWidth), step.inev)
		require.Equal(t, step.expect, current)
		env.g.Hardware.Input.Emit(step.inev)
	}
	if env.g.Alive.IsRunning() {
		t.Logf("display:\n%s", env.displayMock.String())
		t.Error("ui still running")
	}
}

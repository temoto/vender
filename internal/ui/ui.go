package ui

import (
	"context"
	"github.com/juju/errors"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/text_display"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/internal/state"
	"github.com/temoto/vender/internal/types"
	ui_config "github.com/temoto/vender/internal/ui/config"
	tele_api "github.com/temoto/vender/tele"
	"time"
)

type UI struct { //nolint:maligned
	FrontMaxPrice currency.Amount
	FrontResult   UIMenuResult
	Service       uiService

	config   *ui_config.Config
	g        *state.Global
	state    State
	broken   bool
	menu     Menu
	display  *text_display.TextDisplay // FIXME
	inputBuf []byte
	eventch  chan types.Event
	inputch  chan types.InputEvent
	lock     uiLock

	frontResetTimeout time.Duration

	XXX_testHook func(State)
}

var _ types.UIer = &UI{} // compile-time interface test

func (self *UI) Init(ctx context.Context) error {
	self.g = state.GetGlobal(ctx)
	self.config = &self.g.Config.UI
	self.setState(StateBoot)

	self.menu = make(Menu)
	if err := self.menu.Init(ctx); err != nil {
		err = errors.Annotate(err, "ui.menu.Init")
		return err
	}
	self.g.Log.Debugf("menu len=%d", len(self.menu))

	self.display = self.g.MustTextDisplay()
	self.eventch = make(chan types.Event)
	self.inputBuf = make([]byte, 0, 32)
	self.inputch = self.g.Hardware.Input.SubscribeChan("ui", self.g.Alive.StopChan())
	// TODO self.g.Hardware.Input.Unsubscribe("ui")

	self.frontResetTimeout = helpers.IntSecondDefault(self.g.Config.UI.Front.ResetTimeoutSec, 0)

	// self.lock.ch = make(chan struct{}, 1)
	self.g.LockCh = make(chan struct{}, 1)
	self.g.TimerUIStop = make(chan struct{}, 1)
	self.Service.Init(ctx)
	self.g.XXX_uier.Store(types.UIer(self)) // FIXME import cycle traded for pointer cycle
	return nil
}

func (self *UI) ScheduleSync(ctx context.Context, priority tele_api.Priority, fun types.TaskFunc) error {
	if !self.LockWait(priority) {
		return errors.Trace(types.ErrInterrupted)
	}
	defer self.LockDecrementWait()
	return fun(ctx)
}

func (self *UI) wait(timeout time.Duration) types.Event {
	tmr := time.NewTimer(timeout)
	defer tmr.Stop()
again:
	select {

	case <-self.g.TimerUIStop:
		return types.Event{Kind: types.EventUiTimerStop}

	case e := <-self.eventch:
		if e.Kind != types.EventInvalid {
		}
		return e

	case e := <-self.inputch:
		if e.Source != "" {
		}
		if e.Source == input.DevInputEventTag && e.Up {
			return types.Event{Kind: types.EventService}
		}
		return types.Event{Kind: types.EventInput, Input: e}

	case <-self.g.LockCh:
		return types.Event{Kind: types.EventFrontLock}

	case <-self.lock.ch:
		// chan buffer may produce false positive
		if !self.lock.locked() {
			goto again
		}
		return types.Event{Kind: types.EventLock}

	case <-tmr.C:
		return types.Event{Kind: types.EventTime}

	case <-self.g.Alive.StopChan():
		return types.Event{Kind: types.EventStop}
	}
}

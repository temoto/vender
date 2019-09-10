package ui

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/head/money"
	ui_config "github.com/temoto/vender/head/ui/config"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

func GetGlobal(ctx context.Context) *UI {
	return state.GetGlobal(ctx).XXX_ui.Load().(*UI)
}

type UI struct { //nolint:maligned
	State         State
	FrontMaxPrice currency.Amount
	FrontResult   UIMenuResult
	Service       uiService

	config       *ui_config.Config
	g            *state.Global
	broken       bool
	menu         Menu
	display      *lcd.TextDisplay // FIXME
	lastActivity time.Time
	inputBuf     []byte
	eventch      chan Event
	inputch      chan input.Event
	moneych      chan money.Event

	frontResetTimeout time.Duration


	XXX_testHook func(State)
}

func (self *UI) Init(ctx context.Context) error {
	self.g = state.GetGlobal(ctx)
	self.config = &self.g.Config.UI
	self.State = StateBoot

	self.menu = make(Menu)
	if err := self.menu.Init(ctx); err != nil {
		err = errors.Annotate(err, "ui.menu.Init")
		return err
	}
	self.g.Log.Debugf("menu len=%d", len(self.menu))

	self.display = self.g.MustDisplay()
	self.eventch = make(chan Event)
	self.inputBuf = make([]byte, 0, 32)
	self.inputch = self.g.Hardware.Input.SubscribeChan("ui", self.g.Alive.StopChan())
	// TODO self.g.Hardware.Input.Unsubscribe("ui")
	self.moneych = make(chan money.Event)

	self.frontResetTimeout = helpers.IntSecondDefault(self.g.Config.UI.Front.ResetTimeoutSec, 0)

	self.Service.Init(ctx)
	self.g.XXX_ui.Store(self) // FIXME import cycle traded for pointer cycle
	return nil
}

func (self *UI) wait(timeout time.Duration) Event {
	tmr := time.NewTimer(timeout)
	defer tmr.Stop()
	select {
	case e := <-self.inputch:
		self.lastActivity = time.Now()
		if e.Source == input.DevInputEventTag && e.Up {
			return Event{Kind: EventService}
		}
		return Event{Kind: EventInput, Input: e}

	case m := <-self.moneych:
		self.lastActivity = time.Now()
		return Event{Kind: EventMoney, Money: m}

		// TODO tele command

	case <-tmr.C:
		return Event{Kind: EventTime}

	case <-self.g.Alive.StopChan():
		return Event{Kind: EventStop}
	}
}

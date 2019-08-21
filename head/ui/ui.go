package ui

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

// TODO move text messages to config
const (
	msgError   = "error"
	msgCream   = "Сливки"
	msgSugar   = "Сахар"
	msgCredit  = "Кредит:"
	msgMaking1 = "спасибо"
	msgMaking2 = "готовлю"

	msgMenuCodeEmpty          = "нажимайте цифры"
	msgMenuCodeInvalid        = "проверьте код"
	msgMenuInsufficientCredit = "добавьте денег"
	msgMenuNotAvailable       = "сейчас недоступно"

	msgInputCode = "код:%s\x00"
)

type UI struct { //nolint:maligned
	State         State
	FrontMaxPrice currency.Amount
	FrontResult   UIMenuResult

	g            *state.Global
	broken       bool
	menu         Menu
	display      *lcd.TextDisplay // FIXME
	lastActivity time.Time
	inputBuf     []byte
	eventch      chan Event
	inputch      chan input.Event
	moneych      chan money.Event
	testHook     func(State)

	frontResetTimeout time.Duration

	service uiService
}

func (self *UI) Init(ctx context.Context) error {
	self.g = state.GetGlobal(ctx)
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

	self.service.Init(ctx)
	return nil
}

func (self *UI) wait(timeout time.Duration) Event {
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

	case <-time.After(timeout):
		return Event{Kind: EventTime}

	case <-self.g.Alive.StopChan():
		return Event{Kind: EventStop}
	}
}

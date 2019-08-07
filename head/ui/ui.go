package ui

import (
	"context"
	"time"

	"github.com/juju/errors"
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

	msgInputCode = "код:%s\x00"
)

type UI struct { //nolint:maligned
	State       State
	FrontResult UIMenuResult

	g            *state.Global
	broken       bool
	menu         Menu
	display      *lcd.TextDisplay // FIXME
	lastActivity time.Time
	inputBuf     []byte
	inputch      chan input.Event
	moneych      chan money.Event
	testHook     func(State)

	frontResetTimeout time.Duration

	service uiService
}

func (self *UI) Init(ctx context.Context) error {
	self.g = state.GetGlobal(ctx)

	self.menu = make(Menu)
	if err := self.menu.Init(ctx); err != nil {
		err = errors.Annotate(err, "ui.menu.Init")
		return err
	}
	self.g.Log.Debugf("menu len=%d", len(self.menu))

	self.display = self.g.MustDisplay()
	self.inputBuf = make([]byte, 0, 32)
	self.inputch = self.g.Hardware.Input.SubscribeChan("ui", self.g.Alive.StopChan())
	// TODO self.g.Hardware.Input.Unsubscribe("ui")
	self.moneych = make(chan money.Event)

	self.frontResetTimeout = helpers.IntSecondDefault(self.g.Config.UI.Front.ResetTimeoutSec, 0)

	self.service.Init(ctx)
	return nil
}

func (self *UI) showError(text string) {
	const timeout = 10 * time.Second

	self.display.Message(self.g.Config.UI.Front.MsgError, text, func() {
		select {
		case <-self.inputch:
		case <-time.After(timeout):
		}
	})
}

func (self *UI) ConveyText(line1, line2 string) {
	self.display.Message(line1, line2, func() {
		<-self.inputch
	})
}

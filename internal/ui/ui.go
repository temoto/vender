package ui

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/text_display"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/internal/state"
	"github.com/temoto/vender/internal/types"
	ui_config "github.com/temoto/vender/internal/ui/config"
	tele_api "github.com/temoto/vender/tele"
)

type UI struct { //nolint:maligned
	FrontMaxPrice currency.Amount
	FrontResult   UIMenuResult
	Service       uiService

	config       *ui_config.Config
	g            *state.Global
	state        State
	broken       bool
	menu         Menu
	display      *text_display.TextDisplay // FIXME
	lastActivity time.Time
	inputBuf     []byte
	eventch      chan types.Event
	inputch      chan types.InputEvent
	lock         uiLock

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
	if self.g.Config.UI.Front.MsgMenuCodeEmpty == "" {self.g.Config.UI.Front.MsgMenuCodeEmpty = "Code empty"}
	if self.g.Config.UI.Front.MsgMenuCodeInvalid == "" {self.g.Config.UI.Front.MsgMenuCodeInvalid = "Code invalid"}
	if self.g.Config.UI.Front.MsgMenuInsufficientCredit == "" {self.g.Config.UI.Front.MsgMenuInsufficientCredit = "Insufficient credit"}
	if self.g.Config.UI.Front.MsgMenuNotAvailable == "" {self.g.Config.UI.Front.MsgMenuNotAvailable = "Not available"}
	if self.g.Config.UI.Front.MsgMenuError == "" {self.g.Config.UI.Front.MsgMenuError = "menu error"}
	if self.g.Config.UI.Front.MsgCream == "" {self.g.Config.UI.Front.MsgCream = "Cream"}
	if self.g.Config.UI.Front.MsgSugar == "" {self.g.Config.UI.Front.MsgSugar = "Sugar"}
	if self.g.Config.UI.Front.MsgCredit == "" {self.g.Config.UI.Front.MsgCredit = "Credit"}
	if self.g.Config.UI.Front.MsgMaking1 == "" {self.g.Config.UI.Front.MsgMaking1 = "Making text line1"}
	if self.g.Config.UI.Front.MsgMaking2 == "" {self.g.Config.UI.Front.MsgMaking2 = "Making text line2"}
	if self.g.Config.UI.Front.MsgInputCode == "" {self.g.Config.UI.Front.MsgInputCode = "Code :%s\x00"}
	if self.g.Config.UI.Front.MsgStateBroken == "" {self.g.Config.UI.Front.MsgStateBroken = "mashine broken"}
	if self.g.Config.UI.Front.MsgStateLocked == "" {self.g.Config.UI.Front.MsgStateLocked = "mashine locked"}
	if self.g.Config.UI.Front.MsgStateIntro == "" {self.g.Config.UI.Front.MsgStateIntro = "reklama here"}
	if self.g.Config.UI.Front.MsgWait == "" {self.g.Config.UI.Front.MsgWait = "please, wait"}
	if self.g.Config.UI.Front.MsgWaterTemp == "" {self.g.Config.UI.Front.MsgWaterTemp = "temperature: %d"}

	self.display = self.g.MustTextDisplay()
	self.eventch = make(chan types.Event)
	self.inputBuf = make([]byte, 0, 32)
	self.inputch = self.g.Hardware.Input.SubscribeChan("ui", self.g.Alive.StopChan())
	// TODO self.g.Hardware.Input.Unsubscribe("ui")

	self.frontResetTimeout = helpers.IntSecondDefault(self.g.Config.UI.Front.ResetTimeoutSec, 0)

	self.lock.ch = make(chan struct{}, 1)

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
	case e := <-self.eventch:
		if e.Kind != types.EventInvalid {
			self.lastActivity = time.Now()
		}
		return e

	case e := <-self.inputch:
		if e.Source != "" {
			self.lastActivity = time.Now()
		}
		if e.Source == input.DevInputEventTag && e.Up {
			return types.Event{Kind: types.EventService}
		}
		return types.Event{Kind: types.EventInput, Input: e}

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

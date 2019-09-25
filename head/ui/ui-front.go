package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/hardware/mdb/evend"
	"github.com/temoto/vender/head/money"
	tele_api "github.com/temoto/vender/head/tele/api"
)

type UIMenuResult struct {
	Item  MenuItem
	Cream uint8
	Sugar uint8
}

func (self *UI) onFrontBegin(ctx context.Context) State {
	self.FrontResult = UIMenuResult{
		// TODO read config
		Cream: DefaultCream,
		Sugar: DefaultSugar,
	}

	if err := self.g.Engine.ExecList(ctx, "on_front_begin", self.g.Config.Engine.OnFrontBegin); err != nil {
		self.g.Log.Errorf("on_front_begin err=%v", err)
		return StateBroken
	}

	// XXX FIXME custom business logic creeped into code TODO move to config
	if doCheckTempHot := self.g.Engine.Resolve("mdb.evend.valve_check_temp_hot"); doCheckTempHot != nil {
		err := doCheckTempHot.Validate()
		if errtemp, ok := err.(*evend.ErrWaterTemperature); ok {
			line1 := fmt.Sprintf(self.g.Config.UI.Front.MsgWaterTemp, errtemp.Current)
			self.display.SetLines(line1, self.g.Config.UI.Front.MsgWait)
			if e := self.wait(5 * time.Second); e.Kind == EventService {
				return StateServiceBegin
			}
			return StateFrontEnd
		} else if err != nil {
			self.g.Error(err)
			return StateBroken
		}
	}

	var err error
	self.FrontMaxPrice, err = self.menu.MaxPrice(self.g.Log)
	if err != nil {
		self.g.Error(err)
		return StateBroken
	}
	self.g.Tele.State(tele_api.State_Nominal)
	return StateFrontSelect
}

func (self *UI) onFrontSelect(ctx context.Context) State {
	moneysys := money.GetGlobal(ctx)

	alive := alive.NewAlive()
	defer func() {
		alive.Stop() // stop pending AcceptCredit
		alive.Wait()
	}()
	go moneysys.AcceptCredit(ctx, self.FrontMaxPrice, alive.StopChan(), self.moneych)

	for {
	refresh:
		// step 1: refresh display
		credit := moneysys.Credit(ctx)
		if self.State == StateFrontTune { // XXX onFrontTune
			goto wait
		}
		self.frontSelectShow(ctx, credit)

		// step 2: wait for input/timeout
	wait:
		timeout := self.frontResetTimeout
		if self.State == StateFrontTune {
			timeout = modTuneTimeout
		}
		e := self.wait(timeout)
		switch e.Kind {
		case EventInput:
			if input.IsMoneyAbort(&e.Input) {
				moneysys := money.GetGlobal(ctx)
				self.g.Error(errors.Trace(moneysys.Abort(ctx)))
				return StateFrontEnd
			}

			switch e.Input.Key {
			case input.EvendKeyCreamLess, input.EvendKeyCreamMore, input.EvendKeySugarLess, input.EvendKeySugarMore:
				// could skip state machine transition and just State=StateFrontTune; goto refresh
				return self.onFrontTuneInput(e.Input)
			}

			switch {
			case e.Input.IsDigit():
				self.inputBuf = append(self.inputBuf, byte(e.Input.Key))
				goto refresh

			case input.IsReject(&e.Input):
				// backspace semantic
				if len(self.inputBuf) > 0 {
					self.inputBuf = self.inputBuf[:len(self.inputBuf)-1]
				}
				goto refresh

			case input.IsAccept(&e.Input):
				if len(self.inputBuf) == 0 {
					self.display.SetLines(self.g.Config.UI.Front.MsgError, MsgMenuCodeEmpty)
					goto wait
				}

				code := string(self.inputBuf)
				mitem, ok := self.menu[code]
				if !ok {
					self.display.SetLines(self.g.Config.UI.Front.MsgError, MsgMenuCodeInvalid)
					goto wait
				}
				moneysys := money.GetGlobal(ctx)
				credit := moneysys.Credit(ctx)
				self.g.Log.Debugf("compare price=%v credit=%v", mitem.Price, credit)
				if mitem.Price > credit {
					self.display.SetLines(self.g.Config.UI.Front.MsgError, MsgMenuInsufficientCredit)
					goto wait
				}
				self.g.Log.Debugf("mitem=%s validate", mitem.String())
				if err := mitem.D.Validate(); err != nil {
					self.g.Log.Errorf("ui-front selected=%s Validate err=%v", mitem.String(), err)
					self.display.SetLines(self.g.Config.UI.Front.MsgError, MsgMenuNotAvailable)
					goto wait
				}

				self.FrontResult.Item = mitem
				return StateFrontAccept // success path

			default:
				self.g.Log.Errorf("ui-front unhandled input=%v", e)
				return StateFrontSelect
			}

		case EventMoney:
			self.g.Log.Errorf("ui-front money event=%v", e.Money)
			go moneysys.AcceptCredit(ctx, self.FrontMaxPrice, alive.StopChan(), self.moneych)

		case EventService:
			return StateServiceBegin

		case EventTime:
			if self.State == StateFrontTune { // XXX onFrontTune
				return StateFrontSelect // "return to previous mode"
			}
			return StateFrontTimeout

		case EventLock, EventStop:
			return StateFrontEnd

		default:
			panic(fmt.Sprintf("code error state=%v unhandled event=%v", self.State, e))
		}
	}
}

func (self *UI) frontSelectShow(ctx context.Context, credit currency.Amount) {
	config := self.g.Config.UI.Front
	l1 := config.MsgStateIntro
	l2 := ""
	if (credit != 0) || (len(self.inputBuf) > 0) {
		l1 = MsgCredit + credit.FormatCtx(ctx)
		l2 = fmt.Sprintf(MsgInputCode, string(self.inputBuf))
	}
	self.display.SetLines(l1, l2)
}

func (self *UI) onFrontTune(ctx context.Context) State {
	// XXX FIXME
	return self.onFrontSelect(ctx)
}

func (self *UI) onFrontTuneInput(e input.Event) State {
	switch e.Key {
	case input.EvendKeyCreamLess:
		if self.FrontResult.Cream > 0 {
			self.FrontResult.Cream--
			//lint:ignore SA9003 empty branch
		} else {
			// TODO notify "impossible input" (sound?)
		}
	case input.EvendKeyCreamMore:
		if self.FrontResult.Cream < MaxCream {
			self.FrontResult.Cream++
			//lint:ignore SA9003 empty branch
		} else {
			// TODO notify "impossible input" (sound?)
		}
	case input.EvendKeySugarLess:
		if self.FrontResult.Sugar > 0 {
			self.FrontResult.Sugar--
			//lint:ignore SA9003 empty branch
		} else {
			// TODO notify "impossible input" (sound?)
		}
	case input.EvendKeySugarMore:
		if self.FrontResult.Sugar < MaxSugar {
			self.FrontResult.Sugar++
			//lint:ignore SA9003 empty branch
		} else {
			// TODO notify "impossible input" (sound?)
		}
	default:
		return StateFrontSelect
	}
	var t1, t2 []byte
	next := StateFrontSelect
	switch e.Key {
	case input.EvendKeyCreamLess, input.EvendKeyCreamMore:
		t1 = self.display.Translate(fmt.Sprintf("%s  /%d", MsgCream, self.FrontResult.Cream))
		t2 = formatScale(self.FrontResult.Cream, 0, MaxCream, ScaleAlpha)
		next = StateFrontTune
	case input.EvendKeySugarLess, input.EvendKeySugarMore:
		t1 = self.display.Translate(fmt.Sprintf("%s  /%d", MsgSugar, self.FrontResult.Sugar))
		t2 = formatScale(self.FrontResult.Sugar, 0, MaxSugar, ScaleAlpha)
		next = StateFrontTune
	}
	t2 = append(append(append(make([]byte, 0, lcd.MaxWidth), '-', ' '), t2...), ' ', '+')
	self.display.SetLinesBytes(
		self.display.JustCenter(t1),
		self.display.JustCenter(t2),
	)
	return next
}

func (self *UI) onFrontAccept(ctx context.Context) State {
	moneysys := money.GetGlobal(ctx)
	uiConfig := &self.g.Config.UI
	selected := &self.FrontResult.Item
	teletx := tele_api.Telemetry_Transaction{
		Code:  selected.Code,
		Price: uint32(selected.Price),
		// TODO options
		// TODO payment method
		// TODO bills, coins
	}
	self.g.Log.Debugf("ui-front selected=%s begin", selected.String())
	if err := moneysys.WithdrawPrepare(ctx, selected.Price); err != nil {
		self.g.Log.Errorf("ui-front CRITICAL error while return change")
	}
	itemCtx := money.SetCurrentPrice(ctx, selected.Price)
	if tuneCream := ScaleTuneRate(self.FrontResult.Cream, MaxCream, DefaultCream); tuneCream != 1 {
		const name = "cream"
		var err error
		self.g.Log.Errorf("ui-front tuning stock=%s tune=%v", name, tuneCream)
		if itemCtx, err = self.g.Inventory.WithTuning(itemCtx, name, tuneCream); err != nil {
			self.g.Log.Errorf("ui-front tuning stock=%s err=%v", name, err)
		}
	}
	if tuneSugar := ScaleTuneRate(self.FrontResult.Sugar, MaxSugar, DefaultSugar); tuneSugar != 1 {
		const name = "sugar"
		var err error
		self.g.Log.Errorf("ui-front tuning stock=%s tune=%v", name, tuneSugar)
		if itemCtx, err = self.g.Inventory.WithTuning(itemCtx, name, tuneSugar); err != nil {
			self.g.Log.Errorf("ui-front tuning stock=%s err=%v", name, err)
		}
	}
	self.display.SetLines(MsgMaking1, MsgMaking2)

	err := selected.D.Do(itemCtx)
	_ = self.g.Inventory.Persist.Store()
	self.g.Log.Debugf("ui-front selected=%s end err=%v", selected.String(), err)
	if err == nil { // success path
		self.g.Tele.Transaction(teletx)
		return StateFrontEnd
	}

	self.display.SetLines(uiConfig.Front.MsgError, uiConfig.Front.MsgMenuError)
	err = errors.Annotatef(err, "execute %s", selected.String())
	self.g.Error(err)

	if err := self.g.Engine.ExecList(ctx, "on_menu_error", self.g.Config.Engine.OnMenuError); err != nil {
		self.g.Error(err)
	} else {
		self.g.Log.Infof("on_menu_error success")
	}
	return StateBroken
}

func (self *UI) onFrontTimeout(ctx context.Context) State {
	self.g.Log.Debugf("ui state=%s result=%#v", self.State.String(), self.FrontResult)
	// moneysys := money.GetGlobal(ctx)
	// moneysys.save
	return StateFrontEnd
}

// tightly coupled to len(alphabet)=4
func formatScale(value, min, max uint8, alphabet []byte) []byte {
	var vicon [6]byte
	switch value {
	case min:
		vicon[0], vicon[1], vicon[2], vicon[3], vicon[4], vicon[5] = 0, 0, 0, 0, 0, 0
	case max:
		vicon[0], vicon[1], vicon[2], vicon[3], vicon[4], vicon[5] = 3, 3, 3, 3, 3, 3
	default:
		rng := uint16(max) - uint16(min)
		part := uint8((float32(value-min) / float32(rng)) * 24)
		// log.Printf("scale(%d,%d..%d) part=%d", value, min, max, part)
		for i := 0; i < len(vicon); i++ {
			if part >= 4 {
				vicon[i] = 3
				part -= 4
			} else {
				vicon[i] = part
				break
			}
		}
	}
	for i := 0; i < len(vicon); i++ {
		vicon[i] = alphabet[vicon[i]]
	}
	return vicon[:]
}

func ScaleTuneRate(value, max, center uint8) float32 {
	switch {
	case value == center: // most common path
		return 1
	case value == 0:
		return 0
	}
	if value > max {
		value = max
	}
	if value > 0 && value < center {
		return 1 - (0.25 * float32(center-value))
	}
	if value > center && value <= max {
		return 1 + (0.25 * float32(value-center))
	}
	panic("code error")
}

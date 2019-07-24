package ui

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/temoto/alive"
	"github.com/temoto/errors"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/hardware/mdb/evend"
	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/head/tele"
)

const (
	DefaultCream = 4
	MaxCream     = 6
	DefaultSugar = 4
	MaxSugar     = 8
)

const (
	frontModeStatus = "menu-status"
	frontModeCream  = "cream"
	frontModeSugar  = "sugar"
	frontModeBroken = "broken"
)

const modTuneTimeout = 3 * time.Second

var ScaleAlpha = []byte{
	0x94, // empty
	0x95,
	0x96,
	0x97, // full
	// '0', '1', '2', '3',
}

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
	return StateFrontSelect
}

func (self *UI) onFrontSelect(ctx context.Context) State {
	if self.broken {
		panic("self.broken")
	}

	alive := alive.NewAlive()
	defer func() {
		alive.Stop() // stop pending AcceptCredit
		alive.Wait()
	}()

	config := self.g.Config.UI.Front
	moneysys := money.GetGlobal(ctx)
	maxPrice := self.menu.MaxPrice() // TODO decide if this should be recalculated during ui
	mode := frontModeStatus
	go moneysys.AcceptCredit(ctx, maxPrice, alive.StopChan(), self.moneych)

	for alive.IsRunning() {
		// step 1: refresh display
		if self.broken {
			mode = frontModeBroken
		}
		credit := moneysys.Credit(ctx)
		switch mode {
		case frontModeStatus:
			l1 := config.MsgStateIntro
			l2 := ""
			if (credit != 0) || (len(self.inputBuf) > 0) {
				l1 = msgCredit + credit.Format100I()
				l2 = fmt.Sprintf(msgInputCode, string(self.inputBuf))
			} else {
				if doCheckTempHot := self.g.Engine.Resolve("mdb.evend.valve_check_temp_hot"); doCheckTempHot != nil {
					err := doCheckTempHot.Validate()
					if errtemp, ok := err.(*evend.ErrWaterTemperature); ok {
						l2 = fmt.Sprintf("no hot water %d", errtemp.Current)
					}
				}
			}
			self.display.SetLines(l1, l2)
		case frontModeBroken:
			self.display.SetLines(config.MsgStateBroken, "")
		}

		// step 2: wait for input/timeout
		var e input.Event
		timeout := self.frontResetTimeout
		switch mode {
		case frontModeCream, frontModeSugar:
			timeout = modTuneTimeout
		}
		select {
		case e = <-self.inputch:
			self.lastActivity = time.Now()

		case em := <-self.moneych:
			self.lastActivity = time.Now()

			self.g.Log.Errorf("ui-front money event=%v", em)
			switch em.Name() {
			case money.EventAbort:
				self.g.Error(errors.Trace(moneysys.Abort(ctx)))
			}
			// likely redundant
			credit = moneysys.Credit(ctx)

			go moneysys.AcceptCredit(ctx, maxPrice, alive.StopChan(), self.moneych)

		case <-time.After(timeout):
			switch mode {
			case frontModeCream, frontModeSugar:
				mode = frontModeStatus
				// "return to previous mode"
				return StateFrontSelect
			default:
				return StateFrontTimeout
			}
		}

		// step 3: handle input/timeout
		if e.Source == input.DevInputEventTag && e.Up {
			return StateServiceBegin
		}
		switch mode {
		case frontModeStatus:
			switch e.Key {
			case input.EvendKeyCreamLess, input.EvendKeyCreamMore, input.EvendKeySugarLess, input.EvendKeySugarMore:
				return self.onFrontCreamSugar(mode, e)
			}

			switch {
			case e.IsDigit():
				self.inputBuf = append(self.inputBuf, byte(e.Key))

			case input.IsReject(&e):
				// backspace semantic
				if len(self.inputBuf) > 0 {
					self.inputBuf = self.inputBuf[:len(self.inputBuf)-1]
					break
				}
				return StateFrontEnd

			case input.IsAccept(&e):
				if len(self.inputBuf) == 0 {
					self.showError(msgMenuCodeEmpty)
					break
				}

				x, err := strconv.ParseUint(string(self.inputBuf), 10, 16)
				if err != nil {
					self.inputBuf = self.inputBuf[:0]
					self.showError(msgMenuCodeInvalid)
					break
				}
				code := uint16(x)

				mitem, ok := self.menu[code]
				if !ok {
					self.showError(msgMenuCodeInvalid)
					break
				}
				self.g.Log.Debugf("compare price=%v credit=%v", mitem.Price, credit)
				if mitem.Price > credit {
					self.showError(msgMenuInsufficientCredit)
					break
				}
				self.g.Log.Debugf("mitem=%s validate", mitem.String())
				if err := mitem.D.Validate(); err != nil {
					self.g.Log.Errorf("ui-front selected=%s Validate err=%v", mitem.String(), err)
					self.showError("сейчас недоступно")
					return StateFrontBegin
				}

				self.FrontResult.Item = mitem
				return StateFrontAccept
			}

		case frontModeCream, frontModeSugar:
			switch e.Key {
			case input.EvendKeyCreamLess, input.EvendKeyCreamMore, input.EvendKeySugarLess, input.EvendKeySugarMore:
				return self.onFrontCreamSugar(mode, e)
			}
			if input.IsAccept(&e) || input.IsReject(&e) {
				return StateFrontSelect
			}
		}
	}

	// external stop
	return StateFrontEnd
}

func (self *UI) onFrontCreamSugar(mode string, e input.Event) State {
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
		return StateFrontTune
	}
	var t1, t2 []byte
	switch e.Key {
	case input.EvendKeyCreamLess, input.EvendKeyCreamMore:
		t1 = self.display.Translate(fmt.Sprintf("%s  /%d", msgCream, self.FrontResult.Cream))
		t2 = formatScale(self.FrontResult.Cream, 0, MaxCream, ScaleAlpha)
		mode = frontModeCream
	case input.EvendKeySugarLess, input.EvendKeySugarMore:
		t1 = self.display.Translate(fmt.Sprintf("%s  /%d", msgSugar, self.FrontResult.Sugar))
		t2 = formatScale(self.FrontResult.Sugar, 0, MaxSugar, ScaleAlpha)
		mode = frontModeSugar
	}
	t2 = append(append(append(make([]byte, 0, lcd.MaxWidth), '-', ' '), t2...), ' ', '+')
	self.display.SetLinesBytes(
		self.display.JustCenter(t1),
		self.display.JustCenter(t2),
	)
	return StateFrontTune
}

func (self *UI) onFrontAccept(ctx context.Context) State {
	moneysys := money.GetGlobal(ctx)
	uiConfig := &self.g.Config.UI
	selected := &self.FrontResult.Item
	teletx := tele.Telemetry_Transaction{
		Code:  int32(selected.Code),
		Price: uint32(selected.Price),
		// TODO options
		// TODO payment method
		// TODO bills, coins
	}
	self.g.Log.Debugf("ui-front selected=%s begin", selected.String())
	if err := moneysys.WithdrawPrepare(ctx, selected.Price); err != nil {
		self.g.Log.Debugf("ui-front CRITICAL error while return change")
	}
	itemCtx := money.SetCurrentPrice(ctx, selected.Price)
	self.display.SetLines("спасибо", "готовлю")

	err := selected.D.Do(itemCtx)
	_ = self.g.Inventory.Persist.Store()
	self.g.Log.Debugf("ui-front selected=%s end err=%v", selected.String(), err)
	if err == nil {
		self.g.Tele.Transaction(teletx)
		return StateFrontEnd
	}

	err = errors.Annotatef(err, "execute %s", selected.String())
	self.g.Log.Errorf(errors.ErrorStack(err))

	self.g.Log.Errorf("tele.error")
	self.g.Tele.Error(err)

	self.display.SetLines(uiConfig.Front.MsgError, "не получилось")
	self.g.Log.Errorf("on_menu_error")
	if err := self.g.Engine.ExecList(ctx, "on_menu_error", self.g.Config.Engine.OnMenuError); err != nil {
		self.g.Log.Errorf("on_menu_error err=%v", err)
		return StateBroken
	} else {
		self.g.Log.Infof("on_menu_error success")
	}
	return StateFrontEnd
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

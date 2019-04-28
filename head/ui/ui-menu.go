package ui

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	keyboard "github.com/temoto/vender/hardware/evend-keyboard"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/state"
)

const (
	DefaultCream = 4
	MaxCream     = 6
	DefaultSugar = 4
	MaxSugar     = 8
)

// TODO extract text messages to catalog
const (
	msgIntro  = "ох уж эта длинная рекламная строка"
	msgError  = "Ошибка"
	msgCream  = "Сливки"
	msgSugar  = "Сахар"
	msgCredit = "Кредит:"

	msgMenuCodeEmpty          = "нажимайте цифры"
	msgMenuCodeInvalid        = "проверьте код"
	msgMenuInsufficientCredit = "добавьте денег"
)

const (
	modeMenuStatus = "menu-status"
	modeMenuCream  = "cream"
	modeMenuSugar  = "sugar"
)

const modCreamSugarTimeout = 3 * time.Second
const resetTimeout = 11 * time.Second

var ScaleAlpha = []byte{
	0x94, // empty
	0x95,
	0x96,
	0x97, // full
	// '0', '1', '2', '3',
}

type UIMenu struct {
	alive     *alive.Alive
	menu      Menu
	credit    atomic.Value
	display   *lcd.TextDisplay
	inputCh   <-chan InputEvent
	refreshCh chan struct{}
	result    UIMenuResult
}

type UIMenuResult struct {
	Item    MenuItem
	Confirm bool
	Cream   uint8
	Sugar   uint8
}

func NewUIMenu(ctx context.Context, menu Menu) *UIMenu {
	config := state.GetConfig(ctx)

	self := &UIMenu{
		alive:     alive.NewAlive(),
		menu:      menu,
		display:   config.Global().Hardware.HD44780.Display,
		refreshCh: make(chan struct{}),
		result: UIMenuResult{
			// TODO read config
			Cream: DefaultCream,
			Sugar: DefaultSugar,
		},
	}
	self.inputCh = InputEvents(ctx, self.alive.StopChan())
	self.SetCredit(0)

	return self
}

func (self *UIMenu) SetCredit(a currency.Amount) {
	self.credit.Store(a)
	select {
	case self.refreshCh <- struct{}{}:
	default:
	}
}

func (self *UIMenu) StopChan() <-chan struct{} { return self.alive.StopChan() }

// func (self *UIMenu) Run(ctx context.Context) UIMenuResult {
func (self *UIMenu) Run() UIMenuResult {
	if !self.alive.IsRunning() {
		panic("code error")
	}

	// wasted in rare case external stop
	defer self.alive.Stop()

	timer := time.NewTicker(200 * time.Millisecond)
	inputBuf := make([]byte, 0, 32)

init:
	self.result = UIMenuResult{
		Cream: DefaultCream,
		Sugar: DefaultSugar,
	}
	inputBuf = inputBuf[:0]
	mode := modeMenuStatus
	lastActivity := time.Now()

	for self.alive.IsRunning() {
		// step 1: refresh display
		credit := self.credit.Load().(currency.Amount)
		switch mode {
		case modeMenuStatus:
			l1 := self.display.Translate(msgIntro)
			// TODO write state flags such as "no hot water" on line2
			l2 := self.display.Translate("")
			if (credit != 0) || (len(inputBuf) > 0) {
				// l1 = self.display.Translate(msgCredit + credit.FormatCtx(ctx))
				l1 = self.display.Translate(msgCredit + credit.Format100I())
				l2 = self.display.Translate(fmt.Sprintf("код:%s\x00", string(inputBuf)))
			}
			self.display.SetLinesBytes(l1, l2)
		}

		// step 2: wait for input/timeout
	waitInput:
		var e InputEvent
		select {
		case e = <-self.inputCh:
			lastActivity = time.Now()
		case <-self.refreshCh:
			lastActivity = time.Now()
			goto handleEnd
		case <-timer.C:
			inactive := time.Since(lastActivity)
			switch {
			case (mode == modeMenuCream || mode == modeMenuSugar) && (inactive >= modCreamSugarTimeout):
				lastActivity = time.Now()
				mode = modeMenuStatus // "return to previous mode"
				goto handleEnd
			case inactive >= resetTimeout:
				log.Printf("reset timeout")
				goto init
			default:
				goto waitInput
			}
		}

		// step 3: handle input/timeout
		switch e.Kind {
		case InputOther:
			mode = self.handleCreamSugar(mode, e.Key)
			goto handleEnd
		case InputNothing:
			panic("code error InputNothing")
		}
		switch mode {
		case modeMenuStatus:
			switch e.Kind {
			case InputNormal:
				inputBuf = append(inputBuf, byte(e.Key))
			case InputReject:
				// backspace semantic
				if len(inputBuf) > 0 {
					inputBuf = inputBuf[:len(inputBuf)-1]
					break
				}

				self.result = UIMenuResult{Confirm: false}
				return self.result
			case InputAccept:
				if len(inputBuf) == 0 {
					self.ConveyError(msgMenuCodeEmpty)
					break
				}

				x, err := strconv.ParseUint(string(inputBuf), 10, 16)
				if err != nil {
					inputBuf = inputBuf[:0]
					self.ConveyError(msgMenuCodeInvalid)
					break
				}
				code := uint16(x)

				mitem, ok := self.menu[code]
				if !ok {
					self.ConveyError(msgMenuCodeInvalid)
					break
				}
				log.Printf("compare price=%v credit=%v", mitem.Price, credit)
				if mitem.Price > credit {
					self.ConveyError(msgMenuInsufficientCredit)
					break
				}

				self.result.Confirm = true
				self.result.Item = mitem

				// debug
				self.ConveyText("debug", fmt.Sprintf("%s +%d +%d",
					self.result.Item.Name, self.result.Cream, self.result.Sugar))

				return self.result
			}
		case modeMenuCream, modeMenuSugar:
			switch e.Kind {
			case InputReject, InputAccept:
				mode = modeMenuStatus // "return to previous mode"
			}
		}
	handleEnd:
	}

	// external stop
	self.result = UIMenuResult{Confirm: false}
	return self.result
}

func (self *UIMenu) ConveyError(text string) {
	const timeout = 10 * time.Second

	self.display.Message(msgError, text, func() {
		select {
		case <-self.refreshCh:
		case <-self.inputCh:
		case <-time.After(timeout):
		}
	})
}

func (self *UIMenu) ConveyText(line1, line2 string) {
	self.display.Message(line1, line2, func() {
		select {
		case <-self.refreshCh:
		case <-self.inputCh:
		}
	})
}

func (self *UIMenu) handleCreamSugar(mode string, key keyboard.Key) string {
	switch key {
	case keyboard.KeyCreamLess:
		if self.result.Cream > 0 {
			self.result.Cream--
			//lint:ignore SA9003 empty branch
		} else {
			// TODO notify "impossible input" (sound?)
		}
	case keyboard.KeyCreamMore:
		if self.result.Cream < MaxCream {
			self.result.Cream++
			//lint:ignore SA9003 empty branch
		} else {
			// TODO notify "impossible input" (sound?)
		}
	case keyboard.KeySugarLess:
		if self.result.Sugar > 0 {
			self.result.Sugar--
			//lint:ignore SA9003 empty branch
		} else {
			// TODO notify "impossible input" (sound?)
		}
	case keyboard.KeySugarMore:
		if self.result.Sugar < MaxSugar {
			self.result.Sugar++
			//lint:ignore SA9003 empty branch
		} else {
			// TODO notify "impossible input" (sound?)
		}
	default:
		return mode
	}
	var t1, t2 []byte
	switch key {
	case keyboard.KeyCreamLess, keyboard.KeyCreamMore:
		t1 = self.display.Translate(fmt.Sprintf("%s  /%d", msgCream, self.result.Cream))
		t2 = formatScale(self.result.Cream, 0, MaxCream, ScaleAlpha)
		mode = modeMenuCream
	case keyboard.KeySugarLess, keyboard.KeySugarMore:
		t1 = self.display.Translate(fmt.Sprintf("%s  /%d", msgSugar, self.result.Sugar))
		t2 = formatScale(self.result.Sugar, 0, MaxSugar, ScaleAlpha)
		mode = modeMenuSugar
	}
	t2 = append(append(append(make([]byte, 0, lcd.MaxWidth), '-', ' '), t2...), ' ', '+')
	self.display.SetLinesBytes(
		self.display.JustCenter(t1),
		self.display.JustCenter(t2),
	)
	return mode
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

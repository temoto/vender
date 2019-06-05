package ui

import (
	"context"
	"encoding/base64"
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/temoto/alive"
	"github.com/temoto/vender/engine/inventory"
	keyboard "github.com/temoto/vender/hardware/evend-keyboard"
	"github.com/temoto/vender/hardware/lcd"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

// TODO move text messages to config
const (
	msgServiceInputAuth = "\x8d %s\x00"
	msgServiceMenu      = "Menu"
)

const (
	serviceModeAuth      = "auth"
	serviceModeMenu      = "menu"
	serviceModeInventory = "inventory"
	serviceModeExit      = "exit"
)

var /*const*/ serviceMenu = []string{serviceModeInventory, serviceModeExit}
var /*const*/ serviceMenuMax = uint8(len(serviceMenu) - 1)

type UIService struct {
	// config
	resetTimeout time.Duration
	secretSalt   []byte

	// state
	g            *state.Global
	display      *lcd.TextDisplay // FIXME
	inputBuf     []byte
	inputCh      <-chan state.InputEvent
	lastActivity time.Time
	mode         string
	menuIdx      uint8

	// mode=inventory
	invIdx  uint8
	invList []*inventory.Stock
}

func NewUIService(ctx context.Context) *UIService {
	self := &UIService{
		g:          state.GetGlobal(ctx),
		inputBuf:   make([]byte, 0, 32),
		secretSalt: []byte{0}, // FIXME read from config
		invList:    make([]*inventory.Stock, 0, 16),
	}
	config := self.g.Config()
	self.display = self.g.Hardware.HD44780.Display
	self.inputCh = self.g.InputChan()
	self.resetTimeout = helpers.IntSecondDefault(config.UI.Service.ResetTimeoutSec, 1*time.Second)
	self.g.Inventory.Iter(func(s *inventory.Stock) {
		self.invList = append(self.invList, s)
	})
	sort.Slice(self.invList, func(a, b int) bool {
		return self.invList[a].Name < self.invList[b].Name
	})
	return self
}

func (self *UIService) Run(alive *alive.Alive) {
	defer alive.Stop()

	state.InputDrain(self.inputCh)
	timer := time.NewTicker(200 * time.Millisecond)
	serviceConfig := &self.g.Config().UI.Service

	self.inputBuf = self.inputBuf[:0]
	self.mode = serviceModeAuth
	self.menuIdx = 0
	self.invIdx = 0
	self.lastActivity = time.Now()
	self.g.Tele.Service("mode=" + self.mode)

loop:
	for alive.IsRunning() {
		// step 1: refresh display
		switch self.mode {
		case serviceModeAuth:
			if !serviceConfig.Auth.Enable {
				self.mode = serviceModeMenu
				continue loop
			}

			passVisualHash := visualHash(self.inputBuf, self.secretSalt)
			self.display.SetLinesBytes(
				self.display.Translate(serviceConfig.MsgAuth),
				self.display.Translate(fmt.Sprintf(msgServiceInputAuth, passVisualHash)),
			)

		case serviceModeMenu:
			menuName := serviceMenu[self.menuIdx]
			self.display.SetLinesBytes(
				self.display.Translate(msgServiceMenu),
				self.display.Translate(fmt.Sprintf("%d %s", self.menuIdx+1, menuName)),
			)

		case serviceModeInventory:
			if len(self.invList) == 0 {
				self.ConveyText(msgError, "inv empty")
				self.mode = serviceModeMenu
				continue loop
			}

			invCurrent := self.invList[self.invIdx]
			self.display.SetLinesBytes(
				self.display.Translate(fmt.Sprintf("I%d %s", self.menuIdx+1, invCurrent.Name)),
				self.display.Translate(fmt.Sprintf("%d %s\x00", invCurrent.Value(), string(self.inputBuf))),
			)

		case serviceModeExit:
			break loop
		}

		// step 2: wait for input/timeout
	waitInput:
		var e state.InputEvent
		select {
		case e = <-self.inputCh:
			self.lastActivity = time.Now()
		case <-timer.C:
			inactive := time.Since(self.lastActivity)
			self.g.Log.Infof("inactive=%v", inactive)
			switch {
			case inactive >= self.resetTimeout:
				break loop
			default:
				goto waitInput
			}
		}

		// step 3: handle input/timeout
		switch self.mode {
		case serviceModeAuth:
			self.handleAuth(e)
		case serviceModeMenu:
			self.handleMenu(e)
		case serviceModeInventory:
			self.handleInventory(e)
		}
	}

	self.g.Tele.Service("mode=" + self.mode)
}

func (self *UIService) ConveyText(line1, line2 string) {
	self.display.Message(line1, line2, func() {
		<-self.inputCh
	})
}

func (self *UIService) handleAuth(e state.InputEvent) {
	switch e.Kind {
	case state.InputNormal:
		self.inputBuf = append(self.inputBuf, byte(e.Key))
		if len(self.inputBuf) > 16 {
			self.ConveyText(msgError, "len") // FIXME extract message string
			self.mode = serviceModeExit
		}

	case state.InputNothing, state.InputReject:
		self.mode = serviceModeExit

	case state.InputAccept:
		if len(self.inputBuf) == 0 {
			self.ConveyText(msgError, "empty") // FIXME extract message string
			self.mode = serviceModeExit
			return
		}

		// FIXME fnv->secure hash for actual password comparison
		inputHash := visualHash(self.inputBuf, self.secretSalt)
		for i, p := range self.g.Config().UI.Service.Auth.Passwords {
			if inputHash == p {
				self.g.Log.Infof("service auth ok i=%d hash=%s", i, inputHash)
				self.mode = serviceModeMenu
				return
			}
		}

		self.ConveyText(msgError, "sorry") // FIXME extract message string
		self.mode = serviceModeExit
	}
}

func (self *UIService) handleMenu(e state.InputEvent) {
	switch e.Kind {
	case state.InputNormal:
		x := byte(e.Key) - byte('0')
		if x > 0 && x <= serviceMenuMax {
			self.menuIdx = x - 1
		}

	case state.InputOther:
		switch e.Key {
		case keyboard.KeyCreamLess, keyboard.KeySugarLess:
			self.menuIdx = (self.menuIdx + serviceMenuMax - 1) % serviceMenuMax
		case keyboard.KeyCreamMore, keyboard.KeySugarMore:
			self.menuIdx = (self.menuIdx + 1) % serviceMenuMax
		}

	case state.InputAccept:
		if int(self.menuIdx) >= len(serviceMenu) {
			panic("code error service menuIdx out of range")
		}
		self.mode = serviceMenu[self.menuIdx]

	case state.InputReject:
		self.mode = serviceModeExit
	}
}

func (self *UIService) handleInventory(e state.InputEvent) {
	invIdxMax := uint8(len(self.invList) - 1)
	switch e.Kind {
	case state.InputNormal:
		self.inputBuf = append(self.inputBuf, byte(e.Key))

	case state.InputOther:
		switch e.Key {
		case keyboard.KeyCreamLess:
			self.invIdx = (self.invIdx + invIdxMax - 1) % invIdxMax
		case keyboard.KeyCreamMore:
			self.invIdx = (self.invIdx + 1) % invIdxMax
		}
		self.inputBuf = self.inputBuf[:0]

	case state.InputAccept:
		if len(self.inputBuf) == 0 {
			self.ConveyText(msgError, "empty") // FIXME extract message string
			return
		}

		x, err := strconv.ParseUint(string(self.inputBuf), 10, 16)
		self.inputBuf = self.inputBuf[:0]
		if err != nil {
			self.ConveyText(msgError, "int-invalid")
			return
		}

		invCurrent := self.invList[self.invIdx]
		invCurrent.Set(int32(x))

	case state.InputReject:
		// backspace semantic
		if len(self.inputBuf) > 0 {
			self.inputBuf = self.inputBuf[:len(self.inputBuf)-1]
			return
		}
		self.mode = serviceModeMenu
	}
}

func visualHash(input, salt []byte) string {
	h := fnv.New32()
	_, _ = h.Write(salt)
	_, _ = h.Write(input)
	_, _ = h.Write(salt)
	var buf [4]byte
	binary := h.Sum(buf[:0])
	b64 := base64.RawStdEncoding.EncodeToString(binary)
	return strings.ToLower(b64)
}

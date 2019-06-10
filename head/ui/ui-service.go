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
	"github.com/temoto/vender/hardware/input"
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
	inputCh      <-chan input.Event
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
	self.resetTimeout = helpers.IntSecondDefault(config.UI.Service.ResetTimeoutSec, 3*time.Second)
	self.g.Inventory.Iter(func(s *inventory.Stock) {
		self.invList = append(self.invList, s)
	})
	sort.Slice(self.invList, func(a, b int) bool {
		return self.invList[a].Name < self.invList[b].Name
	})
	return self
}

func (self *UIService) String() string { return "ui-service" }

func (self *UIService) Run(alive *alive.Alive) {
	defer alive.Stop()

	self.inputCh = self.g.Hardware.Input.SubscribeChan(self.String(), alive.StopChan())
	timer := time.NewTicker(200 * time.Millisecond)
	serviceConfig := &self.g.Config().UI.Service

	self.inputBuf = self.inputBuf[:0]
	self.mode = serviceModeAuth
	self.menuIdx = 0
	self.invIdx = 0
	self.lastActivity = time.Now()
	self.g.Tele.Service("mode=" + self.mode)

	self.g.Log.Debugf("UIService begin")

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
			self.g.Log.Debugf("UIService stop mode=exit")
			break loop
		}

		// step 2: wait for input/timeout
	waitInput:
		var e input.Event
		select {
		case e = <-self.inputCh:
			self.lastActivity = time.Now()
		case <-timer.C:
			inactive := time.Since(self.lastActivity)
			// self.g.Log.Infof("inactive=%v", inactive)
			switch {
			case inactive >= self.resetTimeout:
				self.g.Log.Debugf("UIService stop resettimeout")
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

func (self *UIService) handleAuth(e input.Event) {
	switch {
	case e.IsDigit():
		self.inputBuf = append(self.inputBuf, byte(e.Key))
		if len(self.inputBuf) > 16 {
			self.ConveyText(msgError, "len") // FIXME extract message string
			self.mode = serviceModeExit
		}

	case e.IsZero() || input.IsReject(&e):
		self.mode = serviceModeExit

	case input.IsAccept(&e):
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

func (self *UIService) handleMenu(e input.Event) {
	switch {
	case e.Key == input.EvendKeyCreamLess:
		self.menuIdx = (self.menuIdx + serviceMenuMax - 1) % serviceMenuMax
	case e.Key == input.EvendKeyCreamMore:
		self.menuIdx = (self.menuIdx + 1) % serviceMenuMax

	case input.IsAccept(&e):
		if int(self.menuIdx) >= len(serviceMenu) {
			panic("code error service menuIdx out of range")
		}
		self.mode = serviceMenu[self.menuIdx]

	case input.IsReject(&e):
		self.mode = serviceModeExit

	case e.IsDigit():
		x := byte(e.Key) - byte('0')
		if x > 0 && x <= serviceMenuMax {
			self.menuIdx = x - 1
		}
	}
}

func (self *UIService) handleInventory(e input.Event) {
	invIdxMax := uint8(len(self.invList) - 1)
	switch {
	case e.Key == input.EvendKeyCreamLess:
		self.invIdx = (self.invIdx + invIdxMax - 1) % invIdxMax
		self.inputBuf = self.inputBuf[:0]
	case e.Key == input.EvendKeyCreamMore:
		self.invIdx = (self.invIdx + 1) % invIdxMax
		self.inputBuf = self.inputBuf[:0]

	case e.IsDigit():
		self.inputBuf = append(self.inputBuf, byte(e.Key))

	case input.IsAccept(&e):
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

	case input.IsReject(&e):
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

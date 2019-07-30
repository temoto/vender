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

	"github.com/temoto/vender/engine/inventory"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

// TODO move text messages to config
const (
	msgServiceInputAuth = "\x8d %s\x00"
	msgServiceMenu      = "Menu"
)

const (
	serviceMenuInventory = "inventory"
	serviceMenuReboot    = "reboot"
)

var /*const*/ serviceMenu = []string{serviceMenuInventory, serviceMenuReboot}
var /*const*/ serviceMenuMax = uint8(len(serviceMenu) - 1)

type uiService struct {
	// config
	resetTimeout time.Duration
	secretSalt   []byte

	// state
	menuIdx uint8
	invIdx  uint8
	invList []*inventory.Stock
}

func (self *uiService) Init(ctx context.Context) {
	g := state.GetGlobal(ctx)
	self.secretSalt = []byte{0} // FIXME read from config
	self.resetTimeout = helpers.IntSecondDefault(g.Config.UI.Service.ResetTimeoutSec, 3*time.Second)
}

func (self *UI) onServiceBegin(ctx context.Context) State {
	self.inputBuf = self.inputBuf[:0]
	self.lastActivity = time.Now()
	self.service.menuIdx = 0
	self.service.invIdx = 0
	self.service.invList = make([]*inventory.Stock, 0, 16)
	self.g.Inventory.Iter(func(s *inventory.Stock) {
		self.g.Log.Debugf("ui service inventory: - %s", s.String())
		self.service.invList = append(self.service.invList, s)
	})
	sort.Slice(self.service.invList, func(a, b int) bool {
		return self.service.invList[a].Name < self.service.invList[b].Name
	})
	// self.g.Log.Debugf("invlist=%v, invidx=%d", self.service.invList, self.service.invIdx)

	self.g.Tele.Service("begin")

	self.g.Engine.ExecList(ctx, "on_service_begin", self.g.Config.Engine.OnServiceBegin)

	self.g.Log.Debugf("ui service begin")
	return StateServiceAuth
}

func (self *UI) onServiceAuth() State {
	serviceConfig := &self.g.Config.UI.Service
	if !serviceConfig.Auth.Enable {
		return StateServiceMenu
	}

	passVisualHash := visualHash(self.inputBuf, self.service.secretSalt)
	self.display.SetLinesBytes(
		self.display.Translate(serviceConfig.MsgAuth),
		self.display.Translate(fmt.Sprintf(msgServiceInputAuth, passVisualHash)),
	)

	next, e := self.serviceWaitInput()
	if next != StateInvalid {
		return next
	}

	switch {
	case e.IsDigit():
		self.inputBuf = append(self.inputBuf, byte(e.Key))
		if len(self.inputBuf) > 16 {
			self.ConveyText(msgError, "len") // FIXME extract message string
			return StateServiceEnd
		}
		return self.State

	case e.IsZero() || input.IsReject(&e):
		return StateServiceEnd

	case input.IsAccept(&e):
		if len(self.inputBuf) == 0 {
			self.ConveyText(msgError, "empty") // FIXME extract message string
			return StateServiceEnd
		}

		// FIXME fnv->secure hash for actual password comparison
		inputHash := visualHash(self.inputBuf, self.service.secretSalt)
		for i, p := range self.g.Config.UI.Service.Auth.Passwords {
			if inputHash == p {
				self.g.Log.Infof("service auth ok i=%d hash=%s", i, inputHash)
				return StateServiceMenu
			}
		}

		self.ConveyText(msgError, "sorry") // FIXME extract message string
		return StateServiceEnd
	}
	self.g.Log.Errorf("ui onServiceAuth unhandled branch")
	self.ConveyText(msgError, "code error")
	return StateServiceEnd
}

func (self *UI) onServiceMenu() State {
	menuName := serviceMenu[self.service.menuIdx]
	self.display.SetLinesBytes(
		self.display.Translate(msgServiceMenu),
		self.display.Translate(fmt.Sprintf("%d %s", self.service.menuIdx+1, menuName)),
	)

	next, e := self.serviceWaitInput()
	if next != StateInvalid {
		return next
	}

	switch {
	case e.Key == input.EvendKeyCreamLess:
		self.service.menuIdx = (self.service.menuIdx + serviceMenuMax - 1) % (serviceMenuMax + 1)
	case e.Key == input.EvendKeyCreamMore:
		self.service.menuIdx = (self.service.menuIdx + 1) % (serviceMenuMax + 1)

	case input.IsAccept(&e):
		if int(self.service.menuIdx) >= len(serviceMenu) {
			panic("code error service menuIdx out of range")
		}
		switch serviceMenu[self.service.menuIdx] {
		case serviceMenuInventory:
			return StateServiceInventory
		case serviceMenuReboot:
			return StateServiceReboot
		default:
			panic("code error")
		}

	case input.IsReject(&e):
		return StateServiceEnd

	case e.IsDigit():
		x := byte(e.Key) - byte('0')
		if x > 0 && x <= serviceMenuMax {
			self.service.menuIdx = x - 1
		}
	}
	return StateServiceMenu
}

func (self *UI) onServiceInventory() State {
	if len(self.service.invList) == 0 {
		self.ConveyText(msgError, "inv empty")
		return StateServiceMenu
	}
	invCurrent := self.service.invList[self.service.invIdx]
	self.display.SetLinesBytes(
		self.display.Translate(fmt.Sprintf("I%d %s", self.service.invIdx+1, invCurrent.Name)),
		self.display.Translate(fmt.Sprintf("%d %s\x00", int32(invCurrent.Value()), string(self.inputBuf))),
	)

	next, e := self.serviceWaitInput()
	if next != StateInvalid {
		return next
	}

	invIdxMax := uint8(len(self.service.invList))
	switch {
	case e.Key == input.EvendKeyCreamLess:
		self.service.invIdx = (self.service.invIdx + invIdxMax - 1) % invIdxMax
		self.inputBuf = self.inputBuf[:0]
	case e.Key == input.EvendKeyCreamMore:
		self.service.invIdx = (self.service.invIdx + 1) % invIdxMax
		self.inputBuf = self.inputBuf[:0]

	case e.IsDigit():
		self.inputBuf = append(self.inputBuf, byte(e.Key))

	case input.IsAccept(&e):
		if len(self.inputBuf) == 0 {
			self.g.Log.Errorf("ui onServiceInventory input=accept inputBuf=empty")
			self.ConveyText(msgError, "empty") // FIXME extract message string
			return StateServiceInventory
		}

		x, err := strconv.ParseUint(string(self.inputBuf), 10, 32)
		self.inputBuf = self.inputBuf[:0]
		if err != nil {
			self.g.Log.Errorf("ui onServiceInventory input=accept inputBuf='%s'", string(self.inputBuf))
			self.ConveyText(msgError, "int-invalid")
			return StateServiceInventory
		}

		invCurrent := self.service.invList[self.service.invIdx]
		invCurrent.Set(float32(x))

	case input.IsReject(&e):
		// backspace semantic
		if len(self.inputBuf) > 0 {
			self.inputBuf = self.inputBuf[:len(self.inputBuf)-1]
			return StateServiceInventory
		}
		return StateServiceMenu
	}
	return StateServiceInventory
}

func (self *UI) onServiceReboot() State {
	self.display.SetLines("for reboot", "press 1")

	next, e := self.serviceWaitInput()
	if next != StateInvalid {
		return next
	}

	switch {
	case e.Key == '1':
		self.display.SetLines("reboot", "in progress")
		// os.Exit(0)
		self.g.Alive.Stop()
		return StateServiceEnd
	}
	return StateServiceMenu
}

func (self *UI) serviceWaitInput() (State, input.Event) {
	var e input.Event
	select {
	case e = <-self.inputch:
		self.lastActivity = time.Now()
		return StateInvalid, e
	case <-time.After(self.service.resetTimeout):
		// self.g.Log.Infof("inactive=%v", inactive)
		self.g.Log.Debugf("serviceWaitInput resetTimeout")
		return StateServiceEnd, e
	case <-self.g.Alive.StopChan():
		self.g.Log.Debugf("serviceWaitInput global stop")
		return StateServiceEnd, e
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

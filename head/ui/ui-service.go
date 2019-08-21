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

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/engine/inventory"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/head/tele"
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
	serviceMenuTest      = "test"
	serviceMenuReboot    = "reboot"
)

var /*const*/ serviceMenu = []string{serviceMenuInventory, serviceMenuTest, serviceMenuReboot}
var /*const*/ serviceMenuMax = uint8(len(serviceMenu) - 1)

type uiService struct {
	// config
	resetTimeout time.Duration
	secretSalt   []byte

	// state
	menuIdx  uint8
	invIdx   uint8
	invList  []*inventory.Stock
	testIdx  uint8
	testList []engine.Doer
}

func (self *uiService) Init(ctx context.Context) {
	g := state.GetGlobal(ctx)
	config := g.Config.UI.Service
	self.secretSalt = []byte{0} // FIXME read from config
	self.resetTimeout = helpers.IntSecondDefault(config.ResetTimeoutSec, 3*time.Second)
	errs := make([]error, 0, len(config.Tests))
	for _, t := range config.Tests {
		if d, err := g.Engine.ParseText(t.Name, t.Scenario); err != nil {
			errs = append(errs, err)
		} else {
			self.testList = append(self.testList, d)
		}
	}
	if err := helpers.FoldErrors(errs); err != nil {
		g.Log.Fatal(err)
	}
}

func (self *UI) onServiceBegin(ctx context.Context) State {
	self.inputBuf = self.inputBuf[:0]
	self.lastActivity = time.Now()
	self.service.menuIdx = 0
	self.service.invIdx = 0
	self.service.invList = make([]*inventory.Stock, 0, 16)
	self.service.testIdx = 0
	self.g.Inventory.Iter(func(s *inventory.Stock) {
		self.g.Log.Debugf("ui service inventory: - %s", s.String())
		self.service.invList = append(self.service.invList, s)
	})
	sort.Slice(self.service.invList, func(a, b int) bool {
		return self.service.invList[a].Name < self.service.invList[b].Name
	})
	// self.g.Log.Debugf("invlist=%v, invidx=%d", self.service.invList, self.service.invIdx)

	err := self.g.Engine.ExecList(ctx, "on_service_begin", self.g.Config.Engine.OnServiceBegin)
	if err != nil {
		self.g.Error(err)
		return StateBroken
	}

	self.g.Log.Debugf("ui service begin")
	self.g.Tele.State(tele.State_Service)
	return StateServiceAuth
}

func (self *UI) onServiceAuth() State {
	serviceConfig := &self.g.Config.UI.Service
	if !serviceConfig.Auth.Enable {
		return StateServiceMenu
	}

	passVisualHash := visualHash(self.inputBuf, self.service.secretSalt)
	self.display.SetLines(
		serviceConfig.MsgAuth,
		fmt.Sprintf(msgServiceInputAuth, passVisualHash),
	)

	next, e := self.serviceWaitInput()
	if next != StateInvalid {
		return next
	}

	switch {
	case e.IsDigit():
		self.inputBuf = append(self.inputBuf, byte(e.Key))
		if len(self.inputBuf) > 16 {
			self.display.SetLines(msgError, "len") // FIXME extract message string
			self.serviceWaitInput()
			return StateServiceEnd
		}
		return self.State

	case e.IsZero() || input.IsReject(&e):
		return StateServiceEnd

	case input.IsAccept(&e):
		if len(self.inputBuf) == 0 {
			self.display.SetLines(msgError, "empty") // FIXME extract message string
			self.serviceWaitInput()
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

		self.display.SetLines(msgError, "sorry") // FIXME extract message string
		self.serviceWaitInput()
		return StateServiceEnd
	}
	self.g.Log.Errorf("ui onServiceAuth unhandled branch")
	self.display.SetLines(msgError, "code error") // FIXME extract message string
	self.serviceWaitInput()
	return StateServiceEnd
}

func (self *UI) onServiceMenu() State {
	menuName := serviceMenu[self.service.menuIdx]
	self.display.SetLines(
		msgServiceMenu,
		fmt.Sprintf("%d %s", self.service.menuIdx+1, menuName),
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
		case serviceMenuTest:
			return StateServiceTest
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
		self.display.SetLines(msgError, "inv empty") // FIXME extract message string
		self.serviceWaitInput()
		return StateServiceMenu
	}
	invCurrent := self.service.invList[self.service.invIdx]
	self.display.SetLines(
		fmt.Sprintf("I%d %s", self.service.invIdx+1, invCurrent.Name),
		fmt.Sprintf("%d %s\x00", int32(invCurrent.Value()), string(self.inputBuf)),
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
			self.display.SetLines(msgError, "empty") // FIXME extract message string
			self.serviceWaitInput()
			return StateServiceInventory
		}

		x, err := strconv.ParseUint(string(self.inputBuf), 10, 32)
		self.inputBuf = self.inputBuf[:0]
		if err != nil {
			self.g.Log.Errorf("ui onServiceInventory input=accept inputBuf='%s'", string(self.inputBuf))
			self.display.SetLines(msgError, "int-invalid") // FIXME extract message string
			self.serviceWaitInput()
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

func (self *UI) onServiceTest(ctx context.Context) State {
	self.inputBuf = self.inputBuf[:0]
	if len(self.service.testList) == 0 {
		self.display.SetLines(msgError, "no tests") // FIXME extract message string
		self.serviceWaitInput()
		return StateServiceMenu
	}
	testCurrent := self.service.testList[self.service.testIdx]
	line1 := fmt.Sprintf("T%d %s", self.service.testIdx+1, testCurrent.String())
	self.display.SetLines(line1, "")

wait:
	next, e := self.serviceWaitInput()
	if next != StateInvalid {
		return next
	}

	testIdxMax := uint8(len(self.service.testList))
	switch {
	case e.Key == input.EvendKeyCreamLess:
		self.service.testIdx = (self.service.testIdx + testIdxMax - 1) % testIdxMax
	case e.Key == input.EvendKeyCreamMore:
		self.service.testIdx = (self.service.testIdx + 1) % testIdxMax

	case input.IsAccept(&e):
		self.display.SetLines(line1, "in progress")
		err := testCurrent.Do(ctx)
		if err == nil {
			self.display.SetLines(line1, "OK")
		} else {
			self.g.Error(err)
			self.display.SetLines(line1, "error")
		}
		goto wait

	case input.IsReject(&e):
		return StateServiceMenu
	}
	return StateServiceTest
}

func (self *UI) onServiceReboot() State {
	self.display.SetLines("for reboot", "press 1") // FIXME extract message string

	next, e := self.serviceWaitInput()
	if next != StateInvalid {
		return next
	}

	switch {
	case e.Key == '1':
		self.display.SetLines("reboot", "in progress") // FIXME extract message string
		// os.Exit(0)
		self.g.Alive.Stop()
		return StateServiceEnd
	}
	return StateServiceMenu
}

func (self *UI) serviceWaitInput() (State, input.Event) {
	e := self.wait(self.service.resetTimeout)
	switch e.Kind {
	case EventInput:
		return StateInvalid, e.Input

	case EventMoney:
		self.g.Log.Debugf("serviceWaitInput money event=%v", e.Money)
		return StateInvalid, input.Event{}

	case EventTime:
		// self.g.Log.Infof("inactive=%v", inactive)
		self.g.Log.Debugf("serviceWaitInput resetTimeout")
		return StateServiceEnd, input.Event{}

	case EventStop:
		self.g.Log.Debugf("serviceWaitInput global stop")
		return StateServiceEnd, input.Event{}

	default:
		panic(fmt.Sprintf("code error serviceWaitInput unhandled event=%#v", e))
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

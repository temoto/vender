package ui

import (
	"context"
	"encoding/base64"
	"fmt"
	"hash/fnv"
	// "net"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive/v2"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/internal/engine"
	"github.com/temoto/vender/internal/engine/inventory"
	"github.com/temoto/vender/internal/money"
	"github.com/temoto/vender/internal/state"
	"github.com/temoto/vender/internal/types"
	tele_api "github.com/temoto/vender/tele"
)

const (
	serviceMenuInventory = "inventory"
	serviceMenuTest      = "test"
	serviceMenuReboot    = "reboot"
	serviceMenuNetwork   = "network"
	serviceMenuMoneyLoad = "money-load"
	serviceMenuReport    = "report"
)

var /*const*/ serviceMenu = []string{
	serviceMenuInventory,
	serviceMenuTest,
	serviceMenuReboot,
	serviceMenuNetwork,
	serviceMenuMoneyLoad,
	serviceMenuReport,
}
var /*const*/ serviceMenuMax = uint8(len(serviceMenu) - 1)

type uiService struct { //nolint:maligned
	// config
	resetTimeout time.Duration
	SecretSalt   []byte

	// state
	askReport bool
	menuIdx   uint8
	invIdx    uint8
	invList   []*inventory.Stock
	testIdx   uint8
	testList  []engine.Doer
}

func (self *uiService) Init(ctx context.Context) {
	g := state.GetGlobal(ctx)
	config := g.Config.UI.Service
	self.SecretSalt = []byte{0} // FIXME read from config
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
	self.Service.askReport = false
	self.Service.menuIdx = 0
	self.Service.invIdx = 0
	self.Service.invList = make([]*inventory.Stock, 0, 16)
	self.Service.testIdx = 0
	self.g.Inventory.Iter(func(s *inventory.Stock) {
		self.g.Log.Debugf("ui service inventory: - %s", s.String())
		self.Service.invList = append(self.Service.invList, s)
	})
	sort.Slice(self.Service.invList, func(a, b int) bool {
		xa := self.Service.invList[a]
		xb := self.Service.invList[b]
		if xa.Code != xb.Code {
			return xa.Code < xb.Code
		}
		return xa.Name < xb.Name
	})
	// self.g.Log.Debugf("invlist=%v, invidx=%d", self.Service.invList, self.Service.invIdx)

	if errs := self.g.Engine.ExecList(ctx, "on_service_begin", self.g.Config.Engine.OnServiceBegin); len(errs) != 0 {
		self.g.Error(errors.Annotate(helpers.FoldErrors(errs), "on_service_begin"))
		return StateBroken
	}

	self.g.Log.Debugf("ui service begin")
	self.g.Tele.State(tele_api.State_Service)
	return StateServiceAuth
}

func (self *UI) onServiceAuth() State {
	serviceConfig := &self.g.Config.UI.Service
	if !serviceConfig.Auth.Enable {
		return StateServiceMenu
	}

	passVisualHash := VisualHash(self.inputBuf, self.Service.SecretSalt)
	self.display.SetLines(
		serviceConfig.MsgAuth,
		fmt.Sprintf(msgServiceInputAuth, passVisualHash),
	)

	next, e := self.serviceWaitInput()
	if next != StateDefault {
		return next
	}

	switch {
	case e.IsDigit():
		self.inputBuf = append(self.inputBuf, byte(e.Key))
		if len(self.inputBuf) > 16 {
			self.display.SetLines(MsgError, "len") // FIXME extract message string
			self.serviceWaitInput()
			return StateServiceEnd
		}
		return self.State()

	case e.IsZero() || input.IsReject(&e):
		return StateServiceEnd

	case input.IsAccept(&e):
		if len(self.inputBuf) == 0 {
			self.display.SetLines(MsgError, "empty") // FIXME extract message string
			self.serviceWaitInput()
			return StateServiceEnd
		}

		// FIXME fnv->secure hash for actual password comparison
		inputHash := VisualHash(self.inputBuf, self.Service.SecretSalt)
		for i, p := range self.g.Config.UI.Service.Auth.Passwords {
			if inputHash == p {
				self.g.Log.Infof("service auth ok i=%d hash=%s", i, inputHash)
				return StateServiceMenu
			}
		}

		self.display.SetLines(MsgError, "sorry") // FIXME extract message string
		self.serviceWaitInput()
		return StateServiceEnd
	}
	self.g.Log.Errorf("ui onServiceAuth unhandled branch")
	self.display.SetLines(MsgError, "code error") // FIXME extract message string
	self.serviceWaitInput()
	return StateServiceEnd
}

func (self *UI) onServiceMenu() State {
	menuName := serviceMenu[self.Service.menuIdx]
	self.display.SetLines(
		msgServiceMenu,
		fmt.Sprintf("%d %s", self.Service.menuIdx+1, menuName),
	)

	next, e := self.serviceWaitInput()
	if next != StateDefault {
		return next
	}

	switch {
	case e.Key == input.EvendKeyCreamLess:
		self.Service.menuIdx = addWrap(self.Service.menuIdx, serviceMenuMax+1, -1)
	case e.Key == input.EvendKeyCreamMore:
		self.Service.menuIdx = addWrap(self.Service.menuIdx, serviceMenuMax+1, +1)

	case input.IsAccept(&e):
		if int(self.Service.menuIdx) >= len(serviceMenu) {
			self.g.Fatal(errors.Errorf("code error service menuIdx out of range"))
			return StateBroken
		}
		switch serviceMenu[self.Service.menuIdx] {
		case serviceMenuInventory:
			return StateServiceInventory
		case serviceMenuTest:
			return StateServiceTest
		case serviceMenuReboot:
			return StateServiceReboot
		case serviceMenuNetwork:
			return StateServiceNetwork
		case serviceMenuMoneyLoad:
			return StateServiceMoneyLoad
		case serviceMenuReport:
			return StateServiceReport
		default:
			panic("code error")
		}

	case input.IsReject(&e):
		return StateServiceEnd

	case e.IsDigit():
		x := byte(e.Key) - byte('0')
		if x > 0 && x <= serviceMenuMax {
			self.Service.menuIdx = x - 1
		}
	}
	return StateServiceMenu
}

func (self *UI) onServiceInventory() State {
	if len(self.Service.invList) == 0 {
		self.display.SetLines(MsgError, "inv empty") // FIXME extract message string
		self.serviceWaitInput()
		return StateServiceMenu
	}
	invCurrent := self.Service.invList[self.Service.invIdx]
	self.display.SetLines(
		fmt.Sprintf("I%d %s", invCurrent.Code, invCurrent.Name),
		fmt.Sprintf("%.1f %s\x00", invCurrent.Value(), string(self.inputBuf)), // TODO configurable decimal point
	)

	next, e := self.serviceWaitInput()
	if next != StateDefault {
		return next
	}

	invIdxMax := uint8(len(self.Service.invList))
	switch {
	case e.Key == input.EvendKeyCreamLess || e.Key == input.EvendKeyCreamMore:
		if len(self.inputBuf) != 0 {
			self.display.SetLines(MsgError, "set or clear?") // FIXME extract message string
			self.serviceWaitInput()
			return StateServiceInventory
		}
		if e.Key == input.EvendKeyCreamLess {
			self.Service.invIdx = addWrap(self.Service.invIdx, invIdxMax, -1)
		} else {
			self.Service.invIdx = addWrap(self.Service.invIdx, invIdxMax, +1)
		}
	case e.Key == input.EvendKeyDot || e.IsDigit():
		self.inputBuf = append(self.inputBuf, byte(e.Key))

	case input.IsAccept(&e):
		if len(self.inputBuf) == 0 {
			self.g.Log.Errorf("ui onServiceInventory input=accept inputBuf=empty")
			self.display.SetLines(MsgError, "empty") // FIXME extract message string
			self.serviceWaitInput()
			return StateServiceInventory
		}

		x, err := strconv.ParseFloat(string(self.inputBuf), 32)
		self.inputBuf = self.inputBuf[:0]
		if err != nil {
			self.g.Log.Errorf("ui onServiceInventory input=accept inputBuf='%s'", string(self.inputBuf))
			self.display.SetLines(MsgError, "number-invalid") // FIXME extract message string
			self.serviceWaitInput()
			return StateServiceInventory
		}

		invCurrent := self.Service.invList[self.Service.invIdx]
		invCurrent.Set(float32(x))
		self.Service.askReport = true

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
	if len(self.Service.testList) == 0 {
		self.display.SetLines(MsgError, "no tests") // FIXME extract message string
		self.serviceWaitInput()
		return StateServiceMenu
	}
	testCurrent := self.Service.testList[self.Service.testIdx]
	line1 := fmt.Sprintf("T%d %s", self.Service.testIdx+1, testCurrent.String())
	self.display.SetLines(line1, "")

wait:
	next, e := self.serviceWaitInput()
	if next != StateDefault {
		return next
	}

	testIdxMax := uint8(len(self.Service.testList))
	switch {
	case e.Key == input.EvendKeyCreamLess:
		self.Service.testIdx = addWrap(self.Service.testIdx, testIdxMax, -1)
	case e.Key == input.EvendKeyCreamMore:
		self.Service.testIdx = addWrap(self.Service.testIdx, testIdxMax, +1)

	case input.IsAccept(&e):
		self.display.SetLines(line1, "in progress")
		if err := self.g.Engine.ValidateExec(ctx, testCurrent); err == nil {
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

func (self *UI) onServiceReboot(ctx context.Context) State {
	self.display.SetLines("for reboot", "press 1") // FIXME extract message string

	next, e := self.serviceWaitInput()
	if next != StateDefault {
		return next
	}

	switch {
	case e.Key == '1':
		self.display.SetLines("reboot", "in progress") // FIXME extract message string
		self.g.VmcReload(ctx)
		return StateStop
	}
	return StateServiceMenu
}

func (self *UI) onServiceNetwork() State {

	self.display.SetLines("for restart wifi", "press 1") // FIXME extract message string

	next, e := self.serviceWaitInput()
	if next != StateDefault {
		return next
	}

	switch {
	case e.Key == '1':
		self.display.SetLines("wifi restart", "in progress") // FIXME extract message string

		lsCmd := exec.Command("bash", "-c", "ifdown wlan0 && ifup wlan0")
		bashOut, err := lsCmd.Output()
		self.g.Log.Infof("restart wlan (%v)", bashOut)
		if err != nil {
			panic(err)
		}
		return StateServiceEnd
	}
	return StateServiceMenu
	// 	allAddrs, _ := net.InterfaceAddrs()
	// 	addrs := make([]string, 0, len(allAddrs))
	// 	// TODO parse ignored networks from config
	// addrLoop:
	// 	for _, addr := range allAddrs {
	// 		ip, _, err := net.ParseCIDR(addr.String())
	// 		if err != nil {
	// 			self.g.Log.Errorf("invalid local addr=%v", addr)
	// 			continue addrLoop
	// 		}
	// 		if ip.IsLoopback() {
	// 			continue addrLoop
	// 		}
	// 		addrs = append(addrs, ip.String())
	// 	}
	// 	listString := strings.Join(addrs, " ")
	// 	self.display.SetLines("network", listString)

	// 	for {
	// 		next, e := self.serviceWaitInput()
	// 		if next != StateDefault {
	// 			return next
	// 		}
	// 		if input.IsReject(&e) {
	// 			return StateServiceMenu
	// 		}
	// 	}
}

func (self *UI) onServiceMoneyLoad(ctx context.Context) State {
	moneysys := money.GetGlobal(ctx)

	self.display.SetLines("money-load", "0")
	alive := alive.NewAlive()
	defer func() {
		alive.Stop() // stop pending AcceptCredit
		alive.Wait()
	}()

	self.Service.askReport = true
	accept := true
	loaded := currency.Amount(0)
	for {
		credit := moneysys.Credit(ctx)
		if credit > 0 {
			loaded += credit
			self.display.SetLines("money-load", loaded.FormatCtx(ctx))
			// reset loaded credit
			_ = moneysys.WithdrawCommit(ctx, credit)
		}

		if accept {
			accept = false
			go moneysys.AcceptCredit(ctx, currency.MaxAmount, alive.StopChan(), self.eventch)
		}
		switch e := self.wait(self.Service.resetTimeout); e.Kind {
		case types.EventInput:
			if input.IsReject(&e.Input) {
				return StateServiceMenu
			}

		case types.EventMoneyCredit:
			accept = true

		case types.EventLock:
			return StateLocked

		case types.EventStop:
			self.g.Log.Debugf("onServiceMoneyLoad global stop")
			return StateServiceEnd

		case types.EventTime:

		default:
			panic(fmt.Sprintf("code error onServiceMoneyLoad unhandled event=%s", e.String()))
		}
	}
}

func (self *UI) onServiceReport(ctx context.Context) State {
	_ = self.g.Tele.Report(ctx, true)
	if errs := self.g.Engine.ExecList(ctx, "service-report", []string{"money.cashbox_zero"}); len(errs) != 0 {
		self.g.Error(errors.Annotate(helpers.FoldErrors(errs), "service-report"))
	}
	return StateServiceMenu
}

func (self *UI) onServiceEnd(ctx context.Context) State {
	_ = self.g.Inventory.Persist.Store()
	self.inputBuf = self.inputBuf[:0]

	if self.Service.askReport {
		self.display.SetLines("for tele report", "press 1") // FIXME extract message string
		if e := self.wait(self.Service.resetTimeout); e.Kind == types.EventInput && e.Input.Key == '1' {
			self.Service.askReport = false
			self.onServiceReport(ctx)
		}
	}

	if errs := self.g.Engine.ExecList(ctx, "on_service_end", self.g.Config.Engine.OnServiceEnd); len(errs) != 0 {
		self.g.Error(errors.Annotate(helpers.FoldErrors(errs), "on_service_end"))
		return StateBroken
	}
	return StateDefault
}

func (self *UI) serviceWaitInput() (State, types.InputEvent) {
	e := self.wait(self.Service.resetTimeout)
	switch e.Kind {
	case types.EventInput:
		return StateDefault, e.Input

	case types.EventMoneyCredit:
		self.g.Log.Debugf("serviceWaitInput event=%s", e.String())
		return StateDefault, types.InputEvent{}

	case types.EventTime:
		// self.g.Log.Infof("inactive=%v", inactive)
		self.g.Log.Debugf("serviceWaitInput resetTimeout")
		return StateServiceEnd, types.InputEvent{}

	case types.EventLock:
		return StateLocked, types.InputEvent{}

	case types.EventService:
		self.g.Log.Debugf("service exit")
		return StateServiceEnd, types.InputEvent{}

	case types.EventStop:
		self.g.Log.Debugf("serviceWaitInput global stop")
		return StateServiceEnd, types.InputEvent{}

	default:
		panic(fmt.Sprintf("code error serviceWaitInput unhandled event=%#v", e))
	}
}

func VisualHash(input, salt []byte) string {
	h := fnv.New32()
	_, _ = h.Write(salt)
	_, _ = h.Write(input)
	_, _ = h.Write(salt)
	var buf [4]byte
	binary := h.Sum(buf[:0])
	b64 := base64.RawStdEncoding.EncodeToString(binary)
	return strings.ToLower(b64)
}

func addWrap(current, max uint8, delta int8) uint8 {
	return uint8((int32(current) + int32(max) + int32(delta)) % int32(max))
}

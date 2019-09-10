package ui

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/hardware/mdb/evend"
	"github.com/temoto/vender/head/money"
	tele_api "github.com/temoto/vender/head/tele/api"
)

//go:generate stringer -type=State -trimprefix=State
type State uint32

const (
	StateInvalid State = iota

	StateBoot   // t=onstart +onstartOk=FrontHello +onstartError+retry=Boot +retryMax=Broken
	StateBroken // t=tele/input +inputService=ServiceBegin

	StateFrontBegin   // t=checkVariables +=FrontHello
	StateFrontSelect  // t=input/money/timeout +inputService=ServiceBegin +input=... +money=... +inputAccept=FrontAccept +timeout=FrontTimeout
	StateFrontTune    // t=input/money/timeout +inputTune=FrontTune ->FrontSelect
	StateFrontAccept  // t=Item.Do() +OK=FrontEnd +err=Broken
	StateFrontTimeout // t=saveMoney ->FrontEnd
	StateFrontEnd     // ->FrontBegin

	StateServiceBegin // t=input/timeout ->ServiceAuth
	StateServiceAuth  // +inputAccept+OK=ServiceMenu
	StateServiceMenu
	StateServiceInventory
	StateServiceTest
	StateServiceReboot
	StateServiceEnd // ->FrontBegin
)

func (self *UI) Loop(ctx context.Context) {
	for self.g.Alive.IsRunning() {
		next := self.enter(ctx, self.State)
		if next == StateInvalid {
			self.g.Log.Fatalf("ui state=%s next=invalid", self.State)
		}
		self.exit(ctx, self.State, next)
		self.State = next
		if self.XXX_testHook != nil {
			self.XXX_testHook(next)
		}
	}
}

func (self *UI) enter(ctx context.Context, s State) State {
	self.g.Log.Debugf("ui enter %s", s.String())
	switch s {
	case StateBoot:
		self.g.Tele.State(tele_api.State_Boot)
		onStartSuccess := false
		for i := 1; i <= 3; i++ {
			err := self.g.Engine.ExecList(ctx, "on_boot", self.g.Config.Engine.OnBoot)
			if err == nil {
				onStartSuccess = true
				break
			}
			self.g.Tele.Error(errors.Annotatef(err, "on_boot try=%d", i))
			self.g.Log.Error(err)
			// TODO restart all hardware
			evend.Enum(ctx, nil)
		}
		if !onStartSuccess {
			return StateBroken
		}
		self.broken = false
		return StateFrontBegin

	case StateBroken:
		self.g.Log.Infof("state=broken")
		if !self.broken {
			self.g.Tele.State(tele_api.State_Problem)
			err := self.g.Engine.ExecList(ctx, "on_broken", self.g.Config.Engine.OnBroken)
			if err != nil {
				self.g.Log.Error(errors.ErrorStack(err))
			}
			moneysys := money.GetGlobal(ctx)
			_ = moneysys.SetAcceptMax(ctx, 0)
		}
		self.broken = true
		self.display.SetLines(self.g.Config.UI.Front.MsgStateBroken, "")
		for self.g.Alive.IsRunning() {
			e := self.wait(time.Second)
			// TODO receive tele command to reboot or change state
			if e.Kind == EventService {
				return StateServiceBegin
			}
		}
		return StateInvalid

	case StateFrontBegin:
		self.inputBuf = self.inputBuf[:0]
		self.broken = false
		return self.onFrontBegin(ctx)

	case StateFrontSelect:
		return self.onFrontSelect(ctx)

	case StateFrontTune:
		return self.onFrontTune(ctx)

	case StateFrontAccept:
		return self.onFrontAccept(ctx)

	case StateFrontTimeout:
		return self.onFrontTimeout(ctx)

	case StateFrontEnd:
		// self.onFrontEnd(ctx)
		return StateFrontBegin

	case StateServiceBegin:
		return self.onServiceBegin(ctx)
	case StateServiceAuth:
		return self.onServiceAuth()
	case StateServiceMenu:
		return self.onServiceMenu()
	case StateServiceInventory:
		return self.onServiceInventory()
	case StateServiceTest:
		return self.onServiceTest(ctx)
	case StateServiceReboot:
		return self.onServiceReboot()

	case StateServiceEnd:
		_ = self.g.Inventory.Persist.Store()
		self.inputBuf = self.inputBuf[:0]
		err := self.g.Engine.ExecList(ctx, "on_service_end", self.g.Config.Engine.OnServiceEnd)
		if err != nil {
			self.g.Error(err)
			return StateBroken
		}
		return StateFrontBegin

	default:
		self.g.Log.Fatalf("unhandled ui state=%s", s.String())
		return StateInvalid
	}
}

func (self *UI) exit(ctx context.Context, current, next State) {
	self.g.Log.Debugf("ui exit %s -> %s", current.String(), next.String())

	if next != StateBroken {
		self.broken = false
	}
}

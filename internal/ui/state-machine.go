package ui

import (
	"context"
	"sync/atomic"
	"time"
"os/exec"
	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/internal/money"
	"github.com/temoto/vender/internal/state"
	"github.com/temoto/vender/internal/types"
	tele_api "github.com/temoto/vender/tele"
)

//go:generate stringer -type=State -trimprefix=State
type State uint32

const (
	StateDefault State = iota

	StateBoot   // t=onstart +onstartOk=FrontHello +onstartError+retry=Boot +retryMax=Broken
	StateBroken // t=tele/input +inputService=ServiceBegin
	StateLocked // t=tele

	StateFrontBegin   // t=checkVariables +=FrontHello
	StateFrontSelect  // t=input/money/timeout +inputService=ServiceBegin +input=... +money=... +inputAccept=FrontAccept +timeout=FrontTimeout
	StateFrontTune    // t=input/money/timeout +inputTune=FrontTune ->FrontSelect
	StateFrontAccept  // t=engine.Exec(Item) +OK=FrontEnd +err=Broken
	StateFrontTimeout // t=saveMoney ->FrontEnd
	StateFrontEnd     // ->FrontBegin

	StateServiceBegin // t=input/timeout ->ServiceAuth
	StateServiceAuth  // +inputAccept+OK=ServiceMenu
	StateServiceMenu
	StateServiceInventory
	StateServiceTest
	StateServiceReboot
	StateServiceNetwork
	StateServiceMoneyLoad
	StateServiceReport
	StateServiceEnd // +askReport=ServiceReport ->FrontBegin

	StateStop
)

func (self *UI) State() State               { return State(atomic.LoadUint32((*uint32)(&self.state))) }
func (self *UI) setState(new State)         { atomic.StoreUint32((*uint32)(&self.state), uint32(new)) }
func (self *UI) XXX_testSetState(new State) { self.setState(new) }

func (self *UI) Loop(ctx context.Context) {
	self.g.Alive.Add(1)
	defer self.g.Alive.Done()
	next := StateDefault
	for next != StateStop && self.g.Alive.IsRunning() {
		current := self.State()
		next = self.enter(ctx, current)
		if next == StateDefault {
			self.g.Log.Fatalf("ui state=%s next=default", current.String())
		}
		self.exit(ctx, current, next)

		if current != StateLocked && self.checkInterrupt(next) {
			self.lock.next = next
			self.g.Log.Infof("ui lock interrupt")
			next = StateLocked
		}

		if !self.g.Alive.IsRunning() {
			self.g.Log.Debugf("ui Loop stopping because g.Alive")
			next = StateStop
		}

		self.setState(next)
		if self.XXX_testHook != nil {
			self.XXX_testHook(next)
		}
	}
	self.g.Log.Debugf("ui loop end")
}

func (self *UI) enter(ctx context.Context, s State) State {
	self.g.Log.Debugf("ui enter %s", s.String())
	switch s {
	case StateBoot:
		self.g.Tele.State(tele_api.State_Boot)
		onStartSuccess := false
		for i := 1; i <= 3; i++ {
			errs := self.g.Engine.ExecList(ctx, "on_boot", self.g.Config.Engine.OnBoot)
			if err := errors.Annotate(helpers.FoldErrors(errs), "on_boot"); err != nil {
				self.g.Tele.Error(errors.Annotatef(err, "on_boot try=%d", i))
				self.g.Log.Error(err)
			}
			// on_boot special behavior: log, report but don't stop on errors caused by optional offline devices
			if len(removeOptionalOffline(self.g, errs)) == 0 {
				onStartSuccess = true
				break
			}
			// TODO restart all hardware
			// hardware.Enum(ctx)
		}
		if !onStartSuccess {
			return StateBroken
		}
		self.broken = false
		return StateFrontBegin

	case StateBroken:
		self.g.Log.Infof("Alexxx")
		self.g.Log.Infof("state=broken")
		cmd := exec.Command("fbi", " -a -d /dev/fb0 -T 1 -noverbose /home/vmc/broken.jpg ")
	err := cmd.Start()
	self.g.Log.Infof("Alexxx1%s",err)
	if err != nil {
		self.g.Log.Error(err)
	}
	// log.Printf("Waiting for command to finish...")
	err = cmd.Wait()
self.g.Log.Infof("Alexxx2 %s",err)
		if !self.broken {
			self.g.Tele.State(tele_api.State_Problem)
			if errs := self.g.Engine.ExecList(ctx, "on_broken", self.g.Config.Engine.OnBroken); len(errs) != 0 {
				// TODO maybe ErrorStack should be removed
				self.g.Log.Error(errors.ErrorStack(errors.Annotate(helpers.FoldErrors(errs), "on_broken")))
			}
			moneysys := money.GetGlobal(ctx)
			_ = moneysys.SetAcceptMax(ctx, 0)
		}
		self.broken = true
		self.display.SetLines(self.g.Config.UI.Front.MsgStateBroken, "")
		if d, _ := self.g.Display(); d != nil {
			// _ = d.Clear()
			// _ = d.Picture(self.g.Config.UI.Front.PicBroken)


		}
		for self.g.Alive.IsRunning() {
			e := self.wait(time.Second)
			// TODO receive tele command to reboot or change state
			if e.Kind == types.EventService {
				return StateServiceBegin
			}
		}
		return StateDefault

	case StateLocked:
		self.display.SetLines(self.g.Config.UI.Front.MsgStateLocked, "")
		self.g.Tele.State(tele_api.State_Lock)
		for self.g.Alive.IsRunning() {
			e := self.wait(lockPoll)
			// TODO receive tele command to reboot or change state
			if e.Kind == types.EventService {
				return StateServiceBegin
			}
			if !self.lock.locked() {
				return self.lock.next
			}
		}
		return StateDefault

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
	case StateServiceNetwork:
		return self.onServiceNetwork()
	case StateServiceMoneyLoad:
		return self.onServiceMoneyLoad(ctx)
	case StateServiceReport:
		return self.onServiceReport(ctx)
	case StateServiceEnd:
		return replaceDefault(self.onServiceEnd(ctx), StateFrontBegin)

	case StateStop:
		return StateStop

	default:
		self.g.Log.Fatalf("unhandled ui state=%s", s.String())
		return StateDefault
	}
}

func (self *UI) exit(ctx context.Context, current, next State) {
	self.g.Log.Debugf("ui exit %s -> %s", current.String(), next.String())

	if next != StateBroken {
		self.broken = false
	}
}

func replaceDefault(s, def State) State {
	if s == StateDefault {
		return def
	}
	return s
}

func filterErrors(errs []error, take func(error) bool) []error {
	if len(errs) == 0 {
		return nil
	}
	new := errs[:0]
	for _, e := range errs {
		if e != nil && take(e) {
			new = append(new, e)
		}
	}
	for i := len(new); i < len(errs); i++ {
		errs[i] = nil
	}
	return new
}

func removeOptionalOffline(g *state.Global, errs []error) []error {
	take := func(e error) bool {
		if errOffline, ok := errors.Cause(e).(types.DeviceOfflineError); ok {
			if devconf, err := g.GetDeviceConfig(errOffline.Device.Name()); err == nil {
				return devconf.Required
			}
		}
		return true
	}
	return filterErrors(errs, take)
}
func executeScript(ctx context.Context, script string) {
	g := state.GetGlobal(ctx)
	if script != "" {
		cmd := exec.Command(script) //nolint:gosec
		g.Alive.Add(1)
		go func() {
			defer g.Alive.Done()
			execOutput, execErr := cmd.CombinedOutput()
			if execErr != nil {
				execErr = errors.Annotatef(execErr, "state_script %s output=%s", cmd.Path, execOutput)
				g.Log.Error(execErr)
			}
		}()
	}
}
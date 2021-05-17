package state

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive/v2"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/internal/engine"
	"github.com/temoto/vender/internal/engine/inventory"
	"github.com/temoto/vender/internal/global"
	"github.com/temoto/vender/internal/types"
	"github.com/temoto/vender/log2"
	tele_api "github.com/temoto/vender/tele"
)

type Global struct {
	Alive        *alive.Alive
	BuildVersion string
	Config       *Config
	Engine       *engine.Engine
	Hardware     hardware // hardware.go
	Inventory    *inventory.Inventory
	Log          *log2.Log
	Tele         tele_api.Teler
	LockCh       chan struct{}
	// TODO UI           types.UIer

	XXX_money atomic.Value // *money.MoneySystem crutch to import cycle
	XXX_uier  atomic.Value // UIer crutch to import/init cycle

	_copy_guard sync.Mutex //nolint:unused
}

const ContextKey = "run/state-global"

func (g *Global) ClientBegin() {
	gg := &global.GBL.Client
	if !gg.Working {
		gg.Working = true
		gg.WorkTime = time.Now()
		global.Log.Infof("--- client activity begin ---")
		g.Tele.State(tele_api.State_Client)
	}
}

func (g *Global) ClientEnd() {
	gg := &global.GBL.Client
	g.Hardware.Input.Enable(true)
	if gg.Working {
		gg.Working = false
		gg.WorkTime = time.Now()
		global.Log.Infof("--- client activity end ---")
		// g.Tele.State(tele_api.State_Nominal)
	}
}

func CheckClientWorking() error {
	if global.GBL.Client.Working {
		global.Log.Errorf("execute imposible. processing the client.")
		return errors.Errorf("Processing the client")
	}
	return nil
}

func (g *Global) VmcStop(ctx context.Context) {
	global.Log.Infof("--- vmc stop ---")
	if global.GBL.Client.Working {
		global.Log.Infof("stop fail. processing client")
		return
	}
	td := g.MustTextDisplay()
	td.SetLines("ABTOMAT", "HE ABTOMAT! :(") // FIXME extract message string
	g.Tele.State(tele_api.State_Boot)
	_ = g.Engine.ExecList(ctx, "reboot", []string{"evend.cup.light_off evend.valve.set_temp_hot(0)"})

	go func() {
		time.Sleep(2 * time.Second)
		g.Tele.Close()
		g.Stop()
	}()
}

func GetGlobal(ctx context.Context) *Global {
	v := ctx.Value(ContextKey)
	if v == nil {
		panic(fmt.Sprintf("context['%s'] is nil", ContextKey))
	}
	if g, ok := v.(*Global); ok {
		return g
	}
	panic(fmt.Sprintf("context['%s'] expected type *Global actual=%#v", ContextKey, v))
}

// If `Init` fails, consider `Global` is in broken state.
func (g *Global) Init(ctx context.Context, cfg *Config) error {
	g.Config = cfg

	g.Log.Infof("build version=%s", g.BuildVersion)
	global.GBL.Version = g.BuildVersion

	if g.Config.Persist.Root == "" {
		g.Config.Persist.Root = "./tmp-vender-db"
		g.Log.Errorf("config: persist.root=empty changed=%s", g.Config.Persist.Root)
		// return errors.Errorf("config: persist.root=empty")
	}
	g.Log.Debugf("config: persist.root=%s", g.Config.Persist.Root)

	// Since tele is remote error reporting mechanism, it must be inited before anything else
	g.Config.Tele.BuildVersion = g.BuildVersion
	if g.Config.Tele.PersistPath == "" {
		g.Config.Tele.PersistPath = filepath.Join(g.Config.Persist.Root, "tele")
	}
	// Tele.Init gets g.Log clone before SetErrorFunc, so Tele.Log.Error doesn't recurse on itself
	if err := g.Tele.Init(ctx, g.Log.Clone(log2.LInfo), g.Config.Tele); err != nil {
		g.Tele = tele_api.Noop{}
		return errors.Annotate(err, "tele init")
	}
	g.Log.SetErrorFunc(g.Tele.Error)

	if g.BuildVersion == "unknown" {
		g.Error(fmt.Errorf("build version is not set, please use script/build"))
	} else if g.Config.Tele.VmId > 0 && strings.HasSuffix(g.BuildVersion, "-dirty") { // vmid<=0 is staging
		g.Error(fmt.Errorf("running development build with uncommited changes, bad idea for production"))
	}

	if g.Config.Money.Scale == 0 {
		g.Config.Money.Scale = 1
		g.Log.Errorf("config: money.scale is not set")
	} else if g.Config.Money.Scale < 0 {
		return errors.NotValidf("config: money.scale < 0")
	}
	g.Config.Money.CreditMax *= g.Config.Money.Scale
	g.Config.Money.ChangeOverCompensate *= g.Config.Money.Scale

	const initTasks = 4
	wg := sync.WaitGroup{}
	wg.Add(initTasks)
	errch := make(chan error, initTasks)

	// working term signal
    sigs := make(chan os.Signal, 1)
    // done := make(chan bool, 1)
    signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        _ = <-sigs
        g.VmcStop(ctx)
    }()



	go helpers.WrapErrChan(&wg, errch, g.initDisplay)
	go helpers.WrapErrChan(&wg, errch, g.initInput)
	go helpers.WrapErrChan(&wg, errch, func() error { return g.initInventory(ctx) }) // storage read
	go helpers.WrapErrChan(&wg, errch, g.initEngine)
	// TODO init money system, load money state from storage
	g.RegisterCommands(ctx)
	wg.Wait()
	close(errch)

	// TODO engine.try-resolve-all-lazy after all other inits finished

	return helpers.FoldErrChan(errch)
}

func (g *Global) MustInit(ctx context.Context, cfg *Config) {
	err := g.Init(ctx, cfg)
	if err != nil {
		g.Fatal(err)
	}
}

func (g *Global) Error(err error, args ...interface{}) {
	if err != nil {
		if len(args) != 0 {
			msg := args[0].(string)
			args = args[1:]
			err = errors.Annotatef(err, msg, args...)
		}
		g.Tele.Error(err)
		global.Log.Error(err)
	}
}

func (g *Global) Fatal(err error, args ...interface{}) {
	if err != nil {
		g.Error(err, args...)
		g.StopWait(5 * time.Second)
		g.Log.Fatal(err)
		os.Exit(1)
	}
}

func (g *Global) ScheduleSync(ctx context.Context, priority tele_api.Priority, fun types.TaskFunc) error {
	// TODO task := g.Schedule(ctx, priority, fun)
	// return task.wait()

	g.Alive.Add(1)
	defer g.Alive.Done()

	switch priority {
	case tele_api.Priority_Default, tele_api.Priority_Now:
		return fun(ctx)

	case tele_api.Priority_IdleEngine:
		// TODO return g.Engine.Schedule(ctx, priority, fun)
		return fun(ctx)

	case tele_api.Priority_IdleUser:
		return g.UI().ScheduleSync(ctx, priority, fun)

	default:
		return errors.Errorf("code error ScheduleSync invalid priority=(%d)%s", priority, priority.String())
	}
}

func (g *Global) Stop() {
	g.Alive.Stop()
}

func (g *Global) StopWait(timeout time.Duration) bool {
	g.Alive.Stop()
	select {
	case <-g.Alive.WaitChan():
		return true
	case <-time.After(timeout):
		return false
	}
}

func (g *Global) UI() types.UIer {
	for {
		x := g.XXX_uier.Load()
		if x != nil {
			return x.(types.UIer)
		}
		g.Log.Errorf("CRITICAL g.uier is not set")
		time.Sleep(5 * time.Second)
	}
}

func (g *Global) initDisplay() error {
	d, err := g.Display()
	if d != nil {
		d.Clear()
	}
	return err
}

func (g *Global) initEngine() error {
	errs := make([]error, 0)

	for _, x := range g.Config.Engine.Aliases {
		var err error
		x.Doer, err = g.Engine.ParseText(x.Name, x.Scenario)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// g.Log.Debugf("config.engine.alias name=%s scenario=%s", x.Name, x.Scenario)
		g.Engine.Register(x.Name, x.Doer)
	}

	for _, x := range g.Config.Engine.Menu.Items {
		var err error
		x.Price = g.Config.ScaleI(x.XXX_Price)
		x.Doer, err = g.Engine.ParseText(x.Name, x.Scenario)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// g.Log.Debugf("config.engine.menu %s pxxx=%d ps=%d", x.String(), x.XXX_Price, x.Price)
		g.Engine.Register("menu."+x.Code, x.Doer)
	}

	if pcfg := g.Config.Engine.Profile; pcfg.Regexp != "" {
		if re, err := regexp.Compile(pcfg.Regexp); err != nil {
			errs = append(errs, err)
		} else {
			format := pcfg.LogFormat
			if format == "" {
				format = `engine profile action=%s time=%s`
			}
			min := time.Duration(pcfg.MinUs) * time.Microsecond
			g.Engine.SetProfile(re, min, func(d engine.Doer, td time.Duration) { g.Log.Debugf(format, d.String(), td) })
		}
	}

	return helpers.FoldErrors(errs)
}

func (g *Global) initInventory(ctx context.Context) error {
	// TODO ctx should be enough
	if err := g.Inventory.Init(ctx, &g.Config.Engine.Inventory, g.Engine); err != nil {
		return err
	}
	err := g.Inventory.Persist.Init("inventory", g.Inventory, g.Config.Persist.Root, g.Config.Engine.Inventory.Persist, g.Log)
	if err == nil {
		err = g.Inventory.Persist.Load()
	}
	return errors.Annotate(err, "initInventory")
}

func (g *Global) RegisterCommands(ctx context.Context) {
	g.Engine.RegisterNewFunc(
		"vmc.lock!",
		func(ctx context.Context) error {
			g.LockCh <- struct{}{}
			return nil
		},
	)

	g.Engine.RegisterNewFunc(
		"vmc.stop!",
		func(ctx context.Context) error {
			g.VmcStop(ctx)
			return nil
		},
	)

	g.Engine.RegisterNewFunc(
		"input.enable",
		func(ctx context.Context) error {
			g.Hardware.Input.Enable(true)
			return nil
		},
	)
	g.Engine.RegisterNewFunc(
		"input.disable",
		func(ctx context.Context) error {
			g.Hardware.Input.Enable(false)
			return nil
		},
	)

	g.Engine.RegisterNewFunc(
		"envs.print",
		func(ctx context.Context) error {
			err := errors.Errorf(global.ShowEnvs())
			// AlexM надо бы сделать что бы слала как сообщение а не ошибку.
			return err
		},
	)

	doEmuKey := engine.FuncArg{
		Name: "emulate.key(?)",
		F: func(ctx context.Context, arg engine.Arg) error {
			event := types.InputEvent{Source: "evend-keyboard", Key: types.InputKey(arg), Up: true}
			g.Hardware.Input.Emit(event)
			return nil
		}}
	g.Engine.Register(doEmuKey.Name, doEmuKey)

}

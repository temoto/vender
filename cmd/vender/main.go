package main

import (
	"context"
	"flag"
	"os"
	"runtime"

	"github.com/coreos/go-systemd/daemon"
	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb/evend"
	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/head/ui"
	"github.com/temoto/vender/log2"
	"github.com/temoto/vender/state"
)

var log = log2.NewStderr(log2.LDebug)

func main() {
	cmdline := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flagConfig := cmdline.String("config", "vender.hcl", "")
	cmdline.Parse(os.Args[1:])

	logFlags := log2.LInteractiveFlags
	if sdnotify("start") {
		// under systemd assume systemd journal logging, no timestamp
		logFlags = log2.LServiceFlags
	}
	log.SetFlags(logFlags)
	log.Debugf("hello")

	var money money.MoneySystem

	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log)
	ctx = context.WithValue(ctx, engine.ContextKey, engine.NewEngine(ctx))

	config := state.MustReadConfig(ctx, state.NewOsFullReader(""), *flagConfig)
	config.MustInit(ctx)
	log.Debugf("config=%+v", config)
	ctx = state.ContextWithConfig(ctx, config)

	a := alive.NewAlive()
	config.Global().Alive = a
	config.Global().Hardware.Mdb.Mdber.BusResetDefault()

	// TODO func(dev Devicer) { dev.Init() && dev.Register() }
	// right now Enum does IO implicitly
	// FIXME hardware.Enum() but money system inits bill/coin devices explicitly
	evend.Enum(ctx, nil)

	sdnotify(daemon.SdNotifyReady)

	log.Debugf("vender init complete, running")
	for a.IsRunning() {
		uiStep(ctx, &money, a.StopChan())
	}

	a.Wait()
	// maybe these KeepAlive() are redundant
	runtime.KeepAlive(ctx)
}

func uiStep(ctx context.Context, moneysys *money.MoneySystem, stopCh <-chan struct{}) {
	menu := ui.NewUIMenu(ctx, nil)
	menu.SetCredit(moneysys.Credit(ctx))

	endCh := make(chan struct{})
	defer close(endCh)
	go func() {
		for {
			select {
			case <-endCh:
				return
			case <-stopCh:
				return
			case em := <-moneysys.Events():
				log.Debugf("money event: %s", em.String())
				switch em.Name() {
				case money.EventCredit:
					menu.SetCredit(moneysys.Credit(ctx))
				case money.EventAbort:
					err := moneysys.Abort(ctx)
					log.Infof("user requested abort err=%v", err)
					menu.SetCredit(moneysys.Credit(ctx))
				default:
					panic("head: unknown money event: " + em.String())
				}
			}
		}
	}()

	menuResult := menu.Run()
	log.Debugf("menu result=%#v", menuResult)
	if !menuResult.Confirm {
		return
	}

	menuResult.Item.D.Do(ctx)
}

func sdnotify(s string) bool {
	ok, err := daemon.SdNotify(false, s)
	if err != nil {
		log.Fatal("sdnotify: ", errors.ErrorStack(err))
	}
	return ok
}

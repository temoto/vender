package main

import (
	"context"
	"flag"
	"os"

	"github.com/coreos/go-systemd/daemon"
	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb/evend"
	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/head/tele"
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

	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log)
	ctx = context.WithValue(ctx, engine.ContextKey, engine.NewEngine(ctx))

	config := state.MustReadConfig(ctx, state.NewOsFullReader(), *flagConfig)
	config.MustInit(ctx)
	log.Debugf("config=%+v", config)
	ctx = state.ContextWithConfig(ctx, config)

	moneysys := new(money.MoneySystem)
	moneysys.Start(ctx)

	mdber, err := config.Mdber()
	if err != nil {
		log.Fatalf("mdb init err=%v", errors.ErrorStack(err))
	}

	a := alive.NewAlive()
	config.Global().Alive = a
	mdber.BusResetDefault()

	// TODO func(dev Devicer) { dev.Init() && dev.Register() }
	// right now Enum does IO implicitly
	// FIXME hardware.Enum() but money system inits bill/coin devices explicitly
	evend.Enum(ctx, nil)

	sdnotify(daemon.SdNotifyReady)

	menuMap := make(ui.Menu)
	if err = menuInit(ctx, menuMap); err != nil {
		log.Fatalf("menu: %v", errors.ErrorStack(err))
	}

	log.Debugf("vender init complete, running")
	for a.IsRunning() {
		uiStep(ctx, moneysys, menuMap, a.StopChan())
	}

	a.Wait()
}

func uiStep(ctx context.Context, moneysys *money.MoneySystem, menuMap ui.Menu, stopCh <-chan struct{}) {
	telesys := &state.GetGlobal(ctx).Tele

	menu := ui.NewUIMenu(ctx, menuMap)
	menu.SetCredit(moneysys.Credit(ctx))
	moneysys.AcceptCredit(ctx, menuMap.MaxPrice())

	moneyUpdateCh := make(chan struct{})
	defer close(moneyUpdateCh)
	endCh := make(chan struct{})
	defer close(endCh)
	go func() { // propagate stopch to close(endch) to stop utility goroutines below
		select {
		case <-stopCh:
			return
		}
	}()
	go func() {
		for {
			select {
			case <-endCh:
				return
			case em := <-moneysys.Events():
				log.Debugf("money event: %s", em.String())
				switch em.Name() {
				case money.EventCredit:
					moneyUpdateCh <- struct{}{}
				case money.EventAbort:
					err := moneysys.Abort(ctx)
					log.Infof("user requested abort err=%v", err)
					moneyUpdateCh <- struct{}{}
				default:
					panic("head: unknown money event: " + em.String())
				}
			case cmd := <-telesys.CommandChan():
				switch cmd.Task.(type) {
				case *tele.Command_Abort:
					err := moneysys.Abort(ctx)
					telesys.CommandReplyErr(&cmd, err)
					log.Infof("admin requested abort err=%v", err)
					moneyUpdateCh <- struct{}{}
				case *tele.Command_SetGiftCredit:
					moneysys.SetGiftCredit(ctx, currency.Amount(cmd.GetSetGiftCredit().Amount))
					moneyUpdateCh <- struct{}{}
				}
			}
		}
	}()
	go func() {
		for {
			select {
			case <-endCh:
				return
			case <-moneyUpdateCh:
				menu.SetCredit(moneysys.Credit(ctx))
				moneysys.AcceptCredit(ctx, menuMap.MaxPrice())
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

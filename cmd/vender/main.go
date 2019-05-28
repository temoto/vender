package main

import (
	"flag"
	"os"

	"github.com/coreos/go-systemd/daemon"
	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
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

	ctx, g := state.NewContext(log)
	g.MustInit(ctx, state.MustReadConfig(log, state.NewOsFullReader(), *flagConfig))
	log.Debugf("config=%+v", g.Config())

	moneysys := new(money.MoneySystem)
	moneysys.Start(ctx)

	mdber, err := g.Mdber()
	if err != nil {
		log.Fatalf("mdb init err=%v", errors.ErrorStack(err))
	}

	a := alive.NewAlive()
	g.Alive = a
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
	log.Debugf("menu len=%d", len(menuMap))

	menu := ui.NewUIMenu(ctx, menuMap)
	moneysys.EventSubscribe(func(em money.Event) {
		menu.SetCredit(moneysys.Credit(ctx))

		log.Debugf("money event: %s", em.String())
		switch em.Name() {
		case money.EventCredit:
		case money.EventAbort:
		default:
			panic("head: unknown money event: " + em.String())
		}
	})
	telesys := &state.GetGlobal(ctx).Tele
	go func() {
		stopCh := a.StopChan()
		for {
			select {
			case <-stopCh:
				return
			case cmd := <-telesys.CommandChan():
				switch cmd.Task.(type) {
				case *tele.Command_Abort:
					err := moneysys.Abort(ctx)
					telesys.CommandReplyErr(&cmd, err)
					log.Infof("admin requested abort err=%v", err)
				case *tele.Command_SetGiftCredit:
					moneysys.SetGiftCredit(ctx, currency.Amount(cmd.GetSetGiftCredit().Amount))
				}
			}
		}
	}()

	g.Inventory.DisableAll()
	log.Debugf("vender init complete, running")
	// TODO listen /dev/input/event0 switch to AdminUI()
	for a.IsRunning() {
		menu.SetCredit(moneysys.Credit(ctx))
		moneysys.AcceptCredit(ctx, menuMap.MaxPrice())

		menuResult := menu.Run()
		log.Debugf("menu result=%#v", menuResult)
		if menuResult.Confirm {
			itemCtx := money.SetCurrentPrice(ctx, menuResult.Item.Price)
			err := menuResult.Item.D.Do(itemCtx)
			if err == nil {
				// telesys.
			} else {
				err = errors.Annotatef(err, "execute %s", menuResult.Item.String())
				log.Errorf(errors.ErrorStack(err))
				telesys.Error(err)
			}
		}
	}

	a.Wait()
}

func sdnotify(s string) bool {
	ok, err := daemon.SdNotify(false, s)
	if err != nil {
		log.Fatal("sdnotify: ", errors.ErrorStack(err))
	}
	return ok
}

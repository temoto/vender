package main

import (
	"flag"
	"os"

	"github.com/coreos/go-systemd/daemon"
	"github.com/temoto/alive"
	"github.com/temoto/errors"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/mdb/evend"
	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/head/tele"
	"github.com/temoto/vender/head/ui"
	"github.com/temoto/vender/log2"
	"github.com/temoto/vender/state"
)

var log = log2.NewStderr(log2.LDebug)

func mainerr(configPath string) error {
	ctx, g := state.NewContext(log)
	g.MustInit(ctx, state.MustReadConfig(log, state.NewOsFullReader(), configPath))
	log.Debugf("config=%+v", g.Config())

	mdber, err := g.Mdber()
	if err != nil {
		err = errors.Annotate(err, "mdb init")
		return err
	}
	if err = mdber.BusResetDefault(); err != nil {
		err = errors.Annotate(err, "mdb bus reset")
		return err
	}

	moneysys := new(money.MoneySystem)
	if err = moneysys.Start(ctx); err != nil {
		err = errors.Annotate(err, "money system Start()")
		return err
	}

	// TODO func(dev Devicer) { dev.Init() && dev.Register() }
	// right now Enum does IO implicitly
	// FIXME hardware.Enum() but money system inits bill/coin devices explicitly
	evend.Enum(ctx, nil)

	menuMap := make(ui.Menu)
	if err = menuMap.Init(ctx); err != nil {
		err = errors.Annotate(err, "menuMap.Init")
		return err
	}
	log.Debugf("uiFront len=%d", len(menuMap))

	uiFront := ui.NewUIFront(ctx, menuMap)
	uiService := ui.NewUIService(ctx)

	moneysys.EventSubscribe(func(em money.Event) {
		log.Debugf("money event: %s", em.String())
		uiFront.SetCredit(moneysys.Credit(ctx))

		switch em.Name() {
		case money.EventCredit:
		case money.EventAbort:
		default:
			panic("head: unknown money event: " + em.String())
		}
	})
	telesys := &state.GetGlobal(ctx).Tele
	go func() {
		stopCh := g.Alive.StopChan()
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

	uiFrontRunner := &state.FuncRunner{Name: "ui-front", F: func(uia *alive.Alive) {
		uiFront.SetCredit(moneysys.Credit(ctx))
		moneysys.AcceptCredit(ctx, menuMap.MaxPrice())
		menuResult := uiFront.Run(uia)
		log.Debugf("uiFront result=%#v", menuResult)
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
	}}
	g.Hardware.Input.SubscribeFunc("service", func(e input.Event) {
		if e.Source == input.DevInputEventTag && e.Up {
			log.Debugf("input event switch to service")
			g.UISwitch(uiService)
		}
	}, g.Alive.StopChan())

	g.Inventory.DisableAll()
	sdnotify(daemon.SdNotifyReady)
	log.Debugf("vender init complete, running")

	for g.Alive.IsRunning() {
		g.UINext(uiFrontRunner)
	}
	g.Alive.Wait()
	return nil
}

func main() {
	errors.SetSourceTrimPrefix(os.Getenv("source_trim_prefix"))

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

	if err := mainerr(*flagConfig); err != nil {
		log.Fatal(errors.ErrorStack(err))
	}
}

func sdnotify(s string) bool {
	ok, err := daemon.SdNotify(false, s)
	if err != nil {
		log.Fatal("sdnotify: ", errors.ErrorStack(err))
	}
	return ok
}

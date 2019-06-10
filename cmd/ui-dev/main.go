// Temporary executable for developing UI
package main

import (
	"flag"
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/input"
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

	log.SetFlags(log2.LInteractiveFlags)

	ctx, g := state.NewContext(log)
	g.MustInit(ctx, state.MustReadConfig(log, state.NewOsFullReader(), *flagConfig))
	log.Debugf("config=%+v", g.Config())

	log.Debugf("Init display")
	d := g.Hardware.HD44780.Display
	d.SetLine1("loaded")
	d.SetLine2("test long wrap bla bla hello world")

	moneysys := new(money.MoneySystem)
	moneysys.Start(ctx)

	menuMap := make(ui.Menu)
	menuMap.Add(1, "chai", g.Config().ScaleU(3),
		engine.Func0{F: func() error {
			d.SetLines("спасибо", "готовим...")
			time.Sleep(7 * time.Second)
			d.SetLines("успех", "спасибо")
			time.Sleep(3 * time.Second)
			return nil
		}})
	menuMap.Add(2, "coffee", g.Config().ScaleU(5),
		engine.Func0{F: func() error {
			d.SetLines("спасибо", "готовим...")
			time.Sleep(7 * time.Second)
			d.SetLines("успех", "спасибо")
			time.Sleep(3 * time.Second)
			return nil
		}})

	uiClient := ui.NewUIMenu(ctx, menuMap)
	uiService := ui.NewUIService(ctx)

	moneysys.EventSubscribe(func(em money.Event) {
		uiClient.SetCredit(moneysys.Credit(ctx))
		log.Debugf("money event: %s", em.String())
		moneysys.AcceptCredit(ctx, menuMap.MaxPrice())
	})

	telesys := &g.Tele
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
					uiClient.SetCredit(moneysys.Credit(ctx))
					moneysys.AcceptCredit(ctx, menuMap.MaxPrice())
				}
			}
		}
	}()

	log.Debugf("vender-ui-dev init complete, running")
	uiClientRunner := &state.FuncRunner{Name: "ui-client", F: func(uia *alive.Alive) {
		uiClient.SetCredit(moneysys.Credit(ctx))
		moneysys.AcceptCredit(ctx, menuMap.MaxPrice())
		result := uiClient.Run(uia)
		log.Debugf("uiClient result=%#v", result)
		if result.Confirm {
			itemCtx := money.SetCurrentPrice(ctx, result.Item.Price)
			err := result.Item.D.Do(itemCtx)
			if err == nil {
				// telesys.
			} else {
				err = errors.Annotatef(err, "execute %s", result.Item.String())
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

	for g.Alive.IsRunning() {
		g.UINext(uiClientRunner)
		// err := moneysys.WithdrawPrepare(ctx, result.Item.Price)
		// if err == money.ErrNeedMoreMoney {
		// 	log.Errorf("uiClientitem=%v price=%s err=%v", result.Item, result.Item.Price.FormatCtx(ctx), err)
		// } else if err == nil {
		// 	if err = result.Item.D.Do(ctx); err != nil {
		// 		log.Errorf("uiClientitem=%v execute err=%v", result.Item, err)
		// 		moneysys.Abort(ctx)
		// 	} else {
		// 		moneysys.WithdrawCommit(ctx, result.Item.Price)
		// 	}
		// } else {
		// 	log.Errorf("uiClientitem=%v price=%s err=%v", result.Item, result.Item.Price.FormatCtx(ctx), err)
		// }
	}
}

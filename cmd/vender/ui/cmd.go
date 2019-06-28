// Helper for developing vender user interfaces
package ui

import (
	"context"
	"time"

	"github.com/temoto/alive"
	"github.com/temoto/errors"
	"github.com/temoto/vender/cmd/vender/subcmd"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/head/tele"
	"github.com/temoto/vender/head/ui"
	"github.com/temoto/vender/state"
)

var Mod = subcmd.Mod{Name: "ui", Main: Main}

func Main(ctx context.Context, config *state.Config) error {
	g := state.GetGlobal(ctx)
	g.MustInit(ctx, config)
	g.Log.Debugf("config=%+v", g.Config())

	g.Log.Debugf("Init display")
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

	uiFront := ui.NewUIFront(ctx, menuMap)
	uiService := ui.NewUIService(ctx)

	moneysys.EventSubscribe(func(em money.Event) {
		uiFront.SetCredit(moneysys.Credit(ctx))
		g.Log.Debugf("money event: %s", em.String())
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
					g.Log.Infof("admin requested abort err=%v", err)
					uiFront.SetCredit(moneysys.Credit(ctx))
					moneysys.AcceptCredit(ctx, menuMap.MaxPrice())
				}
			}
		}
	}()

	g.Log.Debugf("vender-ui-dev init complete, running")
	uiFrontRunner := &state.FuncRunner{Name: "ui-front", F: func(uia *alive.Alive) {
		uiFront.SetCredit(moneysys.Credit(ctx))
		moneysys.AcceptCredit(ctx, menuMap.MaxPrice())
		result := uiFront.Run(uia)
		g.Log.Debugf("uiFront result=%#v", result)
		if result.Confirm {
			itemCtx := money.SetCurrentPrice(ctx, result.Item.Price)
			err := result.Item.D.Do(itemCtx)
			if err == nil {
				// telesys.
			} else {
				err = errors.Annotatef(err, "execute %s", result.Item.String())
				g.Log.Errorf(errors.ErrorStack(err))
				telesys.Error(err)
			}
		}
	}}
	g.Hardware.Input.SubscribeFunc("service", func(e input.Event) {
		if e.Source == input.DevInputEventTag && e.Up {
			g.Log.Debugf("input event switch to service")
			g.UISwitch(uiService)
		}
	}, g.Alive.StopChan())

	for g.Alive.IsRunning() {
		g.UINext(uiFrontRunner)
		// err := moneysys.WithdrawPrepare(ctx, result.Item.Price)
		// if err == money.ErrNeedMoreMoney {
		// 	g.Log.Errorf("uiFrontitem=%v price=%s err=%v", result.Item, result.Item.Price.FormatCtx(ctx), err)
		// } else if err == nil {
		// 	if err = result.Item.D.Do(ctx); err != nil {
		// 		g.Log.Errorf("uiFrontitem=%v execute err=%v", result.Item, err)
		// 		moneysys.Abort(ctx)
		// 	} else {
		// 		moneysys.WithdrawCommit(ctx, result.Item.Price)
		// 	}
		// } else {
		// 	g.Log.Errorf("uiFrontitem=%v price=%s err=%v", result.Item, result.Item.Price.FormatCtx(ctx), err)
		// }
	}

	return nil
}

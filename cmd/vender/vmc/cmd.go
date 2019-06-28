package vmc

import (
	"context"

	"github.com/coreos/go-systemd/daemon"
	"github.com/temoto/alive"
	"github.com/temoto/errors"
	"github.com/temoto/vender/cmd/vender/subcmd"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/mdb/evend"
	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/head/tele"
	"github.com/temoto/vender/head/ui"
	"github.com/temoto/vender/state"
)

var Mod = subcmd.Mod{Name: "vmc", Main: Main}

func Main(ctx context.Context, config *state.Config) error {
	g := state.GetGlobal(ctx)
	g.MustInit(ctx, config)
	g.Log.Debugf("config=%+v", g.Config())

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
	g.Log.Debugf("uiFront len=%d", len(menuMap))

	uiFront := ui.NewUIFront(ctx, menuMap)
	uiService := ui.NewUIService(ctx)

	moneysys.EventSubscribe(func(em money.Event) {
		g.Log.Debugf("money event: %s", em.String())
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
					g.Log.Infof("admin requested abort err=%v", err)
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
		g.Log.Debugf("uiFront result=%#v", menuResult)
		if menuResult.Confirm {
			itemCtx := money.SetCurrentPrice(ctx, menuResult.Item.Price)
			err := menuResult.Item.D.Do(itemCtx)
			if err == nil {
				// telesys.
			} else {
				err = errors.Annotatef(err, "execute %s", menuResult.Item.String())
				g.Log.Errorf(errors.ErrorStack(err))

				g.Log.Errorf("tele.error")
				telesys.Error(err)

				g.Log.Errorf("on_menu_error")
				if err := g.Engine.ExecList(ctx, "on_menu_error", g.Config().Engine.OnMenuError); err != nil {
					g.Log.Error(err)
				}
			}
		}
	}}
	g.Hardware.Input.SubscribeFunc("service", func(e input.Event) {
		if e.Source == input.DevInputEventTag && e.Up {
			g.Log.Debugf("input event switch to service")
			g.UISwitch(uiService)
		}
	}, g.Alive.StopChan())

	// FIXME
	g.Inventory.DisableAll()

	subcmd.SdNotify(daemon.SdNotifyReady)
	g.Log.Debugf("VMC init complete")

	subcmd.SdNotify("executing on_start")
	if err := g.Engine.ExecList(ctx, "on_start", g.Config().Engine.OnStart); err != nil {
		g.Log.Fatal(err)
	}

	for g.Alive.IsRunning() {
		g.UINext(uiFrontRunner)
	}
	g.Alive.Wait()
	return nil
}

package vmc_common

import (
	"context"

	"github.com/temoto/alive"
	"github.com/temoto/errors"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/head/tele"
	"github.com/temoto/vender/head/ui"
	"github.com/temoto/vender/state"
)

func TeleCommandLoop(ctx context.Context) {
	g := state.GetGlobal(ctx)
	moneysys := money.GetGlobal(ctx)
	stopCh := g.Alive.StopChan()
	for {
		select {
		case <-stopCh:
			return
		case cmd := <-g.Tele.CommandChan():
			switch cmd.Task.(type) {
			case *tele.Command_Abort:
				err := moneysys.Abort(ctx)
				g.Tele.CommandReplyErr(&cmd, err)
				g.Log.Infof("admin requested abort err=%v", err)
			case *tele.Command_SetGiftCredit:
				moneysys.SetGiftCredit(ctx, currency.Amount(cmd.GetSetGiftCredit().Amount))
			}
		}
	}
}

func uiFrontFinish(ctx context.Context, menuResult *ui.UIMenuResult) {
	g := state.GetGlobal(ctx)
	moneysys := money.GetGlobal(ctx)
	g.Log.Debugf("ui-front result=%#v", menuResult)
	if !menuResult.Confirm {
		return
	}

	moneysys.AcceptCredit(ctx, 0)
	teletx := tele.Telemetry_Transaction{
		Code:  int32(menuResult.Item.Code),
		Price: uint32(menuResult.Item.Price),
		// TODO options
		// TODO payment method
		// TODO bills, coins
	}
	g.Log.Debugf("menu item=%s begin", menuResult.Item.String())
	if err := moneysys.WithdrawPrepare(ctx, menuResult.Item.Price); err != nil {
		g.Log.Debugf("ui-front CRITICAL error while return change")
	}
	itemCtx := money.SetCurrentPrice(ctx, menuResult.Item.Price)
	err := menuResult.Item.D.Do(itemCtx)
	g.Log.Debugf("menu item=%s end", menuResult.Item.String())
	if err == nil {
		g.Tele.Transaction(teletx)
	} else {
		err = errors.Annotatef(err, "execute %s", menuResult.Item.String())
		g.Log.Errorf(errors.ErrorStack(err))

		g.Log.Errorf("tele.error")
		g.Tele.Error(err)

		g.Log.Errorf("on_menu_error")
		if err := g.Engine.ExecList(ctx, "on_menu_error", g.Config().Engine.OnMenuError); err != nil {
			g.Log.Errorf("on_menu_error err=%v", err)
		} else {
			g.Log.Infof("on_menu_error success")
		}
	}
}

func UILoop(ctx context.Context, uiFront *ui.UIFront) {
	g := state.GetGlobal(ctx)
	gstopch := g.Alive.StopChan()
	uiFront.Finish = uiFrontFinish
	uiService := ui.NewUIService(ctx)
	switchService := make(chan struct{})

	g.Hardware.Input.SubscribeFunc("service", func(e input.Event) {
		if e.Source == input.DevInputEventTag && e.Up {
			g.Log.Debugf("input event switch to service")
			switchService <- struct{}{}
		}
	}, g.Alive.StopChan())

	for g.Alive.IsRunning() {
		na := alive.NewAlive()
		g.Log.Infof("uiloop front start")
		go uiFront.Run(ctx, na)
		select {
		case <-switchService:
			na.Stop()
			na.Wait()
			na = alive.NewAlive()
			uiService.Run(ctx, na)
		case <-na.StopChan():
			na.Wait()
		case <-gstopch:
			return
		}
	}
}

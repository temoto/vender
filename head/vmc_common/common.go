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

func TeleCommandLoop(ctx context.Context, moneysys *money.MoneySystem) {
	g := state.GetGlobal(ctx)
	telesys := &state.GetGlobal(ctx).Tele
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
}

func uiFrontFinish(ctx context.Context, menuResult *ui.UIMenuResult) {
	g := state.GetGlobal(ctx)
	g.Log.Debugf("uiFront result=%#v", menuResult)
	if menuResult.Confirm {
		g.Log.Debugf("uiFront confirmed")
		itemCtx := money.SetCurrentPrice(ctx, menuResult.Item.Price)
		g.Log.Debugf("uiFront curprice set")
		err := menuResult.Item.D.Do(itemCtx)
		g.Log.Debugf("uiFront item Do end err=%v", err)
		if err == nil {
			// g.Tele.
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
}

func UILoop(ctx context.Context, uiFront *ui.UIFront, moneysys *money.MoneySystem, menuMap ui.Menu) {
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
		go func() {
			uiFront.SetCredit(moneysys.Credit(ctx))
			moneysys.AcceptCredit(ctx, menuMap.MaxPrice())
			uiFront.Run(ctx, na)
		}()
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

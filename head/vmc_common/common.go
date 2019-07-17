package vmc_common

import (
	"context"
	"log"

	"github.com/temoto/alive"
	"github.com/temoto/errors"
	"github.com/temoto/vender/currency"
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
	display := g.MustDisplay()
	uiConfig := &g.Config.UI
	moneysys := money.GetGlobal(ctx)
	g.Log.Debugf("ui-front result=%#v", menuResult)
	if !menuResult.Confirm {
		return
	}

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
	display.SetLines("спасибо", "готовлю")
	log.Printf("menuResult.Item: %#v", menuResult.Item)
	log.Printf("menuResult.Item.D: %#v", menuResult.Item.D)
	err := menuResult.Item.D.Do(itemCtx)
	g.Log.Debugf("menu item=%s end", menuResult.Item.String())
	if err == nil {
		g.Tele.Transaction(teletx)
	} else {
		err = errors.Annotatef(err, "execute %s", menuResult.Item.String())
		g.Log.Errorf(errors.ErrorStack(err))

		g.Log.Errorf("tele.error")
		g.Tele.Error(err)

		display.SetLines(uiConfig.Front.MsgError, "не получилось")
		g.Log.Errorf("on_menu_error")
		if err := g.Engine.ExecList(ctx, "on_menu_error", g.Config.Engine.OnMenuError); err != nil {
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

	for g.Alive.IsRunning() {
		na := alive.NewAlive()
		g.Log.Infof("uiloop front start")
		go uiFront.Run(ctx, na)
		select {
		case <-na.StopChan():
			na.Wait()
			if uiFront.SwitchService {
				uiFront.SwitchService = false
				na = alive.NewAlive()
				uiService.Run(ctx, na)
			}
		case <-gstopch:
			na.Stop()
			return
		}
	}
}

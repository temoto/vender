package vmc_common

import (
	"context"

	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/head/tele"
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

package money

import (
	"context"

	"github.com/temoto/vender/head/state"
)

func init() {
	state.RegisterStart(func(ctx context.Context) error {
		bill.Init(ctx)
		changer.Init(ctx)
		return nil
	})

	state.RegisterStop(func(ctx context.Context) error {
		Abort(ctx)
		bill.Stop(ctx)
		changer.Stop(ctx)
		// TODO return escrow
		return nil
	})
}

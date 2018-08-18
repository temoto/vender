package money

import (
	"context"

	"github.com/temoto/vender/head/state"
)

func init() {
	state.RegisterStart(func(ctx context.Context) error {
		globalBs.Init(ctx)
		globalCs.Init(ctx)
		return nil
	})

	state.RegisterStop(func(ctx context.Context) error {
		Abort(ctx)
		globalBs.Stop(ctx)
		globalCs.Stop(ctx)
		// TODO return escrow
		return nil
	})
}

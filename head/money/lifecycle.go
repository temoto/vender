package money

import (
	"context"

	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/head/state"
)

func init() {
	state.RegisterStart(func(ctx context.Context) error {
		m := mdb.ContextValueMdber(ctx, "run/mdber")
		globalBs.Init(ctx, m)
		globalCs.Init(ctx, m)
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

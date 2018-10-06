package money

import (
	"context"

	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/head/state"
)

func init() {
	state.RegisterStart(func(ctx context.Context) error {
		m := mdb.ContextValueMdber(ctx, "run/mdber")
		Global.bs.Init(ctx, m)
		Global.cs.Init(ctx, m)
		return nil
	})

	state.RegisterStop(func(ctx context.Context) error {
		Global.Abort(ctx)
		Global.bs.Stop(ctx)
		Global.cs.Stop(ctx)
		// TODO return escrow
		return nil
	})
}

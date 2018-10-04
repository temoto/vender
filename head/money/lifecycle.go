package money

import (
	"context"

	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/head/state"
)

func init() {
	state.RegisterStart(func(ctx context.Context) error {
		m := mdb.ContextValueMdber(ctx, "run/mdber")
		Global.bs.Init(ctx, m, Global.Events())
		Global.cs.Init(ctx, m, Global.Events())
		return nil
	})

	state.RegisterStop(func(ctx context.Context) error {
		Abort(ctx)
		Global.bs.Stop(ctx)
		Global.cs.Stop(ctx)
		// TODO return escrow
		return nil
	})
}

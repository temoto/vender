package kitchen

import (
	"context"

	"github.com/temoto/vender/head/state"
)

func init() {
	state.RegisterStart(func(ctx context.Context) error {
		// TODO init kitchen devices
		return nil
	})
}

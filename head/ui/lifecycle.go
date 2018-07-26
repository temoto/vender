package ui

import (
	"context"

	"github.com/temoto/vender/head/state"
)

func init() {
	state.RegisterStart(func(ctx context.Context) error {
		// TODO init keyboard
		// TODO init lcd
		return nil
	})
}

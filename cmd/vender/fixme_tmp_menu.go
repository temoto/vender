package main

import (
	"context"
	"strconv"

	"github.com/temoto/vender/head/ui"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

func menuInit(ctx context.Context, menuMap ui.Menu) error {
	config := state.GetGlobal(ctx).Config()

	errs := make([]error, 0, 16)
	for _, x := range config.Engine.Menu.Items {
		codeInt, _ := strconv.Atoi(x.Code)
		menuMap.Add(uint16(codeInt), x.Name, x.Price, x.Doer)
	}
	return helpers.FoldErrors(errs)
}

package main

import (
	"context"
	"strconv"

	"github.com/temoto/vender/head/ui"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

func menuInit(ctx context.Context, menuMap ui.Menu) error {
	config := state.GetConfig(ctx)

	errs := make([]error, 0, 16)
	for _, x := range config.Menu.Items {
		price := config.ScaleU(uint32(x.Price))
		codeInt, _ := strconv.Atoi(x.Code)
		menuMap.Add(uint16(codeInt), x.Name, price, x.Doer)
	}
	return helpers.FoldErrors(errs)
}

package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/engine/inventory"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

const DefaultHopperRate = 10
const DefaultHopperRunTimeout = 200 * time.Millisecond

type DeviceHopper struct {
	Generic

	stock *inventory.Stock
}

func (self *DeviceHopper) Init(ctx context.Context, addr uint8, nameSuffix string) error {
	g := state.GetGlobal(ctx)
	hopperConfig := &g.Config().Hardware.Evend.Hopper
	name := "hopper" + nameSuffix
	err := self.Generic.Init(ctx, addr, name, proto2)

	rate := hopperConfig.DefaultStockRate
	if rate == 0 {
		rate = DefaultHopperRate
	}
	self.stock = g.Inventory.Register(name, rate)

	g.Engine.Register(fmt.Sprintf("mdb.evend.%s_run(?)", name), self.NewRun())

	return err
}

func (self *DeviceHopper) NewRun() engine.Doer {
	tag := fmt.Sprintf("mdb.evend.%s.run", self.dev.Name)

	do := engine.FuncArg{Name: tag, F: func(ctx context.Context, arg engine.Arg) error {
		hopperConfig := &state.GetGlobal(ctx).Config().Hardware.Evend.Hopper
		units := uint8(arg)

		if err := self.Generic.NewWaitReady(tag).Do(ctx); err != nil {
			return err
		}

		if err := self.Generic.txAction([]byte{units}); err != nil {
			return err
		}

		runTimeout := helpers.IntMillisecondDefault(hopperConfig.RunTimeoutMs, DefaultHopperRunTimeout)
		return self.Generic.NewWaitDone(tag, runTimeout*time.Duration(units)).Do(ctx)
	}}

	return self.stock.WrapArg(do)
}

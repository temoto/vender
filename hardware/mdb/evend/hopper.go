package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

const DefaultHopperRunTimeout = 200 * time.Millisecond
const HopperTimeout = 1 * time.Second

type DeviceHopper struct {
	Generic
}

func (self *DeviceHopper) Init(ctx context.Context, addr uint8, nameSuffix string) error {
	g := state.GetGlobal(ctx)
	name := "hopper" + nameSuffix
	err := self.Generic.Init(ctx, addr, name, proto2)
	if err != nil {
		return errors.Annotatef(err, "evend.%s.Init", name)
	}

	do := self.NewRun()
	g.Engine.Register(fmt.Sprintf("mdb.evend.%s_run(?)", name), do)

	return nil
}

func (self *DeviceHopper) NewRun() engine.FuncArg {
	tag := fmt.Sprintf("mdb.evend.%s.run", self.dev.Name)

	return engine.FuncArg{Name: tag, F: func(ctx context.Context, arg engine.Arg) error {
		hopperConfig := &state.GetGlobal(ctx).Config.Hardware.Evend.Hopper
		units := uint8(arg)
		runTimeout := helpers.IntMillisecondDefault(hopperConfig.RunTimeoutMs, DefaultHopperRunTimeout)

		if err := self.Generic.NewWaitReady(tag).Do(ctx); err != nil {
			return err
		}
		if err := self.Generic.txAction([]byte{units}); err != nil {
			return err
		}
		return self.Generic.NewWaitDone(tag, runTimeout*time.Duration(units)+HopperTimeout).Do(ctx)
	}}
}

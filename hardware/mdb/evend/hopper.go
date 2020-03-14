package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/internal/engine"
	"github.com/temoto/vender/internal/state"
)

const DefaultHopperRunTimeout = 200 * time.Millisecond
const HopperTimeout = 1 * time.Second

type DeviceHopper struct {
	Generic
}

func (self *DeviceHopper) init(ctx context.Context, addr uint8, nameSuffix string) error {
	name := "hopper" + nameSuffix
	g := state.GetGlobal(ctx)
	self.Generic.Init(ctx, addr, name, proto2)

	do := newHopperRun(&self.Generic, fmt.Sprintf("%s.run", self.name), nil)
	g.Engine.Register(fmt.Sprintf("%s.run(?)", self.name), do)

	err := self.Generic.FIXME_initIO(ctx)
	return errors.Annotate(err, self.name+".init")
}

type DeviceMultiHopper struct {
	Generic
}

func (self *DeviceMultiHopper) init(ctx context.Context) error {
	const addr = 0xb8
	g := state.GetGlobal(ctx)
	self.Generic.Init(ctx, addr, "multihopper", proto1)

	for i := uint8(1); i <= 8; i++ {
		do := newHopperRun(
			&self.Generic,
			fmt.Sprintf("%s%d.run", self.name, i),
			[]byte{i},
		)
		g.Engine.Register(fmt.Sprintf("%s%d.run(?)", self.name, i), do)
	}

	err := self.Generic.FIXME_initIO(ctx)
	return errors.Annotate(err, self.name+".init")
}

func newHopperRun(gen *Generic, tag string, argsPrefix []byte) engine.FuncArg {
	return engine.FuncArg{Name: tag, F: func(ctx context.Context, arg engine.Arg) error {
		hopperConfig := &state.GetGlobal(ctx).Config.Hardware.Evend.Hopper
		units := uint8(arg)
		runTimeout := helpers.IntMillisecondDefault(hopperConfig.RunTimeoutMs, DefaultHopperRunTimeout)

		if err := gen.NewWaitReady(tag).Do(ctx); err != nil {
			return err
		}
		args := append(argsPrefix, units)
		if err := gen.txAction(args); err != nil {
			return err
		}
		return gen.NewWaitDone(tag, runTimeout*time.Duration(units)+HopperTimeout).Do(ctx)
	}}
}

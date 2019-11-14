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

	do := newHopperRun(&self.Generic, fmt.Sprintf("mdb.evend.%s.run", name), nil)
	g.Engine.Register(fmt.Sprintf("mdb.evend.%s_run(?)", name), do)

	return nil
}

type DeviceMultiHopper struct {
	Generic
}

func (self *DeviceMultiHopper) Init(ctx context.Context) error {
	const addr = 0xb8
	const name = "multihopper"
	g := state.GetGlobal(ctx)
	err := self.Generic.Init(ctx, addr, "multihopper", proto1)
	if err != nil {
		return errors.Annotatef(err, "evend.%s.Init", name)
	}

	for i := uint8(1); i <= 8; i++ {
		do := newHopperRun(
			&self.Generic,
			fmt.Sprintf("mdb.evend.%s%d.run", name, i),
			[]byte{i},
		)
		g.Engine.Register(fmt.Sprintf("mdb.evend.%s%d_run(?)", name, i), do)
	}

	return nil
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

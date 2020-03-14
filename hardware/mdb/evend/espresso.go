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

const DefaultEspressoTimeout = 30 * time.Second

type DeviceEspresso struct {
	Generic

	timeout time.Duration
}

func (self *DeviceEspresso) init(ctx context.Context) error {
	g := state.GetGlobal(ctx)
	espressoConfig := &g.Config.Hardware.Evend.Espresso
	self.timeout = helpers.IntSecondDefault(espressoConfig.TimeoutSec, DefaultEspressoTimeout)
	self.Generic.Init(ctx, 0xe8, "espresso", proto2)

	g.Engine.Register(self.name+".grind", self.Generic.WithRestart(self.NewGrind()))
	g.Engine.Register(self.name+".press", self.NewPress())
	g.Engine.Register(self.name+".dispose", self.Generic.WithRestart(self.NewRelease()))
	g.Engine.Register(self.name+".heat_on", self.NewHeat(true))
	g.Engine.Register(self.name+".heat_off", self.NewHeat(false))

	err := self.Generic.FIXME_initIO(ctx)
	return errors.Annotate(err, self.name+".init")
}

func (self *DeviceEspresso) NewGrind() engine.Doer {
	tag := self.name + ".grind"
	return engine.NewSeq(tag).
		Append(self.Generic.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x01)).
		// TODO expect delay like in cup dispense, ignore immediate error, retry
		Append(self.Generic.NewWaitDone(tag, self.timeout))
}

func (self *DeviceEspresso) NewPress() engine.Doer {
	tag := self.name + ".press"
	return engine.NewSeq(tag).
		Append(self.Generic.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x02)).
		Append(self.Generic.NewWaitDone(tag, self.timeout))
}

func (self *DeviceEspresso) NewRelease() engine.Doer {
	tag := self.name + ".release"
	return engine.NewSeq(tag).
		Append(self.Generic.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x03)).
		Append(self.Generic.NewWaitDone(tag, self.timeout))
}

func (self *DeviceEspresso) NewHeat(on bool) engine.Doer {
	tag := fmt.Sprintf("%s.heat:%t", self.name, on)
	arg := byte(0x05)
	if !on {
		arg = 0x06
	}
	return engine.NewSeq(tag).
		Append(self.Generic.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, arg))
}

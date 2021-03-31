package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/internal/engine"
	"github.com/temoto/vender/internal/global"
	"github.com/temoto/vender/internal/state"
)

const DefaultCupAssertBusyDelay = 30 * time.Millisecond
const DefaultCupDispenseTimeout = 10 * time.Second
const DefaultCupEnsureTimeout = 70 * time.Second

type DeviceCup struct {
	Generic
}

func (self *DeviceCup) init(ctx context.Context) error {
	self.Generic.Init(ctx, 0xe0, "cup", proto2)

	g := state.GetGlobal(ctx)
	doDispense := self.Generic.WithRestart(self.NewDispenseProper())
	g.Engine.Register(self.name+".dispense", doDispense)
	g.Engine.Register(self.name+".light_on", self.NewLight(true))
	g.Engine.Register(self.name+".light_off", self.NewLight(false))
	g.Engine.Register(self.name+".ensure", self.NewEnsure())

	err := self.Generic.FIXME_initIO(ctx)
	return errors.Annotate(err, self.name+".init")
}

func (self *DeviceCup) NewDispenseProper() engine.Doer {
	return engine.NewSeq(self.name + ".dispense_proper").
		Append(self.NewEnsure()).
		Append(self.NewDispense())
}

func (self *DeviceCup) NewDispense() engine.Doer {
	tag := self.name + ".dispense"
	return engine.NewSeq(tag).
		Append(self.Generic.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x01)).
		Append(engine.Func{Name: tag + "/assert-busy", F: func(ctx context.Context) error {
			cupConfig := &state.GetGlobal(ctx).Config.Hardware.Evend.Cup
			time.Sleep(helpers.IntMillisecondDefault(cupConfig.AssertBusyDelayMs, DefaultCupAssertBusyDelay))
			response := mdb.Packet{}
			err := self.dev.TxKnown(self.dev.PacketPoll, &response)
			if err != nil {
				return err
			}
			bs := response.Bytes()
			if len(bs) != 1 {
				return self.NewErrPollUnexpected(response)
			}
			if bs[0] != self.proto2BusyMask {
				self.dev.Log.Errorf("expected BUSY, cup device is broken")
				return self.NewErrPollUnexpected(response)
			}
			return nil
		}}).
		Append(engine.Func{
			F: func(ctx context.Context) error {
				g := state.GetGlobal(ctx)
				cupConfig := &g.Config.Hardware.Evend.Cup
				dispenseTimeout := helpers.IntSecondDefault(cupConfig.DispenseTimeoutSec, DefaultCupDispenseTimeout)
				return g.Engine.Exec(ctx, self.Generic.NewWaitDone(tag, dispenseTimeout))
			},
		})
}

func (self *DeviceCup) NewLight(on bool) engine.Doer {
	tag := fmt.Sprintf("%s.light:%t", self.name, on)
	arg := byte(0x02)
	if !on {
		arg = 0x03
	}
	// return { self.Generic.NewAction(tag, arg)}
	return engine.NewSeq(tag).
		Append(self.Generic.NewAction(tag, arg)).
		Append(engine.Func0{F: func() error { _ = global.ChSetEnvB("light.working", on); return nil }})

}

func (self *DeviceCup) NewEnsure() engine.Doer {
	tag := self.name + ".ensure"
	return engine.NewSeq(tag).
		Append(self.Generic.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x04)).
		Append(engine.Func{
			F: func(ctx context.Context) error {
				g := state.GetGlobal(ctx)
				cupConfig := &g.Config.Hardware.Evend.Cup
				ensureTimeout := helpers.IntSecondDefault(cupConfig.EnsureTimeoutSec, DefaultCupEnsureTimeout)
				return g.Engine.Exec(ctx, self.Generic.NewWaitDone(tag, ensureTimeout))
			},
		})
}

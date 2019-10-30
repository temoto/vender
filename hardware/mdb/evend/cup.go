package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

const DefaultCupAssertBusyDelay = 30 * time.Millisecond
const DefaultCupDispenseTimeout = 10 * time.Second
const DefaultCupEnsureTimeout = 70 * time.Second

type DeviceCup struct {
	Generic
}

func (self *DeviceCup) Init(ctx context.Context) error {
	g := state.GetGlobal(ctx)
	err := self.Generic.Init(ctx, 0xe0, "cup", proto2)
	if err != nil {
		return errors.Annotate(err, "evend.cup.Init")
	}

	doDispense := self.Generic.WithRestart(self.NewDispenseProper())
	g.Engine.Register("mdb.evend.cup_dispense", doDispense)
	g.Engine.Register("mdb.evend.cup_light_on", self.NewLight(true))
	g.Engine.Register("mdb.evend.cup_light_off", self.NewLight(false))
	g.Engine.Register("mdb.evend.cup_ensure", self.NewEnsure())

	return nil
}

func (self *DeviceCup) NewDispenseProper() engine.Doer {
	const tag = "mdb.evend.cup.dispense_proper"
	return engine.NewSeq(tag).
		Append(self.NewEnsure()).
		Append(self.NewDispense())
}

func (self *DeviceCup) NewDispense() engine.Doer {
	const tag = "mdb.evend.cup.dispense"
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
				cupConfig := &state.GetGlobal(ctx).Config.Hardware.Evend.Cup
				dispenseTimeout := helpers.IntSecondDefault(cupConfig.DispenseTimeoutSec, DefaultCupDispenseTimeout)
				return self.Generic.NewWaitDone(tag, dispenseTimeout).Do(ctx)
			},
		})
}

func (self *DeviceCup) NewLight(on bool) engine.Doer {
	tag := fmt.Sprintf("mdb.evend.cup.light:%t", on)
	arg := byte(0x02)
	if !on {
		arg = 0x03
	}
	return self.Generic.NewAction(tag, arg)
}

func (self *DeviceCup) NewEnsure() engine.Doer {
	const tag = "mdb.evend.cup.ensure"
	return engine.NewSeq(tag).
		Append(self.Generic.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x04)).
		Append(engine.Func{
			F: func(ctx context.Context) error {
				cupConfig := &state.GetGlobal(ctx).Config.Hardware.Evend.Cup
				ensureTimeout := helpers.IntSecondDefault(cupConfig.EnsureTimeoutSec, DefaultCupEnsureTimeout)
				return self.Generic.NewWaitDone(tag, ensureTimeout).Do(ctx)
			},
		})
}

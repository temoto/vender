package evend

import (
	"context"
	"fmt"

	// "go/types"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/internal/engine"
	"github.com/temoto/vender/internal/state"
	"github.com/temoto/vender/internal/types"
)

const DefaultCupAssertBusyDelay = 30 * time.Millisecond
const DefaultCupDispenseTimeout = 10 * time.Second
const DefaultCupEnsureTimeout = 70 * time.Second

type DeviceCup struct {
	Generic
	dispenseTimeout   time.Duration
	assertBusyDelayMs time.Duration
}

func (self *DeviceCup) init(ctx context.Context) error {
	self.Generic.Init(ctx, 0xe0, "cup", proto2)

	g := state.GetGlobal(ctx)
	doDispense := self.Generic.WithRestart(self.NewDispenseProper())
	g.Engine.Register(self.name+".dispense", doDispense)
	g.Engine.Register(self.name+".light_on", self.NewLight(true))
	g.Engine.Register(self.name+".light_off", self.NewLight(false))
	g.Engine.Register(self.name+".ensure", self.NewEnsure())
	self.dispenseTimeout = helpers.IntSecondDefault(g.Config.Hardware.Evend.Cup.DispenseTimeoutSec, DefaultCupDispenseTimeout)
	self.assertBusyDelayMs = helpers.IntMillisecondDefault(g.Config.Hardware.Evend.Cup.AssertBusyDelayMs, DefaultCupAssertBusyDelay)

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
		Append(engine.Func0{F: func() error { types.Log.Info("cup dispence"); return nil }}).
		Append(self.Generic.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x01)).
		Append(engine.Func{Name: tag + "/assert-busy", F: func(ctx context.Context) error {
			time.Sleep(self.assertBusyDelayMs)
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
				return g.Engine.Exec(ctx, self.Generic.NewWaitDone(tag, self.dispenseTimeout))
			},
		})
}

func (self *DeviceCup) NewLight(v bool) engine.Doer {
	tag := fmt.Sprintf("%s.light:%t", self.name, v)
	arg := byte(0x02)
	if !v {
		arg = 0x03
	}
	types.SetLight(v)
	// return self.Generic.NewAction(tag, arg)
	return engine.NewSeq(tag).
		Append(self.Generic.NewAction(tag, arg)).
		Append(engine.Func0{F: func() error { types.SetLight(v); return nil }})

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

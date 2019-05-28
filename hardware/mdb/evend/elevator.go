package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

type DeviceElevator struct {
	Generic

	timeout    time.Duration
	calibrated bool
}

func (self *DeviceElevator) Init(ctx context.Context) error {
	self.calibrated = false
	g := state.GetGlobal(ctx)
	config := &g.Config().Hardware.Evend.Elevator
	self.timeout = helpers.IntSecondDefault(config.TimeoutSec, 10*time.Second)
	err := self.Generic.Init(ctx, 0xd0, "elevator", proto1)

	doCalibrate := engine.Func{
		Name: "mdb.evend.elevator.calibrate",
		F:    self.calibrate,
	}
	doMove := engine.FuncArg{
		Name: "mdb.evend.elevator.move",
		F: func(ctx context.Context, arg engine.Arg) error {
			return self.move(ctx, uint8(arg))
		},
		V: func() error {
			if !self.calibrated {
				return nil
			}
			// FIXME Generic offline -> calibrated=false
			if err := self.Generic.dev.ValidateOnline(); err != nil {
				self.calibrated = false
				return err
			}
			return nil
		},
	}
	g.Engine.RegisterNewSeq("mdb.evend.elevator_move(?)", doCalibrate, doMove)

	return err
}

func (self *DeviceElevator) calibrate(ctx context.Context) error {
	if self.calibrated {
		return nil
	}
	err := self.move(ctx, 0)
	if err == nil {
		self.calibrated = true
	}
	return err
}

func (self *DeviceElevator) move(ctx context.Context, position uint8) error {
	tag := fmt.Sprintf("mdb.evend.elevator.move:%d", position)
	return engine.NewSeq(tag).
		Append(self.Generic.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x03, position, 0)).
		Append(self.Generic.NewWaitDone(tag, self.timeout)).Do(ctx)
}

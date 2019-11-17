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
	currentPos int16 // estimated
}

func (self *DeviceElevator) init(ctx context.Context) error {
	self.currentPos = -1
	g := state.GetGlobal(ctx)
	config := &g.Config.Hardware.Evend.Elevator
	keepaliveInterval := helpers.IntMillisecondDefault(config.KeepaliveMs, 0)
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
			if self.currentPos == -1 {
				return nil
			}
			// FIXME Generic offline -> calibrated=false
			if err := self.Generic.dev.ValidateOnline(); err != nil {
				self.currentPos = -1
				return err
			}
			if err := self.Generic.dev.ValidateErrorCode(); err != nil {
				self.currentPos = -1
				return err
			}
			return nil
		},
	}
	moveSeq := engine.NewSeq("mdb.evend.elevator_move(?)").Append(doCalibrate).Append(doMove)
	g.Engine.Register(moveSeq.String(), self.Generic.WithRestart(moveSeq))

	if keepaliveInterval > 0 {
		go self.Generic.dev.Keepalive(keepaliveInterval, g.Alive.StopChan())
	}

	return err
}

func (self *DeviceElevator) calibrate(ctx context.Context) error {
	// self.dev.Log.Debugf("mdb.evend.elevator calibrate ready=%t current=%d", self.dev.Ready(), self.currentPos)
	if self.currentPos != -1 {
		return nil
	}
	err := self.move(ctx, 0)
	// if err == nil {
	// 	self.dev.Log.Debugf("mdb.evend.elevator calibrate success")
	// }
	return err
}

func (self *DeviceElevator) move(ctx context.Context, position uint8) (err error) {
	tag := fmt.Sprintf("mdb.evend.elevator.move:%d", position)
	// self.dev.Log.Debugf("mdb.evend.elevator calibrate ready=%t current=%d", self.dev.Ready(), self.currentPos)
	defer func() {
		if err != nil {
			self.currentPos = -1
		} else {
			self.currentPos = int16(position)
			self.dev.SetReady()
		}
	}()

	if err = self.Generic.NewWaitReady(tag).Do(ctx); err != nil {
		return
	}
	if err = self.Generic.NewAction(tag, 0x03, position, 0).Do(ctx); err != nil {
		return
	}
	err = self.Generic.NewWaitDone(tag, self.timeout).Do(ctx)
	return
}

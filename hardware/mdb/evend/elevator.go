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

type DeviceElevator struct {
	Generic

	timeout time.Duration
	cal0    bool
	cal100  bool
}

func (self *DeviceElevator) init(ctx context.Context) error {
	g := state.GetGlobal(ctx)
	config := &g.Config.Hardware.Evend.Elevator
	keepaliveInterval := helpers.IntMillisecondDefault(config.KeepaliveMs, 0)
	self.timeout = helpers.IntSecondDefault(config.TimeoutSec, 10*time.Second)
	self.Generic.Init(ctx, 0xd0, "elevator", proto1)

	doMove := engine.FuncArg{
		Name: "mdb.evend.elevator.move",
		F:    self.moveProper,
		V: func() error {
			// FIXME Generic offline -> calibrated=false
			if err := self.Generic.dev.ValidateOnline(); err != nil {
				self.calReset()
				return err
			}
			if err := self.Generic.dev.ValidateErrorCode(); err != nil {
				self.calReset()
				return err
			}
			return nil
		},
	}
	g.Engine.Register("mdb.evend.elevator_move(?)", self.Generic.WithRestart(doMove))

	err := self.Generic.FIXME_initIO(ctx)
	if keepaliveInterval > 0 {
		go self.Generic.dev.Keepalive(keepaliveInterval, g.Alive.StopChan())
	}
	return errors.Annotatef(err, "evend.%s.init", self.dev.Name)
}

func (self *DeviceElevator) calibrated() bool { return self.cal0 && self.cal100 }
func (self *DeviceElevator) calReset()        { self.cal0 = false; self.cal100 = false }
func (self *DeviceElevator) calibrate(ctx context.Context) error {
	const tag = "evend.elevator.calibrate"
	self.dev.Log.Debugf("mdb.evend.elevator calibrate ready=%t cal0=%t cal100=%t", self.dev.Ready(), self.cal0, self.cal100)
	if !self.cal0 {
		if err := self.moveRaw(ctx, 0); err != nil {
			return errors.Annotate(err, tag)
		}
	}
	if !self.cal100 {
		if err := self.moveRaw(ctx, 100); err != nil {
			return errors.Annotate(err, tag)
		}
	}
	return nil
}

func (self *DeviceElevator) moveProper(ctx context.Context, arg engine.Arg) (err error) {
	position := uint8(arg)

	if !(position == 0 || position == 100) {
		if err = self.calibrate(ctx); err != nil {
			return
		}
	}

	err = self.moveRaw(ctx, arg)
	return
}

func (self *DeviceElevator) moveRaw(ctx context.Context, arg engine.Arg) (err error) {
	position := uint8(arg)
	tag := fmt.Sprintf("mdb.evend.elevator.moveRaw:%d", position)
	defer func() {
		if err != nil {
			self.calReset()
		} else {
			switch position {
			case 0:
				self.cal0 = true
			case 100:
				self.cal100 = true
			}
			if self.calibrated() {
				self.dev.SetReady()
			}
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

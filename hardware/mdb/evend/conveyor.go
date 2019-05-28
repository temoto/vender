package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/state"
)

type DeviceConveyor struct {
	Generic

	maxTimeout time.Duration
	minSpeed   uint16
	calibrated bool
	currentPos uint16 // estimated
}

func (self *DeviceConveyor) Init(ctx context.Context) error {
	self.calibrated = false
	self.currentPos = 0
	g := state.GetGlobal(ctx)
	devConfig := &g.Config().Hardware.Evend.Conveyor
	self.maxTimeout = speedDistanceDuration(float32(devConfig.MinSpeed), uint(devConfig.PositionMax))
	self.minSpeed = uint16(devConfig.MinSpeed)
	if self.minSpeed == 0 {
		self.minSpeed = 200
	}
	err := self.Generic.Init(ctx, 0xd8, "conveyor", proto2)

	doCalibrate := engine.Func{Name: "mdb.evend.conveyor.calibrate", F: self.calibrate}
	doMove := engine.FuncArg{Name: "mdb.evend.conveyor.move", F: func(ctx context.Context, arg engine.Arg) error { return self.move(ctx, uint16(arg)) }}
	g.Engine.RegisterNewSeq("mdb.evend.conveyor_move(?)", doCalibrate, doMove)

	return err
}

func (self *DeviceConveyor) calibrate(ctx context.Context) error {
	if self.calibrated {
		return nil
	}
	err := self.move(ctx, 0)
	if err == nil {
		self.calibrated = true
	}
	return err
}

func (self *DeviceConveyor) move(ctx context.Context, position uint16) error {
	tag := fmt.Sprintf("mdb.evend.conveyor.move:%d", position)

	if err := self.Generic.NewWaitReady(tag).Do(ctx); err != nil {
		return err
	}
	if err := self.Generic.NewAction(tag, 0x01, byte(position&0xff), byte(position>>8)).Do(ctx); err != nil {
		return err
	}

	timeout := self.maxTimeout
	if self.calibrated {
		distance := absDiffU16(self.currentPos, position)
		eta := speedDistanceDuration(float32(self.minSpeed), uint(distance))
		timeout = eta * 2
	}
	self.dev.Log.Debugf("%s position current=%d target=%d timeout=%s", tag, self.currentPos, position, timeout)
	if err := self.Generic.NewWaitDone(tag, timeout).Do(ctx); err != nil {
		self.calibrated = false
		return err
	}
	self.currentPos = position
	return nil
}

func speedDistanceDuration(speedPerSecond float32, distance uint) time.Duration {
	return time.Duration(float32(distance)/speedPerSecond) * time.Second
}

func absDiffU16(a, b uint16) uint16 {
	if a >= b {
		return a - b
	}
	return b - a
}

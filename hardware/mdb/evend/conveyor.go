package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/state"
)

const ConveyorDefaultTimeout = 30 * time.Second

type DeviceConveyor struct {
	Generic

	maxTimeout time.Duration
	minSpeed   uint16
	currentPos int16 // estimated
}

func (self *DeviceConveyor) Init(ctx context.Context) error {
	self.currentPos = -1
	g := state.GetGlobal(ctx)
	devConfig := &g.Config().Hardware.Evend.Conveyor
	self.maxTimeout = speedDistanceDuration(float32(devConfig.MinSpeed), uint(devConfig.PositionMax))
	if self.maxTimeout == 0 {
		self.maxTimeout = ConveyorDefaultTimeout
	}
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
	self.dev.Log.Debugf("mdb.evend.conveyor calibrate ready=%t current=%d", self.dev.Ready(), self.currentPos)
	if self.currentPos >= 0 {
		return nil
	}
	self.dev.Log.Debugf("mdb.evend.conveyor calibrate begin")
	err := self.move(ctx, 0)
	if err == nil {
		self.dev.Log.Debugf("mdb.evend.conveyor calibrate success")
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
	if self.dev.Ready() && self.currentPos >= 0 {
		distance := absDiffU16(uint16(self.currentPos), position)
		eta := speedDistanceDuration(float32(self.minSpeed), uint(distance))
		timeout = eta * 2
	}
	self.dev.Log.Debugf("%s position current=%d target=%d timeout=%v maxtimeout=%v", tag, self.currentPos, position, timeout, self.maxTimeout)
	if err := self.Generic.NewWaitDone(tag, timeout).Do(ctx); err != nil {
		self.currentPos = -1
		self.dev.SetReady(false)
		return err
	} else {
		self.currentPos = int16(position)
		self.dev.SetReady(true)
	}
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

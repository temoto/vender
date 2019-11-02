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

const ConveyorDefaultTimeout = 30 * time.Second
const ConveyorMinTimeout = 1 * time.Second

type DeviceConveyor struct { //nolint:maligned
	Generic

	maxTimeout time.Duration
	minSpeed   uint16
	currentPos int16 // estimated
}

func (self *DeviceConveyor) Init(ctx context.Context) error {
	self.currentPos = -1
	g := state.GetGlobal(ctx)
	devConfig := &g.Config.Hardware.Evend.Conveyor
	keepaliveInterval := helpers.IntMillisecondDefault(devConfig.KeepaliveMs, 0)
	self.minSpeed = uint16(devConfig.MinSpeed)
	if self.minSpeed == 0 {
		self.minSpeed = 200
	}
	self.maxTimeout = speedDistanceDuration(float32(self.minSpeed), uint(devConfig.PositionMax))
	if self.maxTimeout == 0 {
		self.maxTimeout = ConveyorDefaultTimeout
	}
	g.Log.Debugf("mdb.evend.conveyor minSpeed=%d maxTimeout=%v keepalive=%v", self.minSpeed, self.maxTimeout, keepaliveInterval)
	self.dev.DelayNext = 245 * time.Millisecond // empirically found lower total WaitReady
	err := self.Generic.Init(ctx, 0xd8, "conveyor", proto2)

	doCalibrate := engine.Func{Name: "mdb.evend.conveyor.calibrate", F: self.calibrate}
	doMove := engine.FuncArg{
		Name: "mdb.evend.conveyor.move",
		F: func(ctx context.Context, arg engine.Arg) error {
			return self.move(ctx, uint16(arg))
		}}
	moveSeq := engine.NewSeq("mdb.evend.conveyor_move(?)").Append(doCalibrate).Append(doMove)
	g.Engine.Register(moveSeq.String(), self.Generic.WithRestart(moveSeq))

	doShake := engine.FuncArg{
		Name: "mdb.evend.conveyor.shake",
		F: func(ctx context.Context, arg engine.Arg) error {
			return self.shake(ctx, uint8(arg))
		}}
	g.Engine.RegisterNewSeq("mdb.evend.conveyor_shake(?)", doCalibrate, doShake)

	if keepaliveInterval > 0 {
		go self.Generic.dev.Keepalive(keepaliveInterval, g.Alive.StopChan())
	}

	return err
}

func (self *DeviceConveyor) calibrate(ctx context.Context) error {
	const tag = "mdb.evend.conveyor.calibrate"
	// self.dev.Log.Debugf("mdb.evend.conveyor calibrate ready=%t current=%d", self.dev.Ready(), self.currentPos)
	if self.currentPos >= 0 {
		return nil
	}
	// self.dev.Log.Debugf("mdb.evend.conveyor calibrate begin")
	err := self.move(ctx, 0)
	if err == nil {
		self.dev.Log.Debugf("mdb.evend.conveyor calibrate success")
	}
	return errors.Annotate(err, tag)
}

func (self *DeviceConveyor) move(ctx context.Context, position uint16) error {
	tag := fmt.Sprintf("mdb.evend.conveyor.move:%d", position)

	doWaitDone := engine.Func{F: func(ctx context.Context) error {
		timeout := self.maxTimeout
		if self.dev.Ready() && self.currentPos >= 0 {
			distance := absDiffU16(uint16(self.currentPos), position)
			eta := speedDistanceDuration(float32(self.minSpeed), uint(distance))
			timeout = eta * 2
		}
		if timeout < ConveyorMinTimeout {
			timeout = ConveyorMinTimeout
		}
		self.dev.Log.Debugf("%s position current=%d target=%d timeout=%v maxtimeout=%v", tag, self.currentPos, position, timeout, self.maxTimeout)

		err := self.Generic.NewWaitDone(tag, timeout).Do(ctx)
		if err != nil {
			self.currentPos = -1
			// TODO check SetReady(false)
		} else {
			self.currentPos = int16(position)
			self.dev.SetReady()
		}
		return err
	}}

	// TODO engine InlineSeq
	seq := engine.NewSeq(tag).
		Append(self.Generic.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x01, byte(position&0xff), byte(position>>8))).
		Append(doWaitDone)
	err := seq.Do(ctx)
	return errors.Annotate(err, tag)
}

func (self *DeviceConveyor) shake(ctx context.Context, arg uint8) error {
	tag := fmt.Sprintf("mdb.evend.conveyor.shake:%d", arg)

	doWaitDone := engine.Func{F: func(ctx context.Context) error {
		err := self.Generic.NewWaitDone(tag, self.maxTimeout).Do(ctx)
		if err != nil {
			self.currentPos = -1
			// TODO check SetReady(false)
		}
		return err
	}}

	// TODO engine InlineSeq
	seq := engine.NewSeq(tag).
		Append(self.Generic.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x03, byte(arg), 0)).
		Append(doWaitDone)
	err := seq.Do(ctx)
	return errors.Annotate(err, tag)
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

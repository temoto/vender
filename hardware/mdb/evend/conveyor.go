package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/internal/engine"
	"github.com/temoto/vender/internal/state"
)

const ConveyorDefaultTimeout = 30 * time.Second
const ConveyorMinTimeout = 1 * time.Second

type DeviceConveyor struct { //nolint:maligned
	Generic

	DoSetSpeed engine.FuncArg
	maxTimeout time.Duration
	minSpeed   uint16
	currentPos int16 // estimated
}

func (self *DeviceConveyor) init(ctx context.Context) error {
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
	g.Log.Debugf("evend.conveyor minSpeed=%d maxTimeout=%v keepalive=%v", self.minSpeed, self.maxTimeout, keepaliveInterval)
	self.dev.DelayNext = 245 * time.Millisecond // empirically found lower total WaitReady
	self.Generic.Init(ctx, 0xd8, "conveyor", proto2)
	self.DoSetSpeed = self.newSetSpeed()

	doCalibrate := engine.Func{Name: self.name + ".calibrate", F: self.calibrate}
	doMove := engine.FuncArg{
		Name: self.name + ".move",
		F: func(ctx context.Context, arg engine.Arg) error {
			return self.move(ctx, uint16(arg))
		}}
	moveSeq := engine.NewSeq(self.name + ".move(?)").Append(doCalibrate).Append(doMove)
	g.Engine.Register(moveSeq.String(), self.Generic.WithRestart(moveSeq))
	g.Engine.Register(self.name+".set_speed(?)", self.DoSetSpeed)

	doShake := engine.FuncArg{
		Name: self.name + ".shake",
		F: func(ctx context.Context, arg engine.Arg) error {
			return self.shake(ctx, uint8(arg))
		}}
	g.Engine.RegisterNewSeq(self.name+".shake(?)", doCalibrate, doShake)

	err := self.Generic.FIXME_initIO(ctx)
	if keepaliveInterval > 0 {
		go self.Generic.dev.Keepalive(keepaliveInterval, g.Alive.StopChan())
	}
	return errors.Annotate(err, self.name+".init")
}

func (self *DeviceConveyor) calibrate(ctx context.Context) error {
	// self.dev.Log.Debugf("%s calibrate ready=%t current=%d", self.name, self.dev.Ready(), self.currentPos)
	if self.currentPos >= 0 {
		return nil
	}
	// self.dev.Log.Debugf("%s calibrate begin", self.name)
	err := self.move(ctx, 0)
	if err == nil {
		self.dev.Log.Debugf("%s calibrate success", self.name)
	}
	return errors.Annotate(err, self.name+".calibrate")
}

func (self *DeviceConveyor) move(ctx context.Context, position uint16) error {
	g := state.GetGlobal(ctx)
	tag := fmt.Sprintf("%s.move:%d", self.name, position)
	tbegin := time.Now()
	if g.Config.Hardware.Evend.Conveyor.LogDebug {
		self.dev.Log.Debugf("%s begin", tag)
	}

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

		err := g.Engine.Exec(ctx, self.Generic.NewWaitDone(tag, timeout))
		if err != nil {
			self.currentPos = -1
			// TODO check SetReady(false)
		} else {
			self.currentPos = int16(position)
			self.dev.SetReady()
			if g.Config.Hardware.Evend.Conveyor.LogDebug {
				self.dev.Log.Debugf("%s duration=%s", tag, time.Since(tbegin))
			}
		}
		return err
	}}

	// TODO engine InlineSeq
	seq := engine.NewSeq(tag).
		Append(self.Generic.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x01, byte(position&0xff), byte(position>>8))).
		Append(doWaitDone)
	err := g.Engine.Exec(ctx, seq)
	return errors.Annotate(err, tag)
}

func (self *DeviceConveyor) shake(ctx context.Context, arg uint8) error {
	g := state.GetGlobal(ctx)
	tag := fmt.Sprintf("%s.shake:%d", self.name, arg)

	doWaitDone := engine.Func{F: func(ctx context.Context) error {
		err := g.Engine.Exec(ctx, self.Generic.NewWaitDone(tag, self.maxTimeout))
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
	err := g.Engine.Exec(ctx, seq)
	return errors.Annotate(err, tag)
}

func (self *DeviceConveyor) newSetSpeed() engine.FuncArg {
	tag := self.name + ".set_speed"

	return engine.FuncArg{Name: tag, F: func(ctx context.Context, arg engine.Arg) error {
		speed := uint8(arg)
		bs := []byte{self.dev.Address + 5, 0x10, speed}
		request := mdb.MustPacketFromBytes(bs, true)
		response := mdb.Packet{}
		err := self.dev.TxCustom(request, &response, mdb.TxOpt{})
		if err != nil {
			return errors.Annotatef(err, "%s target=%d request=%x", tag, speed, request.Bytes())
		}
		self.dev.Log.Debugf("%s target=%d request=%x response=%x", tag, speed, request.Bytes(), response.Bytes())
		return nil
	}}
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

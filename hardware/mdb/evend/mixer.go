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

const DefaultShakeSpeed uint8 = 100

type DeviceMixer struct { //nolint:maligned
	Generic

	currentPos   int16 // estimated
	moveTimeout  time.Duration
	shakeTimeout time.Duration
	shakeSpeed   uint8
}

func (self *DeviceMixer) init(ctx context.Context) error {
	self.currentPos = -1
	self.shakeSpeed = DefaultShakeSpeed
	g := state.GetGlobal(ctx)
	config := &g.Config.Hardware.Evend.Mixer
	keepaliveInterval := helpers.IntMillisecondDefault(config.KeepaliveMs, 0)
	self.moveTimeout = helpers.IntSecondDefault(config.MoveTimeoutSec, 10*time.Second)
	self.shakeTimeout = helpers.IntMillisecondDefault(config.ShakeTimeoutMs, 300*time.Millisecond)
	self.Generic.Init(ctx, 0xc8, "mixer", proto1)

	doCalibrate := engine.Func{Name: self.name + ".calibrate", F: self.calibrate}
	doMove := engine.FuncArg{Name: self.name + ".move", F: func(ctx context.Context, arg engine.Arg) error {
		if self.currentPos == 0 && arg == 0 {
			self.dev.Log.Debugf("evend.mixer currentPos=0 skip")
			return nil
		}
		return self.move(uint8(arg)).Do(ctx)
	}}
	moveSeq := engine.NewSeq(self.name + ".move(?)").Append(doCalibrate).Append(doMove)
	g.Engine.Register(self.name+".shake(?)",
		engine.FuncArg{Name: self.name + ".shake", F: func(ctx context.Context, arg engine.Arg) error {
			return self.Generic.WithRestart(self.shake(uint8(arg))).Do(ctx)
		}})
	g.Engine.Register(self.name+".fan_on", self.NewFan(true))
	g.Engine.Register(self.name+".fan_off", self.NewFan(false))
	g.Engine.Register(moveSeq.String(), self.Generic.WithRestart(moveSeq))
	g.Engine.Register(self.name+".shake_set_speed(?)",
		engine.FuncArg{Name: "evend.mixer.shake_set_speed", F: func(ctx context.Context, arg engine.Arg) error {
			self.shakeSpeed = uint8(arg)
			return nil
		}})

	err := self.Generic.FIXME_initIO(ctx)
	if keepaliveInterval > 0 {
		go self.Generic.dev.Keepalive(keepaliveInterval, g.Alive.StopChan())
	}
	return errors.Annotate(err, self.name+".init")
}

// 1step = 100ms
func (self *DeviceMixer) shake(steps uint8) engine.Doer {
	tag := fmt.Sprintf("%s.shake:%d,%d", self.name, steps, self.shakeSpeed)
	return engine.NewSeq(tag).
		Append(self.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x01, steps, self.shakeSpeed)).
		Append(self.NewWaitDone(tag, self.shakeTimeout*time.Duration(1+steps)))
}

func (self *DeviceMixer) NewFan(on bool) engine.Doer {
	tag := fmt.Sprintf("%s.fan:%t", self.name, on)
	arg := uint8(0)
	if on {
		arg = 1
	}
	return self.Generic.NewAction(tag, 0x02, arg, 0x00)
}

func (self *DeviceMixer) calibrate(ctx context.Context) error {
	if self.currentPos >= 0 {
		return nil
	}
	err := self.move(0).Do(ctx)
	if err == nil {
		self.currentPos = 0
	}
	return err
}

func (self *DeviceMixer) move(position uint8) engine.Doer {
	tag := fmt.Sprintf("%s.move:%d", self.name, position)
	return engine.NewSeq(tag).
		Append(self.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x03, position, 0x64)).
		Append(self.NewWaitDone(tag, self.moveTimeout)).
		Append(engine.Func0{F: func() error { self.currentPos = int16(position); return nil }})
}

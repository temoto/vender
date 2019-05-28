package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

const DefaultShakeSpeed uint8 = 100

type DeviceMixer struct { //nolint:maligned
	Generic

	calibrated   bool
	moveTimeout  time.Duration
	shakeTimeout time.Duration
	shakeSpeed   uint8
}

func (self *DeviceMixer) Init(ctx context.Context) error {
	self.calibrated = false
	self.shakeSpeed = DefaultShakeSpeed
	g := state.GetGlobal(ctx)
	config := &g.Config().Hardware.Evend.Mixer
	self.moveTimeout = helpers.IntSecondDefault(config.MoveTimeoutSec, 10*time.Second)
	self.shakeTimeout = helpers.IntMillisecondDefault(config.ShakeTimeoutMs, 300*time.Millisecond)
	err := self.Generic.Init(ctx, 0xc8, "mixer", proto1)

	doCalibrate := engine.Func{Name: "mdb.evend.mixer_calibrate", F: self.calibrate}
	doMove := engine.FuncArg{Name: "mdb.evend.mixer_move", F: func(ctx context.Context, arg engine.Arg) error { return self.move(uint8(arg)).Do(ctx) }}
	g.Engine.Register("mdb.evend.mixer_shake(?)",
		engine.FuncArg{Name: "mdb.evend.mixer_shake", F: func(ctx context.Context, arg engine.Arg) error {
			return self.shake(uint8(arg)).Do(ctx)
		}})
	g.Engine.Register("mdb.evend.mixer_fan_on", self.NewFan(true))
	g.Engine.Register("mdb.evend.mixer_fan_off", self.NewFan(false))
	g.Engine.RegisterNewSeq("mdb.evend.mixer_move(?)", doCalibrate, doMove)
	g.Engine.Register("mdb.evend.mixer_shake_set_speed(?)",
		engine.FuncArg{Name: "mdb.evend.mixer.shake_set_speed", F: func(ctx context.Context, arg engine.Arg) error {
			self.shakeSpeed = uint8(arg)
			return nil
		}})

	return err
}

// 1step = 100ms
func (self *DeviceMixer) shake(steps uint8) engine.Doer {
	tag := fmt.Sprintf("mdb.evend.mixer.shake:%d,%d", steps, self.shakeSpeed)
	return engine.NewSeq(tag).
		Append(self.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x01, steps, self.shakeSpeed)).
		Append(self.NewWaitDone(tag, self.shakeTimeout*time.Duration(1+steps)))
}

func (self *DeviceMixer) NewFan(on bool) engine.Doer {
	tag := fmt.Sprintf("mdb.evend.mixer.fan:%t", on)
	arg := uint8(0)
	if on {
		arg = 1
	}
	return self.Generic.NewAction(tag, 0x02, arg, 0x00)
}

func (self *DeviceMixer) calibrate(ctx context.Context) error {
	if self.calibrated {
		return nil
	}
	err := self.move(0).Do(ctx)
	if err == nil {
		self.calibrated = true
	}
	return err
}

func (self *DeviceMixer) move(position uint8) engine.Doer {
	tag := fmt.Sprintf("mdb.evend.mixer.move:%d", position)
	return engine.NewSeq(tag).
		Append(self.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x03, position, 0x64)).
		Append(self.NewWaitDone(tag, self.moveTimeout))
}

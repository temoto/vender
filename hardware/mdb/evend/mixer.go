package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/temoto/vender/engine"
)

type DeviceMixer struct {
	Generic

	moveTimeout  time.Duration
	shakeTimeout time.Duration

	shakeSpeedNormal uint8
	shakeSpeedClean  uint8

	posClean uint8
	posReady uint8
	posShake uint8
}

func (self *DeviceMixer) Init(ctx context.Context) error {
	// TODO read config
	self.moveTimeout = 10 * time.Second
	self.shakeTimeout = 3 * 100 * time.Millisecond
	self.shakeSpeedClean = 100
	self.shakeSpeedNormal = 15
	self.posClean = 70
	self.posReady = 0
	self.posShake = 100
	err := self.Generic.Init(ctx, 0xc8, "mixer", proto1)

	e := engine.GetEngine(ctx)
	e.Register("mdb.evend.mixer_shake_normal(?)", self.NewShakeNormal())
	e.Register("mdb.evend.mixer_shake_clean(?)", self.NewShakeClean())
	e.Register("mdb.evend.mixer_fan_on", self.NewFan(true))
	e.Register("mdb.evend.mixer_fan_off", self.NewFan(false))
	e.Register("mdb.evend.mixer_move_clean", self.NewMove(self.posClean))
	e.Register("mdb.evend.mixer_move_ready", self.NewMove(self.posReady))
	e.Register("mdb.evend.mixer_move_shake", self.NewMove(self.posShake))

	return err
}

// 1step = 100ms
func (self *DeviceMixer) NewShake(steps uint8, speed uint8) engine.Doer {
	tag := fmt.Sprintf("mdb.evend.mixer.shake:%d,%d", steps, speed)
	return engine.NewSeq(tag).
		Append(self.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x01, steps, speed)).
		Append(self.NewWaitDone(tag, self.shakeTimeout*time.Duration(1+steps)))
}
func (self *DeviceMixer) NewShakeNormal() engine.Doer {
	const tag = "mdb.evend.mixer.shake_normal"
	return engine.FuncArg{Name: tag, F: func(ctx context.Context, arg engine.Arg) error {
		return self.NewShake(uint8(arg), self.shakeSpeedNormal).Do(ctx)
	}}
}
func (self *DeviceMixer) NewShakeClean() engine.Doer {
	const tag = "mdb.evend.mixer.shake_clean"
	return engine.FuncArg{Name: tag, F: func(ctx context.Context, arg engine.Arg) error {
		return self.NewShake(uint8(arg), self.shakeSpeedClean).Do(ctx)
	}}
}

func (self *DeviceMixer) NewFan(on bool) engine.Doer {
	tag := fmt.Sprintf("mdb.evend.mixer.fan:%t", on)
	arg := uint8(0)
	if on {
		arg = 1
	}
	return self.Generic.NewAction(tag, 0x02, arg, 0x00)
}

func (self *DeviceMixer) NewMove(position uint8) engine.Doer {
	tag := fmt.Sprintf("mdb.evend.mixer.move:%d", position)
	return engine.NewSeq(tag).
		Append(self.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x03, position, 0x64)).
		Append(self.NewWaitDone(tag, self.moveTimeout))
}

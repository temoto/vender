package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/temoto/vender/engine"
)

type DeviceMixer struct {
	Generic

	moveTimeout time.Duration
	posClean    uint8
	posReady    uint8
	posShake    uint8
}

func (self *DeviceMixer) Init(ctx context.Context) error {
	// TODO read config
	self.moveTimeout = 10 * time.Second
	self.posClean = 70
	self.posReady = 0
	self.posShake = 100
	err := self.Generic.Init(ctx, 0xc8, "mixer", proto1)

	engine := engine.ContextValueEngine(ctx, engine.ContextKey)
	engine.Register("mdb.evend.mixer_shake_1", self.NewShakeSync(100*time.Millisecond, 100))
	engine.Register("mdb.evend.mixer_shake_2", self.NewShakeSync(200*time.Millisecond, 100))
	engine.Register("mdb.evend.mixer_shake_clean", self.NewShakeSync(1000*time.Millisecond, 15))
	engine.Register("mdb.evend.mixer_fan_on", self.NewFan(true))
	engine.Register("mdb.evend.mixer_fan_off", self.NewFan(false))
	engine.Register("mdb.evend.mixer_move_clean", self.NewMoveSync(self.posClean))
	engine.Register("mdb.evend.mixer_move_ready", self.NewMoveSync(self.posReady))
	engine.Register("mdb.evend.mixer_move_shake", self.NewMoveSync(self.posShake))

	return err
}

func (self *DeviceMixer) NewShake(d time.Duration, speed uint8) engine.Doer {
	tag := fmt.Sprintf("%s.shake:%d,%d", self.dev.Name, d, speed)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		argDuration := uint8(d / time.Millisecond / 100)
		arg := []byte{0x01, argDuration, speed}
		return self.CommandAction(ctx, arg)
	}}
}
func (self *DeviceMixer) NewShakeSync(d time.Duration, speed uint8) engine.Doer {
	timeout := 10 * time.Second //XXX
	tag := fmt.Sprintf("%s.shake_sync:%d,%d", self.dev.Name, d, speed)
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.dev.NewPollUntilEmpty(tag+"/wait-ready", timeout, nil)).
		Append(self.NewShake(d, speed)).
		Append(self.NewProto1PollWaitSuccess(tag+"/wait-done", timeout))
	return tx
}

func (self *DeviceMixer) NewFan(on bool) engine.Doer {
	tag := fmt.Sprintf("%s.fan:%t", self.dev.Name, on)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		argOn := uint8(0)
		if on {
			argOn = 1
		}
		arg := []byte{0x02, argOn, 0}
		return self.CommandAction(ctx, arg)
	}}
}

func (self *DeviceMixer) NewMove(position uint8) engine.Doer {
	tag := fmt.Sprintf("%s.move:%d", self.dev.Name, position)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		arg := []byte{0x03, position, 0x64}
		return self.CommandAction(ctx, arg)
	}}
}
func (self *DeviceMixer) NewMoveSync(position uint8) engine.Doer {
	tag := fmt.Sprintf("%s.move_sync:%d", self.dev.Name, position)
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.dev.NewPollUntilEmpty(tag+"/wait-ready", self.moveTimeout, nil)).
		Append(self.NewMove(position)).
		Append(self.NewProto1PollWaitSuccess(tag+"/wait-done", self.moveTimeout))
	return tx
}

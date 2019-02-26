package evend

import (
	"context"
	"time"

	"github.com/temoto/vender/engine"
)

type DeviceMixer struct {
	Generic
}

func (self *DeviceMixer) Init(ctx context.Context) error {
	// TODO read config
	return self.Generic.Init(ctx, 0xc8, "mixer")
}

func (self *DeviceMixer) NewShake(d time.Duration, speed uint8) engine.Doer {
	return engine.Func{F: func(ctx context.Context) error {
		argDuration := uint8(d / time.Millisecond / 100)
		arg := []byte{0x01, argDuration, speed}
		return self.CommandAction(ctx, arg)
	}}
}

func (self *DeviceMixer) NewFan(on bool) engine.Doer {
	return engine.Func{F: func(ctx context.Context) error {
		argOn := uint8(0)
		if on {
			argOn = 1
		}
		arg := []byte{0x02, argOn, 0}
		return self.CommandAction(ctx, arg)
	}}
}

func (self *DeviceMixer) NewMove(position uint8) engine.Doer {
	return engine.Func{F: func(ctx context.Context) error {
		arg := []byte{0x03, position, 0x64}
		return self.CommandAction(ctx, arg)
	}}
}

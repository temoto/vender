package evend

import (
	"context"

	"github.com/temoto/vender/engine"
)

type DeviceCoffee struct {
	Generic
}

func (self *DeviceCoffee) Init(ctx context.Context) error {
	// TODO read config
	err := self.Generic.Init(ctx, 0xe8, "coffee")

	return err
}

func (self *DeviceCup) NewGrind() engine.Doer {
	return engine.Func{F: func(ctx context.Context) error {
		return self.Generic.CommandAction(ctx, []byte{0x01})
	}}
}

func (self *DeviceCup) NewPress() engine.Doer {
	return engine.Func{F: func(ctx context.Context) error {
		return self.Generic.CommandAction(ctx, []byte{0x02})
	}}
}

func (self *DeviceCup) NewDispose() engine.Doer {
	return engine.Func{F: func(ctx context.Context) error {
		return self.Generic.CommandAction(ctx, []byte{0x03})
	}}
}

func (self *DeviceCup) New_maybe_Heat(on bool) engine.Doer {
	return engine.Func{F: func(ctx context.Context) error {
		arg := byte(0x05)
		if !on {
			arg = 0x06
		}
		return self.Generic.CommandAction(ctx, []byte{arg})
	}}
}

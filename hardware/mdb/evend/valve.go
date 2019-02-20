package evend

import (
	"context"

	"github.com/temoto/vender/engine"
)

type DeviceValve struct {
	Generic
}

func (self *DeviceValve) Init(ctx context.Context) error {
	// TODO read config
	return self.Generic.Init(ctx, 0xc0, "valve")
}

func (self *DeviceValve) NewMove(position uint8) engine.Doer {
	return engine.Func{F: func(ctx context.Context) error {
		arg := []byte{0x03, position, 0x00}
		return self.Generic.CommandAction(ctx, arg)
	}}
}

// TODO poll, returns 40, 44
// TODO c4
// TODO c5

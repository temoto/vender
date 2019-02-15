package evend

import (
	"context"

	"github.com/temoto/vender/helpers/msync"
	"github.com/temoto/vender/engine"
)

type DeviceValve struct {
	g DeviceGeneric
}

func (self *DeviceValve) Init(ctx context.Context) error {
	// TODO read config
	self.g = DeviceGeneric{}
	return self.g.Init(ctx, 0xc0, "valve")
}

func (self *DeviceValve) ReadyChan() <-chan msync.Nothing {
	return self.g.ready
}

func (self *DeviceValve) NewMove(position uint8) engine.Doer {
	return engine.Func{F: func(ctx context.Context) error {
		arg := []byte{0x03, position, 0x00}
		return self.g.CommandAction(ctx, arg)
	}}
}

// TODO poll, returns 40, 44
// TODO c4
// TODO c5

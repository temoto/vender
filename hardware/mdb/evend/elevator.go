package evend

import (
	"context"

	"github.com/temoto/vender/helpers/msync"
	"github.com/temoto/vender/engine"
)

type DeviceElevator struct {
	g DeviceGeneric
}

func (self *DeviceElevator) Init(ctx context.Context) error {
	// TODO read config
	self.g = DeviceGeneric{}
	return self.g.Init(ctx, 0xd0, "elevator")
}

func (self *DeviceElevator) ReadyChan() <-chan msync.Nothing {
	return self.g.ready
}

func (self *DeviceElevator) NewMove(position uint8) engine.Doer {
	return engine.Func{F: func(ctx context.Context) error {
		arg := []byte{0x03, position, 0}
		return self.g.CommandAction(ctx, arg)
	}}
}

// TODO poll, returns 0d, 050b, 0510

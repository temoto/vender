package evend

import (
	"context"

	"github.com/temoto/vender/helpers/msync"
)

type DeviceHopper struct {
	g DeviceGeneric
}

func (self *DeviceHopper) Init(ctx context.Context) error {
	// TODO read config
	self.g = DeviceGeneric{}
	return self.g.Init(ctx, 0xb8, "hopper")
}

func (self *DeviceHopper) ReadyChan() <-chan msync.Nothing {
	return self.g.ready
}

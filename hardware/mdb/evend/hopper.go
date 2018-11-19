package evend

import (
	"context"

	"github.com/temoto/vender/hardware/mdb"
)

type DeviceHopper struct {
	g DeviceGeneric
}

func (self *DeviceHopper) Init(ctx context.Context, mdber mdb.Mdber) error {
	// TODO read config
	self.g = DeviceGeneric{}
	return self.g.Init(ctx, mdber, 0xb8, "hopper")
}

func (self *DeviceHopper) ReadyChan() <-chan struct{} {
	return self.g.ready
}

// TODO
func (self *DeviceHopper) Command1(args ...byte) error {
	return self.g.CommandAction(append([]byte{0x01}, args...))
}

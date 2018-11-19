package evend

import (
	"context"

	"github.com/temoto/vender/hardware/mdb"
)

type DeviceCoffee struct {
	g DeviceGeneric
}

func (self *DeviceCoffee) Init(ctx context.Context, mdber mdb.Mdber) error {
	// TODO read config
	self.g = DeviceGeneric{}
	return self.g.Init(ctx, mdber, 0xe8, "coffee")
}

func (self *DeviceCoffee) ReadyChan() <-chan struct{} {
	return self.g.ready
}

func (self *DeviceCoffee) CommandMove(position uint8) error {
	panic("TODO")
	// return self.g.CommandAction([]byte{0x03, position, 0x00})
}

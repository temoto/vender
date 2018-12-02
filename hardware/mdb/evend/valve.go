package evend

import (
	"context"

	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/helpers/msync"
)

type DeviceValve struct {
	g DeviceGeneric
}

func (self *DeviceValve) Init(ctx context.Context, mdber mdb.Mdber) error {
	// TODO read config
	self.g = DeviceGeneric{}
	return self.g.Init(ctx, mdber, 0xc0, "valve")
}

func (self *DeviceValve) ReadyChan() <-chan msync.Nothing {
	return self.g.ready
}

func (self *DeviceValve) CommandMove(position uint8) error {
	return self.g.CommandAction([]byte{0x03, position, 0x00})
}

// TODO poll, returns 40, 44
// TODO c4
// TODO c5

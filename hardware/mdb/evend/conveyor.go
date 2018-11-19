package evend

import (
	"context"

	"github.com/temoto/vender/hardware/mdb"
)

type DeviceConveyor struct {
	g DeviceGeneric
}

func (self *DeviceConveyor) Init(ctx context.Context, mdber mdb.Mdber) error {
	// TODO read config
	self.g = DeviceGeneric{}
	return self.g.Init(ctx, mdber, 0xd8, "conveyor")
}

func (self *DeviceConveyor) ReadyChan() <-chan struct{} {
	return self.g.ready
}

func (self *DeviceConveyor) CommandMove(position uint16) error {
	return self.g.CommandAction([]byte{0x01, byte(position >> 8), byte(position & 0xff)})
}

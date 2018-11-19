package evend

import (
	"context"

	"github.com/temoto/vender/hardware/mdb"
)

type DeviceCup struct {
	g DeviceGeneric
}

func (self *DeviceCup) Init(ctx context.Context, mdber mdb.Mdber) error {
	// TODO read config
	self.g = DeviceGeneric{}
	return self.g.Init(ctx, mdber, 0xe0, "cup")
}

func (self *DeviceCup) ReadyChan() <-chan struct{} {
	return self.g.ready
}

func (self *DeviceCup) CommandTODO1() error {
	return self.g.CommandAction([]byte{0x02})
}

func (self *DeviceCup) CommandTODO2() error {
	return self.g.CommandAction([]byte{0x04})
}

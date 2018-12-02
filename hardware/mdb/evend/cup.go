package evend

import (
	"context"

	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/helpers/msync"
)

type DeviceCup struct {
	g DeviceGeneric
}

func (self *DeviceCup) Init(ctx context.Context, mdber mdb.Mdber) error {
	// TODO read config
	self.g = DeviceGeneric{}
	return self.g.Init(ctx, mdber, 0xe0, "cup")
}

func (self *DeviceCup) ReadyChan() <-chan msync.Nothing {
	return self.g.ready
}

func (self *DeviceCup) CommandDispense() error {
	return self.g.CommandAction([]byte{0x01})
}

func (self *DeviceCup) CommandTODO2() error {
	return self.g.CommandAction([]byte{0x04})
}

// TODO poll, returns 04, 24

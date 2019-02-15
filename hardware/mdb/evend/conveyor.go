package evend

import (
	"context"

	"github.com/temoto/vender/helpers/msync"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
)

type DeviceConveyor struct {
	g DeviceGeneric
}

func (self *DeviceConveyor) Init(ctx context.Context) error {
	// TODO read config
	self.g = DeviceGeneric{}
	return self.g.Init(ctx, 0xd8, "conveyor")
}

func (self *DeviceConveyor) ReadyChan() <-chan msync.Nothing {
	return self.g.ready
}

func (self *DeviceConveyor) NewMove(position uint16) do.Doer {
	return do.Func{F: func(ctx context.Context) error {
		arg := []byte{0x01, byte(position >> 8), byte(position & 0xff)}
		return self.g.CommandAction(ctx, arg)
	}}
}

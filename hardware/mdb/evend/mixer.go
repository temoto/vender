package evend

import (
	"context"
	"time"

	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/helpers/msync"
)

type DeviceMixer struct {
	g DeviceGeneric
}

func (self *DeviceMixer) Init(ctx context.Context, mdber mdb.Mdber) error {
	// TODO read config
	self.g = DeviceGeneric{}
	return self.g.Init(ctx, mdber, 0xc8, "mixer")
}

func (self *DeviceMixer) ReadyChan() <-chan msync.Nothing {
	return self.g.ready
}

func (self *DeviceMixer) CommandShake(d time.Duration, speed uint8) error {
	argDuration := uint8(d / time.Millisecond / 100)
	return self.g.CommandAction([]byte{0x01, argDuration, speed})
}

func (self *DeviceMixer) CommandFan(on bool) error {
	argOn := uint8(0)
	if on {
		argOn = 1
	}
	return self.g.CommandAction([]byte{0x02, argOn, 0})
}

func (self *DeviceMixer) CommandMove(position uint8) error {
	return self.g.CommandAction([]byte{0x03, position, 0x64})
}

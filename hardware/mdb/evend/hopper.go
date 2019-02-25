package evend

import (
	"context"
	"time"

	"github.com/temoto/vender/engine"
)

type DeviceHopper struct {
	Generic

	runTimeout time.Duration
}

func (self *DeviceHopper) Init(ctx context.Context, addr uint8, nameSuffix string) error {
	// TODO read config
	err := self.Generic.Init(ctx, addr, "hopper"+nameSuffix)
	self.runTimeout = 200 * time.Millisecond
	return err
}
}

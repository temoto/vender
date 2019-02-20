package evend

import (
	"context"
)

type DeviceHopper struct {
	Generic
}

func (self *DeviceHopper) Init(ctx context.Context) error {
	// TODO read config
	return self.Generic.Init(ctx, 0xb8, "hopper")
}

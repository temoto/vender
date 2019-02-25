package evend

import (
	"context"
)

type DeviceValve struct {
	Generic
}

func (self *DeviceValve) Init(ctx context.Context) error {
	// TODO read config
	return self.Generic.Init(ctx, 0xc0, "valve")
}

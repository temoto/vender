package evend

import (
	"context"
)

type DeviceCoffee struct {
	Generic
}

func (self *DeviceCoffee) Init(ctx context.Context) error {
	// TODO read config
	return self.Generic.Init(ctx, 0xe8, "coffee")
}

func (self *DeviceCoffee) CommandMove(position uint8) error {
	panic("TODO")
	// return self.Generic.CommandAction([]byte{0x03, position, 0x00})
}

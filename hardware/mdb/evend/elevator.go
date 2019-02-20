package evend

import (
	"context"

	"github.com/temoto/vender/engine"
)

type DeviceElevator struct {
	Generic

	posCup      uint16
	posConveyor uint16
	posReady    uint16
}

func (self *DeviceElevator) Init(ctx context.Context) error {
	// TODO read config
	err := self.Generic.Init(ctx, 0xd0, "elevator")
	self.posCup = 100
	self.posConveyor = 60
	self.posReady = 0
	return err
}

func (self *DeviceElevator) NewMove(position uint8) engine.Doer {
	return engine.Func{F: func(ctx context.Context) error {
		arg := []byte{0x03, position, 0}
		return self.Generic.CommandAction(ctx, arg)
	}}
}

// TODO poll, returns 0d, 050b, 0510

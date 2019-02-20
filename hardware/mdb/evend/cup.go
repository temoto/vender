package evend

import (
	"context"

	"github.com/temoto/vender/engine"
)

type DeviceCup struct {
	Generic
}

func (self *DeviceCup) Init(ctx context.Context) error {
	// TODO read config
	return self.Generic.Init(ctx, 0xe0, "cup")
}

func (self *DeviceCup) NewDispense() engine.Doer {
	return engine.Func{F: func(ctx context.Context) error {
		arg := []byte{0x01}
		return self.Generic.CommandAction(ctx, arg)
	}}
}

func (self *DeviceCup) NewTODO_04() engine.Doer {
	return engine.Func{F: func(ctx context.Context) error {
		arg := []byte{0x04}
		return self.Generic.CommandAction(ctx, arg)
	}}
}

// TODO poll, returns 04, 24

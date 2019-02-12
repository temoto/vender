package evend

import (
	"context"

	"github.com/temoto/vender/helpers/msync"
)

type DeviceCup struct {
	g DeviceGeneric
}

func (self *DeviceCup) Init(ctx context.Context) error {
	// TODO read config
	self.g = DeviceGeneric{}
	return self.g.Init(ctx, 0xe0, "cup")
}

func (self *DeviceCup) ReadyChan() <-chan msync.Nothing {
	return self.g.ready
}

func (self *DeviceCup) NewDispense() msync.Doer {
	return msync.DoFunc{F: func(ctx context.Context) error {
		arg := []byte{0x01}
		return self.g.CommandAction(ctx, arg)
	}}
}

func (self *DeviceCup) NewTODO_04() msync.Doer {
	return msync.DoFunc{F: func(ctx context.Context) error {
		arg := []byte{0x04}
		return self.g.CommandAction(ctx, arg)
	}}
}

// TODO poll, returns 04, 24

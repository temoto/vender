package evend

import (
	"context"
	"time"

	"github.com/temoto/vender/helpers/msync"
)

type DeviceMixer struct {
	g DeviceGeneric
}

func (self *DeviceMixer) Init(ctx context.Context) error {
	// TODO read config
	self.g = DeviceGeneric{}
	return self.g.Init(ctx, 0xc8, "mixer")
}

func (self *DeviceMixer) ReadyChan() <-chan msync.Nothing {
	return self.g.ready
}

func (self *DeviceMixer) NewShake(d time.Duration, speed uint8) msync.Doer {
	return msync.DoFunc{F: func(ctx context.Context) error {
		argDuration := uint8(d / time.Millisecond / 100)
		arg := []byte{0x01, argDuration, speed}
		return self.g.CommandAction(ctx, arg)
	}}
}

func (self *DeviceMixer) NewFan(on bool) msync.Doer {
	return msync.DoFunc{F: func(ctx context.Context) error {
		argOn := uint8(0)
		if on {
			argOn = 1
		}
		arg := []byte{0x02, argOn, 0}
		return self.g.CommandAction(ctx, arg)
	}}
}

func (self *DeviceMixer) NewMove(position uint8) msync.Doer {
	return msync.DoFunc{F: func(ctx context.Context) error {
		arg := []byte{0x03, position, 0x64}
		return self.g.CommandAction(ctx, arg)
	}}
}

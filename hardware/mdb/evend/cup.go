package evend

import (
	"context"

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
)

type DeviceCup struct {
	Generic

	busyResponses []mdb.Packet
}

func (self *DeviceCup) Init(ctx context.Context) error {
	// TODO read config
	self.busyResponses = []mdb.Packet{
		mdb.MustPacketFromHex("50", true),
		mdb.MustPacketFromHex("54", true),
	}
	return self.Generic.Init(ctx, 0xe0, "cup")
}

func (self *DeviceCup) NewDispense() engine.Doer {
	return engine.Func{F: func(ctx context.Context) error {
		return self.Generic.CommandAction(ctx, []byte{0x01})
		// TODO check 50 untilempty
	}}
}

func (self *DeviceCup) NewLight(on bool) engine.Doer {
	return engine.Func{F: func(ctx context.Context) error {
		arg := byte(0x02)
		if !on {
			arg = 0x03
		}
		return self.CommandAction(ctx, []byte{arg})
	}}
}

func (self *DeviceCup) NewCheck(on bool) engine.Doer {
	return engine.Func{F: func(ctx context.Context) error {
		return self.Generic.CommandAction(ctx, []byte{0x04})
		// TODO check 50 untilempty
	}}
}

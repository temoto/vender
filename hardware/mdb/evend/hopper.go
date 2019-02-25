package evend

import (
	"context"
	"time"

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
)

type DeviceHopper struct {
	Generic

	busyResponses []mdb.Packet
	runTimeout    time.Duration
}

func (self *DeviceHopper) Init(ctx context.Context, addr uint8, nameSuffix string) error {
	// TODO read config
	self.busyResponses = []mdb.Packet{
		mdb.MustPacketFromHex("50", true),
		mdb.MustPacketFromHex("54", true),
	}
	self.runTimeout = 200 * time.Millisecond

	err := self.Generic.Init(ctx, addr, "hopper"+nameSuffix)
	return err
}

func (self *DeviceHopper) NewRun(units uint8) engine.Doer {
	return engine.Func{Name: "run", F: func(ctx context.Context) error {
		return self.Generic.CommandAction(ctx, []byte{units})
	}}
}

func (self *DeviceHopper) NewRunSync(units uint8) engine.Doer {
	tag := "tx_hopper_run"
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.NewRun(units)).
		Append(self.Generic.dev.NewPollUntilEmpty(tag, self.runTimeout*time.Duration(units), self.busyResponses))
	return tx
}

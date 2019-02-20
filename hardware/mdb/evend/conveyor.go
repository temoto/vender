package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
)

type DeviceConveyor struct {
	Generic

	moveTimeout time.Duration
	posCup      uint16
	posHoppers  [16]uint16
	posElevator uint16
}

var busyResponses = []mdb.Packet{
	mdb.MustPacketFromHex("50", true),
	mdb.MustPacketFromHex("54", true),
}

func (self *DeviceConveyor) Init(ctx context.Context) error {
	// TODO read config
	self.moveTimeout = 10 * time.Second
	self.posCup = 1560
	self.posHoppers[0] = 250
	self.posHoppers[1] = 570
	self.posHoppers[2] = 890
	self.posHoppers[3] = 1210
	self.posElevator = 1895
	err := self.Generic.Init(ctx, 0xd8, "conveyor")

	engine := engine.ContextValueEngine(ctx, engine.ContextKey)
	engine.Register("mdb.evend.conveyor_move_cup", self.NewMoveSync(self.posCup))
	// TODO single action with parameter hopper index
	for i, value := range self.posHoppers {
		engine.Register(fmt.Sprintf("mdb.evend.conveyor_move_hopper(%d)", i+1), self.NewMoveSync(value))
	}
	engine.Register("mdb.evend.conveyor_move_elevator", self.NewMoveSync(self.posElevator))

	return err
}

func (self *DeviceConveyor) NewMove(position uint16) engine.Doer {
	return engine.Func{Name: "move", F: func(ctx context.Context) error {
		arg := []byte{0x01, byte(position >> 8), byte(position & 0xff)}
		return self.Generic.CommandAction(ctx, arg)
	}}
}

func (self *DeviceConveyor) NewMoveSync(position uint16) engine.Doer {
	tag := "tx_conveyor_move"
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.NewMove(position)).
		Append(self.Generic.dev.NewPollUntilEmpty(tag, self.moveTimeout, busyResponses))
	return tx
}

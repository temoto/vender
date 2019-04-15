package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/temoto/vender/engine"
)

type DeviceElevator struct {
	Generic

	timeout     time.Duration
	posCup      uint8
	posConveyor uint8
	posReady    uint8
}

func (self *DeviceElevator) Init(ctx context.Context) error {
	// TODO read config
	self.posCup = 100
	self.posConveyor = 60
	self.posReady = 0
	self.timeout = 10 * time.Second
	err := self.Generic.Init(ctx, 0xd0, "elevator", proto1)

	e := engine.GetEngine(ctx)
	e.Register("mdb.evend.elevator_move_conveyor", self.NewMove(self.posConveyor))
	e.Register("mdb.evend.elevator_move_cup", self.NewMove(self.posCup))
	e.Register("mdb.evend.elevator_move_ready", self.NewMove(self.posReady))

	return err
}

func (self *DeviceElevator) NewMove(position uint8) engine.Doer {
	tag := fmt.Sprintf("mdb.evend.elevator.move:%d", position)
	return engine.NewSeq(tag).
		Append(self.Generic.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x03, position, 0)).
		Append(self.Generic.NewWaitDone(tag, self.timeout))
}

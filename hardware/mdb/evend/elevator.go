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

	engine := engine.ContextValueEngine(ctx, engine.ContextKey)
	engine.Register("mdb.evend.elevator_move_conveyor", self.NewMoveSync(self.posConveyor))
	engine.Register("mdb.evend.elevator_move_cup", self.NewMoveSync(self.posCup))
	engine.Register("mdb.evend.elevator_move_ready", self.NewMoveSync(self.posReady))

	return err
}

func (self *DeviceElevator) NewMove(position uint8) engine.Doer {
	tag := fmt.Sprintf("%s.move:%d", self.dev.Name, position)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		arg := []byte{0x03, position, 0}
		return self.CommandAction(ctx, arg)
	}}
}
func (self *DeviceElevator) NewMoveSync(position uint8) engine.Doer {
	tag := fmt.Sprintf("%s.move_sync:%d", self.dev.Name, position)
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.dev.NewPollUntilEmpty(tag+"/wait-empty", self.timeout, nil)).
		Append(self.NewMove(position)).
		Append(self.NewProto1PollWaitSuccess(tag+"/wait-done", self.timeout))
	return tx
}

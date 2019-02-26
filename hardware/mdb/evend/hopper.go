package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/temoto/vender/engine"
)

type DeviceHopper struct {
	Generic

	runTimeout time.Duration
}

func (self *DeviceHopper) Init(ctx context.Context, addr uint8, nameSuffix string) error {
	// TODO read config
	self.runTimeout = 200 * time.Millisecond
	err := self.Generic.Init(ctx, addr, "hopper"+nameSuffix)

	engine := engine.ContextValueEngine(ctx, engine.ContextKey)
	engine.Register(fmt.Sprintf("mdb.evend.hopper%s_run(2)", nameSuffix), self.NewRun(2))

	return err
}

func (self *DeviceHopper) NewRun(units uint8) engine.Doer {
	return engine.Func{Name: "run", F: func(ctx context.Context) error {
		return self.CommandAction(ctx, []byte{units})
	}}
}

func (self *DeviceHopper) NewRunSync(units uint8) engine.Doer {
	tag := "tx_hopper_run"
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.NewRun(units)).
		Append(self.NewPollWait(tag, self.runTimeout*time.Duration(units), genericPollMiss|genericPollBusy))
	return tx
}

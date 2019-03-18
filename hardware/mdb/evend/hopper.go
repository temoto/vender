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
	err := self.Generic.Init(ctx, addr, "hopper"+nameSuffix, proto2)

	engine := engine.ContextValueEngine(ctx, engine.ContextKey)
	engine.Register(fmt.Sprintf("mdb.evend.hopper%s_run(2)", nameSuffix), self.NewRunSync(2))

	return err
}

func (self *DeviceHopper) NewRun(units uint8) engine.Doer {
	tag := fmt.Sprintf("%s.run:%d", self.dev.Name, units)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		return self.CommandAction([]byte{units})
	}}
}

func (self *DeviceHopper) NewRunSync(units uint8) engine.Doer {
	tag := fmt.Sprintf("%s.run:%d", self.dev.Name, units)
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.DoWaitReady(tag)).
		Append(self.NewRun(units)).
		Append(self.DoWaitDone(tag, self.runTimeout*time.Duration(units)))
	return tx
}

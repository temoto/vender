package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/temoto/vender/engine"
)

type DeviceCoffee struct {
	Generic

	timeout time.Duration
}

func (self *DeviceCoffee) Init(ctx context.Context) error {
	// TODO read config
	self.timeout = 10 * time.Second
	err := self.Generic.Init(ctx, 0xe8, "coffee", proto2)

	engine := engine.ContextValueEngine(ctx, engine.ContextKey)
	engine.Register("mdb.evend.coffee_grind", self.NewGrindSync())
	engine.Register("mdb.evend.coffee_press", self.NewPressSync())
	engine.Register("mdb.evend.coffee_dispose", self.NewReleaseSync())
	engine.Register("mdb.evend.coffee_heat_on", self.NewHeat(true))
	engine.Register("mdb.evend.coffee_heat_off", self.NewHeat(false))

	return err
}

func (self *DeviceCoffee) NewGrind() engine.Doer {
	tag := fmt.Sprintf("%s.grind", self.dev.Name)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		return self.CommandAction([]byte{0x01})
	}}
}
func (self *DeviceCoffee) NewGrindSync() engine.Doer {
	tag := fmt.Sprintf("%s.grind_sync", self.dev.Name)
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.DoWaitReady(tag)).
		Append(self.NewGrind()).
		Append(self.DoWaitDone(tag, self.timeout))
	return tx
}

func (self *DeviceCoffee) NewPress() engine.Doer {
	tag := fmt.Sprintf("%s.press", self.dev.Name)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		return self.CommandAction([]byte{0x02})
	}}
}
func (self *DeviceCoffee) NewPressSync() engine.Doer {
	tag := fmt.Sprintf("%s.press_sync", self.dev.Name)
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.DoWaitReady(tag)).
		Append(self.NewPress()).
		Append(self.DoWaitDone(tag, self.timeout))
	return tx
}

func (self *DeviceCoffee) NewRelease() engine.Doer {
	tag := fmt.Sprintf("%s.release", self.dev.Name)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		return self.CommandAction([]byte{0x03})
	}}
}
func (self *DeviceCoffee) NewReleaseSync() engine.Doer {
	tag := fmt.Sprintf("%s.release_sync", self.dev.Name)
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.DoWaitReady(tag)).
		Append(self.NewRelease()).
		Append(self.DoWaitDone(tag, self.timeout))
	return tx
}

func (self *DeviceCoffee) NewHeat(on bool) engine.Doer {
	tag := fmt.Sprintf("%s.heat:%t", self.dev.Name, on)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		arg := byte(0x05)
		if !on {
			arg = 0x06
		}
		return self.CommandAction([]byte{arg})
	}}
}

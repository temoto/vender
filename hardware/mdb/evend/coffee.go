package evend

import (
	"context"
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
	err := self.Generic.Init(ctx, 0xe8, "coffee")

	engine := engine.ContextValueEngine(ctx, engine.ContextKey)
	engine.Register("mdb.evend.coffee_grind", self.NewGrindSync())
	engine.Register("mdb.evend.coffee_press", self.NewPressSync())
	engine.Register("mdb.evend.coffee_dispose", self.NewDisposeSync())
	engine.Register("mdb.evend.coffee_heat_on", self.NewHeat(true))
	engine.Register("mdb.evend.coffee_heat_off", self.NewHeat(false))

	return err
}

func (self *DeviceCoffee) NewGrind() engine.Doer {
	return engine.Func{Name: "grind", F: func(ctx context.Context) error {
		return self.CommandAction(ctx, []byte{0x01})
	}}
}
func (self *DeviceCoffee) NewGrindSync() engine.Doer {
	tag := "tx_coffee_grid"
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.NewGrind()).
		Append(self.NewPollWait(tag, self.timeout, genericPollMiss))
	return tx
}

func (self *DeviceCoffee) NewPress() engine.Doer {
	return engine.Func{Name: "press", F: func(ctx context.Context) error {
		return self.CommandAction(ctx, []byte{0x02})
	}}
}
func (self *DeviceCoffee) NewPressSync() engine.Doer {
	tag := "tx_coffee_press"
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.NewGrind()).
		Append(self.NewPollWait(tag, self.timeout, genericPollMiss))
	return tx
}

func (self *DeviceCoffee) NewDispose() engine.Doer {
	return engine.Func{Name: "dispose", F: func(ctx context.Context) error {
		return self.CommandAction(ctx, []byte{0x03})
	}}
}
func (self *DeviceCoffee) NewDisposeSync() engine.Doer {
	tag := "tx_coffee_dispose"
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.NewGrind()).
		Append(self.NewPollWait(tag, self.timeout, genericPollMiss))
	return tx
}

func (self *DeviceCoffee) NewHeat(on bool) engine.Doer {
	return engine.Func{Name: "heat", F: func(ctx context.Context) error {
		arg := byte(0x05)
		if !on {
			arg = 0x06
		}
		return self.CommandAction(ctx, []byte{arg})
	}}
}

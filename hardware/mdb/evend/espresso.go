package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/engine/inventory"
	"github.com/temoto/vender/head/state"
)

const DefaultCoffeeRate = 9

type DeviceEspresso struct {
	Generic

	coffeeStock *inventory.Stock
	timeout     time.Duration
}

func (self *DeviceEspresso) Init(ctx context.Context) error {
	config := state.GetConfig(ctx)
	self.timeout = 10 * time.Second
	err := self.Generic.Init(ctx, 0xe8, "espresso", proto2)

	self.coffeeStock = config.Global().Inventory.Register("coffee", DefaultCoffeeRate)

	e := engine.ContextValueEngine(ctx, engine.ContextKey)
	e.Register("mdb.evend.espresso_grind", self.NewGrind())
	e.Register("mdb.evend.espresso_press", self.NewPress())
	e.Register("mdb.evend.espresso_dispose", self.NewRelease())
	e.Register("mdb.evend.espresso_heat_on", self.NewHeat(true))
	e.Register("mdb.evend.espresso_heat_off", self.NewHeat(false))

	return err
}

func (self *DeviceEspresso) NewGrind() engine.Doer {
	const tag = "mdb.evend.espresso.grind"
	tx := engine.NewTree(tag)
	tx.Root.
		Append(self.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x01)).
		Append(self.NewWaitDone(tag, self.timeout))
	return self.coffeeStock.Wrap1(tx)
}

func (self *DeviceEspresso) NewPress() engine.Doer {
	const tag = "mdb.evend.espresso.press"
	tx := engine.NewTree(tag)
	tx.Root.
		Append(self.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x02)).
		Append(self.NewWaitDone(tag, self.timeout))
	return tx
}

func (self *DeviceEspresso) NewRelease() engine.Doer {
	const tag = "mdb.evend.espresso.release"
	tx := engine.NewTree(tag)
	tx.Root.
		Append(self.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x03)).
		Append(self.NewWaitDone(tag, self.timeout))
	return tx
}

func (self *DeviceEspresso) NewHeat(on bool) engine.Doer {
	tag := fmt.Sprintf("mdb.evend.espresso.heat:%t", on)
	arg := byte(0x05)
	if !on {
		arg = 0x06
	}
	tx := engine.NewTree(tag)
	tx.Root.
		Append(self.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, arg))
	return tx
}

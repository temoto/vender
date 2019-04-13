package evend

import (
	"context"
	"fmt"
	"time"

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/engine/inventory"
	"github.com/temoto/vender/head/state"
)

type DeviceCup struct {
	Generic

	cupStock        *inventory.Stock
	dispenseTimeout time.Duration
	ensureTimeout   time.Duration
}

func (self *DeviceCup) Init(ctx context.Context) error {
	config := state.GetConfig(ctx)
	// TODO read config
	self.dispenseTimeout = 5 * time.Second
	self.ensureTimeout = 70 * time.Second
	err := self.Generic.Init(ctx, 0xe0, "cup", proto2)
	if err != nil {
		return err
	}

	self.cupStock = config.Global().Inventory.Register("cup", 1)

	e := engine.GetEngine(ctx)
	e.Register("mdb.evend.cup_dispense_proper", self.NewDispenseProper())
	e.Register("mdb.evend.cup_light_on", self.NewLight(true))
	e.Register("mdb.evend.cup_light_off", self.NewLight(false))
	e.Register("mdb.evend.cup_ensure", self.NewEnsure())

	return err
}

func (self *DeviceCup) NewDispenseProper() engine.Doer {
	const tag = "mdb.evend.cup.dispense_proper"
	tx := engine.NewTree(tag)
	tx.Root.
		Append(self.NewEnsure()).
		Append(self.NewDispense())
	return tx
}

func (self *DeviceCup) NewDispense() engine.Doer {
	const tag = "mdb.evend.cup.dispense"
	tx := engine.NewTree(tag)
	tx.Root.
		Append(self.Generic.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x01)).
		Append(engine.Func0{Name: tag + "/assert-busy", F: func() error {
			time.Sleep(30 * time.Millisecond) // TODO tune
			r := self.dev.Tx(self.dev.PacketPoll)
			if r.E != nil {
				return r.E
			}
			bs := r.P.Bytes()
			if len(bs) != 1 {
				return self.NewErrPollUnexpected(r.P)
			}
			if bs[0] != self.proto2BusyMask {
				self.dev.Log.Errorf("expected BUSY, cup device is broken")
				return self.NewErrPollUnexpected(r.P)
			}
			return nil
		}}).
		Append(self.Generic.NewWaitDone(tag, self.dispenseTimeout))
	return self.cupStock.Wrap1(tx)
}

func (self *DeviceCup) NewLight(on bool) engine.Doer {
	tag := fmt.Sprintf("mdb.evend.cup.light:%t", on)
	arg := byte(0x02)
	if !on {
		arg = 0x03
	}
	return self.Generic.NewAction(tag, arg)
}

func (self *DeviceCup) NewEnsure() engine.Doer {
	const tag = "mdb.evend.cup.ensure"
	tx := engine.NewTree(tag)
	tx.Root.
		Append(self.Generic.NewWaitReady(tag)).
		Append(self.Generic.NewAction(tag, 0x04)).
		Append(self.Generic.NewWaitDone(tag, self.ensureTimeout))
	return tx
}

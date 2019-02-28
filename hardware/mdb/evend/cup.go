package evend

import (
	"context"
	"time"

	"github.com/temoto/vender/engine"
)

type DeviceCup struct {
	Generic

	timeout time.Duration
}

func (self *DeviceCup) Init(ctx context.Context) error {
	// TODO read config
	self.timeout = 5 * time.Second
	err := self.Generic.Init(ctx, 0xe0, "cup")
	if err != nil {
		return err
	}

	err = self.NewPollWait("init", self.timeout, genericPollMiss).Do(ctx)

	engine := engine.ContextValueEngine(ctx, engine.ContextKey)
	engine.Register("mdb.evend.cup_dispense", self.NewDispenseSync())
	engine.Register("mdb.evend.cup_light_on", self.NewLight(true))
	engine.Register("mdb.evend.cup_light_off", self.NewLight(false))
	engine.Register("mdb.evend.cup_check", self.NewCheckSync())

	return err
}

func (self *DeviceCup) NewDispense() engine.Doer {
	return engine.Func{Name: self.dev.Name + ".dispense", F: func(ctx context.Context) error {
		return self.CommandAction(ctx, []byte{0x01})
	}}
}
func (self *DeviceCup) NewDispenseSync() engine.Doer {
	tag := "tx_cup_dispense"
	tx := engine.NewTransaction(tag)
	tx.Root.
		// FIXME don't ignore genericPollMiss
		Append(self.NewPollWait(tag, self.timeout, genericPollMiss)).
		Append(self.NewDispense()).
		Append(engine.Func{Name: self.dev.Name + ".ensure-busy", F: func(ctx context.Context) error {
			time.Sleep(30 * time.Millisecond) // TODO tune
			r := self.dev.DoPollSync(ctx)
			if r.E != nil {
				return r.E
			}
			bs := r.P.Bytes()
			if len(bs) != 1 {
				return self.NewErrPollUnexpected(r.P)
			}
			sansMiss := bs[0] &^ genericPollMiss
			if sansMiss != genericPollBusy {
				self.dev.Log.Errorf("expected BUSY, cup device is broken")
				return self.NewErrPollUnexpected(r.P)
			}
			return nil
		}}).
		// FIXME don't ignore genericPollMiss
		Append(self.NewPollWait(tag, self.timeout, genericPollMiss))
	return tx
}

func (self *DeviceCup) NewLight(on bool) engine.Doer {
	return engine.Func{Name: self.dev.Name + ".light", F: func(ctx context.Context) error {
		arg := byte(0x02)
		if !on {
			arg = 0x03
		}
		return self.CommandAction(ctx, []byte{arg})
	}}
}

func (self *DeviceCup) NewCheck() engine.Doer {
	return engine.Func{Name: self.dev.Name + ".check", F: func(ctx context.Context) error {
		return self.CommandAction(ctx, []byte{0x04})
	}}
}
func (self *DeviceCup) NewCheckSync() engine.Doer {
	tag := "tx_cup_check"
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.NewCheck()).
		// FIXME don't ignore genericPollMiss
		Append(self.NewPollWait(tag, self.timeout, genericPollMiss))
	return tx
}

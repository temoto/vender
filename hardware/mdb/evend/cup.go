package evend

import (
	"context"
	"fmt"
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
	err := self.Generic.Init(ctx, 0xe0, "cup", proto2)
	if err != nil {
		return err
	}

	err = self.NewProto2PollWait("init", self.timeout, genericPollMiss).Do(ctx)

	engine := engine.ContextValueEngine(ctx, engine.ContextKey)
	engine.Register("mdb.evend.cup_dispense_proper", self.NewDispenseProper())
	engine.Register("mdb.evend.cup_light_on", self.NewLight(true))
	engine.Register("mdb.evend.cup_light_off", self.NewLight(false))
	engine.Register("mdb.evend.cup_ensure", self.NewEnsureSync())

	return err
}

func (self *DeviceCup) NewDispenseProper() engine.Doer {
	tag := fmt.Sprintf("%s.dispense_proper", self.dev.Name)
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.NewEnsureSync()).
		Append(self.NewDispenseSync())
	return tx
}

func (self *DeviceCup) NewDispense() engine.Doer {
	tag := fmt.Sprintf("%s.dispense", self.dev.Name)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		return self.CommandAction(ctx, []byte{0x01})
	}}
}
func (self *DeviceCup) NewDispenseSync() engine.Doer {
	tag := fmt.Sprintf("%s.dispense_sync", self.dev.Name)
	tx := engine.NewTransaction(tag)
	tx.Root.
		// FIXME don't ignore genericPollMiss
		Append(self.NewProto2PollWait(tag+"/wait-ready", self.timeout, genericPollMiss)).
		Append(self.NewDispense()).
		Append(engine.Func{Name: tag + "/assert-busy", F: func(ctx context.Context) error {
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
		Append(self.NewProto2PollWait(tag+"/wait-done", self.timeout, genericPollMiss))
	return tx
}

func (self *DeviceCup) NewLight(on bool) engine.Doer {
	tag := fmt.Sprintf("%s.light:%t", self.dev.Name, on)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		arg := byte(0x02)
		if !on {
			arg = 0x03
		}
		return self.CommandAction(ctx, []byte{arg})
	}}
}

func (self *DeviceCup) NewEnsure() engine.Doer {
	tag := fmt.Sprintf("%s.ensure", self.dev.Name)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		return self.CommandAction(ctx, []byte{0x04})
	}}
}
func (self *DeviceCup) NewEnsureSync() engine.Doer {
	tag := fmt.Sprintf("%s.ensure_sync", self.dev.Name)
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.NewEnsure()).
		// FIXME don't ignore genericPollMiss
		Append(self.NewProto2PollWait(tag+"/wait", self.timeout, genericPollMiss))
	return tx
}

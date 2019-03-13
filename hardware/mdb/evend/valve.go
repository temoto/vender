package evend

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/temoto/vender/engine"
)

const VolUnitMl float32 = 1.538462

const (
	valvePollBusy   = 0x10
	valvePollNotHot = 0x40
)

type DeviceValve struct {
	Generic

	pourTimeout time.Duration
}

func (self *DeviceValve) Init(ctx context.Context) error {
	// TODO read config
	self.pourTimeout = 30 * time.Second
	err := self.Generic.Init(ctx, 0xc0, "valve", proto2)

	engine := engine.ContextValueEngine(ctx, engine.ContextKey)
	engine.Register("mdb.evend.valve_pour_coffee(120)", self.NewPourCoffeeSync(120))
	engine.Register("mdb.evend.valve_pour_cold(120)", self.NewPourColdSync(120))
	engine.Register("mdb.evend.valve_pour_hot(120)", self.NewPourHotSync(120))

	return err
}

func (self *DeviceValve) MlToUnit(ml uint16) byte {
	x := float32(ml) / VolUnitMl
	y := math.Round(float64(x))
	return byte(y)
}

func (self *DeviceValve) NewPourHot(ml uint16) engine.Doer {
	tag := fmt.Sprintf("%s.pour_hot:%d", self.dev.Name, ml)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		arg := []byte{0x01, self.MlToUnit(ml)}
		return self.CommandAction(ctx, arg)
	}}
}
func (self *DeviceValve) NewPourHotSync(ml uint16) engine.Doer {
	tag := fmt.Sprintf("%s.pour_hot_sync:%d", self.dev.Name, ml)
	tx := engine.NewTransaction(tag)
	tx.Root.
		// FIXME don't ignore genericPollMiss
		Append(self.NewProto2PollWait(tag+"/wait-ready", self.pourTimeout, valvePollBusy|valvePollNotHot|genericPollMiss)).
		Append(self.NewPourHot(ml)).
		// FIXME don't ignore genericPollMiss
		Append(self.NewProto2PollWait(tag+"/wait-done", self.pourTimeout, valvePollNotHot|genericPollMiss))
	return tx
}

func (self *DeviceValve) NewPourCold(ml uint16) engine.Doer {
	tag := fmt.Sprintf("%s.pour_cold:%d", self.dev.Name, ml)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		arg := []byte{0x02, self.MlToUnit(ml)}
		return self.CommandAction(ctx, arg)
	}}
}
func (self *DeviceValve) NewPourColdSync(ml uint16) engine.Doer {
	tag := fmt.Sprintf("%s.pour_cold_sync:%d", self.dev.Name, ml)
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.NewProto2PollWait(tag+"/wait-ready", self.pourTimeout, valvePollNotHot|valvePollBusy)).
		Append(self.NewPourCold(ml)).
		Append(self.NewProto2PollWait(tag+"/wait-done", self.pourTimeout, valvePollNotHot))
	return tx
}

func (self *DeviceValve) NewPourCoffee(ml uint16) engine.Doer {
	tag := fmt.Sprintf("%s.pour_coffee:%d", self.dev.Name, ml)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		arg := []byte{0x03, self.MlToUnit(ml)}
		return self.CommandAction(ctx, arg)
	}}
}
func (self *DeviceValve) NewPourCoffeeSync(ml uint16) engine.Doer {
	tag := fmt.Sprintf("%s.pour_coffee_sync:%d", self.dev.Name, ml)
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.NewProto2PollWait(tag+"/wait-ready", self.pourTimeout, valvePollNotHot|valvePollBusy)).
		Append(self.NewPourCoffee(ml)).
		Append(self.NewProto2PollWait(tag+"/wait-done", self.pourTimeout, valvePollNotHot))
	return tx
}

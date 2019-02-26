package evend

import (
	"context"
	"math"
	"time"

	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
)

const VolUnitMl float32 = 1.538462

const (
	valvePollBusy   = 0x10
	valvePollNotHot = 0x40
)

type DeviceValve struct {
	Generic

	lastPoll      byte
	busyResponses []mdb.Packet
	pourTimeout   time.Duration
}

func (self *DeviceValve) Init(ctx context.Context) error {
	// TODO read config
	self.pourTimeout = 30 * time.Second
	err := self.Generic.Init(ctx, 0xc0, "valve")

	engine := engine.ContextValueEngine(ctx, engine.ContextKey)
	engine.Register("mdb.evend.valve_pour_hot[120]", self.NewPourHotSync(120))

	return err
}

func (self *DeviceValve) MlToUnit(ml uint16) byte {
	x := float32(ml) / VolUnitMl
	y := math.Round(float64(x))
	return byte(y)
}

func (self *DeviceValve) NewPourHot(ml uint16) engine.Doer {
	return engine.Func{Name: "pour_hot", F: func(ctx context.Context) error {
		arg := []byte{0x01, self.MlToUnit(ml)}
		return self.CommandAction(ctx, arg)
	}}
}
func (self *DeviceValve) NewPourHotSync(ml uint16) engine.Doer {
	tag := "tx_valve_pour_hot"
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.NewWait(tag, self.pourTimeout, valvePollBusy)).
		Append(self.NewPourHot(ml)).
		Append(self.NewWait(tag, self.pourTimeout, valvePollNotHot))
	return tx
}

func (self *DeviceValve) NewPourCold(ml uint16) engine.Doer {
	return engine.Func{Name: "pour_cold", F: func(ctx context.Context) error {
		arg := []byte{0x02, self.MlToUnit(ml)}
		return self.CommandAction(ctx, arg)
	}}
}
func (self *DeviceValve) NewPourColdSync(ml uint16) engine.Doer {
	tag := "tx_valve_pour_cold"
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.NewWait(tag, self.pourTimeout, valvePollNotHot, valvePollBusy)).
		Append(self.NewPourCold(ml)).
		Append(self.NewWait(tag, self.pourTimeout, valvePollNotHot))
	return tx
}

func (self *DeviceValve) NewPourCoffee(ml uint16) engine.Doer {
	return engine.Func{Name: "pour_coffee", F: func(ctx context.Context) error {
		arg := []byte{0x03, self.MlToUnit(ml)}
		return self.CommandAction(ctx, arg)
	}}
}
func (self *DeviceValve) NewPourCoffeeSync(ml uint16) engine.Doer {
	tag := "tx_valve_pour_coffee"
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.NewWait(tag, self.pourTimeout, valvePollNotHot, valvePollBusy)).
		Append(self.NewPourCoffee(ml)).
		Append(self.NewWait(tag, self.pourTimeout, valvePollNotHot))
	return tx
}

package evend

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/juju/errors"
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

	pourTimeout time.Duration
	tempHot     uint8
}

func (self *DeviceValve) Init(ctx context.Context) error {
	// TODO read config
	self.pourTimeout = 30 * time.Second
	self.proto2BusyMask = valvePollBusy
	self.proto2IgnoreMask = valvePollNotHot
	err := self.Generic.Init(ctx, 0xc0, "valve", proto2)

	engine := engine.ContextValueEngine(ctx, engine.ContextKey)
	engine.Register("mdb.evend.valve_get_temp_hot", self.DoGetTempHot())
	engine.Register("mdb.evend.valve_set_temp_hot(70)", self.DoSetTempHot(70))
	engine.Register("mdb.evend.valve_pour_coffee(120)", self.DoPourCoffeeSync(120))
	engine.Register("mdb.evend.valve_pour_cold(120)", self.DoPourColdSync(120))
	engine.Register("mdb.evend.valve_pour_hot(120)", self.DoPourHotSync(120))
	engine.Register("mdb.evend.valve_cold_open", self.DoValveCold(true))
	engine.Register("mdb.evend.valve_cold_close", self.DoValveCold(false))
	engine.Register("mdb.evend.valve_hot_open", self.DoValveHot(true))
	engine.Register("mdb.evend.valve_hot_close", self.DoValveHot(false))
	engine.Register("mdb.evend.valve_boiler_open", self.DoValveBoiler(true))
	engine.Register("mdb.evend.valve_boiler_close", self.DoValveBoiler(false))
	engine.Register("mdb.evend.valve_pump_coffee_start", self.DoPumpCoffee(true))
	engine.Register("mdb.evend.valve_pump_coffee_stop", self.DoPumpCoffee(false))
	engine.Register("mdb.evend.valve_pump_start", self.DoPump(true))
	engine.Register("mdb.evend.valve_pump_stop", self.DoPump(false))

	return err
}

func (self *DeviceValve) MlToUnit(ml uint16) byte {
	x := float32(ml) / VolUnitMl
	y := math.Round(float64(x))
	return byte(y)
}
func (self *DeviceValve) MlToTimeout(ml uint16) time.Duration {
	const min = 500 * time.Millisecond
	const perMl = 50 * time.Millisecond // FIXME
	return min + time.Duration(ml)*perMl
}

func (self *DeviceValve) DoGetTempHot() engine.Doer {
	tag := fmt.Sprintf("%s.get_temp_hot", self.dev.Name)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		bs := []byte{self.dev.Address + 4, 0x11}
		request := mdb.MustPacketFromBytes(bs, true)
		r := self.dev.Tx(request)
		if r.E != nil {
			self.dev.Log.Errorf("%s mdb request=%s err=%v", self.logPrefix, request.Format(), r.E)
			return r.E
		}
		bs = r.P.Bytes()
		self.dev.Log.Debugf("%s request=%s response=(%d)%s", self.logPrefix, request.Format(), r.P.Len(), r.P.Format())
		if len(bs) != 1 {
			return errors.NotValidf("invalid")
		}
		return nil
	}}
}
func (self *DeviceValve) DoSetTempHot(arg uint8) engine.Doer {
	tag := fmt.Sprintf("%s.set_temp_hot:%d", self.dev.Name, arg)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		bs := []byte{self.dev.Address + 5, 0x10, arg}
		request := mdb.MustPacketFromBytes(bs, true)
		r := self.dev.Tx(request)
		if r.E != nil {
			self.dev.Log.Errorf("%s mdb request=%s err=%v", self.logPrefix, request.Format(), r.E)
			return r.E
		}
		self.dev.Log.Debugf("%s request=%s response=(%d)%s", self.logPrefix, request.Format(), r.P.Len(), r.P.Format())
		return nil
	}}
}

func (self *DeviceValve) doPourCareful(name string, arg1 byte, ml uint16, abort engine.Doer) engine.Doer {
	tagPour := "pour_" + name
	tag := fmt.Sprintf("%s.%s_sync:%d", self.dev.Name, tagPour, ml)
	tx := engine.NewTransaction(tag)
	const cautionPartMl = 20
	tx.Root.
		Append(self.DoWaitReady(tag)).
		Append(engine.Func{Name: tag + "/careful", F: func(ctx context.Context) error {
			if ml > cautionPartMl {
				cautionTimeout := self.MlToTimeout(cautionPartMl)
				err := self.newCommandDoer(tagPour, strconv.Itoa(int(cautionPartMl)), arg1, self.MlToUnit(cautionPartMl)).Do(ctx)
				if err != nil {
					return err
				}
				err = self.DoWaitDone(tag, cautionTimeout).Do(ctx)
				if err != nil {
					abort.Do(ctx)
					return err
				}
				ml -= cautionPartMl
			}
			err := self.newCommandDoer(tagPour, strconv.Itoa(int(ml)), arg1, self.MlToUnit(ml)).Do(ctx)
			if err != nil {
				return err
			}
			err = self.DoWaitDone(tag, self.MlToTimeout(ml)).Do(ctx)
			return err
		}})
	return tx
}

func (self *DeviceValve) DoPourHotSync(ml uint16) engine.Doer {
	tag := fmt.Sprintf("%s.pour_hot_sync:%d", self.dev.Name, ml)
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.DoWaitReady(tag)).
		Append(self.newCommandDoer("pour_hot", strconv.Itoa(int(ml)), 0x01, self.MlToUnit(ml))).
		Append(self.DoWaitDone(tag, self.pourTimeout))
	return tx
}

func (self *DeviceValve) DoPourColdSync(ml uint16) engine.Doer {
	tag := fmt.Sprintf("%s.pour_cold_sync:%d", self.dev.Name, ml)
	tx := engine.NewTransaction(tag)
	tx.Root.
		Append(self.DoWaitReady(tag)).
		Append(self.newCommandDoer("pour_cold", strconv.Itoa(int(ml)), 0x02, self.MlToUnit(ml))).
		Append(self.DoWaitDone(tag, self.pourTimeout))
	return tx
}

func (self *DeviceValve) DoPourCoffeeSync(ml uint16) engine.Doer {
	// tag := fmt.Sprintf("%s.pour_coffee_sync:%d", self.dev.Name, ml)
	// tx := engine.NewTransaction(tag)
	// tx.Root.
	// 	Append(self.DoWaitReady(tag)).
	// 	Append(self.newCommandDoer("pour_coffee", strconv.Itoa(int(ml)), 0x03, self.MlToUnit(ml))).
	// 	Append(self.DoWaitDone(tag, self.pourTimeout))
	// return tx
	return self.doPourCareful("coffee", 0x03, ml, self.DoPumpCoffee(false))
}

func (self *DeviceValve) DoValveCold(open bool) engine.Doer {
	if open {
		return self.newCommandDoer("valve_cold", "open", 0x10, 0x01)
	} else {
		return self.newCommandDoer("valve_cold", "close", 0x10, 0x00)
	}
}
func (self *DeviceValve) DoValveHot(open bool) engine.Doer {
	if open {
		return self.newCommandDoer("valve_hot", "open", 0x11, 0x01)
	} else {
		return self.newCommandDoer("valve_hot", "close", 0x11, 0x00)
	}
}
func (self *DeviceValve) DoValveBoiler(open bool) engine.Doer {
	if open {
		return self.newCommandDoer("valve_boiler", "open", 0x12, 0x01)
	} else {
		return self.newCommandDoer("valve_boiler", "close", 0x12, 0x00)
	}
}
func (self *DeviceValve) DoPumpCoffee(start bool) engine.Doer {
	if start {
		return self.newCommandDoer("pump_coffee", "start", 0x13, 0x01)
	} else {
		return self.newCommandDoer("pump_coffee", "stop", 0x13, 0x00)
	}
}
func (self *DeviceValve) DoPump(start bool) engine.Doer {
	if start {
		return self.newCommandDoer("pump", "start", 0x14, 0x01)
	} else {
		return self.newCommandDoer("pump", "stop", 0x14, 0x00)
	}
}

func (self *DeviceValve) newCommandDoer(cmdName, argName string, arg1, arg2 byte) engine.Doer {
	tag := fmt.Sprintf("%s.%s:%s", self.dev.Name, cmdName, argName)
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		args := []byte{arg1, arg2}
		return self.CommandAction(args)
	}}
}

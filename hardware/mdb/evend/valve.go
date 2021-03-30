package evend

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/helpers/cacheval"
	"github.com/temoto/vender/internal/engine"
	"github.com/temoto/vender/internal/global"
	"github.com/temoto/vender/internal/state"
)

const (
	valvePollBusy   = 0x10
	valvePollNotHot = 0x40
)

type ErrWaterTemperature struct {
	Source  string
	Current int16
	Target  int16
}

func (e *ErrWaterTemperature) Error() string {
	diff := e.Current - e.Target
	return fmt.Sprintf("source=%s current=%d target=%d diff=%d", e.Source, e.Current, e.Target, diff)
}

type DeviceValve struct { //nolint:maligned
	Generic

	cautionPartUnit uint8
	pourTimeout     time.Duration

	doGetTempHot    engine.Doer
	doCheckTempHot  engine.Doer
	DoSetTempHot    engine.FuncArg
	DoPourCold      engine.Doer
	DoPourHot       engine.Doer
	DoPourEspresso  engine.Doer
	tempHot         cacheval.Int32
	tempHotTarget   uint8
	tempHotReported bool
}

func (self *DeviceValve) init(ctx context.Context) error {
	g := state.GetGlobal(ctx)
	valveConfig := &g.Config.Hardware.Evend.Valve
	self.pourTimeout = helpers.IntSecondDefault(valveConfig.PourTimeoutSec, 10*time.Minute) // big default timeout is fine, depend on valve hardware
	tempValid := helpers.IntMillisecondDefault(valveConfig.TemperatureValidMs, 30*time.Second)
	self.tempHot.Init(tempValid)
	self.proto2BusyMask = valvePollBusy
	self.proto2IgnoreMask = valvePollNotHot
	self.Generic.Init(ctx, 0xc0, "valve", proto2)

	self.doGetTempHot = self.newGetTempHot()
	self.doCheckTempHot = engine.Func0{F: func() error { return nil }, V: self.newCheckTempHotValidate(ctx)}
	self.DoSetTempHot = self.newSetTempHot()
	self.DoPourCold = self.newPourCold()
	self.DoPourHot = self.newPourHot()
	self.DoPourEspresso = self.newPourEspresso()

	waterStock, err := g.Inventory.Get("water")
	if err == nil {
		g.Engine.Register("add.water_hot(?)", waterStock.Wrap(self.DoPourHot))
		g.Engine.Register("add.water_cold(?)", waterStock.Wrap(self.DoPourCold))
		g.Engine.Register("add.water_espresso(?)", waterStock.Wrap(self.DoPourEspresso))
		self.cautionPartUnit = uint8(waterStock.TranslateHw(engine.Arg(valveConfig.CautionPartMl)))
	} else {
		self.dev.Log.Errorf("invalid config, stock water not found err=%v", err)
	}

	g.Engine.Register("evend.valve.check_temp_hot", self.doCheckTempHot)
	g.Engine.Register("evend.valve.get_temp_hot", self.doGetTempHot)
	g.Engine.Register("evend.valve.set_temp_hot(?)", self.DoSetTempHot)
	g.Engine.Register("evend.valve.set_temp_hot_config", engine.Func{F: func(ctx context.Context) error {
		d, _, err := self.DoSetTempHot.Apply(engine.Arg(valveConfig.TemperatureHot))
		if err != nil {
			return err
		}
		return g.Engine.Exec(ctx, d)
	}})
	g.Engine.Register("evend.valve.pour_espresso(?)", self.DoPourEspresso.(engine.Doer))
	g.Engine.Register("evend.valve.pour_cold(?)", self.DoPourCold.(engine.Doer))
	g.Engine.Register("evend.valve.pour_hot(?)", self.DoPourHot.(engine.Doer))
	g.Engine.Register("evend.valve.cold_open", self.NewValveCold(true))
	g.Engine.Register("evend.valve.cold_close", self.NewValveCold(false))
	g.Engine.Register("evend.valve.hot_open", self.NewValveHot(true))
	g.Engine.Register("evend.valve.hot_close", self.NewValveHot(false))
	g.Engine.Register("evend.valve.boiler_open", self.NewValveBoiler(true))
	g.Engine.Register("evend.valve.boiler_close", self.NewValveBoiler(false))
	g.Engine.Register("evend.valve.pump_espresso_start", self.NewPumpEspresso(true))
	g.Engine.Register("evend.valve.pump_espresso_stop", self.NewPumpEspresso(false))
	g.Engine.Register("evend.valve.pump_start", self.NewPump(true))
	g.Engine.Register("evend.valve.pump_stop", self.NewPump(false))

	err = self.Generic.FIXME_initIO(ctx)
	return errors.Annotate(err, self.name+".init")
}

// func (self *DeviceValve) UnitToTimeout(unit uint8) time.Duration {
// 	const min = 500 * time.Millisecond
// 	const perUnit = 50 * time.Millisecond // FIXME
// 	return min + time.Duration(unit)*perUnit
// }

func (self *DeviceValve) newGetTempHot() engine.Func {
	tag := self.name + ".get_temp_hot"

	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		bs := []byte{self.dev.Address + 4, 0x11}
		request := mdb.MustPacketFromBytes(bs, true)
		response := mdb.Packet{}
		err := self.Generic.dev.TxKnown(request, &response)
		if err != nil {
			return errors.Annotate(err, tag)
		}
		bs = response.Bytes()
		self.dev.Log.Debugf("%s request=%s response=(%d)%s", tag, request.Format(), response.Len(), response.Format())
		if len(bs) != 1 {
			return errors.NotValidf("%s response=%x", tag, bs)
		}

		temp := int32(bs[0])
		if temp == 0 {
			self.dev.SetErrorCode(1)
			if doSetZero, _, _ := engine.ArgApply(self.DoSetTempHot, 0); doSetZero != nil {
				_ = engine.GetGlobal(ctx).Exec(ctx, doSetZero)
			}
			sensorErr := errors.Errorf("%s current=0 sensor problem", tag)
			if !self.tempHotReported {
				g := state.GetGlobal(ctx)
				g.Error(sensorErr)
				self.tempHotReported = true
			}
			return sensorErr
		}

		self.tempHot.Set(temp)
		return nil
	}}
}

func (self *DeviceValve) newSetTempHot() engine.FuncArg {
	tag := self.name + ".set_temp_hot"

	return engine.FuncArg{Name: tag, F: func(ctx context.Context, arg engine.Arg) error {
		temp := uint8(arg)
		bs := []byte{self.dev.Address + 5, 0x10, temp}
		request := mdb.MustPacketFromBytes(bs, true)
		response := mdb.Packet{}
		err := self.dev.TxCustom(request, &response, mdb.TxOpt{})
		if err != nil {
			return errors.Annotatef(err, "%s target=%d request=%x", tag, temp, request.Bytes())
		}
		self.tempHotTarget = temp
		self.dev.Log.Debugf("%s target=%d request=%x response=%x", tag, temp, request.Bytes(), response.Bytes())
		return nil
	}}
}

func (self *DeviceValve) newPourCareful(name string, arg1 byte, abort engine.Doer) engine.Doer {
	tagPour := "pour_" + name
	tag := fmt.Sprintf("%s.%s", self.name, tagPour)

	doPour := engine.FuncArg{
		Name: tag + "/careful",
		F: func(ctx context.Context, arg engine.Arg) error {
			if arg >= 256 {
				return errors.Errorf("arg=%d overflows hardware units", arg)
			}
			e := engine.GetGlobal(ctx)
			units := uint8(arg)
			if units > self.cautionPartUnit {
				d := self.newCommand(tagPour, strconv.Itoa(int(self.cautionPartUnit)), arg1, self.cautionPartUnit)
				if err := e.Exec(ctx, d); err != nil {
					return err
				}
				d = self.Generic.NewWaitDone(tag, self.pourTimeout)
				if err := e.Exec(ctx, d); err != nil {
					_ = e.Exec(ctx, abort) // TODO likely redundant
					return err
				}
				units -= self.cautionPartUnit
			}
			d := self.newCommand(tagPour, strconv.Itoa(int(units)), arg1, units)
			if err := e.Exec(ctx, d); err != nil {
				return err
			}
			err := e.Exec(ctx, self.Generic.NewWaitDone(tag, self.pourTimeout))
			return err
		}}

	return engine.NewSeq(tag).
		Append(self.Generic.NewWaitReady(tag)).
		Append(doPour)
}

func (self *DeviceValve) newPourEspresso() engine.Doer {
	return self.newPourCareful("espresso", 0x03, self.NewPumpEspresso(false))
}

func (self *DeviceValve) newPourCold() engine.Doer {
	tag := self.name + ".pour_cold"
	return engine.NewSeq(tag).
		Append(self.Generic.NewWaitReady(tag)).
		Append(self.newPour(tag, 0x02)).
		Append(self.Generic.NewWaitDone(tag, self.pourTimeout))
}

func (self *DeviceValve) newPourHot() engine.Doer {
	tag := self.name + ".pour_hot"
	return engine.NewSeq(tag).
		Append(self.Generic.NewWaitReady(tag)).
		Append(self.newPour(tag, 0x01)).
		Append(self.Generic.NewWaitDone(tag, self.pourTimeout))
}

func (self *DeviceValve) NewValveCold(open bool) engine.Doer {
	if open {
		return self.newCommand("valve_cold", "open", 0x10, 0x01)
	} else {
		return self.newCommand("valve_cold", "close", 0x10, 0x00)
	}
}
func (self *DeviceValve) NewValveHot(open bool) engine.Doer {
	if open {
		return self.newCommand("valve_hot", "open", 0x11, 0x01)
	} else {
		return self.newCommand("valve_hot", "close", 0x11, 0x00)
	}
}
func (self *DeviceValve) NewValveBoiler(open bool) engine.Doer {
	if open {
		return self.newCommand("valve_boiler", "open", 0x12, 0x01)
	} else {
		return self.newCommand("valve_boiler", "close", 0x12, 0x00)
	}
}
func (self *DeviceValve) NewPumpEspresso(start bool) engine.Doer {
	if start {
		return self.newCommand("pump_espresso", "start", 0x13, 0x01)
	} else {
		return self.newCommand("pump_espresso", "stop", 0x13, 0x00)
	}
}
func (self *DeviceValve) NewPump(start bool) engine.Doer {
	if start {
		return self.newCommand("pump", "start", 0x14, 0x01)
	} else {
		return self.newCommand("pump", "stop", 0x14, 0x00)
	}
}

func (self *DeviceValve) newPour(tag string, b1 byte) engine.Doer {
	return engine.FuncArg{
		Name: tag,
		F: func(ctx context.Context, arg engine.Arg) error {
			self.dev.Log.Debugf("%s arg=%d", tag, arg)
			bs := []byte{b1, uint8(arg)}
			return self.txAction(bs)
		},
	}
}

func (self *DeviceValve) newCommand(cmdName, argName string, arg1, arg2 byte) engine.Doer {
	tag := fmt.Sprintf("%s.%s:%s", self.name, cmdName, argName)
	return self.Generic.NewAction(tag, arg1, arg2)
}

func (self *DeviceValve) newCheckTempHotValidate(ctx context.Context) func() error {
	g := state.GetGlobal(ctx)
	return func() error {
		tag := self.name + ".check_temp_hot"
		var getErr error
		temp := self.tempHot.GetOrUpdate(func() {
			// Alexm - если отключить давчик температуры, после инита, то ошибок не будет и температура не меняется.
			if getErr = g.Engine.Exec(ctx, self.doGetTempHot); getErr != nil {
				getErr = errors.Annotate(getErr, tag)
				self.dev.Log.Error(getErr)
			}
		})
		global.SetEnvI("temperature", int(temp))
		if getErr != nil {
			if doSetZero, _, _ := engine.ArgApply(self.DoSetTempHot, 0); doSetZero != nil {
				_ = g.Engine.Exec(ctx, doSetZero)
			}
			return getErr
		}

		diff := absDiffU16(uint16(temp), uint16(self.tempHotTarget))
		const tempHotMargin = 10 // FIXME margin from config
		msg := fmt.Sprintf("%s current=%d config=%d diff=%d", tag, temp, self.tempHotTarget, diff)
		self.dev.Log.Debugf(msg)
		if diff > tempHotMargin {
			if !self.tempHotReported {
				g.Error(errors.New(msg))
				self.tempHotReported = true
			}
			return &ErrWaterTemperature{
				Source:  "hot",
				Current: int16(temp),
				Target:  int16(self.tempHotTarget),
			}
		} else if self.tempHotReported {
			// TODO report OK
			self.tempHotReported = false
		}
		return nil
	}
}
